package agent

import (
	"context"
	"testing"

	"lumi/internal/llm"
	"lumi/internal/story"
)

func TestToolCallBatchRejectsOneInvalidCallWithoutPersistingOrExecutingValidSiblings(t *testing.T) {
	for _, invalidFirst := range []bool{false, true} {
		name := "invalid_last"
		if invalidFirst {
			name = "invalid_first"
		}
		t.Run(name, func(t *testing.T) {
			harness := newAgentHarness(t)
			before, err := story.NewService(harness.store).GetProject(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			valid := llm.ToolCall{ID: "valid-project-update", Name: "request_api", Arguments: requestAPITestArguments(t, map[string]any{
				"method": "PATCH", "url": "/api/v1/projects/" + harness.project.UUID,
				"request_body":    map[string]any{"name": "must-not-be-written", "description": "atomic batch", "expected_revision": float64(before.Revision)},
				"response_filter": ".data | {uuid,name,revision}",
			})}
			invalid := llm.ToolCall{ID: "invalid-project-update", Name: "request_api", Arguments: requestAPITestArguments(t, map[string]any{
				"method": "PATCH", "url": "/api/v1/projects/" + harness.project.UUID,
				"request_body":    map[string]any{"name": "bad", "description": "bad", "expected_revision": float64(before.Revision), "candidate": true},
				"response_filter": ".data | {uuid,name,revision}",
			})}
			calls := []llm.ToolCall{valid, invalid}
			if invalidFirst {
				calls = []llm.ToolCall{invalid, valid}
			}
			harness.model.responses = []llm.ChatResponse{
				{Message: llm.ChatMessage{Role: "assistant", ToolCalls: calls}, FinishReason: "tool_calls"},
				finalResponse("已安全停止该批次。"),
			}
			thread := harness.createThread(t)
			turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "批量修改"})
			if err != nil {
				t.Fatal(err)
			}
			if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
				t.Fatal(err)
			}

			after, err := story.NewService(harness.store).GetProject(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if after.Name != before.Name || after.Revision != before.Revision {
				t.Fatalf("valid sibling produced a partial side effect: before=%+v after=%+v", before, after)
			}
			var executions, validItems, repairs int64
			if err := harness.store.DB().Table("agent_tool_executions").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Count(&executions).Error; err != nil {
				t.Fatal(err)
			}
			if err := harness.store.DB().Table("chat_items").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='tool_call' AND json_extract(metadata_json,'$.provider_call_id')=?", turn.UUID, valid.ID).Count(&validItems).Error; err != nil {
				t.Fatal(err)
			}
			if err := harness.store.DB().Table("chat_items").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='tool_result' AND json_extract(metadata_json,'$.validation_repair')=1", turn.UUID).Count(&repairs).Error; err != nil {
				t.Fatal(err)
			}
			if executions != 0 || validItems != 0 || repairs != 1 {
				t.Fatalf("executions=%d valid_items=%d repairs=%d", executions, validItems, repairs)
			}
		})
	}
}

