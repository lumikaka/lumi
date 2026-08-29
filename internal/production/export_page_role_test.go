package production

import (
	"context"
	"testing"
	"time"

	"lumi/internal/project"
	"lumi/internal/story"
)

func TestExportReadinessRequiresReadyBodyAndFreezesPageRoles(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Picture book", Content: "Story", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	bodyOne, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Body one", StoryboardMD: "First page", PageRole: PageRoleBody})
	if err != nil {
		t.Fatal(err)
	}
	front, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Front", StoryboardMD: "Cover", PageRole: PageRoleFrontCover})
	if err != nil {
		t.Fatal(err)
	}
	bodyTwo, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Body two", StoryboardMD: "Second page", PageRole: PageRoleBody})
	if err != nil {
		t.Fatal(err)
	}
	back, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Back", StoryboardMD: "Back cover", PageRole: PageRoleBackCover})
	if err != nil {
		t.Fatal(err)
	}
	bodyTwo, err = h.service.ImportSectionImage(ctx, chapter.UUID, bodyTwo.UUID, upload(t, h.service, "comic_section_image", imageBytes(t, 61)), bodyTwo.Revision)
	if err != nil || bodyTwo.CurrentImage == nil {
		t.Fatalf("body image=%+v err=%v", bodyTwo, err)
	}

	readiness, err := h.service.ExportReadiness(ctx, "chapter", chapter.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if !readiness.CanExport || readiness.Complete || readiness.ActiveSectionCount != 4 || readiness.ImageSectionCount != 1 || readiness.MissingSectionCount != 3 {
		t.Fatalf("readiness=%+v", readiness)
	}
	wantMissing := []struct {
		uuid       string
		role       string
		bodyPageNo int
	}{
		{uuid: front.UUID, role: PageRoleFrontCover},
		{uuid: bodyOne.UUID, role: PageRoleBody, bodyPageNo: 1},
		{uuid: back.UUID, role: PageRoleBackCover},
	}
	for index, want := range wantMissing {
		got := readiness.MissingSections[index]
		if got.UUID != want.uuid || got.PageRole != want.role || got.BodyPageNo != want.bodyPageNo {
			t.Fatalf("missing[%d]=%+v want=%+v", index, got, want)
		}
	}

	snapshot, _, err := h.service.BuildExportSnapshotWithOptions(ctx, "chapter", chapter.UUID, true)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != exportSnapshotV6 || len(snapshot.Entries) != 1 || snapshot.Entries[0].PageRole != PageRoleBody || snapshot.Entries[0].BodyPageNo != 2 {
		t.Fatalf("snapshot entries=%+v version=%d", snapshot.Entries, snapshot.Version)
	}
	if len(snapshot.MissingSections) != len(wantMissing) || snapshot.MissingSections[1].PageRole != PageRoleBody || snapshot.MissingSections[1].BodyPageNo != 1 {
		t.Fatalf("snapshot missing=%+v", snapshot.MissingSections)
	}
}

func TestExportReadinessDoesNotTreatCoverAsExportableBody(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Cover only", Content: "Story", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Legacy body", StoryboardMD: "Body"})
	if err != nil {
		t.Fatal(err)
	}
	front, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Front", StoryboardMD: "Cover", PageRole: PageRoleFrontCover})
	if err != nil {
		t.Fatal(err)
	}
	front, err = h.service.ImportSectionImage(ctx, chapter.UUID, front.UUID, upload(t, h.service, "comic_section_image", imageBytes(t, 62)), front.Revision)
	if err != nil || front.CurrentImage == nil {
		t.Fatalf("front image=%+v err=%v", front, err)
	}
	// A pre-invariant database can still contain a cover-only sequence. Export
	// readiness must not mistake that cover image for an exportable body page.
	if err := h.service.store.DB().Model(&comicSectionRecord{}).Where("uuid = ?", body.UUID).Update("deleted_at", time.Now().UTC()).Error; err != nil {
		t.Fatal(err)
	}

	readiness, err := h.service.ExportReadiness(ctx, "chapter", chapter.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if readiness.CanExport || readiness.Complete || readiness.ImageSectionCount != 1 || readiness.MissingSectionCount != 0 {
		t.Fatalf("cover-only readiness=%+v", readiness)
	}
	if _, _, err := h.service.BuildExportSnapshotWithOptions(ctx, "chapter", chapter.UUID, true); !productionErrorIs(err, CodeExportEmpty) {
		t.Fatalf("cover-only export error=%v", err)
	}
}
