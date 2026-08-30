package agent

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"lumi/internal/llm"
	"lumi/internal/llmlog"
)

func invalidProviderToolResponse(reason llm.ProviderResponseFailureReason, inputTokens, outputTokens int) (llm.ChatResponse, error) {
	choice, tool := 0, 0
	partial := llm.ChatResponse{Usage: llm.Usage{InputTokens: inputTokens, OutputTokens: outputTokens}, FinishReason: "tool_calls"}
	diagnostic := &llm.ProviderResponseDiagnostic{
		Reason: reason, ChoiceIndex: &choice, ToolIndex: &tool, HTTPStatus: 200,
		ContentType: "application/json", FinishReason: partial.FinishReason, Usage: partial.Usage,
		BodyLength: 128, Preview: `{"choices":[{"message":{"tool_calls":[`,
	}
	return partial, &llm.Error{
		Code: llm.CodeInvalidContent, SafeMessage: "Provider 返回了无效工具调用。",
		Retryable: true, ResponseDiagnostic: diagnostic, PartialResponse: &partial,
	}
}

func TestInvalidProviderResponseRetriesOnceWithIndependentLogsAndUsage(t *testing.T) {
	harness := newAgentHarness(t)
	harness.model.respond = func(call int, _ llm.ChatRequest) (llm.ChatResponse, error) {
		if call == 1 {
			return invalidProviderToolResponse(llm.ProviderResponseMissingToolCallID, 17, 3)
		}
		return finalResponse("第二次响应完整。"), nil
	}
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "执行一个操作"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}

	harness.model.mu.Lock()
	calls := harness.model.calls
	requests := append([]llm.ChatRequest(nil), harness.model.requests...)
	harness.model.mu.Unlock()
	if calls != 2 || len(requests) != 2 {
		t.Fatalf("calls=%d requests=%d", calls, len(requests))
	}
	foundRetryInstruction := false
	for _, message := range requests[1].Messages {
		foundRetryInstruction = foundRetryInstruction || (message.Role == "system" && strings.Contains(message.Content, "discarded in full"))
	}
	if !foundRetryInstruction {
		t.Fatal("second physical request did not receive the ephemeral corrective instruction")
	}
	var persistedContextBytes int
	if err := harness.store.DB().Table("chat_runs").Select("context_bytes").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Scan(&persistedContextBytes).Error; err != nil {
		t.Fatal(err)
	}
	if expected := contextRequestBytes(requests[1].Messages, requests[1].Tools); persistedContextBytes != expected {
		t.Fatalf("retry context bytes=%d want=%d", persistedContextBytes, expected)
	}
	var logs []struct {
		Attempt      int
		Status       string
		InputTokens  int
		OutputTokens int
		Response     *string
	}
	if err := harness.store.DB().Table("llm_logs").Select("attempt,status,input_tokens,output_tokens,response").Where("chat_run_id=(SELECT id FROM chat_runs WHERE turn_id=(SELECT id FROM chat_turns WHERE uuid=?))", turn.UUID).Order("id").Scan(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || logs[0].Attempt != 1 || logs[1].Attempt != 2 || logs[0].Status != "failed" || logs[1].Status != "completed" || logs[0].InputTokens != 17 || logs[0].OutputTokens != 3 || logs[0].Response == nil {
		t.Fatalf("logs=%+v", logs)
	}
	var snapshot struct {
		SnapshotType string `json:"snapshot_type"`
		Reason       string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(*logs[0].Response), &snapshot); err != nil || snapshot.SnapshotType != "provider_response_diagnostic" || snapshot.Reason != string(llm.ProviderResponseMissingToolCallID) {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	var toolItems int64
	if err := harness.store.DB().Table("chat_items").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type IN ('tool_call','tool_result')", turn.UUID).Count(&toolItems).Error; err != nil || toolItems != 0 {
		t.Fatalf("tool_items=%d err=%v", toolItems, err)
	}
}

