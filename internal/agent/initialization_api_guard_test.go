package agent

import (
	"context"
	"testing"

	"lumi/internal/project"
)

func TestInitializationBlocksAgentProductionWritesBeforeDispatch(t *testing.T) {
	h := newAgentHarness(t)
	thread := h.createThread(t)
	tc := toolContext{ProjectUUID: h.project.UUID, Thread: threadRecord{UUID: thread.UUID}, ToolMode: ToolModeProjectAPI}
	workflow, err := h.service.CreateYoloWorkflow(t.Context(), h.project.UUID, CreateYoloInput{Title: "初始化", StoryPrompt: "生成绘本", IdempotencyKey: "api-initialization-lock"})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/projects/" + h.project.UUID
	for _, test := range []struct {
		name, method, path string
		body               map[string]any
	}{
		{"chapter create", "POST", base + "/chapters", map[string]any{"chapter_code": "vol01.ch02", "title": "不应创建"}},
		{"generation", "POST", base + "/chapters/" + mustAgentUUID(t) + "/generations", map[string]any{"prompt_key": "story_chapter", "prompt": "不应生成"}},
		{"dangerous write", "DELETE", base + "/chapters/" + mustAgentUUID(t), map[string]any{"expected_revision": float64(1)}},
		{"child retry", "POST", base + "/tasks/" + mustAgentUUID(t) + "/retries", map[string]any{}},
		{"child cancel", "POST", base + "/production-tasks/" + mustAgentUUID(t) + "/cancellations", map[string]any{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := executeRequestAPITool(t.Context(), h.service, h.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t)}, map[string]any{
				"method": test.method, "url": test.path, "request_body": test.body, "response_filter": ".data | {uuid}",
			})
			if errorCode(err) != project.CodeProjectInitializing {
				t.Fatalf("write escaped initialization guard: %v", err)
			}
		})
	}
	if len(h.queue.requests) != 0 {
		t.Fatal("blocked generation reached the domain queue")
	}
	var chapters int64
	if err := h.store.DB().Table("chapters").Count(&chapters).Error; err != nil || chapters != 0 {
		t.Fatalf("blocked create wrote chapters: count=%d err=%v", chapters, err)
	}
	value, err := executeRequestAPITool(t.Context(), h.service, h.store, tc, toolExecutionRecord{}, map[string]any{
		"method": "GET", "url": base, "response_filter": ".data | {uuid}",
	})
	if err != nil || value == nil {
		t.Fatalf("read blocked during initialization: %v", err)
	}
	if _, err := h.service.CreateYoloWorkflow(t.Context(), h.project.UUID, CreateYoloInput{Title: "初始化", StoryPrompt: "生成绘本", IdempotencyKey: "api-initialization-lock"}); err != nil {
		t.Fatalf("existing workflow replay blocked: %v", err)
	}
	if err := h.store.DB().Exec("UPDATE workflows SET status='failed' WHERE uuid=?", workflow.UUID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.TakeOverInitialization(t.Context(), workflow.UUID); err != nil {
		t.Fatal(err)
	}
	_, err = executeRequestAPITool(t.Context(), h.service, h.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t)}, map[string]any{
		"method": "POST", "url": base + "/chapters", "request_body": map[string]any{"chapter_code": "vol01.ch02", "title": "接管后创建"}, "response_filter": ".data | {uuid}",
	})
	if err != nil {
		t.Fatalf("write remained blocked after takeover: %v", err)
	}
}

func TestInitializationGuardCoversDirectAndDiscoveredAgentRoutes(t *testing.T) {
	h := newAgentHarness(t)
	if _, err := h.service.CreateYoloWorkflow(t.Context(), h.project.UUID, CreateYoloInput{Title: "初始化", StoryPrompt: "生成绘本", IdempotencyKey: "direct-api-initialization-lock"}); err != nil {
		t.Fatal(err)
	}
	dispatched := false
	h.service.projectAPIDispatcher = func(context.Context, ProjectAPIDispatchRequest) (ProjectAPIDispatchResponse, error) {
		dispatched = true
		return ProjectAPIDispatchResponse{}, nil
	}
	for _, discovered := range []bool{false, true} {
		_, err := executeAgentAPIRoute(t.Context(), h.service, h.store, toolContext{}, toolExecutionRecord{}, agentAPIRequest{
			Method: "PATCH", Route: agentAPIRoute{ID: RouteStoryProfileUpdate, Handler: RouteStoryProfileUpdate}, UseDispatcher: discovered,
		})
		if errorCode(err) != project.CodeProjectInitializing || dispatched {
			t.Fatalf("direct route bypassed guard: discovered=%v dispatched=%v err=%v", discovered, dispatched, err)
		}
	}
}

type recoveringInitializationQueue struct {
	*agentQueueFake
	task DomainTask
}

func (q *recoveringInitializationQueue) RecoverExternalTask(context.Context, string, string, string) (DomainTask, bool, error) {
	return q.task, q.task.UUID != "", nil
}

func TestExternalInitializationGuardAllowsAcceptedTaskRecovery(t *testing.T) {
	projectUUID, chapterUUID := mustAgentUUID(t), mustAgentUUID(t)
	queue := &recoveringInitializationQueue{agentQueueFake: &agentQueueFake{}, task: DomainTask{UUID: mustAgentUUID(t), Kind: "story_chapter_generation", Status: "running", ResourceUUID: chapterUUID}}
	api := NewExternalProjectAPI(nil, queue)
	request, err := api.Prepare(projectUUID, map[string]any{"method": "POST", "url": "/api/v1/projects/" + projectUUID + "/chapters/" + chapterUUID + "/generations", "request_body": map[string]any{"prompt_key": "story_chapter", "prompt": "生成章节"}, "response_filter": ".data | {uuid,status}"})
	if err != nil {
		t.Fatal(err)
	}
	guarded := false
	guard := func() error {
		guarded = true
		return project.InitializationLockedError()
	}
	if _, err := api.ExecuteGuarded(t.Context(), request, mustAgentUUID(t), guard); err != nil || guarded || len(queue.requests) != 0 {
		t.Fatalf("accepted task recovery was treated as a write: guarded=%v err=%v", guarded, err)
	}
	queue.task = DomainTask{}
	if _, err := api.ExecuteGuarded(t.Context(), request, mustAgentUUID(t), guard); err == nil || !guarded || len(queue.requests) != 0 {
		t.Fatalf("new external task escaped guard: guarded=%v err=%v", guarded, err)
	}
}
