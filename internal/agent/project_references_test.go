package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"lumi/internal/llm"
	"lumi/internal/story"
)

func TestProjectReferenceForAgentAPIRoutes(t *testing.T) {
	chapterUUID := mustAgentUUID(t)
	sectionUUID := mustAgentUUID(t)
	assetUUID := mustAgentUUID(t)
	workflowUUID := mustAgentUUID(t)
	publicChapter := map[string]any{"uuid": chapterUUID}
	publicSection := map[string]any{"uuid": sectionUUID}
	publicAsset := map[string]any{"uuid": assetUUID}
	publicWorkflow := map[string]any{"uuid": workflowUUID}

	tests := []struct {
		name   string
		route  string
		params map[string]string
		body   map[string]any
		value  any
		want   string
	}{
		{name: "story profile update", route: RouteStoryProfileUpdate, want: "@project/story-profile"},
		{name: "YOLO workflow create", route: RouteYoloWorkflowCreate, value: publicWorkflow, want: "@project/workflows/" + workflowUUID},
		{name: "story profile import", route: RouteStoryProfileImport, want: "@project/story-profile"},
		{name: "story profile regeneration", route: RouteStoryProfileRegenerate, want: "@project/story-profile"},
		{name: "story profile generation", route: RouteStoryProfileGenerationCreate, want: "@project/story-profile"},
		{name: "story profile rebuild", route: RouteStoryProfileRebuildCreate, want: "@project/story-profile"},
		{name: "premise update", route: RoutePremiseUpdate, want: "@project/premise"},
		{name: "premise source create", route: RoutePremiseSourceCreate, want: "@project/premise"},
		{name: "premise source update", route: RoutePremiseSourceUpdate, want: "@project/premise"},
		{name: "setting image import", route: RouteSettingImageImport, want: "@project/premise"},
		{name: "setting image select", route: RouteSettingImageSelect, want: "@project/premise"},
		{name: "premise generation", route: RoutePremiseSettingGenerationCreate, want: "@project/premise"},
		{name: "premise breakdown", route: RoutePremiseBreakdownCreate, want: "@project/premise"},
		{name: "asset create", route: RoutePremiseAssetCreate, value: publicAsset, want: "@project/premise/assets/" + assetUUID},
		{name: "asset update", route: RoutePremiseAssetUpdate, params: map[string]string{"premise_asset_uuid": assetUUID}, want: "@project/premise/assets/" + assetUUID},
		{name: "asset delete", route: RoutePremiseAssetDelete, params: map[string]string{"premise_asset_uuid": assetUUID}, want: "@project/premise/assets/" + assetUUID},
		{name: "asset restore", route: RoutePremiseAssetRestore, params: map[string]string{"premise_asset_uuid": assetUUID}, want: "@project/premise/assets/" + assetUUID},
		{name: "asset variant create", route: RoutePremiseAssetVariantCreate, params: map[string]string{"premise_asset_uuid": assetUUID}, want: "@project/premise/assets/" + assetUUID},
		{name: "asset variant select", route: RoutePremiseAssetVariantSelect, params: map[string]string{"premise_asset_uuid": assetUUID}, want: "@project/premise/assets/" + assetUUID},
		{name: "chapter create", route: RouteChapterCreate, value: publicChapter, want: "@project/chapters/" + chapterUUID},
		{name: "chapter create with body", route: RouteChapterCreate, body: map[string]any{"content": "正文"}, value: publicChapter, want: "@project/chapters/" + chapterUUID + "/body"},
		{name: "chapter update", route: RouteChapterUpdate, params: map[string]string{"chapter_uuid": chapterUUID}, want: "@project/chapters/" + chapterUUID},
		{name: "chapter trash", route: RouteChapterTrash, params: map[string]string{"chapter_uuid": chapterUUID}, want: "@project/chapters/" + chapterUUID},
		{name: "chapter restore", route: RouteChapterRestore, params: map[string]string{"chapter_uuid": chapterUUID}, want: "@project/chapters/" + chapterUUID},
		{name: "chapter body update", route: RouteChapterStoryUpdate, params: map[string]string{"chapter_uuid": chapterUUID}, want: "@project/chapters/" + chapterUUID + "/body"},
		{name: "chapter generation", route: RouteChapterGenerationCreate, params: map[string]string{"chapter_uuid": chapterUUID}, want: "@project/chapters/" + chapterUUID + "/body"},
		{name: "storyboard generation", route: RouteComicStoryboardGenerationCreate, params: map[string]string{"chapter_uuid": chapterUUID}, want: "@project/chapters/" + chapterUUID},
		{name: "section create", route: RouteComicSectionCreate, params: map[string]string{"chapter_uuid": chapterUUID}, value: publicSection, want: "@project/chapters/" + chapterUUID + "/sections/" + sectionUUID},
		{name: "section update", route: RouteComicSectionUpdate, params: map[string]string{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID}, want: "@project/chapters/" + chapterUUID + "/sections/" + sectionUUID},
		{name: "storyboard update", route: RouteStoryboardUpdate, params: map[string]string{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID}, want: "@project/chapters/" + chapterUUID + "/sections/" + sectionUUID},
		{name: "storyboard select", route: RouteStoryboardSelect, params: map[string]string{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID}, want: "@project/chapters/" + chapterUUID + "/sections/" + sectionUUID},
		{name: "section image import", route: RouteComicSectionImageImport, params: map[string]string{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID}, want: "@project/chapters/" + chapterUUID + "/sections/" + sectionUUID},
		{name: "image variant select", route: RouteComicImageVariantSelect, params: map[string]string{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID}, want: "@project/chapters/" + chapterUUID + "/sections/" + sectionUUID},
		{name: "single image generation", route: RouteComicImageGenerationCreate, params: map[string]string{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID}, want: "@project/chapters/" + chapterUUID + "/sections/" + sectionUUID},
		{name: "section delete", route: RouteComicSectionDelete, params: map[string]string{"chapter_uuid": chapterUUID}, want: "@project/chapters/" + chapterUUID},
		{name: "section reorder", route: RouteComicSectionReorder, params: map[string]string{"chapter_uuid": chapterUUID}, want: "@project/chapters/" + chapterUUID},
		{name: "batch image generation", route: RouteComicImageGenerationBatchCreate, params: map[string]string{"chapter_uuid": chapterUUID}, want: "@project/chapters/" + chapterUUID},
		{name: "snapshot restore", route: RouteComicSnapshotRestore, params: map[string]string{"chapter_uuid": chapterUUID}, want: "@project/chapters/" + chapterUUID},
		{name: "export create", route: RouteComicExportCreate, want: "@project/exports"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := projectReferenceForAgentAPI(agentAPIRequest{
				Route:  agentAPIRoute{ID: test.route},
				Method: "POST",
				Params: test.params,
				Body:   test.body,
			}, test.value)
			if got == nil || got.Href != test.want {
				t.Fatalf("reference=%+v want=%q", got, test.want)
			}
		})
	}
}

