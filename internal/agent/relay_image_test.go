package agent

import (
	"context"
	"encoding/json"
	"errors"
	"lumi/internal/imagegen"
	"lumi/internal/modelsettings"
	"lumi/internal/picturebook"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/sitesettings"
	"strings"
	"testing"
)

func TestYoloAndDraftSizingConstraintsAreRelayOnly(t *testing.T) {
	for _, mode := range []string{"images", "responses", "bailian", "cloudflare"} {
		t.Run(mode, func(t *testing.T) {
			h := newAgentHarnessWithPictureBook(t, &project.PictureBookInput{Format: project.PictureBookClassic})
			ctx := context.Background()
			relay := mode == "images" || mode == "responses"
			providerType, model := provider.TypeCloudflareAIGateway, "openai/gpt-5.6-terra"
			settings := map[string]any{}
			if relay {
				providerType, model = provider.TypeOpenAIRelay, "gpt-6.1-sol"
				if mode == "images" {
					model = "gpt-image-2.5-flare"
				}
				settings = map[string]any{sitesettings.RelayImageAPIModeKey: mode, sitesettings.RelayDefaultImageModelKey: model, sitesettings.RelayBaseURLKey: "https://text.example/v1", sitesettings.RelayAPIKeyKey: "secret", sitesettings.RelayImageBaseURLKey: "https://image.example/v1", sitesettings.RelayImageAPIKeyKey: "image-secret"}
			} else if mode == "bailian" {
				providerType, model = provider.TypeAliyunBailian, provider.BailianImageModel
				settings = map[string]any{sitesettings.BailianWorkspaceKey: "workspace-1", sitesettings.BailianRegionKey: "cn-beijing", sitesettings.BailianAPIKeyKey: "bailian-secret"}
			}
			if len(settings) > 0 {
				if _, _, err := h.providers.Settings().Update(ctx, settings); err != nil {
					t.Fatal(err)
				}
			}
			items, err := h.providers.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			providerUUID := ""
			for _, item := range items {
				if item.ProviderType == providerType {
					providerUUID = item.UUID
					if _, err := h.providers.MarkVerified(ctx, providerUUID); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := h.service.models.Patch(ctx, h.store, modelsettings.PatchInput{Changes: map[string]*modelsettings.Selection{modelsettings.ProjectImage: {ProviderUUID: providerUUID, Model: model}}}); err != nil {
				t.Fatal(err)
			}
			workflow, err := h.service.CreateYoloWorkflow(ctx, h.project.UUID, CreateYoloInput{Title: "Exact landscape", StoryPrompt: "A moon adventure", ProviderUUID: h.provider.UUID, IdempotencyKey: "relay-reject-landscape"})
			if relay {
				var domainErr *Error
				if !errors.As(err, &domainErr) || domainErr.Code != picturebook.CodeAspectRatioUnsupported {
					t.Fatalf("relay 4:3 error=%v", err)
				}
				for _, table := range []string{"workflows", "workflow_steps", "chat_threads"} {
					var count int64
					if err := h.store.DB().Table(table).Count(&count).Error; err != nil || count != 0 {
						t.Fatalf("rejected workflow wrote %s: count=%d err=%v", table, count, err)
					}
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				var snapshot yoloSnapshot
				if err := json.Unmarshal(workflow.InputSnapshot, &snapshot); err != nil {
					t.Fatal(err)
				}
				if snapshot.OutputSize != "1536x1152" {
					t.Fatalf("non-relay size changed: %s", snapshot.OutputSize)
				}
			}
			makeHarnessProjectDraft(t, h, mustAgentUUID(t), "A landscape picture book")
			prompts, err := h.service.loadContextPrompts(ctx, h.store, threadRecord{})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(prompts.Assistant, "三档尺寸") != relay {
				t.Fatalf("draft sizing guidance relay=%v prompt=%s", relay, prompts.Assistant)
			}
		})
	}
}

func TestRelayAgentImageUsesImageGroupForGlobalAndProjectSelections(t *testing.T) {
	for _, mode := range []string{"responses", "images"} {
		for _, global := range []bool{false, true} {
			t.Run(mode+"/"+map[bool]string{false: "project", true: "global"}[global], func(t *testing.T) {
				pictureBook := &project.PictureBookInput{Format: project.PictureBookVertical}
				if mode == "images" {
					pictureBook = &project.PictureBookInput{Format: project.PictureBookClassic, AspectRatio: &project.AspectRatioInput{Mode: project.AspectCustom, Width: 3, Height: 2}}
				}
				h := newAgentHarnessWithPictureBook(t, pictureBook)
				ctx := context.Background()
				model := "gpt-6.1-sol"
				if mode == "images" {
					model = "gpt-image-2.5-flare"
				}
				if _, _, err := h.providers.Settings().Update(ctx, map[string]any{sitesettings.RelayImageAPIModeKey: mode, sitesettings.RelayDefaultImageModelKey: model, sitesettings.RelayBaseURLKey: "https://text.example/v1", sitesettings.RelayAPIKeyKey: "text-private-key", sitesettings.RelayImageBaseURLKey: "https://image.example/v1", sitesettings.RelayImageAPIKeyKey: "image-private-key"}); err != nil {
					t.Fatal(err)
				}
				items, err := h.providers.List(ctx)
				if err != nil {
					t.Fatal(err)
				}
				var relay provider.Provider
				for _, item := range items {
					if item.ProviderType == provider.TypeOpenAIRelay {
						relay, err = h.providers.MarkVerified(ctx, item.UUID)
						if err != nil {
							t.Fatal(err)
						}
					}
				}
				input := modelsettings.PatchInput{Changes: map[string]*modelsettings.Selection{modelsettings.ProjectImage: {ProviderUUID: relay.UUID, Model: model}}}
				if global {
					_, err = h.service.models.PatchGlobal(ctx, input)
				} else {
					_, err = h.service.models.Patch(ctx, h.store, input)
				}
				if err != nil {
					t.Fatal(err)
				}
				thread := h.createThread(t)
				turn, err := h.service.CreateTurn(ctx, h.project.UUID, thread.UUID, CreateTurnInput{InputText: "Draw a moon"})
				if err != nil {
					t.Fatal(err)
				}
				tc, err := h.service.loadToolContext(ctx, h.store, thread.UUID, turn.UUID)
				if err != nil {
					t.Fatal(err)
				}
				client := &imageClientFake{response: imagegen.Response{Bytes: agentTestPNG(t), MIMEType: "image/png", RevisedPrompt: "image-private-key"}}
				h.service.WithImageClient(client)
				_, err = h.service.executeImageGenTool(ctx, h.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), ToolName: "image_gen"}, map[string]any{"prompt": "Draw a moon", "reference_uuids": []string{}})
				if err != nil {
					t.Fatal(err)
				}
				if len(client.requests) != 1 {
					t.Fatal("missing image call")
				}
				request := client.requests[0]
				if request.BaseURL != "https://image.example/v1" || request.APIKey != "image-private-key" || request.Model != model || request.ImageAPIMode != mode {
					t.Fatal("image credentials mismatch")
				}
				workflow, err := h.service.CreateYoloWorkflow(ctx, h.project.UUID, CreateYoloInput{Title: "Relay size", StoryPrompt: "A moon adventure", ProviderUUID: h.provider.UUID, IdempotencyKey: "relay-size"})
				if err != nil {
					t.Fatal(err)
				}
				var row workflowRecord
				if err := h.store.DB().Where("uuid=?", workflow.UUID).First(&row).Error; err != nil {
					t.Fatal(err)
				}
				var frozen yoloSnapshot
				if err := json.Unmarshal([]byte(row.InputSnapshot), &frozen); err != nil {
					t.Fatal(err)
				}
				wantSize := "1024x1536"
				if mode == "images" {
					wantSize = "1536x1024"
				}
				if frozen.OutputSize != wantSize || frozen.ImageModel != model || frozen.ImageProviderUUID != relay.UUID {
					t.Fatalf("workflow image settings=%+v", frozen)
				}
				var snapshots []struct{ RequestPayload, Response, OutputSummary string }
				if err := h.store.DB().Table("llm_logs").Select("request_payload,response,output_summary").Where("provider_type=?", provider.TypeOpenAIRelay).Scan(&snapshots).Error; err != nil {
					t.Fatal(err)
				}
				encoded, _ := json.Marshal(snapshots)
				if len(snapshots) != 1 || strings.Contains(string(encoded), "image-private-key") || strings.Contains(string(encoded), "text-private-key") {
					t.Fatal("missing or unsafe snapshots")
				}
			})
		}
	}
}
