package agent

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"lumi/internal/llm"
	"lumi/internal/production"
	"lumi/internal/story"
)

func TestReviewedAgentAPIRoutesIncludeDeletionOverlays(t *testing.T) {
	routes := agentAPIRoutes()
	if len(routes) != 83 {
		t.Fatalf("reviewed routes=%d want=83", len(routes))
	}
	wanted := map[string]struct {
		method, path, projector, revisionSource string
	}{
		RouteChapterPermanentDelete:      {"DELETE", "/api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/permanent", "null_data", agentAPIRevisionQuery},
		RouteChapterTrashEmpty:           {"DELETE", "/api/v1/projects/{project_uuid}/chapters/trash", "chapter_trash_result", agentAPIRevisionNone},
		RoutePremiseAssetPermanentDelete: {"DELETE", "/api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/permanent", "premise_trash_result", agentAPIRevisionQuery},
		RoutePremiseAssetTrashEmpty:      {"DELETE", "/api/v1/projects/{project_uuid}/premise-assets/trash", "premise_trash_result", agentAPIRevisionNone},
	}
	for routeID, expectation := range wanted {
		route, ok := agentAPIRouteByIDFromRoutes(routeID, routes)
		if !ok {
			t.Fatalf("missing reviewed route %s", routeID)
		}
		if route.Method != expectation.method || route.PathTemplate != expectation.path || route.Projector != expectation.projector || route.RevisionSource != expectation.revisionSource {
			t.Fatalf("route %s = %+v", routeID, route)
		}
		if route.Handler != routeProjectAPIDispatch || route.Passthrough || !route.StrictSchema || !route.RequiresConfirmation || route.Risk != RiskDangerous {
			t.Fatalf("route %s is not a strict reviewed dispatcher overlay: %+v", routeID, route)
		}
		if route.ExpectedRevision != (expectation.revisionSource != agentAPIRevisionNone) {
			t.Fatalf("route %s revision metadata inconsistent: %+v", routeID, route)
		}
	}
	for _, route := range routes {
		if route.RevisionSource != agentAPIRevisionNone && route.RevisionSource != agentAPIRevisionBody && route.RevisionSource != agentAPIRevisionQuery {
			t.Fatalf("route %s has no explicit revision source: %q", route.ID, route.RevisionSource)
		}
		if route.ExpectedRevision != (route.RevisionSource != agentAPIRevisionNone) {
			t.Fatalf("route %s has inconsistent expected_revision metadata", route.ID)
		}
		if route.RevisionSource == agentAPIRevisionBody && !agentAPISchemaRequires(route.BodySchema, "expected_revision") {
			t.Fatalf("route %s declares body revision without a required body field", route.ID)
		}
		if route.RevisionSource == agentAPIRevisionQuery && !agentAPISchemaRequires(route.QuerySchema, "expected_revision") {
			t.Fatalf("route %s declares query revision without a required query field", route.ID)
		}
	}
	specs := make([]ProjectAPIRouteSpec, 0, len(wanted))
	for _, expectation := range wanted {
		specs = append(specs, ProjectAPIRouteSpec{Method: expectation.method, Path: expectation.path})
	}
	merged := mergeProjectAPIRoutes(specs)
	if len(merged) != len(wanted) {
		t.Fatalf("merged deletion overlays=%d want=%d", len(merged), len(wanted))
	}
	for _, route := range merged {
		if !route.ServerRoute || route.Passthrough || !route.StrictSchema {
			t.Fatalf("server overlay lost reviewed metadata: %+v", route)
		}
	}
}

func TestRuntimeDangerousConfirmationPresentationIsCanonicalAndLocalized(t *testing.T) {
	route, ok := agentAPIRouteByID(RouteChapterPermanentDelete)
	if !ok {
		t.Fatal("missing permanent-delete route")
	}
	zhID, zh := runtimeDangerousConfirmationPresentation(route, "zh-Hans")
	enID, en := runtimeDangerousConfirmationPresentation(route, "en")
	if zhID != runtimeDangerousConfirmationQuestionID || enID != runtimeDangerousConfirmationQuestionID ||
		zh.Header != "操作确认" || !strings.Contains(zh.Question, route.Action) || zh.SafeLabel != "暂不执行 (Recommended)" || zh.ConfirmLabel != "确认执行" ||
		en.Header != "Confirm" || en.SafeLabel != "Do not proceed (Recommended)" || en.ConfirmLabel != "Proceed" {
		t.Fatalf("localized runtime confirmations zh=%+v en=%+v", zh, en)
	}
	setupRoute, ok := agentAPIRouteByID(RouteProjectSetupFinalize)
	if !ok {
		t.Fatal("missing setup-finalization route")
	}
	setupID, setup := runtimeDangerousConfirmationPresentation(setupRoute, "en")
	if setupID != bootstrapYoloConfirmationQuestionID || setup.SafeLabel != "Keep editing (Recommended)" || setup.ConfirmLabel != "Finalize and start generation" {
		t.Fatalf("localized setup confirmation=%+v id=%q", setup, setupID)
	}
}

