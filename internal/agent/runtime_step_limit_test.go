package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"lumi/internal/llm"
)

func TestCreateTurnIgnoresLegacyMaxStepsAndPersistsLongTurnBudgets(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "旧字段不再限制执行", MaxSteps: 999})
	if err != nil {
		t.Fatal(err)
	}
	var run runRecord
	if err := harness.store.DB().Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Take(&run).Error; err != nil {
		t.Fatal(err)
	}
	if run.MaxModelRequests != DefaultMaxModelRequests || run.MaxActiveDurationMS != DefaultMaxActiveDurationMS || run.MaxTokenUnits != DefaultMaxTokenUnits || run.MaxNoProgressRounds != DefaultMaxNoProgressRounds {
		t.Fatalf("unexpected long-turn budgets: %+v", run)
	}
}

func TestSingleTurnCanExceedLegacySixtyFourRequests(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "读取文档后继续", MaxSteps: 1})
	if err != nil {
		t.Fatal(err)
	}
	harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
		if call > 65 {
			response := finalResponse("已在同一 Turn 完成长期运行。")
			response.Usage = llm.Usage{InputTokens: 1, OutputTokens: 1}
			return response, nil
		}
		path := "/api/v1/agent-docs/overview.md"
		if call%2 == 0 {
			path = "/api/v1/agent-docs/guides/创建章节.md"
		}
		arguments, _ := json.Marshal(map[string]any{"path": path})
		return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: fmt.Sprintf("read-guide-%d", call), Name: "read_agent_doc", Arguments: string(arguments)}}}, FinishReason: "tool_calls", Usage: llm.Usage{InputTokens: 1, OutputTokens: 1}}, nil
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	if harness.model.calls != 66 {
		t.Fatalf("model calls=%d, want 66", harness.model.calls)
	}
	var item itemRecord
	if err := harness.store.DB().Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='assistant_message'", turn.UUID).Order("sequence DESC").Take(&item).Error; err != nil {
		t.Fatal(err)
	}
	if item.Content != "已在同一 Turn 完成长期运行。" || strings.Contains(item.Content, "回复「继续」") {
		t.Fatalf("unexpected completion content: %q", item.Content)
	}
	if !strings.Contains(item.MetadataJSON, `"request_uuid"`) {
		t.Fatalf("missing model request linkage: %s", item.MetadataJSON)
	}
	var runs, finalizationLogs int64
	if err := harness.store.DB().Table("chat_runs").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Count(&runs).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("llm_logs").Where("chat_run_id=? AND scenario='project_chat_finalization'", item.RunID).Count(&finalizationLogs).Error; err != nil {
		t.Fatal(err)
	}
	if runs != 1 || finalizationLogs != 0 {
		t.Fatalf("runs=%d finalization logs=%d", runs, finalizationLogs)
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
