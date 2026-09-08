package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const ipFailureMessage = "Output data is suspected of being involved in IP infringement"

func TestInlineWorkflowProviderFailureReachesRESTAndResumedChat(t *testing.T) {
	fixture := newInlineYoloFixture(t)
	h := fixture.Harness
	ctx := context.Background()
	var workflow workflowRecord
	if err := h.store.DB().Where("uuid=?", fixture.Workflow.UUID).First(&workflow).Error; err != nil {
		t.Fatal(err)
	}
	var step workflowStepRecord
	if err := h.store.DB().Where("workflow_id=? AND step_key='premise'", workflow.ID).First(&step).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	taskID, taskUUID := insertWorkflowTrajectoryRow(t, h, "production_task_runs", map[string]any{
		"project_id": workflow.ProjectID, "kind": "premise_setting_generation", "resource_uuid": workflow.UUID,
		"status": "failed", "input_snapshot": "{}", "idempotency_key": "provider-error-test", "created_at": now, "updated_at": now,
	})
	_, logUUID := insertWorkflowTrajectoryRow(t, h, "llm_logs", map[string]any{
		"project_id": workflow.ProjectID, "production_task_run_id": taskID, "source_type": "production",
		"scenario": "premise_setting_generation", "request_type": "image", "provider_uuid": h.provider.UUID,
		"model": "qwen-image-3.0", "status": "failed", "error_code": "image_provider_error", "http_status": 400,
		"provider_error_code": "IPInfringementSuspect", "error_message": ipFailureMessage + " token=secret-token https://example.com/private /Users/private/data",
		"request_payload": `{"prompt":"private-prompt"}`, "response": `{"secret":"private-body"}`, "created_at": now,
	})
	if err := h.store.DB().Model(&workflowStepRecord{}).Where("id=?", step.ID).Updates(map[string]any{"status": "running", "task_uuid": taskUUID, "started_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.store.DB().Model(&workflowRecord{}).Where("id=?", workflow.ID).Updates(map[string]any{"status": WorkflowRunning, "current_step_key": step.StepKey}).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.service.failWorkflowStep(ctx, h.store, workflow, step, fixture.Thread.UUID, domainError("image_provider_error", "Premise 设置图生成失败", "", nil)); err != nil {
		t.Fatal(err)
	}
	dto, err := h.service.GetWorkflow(ctx, h.project.UUID, workflow.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if dto.ProviderError == nil || dto.ProviderError.Code != "IPInfringementSuspect" || dto.ProviderError.HTTPStatus != 400 || dto.ProviderError.ModelRequestUUID != logUUID {
		t.Fatalf("provider error=%+v", dto.ProviderError)
	}
	var found bool
	for _, s := range dto.Steps {
		if s.StepKey == "premise" {
			found = s.ProviderError != nil && s.ProviderError.Message == dto.ProviderError.Message
		}
	}
	if !found {
		t.Fatal("step lost provider diagnosis")
	}
	if err := h.execute(t, fixture.Thread.UUID, fixture.Turn.UUID, JobChatResume); err != nil {
		t.Fatal(err)
	}
	result := yoloToolResultForTurn(t, fixture)
	encoded, _ := json.Marshal(result)
	dtoJSON, _ := json.Marshal(dto)
	for _, output := range []string{string(encoded), string(dtoJSON)} {
		if !strings.Contains(output, ipFailureMessage) || !strings.Contains(output, "IPInfringementSuspect") {
			t.Fatalf("missing cause: %s", output)
		}
		for _, secret := range []string{"secret-token", "example.com", "/Users/private", "private-prompt", "private-body"} {
			if strings.Contains(output, secret) {
				t.Fatalf("diagnostic leaked %q", secret)
			}
		}
	}
	if !strings.Contains(string(encoded), "知识产权侵权") {
		t.Fatalf("missing actionable summary: %s", encoded)
	}
}

func TestWorkflowProviderFailureUsesLatestMatchingRequest(t *testing.T) {
	for _, source := range []string{"production", "workflow", "story_generation"} {
		t.Run(source, func(t *testing.T) {
			h := newAgentHarness(t)
			fixture := newWorkflowTrajectoryFixture(t, h)
			stepID := fixture.stepID
			if source == "story_generation" {
				if err := h.store.DB().Table("workflow_steps").Where("workflow_id=? AND step_key='story'", fixture.workflowID).Pluck("id", &stepID).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := h.store.DB().Table("workflow_steps").Where("id=?", stepID).Updates(map[string]any{"status": "failed", "error_code": "image_timeout"}).Error; err != nil {
				t.Fatal(err)
			}
			at := fixture.createdAt.Add(time.Second)
			logUUID := fixture.addLog(t, source, "text", "failed", 1, at, 1)
			if err := h.store.DB().Table("llm_logs").Where("uuid=?", logUUID).Updates(map[string]any{"http_status": 400, "provider_error_code": "InvalidParameter", "error_message": "unsupported size"}).Error; err != nil {
				t.Fatal(err)
			}
			read := func() map[string]*WorkflowProviderError {
				t.Helper()
				result, err := workflowProviderErrors(context.Background(), h.store, fixture.workflowID)
				if err != nil {
					t.Fatal(err)
				}
				return result
			}
			if len(read()) != 1 {
				t.Fatal("missing linked failed request")
			}
			if err := h.store.DB().Table("workflow_steps").Where("id=?", stepID).Update("started_at", at.Add(time.Second)).Error; err != nil {
				t.Fatal(err)
			}
			if len(read()) != 0 {
				t.Fatal("previous attempt failure leaked")
			}
			if err := h.store.DB().Table("workflow_steps").Where("id=?", stepID).Update("started_at", nil).Error; err != nil {
				t.Fatal(err)
			}
			fixture.addLog(t, source, "text", "completed", 2, at.Add(2*time.Second), 1)
			if len(read()) != 0 {
				t.Fatal("older failure used after successful request")
			}
		})
	}
}

func TestWorkflowProviderFailureFallback(t *testing.T) {
	state := workflowResumeState{WorkflowKind: WorkflowYolo, WorkflowStatus: WorkflowFailed, ErrorCode: "image_provider_error"}
	original := workflowTerminalToolResult(state)
	if !strings.Contains(string(original), "异步生成未完成。") {
		t.Fatalf("lost fallback: %s", original)
	}
	state.ProviderError = &WorkflowProviderError{Code: "FutureProviderCode", Message: "specific cause"}
	result := workflowTerminalToolResult(state)
	if !strings.Contains(string(result), "specific cause") || strings.Contains(string(result), "知识产权") {
		t.Fatalf("unknown rejection: %s", result)
	}
	state.WorkflowStatus = WorkflowCancelled
	if strings.Contains(string(workflowTerminalToolResult(state)), "specific cause") {
		t.Fatal("cancelled workflow exposed stale provider error")
	}
}