func TestReviewedQueryRevisionRouteRejectsMissingQuery(t *testing.T) {
	projectUUID := mustAgentUUID(t)
	chapterUUID := mustAgentUUID(t)
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	base := map[string]any{
		"method":          "DELETE",
		"url":             "/api/v1/projects/" + projectUUID + "/chapters/" + chapterUUID + "/permanent",
		"response_filter": ".data",
	}
	if _, err := parseAgentAPIRequest(tc, base); err == nil || errorCode(err) != CodeToolValidation {
		t.Fatalf("missing required query accepted: %v", err)
	}
	withQuery := cloneToolArguments(base)
	withQuery["query"] = map[string]any{"expected_revision": float64(7)}
	request, err := parseAgentAPIRequest(tc, withQuery)
	if err != nil {
		t.Fatal(err)
	}
	if request.Route.ID != RouteChapterPermanentDelete || request.UseDispatcher || agentAPIRequestExpectedRevision(request) != 7 {
		t.Fatalf("parsed request=%+v", request)
	}
}

func TestChapterListReviewedQuerySupportsState(t *testing.T) {
	projectUUID := mustAgentUUID(t)
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	base := map[string]any{
		"method": "GET", "url": "/api/v1/projects/" + projectUUID + "/chapters",
		"response_filter": ".data.items[] | {uuid,chapter_code,title,revision}",
	}
	request, err := parseAgentAPIRequest(tc, base)
	if err != nil || request.Route.ID != RouteChapterList || !request.Route.StrictSchema {
		t.Fatalf("default chapter list request=%+v err=%v", request, err)
	}
	withState := cloneToolArguments(base)
	withState["query"] = map[string]any{"state": "trashed"}
	request, err = parseAgentAPIRequest(tc, withState)
	if err != nil || stringArg(request.Query, "state") != "trashed" {
		t.Fatalf("trashed chapter list request=%+v err=%v", request, err)
	}
	invalid := cloneToolArguments(base)
	invalid["query"] = map[string]any{"state": "all"}
	if _, err := parseAgentAPIRequest(tc, invalid); err == nil || errorCode(err) != CodeToolValidation {
		t.Fatalf("invalid chapter state accepted: %v", err)
	}
}

func TestPremiseAssetListReviewedQuerySupportsTagAndState(t *testing.T) {
	projectUUID := mustAgentUUID(t)
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	base := map[string]any{
		"method": "GET", "url": "/api/v1/projects/" + projectUUID + "/premise-assets",
		"response_filter": ".data.items[] | {uuid,asset_type,title,summary,tags,revision}",
	}
	args := cloneToolArguments(base)
	args["query"] = map[string]any{"tag": "主角", "state": "trashed"}
	request, err := parseAgentAPIRequest(tc, args)
	if err != nil || request.Route.ID != RoutePremiseAssetList || !request.Route.StrictSchema || stringArg(request.Query, "tag") != "主角" || stringArg(request.Query, "state") != "trashed" {
		t.Fatalf("premise asset list request=%+v err=%v", request, err)
	}
	invalid := cloneToolArguments(base)
	invalid["query"] = map[string]any{"state": "all"}
	if _, err := parseAgentAPIRequest(tc, invalid); err == nil || errorCode(err) != CodeToolValidation {
		t.Fatalf("invalid premise asset state accepted: %v", err)
	}
}

func TestReviewedListHandlersHonorStateAndTagQuery(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	chapter, err := story.NewService(harness.store).CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "回收站章节"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := story.NewService(harness.store).TrashChapter(ctx, chapter.UUID, chapter.Revision); err != nil {
		t.Fatal(err)
	}
	asset, _ := createAssetReferenceMigrationFixture(t, harness)
	if _, err := production.NewService(harness.store, nil).SetPremiseAssetTrashed(ctx, asset.UUID, true, asset.Revision); err != nil {
		t.Fatal(err)
	}
	tc := toolContext{ProjectUUID: harness.project.UUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	cases := []struct {
		args     map[string]any
		wantUUID string
	}{
		{map[string]any{"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters", "query": map[string]any{"state": "trashed"}, "response_filter": ".data.items[] | {uuid,chapter_code,title,revision}"}, chapter.UUID},
		{map[string]any{"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/premise-assets", "query": map[string]any{"tag": "courier", "state": "trashed"}, "response_filter": ".data.items[] | {uuid,asset_type,title,tags,revision}"}, asset.UUID},
	}
	for _, testCase := range cases {
		request, err := parseAgentAPIRequest(tc, testCase.args)
		if err != nil {
			t.Fatal(err)
		}
		value, err := executeAgentAPIRoute(ctx, harness.service, harness.store, tc, toolExecutionRecord{}, request)
		if err != nil {
			t.Fatal(err)
		}
		projected, err := compactAgentRouteValue(request.Route, value)
		if err != nil {
			t.Fatal(err)
		}
		root, _ := projected.(map[string]any)
		items, _ := root["items"].([]any)
		if len(items) != 1 || stringArg(items[0].(map[string]any), "uuid") != testCase.wantUUID {
			t.Fatalf("route %s projected=%+v", request.Route.ID, projected)
		}
	}
}

func TestOnlyNullDataProjectorRecommendsWholeData(t *testing.T) {
	for _, route := range agentAPIRoutes() {
		filter := recommendedAgentAPIResponseFilter(route)
		if filter == ".data" && route.ID != RouteChapterPermanentDelete {
			t.Fatalf("non-null route %s recommends whole data", route.ID)
		}
	}
	route, _ := agentAPIRouteByID(RouteChapterPermanentDelete)
	value, err := compactAgentRouteValue(route, nil)
	if err != nil || value != nil {
		t.Fatalf("null projector value=%v err=%v", value, err)
	}
	if _, err := compactAgentRouteValue(route, map[string]any{}); err == nil {
		t.Fatal("null projector accepted object data")
	}
	contract := recoveryOutputContract(route)
	if contract.DataShape != "null" || contract.RecommendedResponseFilter != ".data" {
		t.Fatalf("null recovery contract=%+v", contract)
	}
}

func TestReviewedResponseProjectorsRejectWrongShapes(t *testing.T) {
	objectRoute, ok := agentAPIRouteByID(RouteProjectGet)
	if !ok {
		t.Fatal("project route missing")
	}
	listRoute, ok := agentAPIRouteByID(RouteChapterList)
	if !ok {
		t.Fatal("chapter list route missing")
	}
	tests := []struct {
		name  string
		route agentAPIRoute
		value any
	}{
		{name: "object null", route: objectRoute, value: nil},
		{name: "object scalar", route: objectRoute, value: "wrong"},
		{name: "list null", route: listRoute, value: nil},
		{name: "list scalar", route: listRoute, value: "wrong"},
		{name: "list missing items", route: listRoute, value: map[string]any{}},
		{name: "list null items", route: listRoute, value: map[string]any{"items": nil}},
		{name: "list object items", route: listRoute, value: map[string]any{"items": map[string]any{}}},
		{name: "list scalar item", route: listRoute, value: map[string]any{"items": []any{"wrong"}}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := compactAgentRouteValue(testCase.route, testCase.value); err == nil || errorCode(err) != CodeStateConflict {
				t.Fatalf("wrong response shape accepted: value=%#v err=%v", testCase.value, err)
			}
		})
	}
}