func TestReviewedAgentAPICrossFieldPreflight(t *testing.T) {
	projectUUID, chapterUUID, taskUUID := mustAgentUUID(t), mustAgentUUID(t), mustAgentUUID(t)
	fileUUID, uploadUUID := mustAgentUUID(t), mustAgentUUID(t)
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	base := "/api/v1/projects/" + projectUUID
	filter := ".data | {uuid,revision}"
	invalid := []struct {
		name string
		args map[string]any
	}{
		{name: "premise asset source missing", args: map[string]any{
			"method": "POST", "url": base + "/premise-assets",
			"request_body": map[string]any{"asset_type": "scene", "title": "missing source"}, "response_filter": filter,
		}},
		{name: "premise asset sources conflict", args: map[string]any{
			"method": "POST", "url": base + "/premise-assets",
			"request_body": map[string]any{"file_uuid": fileUUID, "upload_uuid": uploadUUID, "asset_type": "scene", "title": "two sources"}, "response_filter": filter,
		}},
		{name: "story task cursors conflict", args: map[string]any{
			"method": "GET", "url": base + "/tasks/" + taskUUID + "/events",
			"query": map[string]any{"before": "0", "after": "0"}, "response_filter": ".data.items[] | {uuid,sequence}",
		}},
		{name: "production task cursor negative", args: map[string]any{
			"method": "GET", "url": base + "/production-tasks/" + taskUUID + "/events",
			"query": map[string]any{"before": "-1"}, "response_filter": ".data.items[] | {uuid,sequence}",
		}},
		{name: "story task cursor empty", args: map[string]any{
			"method": "GET", "url": base + "/tasks/" + taskUUID + "/events",
			"query": map[string]any{"after": ""}, "response_filter": ".data.items[] | {uuid,sequence}",
		}},
		{name: "export readiness chapter missing chapter uuid", args: map[string]any{
			"method": "GET", "url": base + "/comic-exports/readiness",
			"query": map[string]any{"scope": "chapter"}, "response_filter": ".data | {scope,chapter_uuid,can_export}",
		}},
		{name: "export readiness project carries chapter uuid", args: map[string]any{
			"method": "GET", "url": base + "/comic-exports/readiness",
			"query": map[string]any{"scope": "project", "chapter_uuid": chapterUUID}, "response_filter": ".data | {scope,chapter_uuid,can_export}",
		}},
		{name: "export create chapter missing chapter uuid", args: map[string]any{
			"method": "POST", "url": base + "/comic-exports",
			"request_body": map[string]any{"scope": "chapter", "format": "zip"}, "response_filter": ".data | {uuid,kind,status}",
		}},
		{name: "export create project carries chapter uuid", args: map[string]any{
			"method": "POST", "url": base + "/comic-exports",
			"request_body": map[string]any{"scope": "project", "chapter_uuid": chapterUUID, "format": "zip"}, "response_filter": ".data | {uuid,kind,status}",
		}},
		{name: "export list project carries chapter uuid", args: map[string]any{
			"method": "GET", "url": base + "/comic-exports",
			"query": map[string]any{"scope": "project", "chapter_uuid": chapterUUID}, "response_filter": ".data.items[] | {uuid,scope,chapter_uuid}",
		}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseAgentAPIRequest(tc, test.args); errorCode(err) != CodeToolValidation {
				t.Fatalf("error=%v code=%s", err, errorCode(err))
			}
		})
	}

	valid := []map[string]any{
		{"method": "POST", "url": base + "/premise-assets", "request_body": map[string]any{"file_uuid": fileUUID, "asset_type": "scene", "title": "one source"}, "response_filter": filter},
		{"method": "GET", "url": base + "/tasks/" + taskUUID + "/events", "query": map[string]any{"after": "0"}, "response_filter": ".data.items[] | {uuid,sequence}"},
		{"method": "GET", "url": base + "/comic-exports/readiness", "query": map[string]any{"scope": "chapter", "chapter_uuid": chapterUUID}, "response_filter": ".data | {scope,chapter_uuid,can_export}"},
		{"method": "POST", "url": base + "/comic-exports", "request_body": map[string]any{"scope": "project", "format": "pdf"}, "response_filter": ".data | {uuid,kind,status}"},
		{"method": "GET", "url": base + "/comic-exports", "query": map[string]any{"scope": "chapter"}, "response_filter": ".data.items[] | {uuid,scope,chapter_uuid}"},
	}
	for index, args := range valid {
		if _, err := parseAgentAPIRequest(tc, args); err != nil {
			t.Errorf("valid case %d rejected: %v", index, err)
		}
	}
}

