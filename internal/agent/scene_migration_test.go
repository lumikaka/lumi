package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"strings"
	"testing"

	"lumi/internal/files"
	"lumi/internal/imagegen"
	"lumi/internal/llm"
	"lumi/internal/production"
	"lumi/internal/story"
)

func createStoryboardMigrationFixture(t *testing.T, harness *agentHarness) (story.Chapter, production.ComicSection, Thread) {
	t.Helper()
	ctx := context.Background()
	chapter, err := story.NewService(harness.store).CreateChapter(ctx, story.CreateChapterInput{
		ChapterCode: "vol01.ch01", Title: "迁移测试章节", Content: "原始章节正文", ContentFormat: "md",
	})
	if err != nil {
		t.Fatal(err)
	}
	section, err := production.NewService(harness.store, nil).CreateSection(ctx, chapter.UUID, production.CreateSectionInput{
		Title: "迁移测试 Section", StoryboardMD: "# 原始 Storyboard\n\n完整的旧内容。",
	})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{
		Title: "Storyboard Project API 迁移", ProviderUUID: harness.provider.UUID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return chapter, section, thread
}

func TestStoryboardReferenceProjectAPIModeReadsAndReplacesCompleteStoryboard(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	chapter, section, thread := createStoryboardMigrationFixture(t, harness)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "把完整分镜改成雨夜重逢", References: []ReferenceInput{{ResourceType: ReferenceTypeComicSection, ResourceUUID: section.UUID}}})
	if err != nil {
		t.Fatal(err)
	}
	newContent := "# 雨夜重逢\n\n1. 全景：雨幕中的车站。\n2. 近景：两人认出彼此。"
	harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
		tools := definitionNames(request.Tools)
		wantTools := []string{"request_api", "read_agent_doc", "image_gen", "request_user_input"}
		if strings.Join(tools, ",") != strings.Join(wantTools, ",") {
			t.Fatalf("call %d tools=%v want=%v", call, tools, wantTools)
		}
		if len(request.Messages) == 0 || strings.Contains(request.Messages[0].Content, section.UUID) {
			t.Fatalf("comic section reference leaked into system prompt: %+v", request.Messages)
		}
		if !messagesContain(request.Messages[1:], section.UUID) || !messagesContain(request.Messages[1:], "current_turn_references") || !messagesContain(request.Messages[1:], `"page_role":"body"`) {
			t.Fatalf("current Turn did not receive comic section reference data: %+v", request.Messages)
		}
		switch call {
		case 1:
			arguments, _ := json.Marshal(map[string]any{
				"method":          "GET",
				"url":             "/api/v1/projects/" + harness.project.UUID + "/chapters/" + chapter.UUID + "/comic-sections/" + section.UUID,
				"response_filter": ".data | {uuid,page_role,title,current_storyboard,revision}",
			})
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "storyboard-read", Name: "request_api", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
		case 2:
			if !messagesContain(request.Messages, "# 原始 Storyboard") || !messagesContain(request.Messages, `"revision"`) || !messagesContain(request.Messages, `"page_role":"body"`) {
				t.Fatalf("write was selected before reading full fact state: %+v", request.Messages)
			}
			arguments, _ := json.Marshal(map[string]any{
				"method":          "POST",
				"url":             "/api/v1/projects/" + harness.project.UUID + "/chapters/" + chapter.UUID + "/comic-sections/" + section.UUID + "/storyboard-variants",
				"request_body":    map[string]any{"content_md": newContent, "expected_revision": section.Revision},
				"response_filter": ".data | {uuid,page_role,revision}",
			})
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "storyboard-write", Name: "request_api", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
		default:
			return finalResponse("已更新完整 Storyboard。"), nil
		}
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	updated, err := production.NewService(harness.store, nil).GetSection(ctx, chapter.UUID, section.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CurrentStoryboard == nil || updated.CurrentStoryboard.ContentMD != newContent || updated.Revision != section.Revision+1 {
		t.Fatalf("persisted storyboard=%+v want revision=%d content=%q", updated, section.Revision+1, newContent)
	}
	var calls int64
	if err := harness.store.DB().Table("agent_tool_executions").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND tool_name='request_api'", turn.UUID).Count(&calls).Error; err != nil || calls != 2 {
		t.Fatalf("request_api executions=%d err=%v", calls, err)
	}
}

func TestStoryboardReferenceProjectAPIModeKeepsGlobalAccessAndSafety(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	chapter, section, thread := createStoryboardMigrationFixture(t, harness)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "验证全局能力和冲突"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	if value, err := readAgentDoc(tc, map[string]any{"path": storyDocPath}); err != nil || !strings.Contains(value["content"].(string), "`GET /api/v1/projects/{project_uuid}/story-profile`") {
		t.Fatalf("non-recommended Story doc value=%+v err=%v", value, err)
	}
	storyRequest := map[string]any{"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/story-profile", "response_filter": ".data | {uuid}"}
	if value, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "storyboard-non-recommended-read"}, storyRequest); err != nil || value.(map[string]any)["uuid"] == nil {
		t.Fatalf("legal non-recommended route value=%+v err=%v", value, err)
	}
	otherSection, err := production.NewService(harness.store, nil).CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "同项目其他 Section", StoryboardMD: "# 其他事实"})
	if err != nil {
		t.Fatal(err)
	}
	otherRequest := map[string]any{"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters/" + chapter.UUID + "/comic-sections/" + otherSection.UUID, "response_filter": ".data | {uuid}"}
	if value, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "storyboard-other-section-read"}, otherRequest); err != nil || value.(map[string]any)["uuid"] != otherSection.UUID {
		t.Fatalf("same-project non-subject route value=%+v err=%v", value, err)
	}
	write := func(executionKey string, revision int64, content string) error {
		_, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: executionKey}, map[string]any{
			"method":          "POST",
			"url":             "/api/v1/projects/" + harness.project.UUID + "/chapters/" + chapter.UUID + "/comic-sections/" + section.UUID + "/storyboard-variants",
			"request_body":    map[string]any{"content_md": content, "expected_revision": float64(revision)},
			"response_filter": ".data | {uuid,revision}",
		})
		return err
	}
	if err := write("storyboard-first-write", section.Revision, "# 新事实"); err != nil {
		t.Fatal(err)
	}
	if err := write("storyboard-stale-write", section.Revision, "# 不应覆盖"); err == nil {
		t.Fatal("stale storyboard revision was accepted")
	}
	for name, args := range map[string]map[string]any{
		"illegal route":        {"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/not-registered", "response_filter": ".data | {uuid}"},
		"invalid section uuid": {"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters/" + chapter.UUID + "/comic-sections/not-a-uuid", "response_filter": ".data | {uuid}"},
	} {
		if _, err := parseAgentAPIRequest(tc, args); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestStoryboardReferenceCanUseGenericImageGenWithoutAssetBinding(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	_, _, thread := createStoryboardMigrationFixture(t, harness)
	harness.service.WithImageClient(&imageClientFake{response: imagegen.Response{Bytes: agentTestPNG(t), MIMEType: "image/png"}})
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "生成一张构图参考"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	value, err := harness.service.executeImageGenTool(ctx, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), ToolName: "image_gen"}, map[string]any{"prompt": "雨夜车站的构图参考", "size": "1024x1536", "reference_uuids": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if value["purpose"] != "project_chat_image_generation" || len(value["reference_uuids"].([]string)) != 0 || len(value["resolved_file_uuids"].([]string)) != 0 {
		t.Fatalf("storyboard image_gen inherited asset-reference semantics: %+v", value)
	}
}

func TestStoryboardReferenceProjectAPIModeSurvivesUserInputAndFollowUp(t *testing.T) {
	t.Run("user input resume", func(t *testing.T) {
		harness := newAgentHarness(t)
		ctx := context.Background()
		_, _, thread := createStoryboardMigrationFixture(t, harness)
		turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "先确认风格"})
		if err != nil {
			t.Fatal(err)
		}
		harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
			if !containsString(definitionNames(request.Tools), "request_api") || containsString(definitionNames(request.Tools), "get_comic_section") {
				t.Fatalf("call %d lost frozen project API tool mode: %v", call, definitionNames(request.Tools))
			}
			if call == 1 {
				return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "storyboard-input", Name: "request_user_input", Arguments: `{"questions":[{"header":"叙事节奏","id":"story_pace","question":"这次采用哪种叙事节奏？","options":[{"label":"舒缓 (Recommended)","description":"保留更多氛围和情绪铺垫。"},{"label":"紧凑","description":"更快推进关键情节和冲突。"}]}]}`}}}, FinishReason: "tool_calls"}, nil
			}
			return finalResponse("已按选择继续。"), nil
		}
		if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); !errors.Is(err, ErrWaitingInput) {
			t.Fatalf("run did not wait for input: %v", err)
		}
		requests, err := harness.service.ListUserInputRequests(ctx, harness.project.UUID, thread.UUID)
		if err != nil || len(requests) != 1 {
			t.Fatalf("requests=%+v err=%v", requests, err)
		}
		if _, err := harness.service.RespondUserInput(ctx, harness.project.UUID, thread.UUID, requests[0].UUID, UserInputResponse{Answers: map[string]UserInputAnswer{"story_pace": {SelectedOptionUUID: requests[0].Questions[0].Options[0].UUID}}}); err != nil {
			t.Fatal(err)
		}
		if err := harness.execute(t, thread.UUID, turn.UUID, JobChatResume); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("queued follow-up", func(t *testing.T) {
		harness := newAgentHarness(t)
		ctx := context.Background()
		_, _, thread := createStoryboardMigrationFixture(t, harness)
		turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "先分析"})
		if err != nil {
			t.Fatal(err)
		}
		harness.model.onCall = func(call int) {
			if call != 1 {
				return
			}
			if _, err := harness.service.CreateFollowUp(ctx, harness.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "继续细化"}); err != nil {
				t.Fatal(err)
			}
		}
		harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
			if !containsString(definitionNames(request.Tools), "request_api") || containsString(definitionNames(request.Tools), "get_comic_section") {
				t.Fatalf("call %d follow-up lost originating tool mode: %v", call, definitionNames(request.Tools))
			}
			return finalResponse("完成"), nil
		}
		if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
			t.Fatal(err)
		}
		turns, err := harness.service.ListTurns(ctx, harness.project.UUID, thread.UUID)
		if err != nil || len(turns) != 2 || turns[1].SourceType != "follow_up" {
			t.Fatalf("promoted turns=%+v err=%v", turns, err)
		}
		if err := harness.execute(t, thread.UUID, turns[1].UUID, JobChatTurn); err != nil {
			t.Fatal(err)
		}
	})
}

