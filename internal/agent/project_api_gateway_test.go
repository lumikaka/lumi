package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"lumi/internal/llm"
)

func TestProjectAPIGatewayMergesReviewedAndDiscoveredRoutes(t *testing.T) {
	projectUUID := mustAgentUUID(t)
	service := (&Service{}).WithProjectAPIGateway([]ProjectAPIRouteSpec{
		{Method: http.MethodGet, Path: "/api/v1/projects/:project_uuid/story-profile"},
		{Method: http.MethodGet, Path: "/api/v1/projects/:project_uuid/premise-assets"},
		{Method: http.MethodGet, Path: "/api/v1/projects/:project_uuid/llm-logs"},
		{Method: http.MethodPatch, Path: "/api/v1/projects/:project_uuid/model-settings"},
		{Method: http.MethodPatch, Path: "/api/v1/projects/:project_uuid/prompt-groups/:prompt_group"},
		{Method: http.MethodGet, Path: "/api/v1/projects/:project_uuid/project-setup"},
		{Method: http.MethodPatch, Path: "/api/v1/projects/:project_uuid/project-setup"},
		{Method: http.MethodPost, Path: "/api/v1/projects/:project_uuid/project-setup-finalizations"},
		{Method: http.MethodPost, Path: "/api/v1/projects/:project_uuid/workflows"},
		{Method: http.MethodGet, Path: "/api/v1/providers"},
	}, nil)
	if len(service.requestAPIRoutes()) != 9 {
		t.Fatalf("routes=%d want=9", len(service.requestAPIRoutes()))
	}

	byKey := map[string]agentAPIRoute{}
	for _, route := range service.requestAPIRoutes() {
		byKey[agentAPIRouteKey(route.Method, route.PathTemplate)] = route
	}
	if route := byKey[agentAPIRouteKey(http.MethodGet, "/api/v1/projects/{project_uuid}/story-profile")]; route.ID != RouteStoryProfileGet || route.Passthrough {
		t.Fatalf("reviewed route overlay lost: %+v", route)
	}
	if route := byKey[agentAPIRouteKey(http.MethodGet, "/api/v1/projects/{project_uuid}/llm-logs")]; route.Handler != routeProjectAPIDispatch || !route.Passthrough || !route.ReadOnly || route.RequiresConfirmation {
		t.Fatalf("discovered GET route invalid: %+v", route)
	}
	if route := byKey[agentAPIRouteKey(http.MethodPatch, "/api/v1/projects/{project_uuid}/model-settings")]; route.Handler != routeProjectAPIDispatch || !route.Passthrough || route.Risk != RiskDangerous || !route.RequiresConfirmation {
		t.Fatalf("discovered write route did not fail closed: %+v", route)
	}
	if route := byKey[agentAPIRouteKey(http.MethodGet, "/api/v1/projects/{project_uuid}/project-setup")]; route.ID != RouteProjectSetupGet || route.Passthrough || !route.ReadOnly || !route.StrictSchema || route.DocPath != projectSetupDocPath || !strings.Contains(route.RecommendedResponseFilter, "draft_values") || strings.Contains(route.RecommendedResponseFilter, "candidate") {
		t.Fatalf("reviewed setup GET route invalid: %+v", route)
	}
	if route := byKey[agentAPIRouteKey(http.MethodPatch, "/api/v1/projects/{project_uuid}/project-setup")]; route.ID != RouteProjectSetupUpdate || route.Passthrough || route.Risk != RiskWrite || !route.ExpectedRevision || !route.StrictSchema {
		t.Fatalf("reviewed setup PATCH route invalid: %+v", route)
	}
	if route := byKey[agentAPIRouteKey(http.MethodPost, "/api/v1/projects/{project_uuid}/project-setup-finalizations")]; route.ID != RouteProjectSetupFinalize || route.Passthrough || route.Risk != RiskDangerous || !route.RequiresConfirmation || !route.ExpectedRevision || !route.StrictSchema {
		t.Fatalf("reviewed setup finalization route invalid: %+v", route)
	}
	if route := byKey[agentAPIRouteKey(http.MethodPost, "/api/v1/projects/{project_uuid}/workflows")]; route.ID != RouteYoloWorkflowCreate || route.Passthrough || route.Risk != RiskWrite || route.RequiresConfirmation || !route.StrictSchema || route.DocPath != workflowDocPath {
		t.Fatalf("reviewed YOLO route invalid: %+v", route)
	}

	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	request, err := service.parseAgentAPIRequest(tc, map[string]any{
		"method": http.MethodGet, "url": "/api/v1/projects/" + projectUUID + "/llm-logs",
		"query": map[string]any{"limit": float64(5)}, "response_filter": ".data | {items}",
	})
	if err != nil || request.Route.Handler != routeProjectAPIDispatch {
		t.Fatalf("discovered route rejected: request=%+v err=%v", request, err)
	}
	if _, err := service.parseAgentAPIRequest(tc, map[string]any{
		"method": http.MethodPatch, "url": "/api/v1/projects/" + projectUUID + "/prompt-groups/story-profile",
		"request_body": map[string]any{"enabled": true}, "response_filter": ".data",
	}); err != nil {
		t.Fatalf("non-UUID path parameter rejected: %v", err)
	}
	assetList, err := service.parseAgentAPIRequest(tc, map[string]any{
		"method": http.MethodGet, "url": "/api/v1/projects/" + projectUUID + "/premise-assets",
		"query": map[string]any{"state": "trashed"}, "response_filter": ".data.items[] | {uuid,title}",
	})
	if err != nil || assetList.UseDispatcher || !assetList.Route.StrictSchema {
		t.Fatalf("reviewed premise asset query lost its strict in-process contract: request=%+v err=%v", assetList, err)
	}
	storyProfile, err := service.parseAgentAPIRequest(tc, map[string]any{
		"method": http.MethodGet, "url": "/api/v1/projects/" + projectUUID + "/story-profile", "response_filter": ".data | {uuid,revision}",
	})
	if err != nil || storyProfile.UseDispatcher {
		t.Fatalf("reviewed request did not retain in-process handler: request=%+v err=%v", storyProfile, err)
	}
	if _, err := service.parseAgentAPIRequest(tc, map[string]any{
		"method": http.MethodGet, "url": "/api/v1/projects/" + projectUUID + "/not-real", "response_filter": ".data",
	}); err == nil || errorCode(err) != CodeToolNotAllowed {
		t.Fatalf("unknown route accepted: %v", err)
	}
	otherProjectUUID := mustAgentUUID(t)
	if _, err := service.parseAgentAPIRequest(tc, map[string]any{
		"method": http.MethodGet, "url": "/api/v1/projects/" + otherProjectUUID + "/project-setup",
		"response_filter": ".data | {uuid,project_uuid,setup_status,revision}",
	}); err == nil {
		t.Fatal("cross-project Project Setup request was accepted")
	}
	if _, err := service.parseAgentAPIRequest(tc, map[string]any{
		"method": http.MethodPatch, "url": "/api/v1/projects/" + projectUUID + "/project-setup",
		"request_body":    map[string]any{"expected_revision": float64(1), "setup_uuid": mustAgentUUID(t)},
		"response_filter": ".data | {uuid,project_uuid,setup_status,revision}",
	}); err == nil {
		t.Fatal("caller-controlled setup_uuid was accepted")
	}
	for _, forbidden := range []string{"title", "provider_uuid", "idempotency_key", "chapter_count", "max_section_count"} {
		body := map[string]any{"story_prompt": "月光邮差", forbidden: "forbidden"}
		if forbidden == "chapter_count" || forbidden == "max_section_count" {
			body[forbidden] = float64(6)
		}
		if _, err := service.parseAgentAPIRequest(tc, map[string]any{
			"method": http.MethodPost, "url": "/api/v1/projects/" + projectUUID + "/workflows",
			"request_body": body, "response_filter": ".data | {uuid,thread_uuid,status}",
		}); err == nil || errorCode(err) != CodeToolValidation {
			t.Fatalf("caller-controlled YOLO field %s was accepted: %v", forbidden, err)
		}
	}

	overview, err := service.readAgentDoc(tc, map[string]any{"path": agentDocOverviewPath})
	if err != nil || !strings.Contains(overview["content"].(string), projectDocPath) || strings.Contains(overview["content"].(string), "project_api.get.llm_logs") {
		t.Fatalf("overview did not retain only the API Contract index: err=%v", err)
	}
	projectDoc, err := service.readAgentDoc(tc, map[string]any{"path": projectDocPath})
	if err != nil || strings.Contains(projectDoc["content"].(string), "project_api.get.llm_logs") || !strings.Contains(projectDoc["content"].(string), "`GET /api/v1/projects/{project_uuid}`") {
		t.Fatalf("static Project Contract was mutated by discovered routes: err=%v", err)
	}
}

