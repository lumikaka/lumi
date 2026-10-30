package production

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
	"lumi/internal/files"
	"lumi/internal/project"
	"lumi/internal/story"
)

func hostFixture(t *testing.T, width, height int, jpegFormat bool) (string, []byte) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x % 251), G: uint8(y % 251), B: 70, A: 255})
		}
	}
	var data bytes.Buffer
	var err error
	if jpegFormat {
		err = jpeg.Encode(&data, img, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&data, img)
	}
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately mislabel every supported fixture: content determines format.
	path := filepath.Join(t.TempDir(), "host-art.wrong-extension")
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, data.Bytes()
}

func hostPage(t *testing.T, h *productionHarness) (string, ComicSection) {
	t.Helper()
	ch, err := h.stories.CreateChapter(t.Context(), story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Host artwork"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := h.service.CreateSection(t.Context(), ch.UUID, CreateSectionInput{Title: "Page", StoryboardMD: "A fox under the stars"})
	if err != nil {
		t.Fatal(err)
	}
	return ch.UUID, p
}

func TestHostImageRatiosAndImmutableVersions(t *testing.T) {
	for _, tc := range []struct {
		name, fit        string
		w, h, outW, outH int
		matches          bool
	}{
		{"exact", "", 400, 300, 400, 300, true},
		{"within-tolerance", "crop", 403, 300, 403, 300, true},
		{"keep", "keep", 600, 300, 600, 300, false},
		{"center-crop", "crop", 600, 300, 400, 300, false},
		{"white-padding", "pad", 600, 300, 600, 450, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
			chapter, page := hostPage(t, h)
			path, original := hostFixture(t, tc.w, tc.h, false)
			input := ImportHostImageInput{Path: path, TargetUUID: page.UUID, Slot: "page_image", FitMode: tc.fit, ExpectedRevision: page.Revision, Prompt: strings.Repeat("原始提示词\n", 300), Description: "宿主画面"}
			result, err := h.service.ImportHostImage(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			if *result.Asset.Width != tc.outW || *result.Asset.Height != tc.outH || result.RatioMatches != tc.matches || (len(result.Warnings) == 0) != tc.matches || result.Revision != page.Revision+1 {
				t.Fatalf("result=%+v asset=%+v", result, result.Asset)
			}
			meta := result.Asset.Metadata.(map[string]any)
			metadataJSON, _ := json.Marshal(meta)
			if meta["origin"] != "host_generated" || meta["original_prompt"] != input.Prompt || meta["description"] != input.Description || strings.Contains(string(metadataJSON), path) {
				t.Fatalf("lost prompt/origin or retained source path: %v", meta)
			}
			content, err := h.service.files.OpenContent(t.Context(), result.Asset.UUID)
			if err != nil {
				t.Fatal(err)
			}
			decoded, _, err := image.Decode(content.File)
			content.File.Close()
			if err != nil {
				t.Fatal(err)
			}
			if tc.fit == "crop" && !tc.matches {
				if got := color.NRGBAModel.Convert(decoded.At(0, 0)).(color.NRGBA); got.R != 100 || got.G != 0 {
					t.Fatalf("crop was not centered: %v", got)
				}
			}
			if tc.fit == "pad" {
				if got := color.NRGBAModel.Convert(decoded.At(0, 0)).(color.NRGBA); got != (color.NRGBA{255, 255, 255, 255}) {
					t.Fatalf("padding not white: %v", got)
				}
				if got := color.NRGBAModel.Convert(decoded.At(0, 75)).(color.NRGBA); got != (color.NRGBA{0, 0, 70, 255}) {
					t.Fatalf("image not centered in padding: %v", got)
				}
			}
			if tc.fit == "crop" && !tc.matches || tc.fit == "pad" {
				if result.Asset.SourceAssetUUID == "" {
					t.Fatal("derived image lost original")
				}
				source, err := h.service.files.OpenContent(t.Context(), result.Asset.SourceAssetUUID)
				if err != nil {
					t.Fatal(err)
				}
				data, _ := io.ReadAll(source.File)
				source.File.Close()
				if !bytes.Equal(data, original) {
					t.Fatal("original bytes changed")
				}
			}
			input.ExpectedRevision = result.Revision
			second, err := h.service.ImportHostImage(t.Context(), input)
			if err != nil || second.VersionNo != 2 || second.Asset.UUID == result.Asset.UUID {
				t.Fatalf("replacement=%+v err=%v", second, err)
			}
			_, err = h.service.ImportHostImage(t.Context(), input)
			assertHostError(t, err, CodeConflict)
			versions, err := h.service.ListImageVariants(t.Context(), chapter, page.UUID)
			if err != nil || len(versions) != 2 || versions[1].UUID != result.VariantUUID || versions[0].SourceType != "host_generated" {
				t.Fatalf("versions=%+v err=%v", versions, err)
			}
		})
	}
}

func assertHostError(t *testing.T, err error, want string) {
	t.Helper()
	var f *files.Error
	var p *Error
	got := ""
	if errors.As(err, &f) {
		got = f.Code
	} else if errors.As(err, &p) {
		got = p.Code
	}
	if got != want {
		t.Fatalf("code=%q want=%q err=%v", got, want, err)
	}
}

func TestHostImageInvalidInputsDoNotCreateVersions(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	chapter, page := hostPage(t, h)
	valid, data := hostFixture(t, 80, 60, false)
	corrupt := filepath.Join(t.TempDir(), "truncated.png")
	if err := os.WriteFile(corrupt, data[:len(data)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	spoof := filepath.Join(t.TempDir(), "fake.png")
	if err := os.WriteFile(spoof, []byte("This is text, not PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	pixels := filepath.Join(t.TempDir(), "too-many-pixels.png")
	header := append([]byte(nil), data...)
	binary.BigEndian.PutUint32(header[16:20], 10000)
	binary.BigEndian.PutUint32(header[20:24], 10000)
	binary.BigEndian.PutUint32(header[29:33], crc32.ChecksumIEEE(header[12:29]))
	if err := os.WriteFile(pixels, header, 0o600); err != nil {
		t.Fatal(err)
	}
	large := filepath.Join(t.TempDir(), "huge.png")
	f, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(HostImageMaxBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	for _, tc := range []struct{ path, target, fit, code string }{
		{filepath.Join(t.TempDir(), "missing.png"), page.UUID, "", "image_path_unavailable"},
		{t.TempDir(), page.UUID, "", "image_path_unavailable"},
		{"relative.png", page.UUID, "", files.CodeUnsafePath},
		{spoof, page.UUID, "", files.CodeInvalidContent},
		{corrupt, page.UUID, "", files.CodeInvalidContent},
		{large, page.UUID, "", files.CodeFileTooLarge},
		{pixels, page.UUID, "", files.CodePixelsTooLarge},
		{valid, mustUUID(t), "", CodeNotFound},
		{valid, page.UUID, "stretch", CodeValidation},
	} {
		_, err := h.service.ImportHostImage(t.Context(), ImportHostImageInput{Path: tc.path, TargetUUID: tc.target, Slot: "page_image", FitMode: tc.fit, ExpectedRevision: page.Revision})
		assertHostError(t, err, tc.code)
	}
	versions, err := h.service.ListImageVariants(t.Context(), chapter, page.UUID)
	if err != nil || len(versions) != 0 {
		t.Fatalf("invalid import created versions: %v %v", versions, err)
	}
}

func TestHostSettingImportAndGuidance(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	profile, err := h.service.GetPremise(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"character", "scene", "prop", "reference"} {
		setting, err := h.service.CreateTextPremiseAsset(t.Context(), CreateAssetInput{AssetType: kind, Title: kind, Summary: "Reference"}, profile.Revision)
		if err != nil {
			t.Fatal(err)
		}
		profile, _ = h.service.GetPremise(t.Context())
		target, err := h.service.HostImageTarget(t.Context(), setting.UUID, "setting_image")
		if err != nil || target.SuggestedFit != "pad" || target.ExpectedRevision != setting.Revision || target.GenerationHint == "" {
			t.Fatalf("target=%+v err=%v", target, err)
		}
		want := ImageRatio{1, 1}
		if kind == "scene" {
			want = ImageRatio{4, 3}
		}
		if target.TargetRatio != want {
			t.Fatalf("%s target=%+v", kind, target)
		}
		path, _ := hostFixture(t, 120, 80, true)
		first, err := h.service.ImportHostImage(t.Context(), ImportHostImageInput{Path: path, TargetUUID: setting.UUID, Slot: "setting_image", ExpectedRevision: setting.Revision, Prompt: "Keep this character"})
		if err != nil || first.Asset.MIMEType != "image/jpeg" {
			t.Fatalf("import=%+v err=%v", first, err)
		}
		second, err := h.service.ImportHostImage(t.Context(), ImportHostImageInput{Path: path, TargetUUID: setting.UUID, Slot: "setting_image", FitMode: "pad", ExpectedRevision: first.Revision})
		if err != nil || second.VersionNo != 2 {
			t.Fatalf("second=%+v err=%v", second, err)
		}
		versions, err := h.service.ListAssetVariants(t.Context(), setting.UUID)
		if err != nil || len(versions) != 2 || versions[0].SourceType != "host_generated" {
			t.Fatalf("versions=%+v err=%v", versions, err)
		}
		_, err = h.service.ImportHostImage(t.Context(), ImportHostImageInput{Path: path, TargetUUID: setting.UUID, Slot: "setting_image", ExpectedRevision: first.Revision})
		assertHostError(t, err, CodeConflict)
	}
}

func TestHostImageWebPUsesContentFormat(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	_, page := hostPage(t, h)
	data, err := os.ReadFile("../../root-site/assets/lumi-preview.webp")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "disguised.png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := h.service.ImportHostImage(t.Context(), ImportHostImageInput{Path: path, TargetUUID: page.UUID, Slot: "page_image", ExpectedRevision: page.Revision})
	if err != nil || result.Asset.MIMEType != "image/webp" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestImageBindingRechecksRevisionInsideAssetCommit(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	chapter, page := hostPage(t, h)
	_, data := hostFixture(t, 80, 60, false)
	var before int64
	if err := h.service.store.DB().Table("files").Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	// Simulate another writer after host preflight, before the final Asset
	// transaction. The shared binding must roll back File + variant together.
	_, err := h.service.importSectionImage(t.Context(), chapter, page.UUID, page.Revision, func(bind func(*gorm.DB, int64) error) error {
		if err := h.service.store.DB().Model(&comicSectionRecord{}).Where("uuid=?", page.UUID).Update("revision", gorm.Expr("revision+1")).Error; err != nil {
			return err
		}
		_, err := h.service.files.CommitReader(t.Context(), files.CommitInput{Purpose: "comic_section_image", OriginalFilename: "host.png", SourceType: "imported", Metadata: map[string]any{"origin": "host_generated"}, Reader: bytes.NewReader(data), Bind: bind})
		return err
	})
	assertHostError(t, err, CodeConflict)
	var after int64
	if err := h.service.store.DB().Table("files").Count(&after).Error; err != nil || before != after {
		t.Fatalf("File escaped rollback: %d -> %d, err=%v", before, after, err)
	}
	current, err := h.service.GetSection(t.Context(), chapter, page.UUID)
	if err != nil || current.CurrentImage != nil || current.Revision != page.Revision+1 {
		t.Fatalf("failed import changed current image: %+v err=%v", current, err)
	}
	versions, err := h.service.ListImageVariants(t.Context(), chapter, page.UUID)
	if err != nil || len(versions) != 0 {
		t.Fatalf("variant escaped rollback: %+v err=%v", versions, err)
	}
}