func TestProjectReferenceForAgentAPIRejectsUntrustedTargets(t *testing.T) {
	chapterUUID := mustAgentUUID(t)
	tests := []agentAPIRequest{
		{Route: agentAPIRoute{ID: RouteChapterUpdate}, Method: "GET", Params: map[string]string{"chapter_uuid": chapterUUID}},
		{Route: agentAPIRoute{ID: RouteChapterUpdate}, Method: "PATCH", Params: map[string]string{"chapter_uuid": strings.ToUpper(chapterUUID)}},
		{Route: agentAPIRoute{ID: RouteChapterUpdate}, Method: "PATCH", Params: map[string]string{"chapter_uuid": "550e8400-e29b-41d4-a716-446655440000"}},
		{Route: agentAPIRoute{ID: RouteProjectUpdate}, Method: "PATCH", Params: map[string]string{"chapter_uuid": chapterUUID}},
		{Route: agentAPIRoute{ID: RouteYoloWorkflowCreate}, Method: "POST"},
	}
	for _, request := range tests {
		if got := projectReferenceForAgentAPI(request, nil); got != nil {
			t.Fatalf("unexpected reference for %+v: %+v", request, got)
		}
	}
}

func TestRequestAPIToolBuildsUIRefBeforeResponseFilter(t *testing.T) {
	harness := newAgentHarness(t)
	tc := toolContext{
		ProjectUUID: harness.project.UUID,
		ToolMode:    ToolModeProjectAPI,
		Thread:      threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject},
	}
	output, err := executeRequestAPIToolWithUIRef(context.Background(), harness.service, harness.store, tc, toolExecutionRecord{
		UUID: mustAgentUUID(t), IdempotencyKey: "chapter-create-ui-ref",
	}, map[string]any{
		"method": "POST",
		"url":    "/api/v1/projects/" + harness.project.UUID + "/chapters",
		"request_body": map[string]any{
			"chapter_code":   "vol01.ch01",
			"title":          "UI Ref Chapter",
			"content":        "initial body",
			"content_format": "md",
		},
		"response_filter": ".data | {title}",
	})
	if err != nil {
		t.Fatal(err)
	}
	filtered, _ := output.Data.(map[string]any)
	if filtered["uuid"] != nil || filtered["title"] != "UI Ref Chapter" {
		t.Fatalf("filtered data=%+v", filtered)
	}
	if output.UIRef == nil || !strings.HasPrefix(output.UIRef.Href, "@project/chapters/") || !strings.HasSuffix(output.UIRef.Href, "/body") {
		t.Fatalf("ui_ref=%+v", output.UIRef)
	}
}