func TestStoryboardReferenceProjectAPIModeRejectsMixedUserInputCalls(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	chapter, section, thread := createStoryboardMigrationFixture(t, harness)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "混合调用不应执行"})
	if err != nil {
		t.Fatal(err)
	}
	apiArgs, _ := json.Marshal(map[string]any{"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters/" + chapter.UUID + "/comic-sections/" + section.UUID, "response_filter": ".data | {uuid,revision}"})
	harness.model.responses = []llm.ChatResponse{{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
		{ID: "ask", Name: "request_user_input", Arguments: `{"input_type":"single_choice","question":"继续吗？","options":[{"label":"继续"},{"label":"取消"}]}`},
		{ID: "read", Name: "request_api", Arguments: string(apiArgs)},
	}}, FinishReason: "tool_calls"}}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	var executions int64
	if err := harness.store.DB().Table("agent_tool_executions").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Count(&executions).Error; err != nil || executions != 0 {
		t.Fatalf("mixed calls persisted executions=%d err=%v", executions, err)
	}
}

func TestPremiseAssetGenerationProjectAPIModeCreatesAssetAfterImage(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	productionService := production.NewService(harness.store, nil)
	premise, err := productionService.GetPremise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	style := "靛蓝水彩、柔和月光、克制暖金点缀"
	if _, err := productionService.UpdatePremise(ctx, production.UpdatePremiseInput{DefaultStyle: style, ExpectedRevision: premise.Revision}); err != nil {
		t.Fatal(err)
	}
	imageClient := &imageClientFake{response: imagegen.Response{Bytes: agentTestColorPNG(t, color.RGBA{R: 60, G: 90, B: 180, A: 255}), MIMEType: "image/png", RevisedPrompt: "孤立的月亮邮局"}}
	harness.service.WithImageClient(imageClient)
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "生成月亮邮局", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "生成一个月亮邮局地点设定"})
	if err != nil {
		t.Fatal(err)
	}
	imagePrompt := style + "；月亮邮局地点设计；纯白、无纹理背景；只出现一个完整主体；主体居中并留有安全边距；无环境场景、文字、边框、拼贴、多视图；孤立地点设计。"
	harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
		wantTools := []string{"request_api", "read_agent_doc", "image_gen", "request_user_input"}
		if got := definitionNames(request.Tools); strings.Join(got, ",") != strings.Join(wantTools, ",") {
			t.Fatalf("call %d tools=%v want=%v", call, got, wantTools)
		}
		if len(request.Messages) == 0 {
			t.Fatal("missing system prompt")
		}
		for _, workflowDetail := range []string{"纯白、无纹理背景", "一个完整主体", "512x512"} {
			if strings.Contains(request.Messages[0].Content, workflowDetail) {
				t.Fatalf("premise asset workflow detail %q leaked into Scene prompt", workflowDetail)
			}
		}
		switch call {
		case 1:
			arguments, _ := json.Marshal(map[string]any{"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/premise", "response_filter": ".data | {default_style}"})
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "premise-read", Name: "request_api", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
		case 2:
			if !messagesContain(request.Messages, style) {
				t.Fatalf("premise default_style was not read before listing assets: %+v", request.Messages)
			}
			arguments, _ := json.Marshal(map[string]any{"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/premise-assets", "response_filter": ".data.items[] | {uuid,asset_type,title}"})
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "asset-list", Name: "request_api", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
		case 3:
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "asset-image", Name: "image_gen", Arguments: `{"prompt":` + mustJSONText(t, imagePrompt) + `,"size":"512x512","reference_uuids":[]}`}}}, FinishReason: "tool_calls"}, nil
		case 4:
			fileUUID := toolResultFileUUID(t, request.Messages)
			arguments, _ := json.Marshal(map[string]any{
				"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/premise-assets",
				"request_body": map[string]any{"file_uuid": fileUUID, "asset_type": "scene", "title": "月亮邮局", "summary": "孤立的月光邮局地点设定", "tags": []string{"月光", "邮局"}}, "response_filter": ".data | {uuid,title,revision}",
			})
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "asset-create", Name: "request_api", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
		default:
			return finalResponse("月亮邮局设定项已创建。"), nil
		}
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	assets, err := productionService.ListPremiseAssets(ctx, "", "active")
	if err != nil || len(assets) != 1 || assets[0].Title != "月亮邮局" || assets[0].CurrentVariant == nil {
		t.Fatalf("created assets=%+v err=%v", assets, err)
	}
	imageClient.mu.Lock()
	requests := append([]imagegen.Request(nil), imageClient.requests...)
	imageClient.mu.Unlock()
	if len(requests) != 1 || requests[0].Size != "512x512" || requests[0].Prompt != imagePrompt || len(requests[0].Images) != 0 {
		t.Fatalf("image requests=%+v", requests)
	}
	var routeCalls int64
	if err := harness.store.DB().Table("agent_tool_executions").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND tool_name='request_api'", turn.UUID).Count(&routeCalls).Error; err != nil || routeCalls != 3 {
		t.Fatalf("request_api calls=%d err=%v", routeCalls, err)
	}
}

