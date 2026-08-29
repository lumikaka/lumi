package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"lumi/internal/llm"
)

func TestBudgetReasonUsesStablePrecedence(t *testing.T) {
	run := runRecord{
		ModelRequestCount: 9, MaxModelRequests: 10,
		TokenUnits: 10, MaxTokenUnits: 10,
		ActiveDurationMS: 10, MaxActiveDurationMS: 10,
		NoProgressStreak: 2, MaxNoProgressRounds: 2,
	}
	if reason := budgetReason(run); reason != BudgetReasonNoProgress {
		t.Fatalf("reason=%q", reason)
	}
	run.NoProgressStreak = 0
	if reason := budgetReason(run); reason != BudgetReasonModelRequests {
		t.Fatalf("reason=%q", reason)
	}
	run.ModelRequestCount = 0
	if reason := budgetReason(run); reason != BudgetReasonTokens {
		t.Fatalf("reason=%q", reason)
	}
	run.TokenUnits = 0
	if reason := budgetReason(run); reason != BudgetReasonActiveDuration {
		t.Fatalf("reason=%q", reason)
	}
}

func TestLongTurnBudgetsUseExactlyOneToolFreeFinalizationRequest(t *testing.T) {
	tests := []struct {
		name                string
		reason              string
		limits              turnBudgetLimits
		expectedModelCalls  int
		normalUsage         llm.Usage
		forceActiveDuration bool
		repeatArguments     bool
	}{
		{name: "no progress", reason: BudgetReasonNoProgress, limits: turnBudgetLimits{MaxModelRequests: 20, MaxActiveDurationMS: 1_000_000, MaxTokenUnits: 1_000_000, MaxNoProgressRounds: 2}, expectedModelCalls: 3, normalUsage: llm.Usage{InputTokens: 1, OutputTokens: 1}, repeatArguments: true},
		{name: "model requests", reason: BudgetReasonModelRequests, limits: turnBudgetLimits{MaxModelRequests: 3, MaxActiveDurationMS: 1_000_000, MaxTokenUnits: 1_000_000, MaxNoProgressRounds: 20}, expectedModelCalls: 3, normalUsage: llm.Usage{InputTokens: 1, OutputTokens: 1}},
		{name: "tokens", reason: BudgetReasonTokens, limits: turnBudgetLimits{MaxModelRequests: 20, MaxActiveDurationMS: 1_000_000, MaxTokenUnits: 3, MaxNoProgressRounds: 20}, expectedModelCalls: 2, normalUsage: llm.Usage{InputTokens: 2, OutputTokens: 2}},
		{name: "active duration", reason: BudgetReasonActiveDuration, limits: turnBudgetLimits{MaxModelRequests: 20, MaxActiveDurationMS: 1, MaxTokenUnits: 1_000_000, MaxNoProgressRounds: 20}, expectedModelCalls: 2, normalUsage: llm.Usage{InputTokens: 1, OutputTokens: 1}, forceActiveDuration: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newAgentHarness(t)
			harness.service.turnBudget = test.limits
			thread := harness.createThread(t)
			turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "持续执行直到内部预算收尾"})
			if err != nil {
				t.Fatal(err)
			}
			harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
				if len(request.Tools) == 0 {
					response := finalResponse("已完成可验证部分；因内部预算停止，剩余部分未完成。")
					response.Usage = llm.Usage{InputTokens: 1, OutputTokens: 1}
					return response, nil
				}
				if test.forceActiveDuration {
					if err := harness.store.DB().Table("chat_runs").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Update("active_duration_ms", test.limits.MaxActiveDurationMS).Error; err != nil {
						t.Fatal(err)
					}
				}
				path := "/api/v1/agent-docs/overview.md"
				if !test.repeatArguments && call%2 == 0 {
					path = "/api/v1/agent-docs/guides/创建章节.md"
				}
				arguments, _ := json.Marshal(map[string]any{"path": path})
				return llm.ChatResponse{
					Message:      llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: fmt.Sprintf("budget-call-%d", call), Name: "read_agent_doc", Arguments: string(arguments)}}},
					FinishReason: "tool_calls", Usage: test.normalUsage,
				}, nil
			}
			if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
				t.Fatal(err)
			}
			if harness.model.calls != test.expectedModelCalls {
				t.Fatalf("model calls=%d want=%d", harness.model.calls, test.expectedModelCalls)
			}
			var run runRecord
			if err := harness.store.DB().Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Take(&run).Error; err != nil {
				t.Fatal(err)
			}
			if run.Status != TurnCompleted || run.LimitReason != test.reason || run.FinalizationAttemptedAt == nil || run.ModelRequestCount != test.expectedModelCalls {
				t.Fatalf("unexpected completed run: %+v", run)
			}
			var finalizationLogs, budgetEvents int64
			if err := harness.store.DB().Table("llm_logs").Where("chat_run_id=? AND scenario='project_chat_finalization'", run.ID).Count(&finalizationLogs).Error; err != nil {
				t.Fatal(err)
			}
			if err := harness.store.DB().Table("chat_events").Where("run_id=? AND event_type='run_budget_exhausted'", run.ID).Count(&budgetEvents).Error; err != nil {
				t.Fatal(err)
			}
			if finalizationLogs != 1 || budgetEvents != 1 {
				t.Fatalf("finalization logs=%d budget events=%d", finalizationLogs, budgetEvents)
			}
			harness.model.mu.Lock()
			finalRequest := harness.model.requests[len(harness.model.requests)-1]
			harness.model.mu.Unlock()
			if len(finalRequest.Tools) != 0 || len(finalRequest.Messages) == 0 || !strings.Contains(finalRequest.Messages[0].Content, "Never claim that unverified work is complete") {
				t.Fatalf("invalid finalization request: %+v", finalRequest)
			}
			var finalItem itemRecord
			if err := harness.store.DB().Where("run_id=? AND item_type='assistant_message'", run.ID).Order("sequence DESC").Take(&finalItem).Error; err != nil {
				t.Fatal(err)
			}
			if strings.Contains(finalItem.Content, "回复「继续」") || !strings.Contains(finalItem.MetadataJSON, `"completion_reason":"budget_limit"`) || !strings.Contains(finalItem.MetadataJSON, `"budget_reason":"`+test.reason+`"`) || !strings.Contains(finalItem.MetadataJSON, `"request_uuid"`) {
				t.Fatalf("invalid final item content=%q metadata=%s", finalItem.Content, finalItem.MetadataJSON)
			}
		})
	}
}

