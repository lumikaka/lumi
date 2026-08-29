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

func seedReadyBootstrapYoloAuthorization(t *testing.T, harness *agentHarness, tc toolContext, creationSessionUUID string) toolContext {
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
	binding := dangerousConfirmationBinding{
		Route: RouteProjectSetupFinalize, ProjectUUID: tc.ProjectUUID, TargetUUID: tc.ProjectUUID,
		ExpectedRevision: 1, RequestFingerprint: "sha256:" + strings.Repeat("a", 64),
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
		uuid,project_id,status,revision,original_input,generation_language,field_sources_json,missing_fields_json,
		error_code,error_message,created_at,updated_at
	) VALUES(?,?,'draft',1,?,'zh-Hans','{"generation_language":"system_default"}','["project_name","overall_style","picture_book.format"]','','',?,?)`, setupUUID, projectID, originalInput, now, now).Error; err != nil {
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
				PictureBook        *project.PictureBookInput `json:"picture_book"`
			}
			if decodeErr := json.Unmarshal(encoded, &request); decodeErr != nil {
				return ProjectAPIDispatchResponse{}, decodeErr
			}
			state, err = harness.store.UpdateProjectSetup(ctx, project.SetupPatchInput{
				ExpectedRevision: request.ExpectedRevision, ProjectName: request.ProjectName,
				GenerationLanguage: request.GenerationLanguage, OverallStyle: request.OverallStyle,
				PictureBook: request.PictureBook,
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
	harness := newAgentHarness(t, finalResponse("我会先整理候选设置。"))
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
	reloaded, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil || reloaded.BootstrapCreationSessionUUID != sessionUUID {
		t.Fatalf("bootstrap context=%+v err=%v", reloaded, err)
	}
	reloaded.ToolMode = ToolModeProjectAPI

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
	first, err := executeRequestAPIToolWithUIRef(ctx, harness.service, harness.store, reloaded, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "bootstrap-yolo-first"}, yoloArgs("一只小狐狸替月亮送信。"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := executeRequestAPIToolWithUIRef(ctx, harness.service, harness.store, reloaded, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "bootstrap-yolo-replay"}, yoloArgs("即使故事 Brief 改变也只能返回同一个工作流。"))
	if err != nil {
		t.Fatal(err)
	}
	firstValue, _ := first.Data.(map[string]any)
	secondValue, _ := second.Data.(map[string]any)
	if firstValue["uuid"] == "" || firstValue["uuid"] != secondValue["uuid"] || firstValue["thread_uuid"] != secondValue["thread_uuid"] || firstValue["kind"] != WorkflowYolo || firstValue["presentation_mode"] != string(PresentationDedicatedThread) {
		t.Fatalf("first=%+v second=%+v", firstValue, secondValue)
	}
	if first.UIRef == nil || first.UIRef.Href != "@project/workflows/"+firstValue["uuid"].(string) {
		t.Fatalf("workflow ui_ref=%+v value=%+v", first.UIRef, firstValue)
	}
	var workflowCount int64
	if err := harness.store.DB().Table("workflows").Where("kind=?", WorkflowYolo).Count(&workflowCount).Error; err != nil || workflowCount != 1 {
		t.Fatalf("workflow count=%d err=%v", workflowCount, err)
	}

	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
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

func TestBootstrapYoloAuthorizationRequiresExactConfirmedFinalizationEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *agentHarness, *toolContext)
		want   bool
	}{
		{name: "exact confirmed replay", want: true},
		{name: "safe option", mutate: func(t *testing.T, harness *agentHarness, _ *toolContext) {
			setBootstrapConfirmationAnswer(t, harness, false, false)
		}},
		{name: "Other text", mutate: func(t *testing.T, harness *agentHarness, _ *toolContext) {
			setBootstrapConfirmationAnswer(t, harness, false, true)
		}},
		{name: "cancelled", mutate: func(t *testing.T, harness *agentHarness, _ *toolContext) {
			if err := harness.store.DB().Table("chat_user_input_requests").Where("1=1").Updates(map[string]any{"response_json": nil, "status": "cancelled"}).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "wrong question id", mutate: func(t *testing.T, harness *agentHarness, _ *toolContext) {
			if err := harness.store.DB().Exec(`UPDATE agent_tool_executions SET arguments_json=json_set(arguments_json,'$.confirmation.question_id','confirm_something_else') WHERE tool_name='request_user_input'`).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "wrong target", mutate: func(t *testing.T, harness *agentHarness, _ *toolContext) {
			if err := harness.store.DB().Exec(`UPDATE agent_tool_executions SET arguments_json=json_set(arguments_json,'$.confirmation.target_uuid',?) WHERE tool_name='request_user_input'`, mustAgentUUID(t)).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "request not resumed", mutate: func(t *testing.T, harness *agentHarness, _ *toolContext) {
			if err := harness.store.DB().Table("chat_user_input_requests").Where("1=1").Update("status", "cancelled").Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "replay not runtime generated", mutate: func(t *testing.T, harness *agentHarness, _ *toolContext) {
			if err := harness.store.DB().Exec(`UPDATE agent_tool_executions SET arguments_json=json_remove(arguments_json,'$.__confirmation_auto_replay') WHERE tool_name='request_api'`).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "failed finalization replay", mutate: func(t *testing.T, harness *agentHarness, _ *toolContext) {
			if err := harness.store.DB().Exec(`UPDATE agent_tool_executions SET result_json='{"success":false,"data":null,"error":{"code":"failed"}}' WHERE json_extract(arguments_json,'$.__confirmation_auto_replay')=1`).Error; err != nil {
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

func setBootstrapConfirmationAnswer(t *testing.T, harness *agentHarness, confirm, other bool) {
	t.Helper()
	var requestJSON string
	if err := harness.store.DB().Table("chat_user_input_requests").Select("request_json").Take(&requestJSON).Error; err != nil {
		t.Fatal(err)
	}
	var request struct {
		Questions []UserInputQuestion `json:"questions"`
	}
	if json.Unmarshal([]byte(requestJSON), &request) != nil || len(request.Questions) != 1 || len(request.Questions[0].Options) != 2 {
		t.Fatalf("invalid seeded request: %s", requestJSON)
	}
	selected := request.Questions[0].Options[0].UUID
	otherText := ""
	if confirm {
		selected = request.Questions[0].Options[1].UUID
	}
	if other {
		selected, otherText = "", "只定稿，不启动"
	}
	responseJSON, _ := json.Marshal(map[string]any{"answers": map[string]any{
		bootstrapYoloConfirmationQuestionID: map[string]any{"selected_option_uuid": selected, "other_text": otherText},
	}})
	if err := harness.store.DB().Table("chat_user_input_requests").Where("1=1").Update("response_json", string(responseJSON)).Error; err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapConfirmationAutoFinalizesAndStartsOneYoloWorkflow(t *testing.T) {
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
			"overall_style": "温暖透明水彩，柔和月光", "picture_book": map[string]any{"format": "vertical_strip"},
		},
		"response_filter": ".data | {uuid,setup_status,status,revision,candidate,field_sources,missing_information}",
	}
	finalization := map[string]any{
		"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/project-setup-finalizations",
		"request_body":    map[string]any{"expected_revision": float64(2)},
		"response_filter": ".data | {uuid,setup_status,status,revision,final_picture_book}",
	}
	finalizationRequest, err := parseAgentAPIRequest(toolContext{ProjectUUID: harness.project.UUID, ToolMode: ToolModeProjectAPI}, finalization)
	if err != nil {
		t.Fatal(err)
	}
	confirmation := map[string]any{
		"questions": []map[string]any{{
			"header": "创建确认", "id": bootstrapYoloConfirmationQuestionID,
			"question": "是否定稿当前设置并立即启动只创建首章和首张漫画图的 YOLO？",
			"options": []map[string]any{
				{"label": "继续修改 (Recommended)", "description": "保留草稿，不定稿也不启动。"},
				{"label": "定稿并启动 YOLO", "description": "定稿后启动固定范围的 YOLO。"},
			},
		}},
		"confirmation": map[string]any{
			"route": RouteProjectSetupFinalize, "project_uuid": harness.project.UUID, "target_uuid": harness.project.UUID,
			"expected_revision": float64(2), "request_fingerprint": agentRequestFingerprint(finalizationRequest),
			"question_id": bootstrapYoloConfirmationQuestionID, "confirm_option": float64(1),
		},
	}
	setupGet := map[string]any{
		"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/project-setup",
		"response_filter": ".data | {uuid,setup_status,status,revision,final_picture_book}",
	}
	yoloCreate := map[string]any{
		"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/workflows",
		"request_body":    map[string]any{"story_prompt": "原始需求：小狐狸给月亮送信；温暖水彩，面向儿童，温暖结局。"},
		"response_filter": ".data | {uuid,thread_uuid,presentation_mode,kind,title,status,current_step_key,steps}",
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
			return toolCall("setup-finalize", "request_api", finalization), nil
		case 3:
			if !messagesContain(request.Messages, CodeToolConfirmation) {
				t.Fatal("finalization confirmation error was not returned to the model")
			}
			return toolCall("confirm-and-yolo", "request_user_input", confirmation), nil
		case 4:
			return toolCall("setup-ready-get", "request_api", setupGet), nil
		case 5:
			return toolCall("bootstrap-yolo", "request_api", yoloCreate), nil
		case 6:
			var href string
			for _, message := range request.Messages {
				if message.Role != "tool" {
					continue
				}
				var result struct {
					UIRef *agentUIReference `json:"ui_ref"`
				}
				if json.Unmarshal([]byte(message.Content), &result) == nil && result.UIRef != nil && strings.HasPrefix(result.UIRef.Href, "@project/workflows/") {
					href = result.UIRef.Href
				}
			}
			if !strings.HasPrefix(href, "@project/workflows/") {
				t.Fatalf("YOLO Tool Result did not expose a Workflow ui_ref: %q", href)
			}
			return finalResponse("[YOLO 快速创作](" + href + ")已启动。"), nil
		default:
			t.Fatalf("unexpected model call %d", call)
			return finalResponse("unexpected"), nil
		}
	}

	if err := harness.execute(t, bootstrap.Thread.UUID, bootstrap.Turn.UUID, JobChatTurn); !errors.Is(err, ErrWaitingInput) {
		t.Fatalf("bootstrap did not pause for confirmation: %v", err)
	}
	requests, err := harness.service.ListUserInputRequests(ctx, harness.project.UUID, bootstrap.Thread.UUID)
	if err != nil || len(requests) != 1 || len(requests[0].Questions) != 1 || requests[0].Questions[0].ID != bootstrapYoloConfirmationQuestionID {
		t.Fatalf("confirmation requests=%+v err=%v", requests, err)
	}
	confirmOptionUUID := requests[0].Questions[0].Options[1].UUID
	if _, err := harness.service.RespondUserInput(ctx, harness.project.UUID, bootstrap.Thread.UUID, requests[0].UUID, UserInputResponse{Answers: map[string]UserInputAnswer{
		bootstrapYoloConfirmationQuestionID: {SelectedOptionUUID: confirmOptionUUID},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, bootstrap.Thread.UUID, bootstrap.Turn.UUID, JobChatResume); err != nil {
		t.Fatal(err)
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
	var idempotencyKey string
	if err := harness.store.DB().Table("workflows").Select("idempotency_key").Where("kind=?", WorkflowYolo).Take(&idempotencyKey).Error; err != nil || idempotencyKey != bootstrapYoloIdempotencyPrefix+creationSessionUUID {
		t.Fatalf("idempotency_key=%q err=%v", idempotencyKey, err)
	}
}