func TestReviewedAgentAPIResponseFilterShapePreflight(t *testing.T) {
	projectUUID, chapterUUID := mustAgentUUID(t), mustAgentUUID(t)
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	base := "/api/v1/projects/" + projectUUID
	chapterCreate := func(filter string) map[string]any {
		return map[string]any{"method": "POST", "url": base + "/chapters", "request_body": map[string]any{"chapter_code": "c1", "title": "chapter"}, "response_filter": filter}
	}
	chapterList := func(filter string) map[string]any {
		return map[string]any{"method": "GET", "url": base + "/chapters", "response_filter": filter}
	}
	nullDelete := func(filter string) map[string]any {
		return map[string]any{"method": "DELETE", "url": base + "/chapters/" + chapterUUID + "/permanent", "query": map[string]any{"expected_revision": float64(1)}, "response_filter": filter}
	}
	invalid := []struct {
		name string
		args map[string]any
	}{
		{name: "object broad data", args: chapterCreate(".data")},
		{name: "object items index", args: chapterCreate(".data.items[0]")},
		{name: "object scalar deep path", args: chapterCreate(".data.uuid.foo")},
		{name: "object scalar nested projection", args: chapterCreate(".data | {uuid:{foo}}")},
		{name: "object opaque nested projection too deep", args: chapterCreate(".data | {current_story:{uuid:{foo}}}")},
		{name: "object unknown items projection", args: chapterCreate(".data | {items}")},
		{name: "object unknown projector field", args: chapterCreate(".data | {uuid,private_state}")},
		{name: "list broad data", args: chapterList(".data")},
		{name: "list data dependent index", args: chapterList(".data.items[0] | {uuid}")},
		{name: "list broad items", args: chapterList(".data.items[]")},
		{name: "list scalar deep path", args: chapterList(".data.items[].uuid.foo")},
		{name: "list scalar nested projection", args: chapterList(".data | {items:{uuid:{foo}}}")},
		{name: "list omits items", args: chapterList(".data | {pagination:{total}}")},
		{name: "list unknown item field", args: chapterList(".data.items[] | {uuid,private_state}")},
		{name: "null path", args: nullDelete(".data.uuid")},
		{name: "null projection", args: nullDelete(".data | {uuid}")},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseAgentAPIRequest(tc, test.args); errorCode(err) != CodeToolValidation {
				t.Fatalf("error=%v code=%s", err, errorCode(err))
			}
		})
	}

	valid := []map[string]any{
		chapterCreate(".data | {uuid,title,revision}"),
		chapterCreate(".data | {uuid,current_story:{uuid,content}}"),
		chapterCreate(".data.current_story.uuid"),
		chapterList(".data.items[] | {uuid,title,revision}"),
		chapterList(".data.items[] | {uuid,current_story:{uuid,content}}"),
		chapterList(".data.items[].uuid"),
		chapterList(".data | {items:{uuid,title},pagination:{per_page,current_page,last_page,total}}"),
		nullDelete(".data"),
	}
	for index, args := range valid {
		if _, err := parseAgentAPIRequest(tc, args); err != nil {
			t.Errorf("valid case %d rejected: %v", index, err)
		}
	}
}