func TestProjectAPIImageGenSelectsCurrentTurnReferencesInArgumentOrder(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	firstBytes := agentTestColorPNG(t, color.RGBA{R: 220, A: 255})
	secondBytes := agentTestColorPNG(t, color.RGBA{G: 220, A: 255})
	explicitBytes := agentTestColorPNG(t, color.RGBA{B: 220, A: 255})
	first := createChatReferenceFile(t, harness.store, "first.png", firstBytes)
	second := createChatReferenceFile(t, harness.store, "second.png", secondBytes)
	fileService := files.NewService(harness.store, nil)
	explicitUpload, err := fileService.CreateUpload(ctx, files.CreateUploadInput{Purpose: "project_chatbot_reference", OriginalFilename: "explicit.png", Reader: bytes.NewReader(explicitBytes)})
	if err != nil {
		t.Fatal(err)
	}
	explicitFile, err := fileService.FinalizeUpload(ctx, explicitUpload.UUID, "project_chatbot_reference")
	if err != nil {
		t.Fatal(err)
	}
	imageClient := &imageClientFake{response: imagegen.Response{Bytes: agentTestPNG(t), MIMEType: "image/png"}}
	harness.service.WithImageClient(imageClient)
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Reference 顺序", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "按参考图生成", References: []ReferenceInput{
		{ResourceType: ReferenceTypeFile, ResourceUUID: first.UUID},
		{ResourceType: ReferenceTypeFile, ResourceUUID: second.UUID},
	}})
	if err != nil {
		t.Fatal(err)
	}
	items, err := harness.service.ListItems(ctx, harness.project.UUID, thread.UUID, "", "", 20)
	if err != nil || len(items.Items) != 1 || len(items.Items[0].References) != 2 {
		t.Fatalf("references=%+v err=%v", items.Items, err)
	}
	firstFile, secondFile := items.Items[0].References[0].ResourceUUID, items.Items[0].References[1].ResourceUUID
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	tc.ToolProtocol = ToolProtocolProjectAPI
	value, err := harness.service.executeImageGenTool(ctx, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), ToolName: "image_gen"}, map[string]any{
		"prompt": "Reference 顺序测试", "size": "512x512", "reference_uuids": []any{secondFile, firstFile},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantRefs := []string{secondFile, firstFile}
	if got := value["reference_uuids"].([]string); strings.Join(got, ",") != strings.Join(wantRefs, ",") {
		t.Fatalf("reference order=%v want=%v", got, wantRefs)
	}
	imageClient.mu.Lock()
	requests := append([]imagegen.Request(nil), imageClient.requests...)
	imageClient.mu.Unlock()
	if len(requests) != 1 || len(requests[0].Images) != 2 || !bytes.Equal(requests[0].Images[0].Data, secondBytes) || !bytes.Equal(requests[0].Images[1].Data, firstBytes) {
		t.Fatalf("ordered image inputs=%+v", requests)
	}
	if _, err := harness.service.executeImageGenTool(ctx, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), ToolName: "image_gen"}, map[string]any{"prompt": "越界参考", "reference_uuids": []any{explicitFile.UUID}}); err == nil {
		t.Fatal("image_gen accepted a File that was not a current Turn Reference")
	}
	if _, err := harness.service.executeImageGenTool(ctx, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), ToolName: "image_gen"}, map[string]any{"prompt": "旧参数", "reference_file_uuids": []any{firstFile}}); errorCode(err) != CodeToolValidation {
		t.Fatalf("new Turn accepted the project_api_v2 image argument: %v", err)
	}
	if _, err := harness.service.executeImageGenTool(ctx, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), ToolName: "image_gen"}, map[string]any{"prompt": "通用尺寸", "size": "1024x1024", "reference_uuids": []string{}}); err != nil {
		t.Fatalf("generic image_gen size failed: %v", err)
	}
	imageClient.mu.Lock()
	requests = append([]imagegen.Request(nil), imageClient.requests...)
	imageClient.mu.Unlock()
	if len(requests) != 2 || requests[1].Size != "1024x1024" {
		t.Fatalf("generic image_gen size was not preserved: %+v", requests)
	}
}

func TestPremiseAssetGenerationProjectAPIModeSeparatesImageAndCreateRecovery(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	imageClient := &imageClientFake{response: imagegen.Response{Bytes: agentTestPNG(t), MIMEType: "image/png"}}
	harness.service.WithImageClient(imageClient)
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "创建恢复", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "生成恢复测试"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	imageExecutionUUID := mustAgentUUID(t)
	imageValue, err := harness.service.executeImageGenTool(ctx, harness.store, tc, toolExecutionRecord{UUID: imageExecutionUUID, ToolName: "image_gen"}, map[string]any{"prompt": "纯白背景的单一角色", "size": "512x512", "reference_uuids": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	fileUUID := stringArg(imageValue, "file_uuid")
	collectionURL := "/api/v1/projects/" + harness.project.UUID + "/premise-assets"
	invalidCreate := map[string]any{"method": "POST", "url": collectionURL, "request_body": map[string]any{"file_uuid": fileUUID, "asset_type": "character", "title": ""}, "response_filter": ".data | {uuid}"}
	if _, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "failed-create"}, invalidCreate); err == nil {
		t.Fatal("invalid asset create succeeded after image generation")
	}
	if _, err := files.NewService(harness.store, nil).GetAsset(ctx, fileUUID, false); err != nil {
		t.Fatalf("generated image was lost after create failure: %v", err)
	}
	if assets, err := production.NewService(harness.store, nil).ListPremiseAssets(ctx, "", "active"); err != nil || len(assets) != 0 {
		t.Fatalf("failed create left assets=%+v err=%v", assets, err)
	}
	createExecution := toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "recovered-create"}
	validCreate := map[string]any{"method": "POST", "url": collectionURL, "request_body": map[string]any{"file_uuid": fileUUID, "asset_type": "character", "title": "恢复角色"}, "response_filter": ".data | {uuid,title,revision}"}
	created, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, createExecution, validCreate)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, createExecution, validCreate)
	if err != nil || replayed.(map[string]any)["uuid"] != created.(map[string]any)["uuid"] {
		t.Fatalf("replayed create=%+v created=%+v err=%v", replayed, created, err)
	}
	assets, err := production.NewService(harness.store, nil).ListPremiseAssets(ctx, "", "active")
	if err != nil || len(assets) != 1 {
		t.Fatalf("recovery duplicated assets=%+v err=%v", assets, err)
	}
	replayedImage, err := harness.service.executeImageGenTool(ctx, harness.store, tc, toolExecutionRecord{UUID: imageExecutionUUID, ToolName: "image_gen"}, map[string]any{"prompt": "不会再次执行", "size": "512x512", "reference_uuids": []string{}})
	if err != nil || stringArg(replayedImage, "file_uuid") != fileUUID {
		t.Fatalf("replayed image=%+v err=%v", replayedImage, err)
	}
	imageClient.mu.Lock()
	callCount := len(imageClient.requests)
	imageClient.mu.Unlock()
	if callCount != 1 {
		t.Fatalf("image generation repeated %d times", callCount)
	}
}

func createAssetReferenceMigrationFixture(t *testing.T, harness *agentHarness) (production.PremiseAsset, Thread) {
	t.Helper()
	ctx := context.Background()
	productionService := production.NewService(harness.store, nil)
	upload, err := productionService.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "source.png", Reader: bytes.NewReader(agentTestColorPNG(t, color.RGBA{R: 180, G: 60, B: 80, A: 255}))})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := productionService.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: upload.UUID, AssetType: production.AssetCharacter, Title: "月光邮差", Summary: "银发、蓝色制服", Tags: []string{"courier"}})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "设定项引用迁移", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	return asset, thread
}

func TestProjectAssistantReadsGuideAndAPIContractBeforeCreatingAsset(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	source, _ := createAssetReferenceMigrationFixture(t, harness)
	imageClient := &imageClientFake{response: imagegen.Response{Bytes: agentTestColorPNG(t, color.RGBA{R: 210, G: 180, B: 240, A: 255}), MIMEType: "image/png", RevisedPrompt: "derived starlight courier"}}
	harness.service.WithImageClient(imageClient)
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "通用助手派生设定", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "参考现有月光邮差，创建星光分拣员", References: []ReferenceInput{{ResourceType: ReferenceTypePremiseAsset, ResourceUUID: source.UUID}}})
	if err != nil {
		t.Fatal(err)
	}
	guidePath := agentDocBasePath + "/guides/创建设定资产.md"
	harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
		if got := definitionNames(request.Tools); strings.Join(got, ",") != strings.Join(projectAPIToolNames, ",") {
			t.Fatalf("call %d tools=%v want=%v", call, got, projectAPIToolNames)
		}
		switch call {
		case 1:
			arguments, _ := json.Marshal(map[string]any{"path": guidePath})
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "read-create-guide", Name: "read_agent_doc", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
		case 2:
			if !messagesContain(request.Messages, "不能直接充当新资产文件") || !messagesContain(request.Messages, "当前 Turn Reference") {
				t.Fatalf("create Guide was not returned before API Contract: %+v", request.Messages)
			}
			arguments, _ := json.Marshal(map[string]any{"path": premiseAssetDocPath})
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "read-create-contract", Name: "read_agent_doc", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
		case 3:
			if !messagesContain(request.Messages, "`POST /api/v1/projects/{project_uuid}/premise-assets`") || !messagesContain(request.Messages, "file_uuid") {
				t.Fatalf("premise asset API Contract was not returned before image generation: %+v", request.Messages)
			}
			arguments, _ := json.Marshal(map[string]any{"prompt": "参考月光邮差的星光分拣员，保持同一视觉语言", "reference_uuids": []string{source.UUID}, "size": "512x512"})
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "derive-image", Name: "image_gen", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
		case 4:
			fileUUID := toolResultFileUUID(t, request.Messages)
			arguments, _ := json.Marshal(map[string]any{
				"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/premise-assets",
				"request_body":    map[string]any{"file_uuid": fileUUID, "asset_type": "character", "title": "星光分拣员", "summary": "由月光邮差视觉语言派生"},
				"response_filter": ".data | {uuid,title,revision}",
			})
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "create-derived-asset", Name: "request_api", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
		default:
			return finalResponse("已创建星光分拣员。"), nil
		}
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	assets, err := production.NewService(harness.store, nil).ListPremiseAssets(ctx, "", "active")
	if err != nil || len(assets) != 2 {
		t.Fatalf("derived assets=%+v err=%v", assets, err)
	}
	found := false
	for _, asset := range assets {
		found = found || asset.Title == "星光分拣员"
	}
	if !found {
		t.Fatalf("derived asset missing: %+v", assets)
	}
	imageClient.mu.Lock()
	requests := append([]imagegen.Request(nil), imageClient.requests...)
	imageClient.mu.Unlock()
	if len(requests) != 1 || requests[0].Size != "512x512" || len(requests[0].Images) != 1 {
		t.Fatalf("derived image request=%+v", requests)
	}
}

