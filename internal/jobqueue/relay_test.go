package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"lumi/internal/modelsettings"
	"lumi/internal/picturebook"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/sitesettings"
	"strings"
	"testing"
	"time"
)

func TestRelayProductionImageUsesImageGroup(t *testing.T) {
	for _, mode := range []string{"responses", "images", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			h := newQueueHarness(t)
			ctx := context.Background()
			model, initialMode := "gpt-5.6-sol", "responses"
			if mode == "images" {
				model, initialMode = "gpt-image-2", "images"
			}
			imageClient := &failFirstRecordingImageProvider{content: productionPNG(t)}
			h.queue.WithImageClient(imageClient)
			if _, _, err := h.queue.providers.Settings().Update(ctx, map[string]any{sitesettings.RelayImageAPIModeKey: initialMode, sitesettings.RelayDefaultImageModelKey: model, sitesettings.RelayBaseURLKey: "https://text.example/v1", sitesettings.RelayAPIKeyKey: "text-private-key", sitesettings.RelayImageBaseURLKey: "https://image.example/v1", sitesettings.RelayImageAPIKeyKey: "image-private-key"}); err != nil {
				t.Fatal(err)
			}
			items, err := h.queue.providers.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var relay provider.Provider
			for _, item := range items {
				if item.ProviderType == provider.TypeOpenAIRelay {
					relay, err = h.queue.providers.MarkVerified(ctx, item.UUID)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			var service *production.Service
			if err := h.projects.WithCurrentStore(ctx, h.project.UUID, func(store *project.Store) error { service = production.NewService(store, nil); return nil }); err != nil {
				t.Fatal(err)
			}
			source, err := service.CreatePremiseSource(ctx, production.CreateSourceInput{SourceText: "A moonlit forest", StyleSnapshot: "Watercolor", SourceType: "generated"})
			if err != nil {
				t.Fatal(err)
			}
			task, err := h.queue.CreatePremiseSettingGeneration(ctx, h.project.UUID, source.UUID, CreateProductionGenerationInput{ProviderUUID: relay.UUID, Model: model, Prompt: source.SourceText, IdempotencyKey: "relay-setting"})
			if err != nil {
				t.Fatal(err)
			}
			waitProductionStatus(t, h.queue, h.project.UUID, task.UUID, StatusFailed)
			time.Sleep(200 * time.Millisecond)
			// Change the live provider before retrying; the durable task keeps its wire protocol.
			nextMode, nextModel := "images", "gpt-image-2"
			if mode == "images" {
				nextMode, nextModel = "responses", "gpt-6.1-sol"
			}
			if _, _, err := h.queue.providers.Settings().Update(ctx, map[string]any{sitesettings.RelayImageAPIModeKey: nextMode, sitesettings.RelayDefaultImageModelKey: nextModel}); err != nil {
				t.Fatal(err)
			}
			if mode == "legacy" {
				var fields map[string]any
				if err := json.Unmarshal(task.InputSnapshot, &fields); err != nil {
					t.Fatal(err)
				}
				delete(fields, "image_api_mode")
				legacy, err := json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
				if err := h.projects.WithCurrentStore(ctx, h.project.UUID, func(store *project.Store) error {
					return store.DB().Table("production_task_runs").Where("uuid=?", task.UUID).Update("input_snapshot", string(legacy)).Error
				}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := h.queue.RetryProductionTask(ctx, h.project.UUID, task.UUID); err != nil {
				t.Fatal(err)
			}
			waitProductionStatus(t, h.queue, h.project.UUID, task.UUID, StatusCompleted)
			requests := imageClient.snapshot()
			if len(requests) != 2 {
				t.Fatalf("requests=%d", len(requests))
			}
			for _, request := range requests {
				if request.APIMode() != initialMode || request.APIKey != "image-private-key" || request.BaseURL != "https://image.example/v1" || request.Model != model {
					t.Fatal("image group not used")
				}
			}
			var snapshot production.GenerationSnapshot
			if err := json.Unmarshal(task.InputSnapshot, &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.ImageAPIMode != initialMode || snapshot.ProviderBaseURL != "https://image.example/v1" || strings.Contains(string(task.InputSnapshot), "private-key") {
				t.Fatal("invalid task snapshot")
			}

		})
	}
}

func TestRelayImagesComicTaskValidatesExactLandscapeSize(t *testing.T) {
	for _, global := range []bool{false, true} {
		for _, ratio := range []struct {
			name          string
			width, height int
			supported     bool
		}{{"3:2", 3, 2, true}, {"4:3", 4, 3, false}} {
			t.Run(map[bool]string{false: "project", true: "global"}[global]+"/"+ratio.name, func(t *testing.T) {
				layout := project.ComicLayoutPageComic
				h := newQueueHarnessWithPictureBook(t, &project.PictureBookInput{Format: project.PictureBookComicStory, AspectRatio: &project.AspectRatioInput{Mode: project.AspectCustom, Width: ratio.width, Height: ratio.height}, ComicLayout: &layout})
				ctx := context.Background()
				imageClient := &recordingImageProvider{content: productionPNG(t)}
				h.queue.WithImageClient(imageClient)
				providers := h.queue.providers
				if _, _, err := providers.Settings().Update(ctx, map[string]any{
					sitesettings.RelayImageAPIModeKey: sitesettings.ImageAPIImages, sitesettings.RelayDefaultImageModelKey: "gpt-image-2.5-flare",
					sitesettings.RelayBaseURLKey: "https://text.example/v1", sitesettings.RelayAPIKeyKey: "text-private-key",
					sitesettings.RelayImageBaseURLKey: "https://image.example/v1", sitesettings.RelayImageAPIKeyKey: "image-private-key",
				}); err != nil {
					t.Fatal(err)
				}
				items, err := providers.List(ctx)
				if err != nil {
					t.Fatal(err)
				}
				var relay provider.Provider
				for _, item := range items {
					if item.ProviderType == provider.TypeOpenAIRelay {
						relay, err = providers.MarkVerified(ctx, item.UUID)
						if err != nil {
							t.Fatal(err)
						}
					}
				}
				var service *production.Service
				if err := h.projects.WithCurrentStore(ctx, h.project.UUID, func(store *project.Store) error {
					service = production.NewService(store, nil)
					models := modelsettings.NewResolver(providers)
					input := modelsettings.PatchInput{Changes: map[string]*modelsettings.Selection{modelsettings.ProjectImage: {ProviderUUID: relay.UUID, Model: "gpt-image-2.5-flare"}}}
					if global {
						_, err := models.PatchGlobal(ctx, input)
						return err
					}
					_, err := models.Patch(ctx, store, input)
					return err
				}); err != nil {
					t.Fatal(err)
				}
				chapter := h.createChapter(t, "vol01.ch31")
				section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Time lesson", StoryboardMD: "A clock on a comic page."})
				if err != nil {
					t.Fatal(err)
				}
				task, err := h.queue.CreateComicImageGeneration(ctx, h.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{IdempotencyKey: "relay-exact-landscape"})
				if !ratio.supported {
					var taskErr *Error
					if !errors.As(err, &taskErr) || taskErr.Code != picturebook.CodeAspectRatioUnsupported {
						t.Fatalf("unsupported page ratio: %v", err)
					}
					var count int64
					countErr := h.projects.WithCurrentStore(ctx, h.project.UUID, func(store *project.Store) error {
						return store.DB().Table("production_task_runs").Count(&count).Error
					})
					if countErr != nil || count != 0 {
						t.Fatalf("rejected ratio created tasks: count=%d err=%v", count, countErr)
					}
					if len(imageClient.snapshot()) != 0 {
						t.Fatal("rejected ratio called image provider")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var frozen production.GenerationSnapshot
				if err := json.Unmarshal(task.InputSnapshot, &frozen); err != nil {
					t.Fatal(err)
				}
				if frozen.ImageAPIMode != "images" || frozen.Model != "gpt-image-2.5-flare" || frozen.OutputSize != "1536x1024" || frozen.PictureBook == nil || frozen.PictureBook.AspectRatio.Width != 3 || frozen.PictureBook.AspectRatio.Height != 2 {
					t.Fatalf("snapshot=%+v", frozen)
				}
				waitProductionStatus(t, h.queue, h.project.UUID, task.UUID, StatusCompleted)
				requests := imageClient.snapshot()
				if len(requests) != 1 {
					t.Fatalf("requests=%d", len(requests))
				}
				request := requests[0]
				if request.Size != "1536x1024" || request.ImageAPIMode != "images" || request.Model != "gpt-image-2.5-flare" || request.BaseURL != "https://image.example/v1" {
					t.Fatalf("incorrect image mode, model, endpoint or size")
				}
			})
		}
	}
}
