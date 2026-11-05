package production

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"lumi/internal/project"

	"gorm.io/gorm"
)

// A receipt is stateless and process-bound. Restarting Lumi requires preparing
// again. The signature binds page facts independently of the reusable reference PNG.
var pagePreparationKey = sync.OnceValue(func() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	return key
})

type PreparePageImageInput struct {
	PageUUID            string
	SettingUUIDs        []string
	AdditionalDirection string
	SettingReviews      []HostSettingReview
}

type PageImagePreparationReceipt struct {
	ProjectUUID         string              `json:"project_uuid"`
	PageUUID            string              `json:"page_uuid"`
	ExpectedRevision    int64               `json:"expected_revision"`
	SettingUUIDs        []string            `json:"setting_uuids"`
	AdditionalDirection string              `json:"additional_direction"`
	SettingReviews      []HostSettingReview `json:"setting_reviews,omitempty"`
	Digest              string              `json:"digest"`
	Signature           string              `json:"signature"`
}

type PageImageSource struct {
	Order       int    `json:"order"`
	Label       string `json:"label"`
	SettingUUID string `json:"setting_uuid"`
	Title       string `json:"title"`
	AssetType   string `json:"asset_type"`
	Revision    int64  `json:"revision"`
	VariantUUID string `json:"variant_uuid"`
	VersionNo   int    `json:"version_no"`
	FileUUID    string `json:"file_uuid"`
	SHA256      string `json:"sha256"`
}

type PageImageWriteBack struct {
	TargetUUID       string `json:"target_uuid"`
	Slot             string `json:"slot"`
	ExpectedRevision int64  `json:"expected_revision"`
}

type PreparedPageImage struct {
	Target               HostImageTarget             `json:"target"`
	ReferenceMode        string                      `json:"reference_mode"`
	ReferenceImagePath   string                      `json:"reference_image_path"`
	ReferenceImageSHA256 string                      `json:"reference_image_sha256,omitempty"`
	Sources              []PageImageSource           `json:"sources"`
	ImageInfo            *SectionPremiseImageInfo    `json:"image_info"`
	GenerationPrompt     string                      `json:"generation_prompt"`
	ImageWriteBack       PageImageWriteBack          `json:"image_write_back"`
	Preparation          PageImagePreparationReceipt `json:"preparation"`
}

func (receipt PageImagePreparationReceipt) signature() string {
	receipt.Signature = ""
	encoded, _ := json.Marshal(receipt)
	mac := hmac.New(sha256.New, pagePreparationKey())
	_, _ = mac.Write(encoded)
	return hex.EncodeToString(mac.Sum(nil))
}

func preparationConflict() error {
	return domainError(CodePageImagePreparationStale, "页面生图准备已失效", "页面、所选设定图片或提示词输入发生变化，或准备凭据无效。重新 prepare_page_image，按返回的合图核对后生图；不能仅替换 revision 或摘要导入。", nil)
}