func TestToolCallBatchRejectsCrossFieldAndProjectorPreflightWithoutSiblingSideEffects(t *testing.T) {
	for _, invalidKind := range []string{"cross_field", "object_broad", "object_index", "object_deep_path", "object_deep_projection", "list_broad", "list_deep_path", "list_deep_projection"} {
		for _, invalidFirst := range []bool{false, true} {
			name := invalidKind + "_last"
			if invalidFirst {
				name = invalidKind + "_first"
			}
			t.Run(name, func(t *testing.T) {
				harness := newAgentHarness(t)
				before, err := story.NewService(harness.store).GetProject(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				valid := llm.ToolCall{ID: "valid-sibling", Name: "request_api", Arguments: requestAPITestArguments(t, map[string]any{
					"method": "PATCH", "url": "/api/v1/projects/" + harness.project.UUID,
					"request_body":    map[string]any{"name": "must-not-be-written", "description": "preflight batch", "expected_revision": float64(before.Revision)},
					"response_filter": ".data | {uuid,name,revision}",
				})}
				invalidArgs := map[string]any{
					"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/premise-assets",
					"request_body":    map[string]any{"file_uuid": mustAgentUUID(t), "upload_uuid": mustAgentUUID(t), "asset_type": "scene", "title": "invalid"},
					"response_filter": ".data | {uuid,title,revision}",
				}
				switch invalidKind {
				case "object_broad":
					invalidArgs = map[string]any{
						"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters",
						"request_body":    map[string]any{"chapter_code": "must-not-exist", "title": "Must not exist"},
						"response_filter": ".data",
					}
				case "object_index":
					invalidArgs = map[string]any{
						"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters",
						"request_body":    map[string]any{"chapter_code": "must-not-exist", "title": "Must not exist"},
						"response_filter": ".data.items[0]",
					}
				case "object_deep_path":
					invalidArgs = map[string]any{
						"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters",
						"request_body":    map[string]any{"chapter_code": "must-not-exist", "title": "Must not exist"},
						"response_filter": ".data.uuid.foo",
					}
				case "object_deep_projection":
					invalidArgs = map[string]any{
						"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters",
						"request_body":    map[string]any{"chapter_code": "must-not-exist", "title": "Must not exist"},
						"response_filter": ".data | {uuid:{foo}}",
					}
				case "list_deep_path":
					invalidArgs = map[string]any{
						"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters",
						"response_filter": ".data.items[].uuid.foo",
					}
				case "list_broad":
					invalidArgs = map[string]any{
						"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters",
						"response_filter": ".data",
					}
				case "list_deep_projection":
					invalidArgs = map[string]any{
						"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters",
						"response_filter": ".data | {items:{uuid:{foo}}}",
					}
				}
				invalid := llm.ToolCall{ID: "invalid-call", Name: "request_api", Arguments: requestAPITestArguments(t, invalidArgs)}
				calls := []llm.ToolCall{valid, invalid}
				if invalidFirst {
					calls = []llm.ToolCall{invalid, valid}
				}
				harness.model.responses = []llm.ChatResponse{
					{Message: llm.ChatMessage{Role: "assistant", ToolCalls: calls}, FinishReason: "tool_calls"},
					finalResponse("已拒绝整批调用。"),
				}
				thread := harness.createThread(t)
				turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "批次预检"})
				if err != nil {
					t.Fatal(err)
				}
				if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
					t.Fatal(err)
				}

				after, err := story.NewService(harness.store).GetProject(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				chapters, err := story.NewService(harness.store).ListChapters(context.Background(), "active")
				if err != nil {
					t.Fatal(err)
				}
				var executions, validItems, repairs int64
				if err := harness.store.DB().Table("agent_tool_executions").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Count(&executions).Error; err != nil {
					t.Fatal(err)
				}
				if err := harness.store.DB().Table("chat_items").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='tool_call' AND json_extract(metadata_json,'$.provider_call_id')=?", turn.UUID, valid.ID).Count(&validItems).Error; err != nil {
					t.Fatal(err)
				}
				if err := harness.store.DB().Table("chat_items").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='tool_result' AND json_extract(metadata_json,'$.validation_repair')=1", turn.UUID).Count(&repairs).Error; err != nil {
					t.Fatal(err)
				}
				if after.Name != before.Name || after.Revision != before.Revision || len(chapters) != 0 || executions != 0 || validItems != 0 || repairs != 1 {
					t.Fatalf("project=%+v -> %+v chapters=%d executions=%d valid_items=%d repairs=%d", before, after, len(chapters), executions, validItems, repairs)
				}
			})
		}
	}
}

