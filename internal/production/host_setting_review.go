package production

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"lumi/internal/story"
)

// HostSettingReview is the host's visual assessment, not a server/model score.
// It binds that assessment to the exact adopted image and effective art style.
// Page preparation signs and import persists it with the existing receipt.
type HostSettingReview struct {
	SettingUUID      string `json:"setting_uuid"`
	VariantUUID      string `json:"variant_uuid"`
	ExpectedRevision int64  `json:"expected_revision"`
	StyleDigest      string `json:"style_digest"`
	Verdict          string `json:"verdict"`
	ReferenceBasis   string `json:"reference_basis"`
	Comparison       string `json:"comparison"`
}

func SettingReviewStyleDigest(style string) string {
	digest := sha256.Sum256([]byte(style))
	return hex.EncodeToString(digest[:])
}

func (service *Service) HostSettingReviewStyle(ctx context.Context) (string, error) {
	st := story.NewService(service.store)
	p, err := st.GetProject(ctx)
	if err != nil {
		return "", err
	}
	premise, err := service.GetPremise(ctx)
	if err != nil {
		return "", err
	}
	return EffectiveImageStyle(ctx, service.store, st, premise.DefaultStyle, p.GenerationLanguage)
}

func validateHostSettingReviews(settings []PremiseAsset, style string, reviews []HostSettingReview) error {
	required := func() error {
		return domainError("setting_image_review_required", "设定图尚未通过视觉核对", "逐项对照用户要求及原参考图核对当前设定图，提交 setting_reviews（当前 setting_uuid、variant_uuid、expected_revision、style_digest、verdict=passed、reference_basis、具体 comparison）。有差异先重绘；无原图时按已保存画风核对。不能以导入成功代替核对。", nil)
	}
	if len(reviews) != len(settings) {
		return required()
	}
	byUUID := make(map[string]HostSettingReview, len(reviews))
	for _, review := range reviews {
		if _, duplicate := byUUID[review.SettingUUID]; duplicate || review.Verdict != "passed" {
			return required()
		}
		for _, evidence := range []string{review.ReferenceBasis, review.Comparison} {
			if strings.TrimSpace(evidence) == "" || len([]rune(evidence)) > 2000 {
				return required()
			}
		}
		byUUID[review.SettingUUID] = review
	}
	for _, setting := range settings {
		review, ok := byUUID[setting.UUID]
		if !ok {
			return required()
		}
		if setting.CurrentVariant == nil || review.VariantUUID != setting.CurrentVariant.UUID || review.ExpectedRevision != setting.Revision || review.StyleDigest != SettingReviewStyleDigest(style) {
			return domainError("setting_image_review_stale", "设定图核对已失效", "设定内容、采用图片或有效画风已变化。重新 image_target 读取当前版本和 style_digest，实际核对后再提交；不能只替换版本或摘要。", nil)
		}
	}
	return nil
}