func TestReviewedProjectAPIRouteNormalizesEmptyQueryBeforeDispatch(t *testing.T) {
	projectUUID := mustAgentUUID(t)
	chapterUUID := mustAgentUUID(t)
	sectionUUID := mustAgentUUID(t)
	queue := &agentQueueFake{}
	dispatched := false
	service := (&Service{queue: queue}).WithProjectAPIGateway([]ProjectAPIRouteSpec{
		{Method: http.MethodPost, Path: "/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid/image-generations"},
	}, func(_ context.Context, _ ProjectAPIDispatchRequest) (ProjectAPIDispatchResponse, error) {
		dispatched = true
		return ProjectAPIDispatchResponse{}, nil
	})
	tc := toolContext{
		ProjectUUID: projectUUID,
		ToolMode:    ToolModeProjectAPI,
		Thread:      threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject},
		Turn:        turnRecord{UUID: mustAgentUUID(t)},
		Run:         runRecord{UUID: mustAgentUUID(t)},
	}
	idempotencyKey := "agent-tool-v1:" + strings.Repeat("a", 64)
	value, err := executeRequestAPITool(context.Background(), service, nil, tc, toolExecutionRecord{
		UUID: mustAgentUUID(t), IdempotencyKey: idempotencyKey,
	}, map[string]any{
		"method": http.MethodPost,
		"url": "/api/v1/projects/" + projectUUID + "/chapters/" + chapterUUID +
			"/comic-sections/" + sectionUUID + "/image-generations",
		"query":           map[string]any{},
		"request_body":    map[string]any{"prompt": "重新生成第一页图片"},
		"response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dispatched {
		t.Fatal("reviewed route with an empty query was incorrectly sent to the HTTP dispatcher")
	}
	if len(queue.requests) != 1 || queue.requests[0].IdempotencyKey != idempotencyKey {
		t.Fatalf("domain task request=%+v", queue.requests)
	}
	result, _ := value.(map[string]any)
	if result["status"] != "queued" || result["resource_uuid"] != sectionUUID {
		t.Fatalf("generation result=%+v", result)
	}
}

