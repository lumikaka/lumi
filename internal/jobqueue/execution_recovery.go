package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"lumi/internal/agent"
	"lumi/internal/production"
	"lumi/internal/taskexecution"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver"
	"github.com/riverqueue/river/riverdriver/riversqlite"
	"github.com/riverqueue/river/rivertype"
)

var activeExecutionStates = []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateRetryable, rivertype.JobStateScheduled}

func (runtime *projectRuntime) enqueueProductRecordTx(ctx context.Context, tx *sql.Tx, record executionRecord) error {
	var snapshot string
	var maxAttempts int
	if err := tx.QueryRowContext(ctx, `SELECT input_snapshot,max_attempts FROM `+record.Table+` WHERE id=?`, record.ID).Scan(&snapshot, &maxAttempts); err != nil {
		return err
	}
	var args river.JobArgs
	queue := ""
	switch record.Table {
	case "task_runs":
		var input storyGenerationSnapshot
		if json.Unmarshal([]byte(snapshot), &input) != nil || !validStoryTaskKind(record.Kind) || input.ProjectUUID != runtime.projectUUID || input.Version < 1 || input.Version > 3 ||
			(record.Kind == KindStoryChapterGeneration && input.ChapterUUID != record.ResourceUUID) || (record.Kind != KindStoryChapterGeneration && input.WorkflowKind != record.Kind) {
			return taskError(CodeInvalidTask, "任务输入快照无法恢复", "请重新创建任务。", nil)
		}
		args = riverArgs{ProjectUUID: runtime.projectUUID, TaskUUID: record.UUID, TaskKind: record.Kind, ResourceUUID: record.ResourceUUID}
		queue = QueueStory
	case "production_task_runs":
		var input production.GenerationSnapshot
		if json.Unmarshal([]byte(snapshot), &input) != nil || !validProductionKind(record.Kind) || input.ProjectUUID != runtime.projectUUID || input.Kind != record.Kind || input.ResourceUUID != record.ResourceUUID || input.Version < 1 || input.Version > 5 {
			return taskError(CodeInvalidTask, "生产任务输入快照无法恢复", "请重新创建任务。", nil)
		}
		args = productionArgs{ProjectUUID: runtime.projectUUID, TaskUUID: record.UUID, TaskKind: record.Kind, ResourceUUID: record.ResourceUUID}
		queue = QueueProduction
	case "asset_maintenance_runs":
		var input maintenanceSnapshot
		if json.Unmarshal([]byte(snapshot), &input) != nil || input.Version != 1 || !validMaintenanceKind(record.Kind) || record.Kind == KindAssetGCApply {
			return taskError(CodeInvalidTask, "维护任务不能自动重放", "请重新发起维护操作。", nil)
		}
		args = maintenanceArgs{ProjectUUID: runtime.projectUUID, TaskUUID: record.UUID, MaintenanceKind: record.Kind, ResourceUUID: record.ResourceUUID}
		queue = QueueAssetMaintenance
	default:
		return taskexecution.ErrSuperseded
	}
	inserted, err := runtime.insertExecutionTx(ctx, tx, args, &river.InsertOpts{Queue: queue, MaxAttempts: maxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: activeExecutionStates}})
	if err != nil {
		return err
	}
	if !record.owns(inserted.Job, false) {
		return taskError(CodeTaskConflict, "资源已有其他执行", "请等待当前资源任务结束。", nil)
	}
	binding := executionBindingFor(inserted.Job)
	_, err = tx.ExecContext(ctx, `UPDATE `+record.Table+` SET river_job_id=?,execution_uuid=?,attempt=?,updated_at=? WHERE id=?`, binding.JobID, binding.NullableUUID(), inserted.Job.Attempt, runtime.manager.now().UTC(), record.ID)
	return err
}

