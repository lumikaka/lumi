package production

import (
	"context"
	"time"
)

type SectionSummary struct {
	UUID            string        `json:"uuid"`
	ChapterUUID     string        `json:"chapter_uuid"`
	SectionNo       int           `json:"section_no"`
	PageRole        string        `json:"page_role"`
	Title           string        `json:"title"`
	Revision        int64         `json:"revision"`
	HasStoryboard   bool          `json:"has_storyboard"`
	CurrentImage    *ImageVariant `json:"current_image" gorm:"-"`
	DeletedAt       *time.Time    `json:"deleted_at,omitempty"`
	FileUUID        string        `json:"-"`
	ImageUUID       string        `json:"-"`
	ImageVersionNo  int           `json:"-"`
	ImageSourceType string        `json:"-"`
}

func (service *Service) ListSectionSummaries(ctx context.Context, chapterUUID, state string) ([]SectionSummary, error) {
	if state == "" {
		state = "active"
	}
	if state != "active" && state != "trashed" {
		return nil, domainError(CodeValidation, "页面状态筛选无效", "state 只支持 active/trashed。", nil)
	}
	chapter, err := service.chapterByUUID(ctx, service.store.DB(), chapterUUID)
	if err != nil {
		return nil, err
	}
	query := service.store.DB().WithContext(ctx).Table("comic_sections s").Joins("JOIN chapter_comic_states cs ON cs.id=s.chapter_comic_state_id").Joins("LEFT JOIN comic_image_variants v ON v.id=s.current_image_variant_id").Joins("LEFT JOIN files f ON f.id=v.file_id AND f.deleted_at IS NULL").Where("cs.chapter_id=?", chapter.ID)
	if state == "active" {
		query = query.Where("s.deleted_at IS NULL").Order("s.section_no ASC")
	} else {
		query = query.Where("s.deleted_at IS NOT NULL").Order("s.deleted_at DESC,s.id DESC")
	}
	var rows []SectionSummary
	if err := query.Select("s.uuid,s.section_no,s.page_role,s.title,s.revision,s.deleted_at,(s.current_storyboard_variant_id IS NOT NULL) AS has_storyboard,f.uuid AS file_uuid,v.uuid AS image_uuid,v.version_no AS image_version_no,v.source_type AS image_source_type").Scan(&rows).Error; err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []SectionSummary{}
	}
	for i := range rows {
		rows[i].ChapterUUID = chapterUUID
		if rows[i].FileUUID != "" {
			asset, err := service.files.GetAsset(ctx, rows[i].FileUUID, false)
			if err != nil {
				return nil, err
			}
			rows[i].CurrentImage = &ImageVariant{UUID: rows[i].ImageUUID, VersionNo: rows[i].ImageVersionNo, SourceType: hostImageSourceType(asset, rows[i].ImageSourceType), Asset: asset}
		}
	}
	return rows, nil
}
