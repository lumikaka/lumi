package agent

import (
	"context"
	"strings"
	"testing"

	"lumi/internal/llm"
)

func TestUnknownProjectAPIRouteRepairPreservesProjectAndToolBoundaries(t *testing.T) {
	projectUUID := mustAgentUUID(t)
	base := "/api/v1/projects/" + projectUUID
	otherBase := "/api/v1/projects/" + mustAgentUUID(t)
	for _, test := range []struct {
		name, path, mode, code string
	}{
		{"current project", base + "/story", ToolModeProjectAPI, CodeToolValidation},
		{"other project unknown route", otherBase + "/story", ToolModeProjectAPI, CodeToolNotAllowed},
		{"other project registered route", otherBase + "/story-profile", ToolModeProjectAPI, CodeToolNotAllowed},
		{"project prefix collision", base + "extra/story", ToolModeProjectAPI, CodeToolNotAllowed},
		{"wrong tool mode", base + "/story", ToolModeLegacyTyped, CodeToolNotAllowed},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseAgentAPIRequest(toolContext{ProjectUUID: projectUUID, ToolMode: test.mode}, map[string]any{
				"method": "GET", "url": test.path, "response_filter": ".data | {uuid,revision,story_md}",
			})
			if errorCode(err) != test.code {
				t.Fatalf("error=%v want=%s", err, test.code)
			}
			if test.code == CodeToolValidation {
				violation, ok := toolValidationViolationFromError(err)
				if !ok || violation.Path != "url" || violation.Rule != "route_not_found" || !strings.Contains(string(toolErrorResult(err)), agentDocOverviewPath) {
					t.Fatalf("missing route repair guidance: violation=%+v result=%s", violation, toolErrorResult(err))
				}
			}
		})
	}
}

func TestUnknownProjectAPIRouteCanReadContractAndCorrectRequest(t *testing.T) {
	harness := newAgentHarness(t)
	base := "/api/v1/projects/" + harness.project.UUID
	harness.model.responses = []llm.ChatResponse{
		requestAPICallResponse(t, "unknown-story-route", map[string]any{
			"method": "GET", "url": base + "/story", "response_filter": ".data | {uuid,revision,story_md}",
		}),
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{
			ID: "read-story-contract", Name: "read_agent_doc", Arguments: requestAPITestArguments(t, map[string]any{"path": storyDocPath}),
		}}}, FinishReason: "tool_calls"},
		requestAPICallResponse(t, "corrected-story-route", map[string]any{
			"method": "GET", "url": base + "/story-profile", "response_filter": ".data | {uuid,revision,story_md,projection_state}",
		}),
		finalResponse("已读取故事总纲。"),
	}
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "读取故事总纲"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	turns, err := harness.service.ListTurns(context.Background(), harness.project.UUID, thread.UUID)
	if err != nil || len(turns) != 1 || turns[0].Status != TurnCompleted {
		t.Fatalf("turns=%+v err=%v", turns, err)
	}
	var executions int64
	if err := harness.store.DB().Table("agent_tool_executions").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND tool_name='request_api'", turn.UUID).Count(&executions).Error; err != nil {
		t.Fatal(err)
	}
	if executions != 1 {
		t.Fatalf("API executions=%d want=1", executions)
	}
	harness.model.mu.Lock()
	requests := append([]llm.ChatRequest(nil), harness.model.requests...)
	harness.model.mu.Unlock()
	if len(requests) != 4 {
		t.Fatalf("model requests=%d", len(requests))
	}
	found := false
	for _, message := range requests[1].Messages {
		if message.Role == "tool" && message.ToolCallID == "unknown-story-route" && strings.Contains(message.Content, CodeToolValidation) && strings.Contains(message.Content, agentDocOverviewPath) {
			found = true
		}
	}
	if !found {
		t.Fatal("model did not receive paired route repair guidance")
	}
	found = false
	for _, message := range requests[3].Messages {
		if message.Role == "tool" && message.ToolCallID == "corrected-story-route" && strings.Contains(message.Content, `"story_md"`) && strings.Contains(message.Content, `"success":true`) {
			found = true
		}
	}
	if !found {
		t.Fatal("corrected request did not return the story content")
	}
}

func TestUnknownProjectAPIRouteRepairIsBoundedAndCrossProjectIsFatal(t *testing.T) {
	for _, crossProject := range []bool{false, true} {
		name := "repair limit"
		if crossProject {
			name = "cross project"
		}
		t.Run(name, func(t *testing.T) {
			harness := newAgentHarness(t)
			harness.service.turnBudget.MaxNoProgressRounds = 10
			projectUUID := harness.project.UUID
			wantCalls, wantRepairs, wantCode := maxToolValidationRepairs+1, int64(maxToolValidationRepairs), CodeToolValidation
			if crossProject {
				projectUUID = mustAgentUUID(t)
				wantCalls, wantRepairs, wantCode = 1, 0, CodeToolNotAllowed
			}
			for _, callID := range []string{"unknown-1", "unknown-2", "unknown-3"} {
				harness.model.responses = append(harness.model.responses, requestAPICallResponse(t, callID, map[string]any{
					"method": "GET", "url": "/api/v1/projects/" + projectUUID + "/story", "response_filter": ".data | {uuid}",
				}))
			}
			thread := harness.createThread(t)
			turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "读取故事总纲"})
			if err != nil {
				t.Fatal(err)
			}
			if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
				t.Fatal(err)
			}
			turns, err := harness.service.ListTurns(context.Background(), harness.project.UUID, thread.UUID)
			if err != nil || len(turns) != 1 || turns[0].Status != TurnFailed || turns[0].ErrorCode != wantCode {
				t.Fatalf("turns=%+v err=%v", turns, err)
			}
			var repairs, executions int64
			if err := harness.store.DB().Table("chat_items").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='tool_result' AND json_extract(metadata_json,'$.validation_repair')=1", turn.UUID).Count(&repairs).Error; err != nil {
				t.Fatal(err)
			}
			if err := harness.store.DB().Table("agent_tool_executions").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Count(&executions).Error; err != nil {
				t.Fatal(err)
			}
			harness.model.mu.Lock()
			calls := harness.model.calls
			harness.model.mu.Unlock()
			if calls != wantCalls || repairs != wantRepairs || executions != 0 {
				t.Fatalf("calls=%d repairs=%d executions=%d", calls, repairs, executions)
			}
		})
	}
}