func TestPremiseAssetCreateReportsPreciseFileSourceErrors(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	source, _ := createAssetReferenceMigrationFixture(t, harness)
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "来源校验", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "验证图片来源"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	collectionURL := "/api/v1/projects/" + harness.project.UUID + "/premise-assets"
	createWithFile := func(key, fileUUID, title string) (any, error) {
		t.Helper()
		return executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: key}, map[string]any{
			"method": "POST", "url": collectionURL,
			"request_body":    map[string]any{"file_uuid": fileUUID, "asset_type": "character", "title": title},
			"response_filter": ".data | {uuid,title,revision}",
		})
	}
	if _, err := createWithFile("existing-asset-file", source.CurrentVariant.Asset.UUID, "不应创建"); err == nil || productionErrorCode(err) != production.CodeValidation || strings.Contains(err.Error(), "不存在") {
		t.Fatalf("existing Premise image source error=%v", err)
	}
	if _, err := createWithFile("missing-file", mustAgentUUID(t), "不存在"); err == nil || productionErrorCode(err) != production.CodeNotFound {
		t.Fatalf("missing file source error=%v", err)
	}
	imageClient := &imageClientFake{response: imagegen.Response{Bytes: agentTestPNG(t), MIMEType: "image/png"}}
	harness.service.WithImageClient(imageClient)
	generated, err := harness.service.executeImageGenTool(ctx, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), ToolName: "image_gen"}, map[string]any{"prompt": "有效的新设定图", "reference_uuids": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	fileUUID := stringArg(generated, "file_uuid")
	if _, err := createWithFile("first-consumer", fileUUID, "有效创建"); err != nil {
		t.Fatal(err)
	}
	if _, err := createWithFile("second-consumer", fileUUID, "不应复用"); err == nil || productionErrorCode(err) != production.CodeStateConflict {
		t.Fatalf("consumed file source error=%v", err)
	}
	assets, err := production.NewService(harness.store, nil).ListPremiseAssets(ctx, "", "active")
	if err != nil || len(assets) != 2 {
		t.Fatalf("invalid source created an asset: assets=%+v err=%v", assets, err)
	}
}

func TestAssetReferenceProjectAPIModePreservesReferenceOrderAndMutationSemantics(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	source, thread := createAssetReferenceMigrationFixture(t, harness)
	productionService := production.NewService(harness.store, nil)
	attachmentBytes := agentTestColorPNG(t, color.RGBA{G: 200, A: 255})
	explicitBytes := agentTestColorPNG(t, color.RGBA{B: 200, A: 255})
	attachmentFile := createChatReferenceFile(t, harness.store, "attachment.png", attachmentBytes)
	explicitFile := createChatReferenceFile(t, harness.store, "explicit.png", explicitBytes)
	imageClient := &imageClientFake{response: imagegen.Response{Bytes: agentTestColorPNG(t, color.RGBA{R: 100, G: 120, B: 220, A: 255}), MIMEType: "image/png"}}
	harness.service.WithImageClient(imageClient)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "保持身份和画风，换成冬季制服", References: []ReferenceInput{
		{ResourceType: ReferenceTypePremiseAsset, ResourceUUID: source.UUID},
		{ResourceType: ReferenceTypeFile, ResourceUUID: attachmentFile.UUID},
		{ResourceType: ReferenceTypeFile, ResourceUUID: explicitFile.UUID},
	}})
	if err != nil {
		t.Fatal(err)
	}
	items, err := harness.service.ListItems(ctx, harness.project.UUID, thread.UUID, "", "", 20)
	if err != nil || len(items.Items) != 1 || len(items.Items[0].References) != 3 {
		t.Fatalf("turn references=%+v err=%v", items.Items, err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	wantTools := []string{"request_api", "read_agent_doc", "image_gen", "request_user_input"}
	if got := definitionNames(llmToolDefinitionsForMode(tc.Thread, tc.ToolMode)); strings.Join(got, ",") != strings.Join(wantTools, ",") {
		t.Fatalf("asset reference tools=%v want=%v", got, wantTools)
	}
	firstImageExecution := toolExecutionRecord{UUID: mustAgentUUID(t), ToolName: "image_gen"}
	firstImage, err := harness.service.executeImageGenTool(ctx, harness.store, tc, firstImageExecution, map[string]any{
		"prompt": "保持月光邮差的银发身份与蓝色制服视觉语言，改为冬季版本", "reference_uuids": []any{source.UUID, attachmentFile.UUID, explicitFile.UUID},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantReferences := []string{source.UUID, attachmentFile.UUID, explicitFile.UUID}
	if got := firstImage["reference_uuids"].([]string); strings.Join(got, ",") != strings.Join(wantReferences, ",") {
		t.Fatalf("asset reference order=%v want=%v", got, wantReferences)
	}
	imageClient.mu.Lock()
	imageRequests := append([]imagegen.Request(nil), imageClient.requests...)
	imageClient.mu.Unlock()
	if len(imageRequests) != 1 || len(imageRequests[0].Images) != 3 {
		t.Fatalf("image requests=%+v", imageRequests)
	}
	assetURL := "/api/v1/projects/" + harness.project.UUID + "/premise-assets/" + source.UUID
	if value, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "asset-current-read"}, map[string]any{"method": "GET", "url": assetURL, "response_filter": ".data | {uuid,revision}"}); err != nil || value.(map[string]any)["uuid"] != source.UUID {
		t.Fatalf("current asset read=%+v err=%v", value, err)
	}
	updatedValue, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "asset-current-update"}, map[string]any{
		"method": "PATCH", "url": assetURL,
		"request_body":    map[string]any{"expected_revision": float64(source.Revision), "file_uuid": stringArg(firstImage, "file_uuid"), "title": "月光邮差·冬季"},
		"response_filter": ".data | {uuid,title,revision}",
	})
	if err != nil {
		t.Fatal(err)
	}
	updatedRevision := intArg(updatedValue.(map[string]any), "revision")
	if updatedValue.(map[string]any)["title"] != "月光邮差·冬季" || updatedRevision != source.Revision+1 {
		t.Fatalf("updated asset=%+v", updatedValue)
	}
	if _, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "asset-stale-update"}, map[string]any{
		"method": "PATCH", "url": assetURL, "request_body": map[string]any{"expected_revision": float64(source.Revision), "title": "不应覆盖"}, "response_filter": ".data | {uuid,revision}",
	}); err == nil {
		t.Fatal("stale asset revision was accepted")
	}
	secondImage, err := harness.service.executeImageGenTool(ctx, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), ToolName: "image_gen"}, map[string]any{"prompt": "保持同一画风，创建月光邮差的搭档", "reference_uuids": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	createdValue, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "asset-derived-create"}, map[string]any{
		"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/premise-assets",
		"request_body":    map[string]any{"file_uuid": stringArg(secondImage, "file_uuid"), "asset_type": "character", "title": "星光分拣员"},
		"response_filter": ".data | {uuid,title,revision}",
	})
	if err != nil || createdValue.(map[string]any)["uuid"] == source.UUID {
		t.Fatalf("derived asset=%+v err=%v", createdValue, err)
	}
	afterDerived, err := productionService.GetPremiseAsset(ctx, source.UUID)
	if err != nil || afterDerived.Title != "月光邮差·冬季" || afterDerived.Revision != updatedRevision {
		t.Fatalf("derived creation mutated source=%+v err=%v", afterDerived, err)
	}
	otherUpload, err := productionService.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "other.png", Reader: bytes.NewReader(agentTestPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	other, err := productionService.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: otherUpload.UUID, AssetType: production.AssetProp, Title: "同项目其他道具"})
	if err != nil {
		t.Fatal(err)
	}
	if value, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "asset-other-read"}, map[string]any{"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/premise-assets/" + other.UUID, "response_filter": ".data | {uuid}"}); err != nil || value.(map[string]any)["uuid"] != other.UUID {
		t.Fatalf("same-project other asset=%+v err=%v", value, err)
	}
	if _, err := parseAgentAPIRequest(tc, map[string]any{"method": "GET", "url": "/api/v1/projects/" + mustAgentUUID(t) + "/premise-assets/" + other.UUID, "response_filter": ".data | {uuid}"}); err == nil || errorCode(err) != CodeToolNotAllowed {
		t.Fatalf("cross-project asset path accepted: %v", err)
	}
}

