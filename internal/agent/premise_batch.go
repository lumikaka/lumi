package agent

import (
	"context"

	"lumi/internal/project"

	"gorm.io/gorm"
)

func premiseBatchTaskKind(stepKey string) string {
	if stepKey == WorkflowStepGenerateSetting {
		return "premise_setting_generation"
	}
	return "premise_asset_breakdown"
}

func (service *Service) cancelPremiseBatch(ctx context.Context, projectUUID, workflowUUID string) (Workflow, error) {
	// Fence the stage transition first. Read task links again afterwards, so a
	// concurrently completed setting cannot leave a newly queued breakdown alive.
	err := service.withStore(ctx, projectUUID, func(store *project.Store) error {
		return store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			now := service.now().UTC()
			if err := tx.Exec(`UPDATE workflows SET cancel_requested_at=COALESCE(cancel_requested_at,?),updated_at=? WHERE uuid=? AND kind=? AND status<>'completed'`, now, now, workflowUUID, WorkflowPremiseBatch).Error; err != nil {
				return err
			}
			return tx.Exec(`UPDATE production_task_runs SET cancel_requested_at=COALESCE(cancel_requested_at,?),updated_at=? WHERE uuid IN (SELECT s.task_uuid FROM workflow_steps s JOIN workflows w ON w.id=s.workflow_id WHERE w.uuid=? AND w.cancel_requested_at IS NOT NULL) AND status NOT IN ('completed','cancelled')`, now, now, workflowUUID).Error
		})
	})
	if err != nil {
		return Workflow{}, err
	}
	workflow, err := service.GetWorkflow(ctx, projectUUID, workflowUUID)
	if err != nil {
		return Workflow{}, err
	}
	for _, step := range workflow.Steps {
		if step.TaskUUID == "" || step.Status == WorkflowCompleted {
			continue
		}
		if err := service.queue.CancelDomainTask(context.WithoutCancel(ctx), projectUUID, premiseBatchTaskKind(step.StepKey), step.TaskUUID); err != nil {
			return Workflow{}, err
		}
	}
	result, err := service.GetWorkflow(ctx, projectUUID, workflowUUID)
	if err == nil {
		service.broadcastWorkflow(projectUUID, result, "workflow:"+result.Status, "")
	}
	return result, err
}
