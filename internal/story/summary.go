package story

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"lumi/internal/files"
)

// ChapterSummary is the card/navigation projection of existing facts. It never
// loads chapter prose or image input snapshots.
type ChapterSummary struct {
	Chapter
	Summary       string       `json:"summary"`
	PageCount     int          `json:"page_count"`
	ImageCount    int          `json:"image_count"`
	CoverAsset    *files.Asset `json:"cover_asset,omitempty" gorm:"-"`
	CoverFileUUID string       `json:"-"`
}

func (service *Service) ListChapterSummaries(ctx context.Context, state string) ([]ChapterSummary, error) {
	if state == "" {
		state = "active"
	}
	if state != "active" && state != "trashed" {
		return nil, storyError(CodeValidationFailed, "章节列表状态无效", "state 只支持 active 或 trashed。", nil)
	}
	query := service.store.DB().WithContext(ctx).Table("chapters c").Joins("LEFT JOIN chapter_stories story ON story.id=c.current_story_id").Where("c.project_id=(SELECT id FROM projects WHERE uuid=?)", service.store.ProjectUUID())
	if state == "active" {
		query = query.Where("c.deleted_at IS NULL").Order("c.sort_order ASC")
	} else {
		query = query.Where("c.deleted_at IS NOT NULL").Order("c.deleted_at DESC,c.sort_order ASC")
	}
	var rows []ChapterSummary
	fields := `c.uuid,c.chapter_code,c.volume_no,c.chapter_no,c.sort_order,c.title,c.revision,c.deleted_at AS trashed_at,c.created_at,c.updated_at,COALESCE(substr(story.content,1,180),'') AS summary,
 (SELECT count(*) FROM comic_sections s JOIN chapter_comic_states cs ON cs.id=s.chapter_comic_state_id WHERE cs.chapter_id=c.id AND s.deleted_at IS NULL) AS page_count,
 (SELECT count(*) FROM comic_sections s JOIN chapter_comic_states cs ON cs.id=s.chapter_comic_state_id WHERE cs.chapter_id=c.id AND s.deleted_at IS NULL AND s.current_image_variant_id IS NOT NULL) AS image_count,
 (SELECT f.uuid FROM comic_sections s JOIN chapter_comic_states cs ON cs.id=s.chapter_comic_state_id JOIN comic_image_variants v ON v.id=s.current_image_variant_id JOIN files f ON f.id=v.file_id WHERE cs.chapter_id=c.id AND s.deleted_at IS NULL AND f.deleted_at IS NULL ORDER BY CASE WHEN s.page_role='front_cover' THEN 0 ELSE 1 END,s.section_no LIMIT 1) AS cover_file_uuid`
	if err := query.Select(fields).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []ChapterSummary{}
	}
	assets := files.NewService(service.store, nil)
	for i := range rows {
		if rows[i].CoverFileUUID != "" {
			asset, err := assets.GetAsset(ctx, rows[i].CoverFileUUID, false)
			if err != nil {
				return nil, err
			}
			rows[i].CoverAsset = &asset
		}
	}
	return rows, nil
}

// StoryProfileSummary reads only committed facts. A draft with no profile is
// valid empty content; browsing must not create a profile or repair STORY.md.
type StoryProfileSummary struct {
	UUID            string `json:"uuid,omitempty"`
	Revision        int64  `json:"revision"`
	StoryMD         string `json:"story_md"`
	ProjectionState string `json:"projection_state"`
	Exists          bool   `json:"exists"`
}

func (service *Service) GetStoryProfileSummary(ctx context.Context) (StoryProfileSummary, error) {
	profile, err := service.storedStoryProfile(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return StoryProfileSummary{}, nil
	}
	if err != nil {
		return StoryProfileSummary{}, err
	}
	text := []rune(profile.StoryMD)
	if len(text) > 300 {
		text = text[:300]
	}
	return StoryProfileSummary{UUID: profile.UUID, Revision: profile.Revision, StoryMD: string(text), ProjectionState: profile.ProjectionState, Exists: true}, nil
}
