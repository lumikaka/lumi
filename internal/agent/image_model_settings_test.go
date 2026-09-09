package agent

import (
	"context"
	"encoding/json"
	"testing"

	"lumi/internal/imagegen"
	"lumi/internal/modelsettings"
	"lumi/internal/provider"
	"lumi/internal/sitesettings"
)

func TestChatAndYoloUseProjectImageOptions(t *testing.T) {
	for _, global := range []bool{false, true} {
		name := "project"
		if global {
			name = "global"
		}
		t.Run(name, func(t *testing.T) { testChatAndYoloImageOptions(t, global) })
	}
}

func testChatAndYoloImageOptions(t *testing.T, global bool) {
	h := newAgentHarness(t)
	ctx := context.Background()
	if _, _, err := h.providers.Settings().Update(ctx, map[string]any{
		sitesettings.BailianWorkspaceKey: "image-thinking",
		sitesettings.BailianRegionKey:    "cn-beijing",
		sitesettings.BailianAPIKeyKey:    "test-secret",
	}); err != nil {
		t.Fatal(err)
	}
	items, err := h.providers.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var bailian provider.Provider
	for _, item := range items {
		if item.ProviderType == provider.TypeAliyunBailian {
			bailian, err = h.providers.MarkVerified(ctx, item.UUID)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	patchSettings := func(input modelsettings.PatchInput) (modelsettings.View, error) {
		if global {
			return h.service.models.PatchGlobal(ctx, input)
		}
		return h.service.models.Patch(ctx, h.store, input)
	}
	disabled := false
	settings, err := patchSettings(modelsettings.PatchInput{Changes: map[string]*modelsettings.Selection{
		modelsettings.ProjectImage: {ProviderUUID: bailian.UUID, Model: provider.BailianImageModelPro, EnableThinking: &disabled, PromptExtend: &disabled},
	}})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := h.service.CreateThread(ctx, h.project.UUID, CreateThreadInput{Title: "Thinking", ProviderUUID: h.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := h.service.CreateTurn(ctx, h.project.UUID, thread.UUID, CreateTurnInput{InputText: "Draw a moon"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := h.service.loadToolContext(ctx, h.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	client := &imageClientFake{response: imagegen.Response{Bytes: agentTestPNG(t), MIMEType: "image/png"}}
	h.service.WithImageClient(client)
	_, err = h.service.executeImageGenTool(ctx, h.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), ToolName: "image_gen"}, map[string]any{"prompt": "Draw a moon", "reference_uuids": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("requests=%+v", client.requests)
	}
	request := client.requests[0]
	if request.ProviderType != provider.TypeAliyunBailian || request.Model != provider.BailianImageModelPro || request.EnableThinking == nil || *request.EnableThinking || request.EnablePromptExtend == nil || *request.EnablePromptExtend {
		t.Fatalf("image tool ignored project override: %+v", request)
	}
	workflow, err := h.service.CreateYoloWorkflow(ctx, h.project.UUID, CreateYoloInput{Title: "Frozen thinking", StoryPrompt: "A moon adventure", ProviderUUID: h.provider.UUID, IdempotencyKey: "frozen-image-thinking"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := patchSettings(modelsettings.PatchInput{ExpectedRevision: settings.Revision, Changes: map[string]*modelsettings.Selection{modelsettings.ProjectImage: nil}}); err != nil {
		t.Fatal(err)
	}
	var row workflowRecord
	if err := h.store.DB().Where("uuid=?", workflow.UUID).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot yoloSnapshot
	if err := json.Unmarshal([]byte(row.InputSnapshot), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.ImageProviderUUID != bailian.UUID || snapshot.ImageModel != provider.BailianImageModelPro || snapshot.ImageEnableThinking == nil || *snapshot.ImageEnableThinking || snapshot.ImagePromptExtend == nil || *snapshot.ImagePromptExtend {
		t.Fatalf("workflow did not freeze thinking: %+v", snapshot)
	}
}

func TestBootstrapAndNewChatsUseGlobalScenarioWithoutAnActiveReadyProvider(t *testing.T) {
	h := newAgentHarness(t)
	ctx := context.Background()
	if _, _, err := h.providers.Settings().Update(ctx, map[string]any{
		sitesettings.BailianWorkspaceKey: "global-chat",
		sitesettings.BailianRegionKey:    "cn-beijing",
		sitesettings.BailianAPIKeyKey:    "test-secret",
	}); err != nil {
		t.Fatal(err)
	}
	items, err := h.providers.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bailian, err := h.providers.MarkVerified(ctx, items[1].UUID)
	if err != nil {
		t.Fatal(err)
	}
	view, err := h.service.models.PatchGlobal(ctx, modelsettings.PatchInput{Changes: map[string]*modelsettings.Selection{
		modelsettings.ChatArea: {ProviderUUID: bailian.UUID, Model: provider.BailianTextModelQwen38Max},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.providers.Settings().Reset(ctx, []string{sitesettings.CloudflareAPITokenKey}); err != nil {
		t.Fatal(err)
	}
	if err := h.service.ValidateBootstrapTextModel(ctx); err != nil {
		t.Fatalf("global ChatArea must satisfy pre-creation validation: %v", err)
	}
	thread, err := h.service.CreateThread(ctx, h.project.UUID, CreateThreadInput{Title: "Global chat"})
	if err != nil {
		t.Fatal(err)
	}
	if thread.Model != provider.BailianTextModelQwen38Max || thread.ModelSource != modelsettings.SourceGlobalScenario {
		t.Fatalf("thread=%+v", thread)
	}
	if _, err := h.service.models.PatchGlobal(ctx, modelsettings.PatchInput{ExpectedRevision: view.Revision, Changes: map[string]*modelsettings.Selection{
		modelsettings.ChatArea: {ProviderUUID: bailian.UUID, Model: provider.BailianTextModel},
	}}); err != nil {
		t.Fatal(err)
	}
	frozen, err := h.service.GetThread(ctx, h.project.UUID, thread.UUID)
	if err != nil || frozen.Model != provider.BailianTextModelQwen38Max {
		t.Fatalf("existing chat changed: %+v err=%v", frozen, err)
	}
	latest, err := h.service.CreateThread(ctx, h.project.UUID, CreateThreadInput{Title: "Latest defaults"})
	if err != nil || latest.Model != provider.BailianTextModel {
		t.Fatalf("new chat=%+v err=%v", latest, err)
	}
}
