package jobqueue

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"strings"
	"unicode"

	"lumi/internal/platformpath"
	"lumi/internal/production"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	_ "golang.org/x/image/webp"
)

const (
	maxSectionPremiseAssets       = 12
	sectionPremiseTileWidth       = 360
	sectionPremiseTileHeight      = 300
	sectionPremiseLabelHeight     = 72
	sectionPremiseGap             = 24
	sectionPremiseMaxColumns      = 3
	sectionPremiseComposerVersion = "v1"
)

var sectionPremiseFontPaths = platformpath.CJKFontPaths()

type sectionPremiseSource struct {
	Reference production.PremiseAssetReference
	Image     image.Image
}

type sectionPremiseComposition struct {
	Bytes  []byte
	Width  int
	Height int
}

func (runtime *projectRuntime) prepareSectionPremise(ctx context.Context, service *production.Service, record productionTaskRecord, generationUUID, sectionUUID string, selection sectionReferenceSelection) (*production.SectionPremise, []byte, error) {
	if len(selection.References) == 0 {
		return nil, nil, nil
	}
	if len(selection.References) > maxSectionPremiseAssets {
		return nil, nil, productionError("too_many_section_premise_assets", fmt.Sprintf("Section 设定参考图最多允许 %d 个", maxSectionPremiseAssets), false)
	}

	existing, err := service.GenerationSectionPremise(ctx, generationUUID)
	if err != nil {
		return nil, nil, err
	}
	canRepairExisting := sectionPremiseMatches(existing, selection)
	if canRepairExisting {
		if data, readErr := readSectionPremiseAsset(ctx, service, existing); readErr == nil {
			if eventErr := runtime.ensureSectionPremiseComposedEvent(ctx, record.ID, sectionUUID, *existing); eventErr != nil {
				return nil, nil, eventErr
			}
			return existing, data, nil
		}
	}

	sources := make([]sectionPremiseSource, 0, len(selection.References))
	for index, reference := range selection.References {
		if strings.TrimSpace(reference.FileUUID) == "" {
			return nil, nil, productionError("section_premise_source_unreadable", fmt.Sprintf("Section 设定源图 %d「%s」缺少公开 file UUID", index+1, reference.Title), false)
		}
		content, openErr := service.Files().OpenContent(ctx, reference.FileUUID)
		if openErr != nil {
			return nil, nil, productionError("section_premise_source_unreadable", fmt.Sprintf("无法读取 Section 设定源图 %d「%s」：%v", index+1, reference.Title, openErr), false)
		}
		data, readErr := io.ReadAll(io.LimitReader(content.File, 64<<20+1))
		closeErr := content.File.Close()
		if readErr != nil || closeErr != nil || len(data) == 0 || len(data) > 64<<20 {
			cause := readErr
			if cause == nil {
				cause = closeErr
			}
			return nil, nil, productionError("section_premise_source_unreadable", fmt.Sprintf("Section 设定源图 %d「%s」为空、过大或无法读取：%v", index+1, reference.Title, cause), false)
		}
		decoded, _, decodeErr := image.Decode(bytes.NewReader(data))
		if decodeErr != nil || decoded.Bounds().Dx() <= 0 || decoded.Bounds().Dy() <= 0 {
			return nil, nil, productionError("invalid_section_premise_source", fmt.Sprintf("Section 设定源图 %d「%s」不是有效图片：%v", index+1, reference.Title, decodeErr), false)
		}
		sources = append(sources, sectionPremiseSource{Reference: reference, Image: decoded})
	}

	composition, composeErr := composeSectionPremise(sources)
	if composeErr != nil {
		code := "section_premise_composition_failed"
		if strings.Contains(composeErr.Error(), "CJK font") {
			code = "section_premise_font_unavailable"
		}
		return nil, nil, productionError(code, fmt.Sprintf("无法生成 Section 设定参考合图：%v", composeErr), false)
	}
	titles := make([]string, len(selection.References))
	for index, reference := range selection.References {
		titles[index] = reference.Title
	}
	metadata := production.SectionPremiseMetadata{
		SelectedAssets:  append([]production.PremiseAssetReference(nil), selection.References...),
		SelectedTitles:  titles,
		SelectionReason: strings.TrimSpace(selection.Reason),
		ImageInfo: production.SectionPremiseImageInfo{
			Width: composition.Width, Height: composition.Height, MIMEType: "image/png", ComposerVersion: sectionPremiseComposerVersion,
		},
	}
	if canRepairExisting {
		repaired, repairErr := service.Files().RepairContent(ctx, existing.Asset.UUID, bytes.NewReader(composition.Bytes))
		if repairErr != nil {
			return nil, nil, repairErr
		}
		existing.Asset = repaired
		if eventErr := runtime.ensureSectionPremiseComposedEvent(ctx, record.ID, sectionUUID, *existing); eventErr != nil {
			return nil, nil, eventErr
		}
		return existing, composition.Bytes, nil
	}
	premise, commitErr := service.CommitGeneratedSectionPremise(ctx, generationUUID, sectionUUID, metadata, bytes.NewReader(composition.Bytes))
	if commitErr != nil {
		return nil, nil, commitErr
	}
	if eventErr := runtime.ensureSectionPremiseComposedEvent(ctx, record.ID, sectionUUID, premise); eventErr != nil {
		return nil, nil, eventErr
	}
	return &premise, composition.Bytes, nil
}