func TestReviewedDeletionOverlaysExecuteThroughExistingDispatcher(t *testing.T) {
	projectUUID := mustAgentUUID(t)
	chapterUUID := mustAgentUUID(t)
	assetUUID := mustAgentUUID(t)
	executionUUID := mustAgentUUID(t)
	specs := []ProjectAPIRouteSpec{
		{Method: "DELETE", Path: "/api/v1/projects/:project_uuid/chapters/:chapter_uuid/permanent"},
		{Method: "DELETE", Path: "/api/v1/projects/:project_uuid/chapters/trash"},
		{Method: "DELETE", Path: "/api/v1/projects/:project_uuid/premise-assets/:premise_asset_uuid/permanent"},
		{Method: "DELETE", Path: "/api/v1/projects/:project_uuid/premise-assets/trash"},
	}
	service := (&Service{}).WithProjectAPIGateway(specs, func(_ context.Context, request ProjectAPIDispatchRequest) (ProjectAPIDispatchResponse, error) {
		if request.ToolExecutionUUID != executionUUID || request.RouteID == "" {
			t.Fatalf("dispatcher checkpoint metadata execution=%q route=%q", request.ToolExecutionUUID, request.RouteID)
		}
		var data any
		switch {
		case strings.Contains(request.Path, "/chapters/") && strings.HasSuffix(request.Path, "/permanent"):
			if intArg(request.Query, "expected_revision") != 3 {
				t.Fatalf("chapter permanent query=%+v", request.Query)
			}
			data = nil
		case strings.HasSuffix(request.Path, "/chapters/trash"):
			data = map[string]any{"deleted_count": 2, "blocked_items": []any{map[string]any{"uuid": chapterUUID, "chapter_code": "vol01.ch01", "error_code": "chapter_delete_blocked"}}}
		case strings.Contains(request.Path, "/premise-assets/") && strings.HasSuffix(request.Path, "/permanent"):
			if intArg(request.Query, "expected_revision") != 4 {
				t.Fatalf("premise asset permanent query=%+v", request.Query)
			}
			data = map[string]any{"deleted_count": 1, "file_soft_deleted_count": 1, "retained_file_count": 0, "blocked_items": []any{}}
		default:
			data = map[string]any{"deleted_count": 1, "file_soft_deleted_count": 1, "retained_file_count": 0, "blocked_items": []any{}}
		}
		body, _ := json.Marshal(map[string]any{"success": true, "data": data})
		return ProjectAPIDispatchResponse{Status: 200, Body: body}, nil
	})
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	cases := []map[string]any{
		{"method": "DELETE", "url": "/api/v1/projects/" + projectUUID + "/chapters/" + chapterUUID + "/permanent", "query": map[string]any{"expected_revision": float64(3)}, "response_filter": ".data"},
		{"method": "DELETE", "url": "/api/v1/projects/" + projectUUID + "/chapters/trash", "response_filter": ".data | {deleted_count,blocked_items}"},
		{"method": "DELETE", "url": "/api/v1/projects/" + projectUUID + "/premise-assets/" + assetUUID + "/permanent", "query": map[string]any{"expected_revision": float64(4)}, "response_filter": ".data | {deleted_count,file_soft_deleted_count,retained_file_count,blocked_items}"},
		{"method": "DELETE", "url": "/api/v1/projects/" + projectUUID + "/premise-assets/trash", "response_filter": ".data | {deleted_count,file_soft_deleted_count,retained_file_count,blocked_items}"},
	}
	for _, args := range cases {
		request, err := service.parseAgentAPIRequest(tc, args)
		if err != nil {
			t.Fatal(err)
		}
		if request.UseDispatcher || request.Route.Handler != routeProjectAPIDispatch {
			t.Fatalf("reviewed overlay degraded to passthrough: %+v", request)
		}
		value, err := executeAgentAPIRoute(context.Background(), service, nil, tc, toolExecutionRecord{UUID: executionUUID}, request)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := compactAgentRouteValue(request.Route, value); err != nil {
			t.Fatalf("compact %s: %v", request.Route.ID, err)
		}
	}
}

