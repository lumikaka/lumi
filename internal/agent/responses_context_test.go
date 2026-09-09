package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"lumi/internal/llm"
	"lumi/internal/provider"
)

func TestResponsesContinuationRestoresPersistedOutputForToolsAndLaterTurns(t *testing.T) {
	output := []json.RawMessage{
		json.RawMessage(`{"type":"reasoning","id":"rs-1","summary":[],"encrypted_content":"opaque-state"}`),
		json.RawMessage(`{"type":"message","id":"msg-1","role":"assistant","phase":"commentary","content":[{"type":"output_text","text":"Reading guide"}]}`),
		json.RawMessage(`{"type":"function_call","id":"fc-1","call_id":"read-guide-1","name":"read_agent_doc","arguments":"{\"path\":\"/api/v1/agent-docs/overview.md\"}"}`),
	}
	finalOutput := []json.RawMessage{json.RawMessage(`{"type":"message","id":"msg-2","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":"Done"}]}`)}
	harness := newAgentHarness(t,
		llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ResponsesOutput: output, ToolCalls: []llm.ToolCall{{ID: "read-guide-1", Name: "read_agent_doc", Arguments: `{"path":"/api/v1/agent-docs/overview.md"}`}}}, FinishReason: "tool_calls"},
		llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "Done", ResponsesOutput: finalOutput}, FinishReason: "stop"},
		finalResponse("Again"),
	)
	ctx := context.Background()
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "Read the guide"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	followup, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "Continue"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, followup.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	requests := harness.model.requests
	if len(requests) != 3 {
		t.Fatalf("requests=%d", len(requests))
	}
	var restoredTool, restoredFinal bool
	for _, message := range requests[1].Messages {
		if len(message.ToolCalls) > 0 && message.ToolCalls[0].ID == "read-guide-1" {
			encoded, _ := json.Marshal(message.ResponsesOutput)
			want, _ := json.Marshal(output)
			var gotValue, wantValue any
			_ = json.Unmarshal(encoded, &gotValue)
			_ = json.Unmarshal(want, &wantValue)
			if !reflect.DeepEqual(gotValue, wantValue) {
				t.Fatalf("persisted output lost: %s", encoded)
			}
			restoredTool = true
		}
	}
	for _, message := range requests[2].Messages {
		if message.Role == "assistant" && message.Content == "Done" && len(message.ResponsesOutput) == 1 {
			restoredFinal = true
		}
	}
	if !restoredTool || !restoredFinal {
		t.Fatalf("restored tool=%v final=%v", restoredTool, restoredFinal)
	}
	for _, request := range requests {
		if request.ProviderType != provider.TypeCloudflareAIGateway {
			t.Fatalf("provider type=%s", request.ProviderType)
		}
	}
}

func TestResponsesContinuationDoesNotReplayRejectedCalls(t *testing.T) {
	output := []json.RawMessage{json.RawMessage(`{"type":"function_call","call_id":"one","name":"lookup","arguments":"{}"}`), json.RawMessage(`{"type":"function_call","call_id":"two","name":"lookup","arguments":"{}"}`)}
	if got := matchingResponsesOutput(output, []llm.ToolCall{{ID: "one", Name: "lookup"}}); len(got) > 0 {
		t.Fatal("replayed orphan tool call")
	}
}