func TestRequestAPIToolEnvelopeExposesUIRefBesideFilteredData(t *testing.T) {
	harness := newAgentHarness(t)
	arguments, err := json.Marshal(map[string]any{
		"method": "POST",
		"url":    "/api/v1/projects/" + harness.project.UUID + "/chapters",
		"request_body": map[string]any{
			"chapter_code":   "vol01.ch02",
			"title":          "Envelope Chapter",
			"content_format": "md",
		},
		"response_filter": ".data | {title}",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := harness.service.executeTool(context.Background(), harness.store, toolContext{
		ProjectUUID: harness.project.UUID,
		ToolMode:    ToolModeProjectAPI,
		Thread:      threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject},
	}, toolExecutionRecord{
		UUID:           mustAgentUUID(t),
		ToolName:       "request_api",
		ArgumentsJSON:  string(arguments),
		IdempotencyKey: "chapter-envelope-ui-ref",
	})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Success bool              `json:"success"`
		Data    map[string]any    `json:"data"`
		UIRef   *agentUIReference `json:"ui_ref"`
	}
	if err := json.Unmarshal(result, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Success || envelope.Data["title"] != "Envelope Chapter" || envelope.Data["uuid"] != nil {
		t.Fatalf("envelope=%s", result)
	}
	if envelope.UIRef == nil || !strings.HasPrefix(envelope.UIRef.Href, "@project/chapters/") || strings.HasSuffix(envelope.UIRef.Href, "/body") {
		t.Fatalf("ui_ref=%+v", envelope.UIRef)
	}
}

func TestAgentFinalReplyCanUseToolUIRefInline(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	chapter, err := story.NewService(harness.store).CreateChapter(ctx, story.CreateChapterInput{
		ChapterCode: "vol01.ch03", Title: "第一章", Content: "旧正文", ContentFormat: "md",
	})
	if err != nil {
		t.Fatal(err)
	}
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "修改第一章正文"})
	if err != nil {
		t.Fatal(err)
	}
	wantHref := "@project/chapters/" + chapter.UUID + "/body"
	harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
		if call == 1 {
			if len(request.Messages) == 0 || !strings.Contains(request.Messages[0].Content, "ui_ref") {
				t.Fatalf("system prompt is missing the ui_ref completion rule")
			}
			arguments, _ := json.Marshal(map[string]any{
				"method": "PUT",
				"url":    "/api/v1/projects/" + harness.project.UUID + "/chapters/" + chapter.UUID + "/current-story",
				"request_body": map[string]any{
					"content": "新正文", "content_format": "md", "expected_revision": chapter.Revision,
				},
				"response_filter": ".data | {title,revision}",
			})
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "update-chapter-body", Name: "request_api", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
		}
		var href string
		for _, message := range request.Messages {
			if message.Role != "tool" {
				continue
			}
			var envelope struct {
				Success bool              `json:"success"`
				UIRef   *agentUIReference `json:"ui_ref"`
			}
			if json.Unmarshal([]byte(message.Content), &envelope) == nil && envelope.Success && envelope.UIRef != nil {
				href = envelope.UIRef.Href
			}
		}
		if href != wantHref {
			t.Fatalf("tool messages exposed href=%q want=%q", href, wantHref)
		}
		return finalResponse("[第一章正文](" + href + ")已经修改完成。"), nil
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	items, err := harness.service.ListItems(ctx, harness.project.UUID, thread.UUID, "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	wantMessage := "[第一章正文](" + wantHref + ")已经修改完成。"
	for _, item := range items.Items {
		if item.Role == "assistant" && item.Content == wantMessage {
			return
		}
	}
	t.Fatalf("assistant message %q was not persisted: %+v", wantMessage, items.Items)
}

func TestCompactToolResultPreservesUIRef(t *testing.T) {
	chapterUUID := mustAgentUUID(t)
	uiRef := &agentUIReference{Href: "@project/chapters/" + chapterUUID + "/body"}
	result := compactToolResult(map[string]any{
		"success": true,
		"data":    strings.Repeat("x", MaxToolResult),
		"ui_ref":  uiRef,
	}, chapterUUID)
	var decoded struct {
		Data struct {
			Compacted bool `json:"compacted"`
		} `json:"data"`
		UIRef *agentUIReference `json:"ui_ref"`
	}
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Data.Compacted || decoded.UIRef == nil || decoded.UIRef.Href != uiRef.Href {
		t.Fatalf("compacted result=%s", result)
	}
}