func TestPartialProviderBodyReadErrorUsesOneWireRetry(t *testing.T) {
	harness := newAgentHarness(t)
	harness.model.respond = func(call int, _ llm.ChatRequest) (llm.ChatResponse, error) {
		if call == 1 {
			return invalidProviderToolResponse(llm.ProviderResponseBodyReadError, 0, 0)
		}
		return finalResponse("重试成功。"), nil
	}
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "读取响应中途断开"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	harness.model.mu.Lock()
	calls := harness.model.calls
	harness.model.mu.Unlock()
	if calls != 2 {
		t.Fatalf("partial body read failure used %d physical requests", calls)
	}
	var firstResponse string
	if err := harness.store.DB().Table("llm_logs").Select("response").Where("chat_run_id=(SELECT id FROM chat_runs WHERE turn_id=(SELECT id FROM chat_turns WHERE uuid=?))", turn.UUID).Order("id ASC").Limit(1).Scan(&firstResponse).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		SnapshotType string `json:"snapshot_type"`
		Reason       string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(firstResponse), &snapshot); err != nil || snapshot.SnapshotType != "provider_response_diagnostic" || snapshot.Reason != string(llm.ProviderResponseBodyReadError) {
		t.Fatalf("first response=%s snapshot=%+v err=%v", firstResponse, snapshot, err)
	}
}

func TestInvalidProviderResponseExhaustionDoesNotReachRiverRetryOrCreateToolItems(t *testing.T) {
	harness := newAgentHarness(t)
	harness.model.respond = func(call int, _ llm.ChatRequest) (llm.ChatResponse, error) {
		return invalidProviderToolResponse(llm.ProviderResponseDuplicateToolCallID, 11+call, 2)
	}
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "执行操作"})
	if err != nil {
		t.Fatal(err)
	}
	executeErr := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn)
	if errorCode(executeErr) != llm.CodeInvalidContent {
		t.Fatalf("execute error=%v code=%s", executeErr, errorCode(executeErr))
	}
	harness.model.mu.Lock()
	calls := harness.model.calls
	harness.model.mu.Unlock()
	if calls != maxConsecutiveInvalidProviderResponses {
		t.Fatalf("physical calls=%d", calls)
	}
	turns, err := harness.service.ListTurns(context.Background(), harness.project.UUID, thread.UUID)
	if err != nil || len(turns) != 1 || turns[0].Status != TurnFailed || turns[0].ErrorCode != llm.CodeInvalidContent {
		t.Fatalf("turns=%+v err=%v", turns, err)
	}
	var toolItems int64
	if err := harness.store.DB().Table("chat_items").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type IN ('tool_call','tool_result')", turn.UUID).Count(&toolItems).Error; err != nil || toolItems != 0 {
		t.Fatalf("tool_items=%d err=%v", toolItems, err)
	}
}

func TestMixedRequestUserInputResponseExhaustsOneWireRetryWithoutToolItems(t *testing.T) {
	harness := newAgentHarness(t)
	mixed := llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
		{ID: "ask", Name: "request_user_input", Arguments: `{"input_type":"text","question":"继续吗？"}`},
		{ID: "read", Name: "read_agent_doc", Arguments: `{"path":"/api/v1/agent-docs/overview.md"}`},
	}}, FinishReason: "tool_calls", Usage: llm.Usage{InputTokens: 5, OutputTokens: 2}}
	harness.model.respond = func(_ int, _ llm.ChatRequest) (llm.ChatResponse, error) { return mixed, nil }
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "返回混合工具"})
	if err != nil {
		t.Fatal(err)
	}
	if executeErr := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); errorCode(executeErr) != llm.CodeInvalidContent {
		t.Fatalf("execute error=%v", executeErr)
	}
	harness.model.mu.Lock()
	calls := harness.model.calls
	harness.model.mu.Unlock()
	if calls != maxConsecutiveInvalidProviderResponses {
		t.Fatalf("mixed response physical calls=%d", calls)
	}
	var toolItems, executions, diagnostics int64
	runSubquery := "(SELECT runs.id FROM chat_runs runs JOIN chat_turns turns ON turns.id=runs.turn_id WHERE turns.uuid=?)"
	if err := harness.store.DB().Table("chat_items").Where("run_id="+runSubquery+" AND item_type IN ('tool_call','tool_result')", turn.UUID).Count(&toolItems).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("agent_tool_executions").Where("run_id="+runSubquery, turn.UUID).Count(&executions).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("llm_logs").Where("chat_run_id="+runSubquery+" AND json_extract(response,'$.reason')=?", turn.UUID, string(llm.ProviderResponseRequestUserInputMixed)).Count(&diagnostics).Error; err != nil {
		t.Fatal(err)
	}
	if toolItems != 0 || executions != 0 || diagnostics != maxConsecutiveInvalidProviderResponses {
		t.Fatalf("tool_items=%d executions=%d diagnostics=%d", toolItems, executions, diagnostics)
	}
}

