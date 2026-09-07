package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"lumi/internal/llm"
	"lumi/internal/llmlog"
)

type workflowTrajectoryFixture struct {
	harness                                              *agentHarness
	thread                                               Thread
	projectID, workflowID, stepID, productionID, storyID int64
	createdAt                                            time.Time
}

func insertWorkflowTrajectoryRow(t *testing.T, harness *agentHarness, table string, values map[string]any) (int64, string) {
	t.Helper()
	uuid, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	values["uuid"] = uuid
	if err := harness.store.DB().Table(table).Create(values).Error; err != nil {
		t.Fatalf("insert %s: %v", table, err)
	}
	var id int64
	if err := harness.store.DB().Table(table).Where("uuid=?", uuid).Pluck("id", &id).Error; err != nil {
		t.Fatal(err)
	}
	return id, uuid
}

func newWorkflowTrajectoryFixture(t *testing.T, harness *agentHarness) workflowTrajectoryFixture {
	t.Helper()
	thread := harness.createThread(t)
	var record threadRecord
	if err := harness.store.DB().Where("uuid=?", thread.UUID).First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Model(&record).Update("thread_type", ThreadTypeWorkflow).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Hour)
	workflowID, workflowUUID := insertWorkflowTrajectoryRow(t, harness, "workflows", map[string]any{
		"project_id": record.ProjectID, "thread_id": record.ID, "kind": WorkflowComicSectionImage, "title": "Trajectory workflow", "status": "completed",
		"input_snapshot": "{}", "idempotency_key": "trajectory:" + thread.UUID, "provider_uuid": harness.provider.UUID, "model": "image-model", "created_at": now, "updated_at": now,
	})
	productionID, taskUUID := insertWorkflowTrajectoryRow(t, harness, "production_task_runs", map[string]any{
		"project_id": record.ProjectID, "kind": "comic_image_generation", "resource_uuid": workflowUUID, "status": "completed",
		"input_snapshot": "{}", "idempotency_key": "trajectory:" + thread.UUID, "created_at": now, "updated_at": now,
	})
	var stepID int64
	for index, key := range []string{"references", "image"} {
		stepID, _ = insertWorkflowTrajectoryRow(t, harness, "workflow_steps", map[string]any{
			"workflow_id": workflowID, "step_key": key, "position": index + 1, "task_uuid": taskUUID, "status": "completed",
			"idempotency_key": workflowUUID + ":" + key, "created_at": now, "updated_at": now,
		})
	}
	storyID, storyUUID := insertWorkflowTrajectoryRow(t, harness, "task_runs", map[string]any{
		"project_id": record.ProjectID, "kind": "story_chapter_generation", "resource_uuid": workflowUUID, "input_version": 1,
		"input_snapshot": "{}", "status": "completed", "idempotency_key": "trajectory:" + thread.UUID, "provider_uuid": harness.provider.UUID,
		"model": "story-model", "created_at": now, "updated_at": now,
	})
	insertWorkflowTrajectoryRow(t, harness, "workflow_steps", map[string]any{
		"workflow_id": workflowID, "step_key": "story", "position": 3, "task_uuid": storyUUID, "status": "completed",
		"idempotency_key": workflowUUID + ":story", "created_at": now, "updated_at": now,
	})
	return workflowTrajectoryFixture{harness, thread, record.ProjectID, workflowID, stepID, productionID, storyID, now}
}

func (fixture workflowTrajectoryFixture) addLog(t *testing.T, source, requestType, status string, attempt int, at time.Time, durationMS int64) string {
	t.Helper()
	values := map[string]any{
		"project_id": fixture.projectID, "source_type": source, "scenario": "trajectory_generation", "request_type": requestType,
		"provider_uuid": fixture.harness.provider.UUID, "provider_type": "openai_compatible", "model": "test-model", "status": status, "attempt": attempt,
		"duration_ms": durationMS, "input_summary": "Authorization: Bearer secret-token /Users/private/prompt", "output_summary": "result",
		"request_payload": `{"model":"test-model","prompt":"safe prompt","size":"1024x1024"}`, "created_at": at,
	}
	if status != "pending" {
		values["completed_at"] = at.Add(time.Duration(durationMS) * time.Millisecond)
		values["response"] = `{"content":"safe response"}`
	}
	if status == "failed" {
		values["error_code"] = "image_timeout"
	}
	switch source {
	case "workflow":
		values["workflow_id"], values["workflow_step_id"] = fixture.workflowID, fixture.stepID
	case "production":
		values["production_task_run_id"] = fixture.productionID
	case "story_generation":
		values["task_run_id"] = fixture.storyID
	}
	_, uuid := insertWorkflowTrajectoryRow(t, fixture.harness, "llm_logs", values)
	return uuid
}

