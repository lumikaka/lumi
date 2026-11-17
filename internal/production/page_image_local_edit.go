package production

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// NormalizedImageRect is relative to the entire page, never the context crop.
type NormalizedImageRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}
type ImagePixelRect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func pixelRect(r image.Rectangle) ImagePixelRect {
	return ImagePixelRect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}
}
func (r ImagePixelRect) rect() image.Rectangle {
	return image.Rect(r.X, r.Y, r.X+r.Width, r.Y+r.Height)
}

type PageImageLocalEdit struct {
	BaseVariantUUID   string              `json:"base_variant_uuid"`
	BaseVersionNo     int                 `json:"base_version_no"`
	BaseAssetUUID     string              `json:"base_asset_uuid"`
	BaseSHA256        string              `json:"base_sha256"`
	OriginalSize      ImageRatio          `json:"original_size"`
	Selection         NormalizedImageRect `json:"selection"`
	SelectionPixels   ImagePixelRect      `json:"selection_pixels"`
	CropPixels        ImagePixelRect      `json:"crop_pixels"`
	FeatherPixels     int                 `json:"feather_pixels"`
	CompositorVersion string              `json:"compositor_version"`
}

const localEditCompositorVersion = "page-local-edit-v1"

func localEditGeometry(r NormalizedImageRect, w, h int) (ImagePixelRect, ImagePixelRect, error) {
	for _, v := range []float64{r.X, r.Y, r.Width, r.Height} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return ImagePixelRect{}, ImagePixelRect{}, domainError(CodeValidation, "选区无效", "坐标必须为整页左上角起算的 0～1 有限数值。", nil)
		}
	}
	if r.Width <= 0 || r.Height <= 0 || r.X+r.Width > 1+1e-12 || r.Y+r.Height > 1+1e-12 {
		return ImagePixelRect{}, ImagePixelRect{}, domainError(CodeValidation, "选区无效", "选区必须有面积且完全位于整页内。", nil)
	}
	// Outward rounding keeps every pixel touched by a preview selection.
	snap := func(v float64) float64 {
		if math.Abs(v-math.Round(v)) < 1e-9 {
			return math.Round(v)
		}
		return v
	}
	s := image.Rect(int(math.Floor(snap(r.X*float64(w)))), int(math.Floor(snap(r.Y*float64(h)))), min(w, int(math.Ceil(snap((r.X+r.Width)*float64(w))))), min(h, int(math.Ceil(snap((r.Y+r.Height)*float64(h))))))
	if s.Empty() {
		return ImagePixelRect{}, ImagePixelRect{}, domainError(CodeValidation, "选区无效", "选区映射到原图后必须至少覆盖一个像素。", nil)
	}
	margin := max(16, int(math.Ceil(float64(max(s.Dx(), s.Dy()))*0.5)))
	c := s.Inset(-margin).Intersect(image.Rect(0, 0, w, h))
	return pixelRect(s), pixelRect(c), nil
}

