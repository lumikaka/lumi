package story

import (
	"context"
	"fmt"
	"testing"
	"time"

	"lumi/internal/agentcheckpoint"
)

func insertStoryCheckpointExecution(t *testing.T, service *Service, routeID string) string {
	t.Helper()
	db := service.store.DB()
	now := time.Now().UTC()
	threadUUID, _ := newUUIDv7()
	turnUUID, _ := newUUIDv7()
	runUUID, _ := newUUIDv7()
	itemUUID, _ := newUUIDv7()
	executionUUID, _ := newUUIDv7()
	toolCallUUID, _ := newUUIDv7()
	providerUUID, _ := newUUIDv7()
	var projectID int64
	if err := db.Table("projects").Where("uuid=?", service.store.ProjectUUID()).Pluck("id", &projectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO chat_threads(uuid,project_id,title,status,provider_uuid,model,created_at,updated_at) VALUES(?,?,'checkpoint','busy',?,'test-model',?,?)`, threadUUID, projectID, providerUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO chat_turns(uuid,thread_id,source_type,queue_sequence,input_text,status,created_at,updated_at) VALUES(?,(SELECT id FROM chat_threads WHERE uuid=?),'prompt',1,'checkpoint','in_progress',?,?)`, turnUUID, threadUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO chat_runs(uuid,thread_id,turn_id,trigger_type,status,provider_uuid,model,created_at,updated_at) VALUES(?,(SELECT id FROM chat_threads WHERE uuid=?),(SELECT id FROM chat_turns WHERE uuid=?),'prompt','in_progress',?,'test-model',?,?)`, runUUID, threadUUID, turnUUID, providerUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO chat_items(uuid,thread_id,turn_id,run_id,sequence,item_type,role,content,content_format,status,remote_item_uuid,tool_name,metadata_json,created_at) VALUES(?,(SELECT id FROM chat_threads WHERE uuid=?),(SELECT id FROM chat_turns WHERE uuid=?),(SELECT id FROM chat_runs WHERE uuid=?),1,'tool_call','assistant','{}','json','in_progress',?,'request_api','{}',?)`, itemUUID, threadUUID, turnUUID, runUUID, toolCallUUID, now).Error; err != nil {
		t.Fatal(err)
	}
	arguments := fmt.Sprintf(`{"method":"DELETE","response_filter":".data","__route_id":%q}`, routeID)
	if err := db.Exec(`INSERT INTO agent_tool_executions(uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,tool_name,target_uuid,arguments_json,idempotency_key,state,started_at,created_at,updated_at) VALUES(?,(SELECT id FROM chat_threads WHERE uuid=?),(SELECT id FROM chat_runs WHERE uuid=?),(SELECT id FROM chat_turns WHERE uuid=?),(SELECT id FROM chat_items WHERE uuid=?),?,'request_api','',?,?,'executing',?,?,?)`, executionUUID, threadUUID, runUUID, turnUUID, itemUUID, toolCallUUID, arguments, "checkpoint:"+executionUUID, now, now, now).Error; err != nil {
		t.Fatal(err)
	}
	return executionUUID
}

func TestChapterPermanentDeleteToolCheckpointSurvivesCrashWindow(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	chapter, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol12.ch01", Title: "Delete once"})
	if err != nil {
		t.Fatal(err)
	}
	chapter, err = service.TrashChapter(ctx, chapter.UUID, chapter.Revision)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &storyEventRecorder{}
	service.WithEvents(recorder)
	executionUUID := insertStoryCheckpointExecution(t, service, agentcheckpoint.RouteChapterPermanentDelete)
	if err := service.PermanentlyDeleteChapterFromTool(ctx, chapter.UUID, chapter.Revision, executionUUID); err != nil {
		t.Fatal(err)
	}
	assertChapterPermanentDeleteEvent(t, recorder, 1, service.store.ProjectUUID(), chapter.UUID)
	// Simulate a crash before Agent persistToolResult: state remains executing,
	// but the deletion and checkpoint have committed.
	if err := service.PermanentlyDeleteChapterFromTool(ctx, chapter.UUID, chapter.Revision, executionUUID); err != nil {
		t.Fatalf("checkpoint replay repeated the delete: %v", err)
	}
	assertChapterPermanentDeleteEvent(t, recorder, 2, service.store.ProjectUUID(), chapter.UUID)
	var chapters int64
	if err := service.store.DB().Table("chapters").Where("uuid=?", chapter.UUID).Count(&chapters).Error; err != nil || chapters != 0 {
		t.Fatalf("chapter count=%d err=%v", chapters, err)
	}
}

func TestEmptyChapterTrashToolCheckpointKeepsOriginalResultAndScope(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	first, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol13.ch01", Title: "First trash"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.TrashChapter(ctx, first.UUID, first.Revision); err != nil {
		t.Fatal(err)
	}
	recorder := &storyEventRecorder{}
	service.WithEvents(recorder)
	executionUUID := insertStoryCheckpointExecution(t, service, agentcheckpoint.RouteChapterTrashEmpty)
	want, err := service.EmptyChapterTrashFromTool(ctx, executionUUID)
	if err != nil || want.DeletedCount != 1 {
		t.Fatalf("first result=%+v err=%v", want, err)
	}
	assertChapterTrashEmptiedEvent(t, recorder, 1, service.store.ProjectUUID(), want)
	service.WithEvents(nil)
	late, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol13.ch02", Title: "Late trash"})
	if err != nil {
		t.Fatal(err)
	}
	late, err = service.TrashChapter(ctx, late.UUID, late.Revision)
	if err != nil {
		t.Fatal(err)
	}
	service.WithEvents(recorder)
	got, err := service.EmptyChapterTrashFromTool(ctx, executionUUID)
	if err != nil || got.DeletedCount != want.DeletedCount || len(got.BlockedItems) != len(want.BlockedItems) {
		t.Fatalf("replayed result=%+v want=%+v err=%v", got, want, err)
	}
	assertChapterTrashEmptiedEvent(t, recorder, 2, service.store.ProjectUUID(), want)
	var remaining int64
	if err := service.store.DB().Table("chapters").Where("uuid=? AND deleted_at IS NOT NULL", late.UUID).Count(&remaining).Error; err != nil || remaining != 1 {
		t.Fatalf("late trash count=%d err=%v", remaining, err)
	}
}