func TestInvalidProviderResponseRetryBudgetRestoresFromDurableLogs(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "恢复后继续"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(context.Background(), harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	response, responseErr := invalidProviderToolResponse(llm.ProviderResponseMalformedJSON, 9, 1)
	var modelErr *llm.Error
	if !errors.As(responseErr, &modelErr) {
		t.Fatalf("model error=%v", responseErr)
	}
	encoded, err := llmlog.EncodeProviderResponseDiagnostic(*modelErr.ResponseDiagnostic, "must-not-leak")
	if err != nil {
		t.Fatal(err)
	}
	handle, err := llmlog.Begin(context.Background(), harness.store, nil, llmlog.StartInput{
		ProjectID: tc.Thread.ProjectID, ChatThreadID: tc.Thread.ID, ChatRunID: tc.Run.ID,
		SourceType: llmlog.SourceProjectChat, Scenario: "project_chat", RequestType: llmlog.RequestText, Attempt: 1,
		ProviderUUID: tc.Run.ProviderUUID, ProviderType: harness.provider.ProviderType, Model: tc.Run.Model,
		RequestPayload: json.RawMessage(`{"model":"test/agent-model","messages":[]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := llmlog.Finish(context.Background(), harness.store, nil, handle, llmlog.FinishInput{
		InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens, FinishReason: response.FinishReason,
		Response: encoded, Err: responseErr,
	}); err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("chat_runs").Where("id=?", tc.Run.ID).Update("model_request_count", 1).Error; err != nil {
		t.Fatal(err)
	}
	harness.model.respond = func(_ int, _ llm.ChatRequest) (llm.ChatResponse, error) {
		return invalidProviderToolResponse(llm.ProviderResponseMissingToolName, 8, 1)
	}
	executeErr := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn)
	if errorCode(executeErr) != llm.CodeInvalidContent {
		t.Fatalf("execute error=%v", executeErr)
	}
	harness.model.mu.Lock()
	calls := harness.model.calls
	harness.model.mu.Unlock()
	if calls != 1 {
		t.Fatalf("restart reset the retry budget: calls=%d", calls)
	}
}

func TestInterruptedProviderRetryRequestConsumesDurableWireBudget(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "恢复中断请求"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(context.Background(), harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	partial, invalidErr := invalidProviderToolResponse(llm.ProviderResponseMalformedJSON, 7, 1)
	var modelErr *llm.Error
	if !errors.As(invalidErr, &modelErr) {
		t.Fatal(invalidErr)
	}
	diagnostic, err := llmlog.EncodeProviderResponseDiagnostic(*modelErr.ResponseDiagnostic, "must-not-leak")
	if err != nil {
		t.Fatal(err)
	}
	first, err := llmlog.Begin(context.Background(), harness.store, nil, llmlog.StartInput{
		ProjectID: tc.Thread.ProjectID, ChatThreadID: tc.Thread.ID, ChatRunID: tc.Run.ID,
		SourceType: llmlog.SourceProjectChat, Scenario: "project_chat", RequestType: llmlog.RequestText, Attempt: 1,
		ProviderUUID: tc.Run.ProviderUUID, ProviderType: harness.provider.ProviderType, Model: tc.Run.Model,
		RequestPayload: json.RawMessage(`{"model":"test/agent-model","messages":[]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := llmlog.Finish(context.Background(), harness.store, nil, first, llmlog.FinishInput{
		InputTokens: partial.Usage.InputTokens, OutputTokens: partial.Usage.OutputTokens,
		FinishReason: partial.FinishReason, Response: diagnostic, Err: invalidErr,
	}); err != nil {
		t.Fatal(err)
	}
	retryPayload, err := llmlog.EncodeChatRequest(llm.ChatRequest{
		Model:    tc.Run.Model,
		Messages: providerResponseRetryMessages([]llm.ChatMessage{{Role: "user", Content: "恢复中断请求"}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := llmlog.Begin(context.Background(), harness.store, nil, llmlog.StartInput{
		ProjectID: tc.Thread.ProjectID, ChatThreadID: tc.Thread.ID, ChatRunID: tc.Run.ID,
		SourceType: llmlog.SourceProjectChat, Scenario: "project_chat", RequestType: llmlog.RequestText, Attempt: 2,
		ProviderUUID: tc.Run.ProviderUUID, ProviderType: harness.provider.ProviderType, Model: tc.Run.Model,
		RequestPayload: retryPayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Exec(`UPDATE llm_logs SET status='failed',error_code='provider_call_interrupted',completed_at=? WHERE id=?`, time.Now().UTC(), interrupted.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("chat_runs").Where("id=?", tc.Run.ID).Update("model_request_count", 2).Error; err != nil {
		t.Fatal(err)
	}

	executeErr := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn)
	if errorCode(executeErr) != llm.CodeInvalidContent {
		t.Fatalf("execute error=%v", executeErr)
	}
	harness.model.mu.Lock()
	calls := harness.model.calls
	harness.model.mu.Unlock()
	if calls != 0 {
		t.Fatalf("interrupted retry reset wire budget: calls=%d", calls)
	}
}

func TestConcurrentWorkersShareOneInvalidProviderWireBudget(t *testing.T) {
	harness := newAgentHarness(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	harness.model.respond = func(call int, _ llm.ChatRequest) (llm.ChatResponse, error) {
		if call == 1 {
			startOnce.Do(func() { close(started) })
			<-release
			return invalidProviderToolResponse(llm.ProviderResponseMissingToolCallID, 4, 1)
		}
		return finalResponse("并发请求已串行收敛。"), nil
	}
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "并发恢复"})
	if err != nil {
		t.Fatal(err)
	}
	errorsCh := make(chan error, 2)
	go func() {
		errorsCh <- harness.service.ExecuteJob(context.Background(), harness.store, JobSpec{Version: 1, ProjectUUID: harness.project.UUID, JobKind: JobChatTurn, ResourceUUID: turn.UUID, ThreadUUID: thread.UUID})
	}()
	<-started
	go func() {
		errorsCh <- harness.service.ExecuteJob(context.Background(), harness.store, JobSpec{Version: 1, ProjectUUID: harness.project.UUID, JobKind: JobChatTurn, ResourceUUID: turn.UUID, ThreadUUID: thread.UUID})
	}()
	close(release)
	for range 2 {
		if err := <-errorsCh; err != nil {
			t.Fatal(err)
		}
	}
	harness.model.mu.Lock()
	calls := harness.model.calls
	harness.model.mu.Unlock()
	if calls != maxConsecutiveInvalidProviderResponses {
		t.Fatalf("concurrent workers issued %d physical requests", calls)
	}
}

func TestNetworkFailureKeepsLLMLogResponseNull(t *testing.T) {
	harness := newAgentHarness(t)
	harness.model.respond = func(_ int, _ llm.ChatRequest) (llm.ChatResponse, error) {
		return llm.ChatResponse{}, &llm.Error{Code: llm.CodeNetwork, SafeMessage: "Provider 网络请求失败。", Retryable: false}
	}
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "网络失败"})
	if err != nil {
		t.Fatal(err)
	}
	if executeErr := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); errorCode(executeErr) != llm.CodeNetwork {
		t.Fatalf("execute error=%v", executeErr)
	}
	var response *string
	if err := harness.store.DB().Table("llm_logs").Select("response").Where("chat_run_id=(SELECT id FROM chat_runs WHERE turn_id=(SELECT id FROM chat_turns WHERE uuid=?))", turn.UUID).Order("id DESC").Limit(1).Scan(&response).Error; err != nil {
		t.Fatal(err)
	}
	if response != nil {
		t.Fatalf("network failure persisted a fake response: %s", *response)
	}
}

func TestAgentProviderFinishReasonIsBoundedAndRedactedInLogColumnsAndSnapshot(t *testing.T) {
	harness := newAgentHarness(t, llm.ChatResponse{
		Message:      llm.ChatMessage{Role: "assistant", Content: "完成"},
		FinishReason: "stop must-not-leak https://signed.example/object /Users/private/file " + strings.Repeat("诊", 300),
	})
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "完成一次调用"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	var log struct {
		FinishReason string
		Response     *string
	}
	if err := harness.store.DB().Table("llm_logs").Select("finish_reason,response").Where("chat_run_id=(SELECT id FROM chat_runs WHERE turn_id=(SELECT id FROM chat_turns WHERE uuid=?))", turn.UUID).Take(&log).Error; err != nil {
		t.Fatal(err)
	}
	joined := log.FinishReason
	if log.Response != nil {
		joined += *log.Response
	}
	for _, forbidden := range []string{"must-not-leak", "signed.example", "/Users/private"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("finish reason leaked %q: %s", forbidden, joined)
		}
	}
	if len(log.FinishReason) > 255 {
		t.Fatalf("finish_reason bytes=%d", len(log.FinishReason))
	}
}

func TestCompletedNetworkFailureBreaksConsecutiveInvalidContentStreak(t *testing.T) {
	harness := newAgentHarness(t)
	harness.model.respond = func(call int, _ llm.ChatRequest) (llm.ChatResponse, error) {
		switch call {
		case 1:
			return invalidProviderToolResponse(llm.ProviderResponseMalformedJSON, 5, 1)
		case 2:
			return llm.ChatResponse{}, &llm.Error{Code: llm.CodeNetwork, SafeMessage: "Provider 网络请求失败。", Retryable: true}
		default:
			return finalResponse("网络恢复后完成。"), nil
		}
	}
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "结构错误后网络失败"})
	if err != nil {
		t.Fatal(err)
	}
	if executeErr := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); errorCode(executeErr) != llm.CodeNetwork {
		t.Fatalf("first execute error=%v", executeErr)
	}
	tc, err := harness.service.loadToolContext(context.Background(), harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	count, err := harness.service.consecutiveInvalidProviderResponses(context.Background(), harness.store, tc.Run.ID)
	if err != nil || count != 0 {
		t.Fatalf("network failure continued structural streak: count=%d err=%v", count, err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	harness.model.mu.Lock()
	calls := harness.model.calls
	harness.model.mu.Unlock()
	if calls != 3 {
		t.Fatalf("physical calls=%d want=3", calls)
	}
}

func TestPendingInitialProviderRequestConsumesOneUnknownWireSlot(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "恢复首次中断请求"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(context.Background(), harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := llmlog.EncodeChatRequest(llm.ChatRequest{Model: tc.Run.Model, Messages: []llm.ChatMessage{{Role: "user", Content: "恢复首次中断请求"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := llmlog.Begin(context.Background(), harness.store, nil, llmlog.StartInput{
		ProjectID: tc.Thread.ProjectID, ChatThreadID: tc.Thread.ID, ChatRunID: tc.Run.ID,
		SourceType: llmlog.SourceProjectChat, Scenario: "project_chat", RequestType: llmlog.RequestText, Attempt: 1,
		ProviderUUID: tc.Run.ProviderUUID, ProviderType: harness.provider.ProviderType, Model: tc.Run.Model, RequestPayload: payload,
	}); err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("chat_runs").Where("id=?", tc.Run.ID).Update("model_request_count", 1).Error; err != nil {
		t.Fatal(err)
	}
	count, err := harness.service.consecutiveInvalidProviderResponses(context.Background(), harness.store, tc.Run.ID)
	if err != nil || count != 1 {
		t.Fatalf("pending initial request count=%d err=%v", count, err)
	}
	harness.model.respond = func(_ int, _ llm.ChatRequest) (llm.ChatResponse, error) {
		return invalidProviderToolResponse(llm.ProviderResponseMissingToolCallID, 4, 1)
	}
	if executeErr := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); errorCode(executeErr) != llm.CodeInvalidContent {
		t.Fatalf("execute error=%v", executeErr)
	}
	harness.model.mu.Lock()
	calls := harness.model.calls
	harness.model.mu.Unlock()
	if calls != 1 {
		t.Fatalf("unknown initial wire slot was reset: calls=%d", calls)
	}
}

func TestModelUsageTokenUnitsNeverSubtractsBudget(t *testing.T) {
	if got := modelUsageTokenUnits(llm.Usage{InputTokens: -1000, OutputTokens: 7}, 0); got != 7 {
		t.Fatalf("negative input changed budget charge: %d", got)
	}
	if got := modelUsageTokenUnits(llm.Usage{InputTokens: -1000, OutputTokens: -7}, 9); got != 3 {
		t.Fatalf("negative usage did not use safe byte fallback: %d", got)
	}
}

func TestDiagnosticBodyLengthIsUsedForFallbackTokenAccounting(t *testing.T) {
	harness := newAgentHarness(t)
	harness.model.respond = func(_ int, _ llm.ChatRequest) (llm.ChatResponse, error) {
		partial, err := invalidProviderToolResponse(llm.ProviderResponseBodyTooLarge, 0, 0)
		var modelErr *llm.Error
		if errors.As(err, &modelErr) {
			modelErr.ResponseDiagnostic.BodyLength = 4 << 20
			modelErr.ResponseDiagnostic.BodyTruncated = true
		}
		return partial, err
	}
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "超大响应"})
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
	resolved, err := harness.providers.Resolve(context.Background(), tc.Run.ProviderUUID)
	if err != nil {
		t.Fatal(err)
	}
	messages := []llm.ChatMessage{{Role: "user", Content: "超大响应"}}
	if _, requestErr := harness.service.performChatModelRequest(context.Background(), harness.store, &tc, resolved, messages, nil, contextRequestBytes(messages, nil), "project_chat"); invalidProviderResponse(requestErr) == nil {
		t.Fatalf("request error=%v", requestErr)
	}
	updated, err := harness.service.refreshRun(context.Background(), harness.store, tc.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.TokenUnits < (4<<20)/4 {
		t.Fatalf("token fallback=%d did not account for diagnostic body length", updated.TokenUnits)
	}
}

func TestDiagnosticBodyLengthFallbackAccountingSaturates(t *testing.T) {
	if got := saturatingAddInt64(1024, math.MaxInt64); got != math.MaxInt64 {
		t.Fatalf("fallback bytes overflowed: got=%d want=%d", got, int64(math.MaxInt64))
	}
	if got := modelUsageTokenUnits(llm.Usage{}, math.MaxInt64); got != math.MaxInt64/4+1 {
		t.Fatalf("saturated fallback token units=%d want=%d", got, int64(math.MaxInt64/4+1))
	}
}
