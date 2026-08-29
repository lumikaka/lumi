package project

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func setupTestUUID(t *testing.T) string {
	t.Helper()
	value, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return value.String()
}

func TestDraftProjectSetupLifecycleIsRecoverableCanonicalAndImmutable(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	parent := filepath.Join(t.TempDir(), "Documents", "Lumi")
	previousResolver := resolveDefaultProjectParentDir
	resolveDefaultProjectParentDir = func() (string, error) { return parent, nil }
	t.Cleanup(func() { resolveDefaultProjectParentDir = previousResolver })

	root, err := manager.PlanDraftProjectRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	projectUUID, setupUUID := setupTestUUID(t), setupTestUUID(t)
	originalInput := "  我要一本水彩风格、讲小狐狸给月亮送信的儿童绘本。\n"
	created, err := manager.CreateDraftAt(ctx, DraftCreateInput{
		ProjectUUID: projectUUID, SetupUUID: setupUUID, RootPath: root,
		InitialInput: originalInput, GenerationLanguage: GenerationLanguageSimplifiedChinese,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.SetupStatus != SetupStatusDraft || created.PictureBook != nil || filepath.Base(created.RootPath) != projectDirectoryName(DraftProjectPlaceholderName) {
		t.Fatalf("draft summary=%+v", created)
	}

	store := openStoreForTest(t, manager, projectUUID)
	var profileCount int64
	if err := store.DB().Model(&pictureBookProfileRecord{}).Where("project_id = ?", store.project.ID).Count(&profileCount).Error; err != nil || profileCount != 0 {
		t.Fatalf("draft profile count=%d err=%v", profileCount, err)
	}
	if errorCode(store.RequireReady()) != CodeProjectSetupIncomplete {
		t.Fatalf("RequireReady error=%v", store.RequireReady())
	}
	initial, err := store.ProjectSetup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if initial.UUID != setupUUID || initial.Revision != 1 || initial.OriginalInput != originalInput || initial.FieldSources["generation_language"] != SetupSourceSystemDefault {
		t.Fatalf("initial setup=%+v", initial)
	}
	if initial.DraftValues.GenerationLanguage != GenerationLanguageSimplifiedChinese {
		t.Fatalf("initial draft values=%+v", initial.DraftValues)
	}
	if _, err := store.FinalizeProjectSetup(ctx, initial.Revision); errorCode(err) != CodeProjectSetupInvalid {
		t.Fatalf("incomplete finalization error=%v", err)
	}

	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	reopened, err := manager.OpenRecent(ctx, projectUUID)
	if err != nil || reopened.SetupStatus != SetupStatusDraft || reopened.PictureBook != nil {
		t.Fatalf("reopened=%+v err=%v", reopened, err)
	}
	store = openStoreForTest(t, manager, projectUUID)
	projectName, overallStyle := "月亮信使", "透明水彩、柔和月光、纸张颗粒"
	profileInput := &PictureBookInput{Format: PictureBookClassic}
	updated, err := store.UpdateProjectSetupDraft(ctx, SetupDraftPatchInput{
		ExpectedRevision: initial.Revision, ProjectName: &projectName,
		OverallStyle: &overallStyle, PictureBook: profileInput,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.Status != SetupDraftStatusPendingConfirmation || len(updated.MissingInformation) != 0 {
		t.Fatalf("updated setup=%+v", updated)
	}
	if updated.DraftValues.ProjectName != projectName || updated.DraftValues.OverallStyle != overallStyle || updated.DraftValues.PictureBook == nil {
		t.Fatalf("updated draft values=%+v", updated.DraftValues)
	}
	if updated.FieldSources["project_name"] != SetupSourceAgentProposed || updated.FieldSources["aspect_ratio"] != SetupSourceSystemDefault {
		t.Fatalf("sources=%+v", updated.FieldSources)
	}
	if _, err := store.UpdateProjectSetupDraft(ctx, SetupDraftPatchInput{ExpectedRevision: initial.Revision, ProjectName: &projectName}); errorCode(err) != CodeProjectSetupConflict {
		t.Fatalf("stale update error=%v", err)
	}

	finalized, err := store.FinalizeProjectSetup(ctx, updated.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if finalized.SetupStatus != SetupStatusReady || finalized.Status != SetupDraftStatusFinalized || finalized.FinalPictureBook == nil {
		t.Fatalf("finalized=%+v", finalized)
	}
	wantProfile, err := NormalizePictureBookInput(profileInput)
	if err != nil || !reflect.DeepEqual(*finalized.FinalPictureBook, wantProfile) {
		t.Fatalf("final profile=%+v want=%+v err=%v", finalized.FinalPictureBook, wantProfile, err)
	}
	if replay, err := store.FinalizeProjectSetup(ctx, updated.Revision); err != nil || !reflect.DeepEqual(replay.FinalPictureBook, finalized.FinalPictureBook) {
		t.Fatalf("idempotent replay=%+v err=%v", replay, err)
	}
	if _, err := store.FinalizeProjectSetup(ctx, updated.Revision+1); errorCode(err) != CodeProjectSetupConflict {
		t.Fatalf("different replay error=%v", err)
	}
	if err := store.DB().Model(&pictureBookProfileRecord{}).Where("project_id = ?", store.project.ID).Count(&profileCount).Error; err != nil || profileCount != 1 {
		t.Fatalf("ready profile count=%d err=%v", profileCount, err)
	}
	if err := store.DB().Model(&pictureBookProfileRecord{}).Where("project_id = ?", store.project.ID).Update("format", PictureBookWordless).Error; err == nil {
		t.Fatal("immutable formal profile accepted an update")
	}
	if err := store.DB().Where("project_id = ?", store.project.ID).Delete(&pictureBookProfileRecord{}).Error; err == nil {
		t.Fatal("immutable formal profile accepted a delete")
	}
}

func TestManualProjectCreationRemainsReady(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	created, err := manager.Create(ctx, "Manual ready", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if created.SetupStatus != SetupStatusReady || created.PictureBook == nil {
		t.Fatalf("manual project=%+v", created)
	}
}
