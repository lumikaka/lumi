package production

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"

	"gorm.io/gorm"
	"lumi/internal/files"
)

// Host imports deliberately do not use picturebook.ResolveImageSize: a host
// controls composition and approximate aspect ratio, not exact provider pixels.
const HostImageMaxBytes = 32 << 20
const hostImageMaxPixels = 40_000_000
const HostImageRatioTolerance = 0.02

type ImageRatio struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type HostImageTarget struct {
	TargetUUID       string     `json:"target_uuid"`
	Slot             string     `json:"slot"`
	ExpectedRevision int64      `json:"expected_revision"`
	TargetRatio      ImageRatio `json:"target_ratio"`
	RecommendedRatio ImageRatio `json:"recommended_ratio"`
	GenerationHint   string     `json:"generation_hint"`
	SuggestedFit     string     `json:"suggested_fit"`
	RatioTolerance   float64    `json:"ratio_tolerance"`
	chapterUUID      string
}

func (service *Service) HostImageTarget(ctx context.Context, targetUUID, slot string) (HostImageTarget, error) {
	target := HostImageTarget{TargetUUID: targetUUID, Slot: slot, TargetRatio: ImageRatio{1, 1}, SuggestedFit: "pad", RatioTolerance: HostImageRatioTolerance}
	switch slot {
	case "page_image":
		var chapter struct{ UUID string }
		err := service.store.DB().WithContext(ctx).Table("chapters AS c").Select("c.uuid").
			Joins("JOIN chapter_comic_states AS st ON st.chapter_id=c.id").
			Joins("JOIN comic_sections AS s ON s.chapter_comic_state_id=st.id").
			Where("s.uuid=? AND s.deleted_at IS NULL AND c.deleted_at IS NULL AND c.project_id=(SELECT id FROM projects WHERE uuid=?)", targetUUID, service.store.ProjectUUID()).Take(&chapter).Error
		if err != nil {
			return target, notFound(err, "页面不存在")
		}
		page, err := service.GetSection(ctx, chapter.UUID, targetUUID)
		if err != nil {
			return target, err
		}
		profile, err := service.store.RequirePictureBookProfile()
		if err != nil {
			return target, err
		}
		target.chapterUUID, target.ExpectedRevision = chapter.UUID, page.Revision
		target.TargetRatio = ImageRatio{profile.AspectRatio.Width, profile.AspectRatio.Height}
		target.SuggestedFit = "crop"
	case "setting_image":
		setting, err := service.GetPremiseAsset(ctx, targetUUID)
		if err != nil {
			return target, err
		}
		target.ExpectedRevision = setting.Revision
		// Settings have no persisted canvas constraint. Use a square reference
		// sheet; scenes follow the book's framing as a composition suggestion.
		if setting.AssetType == "scene" {
			profile, err := service.store.RequirePictureBookProfile()
			if err != nil {
				return target, err
			}
			target.TargetRatio = ImageRatio{profile.AspectRatio.Width, profile.AspectRatio.Height}
		}
	default:
		return target, domainError(CodeValidation, "图片槽位无效", "slot 只支持 page_image / setting_image。", nil)
	}
	bestDistance := math.Inf(1)
	for _, ratio := range []ImageRatio{{3, 2}, {1, 1}, {2, 3}} {
		distance := math.Abs(math.Log(float64(ratio.Width*target.TargetRatio.Height) / float64(ratio.Height*target.TargetRatio.Width)))
		if distance < bestDistance {
			bestDistance, target.RecommendedRatio = distance, ratio
		}
	}
	r, wanted := target.RecommendedRatio, target.TargetRatio
	target.GenerationHint = fmt.Sprintf("请生成宽高比约 %d:%d 的图片；目标画布为 %d:%d。完整保留主体和画面文字，四周预留安全边距。宿主输出像素无需精确一致；导入可使用 %s 处理。", r.Width, r.Height, wanted.Width, wanted.Height, target.SuggestedFit)
	return target, nil
}

type ImportHostImageInput struct {
	Path, TargetUUID, Slot, FitMode, Description, Prompt string
	ExpectedRevision                                     int64
}