func TestMergedServerReviewedRouteSchemaFailureRejectsWholeBatchBeforeIntentOrDispatch(t *testing.T) {
	for _, invalidKind := range []string{"unknown_field", "missing_required_field"} {
		for _, invalidFirst := range []bool{false, true} {
			name := invalidKind + "_last"
			if invalidFirst {
				name = invalidKind + "_first"
			}
			t.Run(name, func(t *testing.T) {
				harness := newAgentHarness(t)
				dispatcherCalls := 0
				harness.service.WithProjectAPIGateway([]ProjectAPIRouteSpec{
					{Method: "PATCH", Path: "/api/v1/projects/:project_uuid"},
					{Method: "POST", Path: "/api/v1/projects/:project_uuid/chapters"},
				}, func(_ context.Context, _ ProjectAPIDispatchRequest) (ProjectAPIDispatchResponse, error) {
					dispatcherCalls++
					return ProjectAPIDispatchResponse{Status: 200, Body: []byte(`{"success":true,"data":{}}`)}, nil
				})

				var mergedCreate agentAPIRoute
				for _, route := range harness.service.requestAPIRoutes() {
					if route.ID == RouteChapterCreate {
						mergedCreate = route
						break
					}
				}
				if mergedCreate.ID == "" || !mergedCreate.ServerRoute || mergedCreate.Passthrough || mergedCreate.StrictSchema {
					t.Fatalf("test requires a merged non-strict reviewed route: %+v", mergedCreate)
				}

				before, err := story.NewService(harness.store).GetProject(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				valid := llm.ToolCall{ID: "valid-reviewed-sibling", Name: "request_api", Arguments: requestAPITestArguments(t, map[string]any{
					"method": "PATCH", "url": "/api/v1/projects/" + harness.project.UUID,
					"request_body":    map[string]any{"name": "must-not-be-written", "description": "schema batch", "expected_revision": float64(before.Revision)},
					"response_filter": ".data | {uuid,name,revision}",
				})}
				invalidBody := map[string]any{"chapter_code": "must-not-exist", "title": "Must not exist"}
				if invalidKind == "unknown_field" {
					invalidBody["private_state"] = true
				} else {
					delete(invalidBody, "chapter_code")
				}
				invalidArgs := map[string]any{
					"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters",
					"request_body": invalidBody, "response_filter": ".data | {uuid,title,revision}",
				}
				if _, parseErr := harness.service.parseAgentAPIRequest(toolContext{
					ProjectUUID: harness.project.UUID, ToolMode: ToolModeProjectAPI,
					Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject},
				}, invalidArgs); errorCode(parseErr) != CodeToolValidation {
					t.Fatalf("merged reviewed schema failure was not rejected: %v", parseErr)
				}
				invalid := llm.ToolCall{ID: "invalid-reviewed-schema", Name: "request_api", Arguments: requestAPITestArguments(t, invalidArgs)}
				calls := []llm.ToolCall{valid, invalid}
				if invalidFirst {
					calls = []llm.ToolCall{invalid, valid}
				}
				harness.model.responses = []llm.ChatResponse{
					{Message: llm.ChatMessage{Role: "assistant", ToolCalls: calls}, FinishReason: "tool_calls"},
					finalResponse("已拒绝非法 reviewed schema 批次。"),
				}
				thread := harness.createThread(t)
				turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "schema fail closed"})
				if err != nil {
					t.Fatal(err)
				}
				if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
					t.Fatal(err)
				}

				after, err := story.NewService(harness.store).GetProject(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				chapters, err := story.NewService(harness.store).ListChapters(context.Background(), "active")
				if err != nil {
					t.Fatal(err)
				}
				var executions, validItems int64
				if err := harness.store.DB().Table("agent_tool_executions").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Count(&executions).Error; err != nil {
					t.Fatal(err)
				}
				if err := harness.store.DB().Table("chat_items").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='tool_call' AND json_extract(metadata_json,'$.provider_call_id')=?", turn.UUID, valid.ID).Count(&validItems).Error; err != nil {
					t.Fatal(err)
				}
				if dispatcherCalls != 0 || executions != 0 || validItems != 0 || after.Name != before.Name || after.Revision != before.Revision || len(chapters) != 0 {
					t.Fatalf("dispatcher=%d executions=%d valid_items=%d project=%+v -> %+v chapters=%d", dispatcherCalls, executions, validItems, before, after, len(chapters))
				}
			})
		}
	}
}

