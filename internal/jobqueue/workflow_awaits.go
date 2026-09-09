package jobqueue

import (
	"context"
	"database/sql"
	"time"

	"lumi/internal/agent"
)

type workflowAwaitTarget struct {
	ID, ThreadID, TurnID, RunID     int64
	AwaitUUID, ThreadUUID, TurnUUID string
}

// awaitDomainTask suspends only when the persisted dependency belongs to this
// exact tool invocation, including an idempotent replay of task creation.
func (manager *Manager) awaitDomainTask(ctx context.Context, projectUUID string, result agent.DomainTask, invocation agent.DomainInvocationContext) (agent.DomainTask, error) {
	if !invocation.AwaitCompletion {
		return result, nil
	}
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return result, err
	}
	var exists bool
	err = runtime.sqlDB.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM workflow_awaits a
		JOIN workflow_steps s ON s.workflow_id=a.workflow_id
		JOIN agent_tool_executions x ON x.id=a.tool_execution_id
		JOIN chat_runs r ON r.id=a.chat_run_id
		JOIN chat_turns t ON t.id=a.chat_turn_id
		JOIN chat_threads th ON th.id=a.chat_thread_id
		WHERE s.task_uuid=? AND x.uuid=? AND r.uuid=? AND t.uuid=? AND th.uuid=? AND th.project_id=?
		AND a.status IN ('waiting','ready','resuming')
	)`, result.UUID, invocation.ToolExecutionUUID, invocation.RunUUID, invocation.TurnUUID, invocation.ThreadUUID, runtime.projectID).Scan(&exists)
	if err != nil {
		return result, err
	}
	if !exists {
		return result, taskError(CodeTaskPersistenceFailed, "异步任务缺少对话等待记录", "无法安全等待任务终态，请检查 Workflow 与调用归属。", nil)
	}
	return result, agent.ErrWaitingWorkflow
}

// readyWorkflowAwaitsTx moves an inline Chat dependency across the durable
// Workflow terminal boundary and inserts exactly one active Chat Resume job in
// the same SQLite/River transaction.
func readyWorkflowAwaitsTx(ctx context.Context, runtime *projectRuntime, tx *sql.Tx, taskUUID string, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT a.id,a.chat_thread_id,a.chat_turn_id,a.chat_run_id,a.uuid,th.uuid,t.uuid
		FROM workflow_awaits a
		JOIN workflows w ON w.id=a.workflow_id
		JOIN workflow_steps s ON s.workflow_id=w.id
		JOIN chat_threads th ON th.id=a.chat_thread_id
		JOIN chat_turns t ON t.id=a.chat_turn_id
		WHERE s.task_uuid=? AND a.status='waiting' AND w.status IN ('completed','failed','cancelled','interrupted')
		ORDER BY a.id`, taskUUID)
	if err != nil {
		return err
	}
	var targets []workflowAwaitTarget
	for rows.Next() {
		var target workflowAwaitTarget
		if err := rows.Scan(&target.ID, &target.ThreadID, &target.TurnID, &target.RunID, &target.AwaitUUID, &target.ThreadUUID, &target.TurnUUID); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, target)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, target := range targets {
		runResult, err := tx.ExecContext(ctx, `UPDATE chat_runs SET status='queued',updated_at=? WHERE id=? AND status='in_progress' AND cancel_requested_at IS NULL`, now, target.RunID)
		if err != nil {
			return err
		}
		turnResult, err := tx.ExecContext(ctx, `UPDATE chat_turns SET status='queued',updated_at=? WHERE id=? AND status='in_progress' AND cancel_requested_at IS NULL`, now, target.TurnID)
		if err != nil {
			return err
		}
		runRows, _ := runResult.RowsAffected()
		turnRows, _ := turnResult.RowsAffected()
		if runRows != 1 || turnRows != 1 {
			if _, err := tx.ExecContext(ctx, `UPDATE workflow_awaits SET status='cancelled',cancelled_at=COALESCE(cancelled_at,?),updated_at=? WHERE id=? AND status='waiting'`, now, now, target.ID); err != nil {
				return err
			}
			if _, err := agent.RecomputeThreadStatusTx(ctx, tx, target.ThreadID, now); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_awaits SET status='ready',ready_at=COALESCE(ready_at,?),updated_at=? WHERE id=? AND status='waiting'`, now, now, target.ID); err != nil {
			return err
		}
		jobID, err := runtime.manager.EnqueueAgentTx(ctx, runtime.projectUUID, tx, agent.JobSpec{
			Version: 1, ProjectUUID: runtime.projectUUID, JobKind: agent.JobChatResume,
			ResourceUUID: target.TurnUUID, ThreadUUID: target.ThreadUUID, WakeupUUID: target.AwaitUUID,
		})
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_awaits SET river_job_id=?,updated_at=? WHERE id=?`, jobID, now, target.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET river_job_id=?,updated_at=? WHERE id=?`, jobID, now, target.TurnID); err != nil {
			return err
		}
		if _, err := agent.RecomputeThreadStatusTx(ctx, tx, target.ThreadID, now); err != nil {
			return err
		}
	}
	return nil
}
