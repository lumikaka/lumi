package production

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"regexp"
	"testing"

	"lumi/internal/project"
	"lumi/internal/story"
)

func TestPDFLayoutClassifiesPictureBookRatios(t *testing.T) {
	tests := []struct {
		name      string
		profile   project.PictureBookProfile
		placement string
		wantSlots int
	}{
		{name: "landscape", profile: project.PictureBookProfile{Format: project.PictureBookClassic, AspectRatio: project.AspectRatio{Width: 4, Height: 3}}, placement: ExportPDFTwoUpStacked, wantSlots: 2},
		{name: "custom landscape", profile: project.PictureBookProfile{Format: project.PictureBookWordless, AspectRatio: project.AspectRatio{Width: 3, Height: 2}}, placement: ExportPDFTwoUpStacked, wantSlots: 2},
		{name: "interactive", profile: project.PictureBookProfile{Format: project.PictureBookInteractive, AspectRatio: project.AspectRatio{Width: 4, Height: 3}}, placement: ExportPDFTwoUpStacked, wantSlots: 2},
		{name: "vertical strip", profile: project.PictureBookProfile{Format: project.PictureBookVertical, AspectRatio: project.AspectRatio{Width: 1, Height: 3}}, placement: ExportPDFTwoUpColumns, wantSlots: 2},
		{name: "square", profile: project.PictureBookProfile{Format: project.PictureBookClassic, AspectRatio: project.AspectRatio{Width: 1, Height: 1}}, placement: ExportPDFOneUp, wantSlots: 1},
		{name: "portrait", profile: project.PictureBookProfile{Format: project.PictureBookClassic, AspectRatio: project.AspectRatio{Width: 3, Height: 4}}, placement: ExportPDFOneUp, wantSlots: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			layout := pdfLayoutForPictureBook(test.profile)
			if layout.Placement != test.placement || layout.PageSize != ExportPDFPageSizeA4Portrait || layout.MarginMM != 12 || layout.GutterMM != 6 || layout.RendererVersion != 2 {
				t.Fatalf("layout=%+v", layout)
			}
			if slots := exportPDFSlots(layout); len(slots) != test.wantSlots {
				t.Fatalf("slots=%+v", slots)
			}
		})
	}
}