func TestDiscoveredProjectAPIRoutesAreGroupedIntoDomainContracts(t *testing.T) {
	tests := []struct {
		path string
		doc  string
	}{
		{path: "/api/v1/projects/{project_uuid}/llm-logs", doc: projectDocPath},
		{path: "/api/v1/projects/{project_uuid}/chapters/{chapter_uuid}", doc: chapterDocPath},
		{path: "/api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic", doc: comicDocPath},
		{path: "/api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}", doc: comicSectionDocPath},
		{path: "/api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/storyboard-variants", doc: storyboardDocPath},
		{path: "/api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-snapshots", doc: comicSnapshotDocPath},
		{path: "/api/v1/projects/{project_uuid}/comic-exports", doc: comicExportDocPath},
		{path: "/api/v1/projects/{project_uuid}/premise", doc: premiseDocPath},
		{path: "/api/v1/projects/{project_uuid}/premise-assets", doc: premiseAssetDocPath},
		{path: "/api/v1/projects/{project_uuid}/assets", doc: projectAssetDocPath},
		{path: "/api/v1/projects/{project_uuid}/story-profile", doc: storyDocPath},
		{path: "/api/v1/projects/{project_uuid}/story-profile/generations", doc: generationDocPath},
		{path: "/api/v1/projects/{project_uuid}/production-tasks", doc: taskDocPath},
		{path: "/api/v1/projects/{project_uuid}/project-setup", doc: projectSetupDocPath},
		{path: "/api/v1/projects/{project_uuid}/project-setup-finalizations", doc: projectSetupDocPath},
		{path: "/api/v1/projects/{project_uuid}/workflows", doc: workflowDocPath},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			if got := projectAPIDocPath(test.path); got != test.doc {
				t.Fatalf("doc=%s want=%s", got, test.doc)
			}
		})
	}
}

