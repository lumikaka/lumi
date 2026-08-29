package production

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"lumi/internal/project"
	"lumi/internal/story"
)

func TestComicPageRolesKeepCoversAtBookendsAndReorderOnlyBody(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Bookends"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Front before body", PageRole: PageRoleFrontCover}); !productionErrorIs(err, CodeValidation) {
		t.Fatalf("empty classic special-page create error=%v", err)
	}
	bodyA, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Body A"})
	if err != nil {
		t.Fatal(err)
	}
	back, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Back", PageRole: PageRoleBackCover})
	if err != nil {
		t.Fatal(err)
	}
	bodyB, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Body B"})
	if err != nil {
		t.Fatal(err)
	}
	front, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Front", PageRole: PageRoleFrontCover})
	if err != nil {
		t.Fatal(err)
	}

	sections, err := h.service.ListSections(ctx, chapter.UUID)
	if err != nil {
		t.Fatal(err)
	}
	wantUUIDs := []string{front.UUID, bodyA.UUID, bodyB.UUID, back.UUID}
	wantRoles := []string{PageRoleFrontCover, PageRoleBody, PageRoleBody, PageRoleBackCover}
	for index := range wantUUIDs {
		if sections[index].UUID != wantUUIDs[index] || sections[index].PageRole != wantRoles[index] || sections[index].SectionNo != index+1 {
			t.Fatalf("initial page %d=%+v", index+1, sections[index])
		}
	}
	if _, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{PageRole: PageRoleFrontCover}); !productionErrorIs(err, CodeConflict) {
		t.Fatalf("duplicate front cover error=%v", err)
	}

	reordered, err := h.service.ReorderSections(ctx, chapter.UUID, []string{bodyB.UUID, bodyA.UUID})
	if err != nil {
		t.Fatal(err)
	}
	wantUUIDs = []string{front.UUID, bodyB.UUID, bodyA.UUID, back.UUID}
	for index := range wantUUIDs {
		if reordered[index].UUID != wantUUIDs[index] || reordered[index].PageRole != wantRoles[index] || reordered[index].SectionNo != index+1 {
			t.Fatalf("reordered page %d=%+v", index+1, reordered[index])
		}
	}
	legacyFullOrder := []string{back.UUID, bodyA.UUID, front.UUID, bodyB.UUID}
	legacyReordered, err := h.service.ReorderSections(ctx, chapter.UUID, legacyFullOrder)
	if err != nil || len(legacyReordered) != 4 || legacyReordered[0].UUID != front.UUID || legacyReordered[1].UUID != bodyA.UUID || legacyReordered[2].UUID != bodyB.UUID || legacyReordered[3].UUID != back.UUID {
		t.Fatalf("legacy full-page reorder=%+v error=%v", legacyReordered, err)
	}
	if _, err := h.service.ReorderSections(ctx, chapter.UUID, []string{bodyA.UUID}); !productionErrorIs(err, CodeValidation) {
		t.Fatalf("incomplete body reorder error=%v", err)
	}
}

func TestComicPageRolesRejectVerticalSpecialPagesAndProtectLastBody(t *testing.T) {
	ctx := context.Background()
	vertical := newProductionHarness(t)
	verticalChapter, err := vertical.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Vertical"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vertical.service.CreateSection(ctx, verticalChapter.UUID, CreateSectionInput{PageRole: PageRoleFrontCover}); !productionErrorIs(err, CodeValidation) {
		t.Fatalf("vertical cover error=%v", err)
	}
	verticalBody, err := vertical.service.CreateSection(ctx, verticalChapter.UUID, CreateSectionInput{Title: "Only strip section"})
	if err != nil {
		t.Fatal(err)
	}
	if err := vertical.service.DeleteSection(ctx, verticalChapter.UUID, verticalBody.UUID, verticalBody.Revision); err != nil {
		t.Fatalf("vertical last body deletion error=%v", err)
	}
	verticalSections, err := vertical.service.ListSections(ctx, verticalChapter.UUID)
	if err != nil || len(verticalSections) != 0 {
		t.Fatalf("vertical sections after delete=%+v error=%v", verticalSections, err)
	}

	classic := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	chapter, err := classic.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Body invariant"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := classic.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Only body"})
	if err != nil {
		t.Fatal(err)
	}
	if err := classic.service.DeleteSection(ctx, chapter.UUID, body.UUID, body.Revision); !productionErrorIs(err, CodeConflict) {
		t.Fatalf("delete last body error=%v", err)
	}
	backRole := PageRoleBackCover
	if _, err := classic.service.UpdateSection(ctx, chapter.UUID, body.UUID, UpdateSectionInput{PageRole: &backRole, ExpectedRevision: body.Revision}); !productionErrorIs(err, CodeConflict) {
		t.Fatalf("convert last body error=%v", err)
	}
	second, err := classic.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Second body"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := classic.service.UpdateSection(ctx, chapter.UUID, second.UUID, UpdateSectionInput{PageRole: &backRole, ExpectedRevision: second.Revision})
	if err != nil || updated.PageRole != PageRoleBackCover || updated.SectionNo != 2 {
		t.Fatalf("convert body to back=%+v error=%v", updated, err)
	}
}