func TestPDFJPEGCompressionCapsResolutionWithoutUpscaling(t *testing.T) {
	value := image.NewNRGBA(image.Rect(0, 0, 1000, 750))
	state := uint32(1)
	for index := 0; index < len(value.Pix); index += 4 {
		state = state*1664525 + 1013904223
		value.Pix[index] = byte(state >> 24)
		state = state*1664525 + 1013904223
		value.Pix[index+1] = byte(state >> 24)
		state = state*1664525 + 1013904223
		value.Pix[index+2] = byte(state >> 24)
		value.Pix[index+3] = 255
	}
	var original bytes.Buffer
	if err := png.Encode(&original, value); err != nil {
		t.Fatal(err)
	}
	compressed, err := encodeExportPDFJPEG(value, exportPDFRect{W: 200, H: 150})
	if err != nil {
		t.Fatal(err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 500 || config.Height != 375 {
		t.Fatalf("compressed dimensions=%dx%d", config.Width, config.Height)
	}
	if len(compressed) >= original.Len() {
		t.Fatalf("compressed=%d original_png=%d", len(compressed), original.Len())
	}

	small := image.NewRGBA(image.Rect(0, 0, 20, 10))
	compressed, err = encodeExportPDFJPEG(small, exportPDFRect{W: 500, H: 500})
	if err != nil {
		t.Fatal(err)
	}
	config, err = jpeg.DecodeConfig(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 20 || config.Height != 10 {
		t.Fatalf("small image was upscaled to %dx%d", config.Width, config.Height)
	}
}

func TestPDFDownloadFilenameUsesProjectTitleAndChapterCode(t *testing.T) {
	projectSnapshot := ExportSnapshot{Version: 5, Format: ExportFormatPDF, ProjectTitle: "月光计划", Scope: "project"}
	if filename := exportPDFDownloadFilename(projectSnapshot, "ignored", ""); filename != "月光计划.pdf" {
		t.Fatalf("project filename=%q", filename)
	}
	chapterSnapshot := ExportSnapshot{
		Version: 5, Format: ExportFormatPDF, ProjectTitle: "月光/计划", Scope: "chapter",
		Entries: []ExportEntry{{ChapterCode: "vol01.ch01"}},
	}
	if filename := exportPDFDownloadFilename(chapterSnapshot, "ignored", "ignored"); filename != "月光-计划-vol01-ch01.pdf" {
		t.Fatalf("chapter filename=%q", filename)
	}
	legacySnapshot := ExportSnapshot{Version: 5, Format: ExportFormatPDF, Scope: "chapter"}
	if filename := exportPDFDownloadFilename(legacySnapshot, "旧项目", "vol02.ch03"); filename != "旧项目-vol02-ch03.pdf" {
		t.Fatalf("legacy filename=%q", filename)
	}
}

func TestPDFContainGeometryCentersWithoutCropping(t *testing.T) {
	slot := exportPDFRect{X: 10, Y: 20, W: 200, H: 100}
	wide, err := exportPDFContainRect(slot, 400, 100)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(wide.W-200) > 0.001 || math.Abs(wide.H-50) > 0.001 || math.Abs(wide.X-10) > 0.001 || math.Abs(wide.Y-45) > 0.001 {
		t.Fatalf("wide=%+v", wide)
	}
	tall, err := exportPDFContainRect(slot, 100, 400)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(tall.W-25) > 0.001 || math.Abs(tall.H-100) > 0.001 || math.Abs(tall.X-97.5) > 0.001 || math.Abs(tall.Y-20) > 0.001 {
		t.Fatalf("tall=%+v", tall)
	}
}

func TestPDFSnapshotAndRendererSupportImageFormats(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "月光下的第一章", Content: "Story", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		name string
		data []byte
		mime string
	}{
		{name: "transparent png", data: transparentPNG(t), mime: "image/png"},
		{name: "jpeg", data: jpegFixture(t), mime: "image/jpeg"},
		{name: "animated gif", data: gifFixture(t), mime: "image/gif"},
		{name: "webp", data: webpFixture(t), mime: "image/webp"},
	}
	for index, fixture := range fixtures {
		section, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: fixture.name, StoryboardMD: fixture.name})
		if err != nil {
			t.Fatal(err)
		}
		section, err = h.service.ImportSectionImage(ctx, chapter.UUID, section.UUID, upload(t, h.service, "comic_section_image", fixture.data), section.Revision)
		if err != nil || section.CurrentImage == nil {
			t.Fatalf("fixture %d import=%+v err=%v", index, section, err)
		}
	}

	zipSnapshot, zipHash, err := h.service.BuildExportSnapshotWithOptions(ctx, "chapter", chapter.UUID, false)
	if err != nil {
		t.Fatal(err)
	}
	explicitZIP, explicitZIPHash, err := h.service.BuildExportSnapshotForFormat(ctx, "chapter", chapter.UUID, false, "ZIP")
	if err != nil {
		t.Fatal(err)
	}
	if zipHash != explicitZIPHash || zipSnapshot.Version != exportSnapshotV6 || explicitZIP.Format != "" || explicitZIP.ProjectTitle != "" || explicitZIP.PDFLayout != nil {
		t.Fatalf("zip compatibility hash=%s/%s snapshot=%+v", zipHash, explicitZIPHash, explicitZIP)
	}
	for _, entry := range explicitZIP.Entries {
		if entry.ChapterUUID != "" || entry.MIMEType != "" || entry.Width != 0 || entry.Height != 0 {
			t.Fatalf("zip entry leaked PDF metadata: %+v", entry)
		}
	}

	snapshot, pdfHash, err := h.service.BuildExportSnapshotForFormat(ctx, "chapter", chapter.UUID, false, ExportFormatPDF)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != exportSnapshotV6 || snapshot.Format != ExportFormatPDF || snapshot.ProjectTitle != h.project.Name || len(pdfHash) != 64 || pdfHash == zipHash || snapshot.PDFLayout == nil || snapshot.PDFLayout.Placement != ExportPDFTwoUpColumns || snapshot.PDFLayout.RendererVersion != 2 {
		t.Fatalf("pdf snapshot=%+v hash=%s", snapshot, pdfHash)
	}
	if len(snapshot.Entries) != len(fixtures) {
		t.Fatalf("entries=%d", len(snapshot.Entries))
	}
	for index, entry := range snapshot.Entries {
		if entry.ChapterUUID != chapter.UUID || entry.MIMEType != fixtures[index].mime || entry.Width <= 0 || entry.Height <= 0 {
			t.Fatalf("entry[%d]=%+v", index, entry)
		}
	}

	var output bytes.Buffer
	progress := []int{}
	if err := h.service.writePDF(ctx, &output, snapshot, func(value int) error {
		progress = append(progress, value)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertPDFDocument(t, output.Bytes(), 2)
	if !bytes.Contains(output.Bytes(), []byte("DCTDecode")) {
		t.Fatal("PDF v2 did not embed compressed JPEG image streams")
	}
	if len(progress) < 2 || progress[0] != 10 || progress[len(progress)-1] != 80 {
		t.Fatalf("progress=%v", progress)
	}

	oneUp := snapshot
	oneUpLayout := *snapshot.PDFLayout
	oneUpLayout.Placement = ExportPDFOneUp
	oneUp.PDFLayout = &oneUpLayout
	output.Reset()
	if err := h.service.writePDF(ctx, &output, oneUp, nil); err != nil {
		t.Fatal(err)
	}
	assertPDFDocument(t, output.Bytes(), 4)
}

func TestProjectPDFNeverPairsDifferentChapters(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "One", Content: "Story", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	section, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "One", StoryboardMD: "One"})
	if err != nil {
		t.Fatal(err)
	}
	section, err = h.service.ImportSectionImage(ctx, chapter.UUID, section.UUID, upload(t, h.service, "comic_section_image", imageBytes(t, 40)), section.Revision)
	if err != nil || section.CurrentImage == nil {
		t.Fatalf("section=%+v err=%v", section, err)
	}
	asset := section.CurrentImage.Asset
	width, height := 1, 1
	if asset.Width != nil {
		width = *asset.Width
	}
	if asset.Height != nil {
		height = *asset.Height
	}
	snapshot := ExportSnapshot{
		Version: 5, Format: ExportFormatPDF, ProjectUUID: h.project.UUID, Scope: "project", ActiveChapterCount: 2, SectionCount: 2, ExportedSectionCount: 2,
		PDFLayout: &ExportPDFLayout{PageSize: ExportPDFPageSizeA4Portrait, Placement: ExportPDFTwoUpStacked, MarginMM: 12, GutterMM: 6, RendererVersion: 1},
		Entries: []ExportEntry{
			{ChapterUUID: chapter.UUID, ChapterCode: "one", SectionNo: 1, SectionUUID: mustUUID(t), ImageAssetUUID: asset.UUID, MIMEType: asset.MIMEType, Width: width, Height: height, Extension: "png"},
			{ChapterUUID: mustUUID(t), ChapterCode: "two", SectionNo: 1, SectionUUID: mustUUID(t), ImageAssetUUID: asset.UUID, MIMEType: asset.MIMEType, Width: width, Height: height, Extension: "png"},
		},
	}
	var output bytes.Buffer
	if err := h.service.writePDF(ctx, &output, snapshot, nil); err != nil {
		t.Fatal(err)
	}
	assertPDFDocument(t, output.Bytes(), 2)
}

