package agent

import (
	"context"
	"database/sql"
	"time"

	"lumi/internal/project"
)

// ReconcileOnOpen restores only durable safe boundaries. Original items and
// events remain append-only; in-progress turns resume from persisted tool
// intent, while waiting user-input requests stay waiting.
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
	if _, err := tx.ExecContext(ctx, `UPDATE chat_runs SET status='queued',updated_at=?,error_code='agent_interrupted',error_message='应用重启后从持久安全边界恢复。' WHERE status='in_progress'`, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET status='queued',updated_at=?,error_code='agent_interrupted',error_message='应用重启后从持久安全边界恢复。' WHERE status='in_progress'`, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='queued',updated_at=?,error_code='agent_interrupted',error_message='应用重启后从安全步骤恢复。' WHERE status='running'`, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='queued',updated_at=?,error_code='agent_interrupted',error_message='应用重启后从安全步骤恢复。' WHERE status='running'`, now); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT t.id,t.uuid,th.uuid,CASE WHEN EXISTS(SELECT 1 FROM chat_user_input_requests q WHERE q.turn_id=t.id AND q.status='resuming') THEN 'chat_resume' ELSE 'chat_turn' END FROM chat_turns t JOIN chat_threads th ON th.id=t.thread_id WHERE t.status='queued' ORDER BY th.id,t.queue_sequence`)
	if err != nil {
		return err
	}
	type queuedTurn struct {
		ID                         int64
		TurnUUID, ThreadUUID, Kind string
	}
	var turns []queuedTurn
	for rows.Next() {
		var row queuedTurn
		if err := rows.Scan(&row.ID, &row.TurnUUID, &row.ThreadUUID, &row.Kind); err != nil {
			rows.Close()
			return err
		}
		turns = append(turns, row)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, row := range turns {
		jobID, err := service.queue.EnqueueAgentTx(ctx, store.ProjectUUID(), tx, JobSpec{Version: 1, ProjectUUID: store.ProjectUUID(), JobKind: row.Kind, ResourceUUID: row.TurnUUID, ThreadUUID: row.ThreadUUID})
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET river_job_id=?,updated_at=? WHERE id=?`, jobID, now, row.ID); err != nil {
			return err
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
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status=CASE WHEN EXISTS(SELECT 1 FROM chat_turns t WHERE t.thread_id=chat_threads.id AND t.status='waiting_for_input') THEN 'waiting_for_input' WHEN EXISTS(SELECT 1 FROM chat_turns t WHERE t.thread_id=chat_threads.id AND t.status IN ('queued','in_progress')) OR EXISTS(SELECT 1 FROM workflows w WHERE w.thread_id=chat_threads.id AND w.status IN ('queued','running')) THEN 'busy' ELSE CASE WHEN status IN ('completed','failed','cancelled','interrupted') THEN status ELSE 'idle' END END,updated_at=?`, now); err != nil {
		return err
	}
	return tx.Commit()
}

var _ *sql.Tx
var _ = time.Time{}
