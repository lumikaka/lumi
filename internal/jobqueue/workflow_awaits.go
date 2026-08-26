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
