package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/production"
	"lumi/internal/taskexecution"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riversqlite"
	"github.com/riverqueue/river/rivertype"
	"gorm.io/gorm"
)

// Keep the actual migrated project SQLite and River v0.41.0 adapter, but stop
// workers so cleanup and provider timing cannot mask ID allocation races.
func stoppedExecutionHarness(t *testing.T) (*queueHarness, *projectRuntime) {
	t.Helper()
	h := newQueueHarness(t)
	r := h.runtime(t)
	if err := r.client.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.cancel()
	<-r.eventsDone
	client, err := river.NewClient(riversqlite.New(r.sqlDB), &river.Config{})
	if err != nil {
		t.Fatal(err)
	}
	r.client = client
	jobs, err := client.JobList(t.Context(), river.NewJobListParams().First(1000))
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs.Jobs {
		if _, err := client.JobDelete(t.Context(), job.ID); err != nil {
			t.Fatal(err)
		}
	}
	return h, r
}

func executionFixture(t *testing.T, r *projectRuntime, table, status string, binding taskexecution.Binding) executionRecord {
	t.Helper()
	uuid, _ := newUUIDv7()
	resource, _ := newUUIDv7()
	providerUUID, _ := newUUIDv7()
	now := time.Now().UTC()
	var jobID any
	if binding.JobID != 0 {
		jobID = binding.JobID
	}
	var err error
	switch table {
	case "task_runs":
		snapshot, _ := json.Marshal(storyGenerationSnapshot{Version: 2, ProjectUUID: r.projectUUID, ChapterUUID: resource})
		_, err = r.sqlDB.ExecContext(t.Context(), `INSERT INTO task_runs(uuid,project_id,river_job_id,execution_uuid,kind,resource_uuid,input_version,input_snapshot,status,idempotency_key,retryable,provider_uuid,model,max_attempts,created_at,updated_at) VALUES(?,?,?,?,?,?,2,?,?,?,1,?,'frozen-model',3,?,?)`, uuid, r.projectID, jobID, binding.NullableUUID(), KindStoryChapterGeneration, resource, string(snapshot), status, uuid, providerUUID, now, now)
	case "production_task_runs":
		snapshot, _ := json.Marshal(production.GenerationSnapshot{Version: 1, ProjectUUID: r.projectUUID, Kind: KindPremiseAssetBreakdown, ResourceUUID: resource, Model: "frozen-model", ProviderUUID: providerUUID})
		_, err = r.sqlDB.ExecContext(t.Context(), `INSERT INTO production_task_runs(uuid,project_id,river_job_id,execution_uuid,kind,resource_uuid,input_snapshot,status,idempotency_key,provider_uuid,model,max_attempts,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,'frozen-model',3,?,?)`, uuid, r.projectID, jobID, binding.NullableUUID(), KindPremiseAssetBreakdown, resource, string(snapshot), status, uuid, providerUUID, now, now)
	case "asset_maintenance_runs":
		_, err = r.sqlDB.ExecContext(t.Context(), `INSERT INTO asset_maintenance_runs(uuid,project_id,river_job_id,execution_uuid,kind,resource_uuid,input_snapshot,status,max_attempts,created_at,updated_at) VALUES(?,?,?,?,?,?,?, ?,3,?,?)`, uuid, r.projectID, jobID, binding.NullableUUID(), KindAssetIntegrityScan, resource, `{"version":1}`, status, now, now)
	default:
		t.Fatalf("unsupported fixture table %q", table)
	}
	if err != nil {
		t.Fatal(err)
	}
	return readExecutionFixture(t, r, table, uuid)
}

