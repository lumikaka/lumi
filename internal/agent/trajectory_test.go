package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"lumi/internal/llm"

	"gorm.io/gorm/logger"
)

type trajectoryQueryCounter struct {
	logger.Interface
	count int
}

func (counter *trajectoryQueryCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	counter.count++
	counter.Interface.Trace(ctx, begin, fc, err)
}

func TestTrajectoryLinksModelRequestsToolsAndFinalAssistantWithoutDuplicatingLogs(t *testing.T) {
	first := llm.ChatResponse{
		Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{
			ID: "trajectory-doc-call", Name: "read_agent_doc", Arguments: `{"path":"/api/v1/agent-docs/overview.md"}`,
		}}},
		Usage: llm.Usage{InputTokens: 120, CachedInputTokens: intPointer(20), OutputTokens: 14}, FinishReason: "tool_calls",
	}
	second := llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "The persisted facts are ready."}, Usage: llm.Usage{InputTokens: 160, OutputTokens: 22}, FinishReason: "stop"}
	harness := newAgentHarness(t, first, second)
	ctx := context.Background()
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "Inspect /Users/private/story Authorization: Bearer secret-token"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}

	page, err := harness.service.ListTrajectory(ctx, harness.project.UUID, thread.UUID, "", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.ModelRequests) != 2 || page.ModelRequests[0].UUID == page.ModelRequests[1].UUID || page.ModelRequests[0].RequestOrdinal != 1 || page.ModelRequests[1].RequestOrdinal != 2 {
		t.Fatalf("model requests=%+v", page.ModelRequests)
	}
	for _, request := range page.ModelRequests {
		if !isUUIDv7(request.UUID) || request.OrderingAccuracy != "exact" || request.StartEventSequence == nil || request.EndEventSequence == nil || *request.StartEventSequence >= *request.EndEventSequence {
			t.Fatalf("request lifecycle=%+v", request)
		}
	}
	if !page.ModelRequests[0].HasToolCalls || page.ModelRequests[0].AssistantPreview != "Tool calls requested" || page.ModelRequests[0].InputTokens == nil || *page.ModelRequests[0].InputTokens != 120 || page.ModelRequests[0].CachedInputTokens == nil || *page.ModelRequests[0].CachedInputTokens != 20 {
		t.Fatalf("first request summary=%+v", page.ModelRequests[0])
	}
	if len(page.Tools) != 1 || page.Tools[0].Status != "completed" || page.Tools[0].RequestUUID != page.ModelRequests[0].UUID || page.Tools[0].RequestOrdinal != 1 || page.Tools[0].OrderingAccuracy != "exact" {
		t.Fatalf("tools=%+v", page.Tools)
	}
	if !strings.Contains(string(page.Tools[0].Arguments), `/api/v1/agent-docs/overview.md`) || strings.Contains(string(page.Tools[0].Arguments), "__provider_call_id") || strings.Contains(string(page.Tools[0].Arguments), "__request_uuid") {
		t.Fatalf("public tool arguments=%s", page.Tools[0].Arguments)
	}
	var finalItem *TrajectorySourceItem
	for index := range page.Items {
		item := &page.Items[index]
		if item.ItemType == "assistant_message" {
			finalItem = item
		}
		if strings.Contains(item.Content, "/Users/private") || strings.Contains(item.Content, "secret-token") {
			t.Fatalf("unsafe trajectory content=%q", item.Content)
		}
	}
	if finalItem == nil || finalItem.RequestUUID != page.ModelRequests[1].UUID || finalItem.RequestOrdinal != 2 || finalItem.OrderingAccuracy != "exact" {
		t.Fatalf("final assistant=%+v", finalItem)
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{`"id":`, "must-not-leak", "/Users/private", "secret-token", "__provider_call_id", `"request_payload":`, `"response":`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("trajectory leaked %q: %s", forbidden, text)
		}
	}
	var logCount int64
	if err := harness.store.DB().Table("llm_logs").Where("chat_thread_id=(SELECT id FROM chat_threads WHERE uuid=?)", thread.UUID).Count(&logCount).Error; err != nil || logCount != 2 {
		t.Fatalf("llm logs=%d err=%v", logCount, err)
	}
}

