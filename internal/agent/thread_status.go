package agent

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"
)

// RecomputeThreadStatusTx is the single status projection for ChatArea
// Threads. Conversation Threads aggregate every active Turn and displayed
// Workflow. Dedicated Workflow Threads additionally mirror their Workflow's
// terminal status.
func RecomputeThreadStatusTx(ctx context.Context, tx *sql.Tx, threadID int64, now time.Time) (string, error) {
	var threadType string
	if err := tx.QueryRowContext(ctx, `SELECT thread_type FROM chat_threads WHERE id=?`, threadID).Scan(&threadType); err != nil {
		return "", err
	}

	var waiting int
	if err := tx.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM chat_turns WHERE thread_id=? AND status='waiting_for_input') OR
		EXISTS(
			SELECT 1 FROM workflow_steps s
			JOIN workflows w ON w.id=s.workflow_id
			WHERE w.thread_id=? AND w.status IN ('queued','running') AND s.status='waiting'
		)`, threadID, threadID).Scan(&waiting); err != nil {
		return "", err
	}
	status := ThreadIdle
	if waiting != 0 {
		status = ThreadWaitingForInput
	} else {
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT
			EXISTS(SELECT 1 FROM chat_turns WHERE thread_id=? AND status IN ('queued','in_progress')) OR
			EXISTS(SELECT 1 FROM workflows WHERE thread_id=? AND status IN ('queued','running'))`, threadID, threadID).Scan(&active); err != nil {
			return "", err
		}
		if active != 0 {
			status = ThreadBusy
		} else if threadType == ThreadTypeWorkflow {
			var workflowStatus string
			err := tx.QueryRowContext(ctx, `SELECT status FROM workflows WHERE thread_id=? ORDER BY updated_at DESC,id DESC LIMIT 1`, threadID).Scan(&workflowStatus)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return "", err
			}
			switch workflowStatus {
			case WorkflowCompleted, WorkflowFailed, WorkflowCancelled, WorkflowInterrupted:
				status = workflowStatus
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status=?,updated_at=? WHERE id=?`, status, now, threadID); err != nil {
		return "", err
	}
	return status, nil
}

func recomputeThreadStatusGormTx(ctx context.Context, tx *gorm.DB, threadID int64, now time.Time) (string, error) {
	sqlTx, ok := tx.Statement.ConnPool.(*sql.Tx)
	if !ok {
		return "", domainError(CodeStateConflict, "Thread 状态无法重算", "数据库事务连接不可用。", nil)
	}
	return RecomputeThreadStatusTx(ctx, sqlTx, threadID, now)
}