func readExecutionFixture(t *testing.T, r *projectRuntime, table, uuid string) executionRecord {
	t.Helper()
	tx, err := r.sqlDB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	record, err := r.findExecutionRecordTx(t.Context(), tx, table, uuid)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func enqueueExecutionFixture(t *testing.T, r *projectRuntime, record executionRecord) *rivertype.JobRow {
	t.Helper()
	tx, err := r.sqlDB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := r.enqueueProductRecordTx(t.Context(), tx, record); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	current := readExecutionFixture(t, r, record.Table, record.UUID)
	job, err := r.client.JobGet(t.Context(), current.JobID.Int64)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func seedEightRiverJobs(t *testing.T, r *projectRuntime) {
	t.Helper()
	for id := int64(1); id <= 8; id++ {
		inserted, err := r.client.Insert(t.Context(), gateArgs{Key: fmt.Sprint(id)}, nil)
		if err != nil || inserted.Job.ID != id {
			t.Fatalf("seed ID %d: %+v / %v", id, inserted, err)
		}
	}
}

func TestExecutionIDReuseFirstInsertionSucceeds(t *testing.T) {
	for _, table := range []string{"task_runs", "production_task_runs", "asset_maintenance_runs"} {
		for _, onOpen := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/open_repair_%v", table, onOpen), func(t *testing.T) {
				_, r := stoppedExecutionHarness(t)
				seedEightRiverJobs(t, r)
				old := executionFixture(t, r, table, StatusFailed, taskexecution.Binding{JobID: 9})
				if onOpen {
					if err := r.reconcileExecutionBindings(t.Context()); err != nil {
						t.Fatal(err)
					}
				}
				fresh := executionFixture(t, r, table, StatusQueued, taskexecution.Binding{})
				job := enqueueExecutionFixture(t, r, fresh)
				if job.ID != 9 || !isUUIDv7(executionBindingFor(job).ExecutionUUID) {
					t.Fatalf("new execution = %+v", job)
				}
				history := readExecutionFixture(t, r, table, old.UUID)
				if history.Status != StatusFailed || history.JobID.Valid || history.ExecutionUUID.Valid {
					t.Fatalf("history changed: %+v", history)
				}
				if err := r.reconcileExecutionBindings(t.Context()); err != nil {
					t.Fatal(err)
				}
				current := readExecutionFixture(t, r, table, fresh.UUID)
				if current.binding() != executionBindingFor(job) {
					t.Fatalf("open replaced active execution: %+v", current)
				}
			})
		}
	}
}