func sectionPremiseMatches(premise *production.SectionPremise, selection sectionReferenceSelection) bool {
	if premise == nil || premise.ImageInfo.ComposerVersion != sectionPremiseComposerVersion || premise.ImageInfo.MIMEType != "image/png" || premise.ImageInfo.Width <= 0 || premise.ImageInfo.Height <= 0 {
		return false
	}
	if len(premise.SelectedAssets) != len(selection.References) || premise.SelectionReason != strings.TrimSpace(selection.Reason) {
		return false
	}
	for index, expected := range selection.References {
		actual := premise.SelectedAssets[index]
		if actual.AssetUUID != expected.AssetUUID || actual.VariantUUID != expected.VariantUUID || actual.FileUUID != expected.FileUUID || actual.Title != expected.Title {
			return false
		}
	}
	return true
}

func readSectionPremiseAsset(ctx context.Context, service *production.Service, premise *production.SectionPremise) ([]byte, error) {
	if premise == nil || premise.Asset.UUID == "" || premise.Asset.MIMEType != "image/png" {
		return nil, errors.New("persisted section premise asset is not a PNG")
	}
	content, err := service.Files().OpenContent(ctx, premise.Asset.UUID)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(content.File, 64<<20+1))
	closeErr := content.File.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) == 0 || len(data) > 64<<20 {
		return nil, errors.New("persisted section premise PNG is empty or too large")
	}
	decoded, decodeErr := png.Decode(bytes.NewReader(data))
	if decodeErr != nil {
		return nil, decodeErr
	}
	if decoded.Bounds().Dx() != premise.ImageInfo.Width || decoded.Bounds().Dy() != premise.ImageInfo.Height {
		return nil, errors.New("persisted section premise PNG dimensions do not match metadata")
	}
	return data, nil
}