type HostImageImport struct {
	Target         HostImageTarget `json:"target"`
	Revision       int64           `json:"revision"`
	VariantUUID    string          `json:"variant_uuid"`
	VersionNo      int             `json:"version_no"`
	Asset          files.Asset     `json:"asset"`
	InputSize      ImageRatio      `json:"input_size"`
	FitMode        string          `json:"fit_mode"`
	RatioDeviation float64         `json:"ratio_deviation"`
	RatioMatches   bool            `json:"ratio_matches"`
	Warnings       []string        `json:"warnings"`
}

func hostImageError(code, message, details string) error {
	return &files.Error{Code: code, Message: message, Details: details}
}

// readHostImage snapshots only the explicitly supplied regular file, bounded
// before decoding. No host directory or filename convention is assumed.
func readHostImage(ctx context.Context, path string) ([]byte, image.Image, error) {
	if !filepath.IsAbs(path) {
		return nil, nil, hostImageError(files.CodeUnsafePath, "图片路径无效", "传入宿主实际返回的绝对路径，不要猜测目录或文件名。")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, hostImageError("image_path_unavailable", "无法读取图片文件", "确认宿主返回的文件仍存在且是可读的普通文件；生成后请立即导入。")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, hostImageError("image_path_unavailable", "无法打开图片", "检查文件读取权限；生成后请立即导入。")
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, hostImageError("image_path_unavailable", "图片文件无效", "必须是可读的普通文件。")
	}
	if info.Size() > HostImageMaxBytes {
		return nil, nil, hostImageError(files.CodeFileTooLarge, "图片超过大小限制", "宿主图片最多 32 MiB。")
	}
	data, err := io.ReadAll(io.LimitReader(file, HostImageMaxBytes+1))
	if err != nil {
		return nil, nil, hostImageError("image_path_unavailable", "读取图片失败", "检查文件后重新导入。")
	}
	if len(data) > HostImageMaxBytes {
		return nil, nil, hostImageError(files.CodeFileTooLarge, "图片超过大小限制", "宿主图片最多 32 MiB。")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	decoded, err := decodeHostImage(data)
	return data, decoded, err
}

func decodeHostImage(data []byte) (image.Image, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, hostImageError(files.CodeInvalidContent, "图片无法解码", "需要完整且有效的 PNG、JPEG 或 WebP 图片。")
	}
	if format != "png" && format != "jpeg" && format != "webp" {
		return nil, hostImageError(files.CodeTypeNotAllowed, "图片格式不支持", "只支持真实 PNG、JPEG、WebP 内容，扩展名不决定格式。")
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > hostImageMaxPixels {
		return nil, hostImageError(files.CodePixelsTooLarge, "图片像素超限", "图片总像素最多 4000 万。")
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return nil, hostImageError(files.CodeInvalidContent, "图片内容不完整", "完整解码失败，请重新生成或保存有效图片。")
	}
	return decoded, nil
}

func (service *Service) ImportHostImage(ctx context.Context, input ImportHostImageInput) (HostImageImport, error) {
	var result HostImageImport
	if err := service.store.RequireReady(); err != nil {
		return result, err
	}
	if err := service.store.RequireInitializationAvailable(ctx); err != nil {
		return result, err
	}
	if input.FitMode == "" {
		input.FitMode = "keep"
	}
	if input.FitMode != "keep" && input.FitMode != "crop" && input.FitMode != "pad" {
		return result, domainError(CodeValidation, "图片处理方式无效", "fit_mode 只支持 keep、crop、pad。", nil)
	}
	if len([]rune(input.Prompt)) > 12000 || len([]rune(input.Description)) > 12000 {
		return result, domainError(CodeValidation, "图片说明过长", "提示词和说明最多 12000 字符。", nil)
	}
	target, err := service.HostImageTarget(ctx, input.TargetUUID, input.Slot)
	if err != nil {
		return result, err
	}
	if target.ExpectedRevision != input.ExpectedRevision {
		return result, domainError(CodeConflict, "图片目标已被修改", "重新读取目标内容和 revision 后再决定是否导入。", nil)
	}
	data, decoded, err := readHostImage(ctx, input.Path)
	if err != nil {
		return result, err
	}
	w, h := decoded.Bounds().Dx(), decoded.Bounds().Dy()
	deviation := float64(w*target.TargetRatio.Height)/float64(h*target.TargetRatio.Width) - 1
	result = HostImageImport{Target: target, InputSize: ImageRatio{w, h}, FitMode: input.FitMode, RatioDeviation: deviation, RatioMatches: math.Abs(deviation) <= HostImageRatioTolerance, Warnings: []string{}}
	if !result.RatioMatches {
		result.Warnings = append(result.Warnings, fmt.Sprintf("输入图片 %d:%d 与目标 %d:%d 的比例偏差为 %+.2f%%；已按 %s 导入。%s", w, h, target.TargetRatio.Width, target.TargetRatio.Height, deviation*100, input.FitMode, target.GenerationHint))
	}
	purpose := "premise_asset"
	if input.Slot == "page_image" {
		purpose = "comic_section_image"
	}
	metadata := map[string]any{"origin": "host_generated", "original_prompt": input.Prompt, "description": input.Description, "fit_mode": input.FitMode, "input_width": w, "input_height": h, "target_width": target.TargetRatio.Width, "target_height": target.TargetRatio.Height}
	commitInput := files.CommitInput{Purpose: purpose, OriginalFilename: filepath.Base(input.Path), SourceType: "imported", Metadata: metadata, Reader: bytes.NewReader(data)}
	if input.FitMode != "keep" && !result.RatioMatches {
		processed, err := fitHostImage(decoded, target.TargetRatio, input.FitMode)
		if err != nil {
			return result, err
		}
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, processed); err != nil {
			return result, err
		}
		// Preserve the immutable original through a concrete source_file_id;
		// transformed artwork is a separate Asset, never an overwritten object.
		original, err := service.files.CommitReader(ctx, commitInput)
		if err != nil {
			return result, err
		}
		commitInput.SourceAssetUUID, commitInput.SourceType = original.UUID, "derived"
		commitInput.OriginalFilename = "host-" + input.FitMode + ".png"
		commitInput.Reader = bytes.NewReader(encoded.Bytes())
	}
	commit := func(bind func(*gorm.DB, int64) error) error {
		commitInput.Bind = bind
		_, err := service.files.CommitReader(ctx, commitInput)
		return err
	}
	if input.Slot == "page_image" {
		page, err := service.importSectionImage(ctx, target.chapterUUID, input.TargetUUID, input.ExpectedRevision, commit)
		if err != nil {
			return result, err
		}
		v := page.CurrentImage
		result.Revision, result.VariantUUID, result.VersionNo, result.Asset = page.Revision, v.UUID, v.VersionNo, v.Asset
	} else {
		setting, err := service.importPremiseAssetVariant(ctx, input.TargetUUID, nil, input.ExpectedRevision, commit)
		if err != nil {
			return result, err
		}
		v := setting.CurrentVariant
		result.Revision, result.VariantUUID, result.VersionNo, result.Asset = setting.Revision, v.UUID, v.VersionNo, v.Asset
	}
	result.Target.ExpectedRevision = result.Revision
	return result, nil
}