func TestExecutionIDRepairRollsBackWithFailedBinding(t *testing.T) {
	_, r := stoppedExecutionHarness(t)
	seedEightRiverJobs(t, r)
	old := executionFixture(t, r, "production_task_runs", StatusFailed, taskexecution.Binding{JobID: 9})
	fresh := executionFixture(t, r, "production_task_runs", StatusQueued, taskexecution.Binding{})
	if _, err := r.sqlDB.ExecContext(t.Context(), `CREATE TEMP TRIGGER reject_execution_binding BEFORE UPDATE OF execution_uuid ON production_task_runs WHEN NEW.execution_uuid IS NOT NULL BEGIN SELECT RAISE(ABORT,'injected write failure'); END`); err != nil {
		t.Fatal(err)
	}
	tx, _ := r.sqlDB.BeginTx(t.Context(), nil)
	err := r.enqueueProductRecordTx(t.Context(), tx, fresh)
	if err == nil {
		t.Fatal("expected injected failure")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if history := readExecutionFixture(t, r, old.Table, old.UUID); history.JobID.Int64 != 9 {
		t.Fatal("repair escaped rollback")
	}
	if _, err := r.client.JobGet(t.Context(), 9); !errors.Is(err, rivertype.ErrNotFound) {
		t.Fatalf("rolled back job survived: %v", err)
	}
	if _, err := r.sqlDB.ExecContext(t.Context(), `DROP TRIGGER reject_execution_binding`); err != nil {
		t.Fatal(err)
	}
	if job := enqueueExecutionFixture(t, r, fresh); job.ID != 9 {
		t.Fatalf("first successful insertion = %d", job.ID)
	}
}

func TestForeignExecutionIsNeverCancelledOrRetried(t *testing.T) {
	for _, table := range []string{"task_runs", "production_task_runs", "asset_maintenance_runs"} {
		t.Run(table, func(t *testing.T) {
			h, r := stoppedExecutionHarness(t)
			foreign, err := r.client.Insert(t.Context(), gateArgs{Key: "foreign-kind"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			owner := executionFixture(t, r, table, StatusQueued, taskexecution.Binding{JobID: foreign.Job.ID})
			switch table {
			case "task_runs":
				_, err = h.queue.CancelTask(t.Context(), r.projectUUID, owner.UUID)
			case "production_task_runs":
				_, err = h.queue.CancelProductionTask(t.Context(), r.projectUUID, owner.UUID)
			case "asset_maintenance_runs":
				_, err = h.queue.CancelMaintenanceTask(t.Context(), r.projectUUID, owner.UUID)
			}
			if err != nil {
				t.Fatal(err)
			}
			foreignNow, err := r.client.JobGet(t.Context(), foreign.Job.ID)
			if err != nil || foreignNow.State != rivertype.JobStateAvailable {
				t.Fatalf("foreign job changed: %+v / %v", foreignNow, err)
			}
			if owner = readExecutionFixture(t, r, table, owner.UUID); owner.JobID.Valid {
				t.Fatal("stale ID was retained")
			}
			if table == "task_runs" {
				_, err = h.queue.RetryTask(t.Context(), r.projectUUID, owner.UUID)
			}
			if table == "production_task_runs" {
				_, err = h.queue.RetryProductionTask(t.Context(), r.projectUUID, owner.UUID)
			}
			if err != nil {
				t.Fatal(err)
			}
			foreignNow, _ = r.client.JobGet(t.Context(), foreign.Job.ID)
			if foreignNow.State != rivertype.JobStateAvailable {
				t.Fatal("retry touched foreign execution")
			}
		})
	}
}

func TestSameTaskAndNumericIDUseDistinctExecutions(t *testing.T) {
	h, r := stoppedExecutionHarness(t)
	owner := executionFixture(t, r, "production_task_runs", StatusQueued, taskexecution.Binding{})
	oldJob := enqueueExecutionFixture(t, r, owner)
	oldCtx, err := r.executionContext(t.Context(), oldJob)
	if err != nil {
		t.Fatal(err)
	}
	oldWork, oldCancel := context.WithCancel(oldCtx)
	defer oldCancel()
	unregisterOld := r.registerExecutionWork(oldCtx, oldCancel)
	if _, err := h.queue.CancelProductionTask(t.Context(), r.projectUUID, owner.UUID); err != nil {
		t.Fatal(err)
	}
	oldJob, err = r.client.JobGet(t.Context(), oldJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.JobDelete(t.Context(), oldJob.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.queue.RetryProductionTask(t.Context(), r.projectUUID, owner.UUID); err != nil {
		t.Fatal(err)
	}
	next := readExecutionFixture(t, r, owner.Table, owner.UUID)
	if next.JobID.Int64 != oldJob.ID || next.ExecutionUUID.String == executionBindingFor(oldJob).ExecutionUUID {
		t.Fatalf("reuse fixture failed: %+v", next)
	}
	newJob, _ := r.client.JobGet(t.Context(), next.JobID.Int64)
	newCtx, err := r.executionContext(t.Context(), newJob)
	if err != nil {
		t.Fatal(err)
	}
	newWork, newCancel := context.WithCancel(newCtx)
	defer newCancel()
	unregisterNew := r.registerExecutionWork(newCtx, newCancel)
	defer unregisterNew()
	unregisterOld()
	if err := r.handleExecutionEvent(t.Context(), &river.Event{Kind: river.EventKindJobCancelled, Job: oldJob}); err != nil {
		t.Fatal(err)
	}
	if err := r.releaseTerminalExecution(t.Context(), oldJob); err != nil {
		t.Fatal(err)
	}
	if _, err := taskexecution.Begin(context.WithoutCancel(oldCtx), r.sqlDB); !errors.Is(err, taskexecution.ErrSuperseded) {
		t.Fatalf("old SQL execution accepted: %v", err)
	}
	err = taskexecution.Transaction(r.store.DB().WithContext(oldCtx), func(tx *gorm.DB) error {
		return tx.Exec(`UPDATE production_task_runs SET status='completed' WHERE uuid=?`, owner.UUID).Error
	})
	if !errors.Is(err, taskexecution.ErrSuperseded) {
		t.Fatalf("old GORM execution accepted: %v", err)
	}
	if err := (&productionWorker{runtime: r}).Work(t.Context(), &river.Job[productionArgs]{JobRow: oldJob, Args: productionArgs{Version: 2, ProjectUUID: r.projectUUID, TaskUUID: owner.UUID, TaskKind: owner.Kind, ResourceUUID: owner.ResourceUUID, ExecutionUUID: executionBindingFor(oldJob).ExecutionUUID}}); err == nil {
		t.Fatal("obsolete worker started")
	}
	if current := readExecutionFixture(t, r, owner.Table, owner.UUID); current.Status != StatusQueued || current.binding() != next.binding() {
		t.Fatalf("old completion changed successor: %+v", current)
	}
	if oldWork.Err() == nil {
		t.Fatal("old execution was not cancelled")
	}
	r.cancelExecutionWork(owner.UUID, next.binding())
	if newWork.Err() == nil {
		t.Fatal("old unregister removed the successor cancellation handle")
	}
}

func TestBusinessTerminalStateDoesNotReleaseActiveRiverJob(t *testing.T) {
	_, r := stoppedExecutionHarness(t)
	owner := executionFixture(t, r, "production_task_runs", StatusQueued, taskexecution.Binding{})
	job := enqueueExecutionFixture(t, r, owner)
	if _, err := r.sqlDB.ExecContext(t.Context(), `UPDATE production_task_runs SET status='failed' WHERE uuid=?`, owner.UUID); err != nil {
		t.Fatal(err)
	}
	fakeTerminal := *job
	fakeTerminal.State = rivertype.JobStateCompleted
	if err := r.releaseTerminalExecution(t.Context(), &fakeTerminal); err != nil {
		t.Fatal(err)
	}
	if current := readExecutionFixture(t, r, owner.Table, owner.UUID); current.binding() != executionBindingFor(job) {
		t.Fatal("active execution was released")
	}
	err := r.waitExecutionRetryBoundary(t.Context(), owner.Table, owner.UUID)
	var taskErr *Error
	if !errors.As(err, &taskErr) || taskErr.Code != CodeTaskStateConflict {
		t.Fatalf("unfinished execution must conflict: %v", err)
	}
	if _, err := r.client.JobCancel(t.Context(), job.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.reconcileExecutionBindings(t.Context()); err != nil {
		t.Fatal(err)
	}
	if current := readExecutionFixture(t, r, owner.Table, owner.UUID); current.JobID.Valid || current.Status != StatusFailed {
		t.Fatalf("lost notification recovery: %+v", current)
	}
}

func TestConcurrentExplicitRetriesCreateOneExecution(t *testing.T) {
	for _, table := range []string{"task_runs", "production_task_runs"} {
		t.Run(table, func(t *testing.T) {
			h, r := stoppedExecutionHarness(t)
			owner := executionFixture(t, r, table, StatusFailed, taskexecution.Binding{JobID: 99})
			var before string
			if err := r.sqlDB.QueryRowContext(t.Context(), `SELECT input_snapshot FROM `+table+` WHERE uuid=?`, owner.UUID).Scan(&before); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			errorsCh := make(chan error, 12)
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					var err error
					if table == "task_runs" {
						_, err = h.queue.RetryTask(t.Context(), r.projectUUID, owner.UUID)
					} else {
						_, err = h.queue.RetryProductionTask(t.Context(), r.projectUUID, owner.UUID)
					}
					errorsCh <- err
				}()
			}
			wg.Wait()
			close(errorsCh)
			for err := range errorsCh {
				if err != nil {
					t.Error(err)
				}
			}
			jobs, err := r.client.JobList(t.Context(), river.NewJobListParams())
			if err != nil || len(jobs.Jobs) != 1 {
				t.Fatalf("concurrent jobs=%+v / %v", jobs, err)
			}
			var after string
			var attempt, maxAttempts int
			if err := r.sqlDB.QueryRowContext(t.Context(), `SELECT input_snapshot,attempt,max_attempts FROM `+table+` WHERE uuid=?`, owner.UUID).Scan(&after, &attempt, &maxAttempts); err != nil {
				t.Fatal(err)
			}
			if before != after || attempt != 0 || maxAttempts != 3 {
				t.Fatal("retry changed snapshot or reused attempt budget")
			}
		})
	}
}

func TestDetachedRecoveryIsIdempotentAndPreservesProgress(t *testing.T) {
	_, r := stoppedExecutionHarness(t)
	active := executionFixture(t, r, "task_runs", StatusRunning, taskexecution.Binding{JobID: 999})
	failed := executionFixture(t, r, "task_runs", StatusFailed, taskexecution.Binding{JobID: 998})
	unsafe := executionFixture(t, r, "asset_maintenance_runs", StatusRunning, taskexecution.Binding{JobID: 997})
	if _, err := r.sqlDB.ExecContext(t.Context(), `UPDATE task_runs SET progress=42 WHERE uuid=?`, active.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.sqlDB.ExecContext(t.Context(), `UPDATE asset_maintenance_runs SET kind=?,max_attempts=1 WHERE uuid=?`, KindAssetGCApply, unsafe.UUID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := r.reconcileExecutionBindings(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	jobs, err := r.client.JobList(t.Context(), river.NewJobListParams())
	if err != nil || len(jobs.Jobs) != 1 {
		t.Fatalf("recovery loop created %+v / %v", jobs, err)
	}
	var progress int
	if err := r.sqlDB.QueryRowContext(t.Context(), `SELECT progress FROM task_runs WHERE uuid=?`, active.UUID).Scan(&progress); err != nil {
		t.Fatal(err)
	}
	if progress != 42 {
		t.Fatalf("progress reset to %d", progress)
	}
	if row := readExecutionFixture(t, r, failed.Table, failed.UUID); row.Status != StatusFailed || row.JobID.Valid {
		t.Fatalf("historical task replayed: %+v", row)
	}
	if row := readExecutionFixture(t, r, unsafe.Table, unsafe.UUID); row.Status != StatusInterrupted || row.JobID.Valid {
		t.Fatalf("unsafe task replayed: %+v", row)
	}
}

func TestRecoveryIsolatesInvalidInputsAndHonorsCancellationIntent(t *testing.T) {
	_, r := stoppedExecutionHarness(t)
	invalid := executionFixture(t, r, "task_runs", StatusQueued, taskexecution.Binding{JobID: 999})
	cancelled := executionFixture(t, r, "production_task_runs", StatusQueued, taskexecution.Binding{JobID: 998})
	if _, err := r.sqlDB.ExecContext(t.Context(), `UPDATE task_runs SET input_snapshot='{}' WHERE uuid=?`, invalid.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.sqlDB.ExecContext(t.Context(), `UPDATE production_task_runs SET cancel_requested_at=? WHERE uuid=?`, time.Now().UTC(), cancelled.UUID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := r.reconcileExecutionBindings(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if row := readExecutionFixture(t, r, invalid.Table, invalid.UUID); row.Status != StatusInterrupted || row.JobID.Valid {
		t.Fatalf("invalid input recovery: %+v", row)
	}
	if row := readExecutionFixture(t, r, cancelled.Table, cancelled.UUID); row.Status != StatusCancelled || row.JobID.Valid {
		t.Fatalf("cancel intent replayed: %+v", row)
	}
	jobs, err := r.client.JobList(t.Context(), river.NewJobListParams())
	if err != nil || len(jobs.Jobs) != 0 {
		t.Fatalf("unsafe executions were enqueued: %+v / %v", jobs, err)
	}
}

func TestLegacyExecutionAndDuplicateBindingUseActualArgs(t *testing.T) {
	for _, legacy := range []bool{true, false} {
		t.Run(fmt.Sprint(legacy), func(t *testing.T) {
			_, r := stoppedExecutionHarness(t)
			owner := executionFixture(t, r, "task_runs", StatusQueued, taskexecution.Binding{})
			args := riverArgs{Version: 1, ProjectUUID: r.projectUUID, TaskUUID: owner.UUID, TaskKind: owner.Kind, ResourceUUID: owner.ResourceUUID}
			opts := &river.InsertOpts{Queue: QueueStory, MaxAttempts: 3, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: activeExecutionStates}}
			tx, _ := r.sqlDB.BeginTx(t.Context(), nil)
			var inserted *rivertype.JobInsertResult
			var err error
			if legacy {
				inserted, err = r.client.InsertTx(t.Context(), tx, args, opts)
			} else {
				inserted, err = r.insertExecutionTx(t.Context(), tx, args, opts)
			}
			if err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
			binding := executionBindingFor(inserted.Job)
			if _, err := tx.ExecContext(t.Context(), `UPDATE task_runs SET river_job_id=?,execution_uuid=? WHERE id=?`, binding.JobID, binding.NullableUUID(), owner.ID); err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			// Propose another UUID, but River must return the existing execution.
			job := enqueueExecutionFixture(t, r, owner)
			if executionBindingFor(job) != binding {
				t.Fatalf("duplicate adopted a temporary UUID: %+v", job)
			}
			if _, err := r.executionContext(t.Context(), job); err != nil {
				t.Fatalf("compatible worker rejected: %v", err)
			}
			if err := r.reconcileExecutionBindings(t.Context()); err != nil {
				t.Fatal(err)
			}
			if row := readExecutionFixture(t, r, owner.Table, owner.UUID); row.binding() != binding {
				t.Fatal("legacy binding replaced on open")
			}
		})
	}
}

func TestAgentRecoveryAdoptsExecutionBeforeWorkersStart(t *testing.T) {
	h, r := stoppedExecutionHarness(t)
	agents := agent.NewService(h.projects, h.queue.providers, nil, h.queue, nil)
	thread, err := agents.CreateThread(t.Context(), r.projectUUID, agent.CreateThreadInput{Title: "恢复测试", ProviderUUID: h.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := agents.CreateTurn(t.Context(), r.projectUUID, thread.UUID, agent.CreateTurnInput{InputText: "保持冻结模型"})
	if err != nil {
		t.Fatal(err)
	}
	before := readExecutionFixture(t, r, "chat_turns", turn.UUID)
	if _, err := r.sqlDB.ExecContext(t.Context(), `UPDATE chat_turns SET river_job_id=NULL,execution_uuid=NULL WHERE uuid=?`, turn.UUID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := r.reconcileExecutionBindings(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := agents.ReconcileWithEnqueue(t.Context(), r.store, r.enqueueAgentTx); err != nil {
			t.Fatal(err)
		}
	}
	if after := readExecutionFixture(t, r, "chat_turns", turn.UUID); after.binding() != before.binding() {
		t.Fatalf("existing Agent execution was replaced: %+v", after)
	}
	jobs, err := r.client.JobList(t.Context(), river.NewJobListParams())
	if err != nil || len(jobs.Jobs) != 1 {
		t.Fatalf("Agent repeated recovery: %+v / %v", jobs, err)
	}
	// A terminal River execution without durable completion cannot be queued
	// again on every open. It becomes an explicit, visible interruption.
	if _, err := r.client.JobCancel(t.Context(), before.JobID.Int64); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := r.reconcileExecutionBindings(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := agents.ReconcileWithEnqueue(t.Context(), r.store, r.enqueueAgentTx); err != nil {
			t.Fatal(err)
		}
	}
	if after := readExecutionFixture(t, r, "chat_turns", turn.UUID); after.Status != agent.TurnInterrupted || after.JobID.Valid {
		t.Fatalf("terminal Agent execution replayed: %+v", after)
	}
}

func TestWorkflowExecutionRecoveryAndForeignCancellation(t *testing.T) {
	h, r := stoppedExecutionHarness(t)
	agents := agent.NewService(h.projects, h.queue.providers, nil, h.queue, nil)
	thread, err := agents.CreateThread(t.Context(), r.projectUUID, agent.CreateThreadInput{Title: "Workflow 恢复", ProviderUUID: h.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	// This Turn was queued before initialization acquired the project.
	turn, err := agents.CreateTurn(t.Context(), r.projectUUID, thread.UUID, agent.CreateTurnInput{InputText: "独立 Turn"})
	if err != nil {
		t.Fatal(err)
	}
	workflowUUID, _ := newUUIDv7()
	stepUUID, _ := newUUIDv7()
	now := time.Now().UTC()
	_, err = r.sqlDB.ExecContext(t.Context(), `INSERT INTO workflows(uuid,project_id,thread_id,kind,title,status,input_snapshot,idempotency_key,provider_uuid,model,created_at,updated_at) SELECT ?,?,id,?,'恢复','queued','{}',?,?,'frozen-model',?,? FROM chat_threads WHERE uuid=?`, workflowUUID, r.projectID, agent.WorkflowYolo, workflowUUID, h.provider.UUID, now, now, thread.UUID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.sqlDB.ExecContext(t.Context(), `INSERT INTO workflow_steps(uuid,workflow_id,step_key,position,status,idempotency_key,created_at,updated_at) SELECT ?,id,'test_step',1,'queued',?,?,? FROM workflows WHERE uuid=?`, stepUUID, stepUUID, now, now, workflowUUID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := agents.ReconcileWithEnqueue(t.Context(), r.store, r.enqueueAgentTx); err != nil {
			t.Fatal(err)
		}
	}
	step := readExecutionFixture(t, r, "workflow_steps", stepUUID)
	if !step.JobID.Valid || !isUUIDv7(step.ExecutionUUID.String) {
		t.Fatalf("missing Workflow execution: %+v", step)
	}
	jobs, err := r.client.JobList(t.Context(), river.NewJobListParams())
	if err != nil || len(jobs.Jobs) != 2 {
		t.Fatalf("Workflow repeated recovery: %+v / %v", jobs, err)
	}
	stepJob, err := r.client.JobGet(t.Context(), step.JobID.Int64)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.executionContext(t.Context(), stepJob); err != nil {
		t.Fatal(err)
	}
	// Same River kind, but a different business owner: repair only the stale
	// Turn. Do not cancel the Workflow Step's job.
	_, err = r.sqlDB.ExecContext(t.Context(), `UPDATE chat_turns SET river_job_id=?,execution_uuid=? WHERE uuid=?`, step.JobID.Int64, step.ExecutionUUID.String, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	err = h.queue.CancelAgentJob(t.Context(), r.projectUUID, taskexecution.Owner{Table: "chat_turns", UUID: turn.UUID, ProjectUUID: r.projectUUID}, step.binding())
	if err != nil {
		t.Fatal(err)
	}
	job, err := r.client.JobGet(t.Context(), step.JobID.Int64)
	if err != nil || job.State != rivertype.JobStateAvailable {
		t.Fatalf("cancel touched another owner: %+v / %v", job, err)
	}
}

func TestStartupRepairKeepsAwaitAndTurnOnTheSameNewExecution(t *testing.T) {
	model := newInlineWorkflowAgentModel()
	env := setupInlineWorkflowTestEnv(t, model)
	thread, err := env.agents.CreateThread(t.Context(), env.project.UUID, agent.CreateThreadInput{Title: "等待恢复", ProviderUUID: env.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := env.agents.CreateTurn(t.Context(), env.project.UUID, thread.UUID, agent.CreateTurnInput{InputText: "请发起章节生成"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-model.storyStarted:
	case <-time.After(8 * time.Second):
		t.Fatal("workflow did not start")
	}
	workflow := waitInlineWorkflow(t, env, thread.UUID)
	r, err := env.queue.runtimeFor(env.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.client.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.cancel()
	<-r.eventsDone
	client, err := river.NewClient(riversqlite.New(r.sqlDB), &river.Config{})
	if err != nil {
		t.Fatal(err)
	}
	r.client = client
	jobs, err := r.client.JobList(t.Context(), river.NewJobListParams().First(1000))
	if err != nil {
		t.Fatal(err)
	}
	var original *rivertype.JobRow
	for _, job := range jobs.Jobs {
		identity, _ := executionIdentityFor(job)
		if identity.ResourceUUID == turn.UUID && identity.JobKind == agent.JobChatTurn {
			original = job
		}
	}
	if original == nil {
		t.Fatal("original Chat execution missing")
	}
	if _, err := r.sqlDB.ExecContext(t.Context(), `UPDATE workflow_steps SET status='completed',output_json='{}' WHERE workflow_id=(SELECT id FROM workflows WHERE uuid=?)`, workflow.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.sqlDB.ExecContext(t.Context(), `UPDATE workflows SET status='completed' WHERE uuid=?`, workflow.UUID); err != nil {
		t.Fatal(err)
	}
	if err := r.reconcileExecutionBindings(t.Context()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := env.agents.ReconcileWithEnqueue(t.Context(), r.store, r.enqueueAgentTx); err != nil {
			t.Fatal(err)
		}
	}
	after := readExecutionFixture(t, r, "chat_turns", turn.UUID)
	var awaitUUID string
	if err := r.sqlDB.QueryRowContext(t.Context(), `SELECT uuid FROM workflow_awaits WHERE chat_turn_id=(SELECT id FROM chat_turns WHERE uuid=?)`, turn.UUID).Scan(&awaitUUID); err != nil {
		t.Fatal(err)
	}
	await := readExecutionFixture(t, r, "workflow_awaits", awaitUUID)
	if !after.JobID.Valid || after.binding() != await.binding() || after.binding() == executionBindingFor(original) {
		t.Fatalf("resume binding split: Turn=%+v Await=%+v", after, await)
	}
	if err := r.handleExecutionEvent(t.Context(), &river.Event{Kind: river.EventKindJobCompleted, Job: original}); err != nil {
		t.Fatal(err)
	}
	if next := readExecutionFixture(t, r, "chat_turns", turn.UUID); next.binding() != after.binding() {
		t.Fatal("old completion removed resumed Turn")
	}
	if next := readExecutionFixture(t, r, "workflow_awaits", awaitUUID); next.binding() != after.binding() {
		t.Fatal("old completion removed resumed Await")
	}
	newJob, _ := r.client.JobGet(t.Context(), after.JobID.Int64)
	identity, _ := executionIdentityFor(newJob)
	if identity.WakeupUUID != awaitUUID {
		t.Fatal("resume lost wakeup deduplication identity")
	}
}

func TestUpgradeOpenRepairsHistoryBeforeFirstPublicSubmission(t *testing.T) {
	h, r := stoppedExecutionHarness(t)
	seedEightRiverJobs(t, r)
	var history []executionRecord
	for _, table := range []string{"task_runs", "production_task_runs", "asset_maintenance_runs"} {
		history = append(history, executionFixture(t, r, table, StatusFailed, taskexecution.Binding{JobID: 9}))
	}
	chapter := h.createChapter(t, "vol01.ch01")
	source, err := production.NewService(r.store, nil).CreatePremiseSource(t.Context(), production.CreateSourceInput{SourceType: "manual", SourceText: "A forest", StyleSnapshot: "ink"})
	if err != nil {
		t.Fatal(err)
	}
	h.queue.WithImageClient(successfulImageProvider{content: productionPNG(t)})
	if err := h.projects.CloseCurrent(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.projects.OpenRecent(t.Context(), h.project.UUID); err != nil {
		t.Fatal(err)
	}
	r = h.runtime(t)
	for _, old := range history {
		row := readExecutionFixture(t, r, old.Table, old.UUID)
		if row.Status != StatusFailed || row.JobID.Valid || row.ExecutionUUID.Valid {
			t.Fatalf("open did not repair history: %+v", row)
		}
	}
	storyTask := h.createTask(t, chapter, "升级后首次提交", "first-after-upgrade")
	productionTask, err := h.queue.CreatePremiseSettingGeneration(t.Context(), r.projectUUID, source.UUID, CreateProductionGenerationInput{ProviderUUID: h.provider.UUID, Prompt: "forest", IdempotencyKey: "first-after-upgrade"})
	if err != nil {
		t.Fatal(err)
	}
	maintenance, err := h.queue.CreateMaintenanceTask(t.Context(), r.projectUUID, CreateMaintenanceInput{Kind: KindAssetIntegrityScan})
	if err != nil {
		t.Fatal(err)
	}
	waitTaskStatus(t, h.queue, r.projectUUID, storyTask.UUID, StatusCompleted)
	waitProductionStatus(t, h.queue, r.projectUUID, productionTask.UUID, StatusCompleted)
	waitMaintenanceStatus(t, h.queue, r.projectUUID, maintenance.UUID, StatusCompleted)
}
