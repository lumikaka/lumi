package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"lumi/internal/llm"
	"lumi/internal/project"
	"lumi/internal/story"
)

func seedLegacyBootstrapYoloAuthorization(t *testing.T, harness *agentHarness, tc toolContext, creationSessionUUID string) toolContext {
	t.Helper()
	ctx := context.Background()
	if !isUUIDv7(creationSessionUUID) {
		t.Fatal("creation session UUID must be UUIDv7")
	}
	confirmationCallUUID := mustAgentUUID(t)
	replayCallUUID := mustAgentUUID(t)
	requestUUID := mustAgentUUID(t)
	safeOptionUUID := mustAgentUUID(t)
	confirmOptionUUID := mustAgentUUID(t)
	expectedRevision := int64(1)
	if setup, err := harness.store.ProjectSetup(ctx); err == nil && setup.Revision > 0 {
		expectedRevision = setup.Revision
	}
	binding := dangerousConfirmationBinding{
		Route: RouteProjectSetupFinalize, ProjectUUID: tc.ProjectUUID, TargetUUID: tc.ProjectUUID,
		ExpectedRevision: expectedRevision, RequestFingerprint: "sha256:" + strings.Repeat("a", 64),
		QuestionID: bootstrapYoloConfirmationQuestionID, ConfirmOption: 1,
	}
	requestJSON, _ := json.Marshal(map[string]any{"questions": []map[string]any{{
		"header": "创建确认", "id": bootstrapYoloConfirmationQuestionID, "question": "是否定稿并启动 YOLO？",
		"options": []map[string]any{
			{"uuid": safeOptionUUID, "label": "继续修改 (Recommended)", "description": "不定稿也不启动。"},
			{"uuid": confirmOptionUUID, "label": "定稿并启动 YOLO", "description": "定稿后立即启动受控 YOLO。"},
		},
	}}})
	responseJSON, _ := json.Marshal(map[string]any{"answers": map[string]any{
		bootstrapYoloConfirmationQuestionID: map[string]any{"selected_option_uuid": confirmOptionUUID, "other_text": ""},
	}})
	confirmationArguments, _ := json.Marshal(map[string]any{"questions": []any{}, "confirmation": binding})
	replayArguments, _ := json.Marshal(map[string]any{
		"__confirmation_auto_replay": true, "__confirmation_request_uuid": requestUUID,
		"__route_id": RouteProjectSetupFinalize,
	})

	sqlDB, err := harness.store.DB().DB()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO project_creation_bootstraps(uuid,project_id,creation_session_uuid,thread_id,turn_id,created_at) VALUES(?,?,?,?,?,?)`, mustAgentUUID(t), tc.Thread.ProjectID, creationSessionUUID, tc.Thread.ID, tc.Turn.ID, now); err != nil {
		t.Fatal(err)
	}
	thread := tc.Thread
	confirmationItem, err := appendItemTx(ctx, tx, &thread, &tc.Turn.ID, &tc.Run.ID, "user_input_request", "assistant", string(requestJSON), "json", "completed", confirmationCallUUID, "request_user_input", tc.ProjectUUID, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	replayItem, err := appendItemTx(ctx, tx, &thread, &tc.Turn.ID, &tc.Run.ID, "tool_call", "assistant", "{}", "json", "completed", replayCallUUID, "request_api", tc.ProjectUUID, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_tool_executions(uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,tool_name,target_uuid,arguments_json,idempotency_key,state,result_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,'completed','{"success":true,"data":{}}',?,?)`, mustAgentUUID(t), tc.Thread.ID, tc.Run.ID, tc.Turn.ID, confirmationItem.ID, confirmationCallUUID, "request_user_input", tc.ProjectUUID, string(confirmationArguments), "test-confirmation:"+requestUUID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO chat_user_input_requests(uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,schema_version,request_json,response_json,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,'resumed',?,?)`, requestUUID, tc.Thread.ID, tc.Run.ID, tc.Turn.ID, confirmationItem.ID, confirmationCallUUID, userInputSchemaCodexQuestions, string(requestJSON), string(responseJSON), now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_tool_executions(uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,tool_name,target_uuid,arguments_json,idempotency_key,state,result_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,'completed','{"success":true,"data":{}}',?,?)`, mustAgentUUID(t), tc.Thread.ID, tc.Run.ID, tc.Turn.ID, replayItem.ID, replayCallUUID, "request_api", tc.ProjectUUID, string(replayArguments), "test-replay:"+requestUUID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, now, thread.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	tc.Thread = thread
	tc.BootstrapCreationSessionUUID = creationSessionUUID
	return tc
}

func seedReadyBootstrapYoloAuthorization(t *testing.T, harness *agentHarness, tc toolContext, creationSessionUUID string) toolContext {
	t.Helper()
	ctx := context.Background()
	if !isUUIDv7(creationSessionUUID) {
		t.Fatal("creation session UUID must be UUIDv7")
	}
	expectedRevision := int64(1)
	if setup, err := harness.store.ProjectSetup(ctx); err == nil && setup.Revision > 0 {
		expectedRevision = setup.Revision
	}
	publicCallUUID, executionUUID := mustAgentUUID(t), mustAgentUUID(t)
	arguments, _ := json.Marshal(map[string]any{
		"method": "POST", "url": "/api/v1/projects/" + tc.ProjectUUID + "/project-setup-finalizations",
		"request_body":                      map[string]any{"expected_revision": expectedRevision},
		"response_filter":                   ".data | {uuid,project_uuid,setup_status,status,revision,draft_values,field_sources,final_picture_book,reference_plan,updated_at}",
		"__provider_call_id":                bootstrapRuntimeProviderCallID("finalize", creationSessionUUID, "test"),
		"__route_id":                        RouteProjectSetupFinalize,
		"__action":                          "定稿项目初始化设置",
		"__method":                          "POST",
		"__path":                            "/api/v1/projects/" + tc.ProjectUUID + "/project-setup-finalizations",
		"__target_uuid":                     tc.ProjectUUID,
		"__runtime_generated_bootstrap":     true,
		"__bootstrap_action":                "finalize",
		"__bootstrap_creation_session_uuid": creationSessionUUID,
	})

	sqlDB, err := harness.store.DB().DB()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO project_creation_bootstraps(uuid,project_id,creation_session_uuid,thread_id,turn_id,created_at) VALUES(?,?,?,?,?,?)`, mustAgentUUID(t), tc.Thread.ProjectID, creationSessionUUID, tc.Thread.ID, tc.Turn.ID, now); err != nil {
		t.Fatal(err)
	}
	thread := tc.Thread
	item, err := appendItemTx(ctx, tx, &thread, &tc.Turn.ID, &tc.Run.ID, "tool_call", "assistant", "{}", "json", "completed", publicCallUUID, "request_api", tc.ProjectUUID, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_tool_executions(uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,tool_name,target_uuid,arguments_json,idempotency_key,state,result_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,'completed','{"success":true,"data":{}}',?,?)`, executionUUID, tc.Thread.ID, tc.Run.ID, tc.Turn.ID, item.ID, publicCallUUID, "request_api", tc.ProjectUUID, string(arguments), "test-runtime-finalization:"+creationSessionUUID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, now, thread.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	tc.Thread = thread
	tc.BootstrapCreationSessionUUID = creationSessionUUID
	return tc
}