func TestQueryRevisionDangerousConfirmationAutoReplayEndToEnd(t *testing.T) {
	harness := newAgentHarness(t)
	harness.service.turnBudget.MaxModelRequests = 4
	ctx := context.Background()
	chapterUUID := mustAgentUUID(t)
	var dispatcherCalls int
	gatewayRoutes := []ProjectAPIRouteSpec{{
		Method: "DELETE", Path: "/api/v1/projects/:project_uuid/chapters/:chapter_uuid/permanent",
	}}
	dispatcher := func(_ context.Context, request ProjectAPIDispatchRequest) (ProjectAPIDispatchResponse, error) {
		dispatcherCalls++
		if request.Method != "DELETE" || request.RouteID != RouteChapterPermanentDelete || request.HasBody || len(request.Body) != 0 || intArg(request.Query, "expected_revision") != 7 || !isUUIDv7(request.ToolExecutionUUID) {
			t.Fatalf("replayed dispatcher request=%+v", request)
		}
		body, _ := json.Marshal(map[string]any{"success": true, "data": nil})
		return ProjectAPIDispatchResponse{Status: 200, Body: body}, nil
	}
	harness.service.WithProjectAPIGateway(gatewayRoutes, dispatcher)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "这个章节需要永久删除吗？"})
	if err != nil {
		t.Fatal(err)
	}
	requestArguments := map[string]any{
		"method":          "DELETE",
		"url":             "/api/v1/projects/" + harness.project.UUID + "/chapters/" + chapterUUID + "/permanent",
		"query":           map[string]any{"expected_revision": float64(7)},
		"response_filter": ".data",
	}
	request, err := harness.service.parseAgentAPIRequest(toolContext{ProjectUUID: harness.project.UUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{Scope: ThreadScopeProject}}, requestArguments)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := agentRequestFingerprint(request)
	requestJSON, _ := json.Marshal(requestArguments)
	var replayProviderID, replayPublicCallUUID string
	harness.model.respond = func(call int, modelRequest llm.ChatRequest) (llm.ChatResponse, error) {
		switch call {
		case 1:
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "chapter-permanent-unconfirmed", Name: "request_api", Arguments: string(requestJSON)}}}, FinishReason: "tool_calls"}, nil
		case 2:
			if !messagesContain(modelRequest.Messages, `"success":true`) || !messagesContain(modelRequest.Messages, `"data":null`) {
				t.Fatalf("null auto-replay result missing from resume context: %+v", modelRequest.Messages)
			}
			if !validConfirmationReplayProviderCallID(replayProviderID) || !isUUIDv7(replayPublicCallUUID) || replayProviderID == replayPublicCallUUID {
				t.Fatalf("invalid replay identities provider=%q public=%q", replayProviderID, replayPublicCallUUID)
			}
			callIndex, resultIndex, callCount, resultCount := -1, -1, 0, 0
			for messageIndex, message := range modelRequest.Messages {
				for _, call := range message.ToolCalls {
					if call.ID == replayPublicCallUUID {
						t.Fatalf("historical public UUID leaked onto Provider assistant call: %+v", modelRequest.Messages)
					}
					if call.ID == replayProviderID {
						callIndex, callCount = messageIndex, callCount+1
						if call.Name != "request_api" || !call.SyntheticID {
							t.Fatalf("repaired assistant call=%+v", call)
						}
					}
				}
				if message.ToolCallID == replayPublicCallUUID {
					t.Fatalf("historical public UUID leaked onto Provider tool result: %+v", modelRequest.Messages)
				}
				if message.ToolCallID == replayProviderID {
					resultIndex, resultCount = messageIndex, resultCount+1
					if message.Role != "tool" || !message.ToolCallIDSynthetic {
						t.Fatalf("repaired tool result=%+v", message)
					}
				}
			}
			if callCount != 1 || resultCount != 1 || callIndex < 0 || resultIndex != callIndex+1 {
				t.Fatalf("repaired Provider pair call_count=%d result_count=%d call_index=%d result_index=%d messages=%+v", callCount, resultCount, callIndex, resultIndex, modelRequest.Messages)
			}
			return finalResponse("章节已永久删除。"), nil
		default:
			t.Fatalf("unexpected model call %d", call)
			return llm.ChatResponse{}, nil
		}
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); !errors.Is(err, ErrWaitingInput) {
		t.Fatalf("query revision flow did not pause: %v", err)
	}
	if dispatcherCalls != 0 {
		t.Fatalf("dangerous query route executed before confirmation: %d", dispatcherCalls)
	}
	requests, err := harness.service.ListUserInputRequests(ctx, harness.project.UUID, thread.UUID)
	if err != nil || len(requests) != 1 || len(requests[0].Questions) != 1 || requests[0].Questions[0].ID != runtimeDangerousConfirmationQuestionID || len(requests[0].Questions[0].Options) != 2 || requests[0].Questions[0].Options[0].Label != "暂不执行 (Recommended)" || requests[0].Questions[0].Options[1].Label != "确认执行" {
		t.Fatalf("confirmation requests=%+v err=%v", requests, err)
	}
	harness.model.mu.Lock()
	modelCallsBeforeConfirmation := harness.model.calls
	harness.model.mu.Unlock()
	if modelCallsBeforeConfirmation != 1 {
		t.Fatalf("runtime confirmation unexpectedly required another model call: calls=%d", modelCallsBeforeConfirmation)
	}
	var runtimeConfirmation struct {
		UUID          string `gorm:"column:uuid"`
		ArgumentsJSON string `gorm:"column:arguments_json"`
		ToolCallUUID  string `gorm:"column:tool_call_uuid"`
	}
	if err := harness.store.DB().Table("agent_tool_executions").Select("uuid,arguments_json,tool_call_uuid").Where("tool_name='request_user_input' AND json_extract(arguments_json,'$.__runtime_generated_confirmation')=1").Take(&runtimeConfirmation).Error; err != nil {
		t.Fatal(err)
	}
	var runtimeArguments map[string]any
	if json.Unmarshal([]byte(runtimeConfirmation.ArgumentsJSON), &runtimeArguments) != nil {
		t.Fatalf("invalid runtime confirmation arguments=%s", runtimeConfirmation.ArgumentsJSON)
	}
	runtimeBinding, _ := runtimeArguments["confirmation"].(map[string]any)
	if stringArg(runtimeBinding, "route") != RouteChapterPermanentDelete || stringArg(runtimeBinding, "request_fingerprint") != fingerprint || intArg(runtimeBinding, "expected_revision") != 7 || stringArg(runtimeBinding, "question_id") != runtimeDangerousConfirmationQuestionID || intArg(runtimeBinding, "confirm_option") != 1 || !isUUIDv7(runtimeConfirmation.UUID) || !isUUIDv7(runtimeConfirmation.ToolCallUUID) {
		t.Fatalf("runtime confirmation binding=%+v execution=%+v", runtimeBinding, runtimeConfirmation)
	}
	var publicConfirmationCall string
	if err := harness.store.DB().Table("chat_items").Select("content").Where("remote_item_uuid=? AND item_type='tool_call'", runtimeConfirmation.ToolCallUUID).Scan(&publicConfirmationCall).Error; err != nil {
		t.Fatal(err)
	}
	var publicConfirmationArguments map[string]any
	if json.Unmarshal([]byte(publicConfirmationCall), &publicConfirmationArguments) != nil || publicConfirmationArguments["confirmation"] != nil || publicConfirmationArguments["questions"] == nil {
		t.Fatalf("runtime-only confirmation leaked into public/model tool arguments: %s", publicConfirmationCall)
	}
	var requestItemContentBefore string
	if err := harness.store.DB().Table("chat_items").Select("content").Where("uuid=?", requests[0].ItemUUID).Scan(&requestItemContentBefore).Error; err != nil {
		t.Fatal(err)
	}
	confirmationResponse := UserInputResponse{Answers: map[string]UserInputAnswer{
		runtimeDangerousConfirmationQuestionID: {SelectedOptionUUID: requests[0].Questions[0].Options[1].UUID},
	}}
	confirmed, err := harness.service.RespondUserInput(ctx, harness.project.UUID, thread.UUID, requests[0].UUID, confirmationResponse)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != "resuming" && confirmed.Status != "answered" {
		t.Fatalf("confirmed request status=%s", confirmed.Status)
	}
	var requestItemContentAfter string
	if err := harness.store.DB().Table("chat_items").Select("content").Where("uuid=?", requests[0].ItemUUID).Scan(&requestItemContentAfter).Error; err != nil {
		t.Fatal(err)
	}
	if requestItemContentAfter != requestItemContentBefore {
		t.Fatalf("confirmation response rewrote the v4 user-input card: before=%s after=%s", requestItemContentBefore, requestItemContentAfter)
	}
	if repeated, err := harness.service.RespondUserInput(ctx, harness.project.UUID, thread.UUID, requests[0].UUID, confirmationResponse); err != nil || repeated.Status != "resuming" {
		t.Fatalf("immediate duplicate confirmation response=%+v err=%v", repeated, err)
	}
	if _, err := harness.service.RespondUserInput(ctx, harness.project.UUID, thread.UUID, requests[0].UUID, UserInputResponse{Answers: map[string]UserInputAnswer{
		runtimeDangerousConfirmationQuestionID: {SelectedOptionUUID: requests[0].Questions[0].Options[0].UUID},
	}}); err == nil || errorCode(err) != CodeStateConflict {
		t.Fatalf("different answer during resuming was accepted: %v", err)
	}
	var replay struct {
		ArgumentsJSON string `gorm:"column:arguments_json"`
		ToolCallUUID  string `gorm:"column:tool_call_uuid"`
	}
	if err := harness.store.DB().Table("agent_tool_executions").Select("arguments_json,tool_call_uuid").Where("json_extract(arguments_json,'$.__confirmation_auto_replay')=1").Take(&replay).Error; err != nil {
		t.Fatal(err)
	}
	var replayArguments map[string]any
	if json.Unmarshal([]byte(replay.ArgumentsJSON), &replayArguments) != nil {
		t.Fatalf("invalid replay arguments=%s", replay.ArgumentsJSON)
	}
	replayQuery, _ := replayArguments["query"].(map[string]any)
	wantProviderID := confirmationReplayProviderCallID(requests[0].UUID)
	if intArg(replayQuery, "expected_revision") != 7 || replayArguments["request_body"] != nil || stringArg(replayArguments, "__provider_call_id") != wantProviderID || replay.ToolCallUUID == wantProviderID || !isUUIDv7(replay.ToolCallUUID) {
		t.Fatalf("query replay arguments=%+v public_call_uuid=%q", replayArguments, replay.ToolCallUUID)
	}

	// Simulate a replay intent persisted by the pre-upgrade runtime, which used
	// the public UUID as the Provider call ID in both durable copies. Rebuilding
	// the service then exercises the real pending-tool recovery boundary.
	replayProviderID, replayPublicCallUUID = wantProviderID, replay.ToolCallUUID
	replayArguments["__provider_call_id"] = replayPublicCallUUID
	legacyArguments, err := json.Marshal(replayArguments)
	if err != nil {
		t.Fatal(err)
	}
	updated := harness.store.DB().Table("agent_tool_executions").Where("json_extract(arguments_json,'$.__confirmation_auto_replay')=1 AND state='intent'").Update("arguments_json", string(legacyArguments))
	if updated.Error != nil || updated.RowsAffected != 1 {
		t.Fatalf("seed historical replay arguments rows=%d err=%v", updated.RowsAffected, updated.Error)
	}
	updated = harness.store.DB().Exec(`UPDATE chat_items SET metadata_json=json_set(metadata_json,'$.provider_call_id',?) WHERE item_type='tool_call' AND remote_item_uuid=?`, replayPublicCallUUID, replayPublicCallUUID)
	if updated.Error != nil || updated.RowsAffected != 1 {
		t.Fatalf("seed historical replay metadata rows=%d err=%v", updated.RowsAffected, updated.Error)
	}
	restarted := NewService(harness.projects, harness.providers, harness.model, harness.queue, nil).WithProjectAPIGateway(gatewayRoutes, dispatcher)
	restarted.turnBudget.MaxModelRequests = 4
	harness.service = restarted
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatResume); err != nil {
		t.Fatal(err)
	}
	if dispatcherCalls != 1 {
		t.Fatalf("confirmed dispatcher calls=%d", dispatcherCalls)
	}
	var repairedReplay struct {
		ArgumentsJSON string `gorm:"column:arguments_json"`
		ToolCallUUID  string `gorm:"column:tool_call_uuid"`
		State         string `gorm:"column:state"`
	}
	if err := harness.store.DB().Table("agent_tool_executions").Select("arguments_json,tool_call_uuid,state").Where("json_extract(arguments_json,'$.__confirmation_auto_replay')=1").Take(&repairedReplay).Error; err != nil {
		t.Fatal(err)
	}
	if repairedReplay.State != "completed" || repairedReplay.ToolCallUUID != replayPublicCallUUID || metadataString(repairedReplay.ArgumentsJSON, "__provider_call_id") != replayProviderID {
		t.Fatalf("persisted repaired replay=%+v", repairedReplay)
	}
	var internalPair []struct {
		ItemType     string `gorm:"column:item_type"`
		MetadataJSON string `gorm:"column:metadata_json"`
	}
	if err := harness.store.DB().Table("chat_items").Select("item_type,metadata_json").Where("remote_item_uuid=? AND item_type IN ('tool_call','tool_result')", replayPublicCallUUID).Order("sequence,id").Scan(&internalPair).Error; err != nil {
		t.Fatal(err)
	}
	if len(internalPair) != 2 || internalPair[0].ItemType != "tool_call" || internalPair[1].ItemType != "tool_result" || metadataString(internalPair[0].MetadataJSON, "provider_call_id") != replayProviderID || metadataString(internalPair[1].MetadataJSON, "provider_call_id") != replayProviderID {
		t.Fatalf("persisted repaired call/result metadata=%+v", internalPair)
	}
	publicItems, err := harness.service.ListItems(ctx, harness.project.UUID, thread.UUID, "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	publicPairCount := 0
	for _, item := range publicItems.Items {
		if item.ToolCallUUID != replayPublicCallUUID {
			continue
		}
		publicPairCount++
		if !isUUIDv7(item.ToolCallUUID) || item.ToolCallUUID == replayProviderID || strings.Contains(string(item.Metadata), replayProviderID) || strings.Contains(string(item.Metadata), "provider_call_id") {
			t.Fatalf("public replay item leaked Provider identity: %+v", item)
		}
	}
	if publicPairCount != 2 {
		t.Fatalf("public replay call/result pair count=%d items=%+v", publicPairCount, publicItems.Items)
	}
	if repeated, err := harness.service.RespondUserInput(ctx, harness.project.UUID, thread.UUID, requests[0].UUID, confirmationResponse); err != nil || repeated.Status != "resumed" {
		t.Fatalf("repeated confirmation response=%+v err=%v", repeated, err)
	}
	var replayCount int64
	if err := harness.store.DB().Table("agent_tool_executions").Where("json_extract(arguments_json,'$.__confirmation_auto_replay')=1").Count(&replayCount).Error; err != nil || replayCount != 1 {
		t.Fatalf("auto replay count=%d err=%v", replayCount, err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	if allowed, err := hasMatchingDangerousConfirmation(ctx, harness.store, tc, request); err != nil || allowed {
		t.Fatalf("query confirmation was not consumed: allowed=%v err=%v", allowed, err)
	}
}

func validConfirmationReplayProviderCallID(value string) bool {
	if len(value) != len("call_")+24 || !strings.HasPrefix(value, "call_") {
		return false
	}
	for _, character := range value[len("call_"):] {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}

func TestPhase3ProjectorsHaveConcreteFieldMetadata(t *testing.T) {
	for _, projector := range phase3AgentAPIProjectors() {
		for _, field := range projector.Fields {
			if field.Type == "" || field.Type == "public" || strings.TrimSpace(field.Description) == "" || field.Description == "经审查的公开字段。" {
				t.Fatalf("projector %s field metadata=%+v", projector.Key, field)
			}
		}
	}
	if crop := phase3AgentAPIResponseField("crop"); crop.Type != "JSON value | null" {
		t.Fatalf("crop projector metadata=%+v", crop)
	}
}

func TestGenerationRoutesUseNarrowRequestSchemas(t *testing.T) {
	want := map[string][]string{
		RoutePremiseSettingGenerationCreate:  {"model", "prompt"},
		RoutePremiseBreakdownCreate:          {"model", "prompt"},
		RouteComicImageGenerationCreate:      {"model", "premise_asset_uuids", "prompt"},
		RouteStoryProfileGenerationCreate:    {"chapter_count", "model", "prompt"},
		RouteChapterBatchPlanCreate:          {"chapter_count", "model", "prompt"},
		RouteComicStoryboardGenerationCreate: {"max_section_count", "model", "prompt"},
	}
	for routeID, wantedFields := range want {
		route, ok := agentAPIRouteByID(routeID)
		if !ok {
			t.Fatalf("missing route %s", routeID)
		}
		properties, _ := route.BodySchema["properties"].(map[string]any)
		actual := make([]string, 0, len(properties))
		for field := range properties {
			actual = append(actual, field)
		}
		slices.Sort(actual)
		if !slices.Equal(actual, wantedFields) {
			t.Fatalf("route %s fields=%v want=%v", routeID, actual, wantedFields)
		}
	}
}

func TestConfirmationReplayProviderCallIDIsStableAndContextCanonical(t *testing.T) {
	confirmationUUID := "01a0514d-1999-7071-a284-48a26855f41d"
	want := "call_3a48bfcef1afc30785f1efb5"
	if got := confirmationReplayProviderCallID(confirmationUUID); got != want || len(got) != 29 {
		t.Fatalf("provider call id=%q want=%q", got, want)
	}
	publicCallUUID := "01990000-0000-7000-8000-000000000111"
	oldProviderID := "01990000-0000-7000-8000-000000000222"
	replayMetadata, _ := json.Marshal(map[string]any{
		"provider_call_id": oldProviderID, "runtime_generated": true,
		"confirmation_request_uuid": confirmationUUID,
	})
	resultMetadata, _ := json.Marshal(map[string]any{"provider_call_id": oldProviderID})
	normalCallUUID := "01990000-0000-7000-8000-000000000333"
	normalMetadata, _ := json.Marshal(map[string]any{"provider_call_id": "provider-real-call-id"})
	items := []contextItem{
		{itemRecord: itemRecord{ItemType: "tool_call", ToolName: "request_api", RemoteItemUUID: publicCallUUID, Content: `{}`, MetadataJSON: string(replayMetadata)}},
		{itemRecord: itemRecord{ItemType: "tool_result", ToolName: "request_api", RemoteItemUUID: publicCallUUID, Content: `{"success":true,"data":null}`, MetadataJSON: string(resultMetadata)}},
		{itemRecord: itemRecord{ItemType: "tool_call", ToolName: "request_api", RemoteItemUUID: normalCallUUID, Content: `{}`, MetadataJSON: string(normalMetadata)}},
		{itemRecord: itemRecord{ItemType: "tool_result", ToolName: "request_api", RemoteItemUUID: normalCallUUID, Content: `{"success":true,"data":{}}`, MetadataJSON: string(normalMetadata)}},
	}
	messages := contextMessages(items, "", int64(0), contextPromptSet{})
	var replayCallID, replayResultID, normalProviderCallID, normalResultID string
	var replayCallSynthetic, replayResultSynthetic, normalCallSynthetic, normalResultSynthetic bool
	for _, message := range messages {
		if len(message.ToolCalls) == 1 {
			if replayCallID == "" {
				replayCallID = message.ToolCalls[0].ID
				replayCallSynthetic = message.ToolCalls[0].SyntheticID
			} else {
				normalProviderCallID = message.ToolCalls[0].ID
				normalCallSynthetic = message.ToolCalls[0].SyntheticID
			}
		}
		if message.Role == "tool" {
			if replayResultID == "" {
				replayResultID = message.ToolCallID
				replayResultSynthetic = message.ToolCallIDSynthetic
			} else {
				normalResultID = message.ToolCallID
				normalResultSynthetic = message.ToolCallIDSynthetic
			}
		}
	}
	if replayCallID != want || replayResultID != want || !replayCallSynthetic || !replayResultSynthetic {
		t.Fatalf("historical replay pairing call=%q result=%q want=%q", replayCallID, replayResultID, want)
	}
	if normalProviderCallID != "provider-real-call-id" || normalResultID != "provider-real-call-id" || normalCallSynthetic || normalResultSynthetic {
		t.Fatalf("normal Provider IDs were rewritten: call=%q result=%q", normalProviderCallID, normalResultID)
	}
}

func TestSyntheticAndRealProviderCallIDsStayOutOfPublicItemMetadata(t *testing.T) {
	raw := `{"provider_call_id":"call_3a48bfcef1afc30785f1efb5","runtime_generated":true,"confirmation_request_uuid":"01a0514d-1999-7071-a284-48a26855f41d"}`
	item := itemDTO(itemRecord{MetadataJSON: raw}, "thread", "turn", "run")
	if strings.Contains(string(item.Metadata), "provider_call_id") || strings.Contains(string(item.Metadata), "call_3a48bfcef1afc30785f1efb5") {
		t.Fatalf("public item metadata leaked Provider pairing ID: %s", item.Metadata)
	}
	trajectory := trajectoryPublicMetadata(raw)
	if strings.Contains(string(trajectory), "provider_call_id") || strings.Contains(string(trajectory), "call_3a48bfcef1afc30785f1efb5") {
		t.Fatalf("public trajectory metadata leaked Provider pairing ID: %s", trajectory)
	}
	if !strings.Contains(string(item.Metadata), "confirmation_request_uuid") || !strings.Contains(string(trajectory), "confirmation_request_uuid") {
		t.Fatalf("public confirmation UUID metadata was removed with the internal Provider ID: item=%s trajectory=%s", item.Metadata, trajectory)
	}
}

func TestPendingConfirmationReplayLazilyRepairsProviderCallID(t *testing.T) {
	for _, state := range []string{"intent", "executing"} {
		t.Run(state, func(t *testing.T) {
			harness := newAgentHarness(t)
			ctx := context.Background()
			thread := harness.createThread(t)
			turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "确认后恢复"})
			if err != nil {
				t.Fatal(err)
			}
			tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
			if err != nil {
				t.Fatal(err)
			}
			confirmationUUID := mustAgentUUID(t)
			publicCallUUID := mustAgentUUID(t)
			oldProviderID := publicCallUUID
			executionUUID := mustAgentUUID(t)
			arguments, _ := json.Marshal(map[string]any{
				"__provider_call_id": oldProviderID, "__confirmation_auto_replay": true,
				"__confirmation_request_uuid": confirmationUUID,
			})
			metadata := map[string]any{
				"provider_call_id": oldProviderID, "runtime_generated": true,
				"confirmation_request_uuid": confirmationUUID,
			}
			sqlDB, err := harness.store.DB().DB()
			if err != nil {
				t.Fatal(err)
			}
			tx, err := sqlDB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			threadRow, err := lockThreadSQL(ctx, tx, tc.Thread.ProjectID, tc.Thread.UUID)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			item, err := appendItemTx(ctx, tx, &threadRow, &tc.Turn.ID, &tc.Run.ID, "tool_call", "assistant", `{}`, "json", "in_progress", publicCallUUID, "request_api", tc.ProjectUUID, metadata, now)
			if err != nil {
				t.Fatal(err)
			}
			result, err := tx.ExecContext(ctx, `INSERT INTO agent_tool_executions(uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,tool_name,target_uuid,arguments_json,idempotency_key,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, executionUUID, tc.Thread.ID, tc.Run.ID, tc.Turn.ID, item.ID, publicCallUUID, "request_api", tc.ProjectUUID, string(arguments), "test-lazy-provider-id:"+confirmationUUID, state, now, now)
			if err != nil {
				t.Fatal(err)
			}
			executionID, _ := result.LastInsertId()
			if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,updated_at=? WHERE id=?`, threadRow.NextItemSequence, now, threadRow.ID); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}

			execution := toolExecutionRecord{ID: executionID, ItemID: item.ID, ToolCallUUID: publicCallUUID, ToolName: "request_api", ArgumentsJSON: string(arguments), State: state}
			repaired, err := harness.service.repairPendingConfirmationReplayProviderID(ctx, harness.store, execution)
			if err != nil {
				t.Fatal(err)
			}
			repaired, err = harness.service.repairPendingConfirmationReplayProviderID(ctx, harness.store, repaired)
			if err != nil {
				t.Fatalf("second lazy repair is not idempotent: %v", err)
			}
			var repairedArguments, repairedMetadata, persistedPublicUUID string
			if err := sqlDB.QueryRowContext(ctx, `SELECT arguments_json,tool_call_uuid FROM agent_tool_executions WHERE id=?`, executionID).Scan(&repairedArguments, &persistedPublicUUID); err != nil {
				t.Fatal(err)
			}
			if err := sqlDB.QueryRowContext(ctx, `SELECT metadata_json FROM chat_items WHERE id=?`, item.ID).Scan(&repairedMetadata); err != nil {
				t.Fatal(err)
			}
			providerCallID := confirmationReplayProviderCallID(confirmationUUID)
			if repaired.ToolCallUUID != publicCallUUID || persistedPublicUUID != publicCallUUID || metadataString(repaired.ArgumentsJSON, "__provider_call_id") != providerCallID || metadataString(repairedArguments, "__provider_call_id") != providerCallID || metadataString(repairedMetadata, "provider_call_id") != providerCallID {
				t.Fatalf("lazy repair returned=%+v public=%q args=%s metadata=%s", repaired, persistedPublicUUID, repairedArguments, repairedMetadata)
			}
		})
	}
}
