package agent

import (
	"context"
	"database/sql"
	"time"

	"lumi/internal/project"
)

// ReconcileOnOpen restores only durable safe boundaries. Original items and
// events remain append-only; in-progress turns resume from persisted tool
// intent, while user-input and Workflow dependencies stay worker-free until
// their durable wake-up condition is satisfied.
func (service *Service) ReconcileOnOpen(ctx context.Context, store *project.Store) error {
	if service == nil || service.queue == nil {
		return nil
	}
	sqlDB, err := store.DB().DB()
	if err != nil {
		return err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := service.now().UTC()
	// A cancelled or otherwise terminal parent must never be revived by a later
	// Workflow terminal projection.
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_awaits
		SET status='cancelled',cancelled_at=COALESCE(cancelled_at,?),updated_at=?
		WHERE status IN ('waiting','ready','resuming') AND (EXISTS(
			SELECT 1 FROM chat_runs r WHERE r.id=workflow_awaits.chat_run_id
			AND (r.status NOT IN ('in_progress','queued') OR r.cancel_requested_at IS NOT NULL)
		) OR EXISTS(
			SELECT 1 FROM chat_turns t WHERE t.id=workflow_awaits.chat_turn_id
			AND (t.status NOT IN ('in_progress','queued') OR t.cancel_requested_at IS NOT NULL)
		))`, now, now); err != nil {
		return err
	}
	// Repair a Workflow terminal commit whose await projection or River insert
	// was interrupted. Repeated opens are safe because only waiting rows move.
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_awaits
		SET status='ready',ready_at=COALESCE(ready_at,?),updated_at=?
		WHERE status='waiting' AND EXISTS(
			SELECT 1 FROM workflows w WHERE w.id=workflow_awaits.workflow_id
			AND w.status IN ('completed','failed','cancelled','interrupted')
		) AND EXISTS(
			SELECT 1 FROM chat_runs r WHERE r.id=workflow_awaits.chat_run_id
			AND r.status IN ('in_progress','queued') AND r.cancel_requested_at IS NULL
		) AND EXISTS(
			SELECT 1 FROM chat_turns t WHERE t.id=workflow_awaits.chat_turn_id
			AND t.status IN ('in_progress','queued') AND t.cancel_requested_at IS NULL
		)`, now, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_runs SET status='queued',updated_at=?
		WHERE status='in_progress' AND cancel_requested_at IS NULL AND EXISTS(
			SELECT 1 FROM workflow_awaits a WHERE a.chat_run_id=chat_runs.id AND a.status IN ('ready','resuming')
		)`, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET status='queued',updated_at=?
		WHERE status='in_progress' AND cancel_requested_at IS NULL AND EXISTS(
			SELECT 1 FROM workflow_awaits a WHERE a.chat_turn_id=chat_turns.id AND a.status IN ('ready','resuming')
		)`, now); err != nil {
		return err
	}
	// Ordinary interrupted executions are safe to replay from persisted intent.
	// Active waiting awaits are deliberately excluded so no worker polls them.
	if _, err := tx.ExecContext(ctx, `UPDATE chat_runs SET status='queued',updated_at=?,error_code='agent_interrupted',error_message='应用重启后从持久安全边界恢复。'
		WHERE status='in_progress' AND NOT EXISTS(
			SELECT 1 FROM workflow_awaits a WHERE a.chat_run_id=chat_runs.id AND a.status='waiting'
		)`, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET status='queued',updated_at=?,error_code='agent_interrupted',error_message='应用重启后从持久安全边界恢复。'
		WHERE status='in_progress' AND NOT EXISTS(
			SELECT 1 FROM workflow_awaits a WHERE a.chat_turn_id=chat_turns.id AND a.status='waiting'
		)`, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='queued',updated_at=?,error_code='agent_interrupted',error_message='应用重启后从安全步骤恢复。' WHERE status='running' AND workflow_id IN (SELECT id FROM workflows WHERE kind=?)`, now, WorkflowYolo); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='queued',updated_at=?,error_code='agent_interrupted',error_message='应用重启后从安全步骤恢复。' WHERE status='running' AND kind=?`, now, WorkflowYolo); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT t.id,t.uuid,th.uuid,CASE
		WHEN EXISTS(SELECT 1 FROM workflow_awaits a WHERE a.chat_turn_id=t.id AND a.status IN ('ready','resuming')) THEN 'chat_resume'
		WHEN EXISTS(SELECT 1 FROM chat_user_input_requests q WHERE q.turn_id=t.id AND q.status='resuming') THEN 'chat_resume'
		ELSE 'chat_turn' END,CASE
		WHEN EXISTS(SELECT 1 FROM workflow_awaits a WHERE a.chat_turn_id=t.id AND a.status IN ('ready','resuming')) THEN COALESCE((SELECT a.uuid FROM workflow_awaits a WHERE a.chat_turn_id=t.id AND a.status IN ('ready','resuming') ORDER BY a.id LIMIT 1),'')
		WHEN EXISTS(SELECT 1 FROM chat_user_input_requests q WHERE q.turn_id=t.id AND q.status='resuming') THEN COALESCE((SELECT q.uuid FROM chat_user_input_requests q WHERE q.turn_id=t.id AND q.status='resuming' ORDER BY q.id LIMIT 1),'')
		ELSE '' END
		FROM chat_turns t JOIN chat_threads th ON th.id=t.thread_id
		WHERE t.status='queued' ORDER BY th.id,t.queue_sequence`)
	if err != nil {
		return err
	}
	type queuedTurn struct {
		ID                                     int64
		TurnUUID, ThreadUUID, Kind, WakeupUUID string
	}
	var turns []queuedTurn
	for rows.Next() {
		var row queuedTurn
		if err := rows.Scan(&row.ID, &row.TurnUUID, &row.ThreadUUID, &row.Kind, &row.WakeupUUID); err != nil {
			rows.Close()
			return err
		}
		turns = append(turns, row)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, row := range turns {
		jobID, err := service.queue.EnqueueAgentTx(ctx, store.ProjectUUID(), tx, JobSpec{Version: 1, ProjectUUID: store.ProjectUUID(), JobKind: row.Kind, ResourceUUID: row.TurnUUID, ThreadUUID: row.ThreadUUID, WakeupUUID: row.WakeupUUID})
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET river_job_id=?,updated_at=? WHERE id=?`, jobID, now, row.ID); err != nil {
			return err
		}
		if row.Kind == JobChatResume {
			if _, err := tx.ExecContext(ctx, `UPDATE workflow_awaits SET river_job_id=?,updated_at=? WHERE chat_turn_id=? AND status IN ('ready','resuming')`, jobID, now, row.ID); err != nil {
				return err
			}
		}
	}
	stepRows, err := tx.QueryContext(ctx, `SELECT s.id,s.uuid,th.uuid FROM workflow_steps s JOIN workflows w ON w.id=s.workflow_id JOIN chat_threads th ON th.id=w.thread_id WHERE s.status IN ('queued','waiting') AND w.status IN ('queued','running') AND w.kind=? ORDER BY w.id,s.position`, WorkflowYolo)
	if err != nil {
		return err
	}
	type queuedStep struct {
		ID                   int64
		StepUUID, ThreadUUID string
	}
	var steps []queuedStep
	for stepRows.Next() {
		var row queuedStep
		if err := stepRows.Scan(&row.ID, &row.StepUUID, &row.ThreadUUID); err != nil {
			stepRows.Close()
			return err
		}
		steps = append(steps, row)
	}
	if err := stepRows.Close(); err != nil {
		return err
	}
	for _, row := range steps {
		jobID, err := service.queue.EnqueueAgentTx(ctx, store.ProjectUUID(), tx, JobSpec{Version: 1, ProjectUUID: store.ProjectUUID(), JobKind: JobWorkflowStep, ResourceUUID: row.StepUUID, ThreadUUID: row.ThreadUUID})
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET river_job_id=?,updated_at=? WHERE id=?`, jobID, now, row.ID); err != nil {
			return err
		}
	}
	threadRows, err := tx.QueryContext(ctx, `SELECT id FROM chat_threads ORDER BY id`)
	if err != nil {
		return err
	}
	var threadIDs []int64
	for threadRows.Next() {
		var threadID int64
		if err := threadRows.Scan(&threadID); err != nil {
			threadRows.Close()
			return err
		}
		threadIDs = append(threadIDs, threadID)
	}
	if err := threadRows.Close(); err != nil {
		return err
	}
	for _, threadID := range threadIDs {
		if _, err := RecomputeThreadStatusTx(ctx, tx, threadID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var _ *sql.Tx
var _ = time.Time{}
