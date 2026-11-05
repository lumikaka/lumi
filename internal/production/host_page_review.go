package production

import (
	"context"
	"strings"

	"lumi/internal/taskexecution"

	"gorm.io/gorm"
)

// The compact projection never hydrates full image metadata, chapter prose or storyboards.
type AuthoringPageSummary struct {
	Kind           string `json:"kind"`
	UUID           string `json:"uuid"`
	ChapterUUID    string `json:"chapter_uuid"`
	SectionNo      int    `json:"section_no"`
	PageRole       string `json:"page_role"`
	Title          string `json:"title"`
	Revision       int64  `json:"revision"`
	HasText        bool   `json:"has_text"`
	HasStoryboard  bool   `json:"has_storyboard"`
	HasImage       bool   `json:"has_image"`
	VariantUUID    string `json:"variant_uuid,omitempty"`
	ReviewRevision int64  `json:"review_revision,omitempty"`
	AssetUUID      string `json:"asset_uuid,omitempty"`
	ReviewStatus   string `json:"review_status"`
	Observation    string `json:"observation,omitempty"`
}

func (s *Service) AuthoringPageSummaries(ctx context.Context, chapterUUID, pageUUID string) ([]AuthoringPageSummary, error) {
	chapter, err := s.chapterByUUID(ctx, s.store.DB(), chapterUUID)
	if err != nil {
		return nil, err
	}
	rows := []AuthoringPageSummary{}
	query := `
 SELECT 'page' AS kind, s.uuid, ? AS chapter_uuid, s.section_no,s.page_role,s.title,s.revision,
 s.description_md != '' AS has_text, s.current_storyboard_variant_id IS NOT NULL AS has_storyboard,
 f.id IS NOT NULL AS has_image, v.uuid AS variant_uuid, f.uuid AS asset_uuid, coalesce(r.revision,0) AS review_revision,
 CASE WHEN f.id IS NULL THEN 'missing_image' ELSE COALESCE(r.verdict,'pending') END AS review_status,
 CASE WHEN ? != '' THEN r.observation WHEN r.verdict='needs_revision' THEN substr(r.observation,1,160) END AS observation
 FROM comic_sections s JOIN chapter_comic_states cs ON cs.id=s.chapter_comic_state_id
 LEFT JOIN comic_image_variants v ON v.id=s.current_image_variant_id
 LEFT JOIN files f ON f.id=v.file_id AND f.deleted_at IS NULL
 LEFT JOIN page_image_reviews r ON r.image_variant_id=v.id AND r.page_revision=s.revision
 WHERE cs.chapter_id=? AND s.deleted_at IS NULL`
	args := []any{chapterUUID, pageUUID, chapter.ID}
	if pageUUID != "" {
		query += " AND s.uuid=?"
		args = append(args, pageUUID)
	}
	err = s.store.DB().WithContext(ctx).Raw(query+" ORDER BY s.section_no", args...).Scan(&rows).Error
	return rows, err
}

type PageImageReview struct {
	ReviewRevision int64  `json:"review_revision"`
	PageUUID       string `json:"page_uuid"`
	VariantUUID    string `json:"variant_uuid"`
	Revision       int64  `json:"revision"`
	Verdict        string `json:"verdict"`
	Observation    string `json:"observation"`
}

func (s *Service) RecordPageImageReview(ctx context.Context, chapterUUID string, review PageImageReview) (PageImageReview, error) {
	review.Observation = strings.TrimSpace(review.Observation)
	if (review.Verdict != "passed" && review.Verdict != "needs_revision") || review.Observation == "" || len([]rune(review.Observation)) > 2000 {
		return review, domainError(CodeValidation, "图片核对记录无效", "提供核对结果和具体观察（1–2000 字符）。", nil)
	}
	changed := false
	err := taskexecution.Transaction(s.store.DB().WithContext(ctx), func(tx *gorm.DB) error {
		chapter, err := s.chapterByUUID(ctx, tx, chapterUUID)
		if err != nil {
			return err
		}
		var current struct {
			VariantID, Revision, ReviewRevision int64
			Verdict, Observation                string
		}
		err = tx.Raw(`SELECT v.id AS variant_id,s.revision,r.revision AS review_revision,r.verdict,r.observation
    FROM comic_sections s JOIN chapter_comic_states cs ON cs.id=s.chapter_comic_state_id
    JOIN comic_image_variants v ON v.id=s.current_image_variant_id
    JOIN files f ON f.id=v.file_id AND f.deleted_at IS NULL
    LEFT JOIN page_image_reviews r ON r.image_variant_id=v.id AND r.page_revision=s.revision
    WHERE cs.chapter_id=? AND s.uuid=? AND v.uuid=? AND s.deleted_at IS NULL`, chapter.ID, review.PageUUID, review.VariantUUID).Scan(&current).Error
		if err != nil {
			return err
		}
		if current.VariantID == 0 || current.Revision != review.Revision {
			return domainError(CodeConflict, "页面或采用的图片已变化", "重读当前版本并重新看图核对。", nil)
		}
		if current.Verdict == review.Verdict && current.Observation == review.Observation {
			review.ReviewRevision = current.ReviewRevision
			return nil
		}
		if current.ReviewRevision != review.ReviewRevision {
			return domainError(CodeConflict, "核对记录已被修改", "重读摘要中的 review_revision 后再修订观察。", nil)
		}
		changed = true
		update := tx.Exec(`INSERT INTO page_image_reviews (image_variant_id,page_revision,revision,verdict,observation)
 VALUES (?,?,1,?,?) ON CONFLICT(image_variant_id,page_revision) DO UPDATE SET
 revision=page_image_reviews.revision+1, verdict=excluded.verdict, observation=excluded.observation
 WHERE page_image_reviews.revision=?`, current.VariantID, review.Revision, review.Verdict, review.Observation, review.ReviewRevision)
		if update.Error != nil {
			return conflictErr(update.Error)
		}
		if update.RowsAffected != 1 {
			return domainError(CodeConflict, "核对记录已被修改", "重新读取页面核对记录。", nil)
		}
		review.ReviewRevision++
		return nil
	})
	if err == nil && changed {
		s.emit("comic:section_changed", map[string]any{"chapter_uuid": chapterUUID, "section_uuid": review.PageUUID})
	}
	return review, err
}
