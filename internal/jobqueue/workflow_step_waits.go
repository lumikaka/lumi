package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"lumi/internal/agent"
	"lumi/internal/taskexecution"
)

type workflowStepWait struct {
	ID, StepID, WorkflowID                           int64
	UUID, StepUUID, WorkflowUUID, ThreadUUID, Status string
}

// SuspendWorkflowStep commits a durable dependency set before the Agent worker
// returns. Rechecking the tasks in this transaction closes the race where a
// fast task finished before its dependency was registered.
func (manager *Manager) SuspendWorkflowStep(ctx context.Context, projectUUID, stepUUID, phase string, tasks []agent.DomainTask) error {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return err
	}
	phase = strings.TrimSpace(phase)
	if !isUUIDv7(stepUUID) || phase == "" || len(phase) > 512 || len(tasks) == 0 {
		return taskError(CodeInvalidTask, "工作流等待参数无效", "等待必须引用当前项目的步骤和完整任务集合。", nil)
	}
	tx, err := taskexecution.Begin(ctx, runtime.sqlDB)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var wait workflowStepWait
	var stepStatus, workflowStatus string
	var cancelled bool
	err = tx.QueryRowContext(ctx, `SELECT s.id,w.id,w.uuid,COALESCE(th.uuid,''),s.status,w.status,w.cancel_requested_at IS NOT NULL
		FROM workflow_steps s JOIN workflows w ON w.id=s.workflow_id LEFT JOIN chat_threads th ON th.id=w.thread_id
		WHERE s.uuid=? AND w.project_id=? AND w.kind=?`, stepUUID, runtime.projectID, agent.WorkflowYolo).
		Scan(&wait.StepID, &wait.WorkflowID, &wait.WorkflowUUID, &wait.ThreadUUID, &stepStatus, &workflowStatus, &cancelled)
	if err != nil {
		return err
	}
	if cancelled || workflowStatus != agent.WorkflowRunning || stepStatus != "running" {
		return taskexecution.ErrSuperseded
	}
	wait.UUID, err = newUUIDv7()
	if err != nil {
		return err
	}
	wait.StepUUID, wait.Status = stepUUID, "waiting"
	now := manager.now().UTC()
	result, err := tx.ExecContext(ctx, `INSERT INTO workflow_step_waits(uuid,workflow_step_id,phase_key,generation,status,created_at,updated_at)
		SELECT ?,?,?,COALESCE(MAX(generation),0)+1,'waiting',?,? FROM workflow_step_waits WHERE workflow_step_id=?`, wait.UUID, wait.StepID, phase, now, now, wait.StepID)
	if err != nil {
		return err
	}
	wait.ID, err = result.LastInsertId()
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		if !isUUIDv7(task.UUID) || seen[task.UUID] {
			return taskError(CodeInvalidTask, "工作流依赖无效", "任务 UUID 必须有效且不能重复。", nil)
		}
		seen[task.UUID] = true
		table, column := "production_task_runs", "production_task_run_id"
		if storyDomainTaskKind(task.Kind) {
			table, column = "task_runs", "task_run_id"
		}
		var taskID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM `+table+` WHERE project_id=? AND uuid=? AND kind=?`, runtime.projectID, task.UUID, task.Kind).Scan(&taskID); err != nil {
			return taskError(CodeTaskNotFound, "工作流依赖不存在", "依赖必须属于当前项目且任务类型匹配。", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_step_wait_dependencies(wait_id,`+column+`) VALUES(?,?)`, wait.ID, taskID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='waiting',current_wait_id=?,updated_at=? WHERE id=?`, wait.ID, now, wait.StepID); err != nil {
		return err
	}
	if err := appendWorkflowStepWaitEventTx(ctx, tx, runtime.projectUUID, wait, "step_waiting", "waiting", now); err != nil {
		return err
	}
	if err := runtime.readyWorkflowStepWaitTx(ctx, tx, wait, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	runtime.broadcastWorkflowStepWait(wait)
	return nil
}

// readyWorkflowStepWaitsTx is called inside the task's terminal transaction.
// Events are hints only: readiness always comes from the current SQLite rows,
// so an old execution cannot satisfy a newer retry of the same logical task.
func (runtime *projectRuntime) readyWorkflowStepWaitsTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT a.id,a.uuid,a.workflow_step_id,s.uuid,w.id,w.uuid,COALESCE(th.uuid,''),a.status
		FROM workflow_step_waits a JOIN workflow_steps s ON s.id=a.workflow_step_id AND s.current_wait_id=a.id
		JOIN workflows w ON w.id=s.workflow_id LEFT JOIN chat_threads th ON th.id=w.thread_id
		JOIN workflow_step_wait_dependencies d ON d.wait_id=a.id
		LEFT JOIN task_runs t ON t.id=d.task_run_id LEFT JOIN production_task_runs p ON p.id=d.production_task_run_id
		WHERE w.project_id=? AND a.status='waiting' AND (t.uuid=? OR p.uuid=?) ORDER BY a.id`, runtime.projectID, taskUUID, taskUUID)
	if err != nil {
		return err
	}
	waits, err := scanWorkflowStepWaits(rows)
	if err != nil {
		return err
	}
	for _, wait := range waits {
		if err := runtime.readyWorkflowStepWaitTx(ctx, tx, wait, now); err != nil {
			return err
		}
	}
	return nil
}