func TestDangerousAgentAPIRouteRequiresExplicitOrBoundConfirmation(t *testing.T) {
	t.Run("explicit delete intent", func(t *testing.T) {
		harness := newAgentHarness(t)
		ctx := context.Background()
		asset, thread := createAssetReferenceMigrationFixture(t, harness)
		turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "请删除当前设定项，把它移入回收站"})
		if err != nil {
			t.Fatal(err)
		}
		tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
		if err != nil {
			t.Fatal(err)
		}
		tc.ToolMode = ToolModeProjectAPI
		value, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "explicit-soft-delete"}, map[string]any{
			"method": "DELETE", "url": "/api/v1/projects/" + harness.project.UUID + "/premise-assets/" + asset.UUID,
			"request_body": map[string]any{"expected_revision": float64(asset.Revision)}, "response_filter": ".data | {uuid,deleted_at}",
		})
		if err != nil || value.(map[string]any)["deleted_at"] == nil {
			t.Fatalf("explicit soft delete value=%+v err=%v", value, err)
		}
	})

	t.Run("ambiguous intent and resume", func(t *testing.T) {
		harness := newAgentHarness(t)
		harness.service.turnBudget.MaxModelRequests = 4
		ctx := context.Background()
		asset, thread := createAssetReferenceMigrationFixture(t, harness)
		turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "这个设定项似乎没用了，应该怎么处理？", MaxSteps: 3})
		if err != nil {
			t.Fatal(err)
		}
		assetURL := "/api/v1/projects/" + harness.project.UUID + "/premise-assets/" + asset.UUID
		deleteRequestArguments := map[string]any{"method": "DELETE", "url": assetURL, "request_body": map[string]any{"expected_revision": float64(asset.Revision)}, "response_filter": ".data | {uuid,deleted_at}"}
		deleteRequest, err := parseAgentAPIRequest(toolContext{ProjectUUID: harness.project.UUID, Thread: threadRecord{Scope: ThreadScopePremise, Scene: SceneAssetReference, SubjectUUID: asset.UUID}, ToolMode: ToolModeProjectAPI}, deleteRequestArguments)
		if err != nil {
			t.Fatal(err)
		}
		deleteArguments, _ := json.Marshal(deleteRequestArguments)
		confirmationArguments, _ := json.Marshal(map[string]any{
			"questions":    []map[string]any{{"header": "删除确认", "id": "delete_asset", "question": "是否将当前设定项移入回收站？", "options": []map[string]any{{"label": "保留设定项 (Recommended)", "description": "不执行删除并保留当前设定项。"}, {"label": "确认移入回收站", "description": "执行已绑定到当前版本的删除操作。"}}}},
			"confirmation": map[string]any{"route": RoutePremiseAssetDelete, "project_uuid": harness.project.UUID, "target_uuid": asset.UUID, "expected_revision": asset.Revision, "request_fingerprint": agentRequestFingerprint(deleteRequest), "question_id": "delete_asset", "confirm_option": 1},
		})
		harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
			if call < 4 && (!containsString(definitionNames(request.Tools), "request_api") || containsString(definitionNames(request.Tools), currentProjectAPIToolName)) {
				t.Fatalf("call %d dangerous flow lost project API mode: %v", call, definitionNames(request.Tools))
			}
			switch call {
			case 1:
				arguments, _ := json.Marshal(map[string]any{"method": "GET", "url": assetURL, "response_filter": ".data | {uuid,title,revision}"})
				return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "danger-read", Name: "request_api", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
			case 2:
				return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "danger-unconfirmed", Name: "request_api", Arguments: string(deleteArguments)}}}, FinishReason: "tool_calls"}, nil
			case 3:
				if !messagesContain(request.Messages, CodeToolConfirmation) {
					t.Fatalf("dangerous route did not return confirmation-required error: %+v", request.Messages)
				}
				return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "danger-confirm", Name: "request_user_input", Arguments: string(confirmationArguments)}}}, FinishReason: "tool_calls"}, nil
			case 4:
				if len(request.Tools) != 0 || !messagesContain(request.Messages, `"deleted_at"`) {
					t.Fatalf("confirmation resume did not use a tool-free finalization after automatic replay: tools=%v messages=%+v", definitionNames(request.Tools), request.Messages)
				}
				return finalResponse("设定项已移入回收站。"), nil
			default:
				return finalResponse("设定项已移入回收站。"), nil
			}
		}
		if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); !errors.Is(err, ErrWaitingInput) {
			t.Fatalf("ambiguous delete did not pause: %v", err)
		}
		before, err := production.NewService(harness.store, nil).GetPremiseAsset(ctx, asset.UUID)
		if err != nil || before.DeletedAt != nil {
			t.Fatalf("asset changed before confirmation=%+v err=%v", before, err)
		}
		requests, err := harness.service.ListUserInputRequests(ctx, harness.project.UUID, thread.UUID)
		if err != nil || len(requests) != 1 || len(requests[0].Questions) != 1 || !strings.Contains(requests[0].Questions[0].Question, RoutePremiseAssetDelete) || !strings.Contains(requests[0].Questions[0].Question, asset.UUID) {
			t.Fatalf("bound confirmation request=%+v err=%v", requests, err)
		}
		if _, err := harness.service.RespondUserInput(ctx, harness.project.UUID, thread.UUID, requests[0].UUID, UserInputResponse{Answers: map[string]UserInputAnswer{"delete_asset": {SelectedOptionUUID: requests[0].Questions[0].Options[1].UUID}}}); err != nil {
			t.Fatal(err)
		}
		tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
		if err != nil {
			t.Fatal(err)
		}
		tc.ToolMode = ToolModeProjectAPI
		if _, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "mismatched-confirmation"}, map[string]any{
			"method": "DELETE", "url": assetURL, "request_body": map[string]any{"expected_revision": float64(asset.Revision + 1)},
			"response_filter": ".data | {uuid,deleted_at}",
		}); err == nil || errorCode(err) != CodeToolConfirmation {
			t.Fatalf("mismatched confirmation binding accepted: %v", err)
		}
		if err := harness.execute(t, thread.UUID, turn.UUID, JobChatResume); err != nil {
			t.Fatal(err)
		}
		if repeated, err := harness.service.RespondUserInput(ctx, harness.project.UUID, thread.UUID, requests[0].UUID, UserInputResponse{Answers: map[string]UserInputAnswer{"delete_asset": {SelectedOptionUUID: requests[0].Questions[0].Options[1].UUID}}}); err != nil || repeated.Status != "resumed" {
			t.Fatalf("repeated confirmation answer was not idempotent: request=%+v err=%v", repeated, err)
		}
		deleted, err := production.NewService(harness.store, nil).GetPremiseAsset(ctx, asset.UUID)
		if err != nil || deleted.DeletedAt == nil {
			t.Fatalf("confirmed asset was not soft-deleted=%+v err=%v", deleted, err)
		}
		if replayAllowed, err := hasMatchingDangerousConfirmation(ctx, harness.store, tc, deleteRequest); err != nil || replayAllowed {
			t.Fatalf("completed dangerous request did not consume its confirmation: allowed=%v err=%v", replayAllowed, err)
		}
		var automaticReplays int64
		if err := harness.store.DB().Table("agent_tool_executions").Where("run_id=? AND json_extract(arguments_json,'$.__confirmation_auto_replay')=1", tc.Run.ID).Count(&automaticReplays).Error; err != nil || automaticReplays != 1 {
			t.Fatalf("automatic confirmation replays=%d err=%v", automaticReplays, err)
		}
	})

	t.Run("safe choice does not replay", func(t *testing.T) {
		harness := newAgentHarness(t)
		harness.service.turnBudget.MaxModelRequests = 3
		ctx := context.Background()
		asset, thread := createAssetReferenceMigrationFixture(t, harness)
		turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "这个设定项似乎没用了，应该怎么处理？", MaxSteps: 2})
		if err != nil {
			t.Fatal(err)
		}
		assetURL := "/api/v1/projects/" + harness.project.UUID + "/premise-assets/" + asset.UUID
		deleteRequestArguments := map[string]any{"method": "DELETE", "url": assetURL, "request_body": map[string]any{"expected_revision": float64(asset.Revision)}, "response_filter": ".data | {uuid,deleted_at}"}
		deleteRequest, err := parseAgentAPIRequest(toolContext{ProjectUUID: harness.project.UUID, ToolMode: ToolModeProjectAPI}, deleteRequestArguments)
		if err != nil {
			t.Fatal(err)
		}
		deleteArguments, _ := json.Marshal(deleteRequestArguments)
		confirmationArguments, _ := json.Marshal(map[string]any{
			"questions":    []map[string]any{{"header": "删除确认", "id": "delete_asset", "question": "是否将当前设定项移入回收站？", "options": []map[string]any{{"label": "保留设定项 (Recommended)", "description": "不执行删除并保留当前设定项。"}, {"label": "确认移入回收站", "description": "执行已绑定到当前版本的删除操作。"}}}},
			"confirmation": map[string]any{"route": RoutePremiseAssetDelete, "project_uuid": harness.project.UUID, "target_uuid": asset.UUID, "expected_revision": asset.Revision, "request_fingerprint": agentRequestFingerprint(deleteRequest), "question_id": "delete_asset", "confirm_option": 1},
		})
		harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
			switch call {
			case 1:
				return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "safe-unconfirmed", Name: "request_api", Arguments: string(deleteArguments)}}}, FinishReason: "tool_calls"}, nil
			case 2:
				return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "safe-confirm", Name: "request_user_input", Arguments: string(confirmationArguments)}}}, FinishReason: "tool_calls"}, nil
			case 3:
				if len(request.Tools) != 0 {
					t.Fatalf("safe response finalization still exposed tools: %v", definitionNames(request.Tools))
				}
				return finalResponse("已保留设定项。"), nil
			default:
				return llm.ChatResponse{}, fmt.Errorf("unexpected model call %d", call)
			}
		}
		if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); !errors.Is(err, ErrWaitingInput) {
			t.Fatalf("safe flow did not pause: %v", err)
		}
		requests, err := harness.service.ListUserInputRequests(ctx, harness.project.UUID, thread.UUID)
		if err != nil || len(requests) != 1 {
			t.Fatalf("safe requests=%+v err=%v", requests, err)
		}
		if _, err := harness.service.RespondUserInput(ctx, harness.project.UUID, thread.UUID, requests[0].UUID, UserInputResponse{Answers: map[string]UserInputAnswer{"delete_asset": {SelectedOptionUUID: requests[0].Questions[0].Options[0].UUID}}}); err != nil {
			t.Fatal(err)
		}
		if err := harness.execute(t, thread.UUID, turn.UUID, JobChatResume); err != nil {
			t.Fatal(err)
		}
		current, err := production.NewService(harness.store, nil).GetPremiseAsset(ctx, asset.UUID)
		if err != nil || current.DeletedAt != nil {
			t.Fatalf("safe choice changed asset=%+v err=%v", current, err)
		}
		var automaticReplays int64
		if err := harness.store.DB().Table("agent_tool_executions").Where("json_extract(arguments_json,'$.__confirmation_auto_replay')=1").Count(&automaticReplays).Error; err != nil || automaticReplays != 0 {
			t.Fatalf("safe choice automatic replays=%d err=%v", automaticReplays, err)
		}
	})

	t.Run("storyboard generation replays the persisted request", func(t *testing.T) {
		harness := newAgentHarness(t)
		harness.service.turnBudget.MaxModelRequests = 3
		ctx := context.Background()
		chapter, err := story.NewService(harness.store).CreateChapter(ctx, story.CreateChapterInput{
			ChapterCode: "vol03.ch01", Title: "自动确认分镜", Content: "雨夜重逢。", ContentFormat: "md",
		})
		if err != nil {
			t.Fatal(err)
		}
		thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "生成漫画分镜", ProviderUUID: harness.provider.UUID})
		if err != nil {
			t.Fatal(err)
		}
		turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "为这个章节生成漫画分镜。", MaxSteps: 2})
		if err != nil {
			t.Fatal(err)
		}
		requestArguments := map[string]any{
			"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/chapters/" + chapter.UUID + "/comic-storyboard-generations",
			"request_body":    map[string]any{"prompt": "按雨夜重逢规划完整漫画分镜", "max_section_count": float64(6)},
			"response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}",
		}
		request, err := parseAgentAPIRequest(toolContext{ProjectUUID: harness.project.UUID, ToolMode: ToolModeProjectAPI}, requestArguments)
		if err != nil {
			t.Fatal(err)
		}
		requestJSON, _ := json.Marshal(requestArguments)
		confirmationJSON, _ := json.Marshal(map[string]any{
			"questions": []map[string]any{{"header": "生成确认", "id": "generate_storyboard", "question": "是否创建漫画分镜规划任务？", "options": []map[string]any{{"label": "暂不生成 (Recommended)", "description": "保留当前章节，不创建任务。"}, {"label": "确认生成", "description": "按已绑定参数创建分镜任务。"}}}},
			"confirmation": map[string]any{
				"route": RouteComicStoryboardGenerationCreate, "project_uuid": harness.project.UUID, "target_uuid": chapter.UUID,
				"expected_revision": int64(0), "request_fingerprint": agentRequestFingerprint(request), "question_id": "confirm_comic_storyboard_gen", "confirm_option": 1,
			},
		})
		harness.model.respond = func(call int, modelRequest llm.ChatRequest) (llm.ChatResponse, error) {
			switch call {
			case 1:
				return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "storyboard-generate", Name: "request_api", Arguments: string(requestJSON)}}}, FinishReason: "tool_calls"}, nil
			case 2:
				if !messagesContain(modelRequest.Messages, CodeToolConfirmation) {
					t.Fatal("storyboard generation confirmation error missing from context")
				}
				return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "storyboard-confirm", Name: "request_user_input", Arguments: string(confirmationJSON)}}}, FinishReason: "tool_calls"}, nil
			case 3:
				if len(modelRequest.Tools) != 0 || !messagesContain(modelRequest.Messages, `"status":"queued"`) {
					t.Fatalf("storyboard finalization did not observe queued task: tools=%v messages=%+v", definitionNames(modelRequest.Tools), modelRequest.Messages)
				}
				return finalResponse("漫画分镜规划任务已创建。"), nil
			default:
				return llm.ChatResponse{}, fmt.Errorf("unexpected storyboard model call %d", call)
			}
		}
		if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); !errors.Is(err, ErrWaitingInput) {
			t.Fatalf("storyboard generation did not wait for confirmation: %v", err)
		}
		if len(harness.queue.requests) != 0 {
			t.Fatalf("storyboard task started before confirmation: %+v", harness.queue.requests)
		}
		requests, err := harness.service.ListUserInputRequests(ctx, harness.project.UUID, thread.UUID)
		if err != nil || len(requests) != 1 {
			t.Fatalf("storyboard confirmation requests=%+v err=%v", requests, err)
		}
		var rawQuestionID, storedQuestionID, argumentRepair string
		if err := harness.store.DB().Raw(`SELECT json_extract(items.content,'$.confirmation.question_id'),json_extract(executions.arguments_json,'$.confirmation.question_id'),json_extract(items.metadata_json,'$.argument_repaired')
				FROM chat_items AS items
				JOIN agent_tool_executions AS executions ON executions.item_id=items.id
				WHERE items.turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND items.tool_name='request_user_input'`, turn.UUID).Row().Scan(&rawQuestionID, &storedQuestionID, &argumentRepair); err != nil {
			t.Fatal(err)
		}
		if rawQuestionID != "confirm_comic_storyboard_gen" || storedQuestionID != "generate_storyboard" || argumentRepair != confirmationQuestionIDArgumentRepair {
			t.Fatalf("storyboard confirmation normalization raw=%q stored=%q repair=%q", rawQuestionID, storedQuestionID, argumentRepair)
		}
		if _, err := harness.service.RespondUserInput(ctx, harness.project.UUID, thread.UUID, requests[0].UUID, UserInputResponse{Answers: map[string]UserInputAnswer{"generate_storyboard": {SelectedOptionUUID: requests[0].Questions[0].Options[1].UUID}}}); err != nil {
			t.Fatal(err)
		}
		if err := harness.execute(t, thread.UUID, turn.UUID, JobChatResume); err != nil {
			t.Fatal(err)
		}
		if len(harness.queue.requests) != 1 || harness.queue.requests[0].Kind != WorkflowComicStoryboard || harness.queue.requests[0].ResourceUUID != chapter.UUID {
			t.Fatalf("storyboard automatic replay requests=%+v", harness.queue.requests)
		}
		var run runRecord
		if err := harness.store.DB().Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Take(&run).Error; err != nil || run.Status != TurnCompleted || run.ModelRequestCount != run.MaxModelRequests || run.LimitReason != BudgetReasonModelRequests {
			t.Fatalf("storyboard finalization run=%+v err=%v", run, err)
		}
	})
}