func (service *Service) localEditBase(ctx context.Context, assetUUID string) ([]byte, image.Image, error) {
	content, err := service.files.OpenContent(ctx, assetUUID)
	if err != nil {
		return nil, nil, err
	}
	defer content.File.Close()
	data, err := io.ReadAll(io.LimitReader(content.File, HostImageMaxBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > HostImageMaxBytes {
		return nil, nil, hostImageError("file_too_large", "基准图过大", "局部改图基准最多 32 MiB。")
	}
	decoded, err := decodeHostImage(data)
	return data, decoded, err
}

func (service *Service) localPageImageInputs(ctx context.Context, input PreparePageImageInput) (PreparedPageImage, error) {
	result := PreparedPageImage{ReferenceMode: "local_edit", Sources: []PageImageSource{}}
	if !isUUIDv7(input.BaseVariantUUID) || input.Selection == nil || strings.TrimSpace(input.AdditionalDirection) == "" || len([]rune(input.AdditionalDirection)) > 10000 || len(input.SettingUUIDs) > 0 || len(input.SettingReviews) > 0 {
		return result, domainError(CodeValidation, "局部改图输入无效", "同时提供 base_variant_uuid、selection 和 additional_direction（最多 10000 字）；局部模式使用基准裁图，setting_uuids 省略或 []。", nil)
	}
	target, err := service.HostImageTarget(ctx, input.PageUUID, "page_image")
	if err != nil {
		return result, err
	}
	variants, err := service.ListImageVariants(ctx, target.chapterUUID, input.PageUUID)
	if err != nil {
		return result, err
	}
	var base *ImageVariant
	for i := range variants {
		if variants[i].UUID == input.BaseVariantUUID {
			base = &variants[i]
			break
		}
	}
	if base == nil {
		return result, domainError(CodeValidation, "基准图片版本无效", "base_variant_uuid 必须属于本页；从 pages get(include_versions=true) 按 version_no 取得。", nil)
	}
	data, img, err := service.localEditBase(ctx, base.Asset.UUID)
	if err != nil {
		return result, err
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	s, c, err := localEditGeometry(*input.Selection, w, h)
	if err != nil {
		return result, err
	}
	hash := sha256.Sum256(data)
	edit := &PageImageLocalEdit{BaseVariantUUID: base.UUID, BaseVersionNo: base.VersionNo, BaseAssetUUID: base.Asset.UUID, BaseSHA256: hex.EncodeToString(hash[:]), OriginalSize: ImageRatio{w, h}, Selection: *input.Selection, SelectionPixels: s, CropPixels: c, FeatherPixels: min(8, min(s.Width, s.Height)/8), CompositorVersion: localEditCompositorVersion}
	prompt := fmt.Sprintf("Use case: precise-object-edit\n输入图片是已有绘本第 %d 版的原尺寸上下文裁图（%d×%d），是唯一编辑目标。仅修改裁图内矩形 x=%d, y=%d, width=%d, height=%d（相对裁图左上角的像素）。\n用户修改要求：%s\n保持裁图的构图、位置、比例、人物姿态、皮肤、衣甲、线条和光照一致；选区外上下文保持原样。输出完整裁图，禁止缩放主体、移动、重新构图、加边框或裁边；输出同一宽高比。服务端将完整输出对齐到裁图尺寸，仅选区内贴回原图，边缘过渡也只在选区内。", base.VersionNo, c.Width, c.Height, s.X-c.X, s.Y-c.Y, s.Width, s.Height, strings.TrimSpace(input.AdditionalDirection))
	encoded, _ := json.Marshal(struct {
		Target HostImageTarget
		Edit   *PageImageLocalEdit
		Prompt string
	}{target, edit, prompt})
	result.Target, result.GenerationPrompt = target, prompt
	result.ImageWriteBack = PageImageWriteBack{input.PageUUID, "page_image", target.ExpectedRevision}
	result.Preparation = PageImagePreparationReceipt{ProjectUUID: service.store.ProjectUUID(), PageUUID: input.PageUUID, ExpectedRevision: target.ExpectedRevision, SettingUUIDs: []string{}, AdditionalDirection: input.AdditionalDirection, LocalEdit: edit, Digest: hashJSON(encoded)}
	return result, nil
}

func (service *Service) prepareLocalEditCrop(ctx context.Context, result *PreparedPageImage, references *PageReferenceFiles) error {
	edit := result.Preparation.LocalEdit
	_, base, err := service.localEditBase(ctx, edit.BaseAssetUUID)
	if err != nil {
		return err
	}
	c := edit.CropPixels
	crop := localEditCanvas(base, image.Rect(0, 0, c.Width, c.Height))
	draw.Draw(crop, crop.Bounds(), base, image.Pt(c.X, c.Y), draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, crop); err != nil {
		return err
	}
	result.ReferenceImagePath, result.ReferenceImageSHA256, err = references.save(service.store.ProjectUUID(), buf.Bytes())
	result.ImageInfo = &SectionPremiseImageInfo{Width: c.Width, Height: c.Height, MIMEType: "image/png", ComposerVersion: localEditCompositorVersion}
	return err
}

// Composite only inside selection. Context pixels never participate in writes.
// PNG output avoids JPEG re-encoding changes outside the selection.
func compositeLocalEdit(base, patch image.Image, edit PageImageLocalEdit) image.Image {
	b := base.Bounds()
	out := localEditCanvas(base, image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), base, b.Min, draw.Src)
	c, s := edit.CropPixels, edit.SelectionPixels
	aligned := image.NewNRGBA(image.Rect(0, 0, c.Width, c.Height))
	xdraw.CatmullRom.Scale(aligned, aligned.Bounds(), patch, patch.Bounds(), draw.Src, nil)
	for y := s.Y; y < s.Y+s.Height; y++ {
		for x := s.X; x < s.X+s.Width; x++ {
			src := aligned.NRGBAAt(x-c.X, y-c.Y)
			dst := color.NRGBA64Model.Convert(out.At(x, y)).(color.NRGBA64)
			edge := min(x-s.X, y-s.Y, s.X+s.Width-1-x, s.Y+s.Height-1-y)
			a := 1.0
			if edit.FeatherPixels > 0 {
				a = math.Min(1, float64(edge)/float64(edit.FeatherPixels))
			}
			if a == 0 {
				continue
			}
			if a == 1 {
				out.Set(x, y, src)
				continue
			}
			patchColor := color.NRGBA64Model.Convert(src).(color.NRGBA64)
			blend := func(a0, b0 uint16) uint16 {
				value := float64(a0)*(1-a) + float64(b0)*a
				if _, eightBit := out.(*image.NRGBA); eightBit {
					return uint16(math.Round(value/257)) * 257
				}
				return uint16(math.Round(value))
			}
			out.Set(x, y, color.NRGBA64{R: blend(dst.R, patchColor.R), G: blend(dst.G, patchColor.G), B: blend(dst.B, patchColor.B), A: blend(dst.A, patchColor.A)})
		}
	}
	return out
}

// Preserve 16-bit PNG samples outside the selection as well as 8-bit artwork.
func localEditCanvas(source image.Image, bounds image.Rectangle) draw.Image {
	switch source.(type) {
	case *image.NRGBA64, *image.RGBA64, *image.Gray16, *image.Alpha16:
		return image.NewNRGBA64(bounds)
	default:
		return image.NewNRGBA(bounds)
	}
}