func TestTrajectoryOverviewStatsUseOnlyCompleteRecordedFacts(t *testing.T) {
	requests := []TrajectoryModelRequest{
		{DurationMS: int64Pointer(1000), InputTokens: intPointer(120), CachedInputTokens: intPointer(20), OutputTokens: intPointer(14), UsageAccuracy: "recorded"},
		{DurationMS: int64Pointer(2500), InputTokens: intPointer(160), CachedInputTokens: intPointer(0), OutputTokens: intPointer(22), UsageAccuracy: "recorded"},
	}
	tools := []TrajectoryTool{{DurationMS: int64Pointer(400)}, {DurationMS: int64Pointer(600)}}
	overview := TrajectoryOverview{}
	populateTrajectoryOverviewStats(&overview, requests, tools)
	if overview.LLMDurationMS == nil || *overview.LLMDurationMS != 3500 || overview.ToolDurationMS == nil || *overview.ToolDurationMS != 1000 {
		t.Fatalf("duration stats=%+v", overview)
	}
	if overview.InputTokens == nil || *overview.InputTokens != 280 || overview.CachedInputTokens == nil || *overview.CachedInputTokens != 20 || overview.OutputTokens == nil || *overview.OutputTokens != 36 || overview.CacheHitPercent == nil || *overview.CacheHitPercent != 7 {
		t.Fatalf("usage stats=%+v", overview)
	}
	if overview.AverageTTFTMS != nil || overview.OutputTokensPerSec != nil {
		t.Fatalf("unrecorded streaming stats must stay nil: %+v", overview)
	}

	requests[1].DurationMS = nil
	requests[1].InputTokens = nil
	overview = TrajectoryOverview{}
	populateTrajectoryOverviewStats(&overview, requests, tools)
	if overview.LLMDurationMS != nil || overview.InputTokens != nil || overview.CachedInputTokens != nil || overview.OutputTokens != nil || overview.CacheHitPercent != nil {
		t.Fatalf("partial request facts must not become exact totals: %+v", overview)
	}
	if overview.ToolDurationMS == nil || *overview.ToolDurationMS != 1000 {
		t.Fatalf("independent complete tool facts should remain available: %+v", overview)
	}
}

func TestTrajectoryRetryUsesNewRequestUUIDAndOrdinal(t *testing.T) {
	harness := newAgentHarness(t)
	harness.model.mu.Lock()
	harness.model.respond = func(call int, _ llm.ChatRequest) (llm.ChatResponse, error) {
		if call == 1 {
			return llm.ChatResponse{}, &llm.Error{Code: llm.CodeNetwork, SafeMessage: "Temporary provider failure.", Retryable: true, Cause: errors.New("retry")}
		}
		return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "Recovered."}, Usage: llm.Usage{InputTokens: 10, OutputTokens: 2}, FinishReason: "stop"}, nil
	}
	harness.model.mu.Unlock()
	ctx := context.Background()
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "retry"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err == nil {
		t.Fatal("first retryable execution unexpectedly succeeded")
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	page, err := harness.service.ListTrajectory(ctx, harness.project.UUID, thread.UUID, "", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.ModelRequests) != 2 || page.ModelRequests[0].UUID == page.ModelRequests[1].UUID || page.ModelRequests[0].RequestOrdinal != 1 || page.ModelRequests[1].RequestOrdinal != 2 || page.ModelRequests[0].Status != "failed" || page.ModelRequests[1].Status != "completed" {
		t.Fatalf("retry requests=%+v", page.ModelRequests)
	}
}