func (runtime *projectRuntime) ensureSectionPremiseComposedEvent(ctx context.Context, taskID int64, sectionUUID string, premise production.SectionPremise) error {
	var exists bool
	err := runtime.sqlDB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM production_task_events WHERE production_task_run_id=? AND event_type='section_premise_composed' AND json_extract(payload,'$.file_uuid')=?)`, taskID, premise.Asset.UUID).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return runtime.appendProductionEvent(ctx, taskID, "section_premise_composed", map[string]any{
		"file_uuid": premise.Asset.UUID, "section_uuid": sectionUUID, "selected_titles": premise.SelectedTitles,
		"width": premise.ImageInfo.Width, "height": premise.ImageInfo.Height,
	})
}

func composeSectionPremise(sources []sectionPremiseSource) (sectionPremiseComposition, error) {
	if len(sources) == 0 {
		return sectionPremiseComposition{}, nil
	}
	if len(sources) > maxSectionPremiseAssets {
		return sectionPremiseComposition{}, fmt.Errorf("section premise has %d assets; maximum is %d", len(sources), maxSectionPremiseAssets)
	}
	labels := make([]string, len(sources))
	for index, source := range sources {
		if source.Image == nil || source.Image.Bounds().Dx() <= 0 || source.Image.Bounds().Dy() <= 0 {
			return sectionPremiseComposition{}, fmt.Errorf("section premise source %d is not a valid image", index+1)
		}
		title := strings.TrimSpace(source.Reference.Title)
		if title == "" {
			title = "Untitled"
		}
		labels[index] = fmt.Sprintf("%d. %s", index+1, title)
	}
	face, err := loadSectionPremiseFont(labels, sectionPremiseFontPaths)
	if err != nil {
		return sectionPremiseComposition{}, err
	}
	defer face.Close()
	return composeSectionPremiseWithFace(sources, labels, face)
}

func composeSectionPremiseWithFace(sources []sectionPremiseSource, labels []string, face font.Face) (sectionPremiseComposition, error) {
	columns := min(sectionPremiseMaxColumns, len(sources))
	rows := int(math.Ceil(float64(len(sources)) / float64(columns)))
	width := columns*sectionPremiseTileWidth + (columns+1)*sectionPremiseGap
	height := rows*(sectionPremiseTileHeight+sectionPremiseLabelHeight) + (rows+1)*sectionPremiseGap
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	for index, source := range sources {
		column := index % columns
		row := index / columns
		x := sectionPremiseGap + column*(sectionPremiseTileWidth+sectionPremiseGap)
		y := sectionPremiseGap + row*(sectionPremiseTileHeight+sectionPremiseLabelHeight+sectionPremiseGap)
		tile := image.Rect(x, y, x+sectionPremiseTileWidth, y+sectionPremiseTileHeight)
		drawSectionPremiseBorder(canvas, tile, color.RGBA{R: 218, G: 222, B: 228, A: 255})

		sourceBounds := source.Image.Bounds()
		availableWidth := sectionPremiseTileWidth - 24
		availableHeight := sectionPremiseTileHeight - 24
		scale := math.Min(float64(availableWidth)/float64(sourceBounds.Dx()), float64(availableHeight)/float64(sourceBounds.Dy()))
		scaledWidth := max(1, int(math.Round(float64(sourceBounds.Dx())*scale)))
		scaledHeight := max(1, int(math.Round(float64(sourceBounds.Dy())*scale)))
		destination := image.Rect(
			x+(sectionPremiseTileWidth-scaledWidth)/2,
			y+(sectionPremiseTileHeight-scaledHeight)/2,
			x+(sectionPremiseTileWidth-scaledWidth)/2+scaledWidth,
			y+(sectionPremiseTileHeight-scaledHeight)/2+scaledHeight,
		)
		draw.CatmullRom.Scale(canvas, destination, source.Image, sourceBounds, draw.Over, nil)

		lines := wrapSectionPremiseLabel(labels[index], face, sectionPremiseTileWidth-16, 2)
		drawer := font.Drawer{Dst: canvas, Src: image.NewUniform(color.RGBA{R: 31, G: 35, B: 41, A: 255}), Face: face}
		baseline := y + sectionPremiseTileHeight + 27
		lineHeight := max(22, face.Metrics().Height.Ceil()+4)
		for lineIndex, line := range lines {
			drawer.Dot = fixed.P(x+8, baseline+lineIndex*lineHeight)
			drawer.DrawString(line)
		}
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		return sectionPremiseComposition{}, fmt.Errorf("encode section premise PNG: %w", err)
	}
	return sectionPremiseComposition{Bytes: encoded.Bytes(), Width: width, Height: height}, nil
}

func drawSectionPremiseBorder(destination *image.RGBA, rectangle image.Rectangle, border color.Color) {
	draw.Draw(destination, image.Rect(rectangle.Min.X, rectangle.Min.Y, rectangle.Max.X, rectangle.Min.Y+2), image.NewUniform(border), image.Point{}, draw.Src)
	draw.Draw(destination, image.Rect(rectangle.Min.X, rectangle.Max.Y-2, rectangle.Max.X, rectangle.Max.Y), image.NewUniform(border), image.Point{}, draw.Src)
	draw.Draw(destination, image.Rect(rectangle.Min.X, rectangle.Min.Y, rectangle.Min.X+2, rectangle.Max.Y), image.NewUniform(border), image.Point{}, draw.Src)
	draw.Draw(destination, image.Rect(rectangle.Max.X-2, rectangle.Min.Y, rectangle.Max.X, rectangle.Max.Y), image.NewUniform(border), image.Point{}, draw.Src)
}

func wrapSectionPremiseLabel(value string, face font.Face, maxWidth, maxLines int) []string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) == 0 {
		return []string{"Untitled"}
	}
	lines := make([]string, 0, maxLines)
	for len(runes) > 0 && len(lines) < maxLines {
		end := 1
		lastBreak := 0
		for end <= len(runes) {
			candidate := strings.TrimSpace(string(runes[:end]))
			if font.MeasureString(face, candidate).Ceil() > maxWidth {
				break
			}
			if unicode.IsSpace(runes[end-1]) {
				lastBreak = end
			}
			end++
		}
		if end > len(runes) {
			lines = append(lines, strings.TrimSpace(string(runes)))
			runes = nil
			break
		}
		cut := max(1, end-1)
		if lastBreak > 0 {
			cut = lastBreak
		}
		lines = append(lines, strings.TrimSpace(string(runes[:cut])))
		runes = []rune(strings.TrimSpace(string(runes[cut:])))
	}
	if len(runes) > 0 && len(lines) > 0 {
		last := []rune(lines[len(lines)-1])
		for len(last) > 0 && font.MeasureString(face, string(last)+"…").Ceil() > maxWidth {
			last = last[:len(last)-1]
		}
		lines[len(lines)-1] = strings.TrimSpace(string(last)) + "…"
	}
	return lines
}

func loadSectionPremiseFont(labels, paths []string) (font.Face, error) {
	required := make(map[rune]struct{})
	requiresCJK := false
	for _, label := range labels {
		for _, current := range label {
			if unicode.IsSpace(current) {
				continue
			}
			required[current] = struct{}{}
			if current > unicode.MaxASCII {
				requiresCJK = true
			}
		}
	}
	if !requiresCJK {
		return basicfont.Face7x13, nil
	}
	var fontErrors []error
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				fontErrors = append(fontErrors, fmt.Errorf("%s: %w", path, err))
			}
			continue
		}
		collection, err := opentype.ParseCollectionReaderAt(file)
		if err != nil {
			_ = file.Close()
			fontErrors = append(fontErrors, fmt.Errorf("%s: %w", path, err))
			continue
		}
		for index := 0; index < collection.NumFonts(); index++ {
			parsed, parseErr := collection.Font(index)
			if parseErr != nil {
				fontErrors = append(fontErrors, fmt.Errorf("%s font %d: %w", path, index, parseErr))
				continue
			}
			face, faceErr := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 18, DPI: 72, Hinting: font.HintingFull})
			if faceErr != nil {
				fontErrors = append(fontErrors, fmt.Errorf("%s font %d: %w", path, index, faceErr))
				continue
			}
			if sectionPremiseFontSupports(face, required) {
				return &sectionPremiseFontFace{Face: face, file: file}, nil
			}
			_ = face.Close()
		}
		_ = file.Close()
	}
	message := "no system CJK font covers all section premise labels"
	if len(fontErrors) > 0 {
		return nil, fmt.Errorf("%s: %w", message, errors.Join(fontErrors...))
	}
	return nil, errors.New(message)
}

func sectionPremiseFontSupports(face font.Face, required map[rune]struct{}) bool {
	for current := range required {
		if _, ok := face.GlyphAdvance(current); !ok {
			return false
		}
	}
	return true
}

type sectionPremiseFontFace struct {
	font.Face
	file *os.File
}

func (face *sectionPremiseFontFace) Close() error {
	return errors.Join(face.Face.Close(), face.file.Close())
}
