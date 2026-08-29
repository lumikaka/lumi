package project

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"lumi/internal/config"
	"lumi/internal/dbmigrate"
)

func TestRecentProjectCoverUsesFirstBooksFrontCoverThenBody(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	created, err := manager.Create(ctx, "Cover ordering", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	store := openStoreForTest(t, manager, created.UUID)
	recents, err := manager.RecentProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(recents) != 1 || recents[0].CoverImageURL != "" {
		t.Fatalf("recent without cover=%+v", recents)
	}

	var projectID, actorID int64
	if err := store.DB().Raw("SELECT id FROM projects WHERE uuid = ?", created.UUID).Scan(&projectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Raw("SELECT id FROM actors WHERE kind = 'local_user' ORDER BY id LIMIT 1").Scan(&actorID).Error; err != nil {
		t.Fatal(err)
	}
	firstChapterID, firstStateID := insertRecentCoverChapter(t, store, projectID, 1)
	secondChapterID, secondStateID := insertRecentCoverChapter(t, store, projectID, 2)

	// A ready back cover sorts before the body by section_no, but must never
	// become the project thumbnail.
	insertRecentCoverImage(t, store, projectID, actorID, firstStateID, 3, "back_cover", 1)
	firstBody := insertRecentCoverImage(t, store, projectID, actorID, firstStateID, 2, "body", 2)
	secondFront := insertRecentCoverImage(t, store, projectID, actorID, secondStateID, 1, "front_cover", 3)

	cover, err := loadRecentProjectCoverReference(ctx, created.UUID, created.RootPath)
	if err != nil {
		t.Fatal(err)
	}
	if cover.AssetUUID != firstBody {
		t.Fatalf("cover=%s want first book body=%s; later front=%s chapters=%d/%d", cover.AssetUUID, firstBody, secondFront, firstChapterID, secondChapterID)
	}
	recents, err = manager.RecentProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bodyURL := "/media/recent-projects/" + created.UUID + "/cover?v=" + fmt.Sprintf("%064x", 2)
	if len(recents) != 1 || recents[0].CoverImageURL != bodyURL {
		t.Fatalf("recent body cover URL=%+v want=%s", recents, bodyURL)
	}

	firstFront := insertRecentCoverImage(t, store, projectID, actorID, firstStateID, 1, "front_cover", 4)
	cover, err = loadRecentProjectCoverReference(ctx, created.UUID, created.RootPath)
	if err != nil {
		t.Fatal(err)
	}
	if cover.AssetUUID != firstFront {
		t.Fatalf("cover=%s want first book front=%s", cover.AssetUUID, firstFront)
	}
	recents, err = manager.RecentProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	frontURL := "/media/recent-projects/" + created.UUID + "/cover?v=" + fmt.Sprintf("%064x", 4)
	if len(recents) != 1 || recents[0].CoverImageURL != frontURL || frontURL == bodyURL {
		t.Fatalf("recent front cover URL=%+v want=%s and different from %s", recents, frontURL, bodyURL)
	}
}

func TestRecentProjectCoverFallsBackBeforePageRoleMigration(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	created, err := manager.Create(ctx, "Legacy cover", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	store := openStoreForTest(t, manager, created.UUID)
	var projectID, actorID int64
	if err := store.DB().Raw("SELECT id FROM projects WHERE uuid = ?", created.UUID).Scan(&projectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Raw("SELECT id FROM actors WHERE kind = 'local_user' ORDER BY id LIMIT 1").Scan(&actorID).Error; err != nil {
		t.Fatal(err)
	}
	_, stateID := insertRecentCoverChapter(t, store, projectID, 1)
	wantAssetUUID := insertRecentCoverImage(t, store, projectID, actorID, stateID, 1, "body", 11)
	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}

	runner, err := dbmigrate.OpenProject(config.SQLiteDSN(filepath.Join(created.RootPath, "project.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(1); err != nil {
		_ = runner.Close()
		t.Fatal(err)
	}
	if err := runner.Close(); err != nil {
		t.Fatal(err)
	}

	cover, err := loadRecentProjectCoverReference(ctx, created.UUID, created.RootPath)
	if err != nil {
		t.Fatal(err)
	}
	if cover.AssetUUID != wantAssetUUID {
		t.Fatalf("legacy cover=%s want=%s", cover.AssetUUID, wantAssetUUID)
	}
	recents, err := manager.RecentProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantURL := "/media/recent-projects/" + created.UUID + "/cover?v=" + fmt.Sprintf("%064x", 11)
	if len(recents) != 1 || recents[0].CoverImageURL != wantURL {
		t.Fatalf("legacy recent cover URL=%+v want=%s", recents, wantURL)
	}
}

func insertRecentCoverChapter(t *testing.T, store *Store, projectID int64, ordinal int) (int64, int64) {
	t.Helper()
	now := time.Now().UTC()
	chapterUUID, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	chapterCode := fmt.Sprintf("vol01.ch%02d", ordinal)
	if err := store.DB().Exec(`
INSERT INTO chapters(uuid,project_id,volume_no,chapter_no,chapter_code,sort_order,title,revision,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?)`, chapterUUID, projectID, 1, ordinal, chapterCode, ordinal, chapterCode, 0, now, now).Error; err != nil {
		t.Fatal(err)
	}
	var chapterID int64
	if err := store.DB().Raw("SELECT id FROM chapters WHERE uuid = ?", chapterUUID).Scan(&chapterID).Error; err != nil {
		t.Fatal(err)
	}
	stateUUID, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Exec(`
INSERT INTO chapter_comic_states(uuid,chapter_id,status,revision,created_at,updated_at)
VALUES(?,?,?,0,?,?)`, stateUUID, chapterID, "draft", now, now).Error; err != nil {
		t.Fatal(err)
	}
	var stateID int64
	if err := store.DB().Raw("SELECT id FROM chapter_comic_states WHERE uuid = ?", stateUUID).Scan(&stateID).Error; err != nil {
		t.Fatal(err)
	}
	return chapterID, stateID
}

func insertRecentCoverImage(t *testing.T, store *Store, projectID, actorID, stateID int64, sectionNo int, pageRole string, seed int) string {
	t.Helper()
	now := time.Now().UTC()
	sectionUUID, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Exec(`
INSERT INTO comic_sections(uuid,chapter_comic_state_id,actor_id,section_no,page_role,title,description_md,revision,created_at,updated_at)
VALUES(?,?,?,?,?,'','',0,?,?)`, sectionUUID, stateID, actorID, sectionNo, pageRole, now, now).Error; err != nil {
		t.Fatal(err)
	}
	var sectionID int64
	if err := store.DB().Raw("SELECT id FROM comic_sections WHERE uuid = ?", sectionUUID).Scan(&sectionID).Error; err != nil {
		t.Fatal(err)
	}

	assetUUID, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	objectUUID, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	sha := fmt.Sprintf("%064x", seed)
	keyPath := fmt.Sprintf("test/%d.png", seed)
	if err := store.DB().Exec(`
INSERT INTO file_objects(uuid,project_id,sha256,key_path,mime_type,canonical_ext,byte_size,width,height,state,created_at,verified_at)
VALUES(?,?,?,?,?,'png',1,1,1,'ready',?,?)`, objectUUID, projectID, sha, keyPath, "image/png", now, now).Error; err != nil {
		t.Fatal(err)
	}
	var objectID int64
	if err := store.DB().Raw("SELECT id FROM file_objects WHERE uuid = ?", objectUUID).Scan(&objectID).Error; err != nil {
		t.Fatal(err)
	}
	filename := fmt.Sprintf("fixture-%d.png", seed)
	if err := store.DB().Exec(`
INSERT INTO files(uuid,project_id,file_object_id,kind,purpose,original_filename,source_type,metadata_json,actor_id,created_at)
VALUES(?,?,?,'image','comic_section_image',?,'imported','{}',?,?)`, assetUUID, projectID, objectID, filename, actorID, now).Error; err != nil {
		t.Fatal(err)
	}
	var fileID int64
	if err := store.DB().Raw("SELECT id FROM files WHERE uuid = ?", assetUUID).Scan(&fileID).Error; err != nil {
		t.Fatal(err)
	}
	variantUUID, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Exec(`
INSERT INTO comic_image_variants(uuid,comic_section_id,file_id,actor_id,version_no,source_type,input_snapshot,created_at)
VALUES(?,?,?,?,1,'manual','{}',?)`, variantUUID, sectionID, fileID, actorID, now).Error; err != nil {
		t.Fatal(err)
	}
	var variantID int64
	if err := store.DB().Raw("SELECT id FROM comic_image_variants WHERE uuid = ?", variantUUID).Scan(&variantID).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Exec("UPDATE comic_sections SET current_image_variant_id = ? WHERE id = ?", variantID, sectionID).Error; err != nil {
		t.Fatal(err)
	}
	return assetUUID
}