func TestPDFCoversAreOneUpWhileBodyKeepsConfiguredLayout(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	ctx := context.Background()
	chapter, err := h.stories.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "One", Content: "Story", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	section, err := h.service.CreateSection(ctx, chapter.UUID, CreateSectionInput{Title: "Body", StoryboardMD: "Body", PageRole: PageRoleBody})
	if err != nil {
		t.Fatal(err)
	}
	section, err = h.service.ImportSectionImage(ctx, chapter.UUID, section.UUID, upload(t, h.service, "comic_section_image", imageBytes(t, 41)), section.Revision)
	if err != nil || section.CurrentImage == nil {
		t.Fatalf("section=%+v err=%v", section, err)
	}
	asset := section.CurrentImage.Asset
	width, height := 1, 1
	if asset.Width != nil {
		width = *asset.Width
	}
	if asset.Height != nil {
		height = *asset.Height
	}
	entry := func(role string, sectionNo int) ExportEntry {
		return ExportEntry{
			ChapterUUID: chapter.UUID, ChapterCode: chapter.ChapterCode, SectionNo: sectionNo,
			SectionUUID: mustUUID(t), PageRole: role, ImageAssetUUID: asset.UUID,
			MIMEType: asset.MIMEType, Width: width, Height: height, Extension: "png",
		}
	}
	snapshot := ExportSnapshot{
		Version: exportSnapshotV6, Format: ExportFormatPDF, ProjectUUID: h.project.UUID, Scope: "chapter",
		ActiveChapterCount: 1, SectionCount: 5, ExportedSectionCount: 5,
		PDFLayout: &ExportPDFLayout{PageSize: ExportPDFPageSizeA4Portrait, Placement: ExportPDFTwoUpStacked, MarginMM: 12, GutterMM: 6, RendererVersion: 1},
		Entries: []ExportEntry{
			entry(PageRoleFrontCover, 1),
			entry(PageRoleBody, 2),
			entry(PageRoleBody, 3),
			entry(PageRoleBody, 4),
			entry(PageRoleBackCover, 5),
		},
	}
	var output bytes.Buffer
	if err := h.service.writePDF(ctx, &output, snapshot, nil); err != nil {
		t.Fatal(err)
	}
	assertPDFDocument(t, output.Bytes(), 4)

	legacy := snapshot
	legacy.Version = 5
	for index := range legacy.Entries {
		legacy.Entries[index].PageRole = ""
	}
	output.Reset()
	if err := h.service.writePDF(ctx, &output, legacy, nil); err != nil {
		t.Fatal(err)
	}
	assertPDFDocument(t, output.Bytes(), 3)
}