func TestDangerousAgentAPIRoutePolicyIsSceneIndependent(t *testing.T) {
	tests := []struct {
		name  string
		scene string
	}{
		{name: "project assistant", scene: legacySceneProjectAssistant},
		{name: "premise asset generation", scene: ScenePremiseAsset},
		{name: "asset reference", scene: SceneAssetReference},
		{name: "storyboard reference", scene: SceneStoryboardReference},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newAgentHarness(t)
			ctx := context.Background()
			asset, assetThread := createAssetReferenceMigrationFixture(t, harness)
			thread := assetThread
			switch test.scene {
			case legacySceneProjectAssistant:
				var err error
				thread, err = harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: test.name, ProviderUUID: harness.provider.UUID})
				if err != nil {
					t.Fatal(err)
				}
			case ScenePremiseAsset:
				var err error
				thread, err = harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: test.name, ProviderUUID: harness.provider.UUID})
				if err != nil {
					t.Fatal(err)
				}
			case SceneStoryboardReference:
				var err error
				thread, err = harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: test.name, ProviderUUID: harness.provider.UUID})
				if err != nil {
					t.Fatal(err)
				}
			}
			turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "这个设定似乎没用了，应该怎么处理？"})
			if err != nil {
				t.Fatal(err)
			}
			tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
			if err != nil {
				t.Fatal(err)
			}
			tc.ToolMode = ToolModeProjectAPI
			_, err = executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "uniform-dangerous-policy-" + test.scene}, map[string]any{
				"method": "DELETE", "url": "/api/v1/projects/" + harness.project.UUID + "/premise-assets/" + asset.UUID,
				"request_body":    map[string]any{"expected_revision": float64(asset.Revision)},
				"response_filter": ".data | {uuid,deleted_at}",
			})
			if err == nil || errorCode(err) != CodeToolConfirmation {
				t.Fatalf("scene %s did not apply the global confirmation policy: %v", test.scene, err)
			}
			current, getErr := production.NewService(harness.store, nil).GetPremiseAsset(ctx, asset.UUID)
			if getErr != nil || current.DeletedAt != nil {
				t.Fatalf("scene %s mutated the asset before confirmation: %+v err=%v", test.scene, current, getErr)
			}
		})
	}
}