func TestWorkflowTrajectoryReadsAllRequestSourcesWithoutChatItems(t *testing.T) {
	harness := newAgentHarness(t)
	fixture := newWorkflowTrajectoryFixture(t, harness)
	first := fixture.addLog(t, "production", "image", "completed", 1, fixture.createdAt, 47061)
	if err := harness.store.DB().Table("llm_logs").Where("uuid=?", first).Updates(map[string]any{"input_tokens": 20, "cached_input_tokens": 10, "output_tokens": 30}).Error; err != nil {
		t.Fatal(err)
	}
	fixture.addLog(t, "workflow", "text", "completed", 1, fixture.createdAt.Add(time.Minute), 1000)
	fixture.addLog(t, "story_generation", "text", "completed", 1, fixture.createdAt.Add(2*time.Minute), 2000)
	fixture.addLog(t, "production", "image", "failed", 2, fixture.createdAt.Add(3*time.Minute), 3000)
	other := newWorkflowTrajectoryFixture(t, harness)
	otherUUID := other.addLog(t, "production", "image", "completed", 1, fixture.createdAt, 500)
	page, err := harness.service.ListTrajectory(context.Background(), harness.project.UUID, fixture.thread.UUID, "", "", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 || len(page.Turns) != 0 || len(page.Tools) != 0 || len(page.ModelRequests) != 4 || !page.HistoryComplete || page.CursorPagination.HasMore {
		t.Fatalf("workflow page=%+v", page)
	}
	if page.Overview.ModelRequestCount != 4 || page.Overview.ToolCount != 0 || page.Overview.LLMDurationMS == nil || *page.Overview.LLMDurationMS != 53061 || len(page.Overview.Timeline) != 4 {
		t.Fatalf("workflow overview=%+v", page.Overview)
	}
	wantedSources := []string{"production", "workflow", "story_generation", "production"}
	var workflow workflowRecord
	if err := harness.store.DB().First(&workflow, fixture.workflowID).Error; err != nil {
		t.Fatal(err)
	}
	if len(page.Workflows) != 1 || page.Workflows[0].UUID != workflow.UUID || page.Workflows[0].Status != "completed" {
		t.Fatalf("workflow execution must retain its own state despite a failed request: %+v", page.Workflows)
	}
	for index, request := range page.ModelRequests {
		if request.UUID == otherUUID || request.ThreadUUID != fixture.thread.UUID || request.TurnUUID != "" || request.RunUUID != "" || request.RequestOrdinal != index+1 || request.SourceType != wantedSources[index] {
			t.Fatalf("request=%+v", request)
		}
		if len(request.WorkflowOrigins) != 1 || request.WorkflowOrigins[0].UUID != workflow.UUID || request.WorkflowOrigins[0].Kind != WorkflowComicSectionImage || request.WorkflowOrigins[0].Title != "Trajectory workflow" {
			t.Fatalf("request origin=%+v", request.WorkflowOrigins)
		}
	}
	image := page.ModelRequests[0]
	if image.UUID != first || image.Attempt != 1 || image.DurationMS == nil || *image.DurationMS != 47061 || image.InputTokens != nil || image.OutputTokens != nil || image.CachedInputTokens != nil || !image.HasRequestPayload || !image.HasResponse {
		t.Fatalf("image=%+v", image)
	}
	last := page.ModelRequests[3]
	if last.Attempt != 2 || last.Status != "failed" || last.ErrorCode != "image_timeout" {
		t.Fatalf("retry=%+v", last)
	}
	encoded, _ := json.Marshal(page)
	for _, forbidden := range []string{`"id":`, "secret-token", "/Users/private", `"request_payload":`, `"response":`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("trajectory leaked %q", forbidden)
		}
	}
	detail, err := llmlog.NewService(harness.store).Get(context.Background(), first)
	if err != nil || len(detail.RequestPayload) == 0 || len(detail.Response) == 0 {
		t.Fatalf("existing log detail=%+v err=%v", detail, err)
	}
}

