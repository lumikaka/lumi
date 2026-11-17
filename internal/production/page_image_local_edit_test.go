package production

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"lumi/internal/project"
)

func TestLocalEditPreviewCoordinatesAndPatchAlignment(t *testing.T) {
	// Rectangle selected in a 300×900 preview of a 1200×3600 original.
	selection := NormalizedImageRect{60.0 / 300, 135.0 / 900, 30.0 / 300, 45.0 / 900}
	s, c, err := localEditGeometry(selection, 1200, 3600)
	if err != nil || s != (ImagePixelRect{240, 540, 120, 180}) {
		t.Fatal(s, c, err)
	}
	if c.X >= s.X || c.Y >= s.Y || c.X+c.Width <= s.X+s.Width || c.Y+c.Height <= s.Y+s.Height {
		t.Fatal("context ring missing", c)
	}
	for _, r := range []NormalizedImageRect{{-0.1, 0, .1, .1}, {0, 0, 0, .1}, {.95, 0, .1, .1}, {math.NaN(), 0, .1, .1}, {1, 0, 1e-15, .1}, {0, math.Inf(1), .1, .1}} {
		if _, _, err := localEditGeometry(r, 1200, 3600); err == nil {
			t.Fatal("accepted invalid rect", r)
		}
	}
	for _, r := range []NormalizedImageRect{{0, 0, .1, .1}, {.9, .9, .1, .1}, {0, 0, 1, 1}} {
		s, c, err := localEditGeometry(r, 1200, 3600)
		if err != nil || !s.rect().In(c.rect()) || !c.rect().In(image.Rect(0, 0, 1200, 3600)) {
			t.Fatal(s, c, err)
		}
	}
	base := image.NewNRGBA(image.Rect(0, 0, 1200, 3600))
	for y := 0; y < 3600; y++ {
		for x := 0; x < 1200; x++ {
			base.SetNRGBA(x, y, color.NRGBA{uint8(x % 251), uint8(y % 251), 30, 255})
		}
	}
	// Return 2x model pixels; an asymmetric target marks wrong offsets/mirroring.
	patch := image.NewNRGBA(image.Rect(0, 0, c.Width*2, c.Height*2))
	for y := 0; y < patch.Bounds().Dy(); y++ {
		for x := 0; x < patch.Bounds().Dx(); x++ {
			patch.SetNRGBA(x, y, color.NRGBA{uint8(x / 2 % 251), uint8(y / 2 % 251), 230, 255})
		}
	}
	edit := PageImageLocalEdit{SelectionPixels: s, CropPixels: c, FeatherPixels: 4}
	result := compositeLocalEdit(base, patch, edit)
	outside, inside := 0, 0
	for y := 0; y < 3600; y++ {
		for x := 0; x < 1200; x++ {
			if base.NRGBAAt(x, y) != color.NRGBAModel.Convert(result.At(x, y)).(color.NRGBA) {
				if image.Pt(x, y).In(s.rect()) {
					inside++
				} else {
					outside++
				}
			}
		}
	}
	if outside != 0 || inside == 0 {
		t.Fatal("pixel isolation", outside, inside)
	}
	x, y := s.X+30, s.Y+40
	got := color.NRGBAModel.Convert(result.At(x, y)).(color.NRGBA)
	if got.B != 230 || int(got.R) != ((x-c.X)%251) || int(got.G) != ((y-c.Y)%251) {
		t.Fatal("patch is displaced", got)
	}
	if color.NRGBAModel.Convert(result.At(s.X, s.Y)).(color.NRGBA) != base.NRGBAAt(s.X, s.Y) {
		t.Fatal("feather crosses boundary")
	}
}

