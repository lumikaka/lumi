package production

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/files"
	"lumi/internal/project"
	"lumi/internal/story"
)

type productionHarness struct {
	service  *Service
	stories  *story.Service
	projects *project.Manager
	project  project.Summary
}

func newProductionHarness(t *testing.T) *productionHarness {
	t.Helper()
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	manager := project.NewManager(app).WithOpenHook(story.ReconcileOnOpen)
	created, err := manager.CreateWithInput(ctx, project.CreateInput{
		Name:        "Production",
		PictureBook: &project.PictureBookInput{Format: project.PictureBookVertical},
	}, project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	h := &productionHarness{projects: manager, project: created}
	if err := manager.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		h.service = NewService(store, nil)
		h.stories = story.NewService(store)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close(); _ = app.Close() })
	return h
}

func imageBytes(t *testing.T, seed uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 12, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 12; x++ {
			img.Set(x, y, color.RGBA{R: seed + uint8(x), G: uint8(y * 8), B: 120, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func upload(t *testing.T, service *Service, purpose string, content []byte) string {
	t.Helper()
	value, err := service.Files().CreateUpload(context.Background(), files.CreateUploadInput{Purpose: purpose, OriginalFilename: "fixture.png", Reader: bytes.NewReader(content)})
	if err != nil {
		t.Fatal(err)
	}
	return value.UUID
}

func TestOverallStylePromptAndLegacyPremiseStayInSync(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	initial, err := h.service.GetPremise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	items, err := h.stories.UpdatePromptGroup(ctx, story.UpdatePromptGroupInput{
		PromptGroup:             "premise_style",
		Prompts:                 map[string]string{"project_overall_style": "ink wash style"},
		ExpectedCurrentVersions: map[string]int{"project_overall_style": 1},
	})
	if err != nil || len(items) != 4 {
		t.Fatalf("style group update items=%d error=%v", len(items), err)
	}
	updated, err := h.service.GetPremise(ctx)
	if err != nil || updated.DefaultStyle != "ink wash style" || updated.Revision != initial.Revision+1 {
		t.Fatalf("prompt projection = %+v, error=%v", updated, err)
	}
	versions, _, err := h.stories.ListPromptVersions(ctx, "premise_style", "project_overall_style", 1, 20)
	if err != nil || len(versions) != 2 {
		t.Fatalf("style versions=%+v error=%v", versions, err)
	}
	if _, err := h.stories.RestorePromptVersion(ctx, versions[1].UUID, versions[0].VersionNo); err != nil {
		t.Fatal(err)
	}
	restored, err := h.service.GetPremise(ctx)
	if err != nil || restored.DefaultStyle != versions[1].Prompt || restored.Revision != updated.Revision+1 {
		t.Fatalf("restored projection = %+v, error=%v", restored, err)
	}
	legacyUpdated, err := h.service.UpdatePremise(ctx, UpdatePremiseInput{DefaultStyle: "legacy compatible style", ExpectedRevision: restored.Revision})
	if err != nil || legacyUpdated.DefaultStyle != "legacy compatible style" || legacyUpdated.Revision != restored.Revision+1 {
		t.Fatalf("legacy update = %+v, error=%v", legacyUpdated, err)
	}
	latest, _, err := h.stories.ListPromptVersions(ctx, "premise_style", "project_overall_style", 1, 1)
	if err != nil || len(latest) != 1 || latest[0].Prompt != "legacy compatible style" {
		t.Fatalf("legacy canonical version=%+v error=%v", latest, err)
	}
}

func TestPremiseTagsVariantsAndCurrentPointers(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	profile, err := h.service.GetPremise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	profile, err = h.service.UpdatePremise(ctx, UpdatePremiseInput{DefaultStyle: "soft watercolor", ExpectedRevision: profile.Revision})
	if err != nil {
		t.Fatal(err)
	}
	source, err := h.service.CreatePremiseSource(ctx, CreateSourceInput{SourceText: "A child and a red kite", StyleSnapshot: profile.DefaultStyle, SourceType: "manual", Parameters: map[string]any{"temperature": 0.2}})
	if err != nil {
		t.Fatal(err)
	}
	if source.StyleSnapshot != "soft watercolor" {
		t.Fatalf("source snapshot=%q", source.StyleSnapshot)
	}
	assetUploadUUID := upload(t, h.service, "premise_asset", imageBytes(t, 10))
	assetInput := CreateAssetInput{UploadUUID: assetUploadUUID, AssetType: "character", Title: "Hero", Tags: []string{" Hero ", "hero", "LEAD"}, Position: map[string]any{}, Crop: map[string]any{}}
	asset, err := h.service.ImportPremiseAsset(ctx, assetInput)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := h.service.ImportPremiseAsset(ctx, assetInput)
	if err != nil || retried.UUID != asset.UUID || retried.CurrentVariant.UUID != asset.CurrentVariant.UUID {
		t.Fatalf("idempotent import=%+v err=%v", retried, err)
	}
	if len(asset.Tags) != 2 || asset.Tags[0] != "hero" || asset.Tags[1] != "lead" {
		t.Fatalf("normalized tags=%v", asset.Tags)
	}
	filtered, err := h.service.ListPremiseAssets(ctx, "HERO", "active")
	if err != nil || len(filtered) != 1 {
		t.Fatalf("filtered=%v err=%v", filtered, err)
	}
	first := asset.CurrentVariant.UUID
	asset, err = h.service.ImportPremiseAssetVariant(ctx, asset.UUID, upload(t, h.service, "premise_asset", imageBytes(t, 30)), map[string]any{"x": 1}, asset.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if asset.CurrentVariant.UUID == first || asset.CurrentVariant.VersionNo != 2 {
		t.Fatalf("replacement current=%+v", asset.CurrentVariant)
	}
	variants, err := h.service.ListAssetVariants(ctx, asset.UUID)
	if err != nil || len(variants) != 2 {
		t.Fatalf("variants=%v err=%v", variants, err)
	}
	asset, err = h.service.SelectAssetVariant(ctx, asset.UUID, first, asset.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if asset.CurrentVariant.UUID != first || asset.Revision != 2 {
		t.Fatalf("restored asset=%+v", asset)
	}
	trashed, err := h.service.SetPremiseAssetTrashed(ctx, asset.UUID, true, asset.Revision)
	if err != nil || trashed.DeletedAt == nil {
		t.Fatalf("trashed=%+v err=%v", trashed, err)
	}
	active, _ := h.service.ListPremiseAssets(ctx, "", "active")
	trash, _ := h.service.ListPremiseAssets(ctx, "", "trashed")
	if len(active) != 0 || len(trash) != 1 {
		t.Fatalf("active=%d trash=%d", len(active), len(trash))
	}
}

func TestPremiseSourcePaginationAndPageScopedSettingImages(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	created := make([]PremiseSource, 0, 25)
	for index := 0; index < 25; index++ {
		source, err := h.service.CreatePremiseSource(ctx, CreateSourceInput{SourceText: fmt.Sprintf("source %02d", index), SourceType: "manual", Parameters: map[string]any{}})
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, source)
	}
	firstPage, pagination, err := h.service.ListPremiseSourcesPage(ctx, 1, 10)
	if err != nil || len(firstPage) != 10 || pagination.Total != 25 || pagination.CurrentPage != 1 || pagination.LastPage != 3 || firstPage[0].UUID != created[24].UUID {
		t.Fatalf("first source page=%+v pagination=%+v error=%v", firstPage, pagination, err)
	}
	lastPage, pagination, err := h.service.ListPremiseSourcesPage(ctx, 3, 10)
	if err != nil || len(lastPage) != 5 || pagination.Total != 25 || lastPage[4].UUID != created[0].UUID {
		t.Fatalf("last source page=%+v pagination=%+v error=%v", lastPage, pagination, err)
	}
	newestSetting, err := h.service.ImportSettingImage(ctx, upload(t, h.service, "premise_setting_image", imageBytes(t, 31)), created[24].UUID, "newest")
	if err != nil {
		t.Fatal(err)
	}
	oldestSetting, err := h.service.ImportSettingImage(ctx, upload(t, h.service, "premise_setting_image", imageBytes(t, 32)), created[0].UUID, "oldest")
	if err != nil {
		t.Fatal(err)
	}
	visible, err := h.service.ListSettingImagesForSources(ctx, []string{created[24].UUID, created[24].UUID})
	if err != nil || len(visible) != 1 || visible[0].UUID != newestSetting.UUID || visible[0].UUID == oldestSetting.UUID {
		t.Fatalf("page-scoped setting images=%+v error=%v", visible, err)
	}
	empty, err := h.service.ListSettingImagesForSources(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty setting filter=%+v error=%v", empty, err)
	}
}

func TestComicExportPaginationFiltersBeforePaging(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch20", Title: "Exports"})
	if err != nil {
		t.Fatal(err)
	}
	createRecord := func(scope, chapterUUID, hash string) Export {
		t.Helper()
		taskUUID := insertRunningExportTask(t, h.service)
		record, createErr := h.service.CreateExportRecord(ctx, taskUUID, scope, chapterUUID, ExportSnapshot{Version: 2, ProjectUUID: h.project.UUID, Scope: scope, ChapterUUID: chapterUUID}, hashJSON([]byte(hash)))
		if createErr != nil {
			t.Fatal(createErr)
		}
		if updateErr := h.service.store.DB().Table("production_task_runs").Where("uuid = ?", taskUUID).Updates(map[string]any{"status": "completed", "progress": 100}).Error; updateErr != nil {
			t.Fatal(updateErr)
		}
		return record
	}
	projectFirst := createRecord("project", "", "project-first")
	chapterRecord := createRecord("chapter", chapter.UUID, "chapter-only")
	projectLatest := createRecord("project", "", "project-latest")
	projectItems, pagination, err := h.service.ListExportsPage(ctx, ExportFilter{Scope: "project"}, 1, 1)
	if err != nil || len(projectItems) != 1 || pagination.Total != 2 || pagination.LastPage != 2 || projectItems[0].UUID != projectLatest.UUID || projectItems[0].UUID == projectFirst.UUID {
		t.Fatalf("project export page=%+v pagination=%+v error=%v", projectItems, pagination, err)
	}
	chapterItems, pagination, err := h.service.ListExportsPage(ctx, ExportFilter{Scope: "chapter", ChapterUUID: chapter.UUID}, 1, 10)
	if err != nil || len(chapterItems) != 1 || pagination.Total != 1 || chapterItems[0].UUID != chapterRecord.UUID {
		t.Fatalf("chapter export page=%+v pagination=%+v error=%v", chapterItems, pagination, err)
	}
	if _, _, err := h.service.ListExportsPage(ctx, ExportFilter{Scope: "project", ChapterUUID: chapter.UUID}, 1, 10); err == nil {
		t.Fatal("conflicting export filter unexpectedly succeeded")
	} else {
		var domainErr *Error
		if !errors.As(err, &domainErr) || domainErr.Code != CodeValidation {
			t.Fatalf("conflicting export filter error=%v", err)
		}
	}
	exactItems, pagination, err := h.service.ListExportsPage(ctx, ExportFilter{TaskUUID: chapterRecord.TaskUUID, SnapshotHash: chapterRecord.SnapshotHash, Status: "queued"}, 1, 10)
	if err != nil || len(exactItems) != 1 || pagination.Total != 1 || exactItems[0].UUID != chapterRecord.UUID || exactItems[0].Filename == "" {
		t.Fatalf("exact export filter=%+v pagination=%+v error=%v", exactItems, pagination, err)
	}
	invalidFilters := []ExportFilter{
		{TaskUUID: "not-a-uuid"},
		{SnapshotHash: "bad-hash"},
		{Status: "completed"},
	}
	for _, filter := range invalidFilters {
		if _, _, filterErr := h.service.ListExportsPage(ctx, filter, 1, 10); !productionErrorIs(filterErr, CodeValidation) {
			t.Fatalf("invalid export filter=%+v error=%v", filter, filterErr)
		}
	}
}

func TestComicStateCountsOnlyActiveReadyPremiseAssetsWithCurrentImages(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Opening"})
	if err != nil {
		t.Fatal(err)
	}
	assertReadiness := func(want bool, count int) {
		t.Helper()
		state, stateErr := h.service.GetComicState(ctx, chapter.UUID)
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		if state.HasPremiseAssets != want || state.PremiseAssetCount != count {
			t.Fatalf("comic readiness=%+v want has=%v count=%d", state, want, count)
		}
	}

	assertReadiness(false, 0)
	asset, err := h.service.ImportPremiseAsset(ctx, CreateAssetInput{
		UploadUUID: upload(t, h.service, "premise_asset", imageBytes(t, 41)), AssetType: AssetCharacter, Title: "Ready hero",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertReadiness(true, 1)
	asset, err = h.service.SetPremiseAssetTrashed(ctx, asset.UUID, true, asset.Revision)
	if err != nil {
		t.Fatal(err)
	}
	assertReadiness(false, 0)
	asset, err = h.service.SetPremiseAssetTrashed(ctx, asset.UUID, false, asset.Revision)
	if err != nil {
		t.Fatal(err)
	}
	assertReadiness(true, 1)
	if err := h.service.store.DB().Exec(`UPDATE premise_assets SET current_variant_id=NULL WHERE uuid=?`, asset.UUID).Error; err != nil {
		t.Fatal(err)
	}
	assertReadiness(false, 0)

	unavailable, err := h.service.ImportPremiseAsset(ctx, CreateAssetInput{
		UploadUUID: upload(t, h.service, "premise_asset", imageBytes(t, 42)), AssetType: AssetProp, Title: "Unavailable prop",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertReadiness(true, 1)
	if err := h.service.store.DB().Exec(`UPDATE files SET deleted_at=? WHERE uuid=?`, time.Now().UTC(), unavailable.CurrentVariant.Asset.UUID).Error; err != nil {
		t.Fatal(err)
	}
	assertReadiness(false, 0)
}

func TestPremiseAssetPermanentDeleteProtectsActiveWorkAndSoftDeletesUnreferencedFiles(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	asset, err := h.service.ImportPremiseAsset(ctx, CreateAssetInput{
		UploadUUID: upload(t, h.service, "premise_asset", imageBytes(t, 61)),
		AssetType:  AssetCharacter,
		Title:      "Temporary hero",
	})
	if err != nil {
		t.Fatal(err)
	}
	fileUUID := asset.CurrentVariant.Asset.UUID
	trashed, err := h.service.SetPremiseAssetTrashed(ctx, asset.UUID, true, asset.Revision)
	if err != nil {
		t.Fatal(err)
	}
	var projectID int64
	if err := h.service.store.DB().Table("projects").Where("uuid = ?", h.project.UUID).Pluck("id", &projectID).Error; err != nil {
		t.Fatal(err)
	}
	taskUUID := mustUUID(t)
	now := time.Now().UTC()
	snapshot := `{"asset_uuid":"` + asset.UUID + `","file_uuid":"` + fileUUID + `"}`
	if err := h.service.store.DB().Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,progress,attempt,max_attempts,created_at,updated_at) VALUES(?,?,'premise_asset_generation',?,?,'running',?,5,1,3,?,?)`, taskUUID, projectID, asset.UUID, snapshot, "delete-active-"+taskUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.PermanentlyDeletePremiseAsset(ctx, asset.UUID, trashed.Revision); !productionErrorIs(err, CodeDeleteBlocked) {
		t.Fatalf("active task permanent delete error=%v", err)
	}
	if err := h.service.store.DB().Table("production_task_runs").Where("uuid = ?", taskUUID).Updates(map[string]any{"status": "completed", "progress": 100, "completed_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	result, err := h.service.PermanentlyDeletePremiseAsset(ctx, asset.UUID, trashed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedCount != 1 || result.FileSoftDeletedCount != 0 || result.RetainedFileCount != 1 {
		t.Fatalf("audit-referenced delete result=%+v", result)
	}
	if _, err := h.service.GetPremiseAsset(ctx, asset.UUID); !productionErrorIs(err, CodeNotFound) {
		t.Fatalf("permanently deleted asset error=%v", err)
	}
	if _, err := h.service.Files().GetAsset(ctx, fileUUID, false); err != nil {
		t.Fatalf("historical task snapshot file was not retained: %v", err)
	}
}

func TestEmptyPremiseAssetTrashDeletesSafeItemsAndReturnsStableBlockers(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	createTrashed := func(title string, seed uint8) PremiseAsset {
		asset, err := h.service.ImportPremiseAsset(ctx, CreateAssetInput{UploadUUID: upload(t, h.service, "premise_asset", imageBytes(t, seed)), AssetType: AssetProp, Title: title})
		if err != nil {
			t.Fatal(err)
		}
		trashed, err := h.service.SetPremiseAssetTrashed(ctx, asset.UUID, true, asset.Revision)
		if err != nil {
			t.Fatal(err)
		}
		return trashed
	}
	safe := createTrashed("Safe prop", 71)
	blocked := createTrashed("Busy prop", 72)
	var projectID int64
	if err := h.service.store.DB().Table("projects").Where("uuid = ?", h.project.UUID).Pluck("id", &projectID).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	taskUUID := mustUUID(t)
	if err := h.service.store.DB().Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,progress,attempt,max_attempts,created_at,updated_at) VALUES(?,?,'premise_asset_generation',?,'{}','queued',?,0,0,3,?,?)`, taskUUID, projectID, blocked.UUID, "delete-blocked-"+taskUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	result, err := h.service.EmptyPremiseAssetTrash(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedCount != 1 || result.FileSoftDeletedCount != 1 || result.RetainedFileCount != 0 || len(result.BlockedItems) != 1 || result.BlockedItems[0].UUID != blocked.UUID || result.BlockedItems[0].Reason != "active_task" {
		t.Fatalf("empty trash result=%+v", result)
	}
	if _, err := h.service.GetPremiseAsset(ctx, safe.UUID); !productionErrorIs(err, CodeNotFound) {
		t.Fatalf("safe asset still exists: %v", err)
	}
	remaining, err := h.service.ListPremiseAssets(ctx, "", "trashed")
	if err != nil || len(remaining) != 1 || remaining[0].UUID != blocked.UUID {
		t.Fatalf("remaining trash=%+v err=%v", remaining, err)
	}
}

func TestChatGeneratedVariantRevisionConflictKeepsFileRetryable(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	asset, err := h.service.ImportPremiseAsset(ctx, CreateAssetInput{UploadUUID: upload(t, h.service, "premise_asset", imageBytes(t, 12)), AssetType: AssetCharacter, Title: "Hero"})
	if err != nil {
		t.Fatal(err)
	}
	chatThreadUUID, _ := newUUIDv7()
	generatedUpload, err := h.service.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "project_chat_asset_reference_image", OriginalFilename: "generated.png", Metadata: map[string]any{"chat_thread_uuid": chatThreadUUID, "premise_asset_uuid": asset.UUID}, Reader: bytes.NewReader(imageBytes(t, 44))})
	if err != nil {
		t.Fatal(err)
	}
	generatedFile, err := h.service.Files().FinalizeUpload(ctx, generatedUpload.UUID, "project_chat_asset_reference_image")
	if err != nil {
		t.Fatal(err)
	}
	staleRevision := asset.Revision
	updatedSummary := "updated elsewhere"
	asset, err = h.service.UpdatePremiseAsset(ctx, asset.UUID, UpdateAssetInput{Summary: &updatedSummary, ExpectedRevision: asset.Revision})
	if err != nil {
		t.Fatal(err)
	}
	executionUUID, _ := newUUIDv7()
	newTitle := "Winter Hero"
	if _, err := h.service.UpdatePremiseAssetFromFile(ctx, asset.UUID, UpdateAssetInput{FileUUID: generatedFile.UUID, ToolExecutionUUID: executionUUID, ChatThreadUUID: chatThreadUUID, Title: &newTitle, ExpectedRevision: staleRevision}); !productionErrorIs(err, CodeConflict) {
		t.Fatalf("stale generated variant error=%v", err)
	}
	variants, err := h.service.ListAssetVariants(ctx, asset.UUID)
	if err != nil || len(variants) != 1 || variants[0].Asset.UUID == generatedFile.UUID {
		t.Fatalf("revision conflict selected the generated file: variants=%+v err=%v", variants, err)
	}
	if _, err := h.service.Files().GetAsset(ctx, generatedFile.UUID, false); err != nil {
		t.Fatalf("revision conflict made generated file unavailable: %v", err)
	}

	replaced, err := h.service.UpdatePremiseAssetFromFile(ctx, asset.UUID, UpdateAssetInput{FileUUID: generatedFile.UUID, ToolExecutionUUID: executionUUID, ChatThreadUUID: chatThreadUUID, Title: &newTitle, ExpectedRevision: asset.Revision})
	if err != nil || replaced.Title != newTitle || replaced.CurrentVariant == nil || replaced.CurrentVariant.Asset.UUID != generatedFile.UUID || replaced.CurrentVariant.VersionNo != 2 {
		t.Fatalf("retried generated variant=%+v err=%v", replaced, err)
	}
	replayed, err := h.service.UpdatePremiseAssetFromFile(ctx, asset.UUID, UpdateAssetInput{FileUUID: generatedFile.UUID, ToolExecutionUUID: executionUUID, ChatThreadUUID: chatThreadUUID, ExpectedRevision: staleRevision})
	if err != nil || replayed.UUID != replaced.UUID || replayed.CurrentVariant.UUID != replaced.CurrentVariant.UUID {
		t.Fatalf("idempotent generated variant replay=%+v err=%v", replayed, err)
	}
}

func TestPremiseSourceIgnoreRestoreUsesRevisionAndBlocksActiveWork(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	source, err := h.service.CreatePremiseSource(ctx, CreateSourceInput{SourceText: "A lighthouse at dusk", StyleSnapshot: "paper cutout", SourceType: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if source.Revision != 0 || source.IgnoredAt != nil {
		t.Fatalf("new source=%+v", source)
	}
	if _, err := h.service.SetPremiseSourceIgnored(ctx, source.UUID, true, source.Revision); !productionErrorIs(err, CodeStateConflict) {
		t.Fatalf("source without setting image ignore error=%v", err)
	}
	settingUpload := upload(t, h.service, "premise_setting_image", imageBytes(t, 20))
	if _, err := h.service.ImportSettingImage(ctx, settingUpload, source.UUID, "manual setting"); err != nil {
		t.Fatal(err)
	}
	var projectID int64
	if err := h.service.store.DB().Table("projects").Where("uuid = ?", h.project.UUID).Pluck("id", &projectID).Error; err != nil {
		t.Fatal(err)
	}
	taskUUID := mustUUID(t)
	now := time.Now().UTC()
	if err := h.service.store.DB().Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,progress,attempt,max_attempts,created_at,updated_at) VALUES(?,?,'premise_setting_generation',?,'{}','running',?,5,1,3,?,?)`, taskUUID, projectID, source.UUID, "ignore-active-"+taskUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.SetPremiseSourceIgnored(ctx, source.UUID, true, source.Revision); !productionErrorIs(err, CodeStateConflict) {
		t.Fatalf("active source task ignore error=%v", err)
	}
	if err := h.service.store.DB().Table("production_task_runs").Where("uuid = ?", taskUUID).Updates(map[string]any{"status": "completed", "progress": 100, "completed_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	ignored, err := h.service.SetPremiseSourceIgnored(ctx, source.UUID, true, source.Revision)
	if err != nil || ignored.IgnoredAt == nil || ignored.Revision != 1 {
		t.Fatalf("ignored=%+v err=%v", ignored, err)
	}
	idempotent, err := h.service.SetPremiseSourceIgnored(ctx, source.UUID, true, source.Revision)
	if err != nil || idempotent.Revision != ignored.Revision {
		t.Fatalf("idempotent ignore=%+v err=%v", idempotent, err)
	}
	if _, err := h.service.SetPremiseSourceIgnored(ctx, source.UUID, false, 0); !productionErrorIs(err, CodeConflict) {
		t.Fatalf("stale restore error=%v", err)
	}
	restored, err := h.service.SetPremiseSourceIgnored(ctx, source.UUID, false, ignored.Revision)
	if err != nil || restored.IgnoredAt != nil || restored.Revision != 2 {
		t.Fatalf("restored=%+v err=%v", restored, err)
	}
	if err := h.service.store.DB().Table("premise_sources").Where("uuid = ?", source.UUID).Update("source_text", "mutated").Error; err == nil {
		t.Fatal("append-only source content was mutable after ignore migration")
	}
}

func productionErrorIs(err error, code string) bool {
	var typed *Error
	return errors.As(err, &typed) && typed.Code == code
}

func TestExportReadinessCountsMissingSectionsAndFreezesAuditSnapshot(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	first, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Opening", Content: "Story", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch02", Title: "Second", Content: "Story", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	readySection, err := h.service.CreateSection(ctx, first.UUID, CreateSectionInput{Title: "Ready", StoryboardMD: "wide shot"})
	if err != nil {
		t.Fatal(err)
	}
	missingFirst, err := h.service.CreateSection(ctx, first.UUID, CreateSectionInput{Title: "Missing", StoryboardMD: "close up"})
	if err != nil {
		t.Fatal(err)
	}
	missingSecond, err := h.service.CreateSection(ctx, second.UUID, CreateSectionInput{Title: "Also missing", StoryboardMD: "ending"})
	if err != nil {
		t.Fatal(err)
	}
	readySection, err = h.service.ImportSectionImage(ctx, first.UUID, readySection.UUID, upload(t, h.service, "comic_section_image", imageBytes(t, 50)), readySection.Revision)
	if err != nil || readySection.CurrentImage == nil {
		t.Fatalf("ready section=%+v err=%v", readySection, err)
	}

	readiness, err := h.service.ExportReadiness(ctx, "project", "")
	if err != nil {
		t.Fatal(err)
	}
	if readiness.ActiveChapterCount != 2 || readiness.ActiveSectionCount != 3 || readiness.ImageSectionCount != 1 || readiness.MissingSectionCount != 2 || !readiness.CanExport || readiness.Complete {
		t.Fatalf("project readiness=%+v", readiness)
	}
	if readiness.MissingSections[0].UUID != missingFirst.UUID || readiness.MissingSections[0].ChapterUUID != first.UUID || readiness.MissingSections[1].UUID != missingSecond.UUID || readiness.MissingSections[1].ChapterUUID != second.UUID {
		t.Fatalf("missing sections=%+v", readiness.MissingSections)
	}
	if _, _, err := h.service.BuildExportSnapshot(ctx, "project", ""); !productionErrorIs(err, CodeExportIncomplete) {
		t.Fatalf("incomplete export error=%v", err)
	}
	snapshot, _, err := h.service.BuildExportSnapshotWithOptions(ctx, "project", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 3 || snapshot.PictureBook == nil || snapshot.PictureBook.Format != project.PictureBookVertical || !snapshot.AllowMissingImages || snapshot.ActiveChapterCount != 2 || snapshot.SectionCount != 3 || snapshot.ExportedSectionCount != 1 || snapshot.MissingSectionCount != 2 || len(snapshot.MissingSectionUUIDs) != 2 || len(snapshot.Entries) != 1 {
		t.Fatalf("export snapshot=%+v", snapshot)
	}
	secondReadiness, err := h.service.ExportReadiness(ctx, "chapter", second.UUID)
	if err != nil || secondReadiness.ActiveChapterCount != 1 || secondReadiness.ImageSectionCount != 0 || secondReadiness.MissingSectionCount != 1 {
		t.Fatalf("chapter readiness=%+v err=%v", secondReadiness, err)
	}
	if _, _, err := h.service.BuildExportSnapshotWithOptions(ctx, "chapter", second.UUID, true); !productionErrorIs(err, CodeExportEmpty) {
		t.Fatalf("empty export error=%v", err)
	}
}

func TestExportNamingDistinguishesVerticalStripAndPageFormats(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Opening", Content: "Story", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	section, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Page", StoryboardMD: "A page"})
	if err != nil {
		t.Fatal(err)
	}
	section, err = h.service.ImportSectionImage(ctx, chapter.UUID, section.UUID, upload(t, h.service, "comic_section_image", imageBytes(t, 77)), section.Revision)
	if err != nil || section.CurrentImage == nil {
		t.Fatalf("imported section=%+v err=%v", section, err)
	}
	fileUUID := section.CurrentImage.Asset.UUID
	entry := ExportEntry{ChapterCode: "vol01.ch01", ChapterTitle: "Opening", SectionNo: 2, ImageAssetUUID: fileUUID, Extension: "png"}
	for _, test := range []struct {
		name       string
		snapshot   ExportSnapshot
		wantEntry  string
		wantPrefix string
	}{
		{
			name:       "vertical strip",
			snapshot:   ExportSnapshot{Version: 3, PictureBook: &project.PictureBookProfile{Format: project.PictureBookVertical}, Entries: []ExportEntry{entry}},
			wantEntry:  "vol01.ch01/Opening/section-002.png",
			wantPrefix: "comic-project-",
		},
		{
			name:       "classic page",
			snapshot:   ExportSnapshot{Version: 3, PictureBook: &project.PictureBookProfile{Format: project.PictureBookClassic}, Entries: []ExportEntry{entry}},
			wantEntry:  "vol01.ch01/Opening/page-002.png",
			wantPrefix: "picture-book-project-",
		},
		{
			name:       "historical snapshot",
			snapshot:   ExportSnapshot{Version: 2, Entries: []ExportEntry{entry}},
			wantEntry:  "vol01.ch01/Opening/section-002.png",
			wantPrefix: "comic-project-",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			archiveBytes, err := h.service.renderZip(ctx, test.snapshot, nil)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
			if err != nil {
				t.Fatal(err)
			}
			if len(reader.File) != 1 || reader.File[0].Name != test.wantEntry {
				t.Fatalf("entries=%v", reader.File)
			}
			name := safeExportNameForSnapshot("project", "", "0123456789abcdef", test.snapshot)
			if !strings.HasPrefix(name, test.wantPrefix) {
				t.Fatalf("export name=%q", name)
			}
		})
	}
}

func TestComicReorderSnapshotRestoreAndExportIdempotency(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Opening", Content: "Story", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "A", StoryboardMD: "wide shot"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "B", StoryboardMD: "close up"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "C", StoryboardMD: "ending"})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := h.service.ReorderSections(ctx, chapter.UUID, []string{c.UUID, a.UUID, b.UUID})
	if err != nil {
		t.Fatal(err)
	}
	for index, item := range ordered {
		if item.SectionNo != index+1 {
			t.Fatalf("non-contiguous order=%+v", ordered)
		}
	}
	beforeSnapshots, _ := h.service.ListChapterSnapshots(ctx, chapter.UUID)
	if len(beforeSnapshots) < 4 {
		t.Fatalf("snapshots=%d", len(beforeSnapshots))
	}
	target := beforeSnapshots[len(beforeSnapshots)-1]
	if _, err := h.service.RestoreChapterSnapshot(ctx, chapter.UUID, target.UUID); err != nil {
		t.Fatal(err)
	}
	restored, err := h.service.ListSections(ctx, chapter.UUID)
	if err != nil || len(restored) != 1 || restored[0].UUID != a.UUID {
		t.Fatalf("restored=%+v err=%v", restored, err)
	}
	restoredA := restored[0]
	restoredA, err = h.service.ImportSectionImage(ctx, chapter.UUID, restoredA.UUID, upload(t, h.service, "comic_section_image", imageBytes(t, 50)), restoredA.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if restoredA.CurrentImage == nil {
		t.Fatal("current image not set")
	}
	snapshot, hash, err := h.service.BuildExportSnapshot(ctx, "chapter", chapter.UUID)
	if err != nil {
		t.Fatal(err)
	}
	firstTaskUUID := insertRunningExportTask(t, h.service)
	first, err := h.service.CreateExportRecord(ctx, firstTaskUUID, "chapter", chapter.UUID, snapshot, hash)
	if err != nil {
		t.Fatal(err)
	}
	first, err = h.service.RenderAndCommitExport(ctx, first.TaskUUID, nil)
	if err != nil || first.Status != "ready" || first.OutputAsset == nil {
		t.Fatalf("first export=%+v err=%v", first, err)
	}
	if err := h.service.store.DB().Table("production_task_runs").Where("uuid = ?", firstTaskUUID).Updates(map[string]any{"status": "completed", "progress": 100}).Error; err != nil {
		t.Fatal(err)
	}
	progresses := []int{}
	progressTaskUUID := insertRunningExportTask(t, h.service)
	progressExport, err := h.service.CreateExportRecord(ctx, progressTaskUUID, "chapter", chapter.UUID, snapshot, hashJSON([]byte("another-snapshot")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.RenderAndCommitExport(ctx, progressExport.TaskUUID, func(progress int) error {
		progresses = append(progresses, progress)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(progresses) < 3 || progresses[len(progresses)-1] != 95 {
		t.Fatalf("export progresses=%v", progresses)
	}
	for index := 1; index < len(progresses); index++ {
		if progresses[index] < progresses[index-1] {
			t.Fatalf("export progress regressed: %v", progresses)
		}
	}
	if err := h.service.store.DB().Table("production_task_runs").Where("uuid = ?", progressTaskUUID).Updates(map[string]any{"status": "completed", "progress": 100}).Error; err != nil {
		t.Fatal(err)
	}
	secondTaskUUID := insertRunningExportTask(t, h.service)
	second, err := h.service.CreateExportRecord(ctx, secondTaskUUID, "chapter", chapter.UUID, snapshot, hash)
	if err != nil {
		t.Fatal(err)
	}
	frozenTaskSnapshot, err := json.Marshal(GenerationSnapshot{Version: 1, Kind: "comic_export", ProjectUUID: h.service.store.ProjectUUID(), ResourceUUID: chapter.UUID, ChapterUUID: chapter.UUID, Prompt: hash, Parameters: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.service.store.DB().Table("production_task_runs").Where("uuid = ?", secondTaskUUID).Update("input_snapshot", string(frozenTaskSnapshot)).Error; err != nil {
		t.Fatal(err)
	}
	canonical, err := h.service.RenderAndCommitExport(ctx, second.TaskUUID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if canonical.UUID != first.UUID || canonical.OutputAsset.UUID != first.OutputAsset.UUID {
		t.Fatalf("export not idempotent first=%+v canonical=%+v", first, canonical)
	}
	recoveredProgress := 0
	recovered, err := h.service.RenderAndCommitExport(ctx, second.TaskUUID, func(progress int) error {
		recoveredProgress = progress
		return nil
	})
	if err != nil || recovered.UUID != canonical.UUID || recoveredProgress != 95 {
		t.Fatalf("deleted transient export recovery=%+v progress=%d err=%v", recovered, recoveredProgress, err)
	}
	exports, err := h.service.ListExports(ctx)
	canonicalCount := 0
	for _, item := range exports {
		if item.SnapshotHash == hash {
			canonicalCount++
		}
	}
	if err != nil || canonicalCount != 1 {
		t.Fatalf("exports=%v err=%v", exports, err)
	}
}

func TestComicSnapshotDetailSupportsLegacyMediaPlaceholdersAndSafeRestore(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch08", Title: "Snapshot chapter", Content: "Story", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "First", StoryboardMD: "# First storyboard"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Second", StoryboardMD: "# Second storyboard"})
	if err != nil {
		t.Fatal(err)
	}
	premiseAsset, err := h.service.Files().FinalizeUpload(ctx, upload(t, h.service, "comic_section_premise", imageBytes(t, 60)), "comic_section_premise")
	if err != nil {
		t.Fatal(err)
	}
	var sectionID, premiseFileID int64
	var storyboardUUID string
	if err := h.service.store.DB().Raw(`SELECT sections.id,storyboards.uuid FROM comic_sections AS sections JOIN comic_storyboard_variants AS storyboards ON storyboards.id=sections.current_storyboard_variant_id WHERE sections.uuid=?`, first.UUID).Row().Scan(&sectionID, &storyboardUUID); err != nil {
		t.Fatal(err)
	}
	if err := h.service.store.DB().Table("files").Where("uuid=?", premiseAsset.UUID).Pluck("id", &premiseFileID).Error; err != nil {
		t.Fatal(err)
	}
	generationUUID, _ := newUUIDv7()
	generationProviderUUID := mustUUID(t)
	taskUUID, _ := newUUIDv7()
	now := time.Now().UTC()
	var projectID int64
	if err := h.service.store.DB().Table("projects").Where("uuid=?", h.service.store.ProjectUUID()).Pluck("id", &projectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.service.store.DB().Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,progress,attempt,max_attempts,created_at,updated_at) VALUES(?,?,'comic_image_generation',?,'{}','running',?,5,1,3,?,?)`, taskUUID, projectID, first.UUID, "snapshot-image-"+taskUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	generationSnapshot, err := json.Marshal(GenerationSnapshot{ProviderUUID: generationProviderUUID, ProviderType: "openai_compatible", ProviderBaseURL: "https://provider.invalid/private", Model: "image-model", ModelSource: "project_override"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.service.store.DB().Exec(`INSERT INTO comic_image_generations(uuid,comic_section_id,task_uuid,status,input_snapshot,premise_file_id,premise_metadata,created_at) VALUES(?,?,?,'running',?,?, '{}',?)`, generationUUID, sectionID, taskUUID, string(generationSnapshot), premiseFileID, now).Error; err != nil {
		t.Fatal(err)
	}
	first, err = h.service.CommitGeneratedSectionImage(ctx, chapter.UUID, first.UUID, generationUUID, generationSnapshot, bytes.NewReader(imageBytes(t, 61)))
	if err != nil || first.CurrentImage == nil {
		t.Fatalf("generated image=%+v err=%v", first, err)
	}
	if first.CurrentImage.Generation == nil || first.CurrentImage.Generation.UUID != generationUUID || first.CurrentImage.Generation.ProviderUUID != generationProviderUUID || first.CurrentImage.Generation.ProviderType != "openai_compatible" || first.CurrentImage.Generation.Model != "image-model" || first.CurrentImage.Generation.ModelSource != "project_override" || first.CurrentImage.Generation.Status != "completed" {
		t.Fatalf("safe generation summary=%+v", first.CurrentImage.Generation)
	}
	if err := h.service.store.DB().Table("production_task_runs").Where("uuid=?", taskUUID).Updates(map[string]any{"status": "completed", "progress": 100}).Error; err != nil {
		t.Fatal(err)
	}
	var imageFileID int64
	if err := h.service.store.DB().Table("comic_image_variants").Where("uuid=?", first.CurrentImage.UUID).Pluck("file_id", &imageFileID).Error; err != nil {
		t.Fatal(err)
	}

	summaries, err := h.service.ListChapterSnapshots(ctx, chapter.UUID)
	if err != nil || len(summaries) < 3 || summaries[0].SectionCount != 2 {
		t.Fatalf("snapshot summaries=%+v err=%v", summaries, err)
	}
	baseTarget := summaries[0]
	var targetRow chapterSnapshotRecord
	if err := h.service.store.DB().Where("uuid=?", baseTarget.UUID).First(&targetRow).Error; err != nil {
		t.Fatal(err)
	}
	var targetPayload chapterSnapshotPayload
	if err := json.Unmarshal([]byte(targetRow.SnapshotJSON), &targetPayload); err != nil {
		t.Fatal(err)
	}
	targetPayload.Sections[0], targetPayload.Sections[1] = targetPayload.Sections[1], targetPayload.Sections[0]
	reversed, _ := json.Marshal(targetPayload)
	fixtureUUID, _ := newUUIDv7()
	targetRow = chapterSnapshotRecord{UUID: fixtureUUID, ChapterComicStateID: targetRow.ChapterComicStateID, ActorID: targetRow.ActorID, VersionNo: targetRow.VersionNo + 1, Reason: "sorting_fixture", SnapshotJSON: string(reversed), SnapshotHash: hashJSON(reversed), CreatedAt: now}
	if err := h.service.store.DB().Create(&targetRow).Error; err != nil {
		t.Fatal(err)
	}
	target := chapterSnapshotSummary(targetRow, 2)
	detail, err := h.service.GetChapterSnapshot(ctx, chapter.UUID, target.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.SchemaVersion != 2 || detail.Chapter.UUID != chapter.UUID || detail.Chapter.ChapterCode != chapter.ChapterCode || len(detail.Sections) != 2 || detail.Sections[0].UUID != first.UUID || detail.Sections[1].UUID != second.UUID {
		t.Fatalf("sorted detail=%+v", detail)
	}
	if detail.Sections[0].StoryboardMD != "# First storyboard" || detail.Sections[0].CurrentImage.Status != files.ObjectReady || detail.Sections[0].CurrentImage.AssetUUID == "" || detail.Sections[0].PremiseReference.Status != files.ObjectReady {
		t.Fatalf("snapshot media detail=%+v", detail.Sections[0])
	}

	legacyPayload := map[string]any{"version": 1, "sections": []map[string]any{{
		"uuid": first.UUID, "section_no": 1, "title": "Legacy first", "description_md": "legacy fallback", "storyboard_uuid": storyboardUUID, "image_variant_uuid": first.CurrentImage.UUID,
	}}}
	legacyJSON, _ := json.Marshal(legacyPayload)
	legacyUUID, _ := newUUIDv7()
	legacyRow := chapterSnapshotRecord{UUID: legacyUUID, ChapterComicStateID: targetRow.ChapterComicStateID, ActorID: targetRow.ActorID, VersionNo: targetRow.VersionNo + 1, Reason: "legacy_fixture", SnapshotJSON: string(legacyJSON), SnapshotHash: hashJSON(legacyJSON), CreatedAt: now.Add(time.Millisecond)}
	if err := h.service.store.DB().Create(&legacyRow).Error; err != nil {
		t.Fatal(err)
	}
	legacy := chapterSnapshotSummary(legacyRow, 1)
	legacyDetail, err := h.service.GetChapterSnapshot(ctx, chapter.UUID, legacy.UUID)
	if err != nil || legacyDetail.SchemaVersion != 1 || legacyDetail.Chapter.UUID != chapter.UUID || len(legacyDetail.Sections) != 1 || legacyDetail.Sections[0].StoryboardMD != "# First storyboard" {
		t.Fatalf("legacy detail=%+v err=%v", legacyDetail, err)
	}

	if err := h.service.store.DB().Exec(`UPDATE file_objects SET state='missing' WHERE id IN (SELECT file_object_id FROM files WHERE id IN (?,?))`, imageFileID, premiseFileID).Error; err != nil {
		t.Fatal(err)
	}
	missingDetail, err := h.service.GetChapterSnapshot(ctx, chapter.UUID, target.UUID)
	if err != nil || missingDetail.Sections[0].CurrentImage.Status != files.ObjectMissing || missingDetail.Sections[0].PremiseReference.Status != files.ObjectMissing {
		t.Fatalf("missing media detail=%+v err=%v", missingDetail, err)
	}

	activeTaskUUID, _ := newUUIDv7()
	if err := h.service.store.DB().Exec(`INSERT INTO task_runs(uuid,project_id,kind,resource_uuid,input_version,input_snapshot,status,idempotency_key,provider_uuid,model,max_attempts,created_at,updated_at) VALUES(?,?,'comic_storyboard_generation',?,2,'{}','queued',?,?, 'snapshot-test',3,?,?)`, activeTaskUUID, projectID, chapter.UUID, "snapshot-active-"+activeTaskUUID, activeTaskUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.RestoreChapterSnapshot(ctx, chapter.UUID, legacy.UUID); !productionErrorIs(err, CodeSnapshotBusy) {
		t.Fatalf("active task restore error=%v", err)
	}
	unchanged, _ := h.service.ListSections(ctx, chapter.UUID)
	if len(unchanged) != 2 {
		t.Fatalf("blocked restore changed sections=%+v", unchanged)
	}
	if err := h.service.store.DB().Table("task_runs").Where("uuid=?", activeTaskUUID).Updates(map[string]any{"status": "failed", "retryable": false}).Error; err != nil {
		t.Fatal(err)
	}
	storyTaskUUID, _ := newUUIDv7()
	if err := h.service.store.DB().Exec(`INSERT INTO task_runs(uuid,project_id,kind,resource_uuid,input_version,input_snapshot,status,idempotency_key,provider_uuid,model,max_attempts,created_at,updated_at) VALUES(?,?,'story_chapter_generation',?,2,'{}','running',?,?,'snapshot-test',3,?,?)`, storyTaskUUID, projectID, chapter.UUID, "snapshot-story-"+storyTaskUUID, storyTaskUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.RestoreChapterSnapshot(ctx, chapter.UUID, legacy.UUID); !productionErrorIs(err, CodeSnapshotBusy) {
		t.Fatalf("active story restore error=%v", err)
	}
	if err := h.service.store.DB().Table("task_runs").Where("uuid=?", storyTaskUUID).Updates(map[string]any{"status": "failed", "retryable": false}).Error; err != nil {
		t.Fatal(err)
	}
	imageTaskUUID, _ := newUUIDv7()
	if err := h.service.store.DB().Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,progress,attempt,max_attempts,created_at,updated_at) VALUES(?,?,'comic_image_generation',?,'{}','running',?,5,1,3,?,?)`, imageTaskUUID, projectID, first.UUID, "snapshot-production-"+imageTaskUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.RestoreChapterSnapshot(ctx, chapter.UUID, legacy.UUID); !productionErrorIs(err, CodeSnapshotBusy) {
		t.Fatalf("active image restore error=%v", err)
	}
	if err := h.service.store.DB().Table("production_task_runs").Where("uuid=?", imageTaskUUID).Update("status", "failed").Error; err != nil {
		t.Fatal(err)
	}
	beforeRestore, _ := h.service.ListChapterSnapshots(ctx, chapter.UUID)
	restored, err := h.service.RestoreChapterSnapshot(ctx, chapter.UUID, legacy.UUID)
	if err != nil || len(restored) != 1 || restored[0].UUID != first.UUID {
		t.Fatalf("restored=%+v err=%v", restored, err)
	}
	afterRestore, _ := h.service.ListChapterSnapshots(ctx, chapter.UUID)
	if len(afterRestore) != len(beforeRestore)+1 || afterRestore[0].Reason != "snapshot_restored" || afterRestore[0].SectionCount != 1 {
		t.Fatalf("restore snapshot history before=%d after=%+v", len(beforeRestore), afterRestore)
	}
}

func mustUUID(t *testing.T) string {
	t.Helper()
	value, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func insertRunningExportTask(t *testing.T, service *Service) string {
	t.Helper()
	taskUUID := mustUUID(t)
	now := time.Now().UTC()
	var projectID int64
	if err := service.store.DB().Table("projects").Where("uuid = ?", service.store.ProjectUUID()).Pluck("id", &projectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.store.DB().Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,progress,attempt,max_attempts,created_at,updated_at) VALUES(?,?,'comic_export',?,'{}','running',?,5,1,3,?,?)`, taskUUID, projectID, service.store.ProjectUUID(), taskUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	return taskUUID
}

func TestAppendOnlyProductionHistoryTriggers(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Opening"})
	if err != nil {
		t.Fatal(err)
	}
	section, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{StoryboardMD: "frame"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.service.store.DB().Exec("UPDATE comic_storyboard_variants SET content_md='mutated' WHERE uuid=?", section.CurrentStoryboard.UUID).Error; err == nil {
		t.Fatal("storyboard append-only trigger allowed update")
	}
	asset, err := h.service.ImportPremiseAsset(ctx, CreateAssetInput{UploadUUID: upload(t, h.service, "premise_asset", imageBytes(t, 1)), AssetType: "prop", Title: "Kite"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.service.store.DB().Exec("UPDATE premise_asset_variants SET source_type='manual' WHERE uuid=?", asset.CurrentVariant.UUID).Error; err == nil {
		t.Fatal("premise variant append-only trigger allowed update")
	}
	if err := h.service.store.DB().Exec("DELETE FROM comic_section_events WHERE comic_section_id=(SELECT id FROM comic_sections WHERE uuid=?)", section.UUID).Error; err == nil {
		t.Fatal("append-only section event accepted a direct delete")
	}
	trashed, err := h.stories.TrashChapter(ctx, chapter.UUID, chapter.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.stories.PermanentlyDeleteChapter(ctx, chapter.UUID, trashed.Revision); err != nil {
		t.Fatalf("chapter cascade should remove its comic history: %v", err)
	}
	var comicStates int64
	if err := h.service.store.DB().Table("chapter_comic_states").Count(&comicStates).Error; err != nil || comicStates != 0 {
		t.Fatalf("comic states after chapter delete=%d err=%v", comicStates, err)
	}
}