func TestToolCallBatchRejectsProviderCallIDReusedWithChangedArguments(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "严格配对"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(context.Background(), harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	first := llm.ToolCall{ID: "stable-provider-call", Name: "request_api", Arguments: requestAPITestArguments(t, map[string]any{
		"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID,
		"response_filter": ".data | {uuid,name,revision}",
	})}
	prepared, err := harness.service.prepareToolIntent(context.Background(), harness.store, tc, first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.service.persistPreparedToolIntentBatch(context.Background(), harness.store, tc, []preparedToolIntent{prepared}); err != nil {
		t.Fatal(err)
	}
	changed := first
	changed.Arguments = requestAPITestArguments(t, map[string]any{
		"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID,
		"response_filter": ".data | {uuid,description,revision}",
	})
	if _, err := harness.service.prepareToolIntent(context.Background(), harness.store, tc, changed); errorCode(err) != CodeStateConflict {
		t.Fatalf("changed arguments error=%v code=%s", err, errorCode(err))
	}
	invalidArguments := first
	invalidArguments.Arguments = `{broken`
	if _, err := harness.service.prepareToolIntent(context.Background(), harness.store, tc, invalidArguments); errorCode(err) != CodeStateConflict {
		t.Fatalf("invalid arguments reused ID error=%v code=%s", err, errorCode(err))
	}
	differentTool := llm.ToolCall{ID: first.ID, Name: "read_agent_doc", Arguments: `{"path":"/api/v1/agent-docs/overview.md"}`}
	if _, err := harness.service.prepareToolIntent(context.Background(), harness.store, tc, differentTool); errorCode(err) != CodeStateConflict {
		t.Fatalf("different tool reused ID error=%v code=%s", err, errorCode(err))
	}
	var executions int64
	if err := harness.store.DB().Table("agent_tool_executions").Where("run_id=?", tc.Run.ID).Count(&executions).Error; err != nil || executions != 1 {
		t.Fatalf("executions=%d err=%v", executions, err)
	}
}

func TestToolCallBatchTransactionRollsBackEveryIntentOnInsertFailure(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "事务回滚"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(context.Background(), harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	readProject := requestAPITestArguments(t, map[string]any{
		"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID,
		"response_filter": ".data | {uuid,name,revision}",
	})
	calls := []llm.ToolCall{
		{ID: "rollback-read-1", Name: "request_api", Arguments: readProject},
		{ID: "rollback-read-2", Name: "request_api", Arguments: readProject},
	}
	prepared, _, err := harness.service.prepareToolIntentBatch(context.Background(), harness.store, tc, calls)
	if err != nil {
		t.Fatal(err)
	}
	prepared[1].ExecutionUUID = prepared[0].ExecutionUUID
	if _, err := harness.service.persistPreparedToolIntentBatch(context.Background(), harness.store, tc, prepared); err == nil {
		t.Fatal("expected the duplicate execution UUID to abort the transaction")
	}
	var executions, items int64
	if err := harness.store.DB().Table("agent_tool_executions").Where("run_id=?", tc.Run.ID).Count(&executions).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("chat_items").Where("run_id=? AND item_type='tool_call'", tc.Run.ID).Count(&items).Error; err != nil {
		t.Fatal(err)
	}
	if executions != 0 || items != 0 {
		t.Fatalf("partial transaction survived: executions=%d items=%d", executions, items)
	}
}

func TestCommittedToolCallBatchRecoversAndExecutesInProviderOrder(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "提交后恢复"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(context.Background(), harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	if err := harness.service.claimRun(context.Background(), harness.store, &tc); err != nil {
		t.Fatal(err)
	}
	tc.RequestUUID, tc.RequestOrdinal = mustAgentUUID(t), 1
	readProject := requestAPITestArguments(t, map[string]any{
		"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID,
		"response_filter": ".data | {uuid,name,revision}",
	})
	calls := []llm.ToolCall{
		{ID: "recover-read-1", Name: "request_api", Arguments: readProject},
		{ID: "recover-read-2", Name: "request_api", Arguments: readProject},
	}
	prepared, _, err := harness.service.prepareToolIntentBatch(context.Background(), harness.store, tc, calls)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err = harness.service.persistPreparedToolIntentBatch(context.Background(), harness.store, tc, prepared)
	if err != nil {
		t.Fatal(err)
	}

	restartedModel := &toolModelFake{responses: []llm.ChatResponse{finalResponse("恢复完成。")}}
	restarted := NewService(harness.projects, harness.providers, restartedModel, harness.queue, nil)
	if err := restarted.ExecuteJob(context.Background(), harness.store, JobSpec{Version: 1, ProjectUUID: harness.project.UUID, JobKind: JobChatTurn, ResourceUUID: turn.UUID, ThreadUUID: thread.UUID}); err != nil {
		t.Fatal(err)
	}
	var results []struct {
		RemoteItemUUID string `gorm:"column:remote_item_uuid"`
	}
	if err := harness.store.DB().Table("chat_items").Select("remote_item_uuid").Where("run_id=? AND item_type='tool_result'", tc.Run.ID).Order("sequence,id").Scan(&results).Error; err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].RemoteItemUUID != prepared[0].PublicCallUUID || results[1].RemoteItemUUID != prepared[1].PublicCallUUID {
		t.Fatalf("tool result order=%+v", results)
	}
	var incomplete int64
	if err := harness.store.DB().Table("agent_tool_executions").Where("run_id=? AND state<>'completed'", tc.Run.ID).Count(&incomplete).Error; err != nil || incomplete != 0 {
		t.Fatalf("incomplete=%d err=%v", incomplete, err)
	}

	items, err := loadContextItems(context.Background(), harness.store, tc.Thread.ID, tc.Turn.ID, tc.Turn.QueueSequence)
	if err != nil {
		t.Fatal(err)
	}
	messages := contextMessages(items, "", tc.Turn.ID, contextPromptSet{})
	batchIndex := -1
	for index, message := range messages {
		if len(message.ToolCalls) == 2 {
			batchIndex = index
			break
		}
	}
	if batchIndex < 0 || batchIndex+2 >= len(messages) {
		t.Fatalf("rebuilt context did not contain one complete tool batch: %+v", messages)
	}
	if messages[batchIndex].Role != "assistant" || messages[batchIndex].ToolCalls[0].ID != calls[0].ID || messages[batchIndex].ToolCalls[1].ID != calls[1].ID {
		t.Fatalf("rebuilt assistant batch=%+v", messages[batchIndex])
	}
	if messages[batchIndex+1].Role != "tool" || messages[batchIndex+1].ToolCallID != calls[0].ID || messages[batchIndex+2].Role != "tool" || messages[batchIndex+2].ToolCallID != calls[1].ID {
		t.Fatalf("rebuilt tool results=%+v %+v", messages[batchIndex+1], messages[batchIndex+2])
	}
}

func TestToolCallBatchCommitRechecksConcurrentPreparedIntent(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "并发提交"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(context.Background(), harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	call := llm.ToolCall{ID: "same-prepared-call", Name: "request_api", Arguments: requestAPITestArguments(t, map[string]any{
		"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID,
		"response_filter": ".data | {uuid,name,revision}",
	})}
	first, err := harness.service.prepareToolIntent(context.Background(), harness.store, tc, call)
	if err != nil {
		t.Fatal(err)
	}
	second, err := harness.service.prepareToolIntent(context.Background(), harness.store, tc, call)
	if err != nil {
		t.Fatal(err)
	}
	firstBatch, err := harness.service.persistPreparedToolIntentBatch(context.Background(), harness.store, tc, []preparedToolIntent{first})
	if err != nil {
		t.Fatal(err)
	}
	secondBatch, err := harness.service.persistPreparedToolIntentBatch(context.Background(), harness.store, tc, []preparedToolIntent{second})
	if err != nil {
		t.Fatal(err)
	}
	if firstBatch[0].Existing.ID == 0 || secondBatch[0].Existing.ID != firstBatch[0].Existing.ID || secondBatch[0].New {
		t.Fatalf("first=%+v second=%+v", firstBatch[0], secondBatch[0])
	}
}
