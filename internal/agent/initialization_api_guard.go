package agent

import (
	"context"
	"errors"

	"lumi/internal/project"
)

// Initialization owns production until the user explicitly takes over. Reads
// and the runtime's idempotent bootstrap recovery remain available.
func requireAgentInitializationWriteAvailable(ctx context.Context, store *project.Store, tc toolContext, request agentAPIRequest) error {
	if store == nil || request.Method == "GET" || request.Route.ReadOnly {
		return nil
	}
	if request.Route.ID == RouteYoloWorkflowCreate {
		// CreateYoloWorkflow checks its persisted idempotency key before the
		// initialization gate, so only the existing workflow can be replayed.
		return nil
	}
	if request.Route.ID == RouteProjectSetupFinalize && isBootstrapToolContext(tc) {
		// The existing finalization authorization and revision checks still
		// apply when recovering a bootstrap after it has already started YOLO.
		return nil
	}
	err := store.RequireInitializationAvailable(ctx)
	var projectErr *project.Error
	if errors.As(err, &projectErr) {
		return domainError(projectErr.Code, projectErr.Message, projectErr.Details, err)
	}
	return err
}
