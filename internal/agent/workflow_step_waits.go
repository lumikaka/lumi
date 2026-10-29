package agent

import (
	"context"

	"lumi/internal/project"
)

// WorkflowStepWaiter is implemented by the durable queue adapter. Small in-memory
// queues used by isolated runners can instead drive steps explicitly.
type WorkflowStepWaiter interface {
	SuspendWorkflowStep(context.Context, string, string, string, []DomainTask) error
}

func (service *Service) waitForWorkflowTasks(ctx context.Context, store *project.Store, step workflowStepRecord, phase string, tasks []DomainTask) (map[string]any, bool, error) {
	if waiter, ok := service.queue.(WorkflowStepWaiter); ok {
		if err := waiter.SuspendWorkflowStep(ctx, store.ProjectUUID(), step.UUID, phase, tasks); err != nil {
			return nil, false, err
		}
		// Registration may have already installed a new execution when a task
		// completed concurrently. Do not write through the old execution again.
		return nil, false, ErrWaitingDependencies
	}
	return nil, true, nil
}