func TestValidFinalAnswerWinsWhenRequestCrossesBudget(t *testing.T) {
	harness := newAgentHarness(t)
	harness.service.turnBudget = turnBudgetLimits{MaxModelRequests: 2, MaxActiveDurationMS: 1, MaxTokenUnits: 1, MaxNoProgressRounds: 1}
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "直接完成"})
	if err != nil {
		t.Fatal(err)
	}
	harness.model.respond = func(_ int, _ llm.ChatRequest) (llm.ChatResponse, error) {
		response := finalResponse("有效最终答案")
		response.Usage = llm.Usage{InputTokens: 100, OutputTokens: 100}
		return response, nil
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	var run runRecord
	if err := harness.store.DB().Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Take(&run).Error; err != nil {
		t.Fatal(err)
	}
	var finalizationLogs int64
	_ = harness.store.DB().Table("llm_logs").Where("chat_run_id=? AND scenario='project_chat_finalization'", run.ID).Count(&finalizationLogs).Error
	if run.Status != TurnCompleted || run.LimitReason != "" || run.ModelRequestCount != 1 || run.TokenUnits != 200 || finalizationLogs != 0 || harness.model.calls != 1 {
		t.Fatalf("run=%+v finalization logs=%d calls=%d", run, finalizationLogs, harness.model.calls)
	}
}

func TestMissingProviderUsageFallsBackToRequestAndResponseBytes(t *testing.T) {
	harness := newAgentHarness(t, finalResponse("没有 usage 统计的最终答案"))
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "估算 token"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	var run runRecord
	if err := harness.store.DB().Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Take(&run).Error; err != nil {
		t.Fatal(err)
	}
	var requestBytes, responseBytes int
	if err := harness.store.DB().Raw(`SELECT length(CAST(request_payload AS BLOB)),length(CAST(response AS BLOB)) FROM llm_logs WHERE chat_run_id=? AND request_type='text' ORDER BY id LIMIT 1`, run.ID).Row().Scan(&requestBytes, &responseBytes); err != nil {
		t.Fatal(err)
	}
	if run.ContextBytes > requestBytes {
		requestBytes = run.ContextBytes
	}
	expected := int64((requestBytes + responseBytes + 3) / 4)
	if run.TokenUnits != expected || expected <= 0 {
		t.Fatalf("token units=%d expected=%d request=%d response=%d", run.TokenUnits, expected, requestBytes, responseBytes)
	}
}

func TestBudgetFinalizationFailureMarksOriginalTurnFailed(t *testing.T) {
	tests := []struct {
		name     string
		response llm.ChatResponse
		err      error
	}{
		{name: "tool call", response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "forbidden-final-tool", Name: "read_agent_doc", Arguments: `{"path":"/api/v1/agent-docs/overview.md"}`}}}, FinishReason: "tool_calls"}},
		{name: "empty content", response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant"}, FinishReason: "stop"}},
		{name: "provider error", err: &llm.Error{Code: llm.CodeNetwork, SafeMessage: "网络失败", Retryable: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newAgentHarness(t)
			harness.service.turnBudget = turnBudgetLimits{MaxModelRequests: 2, MaxActiveDurationMS: 1_000_000, MaxTokenUnits: 1_000_000, MaxNoProgressRounds: 20}
			thread := harness.createThread(t)
			turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "触发失败收尾"})
			if err != nil {
				t.Fatal(err)
			}
			harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
				if len(request.Tools) == 0 {
					return test.response, test.err
				}
				return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: fmt.Sprintf("normal-%d", call), Name: "read_agent_doc", Arguments: `{"path":"/api/v1/agent-docs/overview.md"}`}}}, FinishReason: "tool_calls", Usage: llm.Usage{InputTokens: 1, OutputTokens: 1}}, nil
			}
			if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
				t.Fatal(err)
			}
			var run runRecord
			if err := harness.store.DB().Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Take(&run).Error; err != nil {
				t.Fatal(err)
			}
			if run.Status != TurnFailed || run.ErrorCode != CodeTurnBudget || run.FinalizationAttemptedAt == nil || harness.model.calls != 2 {
				t.Fatalf("unexpected failed run=%+v calls=%d", run, harness.model.calls)
			}
		})
	}
}