func seedBootstrapCreationReference(t *testing.T, harness *agentHarness, projectID int64, creationSessionUUID string, position int, role, title, instruction string, include bool) (string, string) {
	t.Helper()
	now := time.Now().UTC()
	objectUUID, fileUUID := mustAgentUUID(t), mustAgentUUID(t)
	bindingUUID, sourceReferenceUUID := mustAgentUUID(t), mustAgentUUID(t)
	sha := strings.Repeat("c", 63) + string(rune('0'+position))
	if err := harness.store.DB().Exec(`INSERT INTO file_objects(uuid,project_id,sha256,key_path,mime_type,canonical_ext,byte_size,width,height,state,created_at,verified_at) VALUES(?,?,?,?,'image/png','png',128,16,16,'ready',?,?)`, objectUUID, projectID, sha, "objects/"+objectUUID+".png", now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Exec(`INSERT INTO files(uuid,project_id,file_object_id,kind,purpose,original_filename,display_name,source_type,metadata_json,created_at) VALUES(?,?,(SELECT id FROM file_objects WHERE uuid=?),'image','project_chatbot_reference',?,?,'imported','{}',?)`, fileUUID, projectID, objectUUID, title+".png", title, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Exec(`INSERT INTO project_creation_reference_files(uuid,project_id,creation_session_uuid,reference_uuid,position,file_id,reference_role,title,instruction,include_in_yolo,plan_source,created_at,updated_at) VALUES(?,?,?,?,?,(SELECT id FROM files WHERE uuid=?),?,?,?,?, 'user_confirmed',?,?)`, bindingUUID, projectID, creationSessionUUID, sourceReferenceUUID, position, fileUUID, role, title, instruction, include, now, now).Error; err != nil {
		t.Fatal(err)
	}
	return bindingUUID, fileUUID
}

func makeHarnessProjectDraft(t *testing.T, harness *agentHarness, setupUUID, originalInput string) {
	t.Helper()
	now := time.Now().UTC()
	db := harness.store.DB()
	var projectID int64
	if err := db.Table("projects").Select("id").Where("uuid = ?", harness.project.UUID).Scan(&projectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DROP TRIGGER project_picture_book_profiles_immutable_delete").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM project_picture_book_profiles WHERE project_id = ?", projectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE projects SET setup_status = 'draft' WHERE id = ?", projectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO project_setup_drafts(
		uuid,project_id,status,revision,original_input,generation_language,generation_brief,field_sources_json,missing_fields_json,
		error_code,error_message,created_at,updated_at
	) VALUES(?,?,'draft',1,?,'zh-Hans',?,'{"generation_language":"system_default","generation_brief":"system_default"}','["project_name","overall_style","picture_book.format"]','','',?,?)`, setupUUID, projectID, originalInput, strings.TrimSpace(originalInput), now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.RefreshProject(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func installProjectSetupDispatcherForTest(t *testing.T, harness *agentHarness) {
	t.Helper()
	harness.service.projectAPIDispatcher = func(ctx context.Context, input ProjectAPIDispatchRequest) (ProjectAPIDispatchResponse, error) {
		var state project.SetupState
		var err error
		switch {
		case input.Method == "GET" && strings.HasSuffix(input.Path, "/project-setup"):
			state, err = harness.store.ProjectSetup(ctx)
		case input.Method == "PATCH" && strings.HasSuffix(input.Path, "/project-setup"):
			encoded, encodeErr := json.Marshal(input.Body)
			if encodeErr != nil {
				return ProjectAPIDispatchResponse{}, encodeErr
			}
			var request struct {
				ExpectedRevision   int64                     `json:"expected_revision"`
				ProjectName        *string                   `json:"project_name"`
				GenerationLanguage *string                   `json:"generation_language"`
				OverallStyle       *string                   `json:"overall_style"`
				GenerationBrief    *string                   `json:"generation_brief"`
				PictureBook        *project.PictureBookInput `json:"picture_book"`
			}
			if decodeErr := json.Unmarshal(encoded, &request); decodeErr != nil {
				return ProjectAPIDispatchResponse{}, decodeErr
			}
			state, err = harness.store.UpdateProjectSetupDraft(ctx, project.SetupDraftPatchInput{
				ExpectedRevision: request.ExpectedRevision, ProjectName: request.ProjectName,
				GenerationLanguage: request.GenerationLanguage, OverallStyle: request.OverallStyle,
				GenerationBrief: request.GenerationBrief,
				PictureBook:     request.PictureBook,
			})
		case input.Method == "POST" && strings.HasSuffix(input.Path, "/project-setup-finalizations"):
			state, err = harness.store.FinalizeProjectSetup(ctx, intArg(input.Body, "expected_revision"))
			if err == nil {
				err = story.NewService(harness.store).EnsurePromptCatalogVersions(ctx, "project_created")
			}
			if err == nil {
				err = harness.projects.SyncProjectName(ctx, harness.project.UUID)
			}
		default:
			return ProjectAPIDispatchResponse{}, domainError(CodeNotFound, "测试项目 API 路由不存在", input.Method+" "+input.Path, nil)
		}
		if err != nil {
			return ProjectAPIDispatchResponse{}, err
		}
		body, err := json.Marshal(map[string]any{"success": true, "data": state})
		return ProjectAPIDispatchResponse{Status: 200, Body: body}, err
	}
}

func TestBootstrapConversationIsExactlyOnceAndRunsInDraftContext(t *testing.T) {
	harness := newAgentHarness(t, finalResponse("我会先整理初始化草稿。"))
	ctx := context.Background()
	creationSessionUUID := mustAgentUUID(t)
	originalInput := "  我要一本水彩风格、讲小狐狸给月亮送信的儿童绘本。\n"
	makeHarnessProjectDraft(t, harness, creationSessionUUID, originalInput)

	first, err := harness.service.BootstrapConversation(ctx, harness.project.UUID, creationSessionUUID, originalInput, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := harness.service.BootstrapConversation(ctx, harness.project.UUID, creationSessionUUID, originalInput, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Thread.UUID != second.Thread.UUID || first.Turn.UUID != second.Turn.UUID || first.Thread.ThreadType != ThreadTypeConversation || first.Turn.InputText != originalInput {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	for table, want := range map[string]int64{"project_creation_bootstraps": 1, "chat_threads": 1, "chat_turns": 1, "chat_runs": 1} {
		var count int64
		if err := harness.store.DB().Table(table).Count(&count).Error; err != nil || count != want {
			t.Fatalf("%s count=%d want=%d err=%v", table, count, want, err)
		}
	}
	var userMessages int64
	if err := harness.store.DB().Table("chat_items").Where("item_type='user_message' AND content=?", originalInput).Count(&userMessages).Error; err != nil || userMessages != 1 {
		t.Fatalf("user messages=%d err=%v", userMessages, err)
	}
	if len(harness.queue.jobs) != 1 || harness.queue.jobs[0].ResourceUUID != first.Turn.UUID || harness.queue.jobs[0].ThreadUUID != first.Thread.UUID {
		t.Fatalf("jobs=%+v", harness.queue.jobs)
	}

	tc := toolContext{ProjectUUID: harness.project.UUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: first.Thread.UUID, ThreadType: ThreadTypeConversation}}
	if _, err := harness.service.executeImageGenTool(ctx, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t)}, map[string]any{"prompt": "moon", "reference_uuids": []any{}}); errorCode(err) != project.CodeProjectSetupIncomplete {
		t.Fatalf("draft image_gen error=%v", err)
	}
	_, err = executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "draft-write"}, map[string]any{
		"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters",
		"request_body": map[string]any{"chapter_code": "vol01.ch01", "title": "Not yet"}, "response_filter": ".data | {uuid,revision}",
	})
	var domainErr *Error
	if !errors.As(err, &domainErr) || domainErr.Code != project.CodeProjectSetupIncomplete {
		t.Fatalf("draft write error=%v", err)
	}

	if err := harness.service.ExecuteJob(ctx, harness.store, harness.queue.jobs[0]); err != nil {
		t.Fatal(err)
	}
	harness.model.mu.Lock()
	requests := append([]string(nil), func() []string {
		items := make([]string, 0, len(harness.model.requests))
		for _, request := range harness.model.requests {
			if len(request.Messages) > 0 {
				items = append(items, request.Messages[0].Content)
			}
		}
		return items
	}()...)
	harness.model.mu.Unlock()
	if len(requests) != 1 || !strings.Contains(requests[0], `"setup_status":"draft"`) || !strings.Contains(requests[0], "/guides/初始化新项目.md") {
		t.Fatalf("draft system prompt=%q", strings.Join(requests, "\n"))
	}
}

func TestBootstrapConversationAttachesReferencesToTheFirstUserItemExactlyOnce(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	referenceFile := createChatReferenceFile(t, harness.store, "bootstrap-reference.png", agentTestPNG(t))
	creationSessionUUID := mustAgentUUID(t)
	input := "请参考这张图创建一本月亮邮差绘本。"
	makeHarnessProjectDraft(t, harness, creationSessionUUID, input)
	references := []ReferenceInput{{ResourceType: ReferenceTypeFile, ResourceUUID: referenceFile.UUID}}

	first, err := harness.service.BootstrapConversation(ctx, harness.project.UUID, creationSessionUUID, input, references)
	if err != nil {
		t.Fatal(err)
	}
	second, err := harness.service.BootstrapConversation(ctx, harness.project.UUID, creationSessionUUID, input, references)
	if err != nil {
		t.Fatal(err)
	}
	if first.Thread.UUID != second.Thread.UUID || first.Turn.UUID != second.Turn.UUID {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	page, err := harness.service.ListItems(ctx, harness.project.UUID, first.Thread.UUID, "", "", 20)
	if err != nil || len(page.Items) != 1 || page.Items[0].ItemType != "user_message" || len(page.Items[0].References) != 1 {
		t.Fatalf("bootstrap items=%+v err=%v", page.Items, err)
	}
	reference := page.Items[0].References[0]
	if reference.ResourceType != ReferenceTypeFile || reference.ResourceUUID != referenceFile.UUID || !reference.ImageAvailable || reference.Position != 1 {
		t.Fatalf("bootstrap reference=%+v", reference)
	}
	var count int64
	if err := harness.store.DB().Table("chat_context_references").Where("resource_type=? AND resource_uuid=?", ReferenceTypeFile, referenceFile.UUID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("stored bootstrap references=%d err=%v", count, err)
	}
}

func TestBootstrapConversationRollsBackAndRecoversEveryAtomicBoundary(t *testing.T) {
	tests := []struct {
		name  string
		table string
		queue bool
	}{
		{name: "thread insert", table: "chat_threads"},
		{name: "turn insert", table: "chat_turns"},
		{name: "run insert", table: "chat_runs"},
		{name: "River job insert", queue: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newAgentHarness(t)
			ctx := context.Background()
			sessionUUID := mustAgentUUID(t)
			makeHarnessProjectDraft(t, harness, sessionUUID, "failure-injected first turn")
			if test.queue {
				harness.queue.enqueueErr = errors.New("injected queue failure")
			} else {
				if err := harness.store.DB().Exec("CREATE TRIGGER fail_bootstrap BEFORE INSERT ON " + test.table + " BEGIN SELECT RAISE(ABORT, 'injected bootstrap failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			if _, err := harness.service.BootstrapConversation(ctx, harness.project.UUID, sessionUUID, "failure-injected first turn", nil); err == nil {
				t.Fatal("injected bootstrap failure was ignored")
			}
			for _, table := range []string{"project_creation_bootstraps", "chat_threads", "chat_turns", "chat_runs"} {
				var count int64
				if err := harness.store.DB().Table(table).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("%s count after rollback=%d err=%v", table, count, err)
				}
			}
			if test.queue {
				harness.queue.enqueueErr = nil
			} else if err := harness.store.DB().Exec("DROP TRIGGER fail_bootstrap").Error; err != nil {
				t.Fatal(err)
			}
			result, err := harness.service.BootstrapConversation(ctx, harness.project.UUID, sessionUUID, "failure-injected first turn", nil)
			if err != nil || result.Thread.UUID == "" || result.Turn.UUID == "" {
				t.Fatalf("recovered=%+v err=%v", result, err)
			}
			if len(harness.queue.jobs) != 1 {
				t.Fatalf("jobs=%+v", harness.queue.jobs)
			}
		})
	}
}

func TestReadyBootstrapTurnOnlyAllowsReadsAndAuthorizedYolo(t *testing.T) {
	harness := newAgentHarness(t, finalResponse("已启动。"))
	ctx := context.Background()
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "创建一本月光邮差绘本"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	sessionUUID := mustAgentUUID(t)
	tc = seedReadyBootstrapYoloAuthorization(t, harness, tc, sessionUUID)
	includedReferenceUUID, includedFileUUID := seedBootstrapCreationReference(t, harness, tc.Thread.ProjectID, sessionUUID, 1, "character", "月光邮差", "保持角色轮廓和服装层次", true)
	excludedReferenceUUID, _ := seedBootstrapCreationReference(t, harness, tc.Thread.ProjectID, sessionUUID, 2, "style", "旧版笔触", "只供讨论，不参与本次生成", false)
	reloaded, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil || reloaded.BootstrapCreationSessionUUID != sessionUUID {
		t.Fatalf("bootstrap context=%+v err=%v", reloaded, err)
	}
	reloaded.ToolMode = ToolModeProjectAPI
	if err := harness.service.claimRun(ctx, harness.store, &reloaded); err != nil {
		t.Fatal(err)
	}

	if _, err := executeRequestAPITool(ctx, harness.service, harness.store, reloaded, toolExecutionRecord{UUID: mustAgentUUID(t)}, map[string]any{
		"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID,
		"response_filter": ".data | {uuid,name,generation_language,revision}",
	}); err != nil {
		t.Fatalf("bootstrap GET was blocked: %v", err)
	}
	blocked := []map[string]any{
		{
			"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters",
			"request_body":    map[string]any{"chapter_code": "vol01.ch02", "title": "不应创建"},
			"response_filter": ".data | {uuid,chapter_code,title,revision}",
		},
		{
			"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/chapter-batches",
			"request_body":    map[string]any{"prompt": "连续创建六章", "chapter_count": float64(6)},
			"response_filter": ".data | {uuid,kind,status}",
		},
	}
	for _, args := range blocked {
		if _, err := executeRequestAPITool(ctx, harness.service, harness.store, reloaded, toolExecutionRecord{UUID: mustAgentUUID(t)}, args); errorCode(err) != CodeBootstrapProductionRequiresYolo {
			t.Fatalf("bootstrap write was not blocked: args=%+v err=%v", args, err)
		}
	}
	if _, err := harness.service.executeImageGenTool(ctx, harness.store, reloaded, toolExecutionRecord{UUID: mustAgentUUID(t)}, map[string]any{"prompt": "moon", "reference_uuids": []any{}}); errorCode(err) != CodeBootstrapProductionRequiresYolo {
		t.Fatalf("bootstrap image_gen error=%v", err)
	}

	yoloArgs := func(prompt string) map[string]any {
		return map[string]any{
			"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/workflows",
			"request_body":    map[string]any{"story_prompt": prompt},
			"response_filter": ".data | {uuid,thread_uuid,presentation_mode,kind,title,status,current_step_key,steps}",
		}
	}
	rawArgs, err := json.Marshal(yoloArgs("一只小狐狸替月亮送信。"))
	if err != nil {
		t.Fatal(err)
	}
	execution, replay, completed, err := harness.service.persistToolIntent(ctx, harness.store, reloaded, "bootstrap-yolo-inline", "request_api", string(rawArgs))
	if err != nil || completed || replay != nil {
		t.Fatalf("persist Yolo intent: execution=%+v completed=%v replay=%s err=%v", execution, completed, replay, err)
	}
	if _, err := executeRequestAPIToolWithUIRef(ctx, harness.service, harness.store, reloaded, execution, yoloArgs("一只小狐狸替月亮送信。")); !errors.Is(err, ErrWaitingWorkflow) {
		t.Fatalf("first inline Yolo did not wait: %v", err)
	}
	jobsAfterFirst := len(harness.queue.jobs)
	if _, err := executeRequestAPIToolWithUIRef(ctx, harness.service, harness.store, reloaded, execution, yoloArgs("即使故事 Brief 改变也只能返回同一个工作流。")); !errors.Is(err, ErrWaitingWorkflow) {
		t.Fatalf("replayed inline Yolo did not remain waiting: %v", err)
	}
	if len(harness.queue.jobs) != jobsAfterFirst {
		t.Fatalf("idempotent replay enqueued another job: before=%d after=%d", jobsAfterFirst, len(harness.queue.jobs))
	}
	workflows, err := harness.service.ListWorkflows(ctx, harness.project.UUID)
	if err != nil || len(workflows) != 1 {
		t.Fatalf("workflows=%+v err=%v", workflows, err)
	}
	workflow := workflows[0]
	if workflow.UUID == "" || workflow.ThreadUUID != thread.UUID || workflow.Kind != WorkflowYolo || workflow.PresentationMode != string(PresentationInline) || workflow.OriginTurnUUID != turn.UUID || workflow.OriginRunUUID != reloaded.Run.UUID || workflow.OriginToolCallUUID != execution.ToolCallUUID || workflow.AwaitStatus != "waiting" {
		t.Fatalf("inline workflow=%+v execution=%+v", workflow, execution)
	}
	var snapshotJSON string
	if err := harness.store.DB().Table("workflows").Select("input_snapshot").Where("uuid=?", workflow.UUID).Scan(&snapshotJSON).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot yoloSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 6 || snapshot.CreationSessionUUID != sessionUUID || len(snapshot.CreationReferences) != 2 {
		t.Fatalf("creation reference snapshot=%+v", snapshot)
	}
	if got := snapshot.CreationReferences[0]; got.ReferenceUUID != includedReferenceUUID || got.FileUUID != includedFileUUID || got.ReferenceRole != "character" || got.Title != "月光邮差" || got.Instruction != "保持角色轮廓和服装层次" || !got.IncludeInYolo || got.PlanSource != project.SetupSourceUserConfirmed {
		t.Fatalf("included creation reference=%+v", got)
	}
	if got := snapshot.CreationReferences[1]; got.ReferenceUUID != excludedReferenceUUID || got.IncludeInYolo {
		t.Fatalf("excluded creation reference=%+v", got)
	}
	var frozenEventCount int64
	if err := harness.store.DB().Table("workflow_events").Joins("JOIN workflows ON workflows.id=workflow_events.workflow_id").Where("workflows.uuid=? AND workflow_events.event_type='creation_references_frozen'", workflow.UUID).Count(&frozenEventCount).Error; err != nil || frozenEventCount != 1 {
		t.Fatalf("creation_references_frozen events=%d err=%v", frozenEventCount, err)
	}
	var workflowCount int64
	if err := harness.store.DB().Table("workflows").Where("kind=?", WorkflowYolo).Count(&workflowCount).Error; err != nil || workflowCount != 1 {
		t.Fatalf("workflow count=%d err=%v", workflowCount, err)
	}
	var threadCount, awaitCount int64
	if err := harness.store.DB().Table("chat_threads").Count(&threadCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("workflow_awaits").Where("status='waiting'").Count(&awaitCount).Error; err != nil {
		t.Fatal(err)
	}
	if threadCount != 1 || awaitCount != 1 {
		t.Fatalf("threads=%d awaits=%d", threadCount, awaitCount)
	}
	projectedTurns, err := harness.service.ListTurns(ctx, harness.project.UUID, thread.UUID)
	if err != nil || len(projectedTurns) != 1 || projectedTurns[0].Status != TurnWaitingForWorkflow {
		t.Fatalf("waiting Turn projection=%+v err=%v", projectedTurns, err)
	}

	if _, err := harness.service.CancelWorkflow(ctx, harness.project.UUID, workflow.UUID); err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatResume); err != nil {
		t.Fatal(err)
	}
	secondTurn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "现在手工补一个章节"})
	if err != nil {
		t.Fatal(err)
	}
	secondContext, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, secondTurn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if secondContext.BootstrapCreationSessionUUID != "" {
		t.Fatalf("later Turn inherited bootstrap metadata: %+v", secondContext)
	}
	secondContext.ToolMode = ToolModeProjectAPI
	if _, err := executeRequestAPITool(ctx, harness.service, harness.store, secondContext, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "later-turn-chapter"}, map[string]any{
		"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters",
		"request_body":    map[string]any{"chapter_code": "vol02.ch01", "title": "后续普通 Turn"},
		"response_filter": ".data | {uuid,chapter_code,title,revision}",
	}); err != nil {
		t.Fatalf("later ready Turn remained restricted: %v", err)
	}
}

func TestBootstrapYoloAuthorizationRequiresExactRuntimeFinalizationEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *agentHarness, *toolContext)
		want   bool
	}{
		{name: "exact runtime finalization", want: true},
		{name: "not runtime generated", mutate: func(t *testing.T, harness *agentHarness, _ *toolContext) {
			if err := harness.store.DB().Exec(`UPDATE agent_tool_executions SET arguments_json=json_remove(arguments_json,'$.__runtime_generated_bootstrap') WHERE tool_name='request_api'`).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "wrong bootstrap action", mutate: func(t *testing.T, harness *agentHarness, _ *toolContext) {
			if err := harness.store.DB().Exec(`UPDATE agent_tool_executions SET arguments_json=json_set(arguments_json,'$.__bootstrap_action','start_generation') WHERE tool_name='request_api'`).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "wrong creation session", mutate: func(t *testing.T, harness *agentHarness, _ *toolContext) {
			if err := harness.store.DB().Exec(`UPDATE agent_tool_executions SET arguments_json=json_set(arguments_json,'$.__bootstrap_creation_session_uuid',?) WHERE tool_name='request_api'`, mustAgentUUID(t)).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "wrong target", mutate: func(t *testing.T, harness *agentHarness, _ *toolContext) {
			if err := harness.store.DB().Exec(`UPDATE agent_tool_executions SET arguments_json=json_set(arguments_json,'$.__target_uuid',?) WHERE tool_name='request_api'`, mustAgentUUID(t)).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "wrong route", mutate: func(t *testing.T, harness *agentHarness, _ *toolContext) {
			if err := harness.store.DB().Exec(`UPDATE agent_tool_executions SET arguments_json=json_set(arguments_json,'$.__route_id','project.get') WHERE tool_name='request_api'`).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "failed runtime finalization", mutate: func(t *testing.T, harness *agentHarness, _ *toolContext) {
			if err := harness.store.DB().Exec(`UPDATE agent_tool_executions SET result_json='{"success":false,"data":null,"error":{"code":"failed"}}' WHERE tool_name='request_api'`).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "other Run", mutate: func(_ *testing.T, _ *agentHarness, tc *toolContext) { tc.Run.ID++ }},
		{name: "ordinary Turn", mutate: func(_ *testing.T, _ *agentHarness, tc *toolContext) { tc.BootstrapCreationSessionUUID = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newAgentHarness(t)
			ctx := context.Background()
			thread := harness.createThread(t)
			turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "authorization evidence"})
			if err != nil {
				t.Fatal(err)
			}
			tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
			if err != nil {
				t.Fatal(err)
			}
			tc = seedReadyBootstrapYoloAuthorization(t, harness, tc, mustAgentUUID(t))
			if test.mutate != nil {
				test.mutate(t, harness, &tc)
			}
			got, err := bootstrapYoloAuthorized(ctx, harness.store, tc)
			if err != nil || got != test.want {
				t.Fatalf("authorized=%v want=%v err=%v", got, test.want, err)
			}
		})
	}
}

func TestBootstrapYoloAuthorizationAcceptsLegacyConfirmedFinalizationEvidence(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "legacy authorization evidence"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc = seedLegacyBootstrapYoloAuthorization(t, harness, tc, mustAgentUUID(t))
	if authorized, err := bootstrapYoloAuthorized(ctx, harness.store, tc); err != nil || !authorized {
		t.Fatalf("legacy authorization accepted=%v err=%v", authorized, err)
	}
}

func TestBootstrapAutoFinalizesAndStartsOneYoloWorkflowWithoutConfirmation(t *testing.T) {
	harness := newAgentHarness(t)
	installProjectSetupDispatcherForTest(t, harness)
	ctx := context.Background()
	setupUUID := mustAgentUUID(t)
	creationSessionUUID := mustAgentUUID(t)
	makeHarnessProjectDraft(t, harness, setupUUID, "我要一本水彩风格、讲小狐狸给月亮送信的儿童绘本。")
	bootstrap, err := harness.service.BootstrapConversation(ctx, harness.project.UUID, creationSessionUUID, "我要一本水彩风格、讲小狐狸给月亮送信的儿童绘本。", nil)
	if err != nil {
		t.Fatal(err)
	}
	setupPatch := map[string]any{
		"method": "PATCH", "url": "/api/v1/projects/" + harness.project.UUID + "/project-setup",
		"request_body": map[string]any{
			"expected_revision": float64(1), "project_name": "月光邮差", "generation_language": "zh-Hans",
			"overall_style": "温暖透明水彩，柔和月光", "generation_brief": "原始需求：小狐狸给月亮送信；温暖水彩，面向儿童，温暖结局。", "picture_book": map[string]any{"format": "vertical_strip"},
		},
		"response_filter": ".data | {uuid,setup_status,status,revision,draft_values,field_sources,missing_information}",
	}
	toolCall := func(id, name string, arguments map[string]any) llm.ChatResponse {
		encoded, _ := json.Marshal(arguments)
		return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: id, Name: name, Arguments: string(encoded)}}}, FinishReason: "tool_calls"}
	}
	harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
		switch call {
		case 1:
			return toolCall("setup-patch", "request_api", setupPatch), nil
		case 2:
			var terminal struct {
				Success bool `json:"success"`
				Data    struct {
					WorkflowUUID     string           `json:"workflow_uuid"`
					ThreadUUID       string           `json:"thread_uuid"`
					PresentationMode string           `json:"presentation_mode"`
					Kind             string           `json:"kind"`
					Status           string           `json:"status"`
					Steps            []map[string]any `json:"steps"`
				} `json:"data"`
			}
			for _, message := range request.Messages {
				if message.Role != "tool" {
					continue
				}
				_ = json.Unmarshal([]byte(message.Content), &terminal)
			}
			if terminal.Success || !isUUIDv7(terminal.Data.WorkflowUUID) || terminal.Data.ThreadUUID != bootstrap.Thread.UUID || terminal.Data.PresentationMode != string(PresentationInline) || terminal.Data.Kind != WorkflowYolo || terminal.Data.Status != WorkflowCancelled || len(terminal.Data.Steps) != len(YoloStepKeys) {
				t.Fatalf("YOLO terminal Tool Result=%+v", terminal)
			}
			return finalResponse("YOLO 已取消，当前对话已恢复。"), nil
		default:
			t.Fatalf("unexpected model call %d", call)
			return finalResponse("unexpected"), nil
		}
	}

	if err := harness.execute(t, bootstrap.Thread.UUID, bootstrap.Turn.UUID, JobChatTurn); !errors.Is(err, ErrWaitingWorkflow) {
		t.Fatalf("bootstrap did not continue directly into the inline workflow: %v", err)
	}
	if harness.model.calls != 1 {
		t.Fatalf("runtime finalization or workflow startup made an extra model call: calls=%d", harness.model.calls)
	}
	requests, err := harness.service.ListUserInputRequests(ctx, harness.project.UUID, bootstrap.Thread.UUID)
	if err != nil || len(requests) != 0 {
		t.Fatalf("bootstrap unexpectedly created confirmation requests=%+v err=%v", requests, err)
	}
	if harness.store.SetupStatus() != project.SetupStatusReady {
		t.Fatalf("setup status=%s", harness.store.SetupStatus())
	}
	var workflows, chapters int64
	if err := harness.store.DB().Table("workflows").Where("kind=?", WorkflowYolo).Count(&workflows).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("chapters").Count(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if workflows != 1 || chapters != 0 {
		t.Fatalf("workflow count=%d chapters before worker=%d", workflows, chapters)
	}
	items, err := harness.service.ListWorkflows(ctx, harness.project.UUID)
	if err != nil || len(items) != 1 || items[0].ThreadUUID != bootstrap.Thread.UUID || items[0].PresentationMode != string(PresentationInline) || items[0].OriginTurnUUID != bootstrap.Turn.UUID || items[0].AwaitStatus != "waiting" {
		t.Fatalf("bootstrap inline workflow=%+v err=%v", items, err)
	}
	var threadCount, awaitCount int64
	if err := harness.store.DB().Table("chat_threads").Count(&threadCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("workflow_awaits").Where("status='waiting'").Count(&awaitCount).Error; err != nil {
		t.Fatal(err)
	}
	if threadCount != 1 || awaitCount != 1 {
		t.Fatalf("threads=%d awaits=%d", threadCount, awaitCount)
	}
	var idempotencyKey string
	if err := harness.store.DB().Table("workflows").Select("idempotency_key").Where("kind=?", WorkflowYolo).Take(&idempotencyKey).Error; err != nil || idempotencyKey != bootstrapYoloIdempotencyPrefix+creationSessionUUID {
		t.Fatalf("idempotency_key=%q err=%v", idempotencyKey, err)
	}
	var inputSnapshot string
	if err := harness.store.DB().Table("workflows").Select("input_snapshot").Where("kind=?", WorkflowYolo).Take(&inputSnapshot).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot yoloSnapshot
	if err := json.Unmarshal([]byte(inputSnapshot), &snapshot); err != nil || snapshot.StoryPrompt != setupPatch["request_body"].(map[string]any)["generation_brief"] || snapshot.CreationSessionUUID != creationSessionUUID {
		t.Fatalf("runtime workflow snapshot=%+v err=%v", snapshot, err)
	}
	var runtimeFinalizations, runtimeStarts, confirmationResults int64
	var finalizationExecutionUUID string
	finalizationQuery := harness.store.DB().Table("agent_tool_executions").Where("run_id=(SELECT id FROM chat_runs WHERE turn_id=(SELECT id FROM chat_turns WHERE uuid=?)) AND json_extract(arguments_json,'$.__route_id')=? AND json_extract(arguments_json,'$.__runtime_generated_bootstrap')=1 AND json_extract(arguments_json,'$.__bootstrap_action')='finalize' AND json_extract(result_json,'$.success')=1", bootstrap.Turn.UUID, RouteProjectSetupFinalize)
	if err := finalizationQuery.Count(&runtimeFinalizations).Error; err != nil {
		t.Fatal(err)
	}
	if err := finalizationQuery.Select("uuid").Take(&finalizationExecutionUUID).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("agent_tool_executions").Where("run_id=(SELECT id FROM chat_runs WHERE turn_id=(SELECT id FROM chat_turns WHERE uuid=?)) AND json_extract(arguments_json,'$.__route_id')=? AND json_extract(arguments_json,'$.__runtime_generated_bootstrap')=1 AND json_extract(arguments_json,'$.__bootstrap_action')='start_generation' AND json_extract(arguments_json,'$.__bootstrap_finalization_execution_uuid')=?", bootstrap.Turn.UUID, RouteYoloWorkflowCreate, finalizationExecutionUUID).Count(&runtimeStarts).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("agent_tool_executions").Where("run_id=(SELECT id FROM chat_runs WHERE turn_id=(SELECT id FROM chat_turns WHERE uuid=?)) AND json_extract(result_json,'$.error.code')=?", bootstrap.Turn.UUID, CodeToolConfirmation).Count(&confirmationResults).Error; err != nil {
		t.Fatal(err)
	}
	if runtimeFinalizations != 1 || runtimeStarts != 1 || confirmationResults != 0 {
		t.Fatalf("runtime bootstrap intents finalization=%d workflow=%d confirmation_results=%d", runtimeFinalizations, runtimeStarts, confirmationResults)
	}
	if _, err := harness.service.CancelWorkflow(ctx, harness.project.UUID, items[0].UUID); err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, bootstrap.Thread.UUID, bootstrap.Turn.UUID, JobChatResume); err != nil {
		t.Fatal(err)
	}
	turns, err := harness.service.ListTurns(ctx, harness.project.UUID, bootstrap.Thread.UUID)
	if err != nil || len(turns) != 1 || turns[0].Status != TurnCompleted {
		t.Fatalf("resumed bootstrap Turn=%+v err=%v", turns, err)
	}
}

func TestBootstrapExplicitLaterTurnRecoversAuthorizedWorkflowWithoutModelCall(t *testing.T) {
	harness := newAgentHarness(t, finalResponse("不应在启动 Workflow 前调用模型。"))
	ctx := context.Background()
	thread := harness.createThread(t)
	firstTurn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "创建一本月光邮差绘本"})
	if err != nil {
		t.Fatal(err)
	}
	firstContext, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, firstTurn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	setupUUID := mustAgentUUID(t)
	makeHarnessProjectDraft(t, harness, setupUUID, "创建一本月光邮差绘本")
	name, language := "月光邮差", "zh-Hans"
	style, brief := "温暖透明水彩", "小狐狸穿过夜色森林，把一封信送给月亮。"
	updated, err := harness.store.UpdateProjectSetupDraft(ctx, project.SetupDraftPatchInput{
		ExpectedRevision: 1, ProjectName: &name, GenerationLanguage: &language,
		OverallStyle: &style, GenerationBrief: &brief,
		PictureBook: &project.PictureBookInput{Format: project.PictureBookVertical},
	})
	if err != nil || updated.Status != project.SetupDraftStatusPendingConfirmation {
		t.Fatalf("updated setup=%+v err=%v", updated, err)
	}
	if _, err := harness.store.FinalizeProjectSetup(ctx, updated.Revision); err != nil {
		t.Fatal(err)
	}
	creationSessionUUID := mustAgentUUID(t)
	firstContext = seedReadyBootstrapYoloAuthorization(t, harness, firstContext, creationSessionUUID)
	now := time.Now().UTC()
	if err := harness.store.DB().Table("chat_runs").Where("id=?", firstContext.Run.ID).Updates(map[string]any{"status": TurnCompleted, "completed_at": now, "updated_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("chat_turns").Where("id=?", firstContext.Turn.ID).Updates(map[string]any{"status": TurnCompleted, "completed_at": now, "updated_at": now}).Error; err != nil {
		t.Fatal(err)
	}

	secondTurn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "继续开始生成"})
	if err != nil {
		t.Fatal(err)
	}
	secondContext, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, secondTurn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if secondContext.BootstrapCreationSessionUUID != "" || secondContext.BootstrapLineageCreationSessionUUID != creationSessionUUID {
		t.Fatalf("later-turn bootstrap context=%+v", secondContext)
	}
	authorizedContext := secondContext
	authorizedContext.BootstrapCreationSessionUUID = creationSessionUUID
	evidence, authorized, err := bootstrapYoloAuthorizationEvidence(ctx, harness.store, authorizedContext, true)
	if err != nil || !authorized {
		t.Fatalf("later-turn finalization evidence=%+v authorized=%v err=%v", evidence, authorized, err)
	}
	executeErr := harness.execute(t, thread.UUID, secondTurn.UUID, JobChatTurn)
	if !errors.Is(executeErr, ErrWaitingWorkflow) {
		var workflowCount, executionCount int64
		var executionState, executionError, executionMessage, executionArguments string
		_ = harness.store.DB().Table("workflows").Where("kind=?", WorkflowYolo).Count(&workflowCount).Error
		_ = harness.store.DB().Table("agent_tool_executions").Where("run_id=?", secondContext.Run.ID).Count(&executionCount).Error
		_ = harness.store.DB().Table("agent_tool_executions").Select("state,error_code,error_message,arguments_json").Where("run_id=?", secondContext.Run.ID).Row().Scan(&executionState, &executionError, &executionMessage, &executionArguments)
		t.Fatalf("later bootstrap recovery did not wait for workflow: err=%v model_calls=%d workflows=%d executions=%d explicit=%v input=%q execution=%s/%s/%s args=%s", executeErr, harness.model.calls, workflowCount, executionCount, bootstrapExplicitRecoveryRequest(secondContext.Turn.InputText), secondContext.Turn.InputText, executionState, executionError, executionMessage, executionArguments)
	}
	if harness.model.calls != 0 {
		t.Fatalf("later bootstrap recovery called model %d times", harness.model.calls)
	}
	var count int64
	var idempotencyKey, snapshotJSON string
	if err := harness.store.DB().Table("workflows").Where("kind=?", WorkflowYolo).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("recovered workflows=%d err=%v", count, err)
	}
	if err := harness.store.DB().Table("workflows").Select("idempotency_key,input_snapshot").Where("kind=?", WorkflowYolo).Row().Scan(&idempotencyKey, &snapshotJSON); err != nil {
		t.Fatal(err)
	}
	var snapshot yoloSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil || idempotencyKey != bootstrapYoloIdempotencyPrefix+creationSessionUUID || snapshot.StoryPrompt != brief || snapshot.CreationSessionUUID != creationSessionUUID {
		t.Fatalf("recovered workflow key=%q snapshot=%+v err=%v", idempotencyKey, snapshot, err)
	}
}

func TestBootstrapExplicitLaterTurnAutoFinalizesPendingSetupAndStartsWorkflow(t *testing.T) {
	harness := newAgentHarness(t, finalResponse("不应在恢复初始化状态机时调用模型。"))
	installProjectSetupDispatcherForTest(t, harness)
	ctx := context.Background()
	setupUUID, creationSessionUUID := mustAgentUUID(t), mustAgentUUID(t)
	makeHarnessProjectDraft(t, harness, setupUUID, "创建一本小狐狸给月亮送信的绘本")
	bootstrap, err := harness.service.BootstrapConversation(ctx, harness.project.UUID, creationSessionUUID, "创建一本小狐狸给月亮送信的绘本", nil)
	if err != nil {
		t.Fatal(err)
	}
	name, language := "月光邮差", "zh-Hans"
	style, brief := "温暖透明水彩", "小狐狸穿过夜色森林，把一封信送给月亮。"
	updated, err := harness.store.UpdateProjectSetupDraft(ctx, project.SetupDraftPatchInput{
		ExpectedRevision: 1, ProjectName: &name, GenerationLanguage: &language,
		OverallStyle: &style, GenerationBrief: &brief,
		PictureBook: &project.PictureBookInput{Format: project.PictureBookVertical},
	})
	if err != nil || updated.Status != project.SetupDraftStatusPendingConfirmation {
		t.Fatalf("pending setup=%+v err=%v", updated, err)
	}
	now := time.Now().UTC()
	if err := harness.store.DB().Table("chat_runs").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", bootstrap.Turn.UUID).Updates(map[string]any{"status": TurnCompleted, "completed_at": now, "updated_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("chat_turns").Where("uuid=?", bootstrap.Turn.UUID).Updates(map[string]any{"status": TurnCompleted, "completed_at": now, "updated_at": now}).Error; err != nil {
		t.Fatal(err)
	}

	recoveryTurn, err := harness.service.CreateTurn(ctx, harness.project.UUID, bootstrap.Thread.UUID, CreateTurnInput{InputText: "继续"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, bootstrap.Thread.UUID, recoveryTurn.UUID, JobChatTurn); !errors.Is(err, ErrWaitingWorkflow) {
		t.Fatalf("pending setup recovery did not continue into the workflow: %v", err)
	}
	if harness.model.calls != 0 {
		t.Fatalf("pending setup recovery called model %d times", harness.model.calls)
	}
	requests, err := harness.service.ListUserInputRequests(ctx, harness.project.UUID, bootstrap.Thread.UUID)
	if err != nil || len(requests) != 0 {
		t.Fatalf("recovered setup unexpectedly requested confirmation=%+v err=%v", requests, err)
	}
	if harness.model.calls != 0 || harness.store.SetupStatus() != project.SetupStatusReady {
		t.Fatalf("recovered setup model_calls=%d setup_status=%s", harness.model.calls, harness.store.SetupStatus())
	}
	var count int64
	var idempotencyKey string
	if err := harness.store.DB().Table("workflows").Where("kind=?", WorkflowYolo).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("recovered workflow count=%d err=%v", count, err)
	}
	if err := harness.store.DB().Table("workflows").Select("idempotency_key").Where("kind=?", WorkflowYolo).Take(&idempotencyKey).Error; err != nil || idempotencyKey != bootstrapYoloIdempotencyPrefix+creationSessionUUID {
		t.Fatalf("recovered workflow idempotency_key=%q err=%v", idempotencyKey, err)
	}
}