// pageImageInputs uses a single SQLite view both while preparing and during the
// final import transaction. It does not write files or call any provider.
func (service *Service) pageImageInputs(ctx context.Context, input PreparePageImageInput) (PreparedPageImage, []SectionPremiseSource, error) {
	result := PreparedPageImage{ReferenceMode: "none", Sources: []PageImageSource{}}
	if input.SettingUUIDs == nil || len(input.SettingUUIDs) > MaxSectionPremiseAssets || len([]rune(input.AdditionalDirection)) > 12000 {
		return result, nil, domainError(CodeValidation, "页面参考选择无效", "明确提供 setting_uuids（无参考图时为 []），最多 12 项；additional_direction 最多 12000 字符。", nil)
	}
	target, err := service.HostImageTarget(ctx, input.PageUUID, "page_image")
	if err != nil {
		return result, nil, err
	}
	page, err := service.GetSection(ctx, target.chapterUUID, input.PageUUID)
	if err != nil {
		return result, nil, err
	}
	if page.CurrentStoryboard == nil || strings.TrimSpace(page.CurrentStoryboard.ContentMD) == "" {
		return result, nil, domainError(CodeValidation, "页面尚无分镜", "先保存完整 storyboard_md，再准备页面生图。", nil)
	}
	rules, err := service.sectionImageRules(ctx, page.PageRole, true)
	if err != nil {
		return result, nil, err
	}
	rules.ResourceUUID, rules.ChapterUUID, rules.StoryboardUUID, rules.StoryboardMD, rules.PageRole = page.UUID, page.ChapterUUID, page.CurrentStoryboard.UUID, page.CurrentStoryboard.ContentMD, page.PageRole
	rules.Prompt = strings.TrimSpace(input.AdditionalDirection)
	selected := make([]PremiseAsset, 0, len(input.SettingUUIDs))
	references := make([]PremiseAssetReference, 0, len(input.SettingUUIDs))
	seen := map[string]bool{}
	for _, uuid := range input.SettingUUIDs {
		if !isUUIDv7(uuid) || seen[uuid] {
			return result, nil, domainError(CodeValidation, "设定选择无效", "setting_uuids 必须为当前项目不重复的 active 设定 UUIDv7。", nil)
		}
		seen[uuid] = true
		setting, err := service.GetPremiseAsset(ctx, uuid)
		if err != nil || setting.DeletedAt != nil {
			return result, nil, domainError(CodeValidation, "设定引用无效", "所选设定不属于当前项目、已删除或不存在。", err)
		}
		if setting.CurrentVariant == nil || setting.CurrentVariant.Asset.Kind != "image" {
			return result, nil, domainError("setting_image_missing", "所选设定缺少当前图片", fmt.Sprintf("先补齐「%s」(%s) 的设定图，再 prepare_page_image。", setting.Title, uuid), nil)
		}
		selected = append(selected, setting)
		references = append(references, PremiseAssetReference{AssetUUID: uuid, VariantUUID: setting.CurrentVariant.UUID, FileUUID: setting.CurrentVariant.Asset.UUID, Title: setting.Title, AssetType: setting.AssetType, Summary: setting.Summary})
	}
	sources, hashes, err := service.ReadSectionPremiseSources(ctx, references)
	if err != nil {
		return result, nil, err
	}
	for i, setting := range selected {
		v := setting.CurrentVariant
		result.Sources = append(result.Sources, PageImageSource{Order: i + 1, Label: fmt.Sprintf("%d. %s", i+1, strings.TrimSpace(setting.Title)), SettingUUID: setting.UUID, Title: setting.Title, AssetType: setting.AssetType, Revision: setting.Revision, VariantUUID: v.UUID, VersionNo: v.VersionNo, FileUUID: v.Asset.UUID, SHA256: hashes[i]})
	}
	if err := validateHostSettingReviews(selected, rules.StyleSnapshot, input.SettingReviews); err != nil {
		return result, nil, err
	}
	prompt, err := RenderSectionImagePrompt(rules, references)
	if err != nil {
		return result, nil, domainError(CodeValidation, "页面生图提示词无法组装", err.Error(), err)
	}
	if len([]rune(prompt)) > 12000 {
		return result, nil, domainError("prompt_too_large", "页面生图提示词过长", "完整提示词最多 12000 字符；先调整保存的输入再重新准备，不要截断规则或对白。", nil)
	}
	profile, err := service.store.RequirePictureBookProfile()
	if err != nil {
		return result, nil, err
	}
	// No temporary path, signature, random ID or timestamp participates in the
	// source identity. Source bytes, versions, labels and order all do.
	encoded, err := json.Marshal(struct {
		ProjectUUID     string
		Page            ComicSection
		Settings        []PremiseAsset
		Sources         []PageImageSource
		Rules           GenerationSnapshot
		Profile         project.PictureBookProfile
		Prompt          string
		ComposerVersion string
	}{service.store.ProjectUUID(), page, selected, result.Sources, rules, profile, prompt, SectionPremiseComposerVersion})
	if err != nil {
		return result, nil, err
	}
	digest := sha256.Sum256(encoded)
	result.Target, result.GenerationPrompt = target, prompt
	result.ImageWriteBack = PageImageWriteBack{page.UUID, "page_image", page.Revision}
	result.Preparation = PageImagePreparationReceipt{ProjectUUID: service.store.ProjectUUID(), PageUUID: page.UUID, ExpectedRevision: page.Revision, SettingUUIDs: append([]string{}, input.SettingUUIDs...), AdditionalDirection: input.AdditionalDirection, SettingReviews: append([]HostSettingReview(nil), input.SettingReviews...), Digest: hex.EncodeToString(digest[:])}
	return result, sources, nil
}

func (service *Service) PreparePageImage(ctx context.Context, input PreparePageImageInput, references *PageReferenceFiles) (PreparedPageImage, error) {
	var result PreparedPageImage
	var sources []SectionPremiseSource
	if err := service.store.RequireReady(); err != nil {
		return result, err
	}
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return service.store.WithReadView(tx, func(view *project.Store) error {
			var err error
			result, sources, err = NewService(view, nil).pageImageInputs(ctx, input)
			return err
		})
	})
	if err != nil {
		return result, err
	}
	if len(sources) > 0 {
		composition, err := ComposeSectionPremise(sources)
		if err != nil {
			return result, domainError("section_premise_composition_failed", "无法生成设定参考合图", err.Error(), err)
		}
		result.ReferenceImagePath, result.ReferenceImageSHA256, err = references.save(service.store.ProjectUUID(), composition.Bytes)
		if err != nil {
			return result, err
		}
		result.ReferenceMode = "collage"
		result.ImageInfo = &SectionPremiseImageInfo{Width: composition.Width, Height: composition.Height, MIMEType: "image/png", ComposerVersion: SectionPremiseComposerVersion}
	}
	result.Preparation.Signature = result.Preparation.signature()
	return result, nil
}

func (service *Service) validatePageImagePreparation(ctx context.Context, input ImportHostImageInput) error {
	receipt := input.Preparation
	if receipt == nil || receipt.ProjectUUID != service.store.ProjectUUID() || receipt.PageUUID != input.TargetUUID || receipt.ExpectedRevision != input.ExpectedRevision || !hmac.Equal([]byte(receipt.Signature), []byte(receipt.signature())) {
		return preparationConflict()
	}
	current, _, err := service.pageImageInputs(ctx, PreparePageImageInput{PageUUID: receipt.PageUUID, SettingUUIDs: receipt.SettingUUIDs, AdditionalDirection: receipt.AdditionalDirection, SettingReviews: receipt.SettingReviews})
	if err != nil {
		return preparationConflict()
	}
	if current.Preparation.Digest != receipt.Digest || current.GenerationPrompt != input.Prompt {
		return preparationConflict()
	}
	return nil
}
