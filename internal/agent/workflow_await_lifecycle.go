package agent

import (
	"context"
	"database/sql"
	"time"

	"gorm.io/gorm"
)

type workflowAwaitTarget struct {
	ID, ThreadID, TurnID, RunID     int64
	AwaitUUID, ThreadUUID, TurnUUID string
	RunCanResume, TurnCanResume     bool
}

// readyWorkflowAwaitTx crosses the durable Workflow terminal boundary and
// inserts the unique Chat Resume job in the same SQLite transaction.
func (service *Service) readyWorkflowAwaitTx(ctx context.Context, tx *sql.Tx, projectUUID string, workflowID int64, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT a.id,a.chat_thread_id,a.chat_turn_id,a.chat_run_id,a.uuid,th.uuid,t.uuid,
		(r.status='in_progress' AND r.cancel_requested_at IS NULL),
		(t.status='in_progress' AND t.cancel_requested_at IS NULL)
		FROM workflow_awaits a
		JOIN chat_threads th ON th.id=a.chat_thread_id
		JOIN chat_turns t ON t.id=a.chat_turn_id
		JOIN chat_runs r ON r.id=a.chat_run_id
		WHERE a.workflow_id=? AND a.status='waiting'
		ORDER BY a.id`, workflowID)
	if err != nil {
		return err
	}
	var targets []workflowAwaitTarget
	for rows.Next() {
		var target workflowAwaitTarget
		if err := rows.Scan(&target.ID, &target.ThreadID, &target.TurnID, &target.RunID, &target.AwaitUUID, &target.ThreadUUID, &target.TurnUUID, &target.RunCanResume, &target.TurnCanResume); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, target)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, target := range targets {
		if !target.RunCanResume || !target.TurnCanResume {
			if _, err := tx.ExecContext(ctx, `UPDATE workflow_awaits SET status='cancelled',cancelled_at=COALESCE(cancelled_at,?),updated_at=? WHERE id=? AND status='waiting'`, now, now, target.ID); err != nil {
				return err
			}
			if _, err := RecomputeThreadStatusTx(ctx, tx, target.ThreadID, now); err != nil {
				return err
			}
			continue
		}
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
			return domainError(CodeStateConflict, "Workflow 父 Run 状态冲突", "父 Turn 与 Run 未能原子进入恢复队列。", nil)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_awaits SET status='ready',ready_at=COALESCE(ready_at,?),updated_at=? WHERE id=? AND status='waiting'`, now, now, target.ID); err != nil {
			return err
		}
		jobID, err := service.queue.EnqueueAgentTx(ctx, projectUUID, tx, JobSpec{
			Version: 1, ProjectUUID: projectUUID, JobKind: JobChatResume,
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
		if _, err := RecomputeThreadStatusTx(ctx, tx, target.ThreadID, now); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) readyWorkflowAwaitGormTx(ctx context.Context, tx *gorm.DB, projectUUID string, workflowID int64, now time.Time) error {
	sqlTx, ok := tx.Statement.ConnPool.(*sql.Tx)
	if !ok {
		return domainError(CodeStateConflict, "Workflow 等待状态无法更新", "数据库事务连接不可用。", nil)
	}
	return service.readyWorkflowAwaitTx(ctx, sqlTx, projectUUID, workflowID, now)
}