func TestWorkflowTrajectoryIncludesExecutionWithoutRequestsAndSupportsWorkflowAnchor(t *testing.T) {
	harness := newAgentHarness(t)
	fixture := newWorkflowTrajectoryFixture(t, harness)
	var workflow workflowRecord
	if err := harness.store.DB().First(&workflow, fixture.workflowID).Error; err != nil {
		t.Fatal(err)
	}
	started := fixture.createdAt.Add(time.Second)
	if err := harness.store.DB().Model(&workflow).Updates(map[string]any{"status": "running", "started_at": started, "current_step_key": "image"}).Error; err != nil {
		t.Fatal(err)
	}
	page, err := harness.service.ListTrajectory(context.Background(), harness.project.UUID, fixture.thread.UUID, "", "", workflow.UUID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Workflows) != 1 || page.Workflows[0].ThreadUUID != fixture.thread.UUID || page.Workflows[0].Status != "running" || page.Workflows[0].DurationMS != nil || len(page.ModelRequests) != 0 || !page.HistoryComplete {
		t.Fatalf("execution before any requests=%+v", page)
	}
	first := fixture.addLog(t, "production", "image", "failed", 1, started, 10)
	last := fixture.addLog(t, "production", "image", "completed", 2, started.Add(time.Second), 20)
	completed := started.Add(3 * time.Second)
	if err := harness.store.DB().Model(&workflow).Updates(map[string]any{"status": "completed", "completed_at": completed}).Error; err != nil {
		t.Fatal(err)
	}
	tail, err := harness.service.ListTrajectory(context.Background(), harness.project.UUID, fixture.thread.UUID, "", "", workflow.UUID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail.Workflows) != 1 || tail.Workflows[0].Status != "completed" || tail.Workflows[0].DurationMS == nil || *tail.Workflows[0].DurationMS != 3000 || len(tail.ModelRequests) != 1 || tail.ModelRequests[0].UUID != last || !tail.CursorPagination.HasMore {
		t.Fatalf("workflow selection must retain the call page and lifecycle duration: %+v", tail)
	}
	older, err := harness.service.ListTrajectory(context.Background(), harness.project.UUID, fixture.thread.UUID, tail.CursorPagination.PrevCursor, "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Workflows) != 1 || older.Workflows[0].UUID != workflow.UUID || len(older.ModelRequests) != 1 || older.ModelRequests[0].UUID != first || !older.HistoryComplete || older.Overview.ToolCount != 0 || *older.Overview.LLMDurationMS != 30 || len(older.Overview.Timeline) != 2 {
		t.Fatalf("workflow context must not consume cursor positions or duplicate call metrics: %+v", older)
	}
	other := newWorkflowTrajectoryFixture(t, harness)
	_, err = harness.service.ListTrajectory(context.Background(), harness.project.UUID, other.thread.UUID, "", "", workflow.UUID, 1)
	if errorCode(err) != CodeNotFound {
		t.Fatalf("cross-thread workflow anchor err=%v", err)
	}
}