func (runtime *projectRuntime) waitExecutionRetryBoundary(ctx context.Context, table, uuid string) (err error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	defer func() {
		if errors.Is(err, context.DeadlineExceeded) {
			err = taskError(CodeTaskStateConflict, "上次执行仍在收尾", "请稍后重试；未创建第二个执行。", err)
		}
	}()
	for {
		tx, err := taskexecution.Begin(ctx, runtime.sqlDB)
		if err != nil {
			return err
		}
		record, err := runtime.findExecutionRecordTx(ctx, tx, table, uuid)
		if err != nil {
			tx.Rollback()
			return err
		}
		// A concurrent retry has already won. The caller rechecks this state
		// in its insertion transaction and returns the existing execution.
		if record.Status == StatusQueued || record.Status == StatusRunning {
			return tx.Rollback()
		}
		job, err := runtime.jobForBindingTx(ctx, tx, record)
		if err != nil {
			tx.Rollback()
			return err
		}
		job, err = runtime.reclaimPreviousRuntimeExecutionTx(ctx, tx, record, job)
		if err != nil {
			tx.Rollback()
			return err
		}
		if job == nil || riverTerminal(job) {
			if err := clearExecutionRecordTx(ctx, tx, record); err != nil {
				tx.Rollback()
				return err
			}
			return tx.Commit()
		}
		_ = tx.Rollback()
		select {
		case <-ctx.Done():
			return taskError(CodeTaskStateConflict, "上次执行仍在收尾", "请稍后重试；未创建第二个执行。", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// Lumi holds an exclusive project file lock, and Manager never replaces a
// runtime until its River client has stopped successfully. Under those two
// constraints, a running job last claimed by a different client is left over
// from a stopped runtime. A different leader alone would not prove this.
// Only explicit retries of terminal business owners reach this path. Keep
// current-client jobs (including the gap before Work registers its cancel
// handle), unknown claimants, and locally registered work on the normal wait
// path. All identity checks and cancellation use the same SQLite transaction.
func (runtime *projectRuntime) reclaimPreviousRuntimeExecutionTx(ctx context.Context, tx *sql.Tx, record executionRecord, job *rivertype.JobRow) (*rivertype.JobRow, error) {
	if job == nil || job.State != rivertype.JobStateRunning || !record.owns(job, true) ||
		(record.Status != StatusFailed && record.Status != StatusCancelled && record.Status != StatusInterrupted) ||
		job.AttemptedAt == nil || len(job.AttemptedBy) == 0 {
		return job, nil
	}
	claimant := job.AttemptedBy[len(job.AttemptedBy)-1]
	if claimant == "" || claimant == runtime.client.ID() {
		return job, nil
	}
	runtime.workMu.Lock()
	_, working := runtime.workCancels[executionWorkKey(record.UUID, record.binding())]
	runtime.workMu.Unlock()
	if working {
		return job, nil
	}
	now := runtime.manager.now().UTC()
	// Use River's state transition API so its JSONB metadata and uniqueness
	// bookkeeping stay intact. Clearing the owner binding below fences late
	// writes, and both changes roll back together if either operation fails.
	jobs, err := riversqlite.New(runtime.sqlDB).UnwrapExecutor(tx).JobSetStateIfRunningMany(ctx, &riverdriver.JobSetStateIfRunningManyParams{
		ID: []int64{job.ID}, Attempt: []*int{nil}, ErrData: [][]byte{nil},
		FinalizedAt: []*time.Time{&now}, ScheduledAt: []*time.Time{nil},
		MetadataDoMerge: []bool{true}, MetadataUpdates: [][]byte{[]byte(`{"lumi_recovery_reason":"retry_previous_runtime"}`)},
		State: []rivertype.JobState{rivertype.JobStateCancelled}, Now: &now,
	})
	if err != nil {
		return nil, err
	}
	if len(jobs) != 1 || !record.owns(jobs[0], true) || jobs[0].State != rivertype.JobStateCancelled {
		return nil, taskexecution.ErrSuperseded
	}
	return jobs[0], nil
}

func (runtime *projectRuntime) activeJobsTx(ctx context.Context, tx *sql.Tx) ([]*rivertype.JobRow, error) {
	params := river.NewJobListParams().States(activeExecutionStates...).First(1000)
	var jobs []*rivertype.JobRow
	for {
		page, err := runtime.client.JobListTx(ctx, tx, params)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, page.Jobs...)
		if len(page.Jobs) < 1000 {
			return jobs, nil
		}
		params = params.After(page.LastCursor)
	}
}

func (runtime *projectRuntime) reconcileExecutionBindings(ctx context.Context) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	records, err := runtime.executionRecordsTx(ctx, tx, nil)
	if err != nil {
		return err
	}
	var recoverable []executionRecord
	counts := map[string]int{}
	var repairedUUIDs []string
	for _, record := range records {
		// A terminal projection can wake another owner in this transaction.
		// Never act on the pre-projection snapshot of its binding.
		record, err = runtime.findExecutionRecordTx(ctx, tx, record.Table, record.UUID)
		if err != nil {
			return err
		}
		job, err := runtime.jobForBindingTx(ctx, tx, record)
		if err != nil {
			return err
		}
		if (record.Table == "task_runs" || record.Table == "production_task_runs" || record.Table == "asset_maintenance_runs") && activeProductStatus(record.Status) {
			var cancelRequested bool
			if err := tx.QueryRowContext(ctx, `SELECT cancel_requested_at IS NOT NULL FROM `+record.Table+` WHERE id=?`, record.ID).Scan(&cancelRequested); err != nil {
				return err
			}
			if cancelRequested {
				if err := runtime.cancelOwnedJobTx(ctx, tx, record.Table, record.UUID); err != nil {
					return err
				}
				// Complete a persisted cancellation intent before considering replay.
				// A still-running River job retains its binding until its terminal event.
				if _, err := tx.ExecContext(ctx, `UPDATE `+record.Table+` SET status='cancelled',completed_at=COALESCE(completed_at,?),updated_at=? WHERE id=? AND status<>'completed'`, runtime.manager.now().UTC(), runtime.manager.now().UTC(), record.ID); err != nil {
					return err
				}
				current, err := runtime.findExecutionRecordTx(ctx, tx, record.Table, record.UUID)
				if err != nil {
					return err
				}
				if err := runtime.syncRepairedProductTx(ctx, tx, current, current.Status, "", ""); err != nil {
					return err
				}
				counts["cancel_intent_completed"]++
				repairedUUIDs = append(repairedUUIDs, record.UUID)
				continue
			}
		}
		if job != nil && !riverTerminal(job) {
			counts["active_retained"]++
			unsafe := record.Table == "asset_maintenance_runs" && record.Kind == KindAssetGCApply
			if record.Table == "task_runs" {
				if err := tx.QueryRowContext(ctx, `SELECT NOT retryable FROM task_runs WHERE id=?`, record.ID).Scan(&unsafe); err != nil {
					return err
				}
			}
			if unsafe && job.Attempt > 0 && activeProductStatus(record.Status) {
				counts["interrupted"]++
				repairedUUIDs = append(repairedUUIDs, record.UUID)
				if err := runtime.cancelOwnedJobTx(ctx, tx, record.Table, record.UUID); err != nil {
					return err
				}
				if err := runtime.interruptDetachedExecutionTx(ctx, tx, record, "破坏性维护已经开始，重启后不能自动重放。"); err != nil {
					return err
				}
			}
			continue
		}
		if job != nil {
			counts["terminal_released"]++
			if err := runtime.repairTerminalProductTx(ctx, tx, record, job); err != nil {
				return err
			}
		}
		if record.JobID.Valid || record.ExecutionUUID.Valid {
			if err := clearExecutionRecordTx(ctx, tx, record); err != nil {
				return err
			}
			repairedUUIDs = append(repairedUUIDs, record.UUID)
			if job == nil {
				counts["stale_released"]++
			}
		}
		if job != nil || !recoverableExecutionStatus(record) {
			continue
		}
		if waiting, err := runtime.workflowStepWaitingTx(ctx, tx, record); err != nil {
			return err
		} else if waiting {
			continue
		}
		recoverable = append(recoverable, record)
	}
	// Clear all stale owners before adopting or creating any execution. An
	// otherwise valid candidate may still be referenced by a later stale row.
	jobs, err := runtime.activeJobsTx(ctx, tx)
	if err != nil {
		return err
	}
	for _, previous := range recoverable {
		record, err := runtime.findExecutionRecordTx(ctx, tx, previous.Table, previous.UUID)
		if err != nil {
			return err
		}
		if record.JobID.Valid || !recoverableExecutionStatus(record) {
			continue
		}
		if waiting, err := runtime.workflowStepWaitingTx(ctx, tx, record); err != nil {
			return err
		} else if waiting {
			continue
		}
		repairedUUIDs = append(repairedUUIDs, record.UUID)
		var matching []*rivertype.JobRow
		for _, candidate := range jobs {
			if record.owns(candidate, false) && (!previous.ExecutionUUID.Valid || previous.ExecutionUUID.String == executionBindingFor(candidate).ExecutionUUID) {
				if record.Table == "chat_turns" {
					matches, err := runtime.matchesChatWakeupTx(ctx, tx, record, candidate)
					if err != nil {
						return err
					}
					if !matches {
						continue
					}
				}
				matching = append(matching, candidate)
			}
		}
		if len(matching) == 1 {
			counts["adopted"]++
			binding := executionBindingFor(matching[0])
			if _, err := tx.ExecContext(ctx, `UPDATE `+record.Table+` SET river_job_id=?,execution_uuid=? WHERE id=?`, binding.JobID, binding.NullableUUID(), record.ID); err != nil {
				return err
			}
			if record.Table == "chat_turns" {
				identity, _ := executionIdentityFor(matching[0])
				if _, err := tx.ExecContext(ctx, `UPDATE workflow_awaits SET river_job_id=?,execution_uuid=? WHERE chat_turn_id=? AND uuid=? AND status IN ('ready','resuming')`, binding.JobID, binding.NullableUUID(), record.ID, identity.WakeupUUID); err != nil {
					return err
				}
			}
			if record.Table == "workflow_steps" && record.WakeupUUID != "" {
				if _, err := tx.ExecContext(ctx, `UPDATE workflow_step_waits SET river_job_id=?,execution_uuid=? WHERE workflow_step_id=? AND uuid=? AND status IN ('ready','consumed')`, binding.JobID, binding.NullableUUID(), record.ID, record.WakeupUUID); err != nil {
					return err
				}
			}
			continue
		}
		if len(matching) > 1 {
			counts["interrupted"]++
			if err := runtime.interruptDetachedExecutionTx(ctx, tx, record, "执行归属不唯一，请显式重试。"); err != nil {
				return err
			}
			continue
		}
		// Agent recovery owns safe replay of persisted tools and wakeups.
		if record.Table == "chat_turns" || record.Table == "workflow_steps" {
			continue
		}
		if record.Table == "task_runs" {
			var retryable bool
			if err := tx.QueryRowContext(ctx, `SELECT retryable FROM task_runs WHERE id=?`, record.ID).Scan(&retryable); err != nil {
				return err
			}
			if !retryable {
				counts["interrupted"]++
				if err := runtime.interruptDetachedExecutionTx(ctx, tx, record, "任务不允许自动重放。"); err != nil {
					return err
				}
				continue
			}
		}
		if err := runtime.enqueueProductRecordTx(ctx, tx, record); err != nil {
			var taskErr *Error
			if !errors.As(err, &taskErr) || (taskErr.Code != CodeInvalidTask && taskErr.Code != CodeTaskConflict) {
				return err
			}
			if err := runtime.interruptDetachedExecutionTx(ctx, tx, record, taskErr.Message); err != nil {
				return err
			}
			counts["interrupted"]++
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+record.Table+` SET status='queued',completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, runtime.manager.now().UTC(), record.ID); err != nil {
			return err
		}
		counts["rebuilt"]++
		repairedUUIDs = append(repairedUUIDs, record.UUID)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	slog.InfoContext(ctx, "project execution recovery committed", "project_uuid", runtime.projectUUID, "counts", counts, "task_uuids", repairedUUIDs)
	return nil
}

func (runtime *projectRuntime) interruptDetachedExecutionTx(ctx context.Context, tx *sql.Tx, record executionRecord, message string) error {
	if record.Table == "chat_turns" || record.Table == "workflow_steps" {
		return agent.InterruptExecutionTx(ctx, tx, record.Table, record.ID, message, runtime.manager.now().UTC())
	}
	job, err := runtime.jobForBindingTx(ctx, tx, record)
	if err != nil {
		return err
	}
	if job == nil || riverTerminal(job) {
		if err := clearExecutionRecordTx(ctx, tx, record); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE `+record.Table+` SET status='interrupted',error_code='queue_execution_interrupted',error_message=?,completed_at=COALESCE(completed_at,?),updated_at=? WHERE id=?`, message, runtime.manager.now().UTC(), runtime.manager.now().UTC(), record.ID)
	return err
}

func recoverableExecutionStatus(record executionRecord) bool {
	switch record.Table {
	case "chat_turns":
		return record.Status == "queued" || record.Status == "in_progress"
	case "workflow_steps":
		return record.Status == "queued" || record.Status == "running" || record.Status == "waiting"
	case "task_runs", "production_task_runs", "asset_maintenance_runs":
		return record.Status == StatusQueued || record.Status == StatusRunning
	}
	return false
}

func (runtime *projectRuntime) matchesChatWakeupTx(ctx context.Context, tx *sql.Tx, record executionRecord, job *rivertype.JobRow) (bool, error) {
	identity, err := executionIdentityFor(job)
	if err != nil {
		return false, err
	}
	var wakeup string
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(
		(SELECT uuid FROM workflow_awaits WHERE chat_turn_id=? AND status IN ('ready','resuming') ORDER BY id LIMIT 1),
		(SELECT uuid FROM chat_user_input_requests WHERE turn_id=? AND status='resuming' ORDER BY id LIMIT 1),'')`, record.ID, record.ID).Scan(&wakeup)
	return err == nil && identity.WakeupUUID == wakeup && ((wakeup == "" && identity.JobKind == agent.JobChatTurn) || (wakeup != "" && identity.JobKind == agent.JobChatResume)), err
}

func (runtime *projectRuntime) repairTerminalProductTx(ctx context.Context, tx *sql.Tx, record executionRecord, job *rivertype.JobRow) error {
	if record.Table == "chat_turns" || record.Table == "workflow_steps" {
		return agent.ReconcileTerminalExecutionTx(ctx, tx, record.Table, record.ID, string(job.State), runtime.manager.now().UTC())
	}
	if record.Table != "task_runs" && record.Table != "production_task_runs" && record.Table != "asset_maintenance_runs" {
		return nil
	}
	if !activeProductStatus(record.Status) || record.Status == StatusWaitingForInput {
		return nil
	}
	status, code, message := StatusInterrupted, "queue_execution_interrupted", "后台执行已结束，但没有完整的完成记录；请按需重试。"
	var hasResult bool
	if record.Table == "task_runs" {
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM story_generation_results WHERE task_run_id=?)`, record.ID).Scan(&hasResult); err != nil {
			return err
		}
	} else if record.Table == "production_task_runs" {
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM premise_generation_steps WHERE task_uuid=? AND status='completed' UNION ALL SELECT 1 FROM comic_image_generations WHERE task_uuid=? AND status='completed' UNION ALL SELECT 1 FROM comic_exports WHERE task_uuid=? AND status='completed')`, record.UUID, record.UUID, record.UUID).Scan(&hasResult); err != nil {
			return err
		}
	}
	if hasResult {
		status, code, message = StatusCompleted, "", ""
	}
	if job.State != rivertype.JobStateCompleted {
		status, code, message = StatusFailed, "queue_execution_failed", "后台执行已结束，请按需重试。"
		var cancelled bool
		if err := tx.QueryRowContext(ctx, `SELECT cancel_requested_at IS NOT NULL FROM `+record.Table+` WHERE id=?`, record.ID).Scan(&cancelled); err != nil {
			return err
		}
		if cancelled {
			status, code, message = StatusCancelled, "", ""
		}
	}
	now := runtime.manager.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE `+record.Table+` SET status=?,progress=CASE WHEN ?='completed' THEN 100 ELSE progress END,completed_at=COALESCE(completed_at,?),updated_at=?,error_code=CASE WHEN error_code='' THEN ? ELSE error_code END,error_message=CASE WHEN error_message='' THEN ? ELSE error_message END WHERE id=?`, status, status, now, now, code, message, record.ID); err != nil {
		return err
	}
	return runtime.syncRepairedProductTx(ctx, tx, record, status, code, message)
}

