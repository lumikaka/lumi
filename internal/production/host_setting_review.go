package production

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// Imported pages already carry their validated setting assessments in their variant snapshot.
// Return evidence only; the caller must confirm its basis still covers this request.
func (s *Service) PreviousPageSettingReviews(ctx context.Context, pageUUID string) (map[string]HostSettingReview, error) {
	var row struct {
		PageUUID string
		Evidence string
	}
	err := s.store.DB().WithContext(ctx).Raw(`SELECT s.uuid AS page_uuid,
 COALESCE(json_extract(v.input_snapshot,'$.page_image_preparation.setting_reviews'),'[]') AS evidence
 FROM comic_sections s JOIN chapter_comic_states cs ON cs.id=s.chapter_comic_state_id
 JOIN chapters c ON c.id=cs.chapter_id JOIN projects p ON p.id=c.project_id
 LEFT JOIN comic_image_variants v ON v.id=s.current_image_variant_id
 WHERE p.uuid=? AND s.uuid=? AND c.deleted_at IS NULL AND s.deleted_at IS NULL`, s.store.ProjectUUID(), pageUUID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.PageUUID == "" {
		return nil, domainError(CodeValidation, "找不到核对来源页面", "请从当前项目的目录选择已导入图片的页面。", nil)
	}
	var reviews []HostSettingReview
	if err := json.Unmarshal([]byte(row.Evidence), &reviews); err != nil {
		return nil, err
	}
	bySetting := make(map[string]HostSettingReview, len(reviews))
	for _, r := range reviews {
		if r.Verdict == "passed" {
			bySetting[r.SettingUUID] = r
		}
	}
	return bySetting, nil
}