func TestProjectAPIToolModeSnapshotSurvivesSteeringForEveryScene(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	_, assetThread := createAssetReferenceMigrationFixture(t, harness)
	_, _, storyboardThread := createStoryboardMigrationFixture(t, harness)
	projectThread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Project snapshot", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	premiseThread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Premise snapshot", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		scene  string
		thread Thread
	}{
		{scene: legacySceneProjectAssistant, thread: projectThread},
		{scene: ScenePremiseAsset, thread: premiseThread},
		{scene: SceneAssetReference, thread: assetThread},
		{scene: SceneStoryboardReference, thread: storyboardThread},
	} {
		t.Run(test.scene, func(t *testing.T) {
			turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, test.thread.UUID, CreateTurnInput{InputText: "冻结 Project API 模式"})
			if err != nil {
				t.Fatal(err)
			}
			tc, err := harness.service.loadToolContext(ctx, harness.store, test.thread.UUID, turn.UUID)
			if err != nil {
				t.Fatal(err)
			}
			if err := harness.service.claimRun(ctx, harness.store, &tc); err != nil {
				t.Fatal(err)
			}
			mode, err := harness.service.loadRunToolMode(ctx, harness.store, tc)
			if err != nil || mode != ToolModeProjectAPI {
				t.Fatalf("initial mode=%q err=%v", mode, err)
			}
			if _, err := harness.service.Steer(ctx, harness.project.UUID, test.thread.UUID, SteeringInput{InputText: "继续使用当前 Tool Mode"}); err != nil {
				t.Fatal(err)
			}
			recovered, err := harness.service.loadToolContext(ctx, harness.store, test.thread.UUID, turn.UUID)
			if err != nil {
				t.Fatal(err)
			}
			recovered.ToolMode, err = harness.service.loadRunToolMode(ctx, harness.store, recovered)
			if err != nil || recovered.ToolMode != ToolModeProjectAPI {
				t.Fatalf("recovered mode=%q err=%v", recovered.ToolMode, err)
			}
			tools := definitionNames(llmToolDefinitionsForMode(recovered.Thread, recovered.ToolMode))
			if !containsString(tools, "request_api") || containsString(tools, "get_premise") || containsString(tools, currentProjectAPIToolName) {
				t.Fatalf("recovered tools=%v", tools)
			}
		})
	}
}

