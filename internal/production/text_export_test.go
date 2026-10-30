package production

import (
	"archive/zip"
	"os"
	"strings"
	"testing"

	"lumi/internal/project"
	"lumi/internal/story"
)

func TestTextExportIncludesExistingImagesAndReusesPDFRenderer(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	ctx := t.Context()
	ch, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Text and art"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := h.service.CreateSection(ctx, ch.UUID, CreateSectionInput{Title: "Opening", DescriptionMD: "A fox in rain.", StoryboardMD: "A warm watercolor forest."})
	if err != nil {
		t.Fatal(err)
	}
	page, err = h.service.ImportSectionImage(ctx, ch.UUID, page.UUID, upload(t, h.service, "comic_section_image", imageBytes(t, 12)), page.Revision)
	if err != nil {
		t.Fatal(err)
	}
	missing, err := h.service.CreateSection(ctx, ch.UUID, CreateSectionInput{Title: "Ending", DescriptionMD: "The star returns home.", StoryboardMD: "Golden sky."})
	if err != nil {
		t.Fatal(err)
	}
	p, err := h.stories.GetProject(ctx)
	if err != nil {
		t.Fatal(err)
	}
	book := TextBook{Project: p, Chapters: []TextChapter{{Chapter: ch, Pages: []ComicSection{page, missing}}}}
	archive, err := h.service.ExportTextBook(ctx, book, "zip", true)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(archive.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	found := false
	for _, f := range z.File {
		if strings.HasPrefix(f.Name, "images/vol01.ch01/pages/page-001.") {
			found = true
		}
	}
	if !found {
		t.Fatal("ZIP lost existing page artwork")
	}
	pdf, err := h.service.ExportTextBook(ctx, book, "pdf", true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(pdf.Path)
	if err != nil {
		t.Fatal(err)
	}
	assertPDFDocument(t, b, 1)
	if pdf.MissingImageCount != 1 {
		t.Fatalf("missing count=%d", pdf.MissingImageCount)
	}
}