func TestSystemPromptIncludesCompleteAPIDocOverviewWithoutConcreteRoutes(t *testing.T) {
	harness := newAgentHarness(t, finalResponse("完成"))
	harness.service.WithProjectAPIGateway([]ProjectAPIRouteSpec{
		{Method: http.MethodGet, Path: "/api/v1/projects/:project_uuid/premise"},
		{Method: http.MethodGet, Path: "/api/v1/projects/:project_uuid/premise-assets"},
		{Method: http.MethodGet, Path: "/api/v1/projects/:project_uuid/llm-logs"},
	}, nil)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "读取项目设定"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}

	harness.model.mu.Lock()
	requests := append([]llm.ChatRequest(nil), harness.model.requests...)
	harness.model.mu.Unlock()
	if len(requests) != 1 || len(requests[0].Messages) == 0 || requests[0].Messages[0].Role != "system" {
		t.Fatalf("model requests=%+v", requests)
	}
	overview, err := renderAgentDocWithRoutes(agentDocOverviewPath, harness.service.requestAPIRoutes())
	if err != nil {
		t.Fatal(err)
	}
	systemPrompt := requests[0].Messages[0].Content
	if !strings.Contains(systemPrompt, strings.TrimSpace(overview)) {
		t.Fatalf("system prompt does not contain the complete effective API overview:\n%s", systemPrompt)
	}
	for _, expected := range []string{
		"`/api/v1/agent-docs/api/premise.md`",
		"`/api/v1/agent-docs/api/premise-asset.md`",
		"`/api/v1/agent-docs/api/project.md`",
	} {
		if !strings.Contains(systemPrompt, expected) {
			t.Fatalf("system prompt is missing API Contract index entry %q", expected)
		}
	}
	for _, unexpected := range []string{
		"`/api/v1/projects/{project_uuid}/premise`",
		"`/api/v1/projects/{project_uuid}/premise-assets`",
		"`project_api.get.llm_logs`",
	} {
		if strings.Contains(systemPrompt, unexpected) {
			t.Fatalf("system prompt leaked concrete route %q", unexpected)
		}
	}
}

func TestDiscoveredProjectAPIRouteDispatchesAndFiltersPublicEnvelope(t *testing.T) {
	projectUUID := mustAgentUUID(t)
	wantedUUID := mustAgentUUID(t)
	var dispatched ProjectAPIDispatchRequest
	service := (&Service{}).WithProjectAPIGateway([]ProjectAPIRouteSpec{
		{Method: http.MethodGet, Path: "/api/v1/projects/:project_uuid/llm-logs/:log_uuid"},
	}, func(_ context.Context, request ProjectAPIDispatchRequest) (ProjectAPIDispatchResponse, error) {
		dispatched = request
		body, _ := json.Marshal(map[string]any{"success": true, "data": map[string]any{"uuid": wantedUUID, "summary": "ok", "ignored": true}})
		return ProjectAPIDispatchResponse{Status: http.StatusOK, Body: body}, nil
	})
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	value, err := executeRequestAPITool(context.Background(), service, nil, tc, toolExecutionRecord{}, map[string]any{
		"method":          http.MethodGet,
		"url":             "/api/v1/projects/" + projectUUID + "/llm-logs/" + wantedUUID,
		"query":           map[string]any{"include": []any{"request", "response"}},
		"response_filter": ".data | {uuid,summary}",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, _ := value.(map[string]any)
	if result["uuid"] != wantedUUID || result["summary"] != "ok" || result["ignored"] != nil {
		t.Fatalf("filtered result=%+v", result)
	}
	if dispatched.Method != http.MethodGet || dispatched.Path == "" || len(dispatched.Query) != 1 || dispatched.HasBody {
		t.Fatalf("dispatch request=%+v", dispatched)
	}
}

func TestDiscoveredProjectAPIWriteUsesDynamicConfirmationRoute(t *testing.T) {
	projectUUID := mustAgentUUID(t)
	service := (&Service{}).WithProjectAPIGateway([]ProjectAPIRouteSpec{
		{Method: http.MethodDelete, Path: "/api/v1/projects/:project_uuid/premise-assets/trash"},
	}, nil)
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	request, err := service.parseAgentAPIRequest(tc, map[string]any{
		"method": http.MethodDelete, "url": "/api/v1/projects/" + projectUUID + "/premise-assets/trash",
		"response_filter": ".data | {deleted_count,file_soft_deleted_count,retained_file_count,blocked_items}",
	})
	if err != nil {
		t.Fatal(err)
	}
	binding := dangerousConfirmationBinding{
		Route: request.Route.ID, ProjectUUID: projectUUID, TargetUUID: request.TargetUUID,
		ExpectedRevision: 0, RequestFingerprint: agentRequestFingerprint(request), ConfirmOption: 0,
	}
	if _, err := service.validateDangerousConfirmationBinding(tc, binding, "single_choice", 2); err != nil {
		t.Fatalf("dynamic confirmation route rejected: %v", err)
	}
}
