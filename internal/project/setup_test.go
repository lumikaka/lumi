package project

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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
	if initial.UUID != setupUUID || initial.Revision != 1 || initial.OriginalInput != originalInput || initial.FieldSources["generation_language"] != SetupSourceSystemDefault || initial.FieldSources["generation_brief"] != SetupSourceSystemDefault {
		t.Fatalf("initial setup=%+v", initial)
	}
	if initial.DraftValues.GenerationLanguage != GenerationLanguageSimplifiedChinese || initial.DraftValues.GenerationBrief != strings.TrimSpace(originalInput) {
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
	generationBrief := "小狐狸穿过夜色森林，把一封信送给月亮，并带回温暖的回信。"
	profileInput := &PictureBookInput{Format: PictureBookClassic}
	updated, err := store.UpdateProjectSetupDraft(ctx, SetupDraftPatchInput{
		ExpectedRevision: initial.Revision, ProjectName: &projectName,
		OverallStyle: &overallStyle, GenerationBrief: &generationBrief, PictureBook: profileInput,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.Status != SetupDraftStatusPendingConfirmation || len(updated.MissingInformation) != 0 {
		t.Fatalf("updated setup=%+v", updated)
	}
	if updated.DraftValues.ProjectName != projectName || updated.DraftValues.OverallStyle != overallStyle || updated.DraftValues.GenerationBrief != generationBrief || updated.DraftValues.PictureBook == nil {
		t.Fatalf("updated draft values=%+v", updated.DraftValues)
	}
	if updated.FieldSources["project_name"] != SetupSourceAgentProposed || updated.FieldSources["generation_brief"] != SetupSourceAgentProposed || updated.FieldSources["aspect_ratio"] != SetupSourceSystemDefault {
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
	if finalized.DraftValues.GenerationBrief != generationBrief || finalized.FieldSources["generation_brief"] != SetupSourceUserConfirmed {
		t.Fatalf("finalized generation brief=%q sources=%+v", finalized.DraftValues.GenerationBrief, finalized.FieldSources)
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

func TestProjectSetupReferencePlanIsRevisionedAndFreezesAtFinalization(t *testing.T) {
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
	created, err := manager.CreateDraftAt(ctx, DraftCreateInput{
		ProjectUUID: projectUUID, SetupUUID: setupUUID, RootPath: root,
		InitialInput: "Create a picture book from this fox reference.", GenerationLanguage: GenerationLanguageEnglish,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := openStoreForTest(t, manager, created.UUID)
	now := time.Now().UTC()
	objectUUID, fileUUID, bindingUUID := setupTestUUID(t), setupTestUUID(t), setupTestUUID(t)
	sessionUUID, sourceReferenceUUID := setupTestUUID(t), setupTestUUID(t)
	if err := store.DB().Exec(`INSERT INTO file_objects(uuid,project_id,sha256,key_path,mime_type,canonical_ext,byte_size,width,height,state,created_at,verified_at) VALUES(?,?,?,'objects/setup-reference.png','image/png','png',128,16,16,'ready',?,?)`, objectUUID, store.project.ID, "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Exec(`INSERT INTO files(uuid,project_id,file_object_id,kind,purpose,original_filename,display_name,source_type,metadata_json,created_at) VALUES(?,?,(SELECT id FROM file_objects WHERE uuid=?),'image','project_chatbot_reference','fox.png','Fox','imported','{}',?)`, fileUUID, store.project.ID, objectUUID, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Exec(`INSERT INTO project_creation_reference_files(uuid,project_id,creation_session_uuid,reference_uuid,position,file_id,reference_role,title,instruction,include_in_yolo,plan_source,created_at,updated_at) VALUES(?,?,?,?,1,(SELECT id FROM files WHERE uuid=?),'auto','','',1,'system_default',?,?)`, bindingUUID, store.project.ID, sessionUUID, sourceReferenceUUID, fileUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}

	initial, err := store.ProjectSetup(ctx)
	if err != nil || len(initial.ReferencePlan.Items) != 1 {
		t.Fatalf("initial setup=%+v err=%v", initial, err)
	}
	reference := initial.ReferencePlan.Items[0]
	if reference.UUID != bindingUUID || reference.FileUUID != fileUUID || reference.Title != "fox" || reference.ReferenceRole != "auto" || !reference.IncludeInYolo || reference.PlanSource != SetupSourceSystemDefault || reference.ThumbnailURL == "" {
		t.Fatalf("initial reference=%+v", reference)
	}
	if _, err := store.UpdateProjectSetupReference(ctx, bindingUUID, SetupReferencePatchInput{ExpectedRevision: initial.Revision}); errorCode(err) != CodeProjectSetupReferenceSystemManaged {
		t.Fatalf("empty reference update error=%v", err)
	}
	foreignTitle := "Foreign"
	if _, err := store.UpdateProjectSetupReference(ctx, setupTestUUID(t), SetupReferencePatchInput{ExpectedRevision: initial.Revision, Title: &foreignTitle}); errorCode(err) != CodeProjectSetupReferenceSystemManaged {
		t.Fatalf("foreign reference update error=%v", err)
	}
	role, title, instruction, included := "style", "Watercolor fox", "Use only the brushwork and palette", false
	if _, err := store.UpdateProjectSetupReference(ctx, bindingUUID, SetupReferencePatchInput{
		ExpectedRevision: initial.Revision, ReferenceRole: &role, Title: &title, Instruction: &instruction,
		IncludeInYolo: &included, Source: SetupSourceAgentProposed,
	}); errorCode(err) != CodeProjectSetupReferenceSystemManaged {
		t.Fatalf("custom reference update error=%v", err)
	}

	projectName, overallStyle := "Moon Fox", "Soft transparent watercolor"
	completedDraft, err := store.UpdateProjectSetupDraft(ctx, SetupDraftPatchInput{ExpectedRevision: initial.Revision, ProjectName: &projectName, OverallStyle: &overallStyle, PictureBook: &PictureBookInput{Format: PictureBookClassic}})
	if err != nil {
		t.Fatal(err)
	}
	finalized, err := store.FinalizeProjectSetup(ctx, completedDraft.Revision)
	if err != nil || finalized.SetupStatus != SetupStatusReady || finalized.ReferencePlan.Items[0].PlanSource != SetupSourceSystemDefault {
		t.Fatalf("finalized setup=%+v err=%v", finalized, err)
	}
	if _, err := store.UpdateProjectSetupReference(ctx, bindingUUID, SetupReferencePatchInput{ExpectedRevision: finalized.Revision, IncludeInYolo: &included}); errorCode(err) != CodeProjectSetupReferenceSystemManaged {
		t.Fatalf("ready reference update error=%v", err)
	}
}