func TestGeneratedComicSectionsReplaceOnlyBodyAndSnapshotPageRoles(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Generated body"})
	if err != nil {
		t.Fatal(err)
	}
	generated := []GeneratedComicSection{{Title: "One", StoryboardMD: "Board one"}, {Title: "Two", StoryboardMD: "Board two"}}
	body, err := h.service.CreateGeneratedSections(ctx, chapter.UUID, generated)
	if err != nil || len(body) != 2 || body[0].PageRole != PageRoleBody || body[1].PageRole != PageRoleBody {
		t.Fatalf("generated body=%+v error=%v", body, err)
	}
	front, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Front", StoryboardMD: "Front board", PageRole: PageRoleFrontCover})
	if err != nil {
		t.Fatal(err)
	}
	back, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Back", StoryboardMD: "Back board", PageRole: PageRoleBackCover})
	if err != nil {
		t.Fatal(err)
	}
	state, err := h.service.GetComicState(ctx, chapter.UUID)
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := h.service.ReplaceGeneratedSections(ctx, chapter.UUID, []GeneratedComicSection{{Title: "Replacement", StoryboardMD: "Replacement board"}}, state.Revision)
	if err != nil || len(replaced) != 1 || replaced[0].PageRole != PageRoleBody {
		t.Fatalf("replaced body=%+v error=%v", replaced, err)
	}
	all, err := h.service.ListSections(ctx, chapter.UUID)
	if err != nil || len(all) != 3 || all[0].UUID != front.UUID || all[0].PageRole != PageRoleFrontCover || all[1].UUID != replaced[0].UUID || all[2].UUID != back.UUID || all[2].PageRole != PageRoleBackCover {
		t.Fatalf("preserved special pages=%+v error=%v", all, err)
	}
	snapshots, err := h.service.ListChapterSnapshots(ctx, chapter.UUID)
	if err != nil || len(snapshots) == 0 {
		t.Fatalf("snapshots=%+v error=%v", snapshots, err)
	}
	detail, err := h.service.GetChapterSnapshot(ctx, chapter.UUID, snapshots[0].UUID)
	if err != nil || detail.SchemaVersion != 3 || len(detail.Sections) != 3 || detail.Sections[0].PageRole != PageRoleFrontCover || detail.Sections[1].PageRole != PageRoleBody || detail.Sections[2].PageRole != PageRoleBackCover {
		t.Fatalf("snapshot detail=%+v error=%v", detail, err)
	}
	var beforeOverwriteUUID string
	for _, snapshot := range snapshots {
		if snapshot.Reason == "before_storyboard_overwrite" {
			beforeOverwriteUUID = snapshot.UUID
			break
		}
	}
	if beforeOverwriteUUID == "" {
		t.Fatal("missing before-overwrite snapshot")
	}
	restored, err := h.service.RestoreChapterSnapshot(ctx, chapter.UUID, beforeOverwriteUUID)
	if err != nil || len(restored) != 4 || restored[0].UUID != front.UUID || restored[0].PageRole != PageRoleFrontCover || restored[1].UUID != body[0].UUID || restored[2].UUID != body[1].UUID || restored[3].UUID != back.UUID || restored[3].PageRole != PageRoleBackCover {
		t.Fatalf("restored page roles=%+v error=%v", restored, err)
	}
}

