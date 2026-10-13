package agent

import (
	"context"
	"errors"

	"lumi/internal/llm"
	"lumi/internal/project"
	"lumi/internal/taskexecution"

	"gorm.io/gorm"
)

// WorkflowMaxAttempts bounds automatic retries per durable step execution.
// An explicit user retry starts a fresh execution at the first unfinished step.
const WorkflowMaxAttempts = 2

func retryableWorkflowRequest(err error) bool {
	var modelErr *llm.Error
	return errors.As(err, &modelErr) && modelErr.Retryable &&
		(modelErr.Code == llm.CodeTimeout || modelErr.Code == llm.CodeNetwork)
}

// Keep the await pending while River backs off and retries this same step.
// Completed steps and downstream jobs are untouched; cancellation and execution
// fencing must win over a late Provider failure.
func (service *Service) retryWorkflowStep(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord, threadUUID string, cause error, attempt int) error {
	now := service.now().UTC()
	code, message := errorCode(cause), safeMessage(cause)
	err := taskexecution.Transaction(store.DB().WithContext(ctx), func(tx *gorm.DB) error {
		result := tx.Model(&workflowRecord{}).Where("id=? AND status='running' AND cancel_requested_at IS NULL", workflow.ID).
			Updates(map[string]any{"status": WorkflowQueued, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errWorkflowTransitionInactive
		}
		result = tx.Model(&workflowStepRecord{}).Where("id=? AND status='running'", step.ID).
			Updates(map[string]any{"status": WorkflowQueued, "error_code": code, "error_message": message, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errWorkflowTransitionInactive
		}
		return appendWorkflowEventGormTx(ctx, tx, workflow.ID, &step.ID, "step_retry_scheduled", map[string]any{
			"project_uuid": store.ProjectUUID(), "workflow_uuid": workflow.UUID, "step_uuid": step.UUID,
			"step_key": step.StepKey, "status": WorkflowQueued, "attempt": attempt, "max_attempts": WorkflowMaxAttempts, "error_code": code,
		}, now)
	})
	if errors.Is(err, errWorkflowTransitionInactive) {
		return nil
	}
	if err != nil {
		return err
	}
	service.broadcastWorkflow(store.ProjectUUID(), Workflow{UUID: workflow.UUID, ThreadUUID: threadUUID, Status: WorkflowQueued}, "workflow:queued", step.UUID)
	return cause
}
