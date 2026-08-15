package production

import (
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"io"
	"math"
	"strings"

	"lumi/internal/project"

	"github.com/signintech/gopdf"
	"golang.org/x/image/webp"
)

const (
	exportPDFRendererVersion = 1
	exportPDFMarginMM        = 12
	exportPDFGutterMM        = 6
	exportPDFFontFamily      = "lumi-droid-sans-fallback"
	exportPDFPageWidth       = 595.0
	exportPDFPageHeight      = 842.0
	exportPDFTitleMaxSize    = 36.0
	exportPDFTitleMinSize    = 20.0
	exportPDFSubtitleSize    = 16.0
	pointsPerMillimeter      = 72.0 / 25.4
)

//go:embed assets/DroidSansFallbackFull.ttf
var exportPDFFont []byte

type exportPDFRect struct {
	X float64
	Y float64
	W float64
	H float64
}

func pdfLayoutForPictureBook(profile project.PictureBookProfile) ExportPDFLayout {
	placement := ExportPDFOneUp
	if profile.Format == project.PictureBookVertical {
		placement = ExportPDFTwoUpColumns
	} else if profile.AspectRatio.Width > profile.AspectRatio.Height {
		placement = ExportPDFTwoUpStacked
	}
	return ExportPDFLayout{
		PageSize: ExportPDFPageSizeA4Portrait, Placement: placement,
		MarginMM: exportPDFMarginMM, GutterMM: exportPDFGutterMM,
		RendererVersion: exportPDFRendererVersion,
	}
}

func validateExportPDFSnapshot(snapshot ExportSnapshot) (ExportPDFLayout, error) {
	if snapshot.Version != 5 || snapshot.Format != ExportFormatPDF || snapshot.Cover == nil || snapshot.PDFLayout == nil {
		return ExportPDFLayout{}, domainError(CodeSnapshotInvalid, "PDF 导出快照无效", "PDF 必须使用包含封面和布局的 v5 快照。", nil)
	}
	layout := *snapshot.PDFLayout
	if layout.PageSize != ExportPDFPageSizeA4Portrait || layout.MarginMM != exportPDFMarginMM || layout.GutterMM != exportPDFGutterMM || layout.RendererVersion != exportPDFRendererVersion {
		return ExportPDFLayout{}, domainError(CodeSnapshotInvalid, "PDF 导出布局无效", "冻结布局不受当前 PDF 渲染器支持。", nil)
	}
	switch layout.Placement {
	case ExportPDFTwoUpStacked, ExportPDFTwoUpColumns, ExportPDFOneUp:
		return layout, nil
	default:
		return ExportPDFLayout{}, domainError(CodeSnapshotInvalid, "PDF 导出布局无效", "placement 不受当前 PDF 渲染器支持。", nil)
	}
}

func exportPDFSlots(layout ExportPDFLayout) []exportPDFRect {
	margin := float64(layout.MarginMM) * pointsPerMillimeter
	gutter := float64(layout.GutterMM) * pointsPerMillimeter
	innerWidth := exportPDFPageWidth - 2*margin
	innerHeight := exportPDFPageHeight - 2*margin
	switch layout.Placement {
	case ExportPDFTwoUpStacked:
		height := (innerHeight - gutter) / 2
		return []exportPDFRect{{X: margin, Y: margin, W: innerWidth, H: height}, {X: margin, Y: margin + height + gutter, W: innerWidth, H: height}}
	case ExportPDFTwoUpColumns:
		width := (innerWidth - gutter) / 2
		return []exportPDFRect{{X: margin, Y: margin, W: width, H: innerHeight}, {X: margin + width + gutter, Y: margin, W: width, H: innerHeight}}
	default:
		return []exportPDFRect{{X: margin, Y: margin, W: innerWidth, H: innerHeight}}
	}
}

func exportPDFContainRect(slot exportPDFRect, imageWidth, imageHeight int) (exportPDFRect, error) {
	if slot.W <= 0 || slot.H <= 0 || imageWidth <= 0 || imageHeight <= 0 {
		return exportPDFRect{}, fmt.Errorf("invalid PDF image geometry: slot=%+v image=%dx%d", slot, imageWidth, imageHeight)
	}
	scale := math.Min(slot.W/float64(imageWidth), slot.H/float64(imageHeight))
	width := float64(imageWidth) * scale
	height := float64(imageHeight) * scale
	return exportPDFRect{X: slot.X + (slot.W-width)/2, Y: slot.Y + (slot.H-height)/2, W: width, H: height}, nil
}