func fitHostImage(source image.Image, ratio ImageRatio, mode string) (image.Image, error) {
	bounds := source.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if mode == "crop" {
		if w*ratio.Height > h*ratio.Width {
			w = max(1, h*ratio.Width/ratio.Height)
		} else {
			h = max(1, w*ratio.Height/ratio.Width)
		}
	} else {
		if w*ratio.Height > h*ratio.Width {
			h = int(math.Ceil(float64(w*ratio.Height) / float64(ratio.Width)))
		} else {
			w = int(math.Ceil(float64(h*ratio.Width) / float64(ratio.Height)))
		}
	}
	if int64(w)*int64(h) > hostImageMaxPixels {
		return nil, hostImageError(files.CodePixelsTooLarge, "处理后图片像素超限", "请缩小原图，或使用 keep / crop 后导入。")
	}
	output := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(output, output.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	// Negative destination coordinates implement a centered crop; positive
	// coordinates center the original on deterministic white padding.
	dest := image.Rect((w-bounds.Dx())/2, (h-bounds.Dy())/2, (w-bounds.Dx())/2+bounds.Dx(), (h-bounds.Dy())/2+bounds.Dy())
	draw.Draw(output, dest, source, bounds.Min, draw.Over)
	return output, nil
}

func hostImageSourceType(asset files.Asset, fallback string) string {
	if metadata, ok := asset.Metadata.(map[string]any); ok && metadata["origin"] == "host_generated" {
		return "host_generated"
	}
	return fallback
}