func (runtime *projectRuntime) syncRepairedProductTx(ctx context.Context, tx *sql.Tx, record executionRecord, status, code, message string) error {
	now := runtime.manager.now().UTC()
	if record.Table == "task_runs" {
		if _, err := tx.ExecContext(ctx, `UPDATE agent_runs SET status=?,error_code=?,error_message=?,completed_at=COALESCE(completed_at,?),updated_at=? WHERE task_run_id=?`, status, code, message, now, now, record.ID); err != nil {
			return err
		}
		var err error
		switch status {
		case StatusCompleted:
			var output map[string]any
			output, err = storyTaskOutputTx(ctx, tx, record.ID)
			if err == nil {
				err = completeStoryTaskWorkflowTx(ctx, tx, record.UUID, output, now)
			}
		case StatusCancelled:
			err = cancelStoryTaskWorkflowTx(ctx, tx, record.UUID, now)
		case StatusFailed:
			err = failStoryTaskWorkflowTx(ctx, tx, record.UUID, code, message, now)
		case StatusInterrupted:
			err = interruptStoryTaskWorkflowTx(ctx, tx, record.UUID, code, message, now)
		}
		if err != nil {
			return err
		}
	} else if record.Table == "production_task_runs" {
		if status != StatusCompleted {
			projectionStatus := status
			if status == StatusInterrupted {
				projectionStatus = StatusFailed
			}
			for _, table := range []string{"premise_generation_steps", "comic_image_generations"} {
				if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET status=?,error_code=?,completed_at=COALESCE(completed_at,?) WHERE task_uuid=? AND status IN ('queued','running')`, projectionStatus, code, now, record.UUID); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE comic_exports SET status=?,error_code=?,completed_at=COALESCE(completed_at,?),expires_at=COALESCE(expires_at,?) WHERE task_uuid=? AND status IN ('queued','running')`, projectionStatus, code, now, production.ExportExpiresAt(now), record.UUID); err != nil {
				return err
			}
		}
		var err error
		switch status {
		case StatusCompleted:
			err = completeComicWorkflowTx(ctx, tx, record.UUID, now)
			if err == nil {
				err = completePremiseAssetWorkflowTx(ctx, tx, record.UUID, now)
			}
		case StatusCancelled:
			err = cancelComicWorkflowTx(ctx, tx, record.UUID, now)
			if err == nil {
				err = cancelPremiseAssetWorkflowTx(ctx, tx, record.UUID, now)
			}
		case StatusFailed:
			err = failComicWorkflowTx(ctx, tx, record.UUID, code, message, now)
			if err == nil {
				err = failPremiseAssetWorkflowTx(ctx, tx, record.UUID, code, message, now)
			}
		case StatusInterrupted:
			err = interruptComicWorkflowTx(ctx, tx, record.UUID, code, message, now)
			if err == nil {
				err = interruptPremiseAssetWorkflowTx(ctx, tx, record.UUID, code, message, now)
			}
		}
		if err != nil {
			return err
		}
		if _, err := syncProductionTaskWorkflowsTx(ctx, runtime, tx, runtime.projectUUID, record.UUID, now); err != nil {
			return err
		}
	}
	return readyWorkflowAwaitsTx(ctx, runtime, tx, record.UUID, now)
}