func TestTrajectoryCursorLegacyCompactionAndInterruptedTool(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread := harness.createThread(t)
	turns := make([]Turn, 0, 5)
	for index := 0; index < 5; index++ {
		turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "turn message"})
		if err != nil {
			t.Fatal(err)
		}
		turns = append(turns, turn)
	}

	tail, err := harness.service.ListTrajectory(ctx, harness.project.UUID, thread.UUID, "", "", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail.Items) != 2 || tail.Items[0].Sequence != 4 || tail.Items[1].Sequence != 5 || !tail.CursorPagination.HasMore || tail.HistoryComplete {
		t.Fatalf("tail=%+v", tail)
	}
	middle, err := harness.service.ListTrajectory(ctx, harness.project.UUID, thread.UUID, tail.CursorPagination.PrevCursor, "", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(middle.Items) != 2 || middle.Items[0].Sequence != 2 || middle.Items[1].Sequence != 3 || !middle.CursorPagination.HasMore || middle.HistoryComplete {
		t.Fatalf("middle=%+v", middle)
	}
	oldest, err := harness.service.ListTrajectory(ctx, harness.project.UUID, thread.UUID, middle.CursorPagination.PrevCursor, "", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(oldest.Items) != 1 || oldest.Items[0].Sequence != 1 || oldest.CursorPagination.HasMore || !oldest.HistoryComplete {
		t.Fatalf("oldest=%+v", oldest)
	}

	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turns[len(turns)-1].UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode, err = harness.service.loadRunToolMode(ctx, harness.store, tc)
	if err != nil {
		t.Fatal(err)
	}
	execution, _, _, err := harness.service.persistToolIntent(ctx, harness.store, tc, "legacy-call", "read_agent_doc", `{"path":"/api/v1/agent-docs/overview.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := harness.store.DB().Table("chat_runs").Where("id=?", tc.Run.ID).Updates(map[string]any{"status": TurnCompleted, "completed_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("chat_turns").Where("id=?", tc.Turn.ID).Updates(map[string]any{"status": TurnCompleted, "completed_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	summaryUUID, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	var threadID int64
	if err := harness.store.DB().Table("chat_threads").Where("uuid=?", thread.UUID).Pluck("id", &threadID).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Exec(`INSERT INTO agent_context_summaries(uuid,thread_id,through_item_sequence,summary,source_bytes,created_at) VALUES(?,?,?,?,?,?)`, summaryUUID, threadID, 3, "legacy /Users/private/summary", 2048, now).Error; err != nil {
		t.Fatal(err)
	}

	page, err := harness.service.ListTrajectory(ctx, harness.project.UUID, thread.UUID, "", "", execution.ToolCallUUID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tools) != 1 || page.Tools[0].Status != "interrupted" || page.Tools[0].RequestUUID != "" || page.Tools[0].DerivedReason == "" {
		t.Fatalf("interrupted tool=%+v", page.Tools)
	}
	compactionPage, err := harness.service.ListTrajectory(ctx, harness.project.UUID, thread.UUID, "", "", summaryUUID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(compactionPage.Compactions) != 1 || compactionPage.Compactions[0].TurnUUID != nil || compactionPage.Compactions[0].OrderingAccuracy != "approximate" || strings.Contains(compactionPage.Compactions[0].Summary, "/Users/private") {
		t.Fatalf("legacy compaction=%+v", compactionPage.Compactions)
	}
	missingUUID, _ := newUUIDv7()
	if _, err := harness.service.ListTrajectory(ctx, harness.project.UUID, thread.UUID, "", "", missingUUID, 20); errorCode(err) != CodeNotFound {
		t.Fatalf("missing deep link err=%v", err)
	}
}

func TestTrajectoryQueryCountDoesNotGrowPerItem(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread := harness.createThread(t)
	if _, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "first"}); err != nil {
		t.Fatal(err)
	}
	originalLogger := harness.store.DB().Config.Logger
	counter := &trajectoryQueryCounter{Interface: originalLogger}
	harness.store.DB().Config.Logger = counter
	t.Cleanup(func() { harness.store.DB().Config.Logger = originalLogger })
	if _, err := harness.service.ListTrajectory(ctx, harness.project.UUID, thread.UUID, "", "", "", 100); err != nil {
		t.Fatal(err)
	}
	baseline := counter.count
	for index := 0; index < 12; index++ {
		if _, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "more"}); err != nil {
			t.Fatal(err)
		}
	}
	counter.count = 0
	if _, err := harness.service.ListTrajectory(ctx, harness.project.UUID, thread.UUID, "", "", "", 100); err != nil {
		t.Fatal(err)
	}
	if counter.count != baseline {
		t.Fatalf("trajectory query count grew with items: baseline=%d current=%d", baseline, counter.count)
	}
}