func supportedExportPDFMIME(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func (service *Service) writePDF(ctx context.Context, output io.Writer, snapshot ExportSnapshot, reportProgress func(int) error) error {
	layout, err := validateExportPDFSnapshot(snapshot)
	if err != nil {
		return err
	}
	if len(snapshot.Entries) == 0 {
		return domainError(CodeExportEmpty, "没有可导出的图片", "PDF 快照没有可用图片。", nil)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	if err := pdf.AddTTFFontData(exportPDFFontFamily, exportPDFFont); err != nil {
		return fmt.Errorf("load PDF cover font: %w", err)
	}
	pdf.AddPage()
	if err := drawExportPDFCover(pdf, *snapshot.Cover); err != nil {
		return err
	}
	if err := notifyExportProgress(reportProgress, 10); err != nil {
		return err
	}

	groups, err := groupExportPDFEntries(snapshot)
	if err != nil {
		return err
	}
	slots := exportPDFSlots(layout)
	rendered := 0
	lastProgress := 10
	for _, entries := range groups {
		for offset := 0; offset < len(entries); offset += len(slots) {
			if err := ctx.Err(); err != nil {
				return err
			}
			pdf.AddPage()
			for slotIndex := range slots {
				entryIndex := offset + slotIndex
				if entryIndex >= len(entries) {
					break
				}
				if err := service.drawExportPDFEntry(ctx, pdf, entries[entryIndex], slots[slotIndex]); err != nil {
					return fmt.Errorf("render section %s: %w", entries[entryIndex].SectionUUID, err)
				}
				rendered++
				progress := 10 + rendered*70/len(snapshot.Entries)
				if progress > lastProgress {
					if err := notifyExportProgress(reportProgress, progress); err != nil {
						return err
					}
					lastProgress = progress
				}
			}
		}
	}
	return pdf.Write(output)
}

func groupExportPDFEntries(snapshot ExportSnapshot) ([][]ExportEntry, error) {
	if snapshot.Scope != "project" {
		return [][]ExportEntry{snapshot.Entries}, nil
	}
	groups := make([][]ExportEntry, 0, snapshot.ActiveChapterCount)
	currentChapter := ""
	for _, entry := range snapshot.Entries {
		if strings.TrimSpace(entry.ChapterUUID) == "" {
			return nil, domainError(CodeSnapshotInvalid, "PDF 章节边界缺失", "项目 PDF 的每个图片条目都必须冻结 chapter_uuid。", nil)
		}
		if entry.ChapterUUID != currentChapter {
			groups = append(groups, []ExportEntry{})
			currentChapter = entry.ChapterUUID
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], entry)
	}
	return groups, nil
}

func drawExportPDFCover(pdf *gopdf.GoPdf, cover ExportCover) error {
	margin := float64(exportPDFMarginMM) * pointsPerMillimeter
	width := exportPDFPageWidth - 2*margin
	height := exportPDFPageHeight - 2*margin
	pdf.SetFillColor(255, 255, 255)
	pdf.RectFromUpperLeftWithStyle(0, 0, exportPDFPageWidth, exportPDFPageHeight, "F")
	title := strings.TrimSpace(cover.ProjectName)
	if title == "" {
		title = "Untitled"
	}
	subtitle := strings.TrimSpace(cover.ChapterCode)
	if chapterTitle := strings.TrimSpace(cover.ChapterTitle); chapterTitle != "" {
		if subtitle != "" {
			subtitle += " · "
		}
		subtitle += chapterTitle
	}

	var subtitleLines []string
	if subtitle != "" {
		if err := pdf.SetFont(exportPDFFontFamily, "", exportPDFSubtitleSize); err != nil {
			return err
		}
		subtitleLines, _ = wrapExportPDFText(pdf, subtitle, width)
		if len(subtitleLines) > 4 {
			subtitleLines = truncateExportPDFLines(pdf, subtitleLines, 4, width)
		}
	}
	subtitleLineHeight := exportPDFSubtitleSize * 1.45
	subtitleHeight := float64(len(subtitleLines)) * subtitleLineHeight
	gap := 0.0
	if len(subtitleLines) > 0 {
		gap = 24
	}

	titleSize := exportPDFTitleMaxSize
	var titleLines []string
	for size := exportPDFTitleMaxSize; size >= exportPDFTitleMinSize; size-- {
		if err := pdf.SetFont(exportPDFFontFamily, "", size); err != nil {
			return err
		}
		titleLines, _ = wrapExportPDFText(pdf, title, width)
		titleSize = size
		if float64(len(titleLines))*size*1.35+gap+subtitleHeight <= height {
			break
		}
	}
	titleLineHeight := titleSize * 1.35
	maxTitleLines := int(math.Floor((height - gap - subtitleHeight) / titleLineHeight))
	if maxTitleLines < 1 {
		maxTitleLines = 1
	}
	if len(titleLines) > maxTitleLines {
		if err := pdf.SetFont(exportPDFFontFamily, "", titleSize); err != nil {
			return err
		}
		titleLines = truncateExportPDFLines(pdf, titleLines, maxTitleLines, width)
	}
	blockHeight := float64(len(titleLines))*titleLineHeight + gap + subtitleHeight
	y := (exportPDFPageHeight - blockHeight) / 2
	if err := pdf.SetFont(exportPDFFontFamily, "", titleSize); err != nil {
		return err
	}
	for _, line := range titleLines {
		pdf.SetXY(margin, y)
		if err := pdf.CellWithOption(&gopdf.Rect{W: width, H: titleLineHeight}, line, gopdf.CellOption{Align: gopdf.Center | gopdf.Middle}); err != nil {
			return err
		}
		y += titleLineHeight
	}
	if len(subtitleLines) == 0 {
		return nil
	}
	y += gap
	if err := pdf.SetFont(exportPDFFontFamily, "", exportPDFSubtitleSize); err != nil {
		return err
	}
	for _, line := range subtitleLines {
		pdf.SetXY(margin, y)
		if err := pdf.CellWithOption(&gopdf.Rect{W: width, H: subtitleLineHeight}, line, gopdf.CellOption{Align: gopdf.Center | gopdf.Middle}); err != nil {
			return err
		}
		y += subtitleLineHeight
	}
	return nil
}

func wrapExportPDFText(pdf *gopdf.GoPdf, value string, maxWidth float64) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return []string{""}, nil
	}
	lines := []string{}
	current := ""
	for _, char := range value {
		if char == '\n' {
			lines = append(lines, strings.TrimSpace(current))
			current = ""
			continue
		}
		candidate := current + string(char)
		width, err := pdf.MeasureTextWidth(candidate)
		if err != nil {
			return nil, err
		}
		if width > maxWidth && current != "" {
			lines = append(lines, strings.TrimSpace(current))
			current = strings.TrimLeft(string(char), " \t")
		} else {
			current = candidate
		}
	}
	if current != "" || len(lines) == 0 {
		lines = append(lines, strings.TrimSpace(current))
	}
	return lines, nil
}

