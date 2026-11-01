package production

import (
	"database/sql"
	"encoding/json"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/config"
	"lumi/internal/dbmigrate"
	"lumi/internal/project"
)

func hostBoardFixture(t *testing.T, h *productionHarness) (HostSettingBoard, ImportHostSettingBoardInput) {
	t.Helper()
	path, _ := hostFixture(t, 300, 200, false)
	profile, err := h.service.GetPremise(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	input := ImportHostSettingBoardInput{Path: path, SourceText: "Fox and lantern", Prompt: "Two independent subjects on white", IdempotencyKey: "board-1", ExpectedRevision: profile.Revision}
	board, err := h.service.ImportHostSettingBoard(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	return board, input
}
func hostCropFixture() []HostSettingCrop {
	return []HostSettingCrop{
		{Type: "character", Title: "Fox", Summary: "Red fox", Tags: []string{"character"}, Confidence: 0.98, CropBox: CropBox{X: 0, Y: 0, Width: 0.5, Height: 1}},
		{Type: "prop", Title: "Lantern", Summary: "Blue lantern", Tags: []string{"prop"}, Confidence: 0.99, CropBox: CropBox{X: 0.5, Y: 0, Width: 0.5, Height: 1}},
	}
}
func hostBreakdownInput(t *testing.T, h *productionHarness, board HostSettingBoard) ApplyHostSettingBreakdownInput {
	t.Helper()
	profile, err := h.service.GetPremise(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return ApplyHostSettingBreakdownInput{BoardUUID: board.UUID, IdempotencyKey: "split-1", ExpectedRevision: board.Revision, ExpectedPremiseRevision: profile.Revision, Assets: hostCropFixture()}
}

func TestHostSettingBoardReplayPixelsAndResume(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	board, importInput := hostBoardFixture(t, h)
	if board.LatestBreakdown != nil || board.Revision != 0 || *board.SettingImage.Asset.Width != 300 || board.Source.StyleSnapshot == "" {
		t.Fatalf("board=%+v", board)
	}
	if err := os.Remove(importInput.Path); err != nil {
		t.Fatal(err)
	}
	replay, err := h.service.ImportHostSettingBoard(t.Context(), importInput)
	if err != nil || replay.UUID != board.UUID {
		t.Fatalf("import replay: %+v %v", replay, err)
	}
	changed := importInput
	changed.SourceText = "Other"
	_, err = h.service.ImportHostSettingBoard(t.Context(), changed)
	assertHostError(t, err, CodeStateConflict)
	input := hostBreakdownInput(t, h, board)
	result, err := h.service.ApplyHostSettingBreakdown(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Revision != 1 {
		t.Fatalf("result=%+v", result)
	}
	for i, item := range result.Items {
		asset, err := h.service.GetPremiseAsset(t.Context(), item.SettingUUID)
		if err != nil {
			t.Fatal(err)
		}
		v := asset.CurrentVariant
		if v.UUID != item.VariantUUID || v.SourceSettingImageUUID != board.SettingImage.UUID || v.Asset.SourceAssetUUID != board.SettingImage.Asset.UUID || *v.Asset.Width != 150 || *v.Asset.Height != 200 {
			t.Fatalf("variant=%+v", v)
		}
		content, err := h.service.files.OpenContent(t.Context(), v.Asset.UUID)
		if err != nil {
			t.Fatal(err)
		}
		pixels, _, err := image.Decode(content.File)
		content.File.Close()
		if err != nil {
			t.Fatal(err)
		}
		if got := color.NRGBAModel.Convert(pixels.At(0, 0)).(color.NRGBA); got.R != uint8(150*i) {
			t.Fatalf("wrong crop pixels: %v", got)
		}
	}
	var filesBefore, filesAfter int64
	h.service.store.DB().Table("files").Count(&filesBefore)
	repeated, err := h.service.ApplyHostSettingBreakdown(t.Context(), input)
	if err != nil || repeated.UUID != result.UUID || repeated.Items[0].VariantUUID != result.Items[0].VariantUUID {
		t.Fatalf("replay=%+v err=%v", repeated, err)
	}
	h.service.store.DB().Table("files").Count(&filesAfter)
	if filesBefore != filesAfter {
		t.Fatal("replay created files")
	}
	// Closing/reopening proves that recovery does not depend on a live agent or file path.
	root := h.service.store.Root()
	if _, err := h.projects.CloseProject(t.Context(), h.project.UUID); err != nil {
		t.Fatal(err)
	}
	reopened, err := h.projects.OpenSelected(t.Context(), project.ExplicitExistingDirectory(root))
	if err != nil {
		t.Fatal(err)
	}
	err = h.projects.WithStore(t.Context(), reopened.UUID, func(store *project.Store) error { h.service = NewService(store, nil); return nil })
	if err != nil {
		t.Fatal(err)
	}
	saved, err := h.service.GetHostSettingBoard(t.Context(), board.UUID)
	if err != nil || saved.LatestBreakdown == nil || saved.LatestBreakdown.UUID != result.UUID || !saved.SettingImage.BreakdownCompleted {
		t.Fatalf("resume=%+v err=%v", saved, err)
	}
	wire, _ := json.Marshal(saved)
	if strings.Contains(string(wire), `"id":`) || strings.Contains(string(wire), importInput.Path) {
		t.Fatalf("internal data leaked: %s", wire)
	}
	input.Assets[0].Title = "Different"
	_, err = h.service.ApplyHostSettingBreakdown(t.Context(), input)
	assertHostError(t, err, CodeStateConflict)
}

func TestHostSettingBreakdownAtomicityAndExplicitUpdates(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	board, _ := hostBoardFixture(t, h)
	input := hostBreakdownInput(t, h, board)
	// Fail the second domain binding after the first one was inserted.
	db := h.service.store.DB()
	if err := db.Exec(`CREATE TRIGGER reject_lantern BEFORE INSERT ON premise_assets WHEN NEW.title='Lantern' BEGIN SELECT RAISE(ABORT,'fixture failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.ApplyHostSettingBreakdown(t.Context(), input); err == nil {
		t.Fatal("expected fixture failure")
	}
	assets, err := h.service.ListPremiseAssets(t.Context(), "", "active")
	if err != nil || len(assets) != 0 {
		t.Fatalf("partial settings escaped: %+v %v", assets, err)
	}
	saved, err := h.service.GetHostSettingBoard(t.Context(), board.UUID)
	if err != nil || saved.Revision != 0 || saved.LatestBreakdown != nil {
		t.Fatalf("partial batch=%+v err=%v", saved, err)
	}
	if err := db.Exec(`DROP TRIGGER reject_lantern`).Error; err != nil {
		t.Fatal(err)
	}
	result, err := h.service.ApplyHostSettingBreakdown(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	// A new operation cannot overwrite by title; it must name the exact target.
	input.IdempotencyKey = "update-1"
	input.ExpectedRevision = result.Revision
	input.ExpectedPremiseRevision = result.PremiseRevision
	_, err = h.service.ApplyHostSettingBreakdown(t.Context(), input)
	assertHostError(t, err, CodeConflict)
	for i := range input.Assets {
		input.Assets[i].SettingUUID = result.Items[i].SettingUUID
		rev := result.Items[i].Revision
		input.Assets[i].ExpectedRevision = &rev
	}
	input.Assets[0].Summary = "An updated fox"
	updated, err := h.service.ApplyHostSettingBreakdown(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range updated.Items {
		asset, err := h.service.GetPremiseAsset(t.Context(), item.SettingUUID)
		if err != nil {
			t.Fatal(err)
		}
		if item.SettingUUID != result.Items[i].SettingUUID || asset.Revision != 1 || asset.CurrentVariant.VersionNo != 2 {
			t.Fatalf("update=%+v", asset)
		}
	}
	input.IdempotencyKey = "stale-1"
	input.ExpectedRevision = updated.Revision
	input.ExpectedPremiseRevision = updated.PremiseRevision
	_, err = h.service.ApplyHostSettingBreakdown(t.Context(), input)
	assertHostError(t, err, CodeConflict)
}

func TestHostSettingBreakdownValidationDoesNotWrite(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	board, _ := hostBoardFixture(t, h)
	for _, kind := range []string{"overlap", "bounds", "tiny", "duplicate", "missing_revision", "confidence", "board_revision", "premise_revision"} {
		t.Run(kind, func(t *testing.T) {
			input := hostBreakdownInput(t, h, board)
			switch kind {
			case "overlap":
				input.Assets[1].CropBox.X = 0.4
			case "bounds":
				input.Assets[0].CropBox.X = -0.1
			case "tiny":
				input.Assets[0].CropBox.Width = 0.001
			case "duplicate":
				input.Assets[1].Title = "FOX"
			case "missing_revision":
				input.Assets[0].SettingUUID = board.UUID
			case "confidence":
				input.Assets[0].Confidence = 0.5
			case "board_revision":
				input.ExpectedRevision++
			case "premise_revision":
				input.ExpectedPremiseRevision++
			}
			var before, after int64
			h.service.store.DB().Table("files").Count(&before)
			if _, err := h.service.ApplyHostSettingBreakdown(t.Context(), input); err == nil {
				t.Fatal("accepted invalid input")
			}
			h.service.store.DB().Table("files").Count(&after)
			if before != after {
				t.Fatal("invalid input wrote files")
			}
		})
	}
}

func TestHostSettingBoardMigrationRefusesLossyDowngrade(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	board, _ := hostBoardFixture(t, h)
	result, err := h.service.ApplyHostSettingBreakdown(t.Context(), hostBreakdownInput(t, h, board))
	if err != nil {
		t.Fatal(err)
	}
	dsn := config.SQLiteDSN(filepath.Join(h.service.store.Root(), "project.sqlite"))
	if _, err := h.projects.CloseProject(t.Context(), h.project.UUID); err != nil {
		t.Fatal(err)
	}
	runner, err := dbmigrate.OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if err := runner.Down(1); err == nil {
		t.Fatal("lossy host history downgrade accepted")
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var output string
	if err := db.QueryRow("SELECT output_json FROM host_setting_breakdowns WHERE uuid=?", result.UUID).Scan(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, result.Items[0].VariantUUID) {
		t.Fatal("downgrade lost output")
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM pragma_foreign_key_check").Scan(&count); err != nil || count != 0 {
		t.Fatalf("foreign keys=%d err=%v", count, err)
	}
}