func TestLocalEditHistoricalCandidateAndSignedReceipt(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	chapter, page := hostPage(t, h)
	path, _ := hostFixture(t, 400, 300, true) // JPEG source must remain identical outside after PNG save.
	firstPrepared := prepareHostPageFixture(t, h.service, page.UUID, "")
	first, err := h.service.ImportHostImage(t.Context(), ImportHostImageInput{Path: path, TargetUUID: page.UUID, Slot: "page_image", ExpectedRevision: page.Revision, Prompt: firstPrepared.GenerationPrompt, Preparation: &firstPrepared.Preparation})
	if err != nil {
		t.Fatal(err)
	}
	nextPrepared := prepareHostPageFixture(t, h.service, page.UUID, "")
	current, err := h.service.ImportHostImage(t.Context(), ImportHostImageInput{Path: path, TargetUUID: page.UUID, Slot: "page_image", ExpectedRevision: first.Revision, Prompt: nextPrepared.GenerationPrompt, Preparation: &nextPrepared.Preparation})
	if err != nil {
		t.Fatal(err)
	}
	refs := new(PageReferenceFiles)
	defer refs.Close()
	input := PreparePageImageInput{PageUUID: page.UUID, BaseVariantUUID: first.VariantUUID, Selection: &NormalizedImageRect{.2, .2, .2, .3}, AdditionalDirection: "Replace the gold ring with silver"}
	prepared, err := h.service.PreparePageImage(t.Context(), input, refs)
	if err != nil {
		t.Fatal(err)
	}
	edit := prepared.Preparation.LocalEdit
	if edit.BaseVariantUUID != first.VariantUUID || edit.OriginalSize != (ImageRatio{400, 300}) || prepared.ReferenceMode != "local_edit" {
		t.Fatal(prepared)
	}
	cropData, err := os.ReadFile(prepared.ReferenceImagePath)
	if err != nil {
		t.Fatal(err)
	}
	crop, err := png.Decode(bytes.NewReader(cropData))
	if err != nil || crop.Bounds().Dx() != edit.CropPixels.Width {
		t.Fatal(err)
	}
	_, base, err := h.service.localEditBase(t.Context(), first.Asset.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if color.NRGBAModel.Convert(crop.At(10, 10)) != color.NRGBAModel.Convert(base.At(edit.CropPixels.X+10, edit.CropPixels.Y+10)) {
		t.Fatal("crop is not original resolution")
	}
	patch := image.NewNRGBA(crop.Bounds())
	for y := 0; y < patch.Bounds().Dy(); y++ {
		for x := 0; x < patch.Bounds().Dx(); x++ {
			patch.SetNRGBA(x, y, color.NRGBA{180, 190, 200, 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, patch)
	patchPath := filepath.Join(t.TempDir(), "host-patch.png")
	_ = os.WriteFile(patchPath, buf.Bytes(), 0600)
	args := ImportHostImageInput{Path: patchPath, TargetUUID: page.UUID, Slot: "page_image", ExpectedRevision: current.Revision, Prompt: prepared.GenerationPrompt, Preparation: &prepared.Preparation}
	args.Preparation.LocalEdit.SelectionPixels.X++
	_, err = h.service.ImportHostImage(t.Context(), args)
	assertHostError(t, err, CodePageImagePreparationStale)
	args.Preparation.LocalEdit.SelectionPixels.X--
	bad := args
	bad.FitMode = "crop"
	_, err = h.service.ImportHostImage(t.Context(), bad)
	assertHostError(t, err, CodeValidation)
	wrongRatio, _ := hostFixture(t, 40, 300, false)
	bad = args
	bad.Path = wrongRatio
	_, err = h.service.ImportHostImage(t.Context(), bad)
	assertHostError(t, err, CodeValidation)
	candidate, err := h.service.ImportHostImage(t.Context(), args)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Adopted || candidate.VersionNo != 3 || candidate.Asset.MIMEType != "image/png" {
		t.Fatal(candidate)
	}
	pageNow, err := h.service.GetSection(t.Context(), chapter, page.UUID)
	if err != nil || pageNow.CurrentImage.UUID != current.VariantUUID {
		t.Fatal("historical edit replaced adopted image", pageNow, err)
	}
	_, out, err := h.service.localEditBase(t.Context(), candidate.Asset.UUID)
	if err != nil {
		t.Fatal(err)
	}
	outside, inside := 0, 0
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			if color.NRGBAModel.Convert(base.At(x, y)) != color.NRGBAModel.Convert(out.At(x, y)) {
				if image.Pt(x, y).In(edit.SelectionPixels.rect()) {
					inside++
				} else {
					outside++
				}
			}
		}
	}
	if outside != 0 || inside == 0 {
		t.Fatal("saved PNG pixel isolation", outside, inside)
	}
	variants, err := h.service.ListImageVariants(t.Context(), chapter, page.UUID)
	if err != nil || len(variants) != 3 {
		t.Fatal(variants, err)
	}
	selected, err := h.service.SelectImageVariant(t.Context(), chapter, page.UUID, candidate.VariantUUID, candidate.Revision)
	if err != nil || selected.CurrentImage.UUID != candidate.VariantUUID {
		t.Fatal("explicit adoption failed", err)
	}
	_, err = h.service.ImportHostImage(t.Context(), args)
	assertHostError(t, err, CodeConflict)
}

func TestLocalEditPreserves16BitAndTransparentPixelsOutsideSelection(t *testing.T) {
	base := image.NewNRGBA64(image.Rect(0, 0, 80, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			base.SetNRGBA64(x, y, color.NRGBA64{R: uint16(x * 731), G: uint16(y * 677), B: 34567, A: uint16((x + y) * 399)})
		}
	}
	patch := image.NewUniform(color.NRGBA{255, 100, 50, 200})
	// Use finite patch bounds for the resampler.
	finite := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			finite.Set(x, y, patch.At(x, y))
		}
	}
	edit := PageImageLocalEdit{SelectionPixels: ImagePixelRect{20, 20, 20, 20}, CropPixels: ImagePixelRect{10, 10, 40, 40}, FeatherPixels: 3}
	out := compositeLocalEdit(base, finite, edit)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, out); err != nil {
		t.Fatal(err)
	}
	saved, err := png.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			if !image.Pt(x, y).In(edit.SelectionPixels.rect()) && color.NRGBA64Model.Convert(saved.At(x, y)) != base.NRGBA64At(x, y) {
				t.Fatalf("outside 16-bit pixel changed at %d,%d", x, y)
			}
		}
	}
}
