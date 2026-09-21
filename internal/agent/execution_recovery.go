package agent

import (
	"context"
	"database/sql"
	"time"

	"lumi/internal/taskexecution"
)

// ReconcileTerminalExecutionTx runs only after the queue adapter has verified
// this owner's current binding and the actual River terminal state. A wait is
// a successful job boundary, not necessarily the end of a Turn or Workflow.
func ReconcileTerminalExecutionTx(ctx context.Context, tx *sql.Tx, table string, id int64, state string, now time.Time) error {
	switch table {
	case "chat_turns":
		var status string
		var waiting bool
		if err := tx.QueryRowContext(ctx, `SELECT t.status,EXISTS(SELECT 1 FROM workflow_awaits a WHERE a.chat_turn_id=t.id AND a.status IN ('waiting','ready')) OR EXISTS(SELECT 1 FROM chat_user_input_requests q WHERE q.turn_id=t.id AND q.status IN ('pending','answered')) FROM chat_turns t WHERE t.id=?`, id).Scan(&status, &waiting); err != nil {
			return err
		}
		if status != TurnQueued && status != TurnInProgress {
			return nil
		}
		if state == "completed" && waiting {
			return nil
		}
		// A committed Run is stronger evidence than a missing Turn projection.
		var runStatus string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM chat_runs WHERE turn_id=?`, id).Scan(&runStatus); err != nil {
			return err
		}
		if runStatus == TurnCompleted || runStatus == TurnFailed || runStatus == TurnCancelled || runStatus == TurnInterrupted {
			if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET status=?,error_code=(SELECT error_code FROM chat_runs WHERE turn_id=?),error_message=(SELECT error_message FROM chat_runs WHERE turn_id=?),completed_at=COALESCE(completed_at,?),updated_at=? WHERE id=?`, runStatus, id, id, now, now, id); err != nil {
				return err
			}
			var threadID int64
			if err := tx.QueryRowContext(ctx, `SELECT thread_id FROM chat_turns WHERE id=?`, id).Scan(&threadID); err != nil {
				return err
			}
			_, err := RecomputeThreadStatusTx(ctx, tx, threadID, now)
			return err
		}
	case "workflow_steps":
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM workflow_steps WHERE id=?`, id).Scan(&status); err != nil {
			return err
		}
		if status != "queued" && status != "running" && status != "waiting" {
			return nil
		}
		if state == "completed" && status == "waiting" {
			return nil
		}
	default:
		return taskexecution.ErrSuperseded
	}
	return InterruptExecutionTx(ctx, tx, table, id, "后台执行已结束，但没有完整的完成记录；请按需重试。", now)
}

// InterruptExecutionTx isolates an owner that cannot be recovered safely.
// The caller has already checked River identity; no numeric queue ID is used
// here. History, tool results and frozen model selection remain intact.
func InterruptExecutionTx(ctx context.Context, tx *sql.Tx, table string, id int64, message string, now time.Time) error {
	var threadID int64
	switch table {
	case "chat_turns":
		if err := tx.QueryRowContext(ctx, `SELECT thread_id FROM chat_turns WHERE id=?`, id).Scan(&threadID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET status='interrupted',river_job_id=NULL,execution_uuid=NULL,error_code='queue_execution_interrupted',error_message=?,completed_at=COALESCE(completed_at,?),updated_at=? WHERE id=?`, message, now, now, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_runs SET status='interrupted',error_code='queue_execution_interrupted',error_message=?,completed_at=COALESCE(completed_at,?),updated_at=? WHERE turn_id=? AND status IN ('queued','in_progress')`, message, now, now, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_awaits SET status='cancelled',cancelled_at=COALESCE(cancelled_at,?),updated_at=? WHERE chat_turn_id=? AND status IN ('waiting','ready','resuming')`, now, now, id); err != nil {
			return err
		}
	case "workflow_steps":
		var workflowID int64
		if err := tx.QueryRowContext(ctx, `SELECT s.workflow_id,COALESCE(w.thread_id,0) FROM workflow_steps s JOIN workflows w ON w.id=s.workflow_id WHERE s.id=?`, id).Scan(&workflowID, &threadID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='interrupted',river_job_id=NULL,execution_uuid=NULL,error_code='queue_execution_interrupted',error_message=?,completed_at=COALESCE(completed_at,?),updated_at=? WHERE id=?`, message, now, now, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='interrupted',error_code='queue_execution_interrupted',error_message=?,completed_at=COALESCE(completed_at,?),updated_at=? WHERE id=? AND status IN ('queued','running')`, message, now, now, workflowID); err != nil {
			return err
		}
	default:
		return taskexecution.ErrSuperseded
	}
	if threadID > 0 {
		_, err := RecomputeThreadStatusTx(ctx, tx, threadID, now)
		return err
	}
	return nil
}