func TestChapterDeleteRollsBackWhenCheckpointCannotCommit(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	chapter, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol14.ch01", Title: "Rollback"})
	if err != nil {
		t.Fatal(err)
	}
	chapter, err = service.TrashChapter(ctx, chapter.UUID, chapter.Revision)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &storyEventRecorder{}
	service.WithEvents(recorder)
	executionUUID := insertStoryCheckpointExecution(t, service, agentcheckpoint.RouteChapterPermanentDelete)
	if err := service.store.DB().Exec(`CREATE TRIGGER reject_story_checkpoint BEFORE UPDATE OF result_json ON agent_tool_executions BEGIN SELECT RAISE(ABORT,'reject checkpoint'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.PermanentlyDeleteChapterFromTool(ctx, chapter.UUID, chapter.Revision, executionUUID); err == nil {
		t.Fatal("delete succeeded without an atomic checkpoint")
	}
	if len(recorder.events) != 0 {
		t.Fatalf("rolled-back delete emitted realtime hints: %v", recorder.events)
	}
	if err := service.store.DB().Exec(`DROP TRIGGER reject_story_checkpoint`).Error; err != nil {
		t.Fatal(err)
	}
	var remaining int64
	if err := service.store.DB().Table("chapters").Where("uuid=? AND deleted_at IS NOT NULL", chapter.UUID).Count(&remaining).Error; err != nil || remaining != 1 {
		t.Fatalf("rolled-back chapter count=%d err=%v", remaining, err)
	}
}

func TestEmptyChapterTrashRollsBackWithoutRealtimeHintWhenCheckpointCannotCommit(t *testing.T) {
	_, _, service := storyHarness(t)
	ctx := context.Background()
	chapter, err := service.CreateChapter(ctx, CreateChapterInput{ChapterCode: "vol14.ch02", Title: "Empty rollback"})
	if err != nil {
		t.Fatal(err)
	}
	chapter, err = service.TrashChapter(ctx, chapter.UUID, chapter.Revision)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &storyEventRecorder{}
	service.WithEvents(recorder)
	executionUUID := insertStoryCheckpointExecution(t, service, agentcheckpoint.RouteChapterTrashEmpty)
	if err := service.store.DB().Exec(`CREATE TRIGGER reject_story_empty_checkpoint BEFORE UPDATE OF result_json ON agent_tool_executions BEGIN SELECT RAISE(ABORT,'reject checkpoint'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.EmptyChapterTrashFromTool(ctx, executionUUID); err == nil {
		t.Fatal("empty trash succeeded without an atomic checkpoint")
	}
	if len(recorder.events) != 0 {
		t.Fatalf("rolled-back empty trash emitted realtime hints: %v", recorder.events)
	}
	if err := service.store.DB().Exec(`DROP TRIGGER reject_story_empty_checkpoint`).Error; err != nil {
		t.Fatal(err)
	}
	var remaining int64
	if err := service.store.DB().Table("chapters").Where("uuid=? AND deleted_at IS NOT NULL", chapter.UUID).Count(&remaining).Error; err != nil || remaining != 1 {
		t.Fatalf("rolled-back chapter count=%d err=%v", remaining, err)
	}
}

func assertChapterPermanentDeleteEvent(t *testing.T, recorder *storyEventRecorder, wantCount int, projectUUID, chapterUUID string) {
	t.Helper()
	count := 0
	for index, event := range recorder.events {
		if event != "story:chapter_permanently_deleted" {
			continue
		}
		count++
		payload := recorder.payloads[index]
		if payload["project_uuid"] != projectUUID || payload["chapter_uuid"] != chapterUUID || !isUUIDv7(projectUUID) || !isUUIDv7(chapterUUID) {
			t.Fatalf("permanent-delete event payload=%#v", payload)
		}
		if _, leaked := payload["id"]; leaked {
			t.Fatalf("permanent-delete event leaked internal id: %#v", payload)
		}
	}
	if count != wantCount {
		t.Fatalf("permanent-delete event count=%d want=%d events=%v", count, wantCount, recorder.events)
	}
}

func assertChapterTrashEmptiedEvent(t *testing.T, recorder *storyEventRecorder, wantCount int, projectUUID string, result EmptyChapterTrashResult) {
	t.Helper()
	count := 0
	for index, event := range recorder.events {
		if event != "story:chapter_trash_emptied" {
			continue
		}
		count++
		payload := recorder.payloads[index]
		if payload["project_uuid"] != projectUUID || !isUUIDv7(projectUUID) || payload["deleted_count"] != result.DeletedCount || payload["blocked_count"] != len(result.BlockedItems) {
			t.Fatalf("trash-emptied event payload=%#v result=%+v", payload, result)
		}
		if _, leaked := payload["id"]; leaked {
			t.Fatalf("trash-emptied event leaked internal id: %#v", payload)
		}
	}
	if count != wantCount {
		t.Fatalf("trash-emptied event count=%d want=%d events=%v", count, wantCount, recorder.events)
	}
}
