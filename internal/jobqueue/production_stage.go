package jobqueue

import "context"

// Stages are durable milestones, not estimates of provider-side completion.
func (runtime *projectRuntime) productionStage(ctx context.Context, record productionTaskRecord, stage string) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE production_task_runs SET stage=?,stage_started_at=?,updated_at=? WHERE id=? AND status='running' AND cancel_requested_at IS NULL`, stage, now, now, record.ID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return context.Canceled
	}
	if err := appendProductionEventTx(ctx, tx, record.ID, "task_stage_changed", map[string]any{
		"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "stage": stage,
	}, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	record.Status, record.Stage, record.StageStartedAt, record.UpdatedAt = StatusRunning, stage, &now, now
	runtime.broadcastProduction("production_task:stage_changed", record.DTO())
	runtime.broadcastProductionWorkflow("workflow:step_changed", record.UUID)
	return nil
}