func scanWorkflowStepWaits(rows *sql.Rows) ([]workflowStepWait, error) {
	defer rows.Close()
	var waits []workflowStepWait
	for rows.Next() {
		var wait workflowStepWait
		if err := rows.Scan(&wait.ID, &wait.UUID, &wait.StepID, &wait.StepUUID, &wait.WorkflowID, &wait.WorkflowUUID, &wait.ThreadUUID, &wait.Status); err != nil {
			return nil, err
		}
		waits = append(waits, wait)
	}
	return waits, rows.Err()
}

func (runtime *projectRuntime) readyWorkflowStepWaitTx(ctx context.Context, tx *sql.Tx, wait workflowStepWait, now time.Time) error {
	var eligible bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_step_waits a
		JOIN workflow_steps s ON s.id=a.workflow_step_id AND s.current_wait_id=a.id JOIN workflows w ON w.id=s.workflow_id
		WHERE a.id=? AND a.status IN ('waiting','ready') AND s.status IN ('waiting','queued')
		AND w.status IN ('queued','running') AND w.cancel_requested_at IS NULL)`, wait.ID).Scan(&eligible); err != nil {
		return err
	}
	if !eligible {
		return nil
	}
	if wait.Status == "waiting" {
		var dependencyCount, unsettled int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(CASE WHEN COALESCE(t.status,p.status,'') IN ('completed','failed','cancelled','interrupted') THEN 0 ELSE 1 END),0)
			FROM workflow_step_wait_dependencies d LEFT JOIN task_runs t ON t.id=d.task_run_id
			LEFT JOIN production_task_runs p ON p.id=d.production_task_run_id WHERE d.wait_id=?`, wait.ID).Scan(&dependencyCount, &unsettled); err != nil {
			return err
		}
		if dependencyCount == 0 {
			return taskError(CodeTaskPersistenceFailed, "工作流等待缺少依赖", "无法恢复没有任务集合的等待轮次。", nil)
		}
		if unsettled != 0 {
			return nil
		}
		result, err := tx.ExecContext(ctx, `UPDATE workflow_step_waits SET status='ready',ready_at=COALESCE(ready_at,?),updated_at=? WHERE id=? AND status='waiting'`, now, now, wait.ID)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil || changed != 1 {
			return err
		}
		wait.Status = "ready"
		if err := appendWorkflowStepWaitEventTx(ctx, tx, runtime.projectUUID, wait, "step_ready", "queued", now); err != nil {
			return err
		}
	}
	binding, err := runtime.enqueueAgentTx(ctx, runtime.projectUUID, tx, agent.JobSpec{
		Version: 1, ProjectUUID: runtime.projectUUID, JobKind: agent.JobWorkflowStep,
		ResourceUUID: wait.StepUUID, ThreadUUID: wait.ThreadUUID, WakeupUUID: wait.UUID,
	})
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_step_waits SET river_job_id=?,execution_uuid=?,updated_at=? WHERE id=? AND status='ready'`, binding.JobID, binding.NullableUUID(), now, wait.ID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE workflow_steps SET status='queued',river_job_id=?,execution_uuid=?,updated_at=? WHERE id=? AND current_wait_id=? AND status IN ('waiting','queued')`, binding.JobID, binding.NullableUUID(), now, wait.StepID, wait.ID)
	return err
}