func TestComicStateReadyRequiresBodyAndEveryExistingPageImage(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Ready book"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Body", StoryboardMD: "Body board"})
	if err != nil {
		t.Fatal(err)
	}
	front, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Front", StoryboardMD: "Front board", PageRole: PageRoleFrontCover})
	if err != nil {
		t.Fatal(err)
	}
	front, err = h.service.ImportSectionImage(ctx, chapter.UUID, front.UUID, upload(t, h.service, "comic_section_image", imageBytes(t, 10)), front.Revision)
	if err != nil {
		t.Fatal(err)
	}
	state, err := h.service.GetComicState(ctx, chapter.UUID)
	if err != nil || state.Status == "ready" {
		t.Fatalf("rendered cover with missing body image state=%+v error=%v", state, err)
	}
	body, err = h.service.ImportSectionImage(ctx, chapter.UUID, body.UUID, upload(t, h.service, "comic_section_image", imageBytes(t, 20)), body.Revision)
	if err != nil {
		t.Fatal(err)
	}
	state, err = h.service.GetComicState(ctx, chapter.UUID)
	if err != nil || state.Status != "ready" {
		t.Fatalf("rendered cover and body state=%+v error=%v", state, err)
	}
	back, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Back", StoryboardMD: "Back board", PageRole: PageRoleBackCover})
	if err != nil {
		t.Fatal(err)
	}
	state, err = h.service.GetComicState(ctx, chapter.UUID)
	if err != nil || state.Status == "ready" {
		t.Fatalf("missing back image state=%+v error=%v", state, err)
	}
	if _, err := h.service.ImportSectionImage(ctx, chapter.UUID, back.UUID, upload(t, h.service, "comic_section_image", imageBytes(t, 30)), back.Revision); err != nil {
		t.Fatal(err)
	}
	state, err = h.service.GetComicState(ctx, chapter.UUID)
	if err != nil || state.Status != "ready" {
		t.Fatalf("fully rendered book state=%+v error=%v", state, err)
	}
}

func TestSnapshotRestoreRequiresBodyOnlyForOrdinaryPictureBooks(t *testing.T) {
	ctx := context.Background()
	classic := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	chapter, err := classic.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Restore invariant"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := classic.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Body"})
	if err != nil {
		t.Fatal(err)
	}
	front, err := classic.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Front", PageRole: PageRoleFrontCover})
	if err != nil {
		t.Fatal(err)
	}
	coverOnly := insertPageRoleSnapshot(t, classic.service, chapter.UUID, chapterSnapshotPayload{Version: 3, Sections: []snapshotSection{{UUID: front.UUID, PageRole: PageRoleFrontCover, Title: front.Title, SectionNo: 1}}})
	if _, err := classic.service.RestoreChapterSnapshot(ctx, chapter.UUID, coverOnly); !productionErrorIs(err, CodeSnapshotInvalid) {
		t.Fatalf("classic cover-only restore error=%v", err)
	}
	sections, err := classic.service.ListSections(ctx, chapter.UUID)
	if err != nil || len(sections) != 2 || sections[1].UUID != body.UUID {
		t.Fatalf("rejected restore changed classic sections=%+v error=%v", sections, err)
	}

	vertical := newProductionHarness(t)
	verticalChapter, err := vertical.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Empty strip snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	empty := insertPageRoleSnapshot(t, vertical.service, verticalChapter.UUID, chapterSnapshotPayload{Version: 1, Sections: []snapshotSection{}})
	restored, err := vertical.service.RestoreChapterSnapshot(ctx, verticalChapter.UUID, empty)
	if err != nil || len(restored) != 0 {
		t.Fatalf("vertical empty restore=%+v error=%v", restored, err)
	}
}

func TestGeneratedImageCommitKeepsLegacyV4SnapshotCompatibility(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Legacy image task"})
	if err != nil {
		t.Fatal(err)
	}
	section, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Body", StoryboardMD: "Body board"})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := json.Marshal(GenerationSnapshot{Version: 4, Kind: "comic_image_generation", ResourceUUID: section.UUID, ChapterUUID: chapter.UUID})
	if err != nil {
		t.Fatal(err)
	}
	committed, err := h.service.CommitGeneratedSectionImage(ctx, chapter.UUID, section.UUID, "", legacy, bytes.NewReader(imageBytes(t, 41)))
	if err != nil || committed.CurrentImage == nil || committed.PageRole != PageRoleBody {
		t.Fatalf("legacy v4 commit=%+v error=%v", committed, err)
	}
}

func insertPageRoleSnapshot(t *testing.T, service *Service, chapterUUID string, payload chapterSnapshotPayload) string {
	t.Helper()
	ctx := context.Background()
	state, _, err := service.ensureComicState(ctx, service.store.DB(), chapterUUID)
	if err != nil {
		t.Fatal(err)
	}
	_, actor, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	uuid, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err := service.store.DB().Model(&chapterSnapshotRecord{}).Where("chapter_comic_state_id = ?", state.ID).Select("COALESCE(MAX(version_no),0)+1").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	row := chapterSnapshotRecord{UUID: uuid, ChapterComicStateID: state.ID, ActorID: actor.ID, VersionNo: version, Reason: "fixture", SnapshotJSON: string(encoded), SnapshotHash: hashJSON(encoded), CreatedAt: service.now().UTC()}
	if err := service.store.DB().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	return uuid
}