func TestWorkflowTrajectoryRespectsPageLimitsWithoutPerRequestQueries(t *testing.T) {
	harness := newAgentHarness(t)
	fixture := newWorkflowTrajectoryFixture(t, harness)
	fixture.addLog(t, "production", "image", "completed", 1, fixture.createdAt, 10)
	originalLogger := harness.store.DB().Config.Logger
	counter := &trajectoryQueryCounter{Interface: originalLogger}
	harness.store.DB().Config.Logger = counter
	t.Cleanup(func() { harness.store.DB().Config.Logger = originalLogger })
	read := func(limit int) TrajectoryPage {
		t.Helper()
		counter.count = 0
		page, err := harness.service.ListTrajectory(context.Background(), harness.project.UUID, fixture.thread.UUID, "", "", "", limit)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	read(0)
	baseline := counter.count
	for index := 1; index < 205; index++ {
		fixture.addLog(t, "production", "image", "completed", index+1, fixture.createdAt.Add(time.Duration(index)*time.Second), 10)
	}
	for _, limit := range []struct{ requested, expected int }{{0, 80}, {500, 200}} {
		page := read(limit.requested)
		if len(page.ModelRequests) != limit.expected || page.CursorPagination.PerPage != limit.expected || !page.CursorPagination.HasMore || page.HistoryComplete || page.Overview.ModelRequestCount != 205 || page.ModelRequests[0].RequestOrdinal != 206-limit.expected {
			t.Fatalf("limit=%+v returned=%d pagination=%+v overview=%+v", limit, len(page.ModelRequests), page.CursorPagination, page.Overview)
		}
		if counter.count != baseline {
			t.Fatalf("queries grew with requests: baseline=%d current=%d", baseline, counter.count)
		}
	}
}

func TestWorkflowTrajectoryRejectsMismatchedProjectRelationships(t *testing.T) {
	harness := newAgentHarness(t)
	fixture := newWorkflowTrajectoryFixture(t, harness)
	expected := fixture.addLog(t, "production", "image", "completed", 1, fixture.createdAt, 10)
	otherProjectID, _ := insertWorkflowTrajectoryRow(t, harness, "projects", map[string]any{
		"name": "Other project", "format_version": 1, "schema_version": 1, "created_at": fixture.createdAt, "updated_at": fixture.createdAt,
	})
	foreign := fixture
	foreign.projectID = otherProjectID
	for _, source := range []string{"workflow", "production", "story_generation"} {
		foreign.addLog(t, source, "text", "completed", 1, fixture.createdAt, 100)
	}
	foreignTaskID, foreignTaskUUID := insertWorkflowTrajectoryRow(t, harness, "production_task_runs", map[string]any{
		"project_id": otherProjectID, "kind": "comic_image_generation", "resource_uuid": fixture.thread.UUID, "status": "completed",
		"input_snapshot": "{}", "idempotency_key": "foreign-task", "created_at": fixture.createdAt, "updated_at": fixture.createdAt,
	})
	insertWorkflowTrajectoryRow(t, harness, "workflow_steps", map[string]any{
		"workflow_id": fixture.workflowID, "step_key": "foreign", "position": 4, "task_uuid": foreignTaskUUID,
		"idempotency_key": "foreign-step", "created_at": fixture.createdAt, "updated_at": fixture.createdAt,
	})
	foreign.projectID, foreign.productionID = fixture.projectID, foreignTaskID
	foreign.addLog(t, "production", "image", "completed", 1, fixture.createdAt, 100)
	page, err := harness.service.ListTrajectory(context.Background(), harness.project.UUID, fixture.thread.UUID, "", "", "", 20)
	if err != nil || len(page.ModelRequests) != 1 || page.ModelRequests[0].UUID != expected || page.Overview.ModelRequestCount != 1 {
		t.Fatalf("project isolation page=%+v err=%v", page, err)
	}
}

func TestWorkflowTrajectoryCursorsAnchorsAndPendingUpserts(t *testing.T) {
	harness := newAgentHarness(t)
	fixture := newWorkflowTrajectoryFixture(t, harness)
	uuid1 := fixture.addLog(t, "production", "text", "completed", 1, fixture.createdAt, 100)
	uuid2 := fixture.addLog(t, "production", "image", "failed", 1, fixture.createdAt.Add(time.Second), 200)
	uuid3 := fixture.addLog(t, "production", "image", "failed", 2, fixture.createdAt.Add(time.Second), 300)
	uuid4 := fixture.addLog(t, "production", "image", "pending", 3, fixture.createdAt.Add(2*time.Second), 0)
	read := func(before, after, anchor string, limit int) TrajectoryPage {
		t.Helper()
		page, err := harness.service.ListTrajectory(context.Background(), harness.project.UUID, fixture.thread.UUID, before, after, anchor, limit)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	tail := read("", "", "", 2)
	if len(tail.ModelRequests) != 2 || tail.HistoryComplete || !tail.CursorPagination.HasMore || tail.ModelRequests[1].UUID != uuid4 || tail.ModelRequests[1].RequestOrdinal != 4 || tail.Overview.ActiveRequestCount != 1 || tail.Overview.LLMDurationMS != nil {
		t.Fatalf("tail=%+v", tail)
	}
	older := read(tail.CursorPagination.PrevCursor, "", "", 2)
	if len(older.ModelRequests) != 2 || !older.HistoryComplete || older.CursorPagination.HasMore || older.ModelRequests[0].UUID != uuid1 {
		t.Fatalf("older=%+v", older)
	}
	if older.ModelRequests[1].UUID == tail.ModelRequests[0].UUID || (older.ModelRequests[1].UUID != uuid2 && older.ModelRequests[1].UUID != uuid3) {
		t.Fatal("same-time requests were lost or duplicated")
	}
	next := read("", older.CursorPagination.NextCursor, "", 1)
	if len(next.ModelRequests) != 1 || !next.CursorPagination.HasMore || next.ModelRequests[0].UUID != tail.ModelRequests[0].UUID {
		t.Fatalf("after=%+v", next)
	}
	anchor := read("", "", uuid4, 1)
	if len(anchor.ModelRequests) != 1 || anchor.ModelRequests[0].UUID != uuid4 || anchor.HistoryComplete || !anchor.CursorPagination.HasMore {
		t.Fatalf("anchor=%+v", anchor)
	}
	if err := harness.store.DB().Table("llm_logs").Where("uuid=?", uuid4).Updates(map[string]any{"status": "completed", "duration_ms": 400, "completed_at": fixture.createdAt.Add(3 * time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	updated := read("", older.CursorPagination.NextCursor, "", 2)
	if updated.ModelRequests[1].UUID != uuid4 || updated.ModelRequests[1].Status != "completed" || updated.Overview.ActiveRequestCount != 0 || updated.Overview.LLMDurationMS == nil || *updated.Overview.LLMDurationMS != 1000 {
		t.Fatalf("updated=%+v", updated)
	}
	if page := read(tail.CursorPagination.PrevCursor, "", "", 2); page.CursorPagination.NextCursor != older.CursorPagination.NextCursor {
		t.Fatal("a lifecycle update changed cursor identity")
	}
	other := newWorkflowTrajectoryFixture(t, harness)
	foreign := other.addLog(t, "workflow", "text", "completed", 1, fixture.createdAt, 10)
	for _, input := range []struct{ before, after, anchor, code string }{
		{before: encodeCursor(1), code: CodeValidation},
		{before: "invalid", code: CodeValidation},
		{before: tail.CursorPagination.PrevCursor, after: tail.CursorPagination.NextCursor, code: CodeValidation},
		{before: tail.CursorPagination.PrevCursor, anchor: uuid1, code: CodeValidation},
		{anchor: foreign, code: CodeNotFound},
		{before: encodeWorkflowTrajectoryCursor(other.thread.UUID, workflowTrajectoryCall{"model_request", uuid1, fixture.createdAt}), code: CodeValidation},
		{before: encodeWorkflowTrajectoryCursor(fixture.thread.UUID, workflowTrajectoryCall{"model_request", foreign, fixture.createdAt}), code: CodeValidation},
	} {
		_, err := harness.service.ListTrajectory(context.Background(), harness.project.UUID, fixture.thread.UUID, input.before, input.after, input.anchor, 2)
		if errorCode(err) != input.code {
			t.Fatalf("input=%+v err=%v", input, err)
		}
	}
	decoded, err := base64.RawURLEncoding.DecodeString(tail.CursorPagination.NextCursor)
	if err != nil || strings.Contains(string(decoded), `"id"`) || !strings.Contains(string(decoded), fixture.thread.UUID) {
		t.Fatalf("public cursor=%s err=%v", decoded, err)
	}
}

func TestWorkflowTrajectoryKeepsRealToolAndOutOfPageRequestContext(t *testing.T) {
	harness := newAgentHarness(t,
		llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "workflow-tool", Name: "read_agent_doc", Arguments: `{"path":"/api/v1/agent-docs/overview.md"}`}}}, FinishReason: "tool_calls"},
		llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "Done"}, FinishReason: "stop"},
	)
	ctx := context.Background()
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "Inspect a document"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("chat_threads").Where("uuid=?", thread.UUID).Update("thread_type", ThreadTypeWorkflow).Error; err != nil {
		t.Fatal(err)
	}
	all, err := harness.service.ListTrajectory(ctx, harness.project.UUID, thread.UUID, "", "", "", 20)
	if err != nil || len(all.Tools) != 1 || len(all.ModelRequests) != 2 {
		t.Fatalf("workflow tool page=%+v err=%v", all, err)
	}
	for _, request := range all.ModelRequests {
		if len(request.WorkflowOrigins) != 0 {
			t.Fatalf("chat request has no persisted workflow relation: %+v", request)
		}
	}
	for _, uuid := range []string{all.Tools[0].UUID, all.Tools[0].ToolCallUUID, all.Tools[0].CallItemUUID, all.Tools[0].ResultItemUUID} {
		page, err := harness.service.ListTrajectory(ctx, harness.project.UUID, thread.UUID, "", "", uuid, 1)
		if err != nil || len(page.Tools) != 1 || len(page.ModelRequests) != 1 || page.Tools[0].RequestUUID != page.ModelRequests[0].UUID || page.Tools[0].RequestOrdinal != page.ModelRequests[0].RequestOrdinal || page.Tools[0].Status != "completed" || !page.CursorPagination.HasMore || page.HistoryComplete {
			t.Fatalf("tool anchor %s page=%+v err=%v", uuid, page, err)
		}
		if page.Overview.ToolCount != 1 || page.Overview.ModelRequestCount != 2 || len(page.Tools[0].Arguments) == 0 || len(page.Tools[0].Result) == 0 {
			t.Fatalf("tool facts=%+v", page)
		}
	}
}
