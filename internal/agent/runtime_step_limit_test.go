package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"lumi/internal/llm"
)

func TestCreateTurnUsesTwentyStepDefault(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "使用默认步骤上限"})
	if err != nil {
		t.Fatal(err)
	}
	var run runRecord
	if err := harness.store.DB().Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Take(&run).Error; err != nil {
		t.Fatal(err)
	}
	if DefaultMaxSteps != 20 {
		t.Fatalf("default max steps constant=%d", DefaultMaxSteps)
	}
	if run.MaxSteps != DefaultMaxSteps {
		t.Fatalf("default max steps=%d constant=%d", run.MaxSteps, DefaultMaxSteps)
	}
}

func TestStepLimitUsesDeterministicHandoffWithoutFinalizationRequest(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "读取文档后继续", MaxSteps: 1})
	if err != nil {
		t.Fatal(err)
	}
	harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
		if call != 1 {
			return finalResponse(`<invoke name="request_api"><parameter name="method">POST</parameter></invoke>`), nil
		}
		arguments, _ := json.Marshal(map[string]any{"path": "/api/v1/agent-docs/guides/创建章节.md"})
		return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "read-guide", Name: "read_agent_doc", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	if harness.model.calls != 1 {
		t.Fatalf("model calls=%d, want 1", harness.model.calls)
	}
	var item itemRecord
	if err := harness.store.DB().Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='assistant_message'", turn.UUID).Order("sequence DESC").Take(&item).Error; err != nil {
		t.Fatal(err)
	}
	if item.Content != stepLimitHandoffMessage || strings.Contains(item.Content, "<invoke") {
		t.Fatalf("unexpected handoff content: %q", item.Content)
	}
	if !strings.Contains(item.MetadataJSON, `"runtime_generated":true`) || !strings.Contains(item.MetadataJSON, `"completion_reason":"step_limit"`) || strings.Contains(item.MetadataJSON, `"request_uuid"`) {
		t.Fatalf("unexpected handoff metadata: %s", item.MetadataJSON)
	}
	var finalizationLogs int64
	if err := harness.store.DB().Table("llm_logs").Where("chat_run_id=? AND scenario='project_chat_finalization'", item.RunID).Count(&finalizationLogs).Error; err != nil {
		t.Fatal(err)
	}
	if finalizationLogs != 0 {
		t.Fatalf("finalization logs=%d", finalizationLogs)
	}
}

func TestUnexecutedToolMarkupIsReplacedBySafeHandoff(t *testing.T) {
	harness := newAgentHarness(t, finalResponse("准备继续。\n<invoke name=\"request_api\">\n<parameter name=\"method\">POST</parameter>\n</invoke>"))
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "生成内容", MaxSteps: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	var item itemRecord
	if err := harness.store.DB().Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='assistant_message'", turn.UUID).Order("sequence DESC").Take(&item).Error; err != nil {
		t.Fatal(err)
	}
	if item.Content != invalidToolMarkupHandoffMessage || strings.Contains(item.Content, "<invoke") {
		t.Fatalf("unexpected safety handoff: %q", item.Content)
	}
	if !strings.Contains(item.MetadataJSON, `"completion_reason":"unexecuted_tool_markup"`) || strings.Contains(item.MetadataJSON, `"request_uuid"`) {
		t.Fatalf("unexpected safety metadata: %s", item.MetadataJSON)
	}
}
