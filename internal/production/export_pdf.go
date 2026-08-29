package production

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"strings"

	"lumi/internal/project"

	"github.com/signintech/gopdf"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const (
	exportPDFLegacyRendererVersion = 1
	exportPDFRendererVersion       = 2
	exportPDFMarginMM              = 12
	exportPDFGutterMM              = 6
	exportPDFPageWidth             = 595.0
	exportPDFPageHeight            = 842.0
	pointsPerMillimeter            = 72.0 / 25.4
	exportPDFRasterDPI             = 180.0
	exportPDFJPEGQuality           = 90
)

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
	if (snapshot.Version != 5 && snapshot.Version != exportSnapshotV6) || snapshot.Format != ExportFormatPDF || snapshot.PDFLayout == nil {
		return ExportPDFLayout{}, domainError(CodeSnapshotInvalid, "PDF 导出快照无效", "PDF 必须使用包含布局的 v5 或 v6 快照。", nil)
	}
	if snapshot.Version >= exportSnapshotV6 {
		for _, entry := range snapshot.Entries {
			switch entry.PageRole {
			case PageRoleFrontCover, PageRoleBody, PageRoleBackCover:
			default:
				return ExportPDFLayout{}, domainError(CodeSnapshotInvalid, "PDF 页面角色无效", "v6 PDF 的每个图片条目都必须冻结有效的 page_role。", nil)
			}
		}
	}
	layout := *snapshot.PDFLayout
	if layout.PageSize != ExportPDFPageSizeA4Portrait || layout.MarginMM != exportPDFMarginMM || layout.GutterMM != exportPDFGutterMM || (layout.RendererVersion != exportPDFLegacyRendererVersion && layout.RendererVersion != exportPDFRendererVersion) {
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
		for offset := 0; offset < len(entries); {
			if err := ctx.Err(); err != nil {
				return err
			}
			pdf.AddPage()
			pageSlots := slots
			if exportPDFEntryPageRole(entries[offset], snapshot.Version) != PageRoleBody {
				pageSlots = exportPDFSlots(ExportPDFLayout{Placement: ExportPDFOneUp, MarginMM: layout.MarginMM, GutterMM: layout.GutterMM})
			}
			consumed := 0
			for slotIndex := range pageSlots {
				entryIndex := offset + slotIndex
				if entryIndex >= len(entries) {
					break
				}
				// Covers always occupy a physical page by themselves. A cover at
				// the next position also stops a partially filled body page.
				if slotIndex > 0 && exportPDFEntryPageRole(entries[entryIndex], snapshot.Version) != PageRoleBody {
					break
				}
				if err := service.drawExportPDFEntry(ctx, pdf, entries[entryIndex], pageSlots[slotIndex], layout.RendererVersion); err != nil {
					return fmt.Errorf("render section %s: %w", entries[entryIndex].SectionUUID, err)
				}
				consumed++
				rendered++
				progress := 10 + rendered*70/len(snapshot.Entries)
				if progress > lastProgress {
					if err := notifyExportProgress(reportProgress, progress); err != nil {
						return err
					}
					lastProgress = progress
				}
			}
			offset += consumed
		}
	}
	return pdf.Write(output)
}

func exportPDFEntryPageRole(entry ExportEntry, snapshotVersion int) string {
	if snapshotVersion < exportSnapshotV6 || strings.TrimSpace(entry.PageRole) == "" {
		return PageRoleBody
	}
	return entry.PageRole
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

func (service *Service) drawExportPDFEntry(ctx context.Context, pdf *gopdf.GoPdf, entry ExportEntry, slot exportPDFRect, rendererVersion int) error {
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
	if rendererVersion >= exportPDFRendererVersion {
		decoded, err := decodeExportPDFImage(content.File, mimeType)
		if err != nil {
			return err
		}
		if decoded.Bounds().Dx() != width || decoded.Bounds().Dy() != height {
			return domainError(CodeSnapshotInvalid, "PDF 图片尺寸已变化", "图片内容尺寸与冻结尺寸不一致。", nil)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		compressed, err := encodeExportPDFJPEG(decoded, target)
		if err != nil {
			return err
		}
		holder, err := gopdf.ImageHolderByReader(bytes.NewReader(compressed))
		if err != nil {
			return err
		}
		return pdf.ImageByHolder(holder, target.X, target.Y, &gopdf.Rect{W: target.W, H: target.H})
	}

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

func decodeExportPDFImage(reader io.Reader, mimeType string) (image.Image, error) {
	switch mimeType {
	case "image/png":
		return png.Decode(reader)
	case "image/jpeg":
		return jpeg.Decode(reader)
	case "image/gif":
		return gif.Decode(reader)
	case "image/webp":
		return webp.Decode(reader)
	default:
		return nil, domainError(CodeSnapshotInvalid, "PDF 图片格式不支持", "只支持 PNG、JPEG、GIF 或 WebP。", nil)
	}
}

func encodeExportPDFJPEG(source image.Image, target exportPDFRect) ([]byte, error) {
	flattened := flattenExportPDFImage(source)
	maxWidth := max(1, int(math.Ceil(target.W*exportPDFRasterDPI/72)))
	maxHeight := max(1, int(math.Ceil(target.H*exportPDFRasterDPI/72)))
	width, height := flattened.Bounds().Dx(), flattened.Bounds().Dy()
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid PDF source image size: %dx%d", width, height)
	}
	scale := math.Min(1, math.Min(float64(maxWidth)/float64(width), float64(maxHeight)/float64(height)))
	prepared := flattened
	if scale < 1 {
		resizedWidth := max(1, int(math.Round(float64(width)*scale)))
		resizedHeight := max(1, int(math.Round(float64(height)*scale)))
		resized := image.NewRGBA(image.Rect(0, 0, resizedWidth, resizedHeight))
		xdraw.CatmullRom.Scale(resized, resized.Bounds(), flattened, flattened.Bounds(), draw.Src, nil)
		prepared = resized
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, prepared, &jpeg.Options{Quality: exportPDFJPEGQuality}); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}

func flattenExportPDFImage(source image.Image) image.Image {
	bounds := source.Bounds()
	target := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(target, target.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(target, target.Bounds(), source, bounds.Min, draw.Over)
	return target
}