func TestProjectAssistantProjectAPIModeCoversLegacyCapabilities(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Project Assistant API 迁移", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "验证全部旧能力映射"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	if got := definitionNames(llmToolDefinitionsForMode(tc.Thread, tc.ToolMode)); strings.Join(got, ",") != "request_api,read_agent_doc,image_gen,request_user_input" {
		t.Fatalf("project assistant tools=%v", got)
	}
	prompts, err := loadContextPromptsForMode(ctx, harness.store, tc.Thread, ToolModeProjectAPI)
	if err != nil {
		t.Fatal(err)
	}
	if prompts.ProjectUUID != harness.project.UUID || prompts.Scene != "" || prompts.LanguageInstruction != "" {
		t.Fatalf("project chat prompt facts=%+v", prompts)
	}
	for _, expected := range []string{"request_api", "read_agent_doc", "必须先用 read_agent_doc", "之后才能使用 request_api"} {
		if !strings.Contains(prompts.Assistant+prompts.APIOverview, expected) {
			t.Fatalf("project assistant prompt missing %q: %+v", expected, prompts)
		}
	}
	for _, guide := range agentGuideDefinitions() {
		if !strings.Contains(prompts.APIOverview, guide.Path) {
			t.Fatalf("project assistant capability index missing Guide %s", guide.Path)
		}
	}
	if !containsString(definitionNames(llmToolDefinitionsForMode(tc.Thread, tc.ToolMode)), "image_gen") {
		t.Fatal("project assistant did not expose image_gen")
	}
	base := "/api/v1/projects/" + harness.project.UUID
	call := func(key, method, url string, body map[string]any) map[string]any {
		t.Helper()
		responseFilter := ""
		for _, route := range harness.service.requestAPIRoutes() {
			if route.Method != method {
				continue
			}
			if _, matched := matchAgentAPIPath(route.PathTemplate, url); !matched {
				continue
			}
			projector, ok := agentAPIProjectorByKey(route.Projector)
			if !ok {
				break
			}
			if projector.NullData {
				responseFilter = ".data"
				break
			}
			prefix := ".data | {"
			if projector.List {
				projector, ok = agentAPIProjectorByKey(projector.ItemProjector)
				if !ok {
					break
				}
				prefix = ".data | {items:{"
				responseFilter = prefix + strings.Join(agentAPIProjectorFieldNames(projector), ",") + "}}"
			} else {
				responseFilter = prefix + strings.Join(agentAPIProjectorFieldNames(projector), ",") + "}"
			}
			break
		}
		if responseFilter == "" {
			t.Fatalf("no reviewed response projector for %s %s", method, url)
		}
		args := map[string]any{"method": method, "url": url, "response_filter": responseFilter}
		if body != nil {
			args["request_body"] = body
		}
		value, callErr := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: key}, args)
		if callErr != nil {
			t.Fatalf("%s %s failed: %v", method, url, callErr)
		}
		result, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("%s %s shape=%T %+v", method, url, value, value)
		}
		return result
	}
	profile := call("assistant-story-get", "GET", base+"/story-profile", nil)
	updatedProfile := call("assistant-story-update", "PUT", base+"/story-profile", map[string]any{"story_md": "# Project API 故事档案", "expected_revision": float64(intArg(profile, "revision"))})
	if updatedProfile["story_md"] != "# Project API 故事档案" || intArg(updatedProfile, "revision") != intArg(profile, "revision")+1 {
		t.Fatalf("updated story profile=%+v", updatedProfile)
	}
	storyService := story.NewService(harness.store)
	chapter, err := storyService.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol02.ch01", Title: "Project API 章节", Content: "旧正文", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	chapterList := call("assistant-chapter-list", "GET", base+"/chapters", nil)
	if len(chapterList["items"].([]any)) != 1 {
		t.Fatalf("chapter list=%+v", chapterList)
	}
	chapterValue := call("assistant-chapter-get", "GET", base+"/chapters/"+chapter.UUID, nil)
	updatedChapter := call("assistant-chapter-update", "PUT", base+"/chapters/"+chapter.UUID+"/current-story", map[string]any{"content": "Project API 新正文", "content_format": "md", "expected_revision": float64(intArg(chapterValue, "revision"))})
	if intArg(updatedChapter, "revision") != chapter.Revision+1 {
		t.Fatalf("updated chapter=%+v", updatedChapter)
	}
	premise := call("assistant-premise-get", "GET", base+"/premise", nil)
	if premise["uuid"] == nil || premise["default_style"] == nil {
		t.Fatalf("premise=%+v", premise)
	}
	productionService := production.NewService(harness.store, nil)
	upload, err := productionService.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "assistant-create.png", Reader: bytes.NewReader(agentTestPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	createdAsset := call("assistant-asset-create", "POST", base+"/premise-assets", map[string]any{"upload_uuid": upload.UUID, "asset_type": "prop", "title": "Project API 道具", "summary": "创建测试"})
	assetUUID, _ := createdAsset["uuid"].(string)
	if !isUUIDv7(assetUUID) {
		t.Fatalf("created asset=%+v", createdAsset)
	}
	assetList := call("assistant-asset-list", "GET", base+"/premise-assets", nil)
	if len(assetList["items"].([]any)) != 1 {
		t.Fatalf("asset list=%+v", assetList)
	}
	assetValue := call("assistant-asset-get", "GET", base+"/premise-assets/"+assetUUID, nil)
	updatedAsset := call("assistant-asset-update", "PATCH", base+"/premise-assets/"+assetUUID, map[string]any{"expected_revision": float64(intArg(assetValue, "revision")), "summary": "Project API 已更新"})
	if updatedAsset["summary"] != "Project API 已更新" {
		t.Fatalf("updated asset=%+v", updatedAsset)
	}
	section, err := productionService.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Project API Section", StoryboardMD: "# 旧分镜"})
	if err != nil {
		t.Fatal(err)
	}
	sectionValue := call("assistant-section-get", "GET", base+"/chapters/"+chapter.UUID+"/comic-sections/"+section.UUID, nil)
	updatedSection := call("assistant-storyboard-update", "POST", base+"/chapters/"+chapter.UUID+"/comic-sections/"+section.UUID+"/storyboard-variants", map[string]any{"content_md": "# Project API 新分镜", "expected_revision": float64(intArg(sectionValue, "revision"))})
	if intArg(updatedSection, "revision") != section.Revision+1 {
		t.Fatalf("updated section=%+v", updatedSection)
	}
	assetReferenceUUID := mustAgentUUID(t)
	generationCases := []struct {
		name, url, kind, resourceUUID, chapterUUID, taskRoute string
	}{
		{name: "chapter", url: base + "/chapters/" + chapter.UUID + "/generations", kind: "story_chapter_generation", resourceUUID: chapter.UUID, chapterUUID: chapter.UUID, taskRoute: "tasks"},
		{name: "premise setting", url: base + "/premise-sources/" + mustAgentUUID(t) + "/setting-generations", kind: "premise_setting_generation", taskRoute: "production-tasks"},
		{name: "premise breakdown", url: base + "/premise-setting-images/" + mustAgentUUID(t) + "/breakdowns", kind: "premise_asset_breakdown", taskRoute: "production-tasks"},
		{name: "comic image", url: base + "/chapters/" + chapter.UUID + "/comic-sections/" + section.UUID + "/image-generations", kind: "comic_image_generation", resourceUUID: section.UUID, chapterUUID: chapter.UUID, taskRoute: "production-tasks"},
	}
	for index := range generationCases {
		fixture := &generationCases[index]
		segments := strings.Split(strings.TrimSuffix(fixture.url, "/"), "/")
		if fixture.resourceUUID == "" {
			fixture.resourceUUID = segments[len(segments)-2]
		}
		key := "assistant-generation-" + fixture.name
		body := map[string]any{"prompt": "生成 " + fixture.name, "model": "explicit-model"}
		if fixture.kind == "story_chapter_generation" {
			body["prompt_key"] = "next_story_chapter"
		} else if fixture.kind == "comic_image_generation" {
			body["premise_asset_uuids"] = []any{assetReferenceUUID}
		}
		task := call(key, "POST", fixture.url, body)
		taskUUID, _ := task["uuid"].(string)
		if !isUUIDv7(taskUUID) || task["status"] != "queued" || task["kind"] != fixture.kind {
			t.Fatalf("%s queued task=%+v", fixture.name, task)
		}
		status := call(key+"-status", "GET", base+"/"+fixture.taskRoute+"/"+taskUUID, nil)
		if status["status"] != "queued" {
			t.Fatalf("%s queued task was misreported=%+v", fixture.name, status)
		}
		harness.queue.mu.Lock()
		request := harness.queue.requests[len(harness.queue.requests)-1]
		harness.queue.mu.Unlock()
		if request.Kind != fixture.kind || request.ResourceUUID != fixture.resourceUUID || request.ChapterUUID != fixture.chapterUUID || request.ProviderUUID != tc.Run.ProviderUUID || request.Model != "explicit-model" || request.Prompt != "生成 "+fixture.name || request.IdempotencyKey != key {
			t.Fatalf("%s generation request=%+v", fixture.name, request)
		}
		if fixture.kind == "story_chapter_generation" && request.PromptKey != "next_story_chapter" {
			t.Fatalf("chapter generation prompt_key was not forwarded: %+v", request)
		}
		if fixture.kind == "comic_image_generation" && strings.Join(request.PremiseAssetUUIDs, ",") != assetReferenceUUID {
			t.Fatalf("%s generation references were not forwarded: %+v", fixture.name, request)
		}
	}

	invalidWrites := []map[string]any{
		{"method": "PUT", "url": base + "/story-profile", "request_body": map[string]any{"story_md": "missing revision"}, "response_filter": ".data | {uuid,revision}"},
		{"method": "PUT", "url": base + "/chapters/" + chapter.UUID + "/current-story", "request_body": map[string]any{"content": "bad", "content_format": "html", "expected_revision": float64(1)}, "response_filter": ".data | {uuid,revision}"},
		{"method": "PATCH", "url": base + "/premise-assets/" + assetUUID, "request_body": map[string]any{"summary": "missing revision"}, "response_filter": ".data | {uuid,revision}"},
		{"method": "POST", "url": base + "/chapters/" + chapter.UUID + "/comic-sections/" + section.UUID + "/storyboard-variants", "request_body": map[string]any{"expected_revision": float64(1)}, "response_filter": ".data | {uuid,revision}"},
		{"method": "POST", "url": base + "/chapters/" + chapter.UUID + "/generations", "request_body": map[string]any{}, "response_filter": ".data | {uuid,status}"},
	}
	for index, args := range invalidWrites {
		if _, err := parseAgentAPIRequest(tc, args); err == nil {
			t.Fatalf("invalid project assistant write %d accepted: %+v", index, args)
		}
	}
	if _, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "assistant-invalid-asset-create"}, map[string]any{
		"method": "POST", "url": base + "/premise-assets", "request_body": map[string]any{"asset_type": "prop", "title": "missing source"}, "response_filter": ".data | {uuid}",
	}); err == nil || errorCode(err) != CodeToolValidation {
		t.Fatalf("asset creation without an image source was accepted: %v", err)
	}
}

func mustJSONText(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func messagesContain(messages []llm.ChatMessage, wanted string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, wanted) {
			return true
		}
	}
	return false
}