func assertPDFDocument(t *testing.T, value []byte, pages int) {
	t.Helper()
	if !bytes.HasPrefix(value, []byte("%PDF-")) || !bytes.Contains(value, []byte("%%EOF")) {
		t.Fatalf("invalid PDF envelope: %q", value[:min(len(value), 16)])
	}
	pagePattern := regexp.MustCompile(`/Type\s*/Page(?:\s|/)`)
	if count := len(pagePattern.FindAll(value, -1)); count != pages {
		t.Fatalf("pages=%d want=%d", count, pages)
	}
	if !bytes.Contains(value, []byte("595.00 842.00")) && !bytes.Contains(value, []byte("595 842")) {
		t.Fatal("PDF does not contain an A4 portrait MediaBox")
	}
}

func transparentPNG(t *testing.T) []byte {
	t.Helper()
	value := image.NewNRGBA(image.Rect(0, 0, 12, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 12; x++ {
			value.Set(x, y, color.NRGBA{R: 220, G: 60, B: 80, A: uint8(30 + x*15)})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, value); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func jpegFixture(t *testing.T) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, 12, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 12; x++ {
			value.Set(x, y, color.RGBA{R: uint8(20 * x), G: uint8(25 * y), B: 140, A: 255})
		}
	}
	var output bytes.Buffer
	if err := jpeg.Encode(&output, value, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func gifFixture(t *testing.T) []byte {
	t.Helper()
	palette := color.Palette{color.Transparent, color.RGBA{R: 30, G: 180, B: 90, A: 255}, color.RGBA{R: 30, G: 80, B: 200, A: 255}}
	first := image.NewPaletted(image.Rect(0, 0, 12, 8), palette)
	second := image.NewPaletted(image.Rect(0, 0, 12, 8), palette)
	for index := range first.Pix {
		first.Pix[index] = 1
		second.Pix[index] = 2
	}
	var output bytes.Buffer
	if err := gif.EncodeAll(&output, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{0, 5}}); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func webpFixture(t *testing.T) []byte {
	t.Helper()
	const encoded = "UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA=="
	value, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
