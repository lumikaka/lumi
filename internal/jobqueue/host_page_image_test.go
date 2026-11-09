package jobqueue

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"lumi/internal/files"
	"lumi/internal/production"
	"lumi/internal/project"
)

// Host mode (Claude generate_page_image) reuses the comic image task but pins
// the prepare_page_image prompt and explicit references: no LLM reference
// selection runs even when the project has candidate settings.
func TestHostPageImageTaskUsesPreparedPromptWithoutSelection(t *testing.T) {
	harness := newQueueHarness(t)
	imageProvider := &recordingImageProvider{content: productionPNG(t)}
	harness.queue.WithImageClient(imageProvider)
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch40")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "灯塔重逢", StoryboardMD: "## Section 核心剧情目标\n月光小狐狸抵达黄昏灯塔。"})
	if err != nil {
		t.Fatal(err)
	}
	upload, err := service.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "fox.png", Reader: bytes.NewReader(productionPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := service.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: upload.UUID, AssetType: production.AssetCharacter, Title: "月光小狐狸", Summary: "月光邮差"})
	if err != nil {
		t.Fatal(err)
	}
	selector := &recordingSelectionProvider{sectionID: section.UUID, title: asset.Title}
	harness.queue.llm = selector

	var prepared production.PreparedPageImage
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		prepared, err = production.NewService(store, nil).ValidatePageImageInputs(ctx, production.PreparePageImageInput{PageUUID: section.UUID, SettingUUIDs: []string{}, AdditionalDirection: "四周留白"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if prepared.GenerationPrompt == "" || prepared.Target.ChapterUUID() != chapter.UUID {
		t.Fatalf("prepared=%+v", prepared)
	}

	// A prompt that differs from the server rendering is refused before any
	// task row exists, so a host cannot smuggle an unreviewed prompt.
	_, err = harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{Prompt: "四周留白", PremiseAssetUUIDs: []string{}, IdempotencyKey: "host-page-stale", HostPagePrompt: prepared.GenerationPrompt + "\n额外指令"})
	var stale *production.Error
	if !errors.As(err, &stale) || stale.Code != production.CodePageImagePreparationStale {
		t.Fatalf("stale host prompt err=%v", err)
	}
	if _, found, err := harness.queue.FindProductionTask(ctx, harness.project.UUID, KindComicImageGeneration, "host-page-stale"); err != nil || found {
		t.Fatalf("stale request persisted a task: found=%v err=%v", found, err)
	}

	task, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{Prompt: "四周留白", PremiseAssetUUIDs: []string{}, IdempotencyKey: "host-page-ok", HostPagePrompt: prepared.GenerationPrompt})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
	if selector.callCount() != 0 {
		t.Fatalf("host mode ran LLM reference selection %d times", selector.callCount())
	}
	requests := imageProvider.snapshot()
	if len(requests) != 1 || requests[0].Prompt != prepared.GenerationPrompt {
		t.Fatalf("image prompt differs from prepare_page_image: %+v", requests)
	}
	found, ok, err := harness.queue.FindProductionTask(ctx, harness.project.UUID, KindComicImageGeneration, "host-page-ok")
	if err != nil || !ok || found.UUID != task.UUID || found.ResourceUUID != section.UUID {
		t.Fatalf("replay lookup=%+v found=%v err=%v", found, ok, err)
	}
	// Replaying the key returns the same task without a second provider call.
	replayed, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{Prompt: "四周留白", PremiseAssetUUIDs: []string{}, IdempotencyKey: "host-page-ok", HostPagePrompt: prepared.GenerationPrompt})
	if err != nil || replayed.UUID != task.UUID || len(imageProvider.snapshot()) != 1 {
		t.Fatalf("replay=%+v err=%v calls=%d", replayed, err, len(imageProvider.snapshot()))
	}
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		prod := production.NewService(store, nil)
		image, ok, err := prod.SectionImageByTask(ctx, section.UUID, task.UUID)
		if err != nil || !ok || image.VariantUUID == "" || image.AssetUUID == "" {
			t.Fatalf("committed image=%+v ok=%v err=%v", image, ok, err)
		}
		page, err := prod.GetSection(ctx, chapter.UUID, section.UUID)
		if err != nil {
			return err
		}
		if page.CurrentImage == nil || page.CurrentImage.UUID != image.VariantUUID {
			t.Fatalf("host image is not current: %+v", page.CurrentImage)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHostImageProviderErrorExplainsSetupWithoutBilling(t *testing.T) {
	var taskErr *Error
	if err := hostImageProviderError(errors.New("no image provider")); !errors.As(err, &taskErr) || taskErr.Code != "image_provider_unavailable" {
		t.Fatalf("err=%v", err)
	}
	for _, want := range []string{"web_url", "view=providers", "idempotency_key", "未计费"} {
		if !bytes.Contains([]byte(taskErr.Details), []byte(want)) {
			t.Fatalf("details missing %q: %s", want, taskErr.Details)
		}
	}
}