func appendWorkflowStepWaitEventTx(ctx context.Context, tx *sql.Tx, projectUUID string, wait workflowStepWait, event, status string, now time.Time) error {
	uuid, err := newUUIDv7()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"project_uuid": projectUUID, "workflow_uuid": wait.WorkflowUUID, "step_uuid": wait.StepUUID, "wait_uuid": wait.UUID, "status": status})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO workflow_events(uuid,workflow_id,step_id,sequence,event_type,payload_json,created_at)
		SELECT ?,?,?,COALESCE(MAX(sequence),0)+1,?,?,? FROM workflow_events WHERE workflow_id=?`, uuid, wait.WorkflowID, wait.StepID, event, string(payload), now, wait.WorkflowID)
	return err
}

func (runtime *projectRuntime) broadcastWorkflowStepWait(wait workflowStepWait) {
	if runtime.manager.hub == nil {
		return
	}
	runtime.manager.hub.Broadcast("project:"+runtime.projectUUID, "workflow:step_changed", map[string]any{
		"project_uuid": runtime.projectUUID, "workflow_uuid": wait.WorkflowUUID, "step_uuid": wait.StepUUID, "thread_uuid": wait.ThreadUUID,
	})
}

// reconcileWorkflowStepWaits repairs wakeups from durable task state before
// Agent startup recovery. Waiting dependencies deliberately get no worker.
func (runtime *projectRuntime) reconcileWorkflowStepWaits(ctx context.Context) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_step_waits SET status='cancelled',cancelled_at=COALESCE(cancelled_at,?),updated_at=?
		WHERE status IN ('waiting','ready') AND EXISTS(SELECT 1 FROM workflow_steps s JOIN workflows w ON w.id=s.workflow_id
		WHERE s.id=workflow_step_waits.workflow_step_id AND (s.current_wait_id IS NOT workflow_step_waits.id OR s.status IN ('completed','failed','cancelled','interrupted') OR w.status IN ('completed','failed','cancelled','interrupted') OR w.cancel_requested_at IS NOT NULL))`, now, now); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.id,a.uuid,a.workflow_step_id,s.uuid,w.id,w.uuid,COALESCE(th.uuid,''),a.status
		FROM workflow_step_waits a JOIN workflow_steps s ON s.id=a.workflow_step_id AND s.current_wait_id=a.id
		JOIN workflows w ON w.id=s.workflow_id LEFT JOIN chat_threads th ON th.id=w.thread_id
		WHERE w.project_id=? AND a.status IN ('waiting','ready') ORDER BY a.id`, runtime.projectID)
	if err != nil {
		return err
	}
	waits, err := scanWorkflowStepWaits(rows)
	if err != nil {
		return err
	}
	for _, wait := range waits {
		if wait.Status == "ready" {
			record, err := runtime.findExecutionRecordTx(ctx, tx, "workflow_steps", wait.StepUUID)
			if err != nil {
				return err
			}
			job, err := runtime.jobForBindingTx(ctx, tx, record)
			if err != nil {
				return err
			}
			if job != nil && !riverTerminal(job) {
				continue
			}
		}
		if err := runtime.readyWorkflowStepWaitTx(ctx, tx, wait, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (runtime *projectRuntime) workflowStepWaitingTx(ctx context.Context, tx *sql.Tx, record executionRecord) (bool, error) {
	if record.Table != "workflow_steps" {
		return false, nil
	}
	var waiting bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_steps s JOIN workflow_step_waits a ON a.id=s.current_wait_id AND a.workflow_step_id=s.id WHERE s.id=? AND a.status='waiting')`, record.ID).Scan(&waiting)
	return waiting, err
}

// Waiting workflows receive the same durable task progress hints as direct
// workflows; no polling is needed to discover a stage or progress change.
func (runtime *projectRuntime) broadcastWaitingWorkflowTask(taskUUID string) {
	if runtime.manager.hub == nil {
		return
	}
	rows, err := runtime.sqlDB.QueryContext(context.Background(), `SELECT DISTINCT a.id,a.uuid,a.workflow_step_id,s.uuid,w.id,w.uuid,COALESCE(th.uuid,''),a.status
		FROM workflow_step_waits a JOIN workflow_steps s ON s.id=a.workflow_step_id AND s.current_wait_id=a.id
		JOIN workflows w ON w.id=s.workflow_id LEFT JOIN chat_threads th ON th.id=w.thread_id
		JOIN workflow_step_wait_dependencies d ON d.wait_id=a.id
		LEFT JOIN task_runs t ON t.id=d.task_run_id LEFT JOIN production_task_runs p ON p.id=d.production_task_run_id
		WHERE w.project_id=? AND a.status IN ('waiting','ready') AND (t.uuid=? OR p.uuid=?)`, runtime.projectID, taskUUID, taskUUID)
	if err != nil {
		return
	}
	waits, err := scanWorkflowStepWaits(rows)
	if err != nil {
		return
	}
	for _, wait := range waits {
		runtime.broadcastWorkflowStepWait(wait)
	}
}