func truncateExportPDFLines(pdf *gopdf.GoPdf, lines []string, limit int, maxWidth float64) []string {
	if len(lines) <= limit || limit <= 0 {
		return lines
	}
	result := append([]string(nil), lines[:limit]...)
	last := strings.TrimSpace(result[len(result)-1])
	for last != "" {
		width, err := pdf.MeasureTextWidth(last + "…")
		if err == nil && width <= maxWidth {
			break
		}
		runes := []rune(last)
		last = strings.TrimSpace(string(runes[:len(runes)-1]))
	}
	result[len(result)-1] = last + "…"
	return result
}

func (service *Service) drawExportPDFEntry(ctx context.Context, pdf *gopdf.GoPdf, entry ExportEntry, slot exportPDFRect) error {
	content, err := service.files.OpenContent(ctx, entry.ImageAssetUUID)
	if err != nil {
		return err
	}
	defer content.File.Close()
	mimeType := strings.ToLower(strings.TrimSpace(entry.MIMEType))
	if !supportedExportPDFMIME(mimeType) {
		return domainError(CodeSnapshotInvalid, "PDF 图片格式不支持", "冻结 MIME 必须是 PNG、JPEG、GIF 或 WebP。", nil)
	}
	if strings.ToLower(strings.TrimSpace(content.Asset.MIMEType)) != mimeType {
		return domainError(CodeSnapshotInvalid, "PDF 图片类型已变化", "冻结 MIME 与当前 Asset 不一致。", nil)
	}
	width, height := entry.Width, entry.Height
	if width <= 0 || height <= 0 {
		return domainError(CodeSnapshotInvalid, "PDF 图片尺寸缺失", "冻结图片必须具有有效尺寸。", nil)
	}
	if content.Asset.Width == nil || content.Asset.Height == nil || width != *content.Asset.Width || height != *content.Asset.Height {
		return domainError(CodeSnapshotInvalid, "PDF 图片尺寸已变化", "冻结尺寸与当前 Asset 不一致。", nil)
	}
	target, err := exportPDFContainRect(slot, width, height)
	if err != nil {
		return err
	}
	pdf.SetFillColor(255, 255, 255)
	pdf.RectFromUpperLeftWithStyle(slot.X, slot.Y, slot.W, slot.H, "F")

	switch mimeType {
	case "image/png", "image/jpeg":
		holder, err := gopdf.ImageHolderByReader(content.File)
		if err != nil {
			return err
		}
		return pdf.ImageByHolder(holder, target.X, target.Y, &gopdf.Rect{W: target.W, H: target.H})
	case "image/gif":
		decoded, err := gif.Decode(content.File)
		if err != nil {
			return err
		}
		return pdf.ImageFrom(flattenExportPDFImage(decoded), target.X, target.Y, &gopdf.Rect{W: target.W, H: target.H})
	case "image/webp":
		decoded, err := webp.Decode(content.File)
		if err != nil {
			return err
		}
		return pdf.ImageFrom(flattenExportPDFImage(decoded), target.X, target.Y, &gopdf.Rect{W: target.W, H: target.H})
	default:
		return domainError(CodeSnapshotInvalid, "PDF 图片格式不支持", "只支持 PNG、JPEG、GIF 或 WebP。", nil)
	}
}

func flattenExportPDFImage(source image.Image) image.Image {
	bounds := source.Bounds()
	target := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(target, target.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(target, target.Bounds(), source, bounds.Min, draw.Over)
	return target
}