func TestInterruptedBudgetFinalizationIsNotRequestedAgain(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "恢复已中断收尾"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(context.Background(), harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.service.claimRun(context.Background(), harness.store, &tc); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := harness.store.DB().Model(&runRecord{}).Where("id=?", tc.Run.ID).Updates(map[string]any{"limit_reason": BudgetReasonTokens, "finalization_attempted_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.service.ReconcileOnOpen(context.Background(), harness.store); err != nil {
		t.Fatal(err)
	}
	var recovered runRecord
	if err := harness.store.DB().Where("id=?", tc.Run.ID).Take(&recovered).Error; err != nil || recovered.Status != TurnQueued || recovered.FinalizationAttemptedAt == nil {
		t.Fatalf("recovered run=%+v err=%v", recovered, err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	var run runRecord
	if err := harness.store.DB().Where("id=?", tc.Run.ID).Take(&run).Error; err != nil {
		t.Fatal(err)
	}
	if harness.model.calls != 0 || run.Status != TurnFailed || run.ErrorCode != CodeTurnBudget {
		t.Fatalf("calls=%d run=%+v", harness.model.calls, run)
	}
}

func TestToolCycleFingerprintChangesAndExplicitResumeResetNoProgress(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "检查无进展重置"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(context.Background(), harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.service.claimRun(context.Background(), harness.store, &tc); err != nil {
		t.Fatal(err)
	}
	result := json.RawMessage(`{"success":true,"data":{"status":"completed"}}`)
	first := []toolCycleEntry{cycleEntry("read_agent_doc", `{"path":"a"}`, thread.UUID, result)}
	if err := harness.service.recordToolCycleProgress(context.Background(), harness.store, tc, first); err != nil {
		t.Fatal(err)
	}
	if err := harness.service.recordToolCycleProgress(context.Background(), harness.store, tc, first); err != nil {
		t.Fatal(err)
	}
	var run runRecord
	_ = harness.store.DB().Where("id=?", tc.Run.ID).Take(&run).Error
	if run.NoProgressStreak != 2 {
		t.Fatalf("same cycle streak=%d", run.NoProgressStreak)
	}
	changed := []toolCycleEntry{cycleEntry("read_agent_doc", `{"path":"b"}`, thread.UUID, result)}
	if err := harness.service.recordToolCycleProgress(context.Background(), harness.store, tc, changed); err != nil {
		t.Fatal(err)
	}
	_ = harness.store.DB().Where("id=?", tc.Run.ID).Take(&run).Error
	if run.NoProgressStreak != 1 {
		t.Fatalf("changed cycle streak=%d", run.NoProgressStreak)
	}
	if err := harness.service.resetNoProgress(context.Background(), harness.store, tc.Run.ID); err != nil {
		t.Fatal(err)
	}
	_ = harness.store.DB().Where("id=?", tc.Run.ID).Take(&run).Error
	if run.NoProgressStreak != 0 || run.LastCycleFingerprint != "" {
		t.Fatalf("reset run=%+v", run)
	}
}

func TestResumeRetryWithoutNewFactsDoesNotResetNoProgress(t *testing.T) {
	harness := newAgentHarness(t, finalResponse("已按无进展预算收尾。"))
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "恢复同一个 Run"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("chat_runs").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Updates(map[string]any{
		"no_progress_streak":     DefaultMaxNoProgressRounds,
		"last_cycle_fingerprint": strings.Repeat("d", 64),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatResume); err != nil {
		t.Fatal(err)
	}
	var run runRecord
	if err := harness.store.DB().Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Take(&run).Error; err != nil {
		t.Fatal(err)
	}
	harness.model.mu.Lock()
	request := harness.model.requests[0]
	harness.model.mu.Unlock()
	if run.Status != TurnCompleted || run.LimitReason != BudgetReasonNoProgress || harness.model.calls != 1 || len(request.Tools) != 0 {
		t.Fatalf("resume retry reset the budget unexpectedly: run=%+v calls=%d tools=%d", run, harness.model.calls, len(request.Tools))
	}
}
