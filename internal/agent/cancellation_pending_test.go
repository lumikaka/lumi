package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAbortPersistsImageCancellationAndReportsDispatchFailure(t *testing.T) {
	f := newInlineYoloFixture(t)
	h := f.Harness
	var workflow workflowRecord
	if err := h.store.DB().Where("uuid=?", f.Workflow.UUID).First(&workflow).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, taskUUID := insertWorkflowTrajectoryRow(t, h, "production_task_runs", map[string]any{
		"project_id": workflow.ProjectID, "kind": "comic_image_generation", "resource_uuid": workflow.UUID,
		"status": "running", "input_snapshot": "{}", "idempotency_key": "abort-pending-test", "created_at": now, "updated_at": now,
	})
	if err := h.store.DB().Exec(`UPDATE workflow_steps SET task_uuid=?,status='running' WHERE workflow_id=? AND step_key='premise'`, taskUUID, workflow.ID).Error; err != nil {
		t.Fatal(err)
	}
	h.queue.domainCancelErr = errors.New("temporary dispatch failure")
	turn, err := h.service.Abort(context.Background(), h.project.UUID, f.Thread.UUID)
	var agentErr *Error
	if !errors.As(err, &agentErr) || agentErr.Code != "agent_cancellation_pending" || turn.Status != TurnCancelled {
		t.Fatalf("turn=%+v err=%v", turn, err)
	}
	var requested bool
	if err := h.store.DB().Raw(`SELECT cancel_requested_at IS NOT NULL FROM production_task_runs WHERE uuid=?`, taskUUID).Scan(&requested).Error; err != nil || !requested {
		t.Fatalf("intent not persisted: %v %v", requested, err)
	}
	var status string
	if err := h.store.DB().Raw(`SELECT status FROM workflow_awaits WHERE workflow_id=?`, workflow.ID).Scan(&status).Error; err != nil || status != "cancelled" {
		t.Fatalf("await=%q err=%v", status, err)
	}
	if len(h.queue.domainCancels) != 1 || h.queue.domainCancels[0] != taskUUID {
		t.Fatalf("cancelled tasks=%v", h.queue.domainCancels)
	}
}
