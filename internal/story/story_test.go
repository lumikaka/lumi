package story

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
	"lumi/internal/promptcatalog"
)

type storyEventRecorder struct {
	events   []string
	payloads []map[string]any
}

func (recorder *storyEventRecorder) Broadcast(_ string, event string, payload any) {
	recorder.events = append(recorder.events, event)
	if value, ok := payload.(map[string]any); ok {
		recorder.payloads = append(recorder.payloads, value)
	}
}

func storyHarness(t *testing.T) (*project.Manager, project.Summary, *Service) {
	t.Helper()
	dataDirectory := filepath.Join(t.TempDir(), "app")
	appStore, err := appstore.Open(dataDirectory, config.SQLiteDSN(filepath.Join(dataDirectory, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	manager := project.NewManager(appStore).WithOpenHook(ReconcileOnOpen)
	created, err := manager.Create(context.Background(), "Story Test", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	var service *Service
	if err := manager.WithCurrentStore(context.Background(), created.UUID, func(store *project.Store) error {
		service = NewService(store)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = manager.Close()
		_ = appStore.Close()
	})
	return manager, created, service
}

func storyErrorCode(err error) string {
	var domainError *Error
	if errors.As(err, &domainError) {
		return domainError.Code
	}
	return ""
}

func replacePromptWithPreviousBuiltin(t *testing.T, service *Service, group, key string) (promptcatalog.Definition, promptVersionRecord) {
	t.Helper()
	ctx := context.Background()
	projectRecord, actor, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := service.promptDefinition(group, key, projectRecord.GenerationLanguage)
	if !ok || len(definition.PreviousDefaultValues) != 1 {
		t.Fatalf("%s/%s previous defaults=%v", group, key, definition.PreviousDefaultValues)
	}
	if err := service.store.DB().Where("project_id = ? AND prompt_group = ? AND prompt_key = ?", projectRecord.ID, group, key).Delete(&promptVersionRecord{}).Error; err != nil {
		t.Fatal(err)
	}
	uuid, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	prompt := strings.TrimSpace(definition.PreviousDefaultValues[0])
	record := promptVersionRecord{
		UUID: uuid, ProjectID: projectRecord.ID, ActorID: actor.ID,
		PromptGroup: group, PromptKey: key, VersionNo: 1,
		Prompt: prompt, PromptHash: contentHash(prompt), SourceType: "project_created", CreatedAt: service.now().UTC(),
	}
	if err := service.store.DB().Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	return definition, record
}

func TestChapterStoriesAreAppendOnlyAndRevisionChecked(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	created, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "VOL01.CH01", Title: "Opening", Content: "First version", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ChapterCode != "vol01.ch01" || created.Revision != 1 || created.CurrentStory == nil || created.CurrentStory.VersionNo != 1 {
		t.Fatalf("created chapter = %+v", created)
	}
	updated, err := service.UpdateStory(ctx, created.UUID, UpdateStoryInput{Content: "Second version", ContentFormat: "md", ExpectedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.CurrentStory == nil || updated.CurrentStory.VersionNo != 2 {
		t.Fatalf("updated chapter = %+v", updated)
	}
	versions, err := service.ListChapterStories(ctx, created.UUID)
	if err != nil || len(versions) != 2 || versions[0].Content != "Second version" || versions[1].Content != "First version" {
		t.Fatalf("versions = %+v, error = %v", versions, err)
	}
	if _, err := service.UpdateStory(ctx, created.UUID, UpdateStoryInput{Content: "Stale edit", ContentFormat: "txt", ExpectedRevision: 1}); storyErrorCode(err) != CodeChapterRevisionConflict {
		t.Fatalf("stale update error = %v", err)
	}
	if err := service.store.DB().Model(&chapterStoryRecord{}).Where("uuid = ?", versions[1].UUID).Update("content", "mutated").Error; err == nil {
		t.Fatal("append-only trigger allowed chapter story mutation")
	}
}

func TestChapterOrderAndStoryProfileRestoreUsePublicResourcesAndEmitHints(t *testing.T) {
	_, _, service := storyHarness(t)
	recorder := &storyEventRecorder{}
	service.WithEvents(recorder)
	ctx := context.Background()
	first, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol01.ch11", Title: "First"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol01.ch12", Title: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	third, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol01.ch13", Title: "Third"})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := service.ReorderChapters(ctx, []string{third.UUID, first.UUID, second.UUID})
	if err != nil {
		t.Fatal(err)
	}
	if len(ordered) != 3 || ordered[0].UUID != third.UUID || ordered[1].UUID != first.UUID || ordered[2].UUID != second.UUID || ordered[0].SortOrder != 1 {
		t.Fatalf("ordered chapters=%+v", ordered)
	}
	if _, err := service.ReorderChapters(ctx, []string{first.UUID, first.UUID, second.UUID}); storyErrorCode(err) != CodeValidationFailed {
		t.Fatalf("duplicate order error=%v", err)
	}
	profile, err := service.GetStoryProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	versionTwo, err := service.UpdateStoryProfile(ctx, "# Version two\n\nA complete story.\n", profile.Revision)
	if err != nil {
		t.Fatal(err)
	}
	versionThree, err := service.UpdateStoryProfile(ctx, "# Version three\n\nA different story.\n", versionTwo.Revision)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := service.RestoreStoryProfile(ctx, versionTwo.UUID, versionThree.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if restored.VersionNo != versionThree.VersionNo+1 || restored.StoryMD != versionTwo.StoryMD || restored.UUID == versionTwo.UUID {
		t.Fatalf("restored profile=%+v", restored)
	}
	if _, err := service.RestoreStoryProfile(ctx, first.UUID, restored.Revision); storyErrorCode(err) != CodeStoryProfileNotFound {
		t.Fatalf("cross-resource restore error=%v", err)
	}
	wanted := map[string]bool{"story:chapter_changed": false, "story:chapters_reordered": false, "story:profile_changed": false}
	for _, event := range recorder.events {
		if _, exists := wanted[event]; exists {
			wanted[event] = true
		}
	}
	for event, found := range wanted {
		if !found {
			t.Fatalf("missing realtime hint %s in %v", event, recorder.events)
		}
	}
	for _, payload := range recorder.payloads {
		if _, leaked := payload["id"]; leaked {
			t.Fatalf("realtime payload leaked internal id: %#v", payload)
		}
	}
}

func TestChapterTrashRestoreConflictAndPermanentDelete(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	original, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol01.ch02", Title: "Original", Content: "Body", ContentFormat: "txt"})
	if err != nil {
		t.Fatal(err)
	}
	trashed, err := service.TrashChapter(ctx, original.UUID, original.Revision)
	if err != nil || trashed.TrashedAt == nil || trashed.Revision != original.Revision+1 {
		t.Fatalf("trashed = %+v, error = %v", trashed, err)
	}
	replacement, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol01.ch02", Title: "Replacement"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RestoreChapter(ctx, original.UUID, trashed.Revision); storyErrorCode(err) != CodeChapterConflict {
		t.Fatalf("restore conflict error = %v", err)
	}
	if err := service.PermanentlyDeleteChapter(ctx, original.UUID, trashed.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetChapter(ctx, original.UUID); storyErrorCode(err) != CodeChapterNotFound {
		t.Fatalf("deleted chapter error = %v", err)
	}
	if replacement.TrashedAt != nil {
		t.Fatalf("replacement unexpectedly trashed: %+v", replacement)
	}
	rows, err := service.store.DB().Raw("PRAGMA foreign_key_check").Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check reported a violation after permanent delete")
	}
}

func TestChapterPermanentDeleteHonorsActiveAndQueuedComicSectionReferences(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	chapter, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol01.ch03", Title: "Referenced section"})
	if err != nil {
		t.Fatal(err)
	}
	chapter, err = service.TrashChapter(ctx, chapter.UUID, chapter.Revision)
	if err != nil {
		t.Fatal(err)
	}
	var projectID, chapterID, actorID int64
	if err := service.store.DB().Table("projects").Where("uuid=?", service.store.ProjectUUID()).Pluck("id", &projectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.store.DB().Table("chapters").Where("uuid=?", chapter.UUID).Pluck("id", &chapterID).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.store.DB().Table("actors").Order("id").Limit(1).Pluck("id", &actorID).Error; err != nil || actorID == 0 {
		t.Fatalf("actor id=%d err=%v", actorID, err)
	}
	stateUUID, _ := newUUIDv7()
	sectionUUID, _ := newUUIDv7()
	threadUUID, _ := newUUIDv7()
	turnUUID, _ := newUUIDv7()
	itemUUID, _ := newUUIDv7()
	followUpUUID, _ := newUUIDv7()
	providerUUID, _ := newUUIDv7()
	now := service.now().UTC()
	if err := service.store.DB().Exec(`INSERT INTO chapter_comic_states(uuid,chapter_id,status,revision,created_at,updated_at) VALUES(?,?,'draft',1,?,?)`, stateUUID, chapterID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	var stateID int64
	if err := service.store.DB().Table("chapter_comic_states").Where("uuid=?", stateUUID).Pluck("id", &stateID).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.store.DB().Exec(`INSERT INTO comic_sections(uuid,chapter_comic_state_id,actor_id,section_no,title,description_md,revision,created_at,updated_at) VALUES(?,?,?,1,'Referenced','Frozen snapshot',1,?,?)`, sectionUUID, stateID, actorID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	var sectionID int64
	if err := service.store.DB().Table("comic_sections").Where("uuid=?", sectionUUID).Pluck("id", &sectionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.store.DB().Exec(`INSERT INTO chat_threads(uuid,project_id,title,status,provider_uuid,model,model_source,created_at,updated_at) VALUES(?,?,'Reference retention','busy',?,'test-model','legacy_frozen',?,?)`, threadUUID, projectID, providerUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	var threadID int64
	if err := service.store.DB().Table("chat_threads").Where("uuid=?", threadUUID).Pluck("id", &threadID).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.store.DB().Exec(`INSERT INTO chat_turns(uuid,thread_id,source_type,queue_sequence,input_text,status,created_at,updated_at) VALUES(?,?,'prompt',1,'Use section','in_progress',?,?)`, turnUUID, threadID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	var turnID int64
	if err := service.store.DB().Table("chat_turns").Where("uuid=?", turnUUID).Pluck("id", &turnID).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.store.DB().Exec(`INSERT INTO chat_items(uuid,thread_id,turn_id,sequence,item_type,role,content,metadata_json,created_at) VALUES(?,?,?,1,'user_message','user','Use section','{}',?)`, itemUUID, threadID, turnID, now).Error; err != nil {
		t.Fatal(err)
	}
	var itemID int64
	if err := service.store.DB().Table("chat_items").Where("uuid=?", itemUUID).Pluck("id", &itemID).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := `{"resource_type":"comic_section","resource_uuid":"` + sectionUUID + `","status":"available","title":"Referenced"}`
	if err := service.store.DB().Exec(`INSERT INTO chat_context_references(chat_item_id,position,resource_type,resource_uuid,snapshot_json,comic_section_id,created_at) VALUES(?,1,'comic_section',?,?,?,?)`, itemID, sectionUUID, snapshot, sectionID, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.PermanentlyDeleteChapter(ctx, chapter.UUID, chapter.Revision); storyErrorCode(err) != CodeChapterDeleteBlocked {
		t.Fatalf("active Turn Reference did not block deletion: %v", err)
	}
	if err := service.store.DB().Table("chat_turns").Where("id=?", turnID).Update("status", "completed").Error; err != nil {
		t.Fatal(err)
	}
	if err := service.store.DB().Exec(`INSERT INTO chat_follow_ups(uuid,thread_id,input_text,position,status,created_at,updated_at) VALUES(?,?,'Use later',1,'queued',?,?)`, followUpUUID, threadID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	var followUpID int64
	if err := service.store.DB().Table("chat_follow_ups").Where("uuid=?", followUpUUID).Pluck("id", &followUpID).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.store.DB().Exec(`INSERT INTO chat_context_references(follow_up_id,position,resource_type,resource_uuid,snapshot_json,comic_section_id,created_at) VALUES(?,1,'comic_section',?,?,?,?)`, followUpID, sectionUUID, snapshot, sectionID, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.PermanentlyDeleteChapter(ctx, chapter.UUID, chapter.Revision); storyErrorCode(err) != CodeChapterDeleteBlocked {
		t.Fatalf("queued Follow-up Reference did not block deletion: %v", err)
	}
	if err := service.store.DB().Table("chat_follow_ups").Where("id=?", followUpID).Update("status", "promoted").Error; err != nil {
		t.Fatal(err)
	}
	if err := service.PermanentlyDeleteChapter(ctx, chapter.UUID, chapter.Revision); err != nil {
		t.Fatal(err)
	}
	var retained, clearedTargets int64
	if err := service.store.DB().Table("chat_context_references").Where("resource_type='comic_section' AND resource_uuid=?", sectionUUID).Count(&retained).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.store.DB().Table("chat_context_references").Where("resource_type='comic_section' AND resource_uuid=? AND comic_section_id IS NULL", sectionUUID).Count(&clearedTargets).Error; err != nil {
		t.Fatal(err)
	}
	if retained != 2 || clearedTargets != 2 {
		t.Fatalf("retained references=%d cleared targets=%d", retained, clearedTargets)
	}
}

func TestEmptyChapterTrashPartiallyDeletesAndIsIdempotent(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	blocked, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol01.ch10", Title: "Blocked"})
	if err != nil {
		t.Fatal(err)
	}
	deletable, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol01.ch11", Title: "Deletable"})
	if err != nil {
		t.Fatal(err)
	}
	blocked, err = service.TrashChapter(ctx, blocked.UUID, blocked.Revision)
	if err != nil {
		t.Fatal(err)
	}
	deletable, err = service.TrashChapter(ctx, deletable.UUID, deletable.Revision)
	if err != nil {
		t.Fatal(err)
	}
	var projectID int64
	if err := service.store.DB().Table("projects").Where("uuid = ?", service.store.ProjectUUID()).Pluck("id", &projectID).Error; err != nil {
		t.Fatal(err)
	}
	taskUUID, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	providerUUID, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	unrelatedResourceUUID, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	now := service.now().UTC()
	if err := service.store.DB().Exec(`INSERT INTO task_runs(uuid,project_id,kind,resource_uuid,input_version,input_snapshot,status,idempotency_key,retryable,provider_uuid,model,progress,attempt,max_attempts,error_code,error_message,created_at,updated_at) VALUES(?,?,?,?,1,?,'waiting_for_input',?,0,?,'story-model',10,1,1,'','',?,?)`, taskUUID, projectID, "story_chapter_generation", unrelatedResourceUUID, `{"chapter_uuid":"`+blocked.UUID+`"}`, "empty-trash-"+taskUUID, providerUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	result, err := service.EmptyChapterTrash(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedCount != 1 || len(result.BlockedItems) != 1 || result.BlockedItems[0].UUID != blocked.UUID || result.BlockedItems[0].ChapterCode != blocked.ChapterCode || result.BlockedItems[0].ErrorCode != CodeChapterDeleteBlocked {
		t.Fatalf("partial empty result = %+v", result)
	}
	if _, err := service.GetChapter(ctx, deletable.UUID); storyErrorCode(err) != CodeChapterNotFound {
		t.Fatalf("deletable chapter remains: %v", err)
	}
	if _, err := service.GetChapter(ctx, blocked.UUID); err != nil {
		t.Fatalf("blocked chapter was removed: %v", err)
	}
	if err := service.store.DB().Table("task_runs").Where("uuid = ?", taskUUID).Updates(map[string]any{"status": "completed", "progress": 100}).Error; err != nil {
		t.Fatal(err)
	}
	result, err = service.EmptyChapterTrash(ctx)
	if err != nil || result.DeletedCount != 1 || len(result.BlockedItems) != 0 {
		t.Fatalf("second empty result = %+v, error = %v", result, err)
	}
	result, err = service.EmptyChapterTrash(ctx)
	if err != nil || result.DeletedCount != 0 || len(result.BlockedItems) != 0 {
		t.Fatalf("idempotent empty result = %+v, error = %v", result, err)
	}
}

func TestBatchImportSkipsExistingAndDuplicateCodesAndIsIdempotent(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	if _, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol01.ch02", Title: "Existing"}); err != nil {
		t.Fatal(err)
	}
	partial, err := service.ImportChapters(ctx, []ImportFile{
		{Filename: "vol01.ch01-first.md", Content: "First body"},
		{Filename: "vol01.ch02-conflict.txt", Content: "Conflicting body"},
		{Filename: "vol01.ch01-duplicate.md", Content: "Duplicate body"},
	})
	if err != nil || len(partial.Items) != 1 || partial.Items[0].ChapterCode != "vol01.ch01" || len(partial.Skipped) != 2 {
		t.Fatalf("partial import = %+v, error = %v", partial, err)
	}
	if partial.Skipped[0].Reason != "existing_chapter" || partial.Skipped[1].Reason != "duplicate_code" {
		t.Fatalf("skipped import entries = %+v", partial.Skipped)
	}
	chapters, err := service.ListChapters(ctx, "active")
	if err != nil || len(chapters) != 2 {
		t.Fatalf("active chapters after skipped import = %+v, error = %v", chapters, err)
	}
	var activeImportAssets int64
	if err := service.store.DB().Table("files").Where("purpose = ? AND deleted_at IS NULL", "story_import").Count(&activeImportAssets).Error; err != nil || activeImportAssets != 1 {
		t.Fatalf("active import assets = %d, error = %v", activeImportAssets, err)
	}
	files := []ImportFile{
		{Filename: "/outside/vol01.ch03-third.md", Content: "Third body"},
		{Filename: `C:\outside\vol01.ch04-fourth.txt`, Content: "Fourth body"},
	}
	first, err := service.ImportChapters(ctx, files)
	if err != nil || len(first.Items) != 2 {
		t.Fatalf("first import = %+v, error = %v", first, err)
	}
	retry, err := service.ImportChapters(ctx, files)
	if err != nil || retry.UUID != first.UUID || len(retry.Items) != 2 {
		t.Fatalf("retry import = %+v, error = %v", retry, err)
	}
	var absoluteNames int64
	if err := service.store.DB().Model(&storySourceItemRecord{}).Where("original_filename LIKE '/%'").Count(&absoluteNames).Error; err != nil || absoluteNames != 0 {
		t.Fatalf("absolute filenames = %d, error = %v", absoluteNames, err)
	}
	var linkedFiles int64
	if err := service.store.DB().Model(&storySourceItemRecord{}).Where("file_id IS NOT NULL").Count(&linkedFiles).Error; err != nil || linkedFiles != 3 {
		t.Fatalf("story import file links = %d, error = %v", linkedFiles, err)
	}
	allSkipped, err := service.ImportChapters(ctx, []ImportFile{{Filename: "vol01.ch02-existing.md", Content: "Ignored"}})
	if err != nil || len(allSkipped.Items) != 0 || len(allSkipped.Skipped) != 1 || allSkipped.Skipped[0].Reason != "existing_chapter" {
		t.Fatalf("all-skipped import = %+v, error = %v", allSkipped, err)
	}
}

func TestStoryMDConflictImportAndRegenerate(t *testing.T) {
	_, created, service := storyHarness(t)
	ctx := context.Background()
	profile, err := service.GetStoryProfile(ctx)
	if err != nil || profile.ProjectionState != "synced" || profile.Revision != 1 {
		t.Fatalf("initial profile = %+v, error = %v", profile, err)
	}
	profile, err = service.UpdateStoryProfile(ctx, "# Story\n\nDatabase version\n", profile.Revision)
	if err != nil || profile.ProjectionState != "synced" || profile.Revision != 2 {
		t.Fatalf("updated profile = %+v, error = %v", profile, err)
	}
	storyPath := filepath.Join(created.RootPath, "STORY.md")
	if err := os.WriteFile(storyPath, []byte("# Story\n\nExternal version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	conflicted, err := service.GetStoryProfile(ctx)
	if err != nil || conflicted.ProjectionState != "conflict" {
		t.Fatalf("conflicted profile = %+v, error = %v", conflicted, err)
	}
	if _, err := service.UpdateStoryProfile(ctx, "# Overwrite\n", conflicted.Revision); storyErrorCode(err) != CodeStoryMDConflict {
		t.Fatalf("conflict update error = %v", err)
	}
	imported, err := service.ImportExternalStoryMD(ctx, conflicted.Revision)
	if err != nil || imported.SourceType != "external_import" || imported.Revision != 3 || imported.StoryMD != "# Story\n\nExternal version\n" {
		t.Fatalf("imported profile = %+v, error = %v", imported, err)
	}
	if err := os.WriteFile(storyPath, []byte("# Another external edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	regenerated, err := service.RegenerateStoryMD(ctx, imported.Revision)
	if err != nil || regenerated.ProjectionState != "synced" {
		t.Fatalf("regenerated profile = %+v, error = %v", regenerated, err)
	}
	content, err := os.ReadFile(storyPath)
	if err != nil || string(content) != imported.StoryMD {
		t.Fatalf("regenerated file = %q, error = %v", content, err)
	}
}

func TestProjectionFailureLeavesRecoverablePendingVersion(t *testing.T) {
	_, created, service := storyHarness(t)
	ctx := context.Background()
	profile, err := service.GetStoryProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	service.writeProjection = func(string) error { return errors.New("simulated exit before rename") }
	if _, err := service.UpdateStoryProfile(ctx, "# Pending\n\nStill in SQLite.\n", profile.Revision); storyErrorCode(err) != CodeStoryProjectionFailed {
		t.Fatalf("projection failure = %v", err)
	}
	recovered, err := NewService(service.store).GetStoryProfile(ctx)
	if err != nil || recovered.Revision != profile.Revision+1 || recovered.ProjectionState != "synced" {
		t.Fatalf("recovered profile = %+v, error = %v", recovered, err)
	}
	content, err := os.ReadFile(filepath.Join(created.RootPath, "STORY.md"))
	if err != nil || string(content) != recovered.StoryMD {
		t.Fatalf("recovered projection = %q, error = %v", content, err)
	}
}

func TestPendingProjectionRecoversAfterExitBeforeAtomicRename(t *testing.T) {
	_, created, service := storyHarness(t)
	ctx := context.Background()
	current, err := service.ensureStoryProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	next, err := service.createStoryProfileVersion(ctx, current, "# After restart\n", "manual_edit", "pending", current.ExportedRevision, current.ExportedHash, current.ObservedFileHash)
	if err != nil {
		t.Fatal(err)
	}
	orphanTemp := filepath.Join(created.RootPath, ".lumi", "tmp", "story-projection-interrupted.tmp")
	if err := os.WriteFile(orphanTemp, []byte(next.StoryMD), 0o644); err != nil {
		t.Fatal(err)
	}
	profile, err := NewService(service.store).GetStoryProfile(ctx)
	if err != nil || profile.UUID != next.UUID || profile.ProjectionState != "synced" {
		t.Fatalf("recovered profile = %+v, error = %v", profile, err)
	}
	content, err := os.ReadFile(filepath.Join(created.RootPath, "STORY.md"))
	if err != nil || string(content) != next.StoryMD {
		t.Fatalf("recovered STORY.md = %q, error = %v", content, err)
	}
}

func TestStoryDataSurvivesProjectMoveWithoutStoredRootPath(t *testing.T) {
	manager, created, service := storyHarness(t)
	ctx := context.Background()
	chapter, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol02.ch01", Title: "Portable", Content: "Moves with the project", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := service.GetStoryProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateStoryProfile(ctx, "# Portable story\n", profile.Revision); err != nil {
		t.Fatal(err)
	}
	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	movedRoot := filepath.Join(t.TempDir(), "moved-story-project")
	if err := os.Rename(created.RootPath, movedRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Relocate(ctx, created.UUID, project.ExplicitExistingDirectory(movedRoot)); err != nil {
		t.Fatal(err)
	}
	var reopened *Service
	if err := manager.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		reopened = NewService(store)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := reopened.GetChapter(ctx, chapter.UUID)
	if err != nil || reloaded.CurrentStory == nil || reloaded.CurrentStory.Content != "Moves with the project" {
		t.Fatalf("reloaded chapter = %+v, error = %v", reloaded, err)
	}
	reloadedProfile, err := reopened.GetStoryProfile(ctx)
	if err != nil || reloadedProfile.StoryMD != "# Portable story\n" {
		t.Fatalf("reloaded profile = %+v, error = %v", reloadedProfile, err)
	}
	var storedPaths int64
	if err := reopened.store.DB().Raw(`
		SELECT COUNT(*) FROM story_source_items
		WHERE original_filename LIKE ? OR original_filename LIKE ?
	`, created.RootPath+"%", movedRoot+"%").Scan(&storedPaths).Error; err != nil || storedPaths != 0 {
		t.Fatalf("stored project paths = %d, error = %v", storedPaths, err)
	}
}

func TestPromptRestoreCreatesNewCandidate(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	first, err := service.CreatePromptVersion(ctx, CreatePromptInput{PromptGroup: "story", PromptKey: "outline", Prompt: "First prompt", ExpectedCurrentVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreatePromptVersion(ctx, CreatePromptInput{PromptGroup: "story", PromptKey: "outline", Prompt: "Second prompt", ExpectedCurrentVersion: 1})
	if err != nil || second.VersionNo != 2 {
		t.Fatalf("second prompt = %+v, error = %v", second, err)
	}
	restored, err := service.RestorePromptVersion(ctx, first.UUID, 2)
	if err != nil || restored.VersionNo != 3 || restored.SourceType != "version_restore" || restored.RestoredFromUUID != first.UUID || restored.Prompt != first.Prompt {
		t.Fatalf("restored prompt = %+v, error = %v", restored, err)
	}
	items, pagination, err := service.ListPromptVersions(ctx, "story", "outline", 1, 2)
	if err != nil || len(items) != 2 || pagination.Total != 3 || pagination.LastPage != 2 {
		t.Fatalf("prompt history = %+v, pagination = %+v, error = %v", items, pagination, err)
	}
	if err := service.store.DB().Model(&promptVersionRecord{}).Where("uuid = ?", first.UUID).Update("prompt", "mutated").Error; err == nil {
		t.Fatal("append-only trigger allowed prompt mutation")
	}
}

func TestPromptCatalogLanguageSwitchPreservesCustomAndMigratesDefaults(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	items, err := service.ListPromptCatalog(ctx, "")
	if err != nil || len(items) != len(promptcatalog.Definitions(promptcatalog.LanguageChinese)) {
		t.Fatalf("initial catalog = %d items, error = %v", len(items), err)
	}
	defaultProfile, err := service.EffectivePrompt(ctx, "story", "story_profile")
	if err != nil || !strings.Contains(defaultProfile, "用户故事想法") {
		t.Fatalf("Chinese story profile = %q, error = %v", defaultProfile, err)
	}
	custom := "CUSTOM {{input_prompt}} {{story_md}} {{chapter_plan_json}} {{generated_summaries_json}} {{chapter_code}}"
	if _, err := service.CreatePromptVersion(ctx, CreatePromptInput{PromptGroup: "story", PromptKey: "story_chapter", Prompt: custom, ExpectedCurrentVersion: 1}); err != nil {
		t.Fatal(err)
	}
	detail, err := service.GetProject(ctx)
	if err != nil {
		t.Fatal(err)
	}
	english := promptcatalog.LanguageEnglish
	updated, err := service.UpdateProject(ctx, UpdateProjectInput{Name: detail.Name, Description: detail.Description, GenerationLanguage: &english, ExpectedRevision: detail.Revision})
	if err != nil || updated.GenerationLanguage != promptcatalog.LanguageEnglish {
		t.Fatalf("language update = %+v, error = %v", updated, err)
	}
	profile, err := service.EffectivePrompt(ctx, "story", "story_profile")
	if err != nil || !strings.Contains(profile, "User story idea") || strings.Contains(profile, "用户故事想法") {
		t.Fatalf("English story profile = %q, error = %v", profile, err)
	}
	preserved, err := service.EffectivePrompt(ctx, "story", "story_chapter")
	if err != nil || preserved != custom {
		t.Fatalf("custom prompt after language switch = %q, error = %v", preserved, err)
	}
	versions, _, err := service.ListPromptVersions(ctx, "story", "story_profile", 1, 20)
	if err != nil || len(versions) != 2 || versions[0].SourceType != "project_language_changed" || !strings.Contains(versions[0].Prompt, "User story idea") {
		t.Fatalf("migrated versions = %+v, error = %v", versions, err)
	}
}

func TestPromptCatalogMigratesPreviousPremiseDefaultsWithoutOverwritingUserChoices(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	keys := []string{"setting_image", "asset_breakdown"}
	definitions := make(map[string]promptcatalog.Definition, len(keys))
	previousRecords := make(map[string]promptVersionRecord, len(keys))
	for _, key := range keys {
		definition, record := replacePromptWithPreviousBuiltin(t, service, promptcatalog.GroupPremise, key)
		definitions[key] = definition
		previousRecords[key] = record
	}

	if err := service.EnsurePromptCatalogVersions(ctx, "migration"); err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		versions, pagination, err := service.ListPromptVersions(ctx, promptcatalog.GroupPremise, key, 1, 20)
		if err != nil || pagination.Total != 2 || len(versions) != 2 {
			t.Fatalf("%s migrated versions=%+v pagination=%+v error=%v", key, versions, pagination, err)
		}
		if versions[0].VersionNo != 2 || versions[0].SourceType != "migration" || versions[0].Prompt != strings.TrimSpace(definitions[key].DefaultValue) {
			t.Fatalf("%s latest migration=%+v", key, versions[0])
		}
	}
	if err := service.EnsurePromptCatalogVersions(ctx, "migration"); err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		_, pagination, err := service.ListPromptVersions(ctx, promptcatalog.GroupPremise, key, 1, 20)
		if err != nil || pagination.Total != 2 {
			t.Fatalf("%s idempotent migration total=%d error=%v", key, pagination.Total, err)
		}
	}

	settingPrevious := strings.TrimSpace(definitions["setting_image"].PreviousDefaultValues[0])
	manual, err := service.CreatePromptVersion(ctx, CreatePromptInput{PromptGroup: promptcatalog.GroupPremise, PromptKey: "setting_image", Prompt: settingPrevious, ExpectedCurrentVersion: 2})
	if err != nil || manual.SourceType != "manual_edit" {
		t.Fatalf("manual previous setting prompt=%+v error=%v", manual, err)
	}
	restored, err := service.RestorePromptVersion(ctx, previousRecords["asset_breakdown"].UUID, 2)
	if err != nil || restored.SourceType != "version_restore" {
		t.Fatalf("restored previous breakdown prompt=%+v error=%v", restored, err)
	}
	if err := service.EnsurePromptCatalogVersions(ctx, "migration"); err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		versions, pagination, err := service.ListPromptVersions(ctx, promptcatalog.GroupPremise, key, 1, 20)
		wantPrompt := strings.TrimSpace(definitions[key].PreviousDefaultValues[0])
		if err != nil || pagination.Total != 3 || len(versions) != 3 || versions[0].Prompt != wantPrompt {
			t.Fatalf("%s user-selected previous prompt was overwritten: versions=%+v pagination=%+v error=%v", key, versions, pagination, err)
		}
	}
}

func TestPromptLanguageSwitchRecognizesPreviousBuiltinDefault(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	replacePromptWithPreviousBuiltin(t, service, promptcatalog.GroupPremise, "setting_image")
	detail, err := service.GetProject(ctx)
	if err != nil {
		t.Fatal(err)
	}
	english := promptcatalog.LanguageEnglish
	if _, err := service.UpdateProject(ctx, UpdateProjectInput{Name: detail.Name, Description: detail.Description, GenerationLanguage: &english, ExpectedRevision: detail.Revision}); err != nil {
		t.Fatal(err)
	}
	want, ok := promptcatalog.Lookup(promptcatalog.GroupPremise, "setting_image", promptcatalog.LanguageEnglish)
	if !ok {
		t.Fatal("missing English setting_image definition")
	}
	effective, err := service.EffectivePrompt(ctx, promptcatalog.GroupPremise, "setting_image")
	if err != nil || effective != strings.TrimSpace(want.DefaultValue) {
		t.Fatalf("English setting prompt=%q error=%v", effective, err)
	}
	versions, pagination, err := service.ListPromptVersions(ctx, promptcatalog.GroupPremise, "setting_image", 1, 20)
	if err != nil || pagination.Total != 2 || len(versions) != 2 || versions[0].SourceType != "project_language_changed" {
		t.Fatalf("language-migrated setting versions=%+v pagination=%+v error=%v", versions, pagination, err)
	}
}

func TestPromptCatalogSeedsDefaultsAndUpdatesGroupsAtomically(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	var initialCount int64
	if err := service.store.DB().Model(&promptVersionRecord{}).Count(&initialCount).Error; err != nil {
		t.Fatal(err)
	}
	if initialCount != 28 {
		t.Fatalf("initial prompt versions = %d, want 28", initialCount)
	}
	if err := service.EnsurePromptCatalogVersions(ctx, "migration"); err != nil {
		t.Fatal(err)
	}
	var reconciledCount int64
	if err := service.store.DB().Model(&promptVersionRecord{}).Count(&reconciledCount).Error; err != nil || reconciledCount != initialCount {
		t.Fatalf("idempotent seed count = %d, error = %v", reconciledCount, err)
	}
	items, err := service.UpdatePromptGroup(ctx, UpdatePromptGroupInput{
		PromptGroup: "story",
		Prompts: map[string]string{
			"json_system":   "Custom JSON system",
			"story_profile": "Custom story profile",
		},
		ExpectedCurrentVersions: map[string]int{"json_system": 1, "story_profile": 1},
	})
	if err != nil || len(items) != 6 {
		t.Fatalf("group update items=%d error=%v", len(items), err)
	}
	_, conflictErr := service.UpdatePromptGroup(ctx, UpdatePromptGroupInput{
		PromptGroup: "story",
		Prompts: map[string]string{
			"json_system":   "Rolled back JSON",
			"story_profile": "Stale profile",
		},
		ExpectedCurrentVersions: map[string]int{"json_system": 2, "story_profile": 1},
	})
	var storyErr *Error
	if conflictErr == nil || !errors.As(conflictErr, &storyErr) || storyErr.Code != CodePromptRevisionConflict {
		t.Fatalf("stale group update error = %v", conflictErr)
	}
	versions, pagination, err := service.ListPromptVersions(ctx, "story", "json_system", 1, 20)
	if err != nil || pagination.Total != 2 || versions[0].Prompt != "Custom JSON system" {
		t.Fatalf("atomic versions=%+v pagination=%+v error=%v", versions, pagination, err)
	}
	if _, err := service.UpdatePromptGroup(ctx, UpdatePromptGroupInput{
		PromptGroup:             "runtime",
		Prompts:                 map[string]string{"project_language_instruction": "Always use the project's chosen language."},
		ExpectedCurrentVersions: map[string]int{"project_language_instruction": 1},
	}); err != nil {
		t.Fatal(err)
	}
	instruction, err := service.EffectiveLanguageInstruction(ctx)
	if err != nil || instruction != "Always use the project's chosen language." {
		t.Fatalf("effective language instruction = %q, error = %v", instruction, err)
	}
}

func TestPromptCatalogUsesCreationAndMigrationSources(t *testing.T) {
	manager, created, service := storyHarness(t)
	ctx := context.Background()
	versions, _, err := service.ListPromptVersions(ctx, "runtime", "project_language_instruction", 1, 20)
	if err != nil || len(versions) != 1 || versions[0].SourceType != "project_created" {
		t.Fatalf("new-project prompt versions=%+v error=%v", versions, err)
	}
	if err := service.store.DB().Where("prompt_group=? AND prompt_key=?", "runtime", "project_language_instruction").Delete(&promptVersionRecord{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.OpenRecent(ctx, created.UUID); err != nil {
		t.Fatal(err)
	}
	var reopened *Service
	if err := manager.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		reopened = NewService(store)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	versions, _, err = reopened.ListPromptVersions(ctx, "runtime", "project_language_instruction", 1, 20)
	if err != nil || len(versions) != 1 || versions[0].SourceType != "migration" {
		t.Fatalf("reconciled prompt versions=%+v error=%v", versions, err)
	}
}

func TestPromptCatalogReadsLegacyPremiseKeyButWritesCanonical(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	if err := service.store.DB().Where("prompt_group = ? AND prompt_key = ?", "premise", "setting_image").Delete(&promptVersionRecord{}).Error; err != nil {
		t.Fatal(err)
	}
	legacy, err := service.CreatePromptVersion(ctx, CreatePromptInput{PromptGroup: "premise", PromptKey: "setting_generation", Prompt: "legacy premise template", ExpectedCurrentVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	effective, err := service.EffectivePrompt(ctx, "premise", "setting_image")
	if err != nil || effective != legacy.Prompt {
		t.Fatalf("legacy effective prompt = %q, error = %v", effective, err)
	}
	items, err := service.ListPromptCatalog(ctx, "premise")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || items[0].PromptKey != "setting_image" || items[0].UsingLegacyKey != "setting_generation" || !items[0].IsCustom {
		t.Fatalf("legacy catalog item = %+v", items)
	}
	canonical, err := service.CreatePromptVersion(ctx, CreatePromptInput{PromptGroup: "premise", PromptKey: "setting_image", Prompt: "canonical premise template", ExpectedCurrentVersion: 0})
	if err != nil || canonical.PromptKey != "setting_image" {
		t.Fatalf("canonical prompt = %+v, error = %v", canonical, err)
	}
	effective, err = service.EffectivePrompt(ctx, "premise", "setting_image")
	if err != nil || effective != canonical.Prompt {
		t.Fatalf("canonical effective prompt = %q, error = %v", effective, err)
	}
}
