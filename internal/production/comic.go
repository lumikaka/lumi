package production

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"lumi/internal/files"
	"lumi/internal/project"

	"gorm.io/gorm"
)

// MaxGeneratedComicSections is the public generation contract shared by the
// task input validator and the atomic projection that installs model output.
const MaxGeneratedComicSections = 24

type comicStateRecord struct {
	ID                   int64 `gorm:"primaryKey"`
	UUID                 string
	ChapterID            int64
	Status               string
	Revision             int64
	CreatedAt, UpdatedAt time.Time
}

func (comicStateRecord) TableName() string { return "chapter_comic_states" }

type comicSectionRecord struct {
	ID                                                int64 `gorm:"primaryKey"`
	UUID                                              string
	ChapterComicStateID, ActorID                      int64
	SectionNo                                         int
	PageRole                                          string
	Title, DescriptionMD                              string
	CurrentStoryboardVariantID, CurrentImageVariantID *int64
	Revision                                          int64
	DeletedAt                                         *time.Time
	CreatedAt, UpdatedAt                              time.Time
}

func (comicSectionRecord) TableName() string { return "comic_sections" }

const MaxSectionPremiseAssetSelections = 12

type comicSectionPremiseAssetSelectionRecord struct {
	ID             int64 `gorm:"primaryKey"`
	ComicSectionID int64
	PremiseAssetID int64
	SortOrder      int
	CreatedAt      time.Time
}

func (comicSectionPremiseAssetSelectionRecord) TableName() string {
	return "comic_section_premise_asset_selections"
}

func (service *Service) normalizePageRole(value string, defaultBody bool) (string, error) {
	role := strings.ToLower(strings.TrimSpace(value))
	if role == "" && defaultBody {
		role = PageRoleBody
	}
	switch role {
	case PageRoleFrontCover, PageRoleBody, PageRoleBackCover:
	default:
		return "", domainError(CodeValidation, "页面角色无效", "page_role 只支持 front_cover、body 或 back_cover。", nil)
	}
	if profile := service.store.OptionalPictureBookProfile(); profile != nil && profile.Format == project.PictureBookVertical && role != PageRoleBody {
		return "", domainError(CodeValidation, "条漫不支持封面或封底页面", "vertical_strip 项目的 page_role 只能是 body。", nil)
	}
	return role, nil
}

func (service *Service) protectsLastBodyPage() bool {
	profile := service.store.OptionalPictureBookProfile()
	return profile == nil || profile.Format != project.PictureBookVertical
}

type storyboardRecord struct {
	ID                      int64 `gorm:"primaryKey"`
	UUID                    string
	ComicSectionID, ActorID int64
	VersionNo               int
	ContentMD, SourceType   string
	CreatedAt               time.Time
}

func (storyboardRecord) TableName() string { return "comic_storyboard_variants" }

type imageVariantRecord struct {
	ID                        int64 `gorm:"primaryKey"`
	UUID                      string
	ComicSectionID, FileID    int64
	ImageGenerationID         *int64
	ActorID                   int64
	VersionNo                 int
	SourceType, InputSnapshot string
	CreatedAt                 time.Time
}

func (imageVariantRecord) TableName() string { return "comic_image_variants" }

type chapterSnapshotRecord struct {
	ID                                 int64 `gorm:"primaryKey"`
	UUID                               string
	ChapterComicStateID, ActorID       int64
	VersionNo                          int
	Reason, SnapshotJSON, SnapshotHash string
	CreatedAt                          time.Time
}

func (chapterSnapshotRecord) TableName() string { return "comic_chapter_snapshots" }

type chapterRow struct {
	ID                       int64
	UUID, ChapterCode, Title string
}

func (service *Service) chapterByUUID(ctx context.Context, db *gorm.DB, chapterUUID string) (chapterRow, error) {
	if !isUUIDv7(chapterUUID) {
		return chapterRow{}, domainError(CodeValidation, "章节 UUID 无效", "chapter_uuid 必须是 UUIDv7。", nil)
	}
	var row chapterRow
	err := db.WithContext(ctx).Table("chapters").Select("id,uuid,chapter_code,title").Where("uuid = ? AND project_id=(SELECT id FROM projects WHERE uuid=?) AND deleted_at IS NULL", chapterUUID, service.store.ProjectUUID()).Scan(&row).Error
	if err != nil || row.ID == 0 {
		return row, notFound(gorm.ErrRecordNotFound, "章节不存在")
	}
	return row, nil
}

func (service *Service) ensureComicState(ctx context.Context, db *gorm.DB, chapterUUID string) (comicStateRecord, chapterRow, error) {
	chapter, err := service.chapterByUUID(ctx, db, chapterUUID)
	if err != nil {
		return comicStateRecord{}, chapter, err
	}
	var state comicStateRecord
	err = db.WithContext(ctx).Where("chapter_id = ?", chapter.ID).First(&state).Error
	if err == nil {
		return state, chapter, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return state, chapter, err
	}
	uuid, err := newUUIDv7()
	if err != nil {
		return state, chapter, err
	}
	now := service.now().UTC()
	state = comicStateRecord{UUID: uuid, ChapterID: chapter.ID, Status: "empty", CreatedAt: now, UpdatedAt: now}
	if err := db.WithContext(ctx).Create(&state).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return service.ensureComicState(ctx, db, chapterUUID)
		}
		return state, chapter, err
	}
	return state, chapter, nil
}

func (service *Service) GetComicState(ctx context.Context, chapterUUID string) (ComicState, error) {
	state, chapter, err := service.ensureComicState(ctx, service.store.DB(), chapterUUID)
	if err != nil {
		return ComicState{}, err
	}
	var premiseAssetCount int64
	if err := service.store.DB().WithContext(ctx).Table("premise_assets AS assets").
		Joins("JOIN premise_asset_variants AS variants ON variants.id=assets.current_variant_id").
		Joins("JOIN files ON files.id=variants.file_id AND files.deleted_at IS NULL").
		Joins("JOIN file_objects ON file_objects.id=files.file_object_id AND file_objects.state='ready'").
		Where("assets.project_id=(SELECT id FROM projects WHERE uuid=?) AND assets.deleted_at IS NULL", service.store.ProjectUUID()).
		Distinct("assets.id").Count(&premiseAssetCount).Error; err != nil {
		return ComicState{}, err
	}
	return ComicState{UUID: state.UUID, ChapterUUID: chapter.UUID, Status: state.Status, HasPremiseAssets: premiseAssetCount > 0, PremiseAssetCount: int(premiseAssetCount), Revision: state.Revision, CreatedAt: state.CreatedAt, UpdatedAt: state.UpdatedAt}, nil
}

func (service *Service) ListSections(ctx context.Context, chapterUUID string) ([]ComicSection, error) {
	return service.ListSectionsByState(ctx, chapterUUID, "active")
}

func (service *Service) ListSectionsByState(ctx context.Context, chapterUUID, sectionState string) ([]ComicSection, error) {
	sectionState = strings.ToLower(strings.TrimSpace(sectionState))
	if sectionState == "" {
		sectionState = "active"
	}
	if sectionState != "active" && sectionState != "trashed" {
		return nil, domainError(CodeValidation, "页面状态筛选无效", "state 只支持 active/trashed。", nil)
	}
	state, chapter, err := service.ensureComicState(ctx, service.store.DB(), chapterUUID)
	if err != nil {
		return nil, err
	}
	query := service.store.DB().WithContext(ctx).Where("chapter_comic_state_id = ?", state.ID)
	if sectionState == "trashed" {
		query = query.Where("deleted_at IS NOT NULL").Order("deleted_at DESC, id DESC")
	} else {
		query = query.Where("deleted_at IS NULL").Order("section_no ASC")
	}
	var rows []comicSectionRecord
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]ComicSection, 0, len(rows))
	for _, row := range rows {
		dto, err := service.sectionDTO(ctx, row, chapter.UUID)
		if err != nil {
			return nil, err
		}
		items = append(items, dto)
	}
	return items, nil
}

func sectionsWithPageRole(sections []ComicSection, pageRole string) []ComicSection {
	items := make([]ComicSection, 0, len(sections))
	for _, section := range sections {
		if section.PageRole == pageRole {
			items = append(items, section)
		}
	}
	return items
}

func (service *Service) listSectionsByPageRole(ctx context.Context, chapterUUID, pageRole string) ([]ComicSection, error) {
	sections, err := service.ListSections(ctx, chapterUUID)
	if err != nil {
		return nil, err
	}
	return sectionsWithPageRole(sections, pageRole), nil
}

func (service *Service) GetSection(ctx context.Context, chapterUUID, sectionUUID string) (ComicSection, error) {
	state, chapter, err := service.ensureComicState(ctx, service.store.DB(), chapterUUID)
	if err != nil {
		return ComicSection{}, err
	}
	var row comicSectionRecord
	if err := service.store.DB().WithContext(ctx).Where("uuid = ? AND chapter_comic_state_id = ? AND deleted_at IS NULL", sectionUUID, state.ID).First(&row).Error; err != nil {
		return ComicSection{}, notFound(err, "Comic section 不存在")
	}
	return service.sectionDTO(ctx, row, chapter.UUID)
}

func (service *Service) CreateSection(ctx context.Context, chapterUUID string, input CreateSectionInput) (ComicSection, error) {
	title := strings.TrimSpace(input.Title)
	description := strings.TrimSpace(input.DescriptionMD)
	storyboard := strings.TrimSpace(input.StoryboardMD)
	pageRole, err := service.normalizePageRole(input.PageRole, true)
	if err != nil {
		return ComicSection{}, err
	}
	if len([]rune(title)) > 160 || len([]rune(description)) > 262144 || len([]rune(storyboard)) > 262144 {
		return ComicSection{}, domainError(CodeValidation, "Section 内容过长", "title 最多 160 字符，description_md 和 storyboard_md 最多 262144 字符。", nil)
	}
	uuid, err := newUUIDv7()
	if err != nil {
		return ComicSection{}, err
	}
	var row comicSectionRecord
	var chapter chapterRow
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, foundChapter, err := service.ensureComicState(ctx, tx, chapterUUID)
		if err != nil {
			return err
		}
		chapter = foundChapter
		_, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		if service.protectsLastBodyPage() && pageRole != PageRoleBody {
			if err := ensureBodyPageExistsTx(tx, state.ID); err != nil {
				return err
			}
		}
		var max int
		if err := tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND deleted_at IS NULL", state.ID).Select("COALESCE(MAX(section_no),0)").Scan(&max).Error; err != nil {
			return err
		}
		if err := ensureSpecialPageRoleAvailableTx(tx, state.ID, pageRole, 0); err != nil {
			return err
		}
		now := service.now().UTC()
		row = comicSectionRecord{UUID: uuid, ChapterComicStateID: state.ID, ActorID: actor.ID, SectionNo: max + 1, PageRole: pageRole, Title: title, DescriptionMD: description, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return conflictErr(err)
		}
		if storyboard != "" {
			variant, err := createStoryboardTx(tx, row, actor.ID, storyboard, "manual", now)
			if err != nil {
				return err
			}
			row.CurrentStoryboardVariantID = &variant.ID
			if err := tx.Model(&row).Update("current_storyboard_variant_id", variant.ID).Error; err != nil {
				return err
			}
		}
		if err := normalizeSectionOrderTx(tx, state.ID, now); err != nil {
			return err
		}
		if err := tx.First(&row, row.ID).Error; err != nil {
			return err
		}
		if err := updateComicStateTx(tx, state.ID, now); err != nil {
			return err
		}
		if err := appendSectionEvent(tx, row.ID, "section_created", map[string]any{"section_uuid": row.UUID, "section_no": row.SectionNo, "page_role": row.PageRole}, now); err != nil {
			return err
		}
		return service.createChapterSnapshotTx(ctx, tx, state.ID, actor.ID, "section_created")
	})
	if err != nil {
		return ComicSection{}, err
	}
	dto, err := service.sectionDTO(ctx, row, chapter.UUID)
	if err == nil {
		service.emit("comic:section_changed", map[string]any{"chapter_uuid": chapterUUID, "section_uuid": dto.UUID})
	}
	return dto, err
}

// CreateGeneratedSections atomically installs a complete model-generated
// storyboard. Repeating the same output is idempotent, which lets a River retry
// finish task projection after the content transaction already committed.
func (service *Service) CreateGeneratedSections(ctx context.Context, chapterUUID string, generated []GeneratedComicSection) ([]ComicSection, error) {
	if err := normalizeGeneratedSections(generated); err != nil {
		return nil, err
	}
	allSections, err := service.ListSections(ctx, chapterUUID)
	if err != nil {
		return nil, err
	}
	existing := sectionsWithPageRole(allSections, PageRoleBody)
	if len(existing) > 0 {
		if len(existing) != len(generated) {
			state, stateErr := service.GetComicState(ctx, chapterUUID)
			if stateErr != nil {
				return nil, stateErr
			}
			return nil, generatedSectionsConflict("章节已有 Comic Sections", "为避免覆盖手工内容，AI storyboard 只能写入空章节。", len(existing), len(generated), state.Revision)
		}
		for index, section := range existing {
			if section.Title != generated[index].Title || section.CurrentStoryboard == nil || strings.TrimSpace(section.CurrentStoryboard.ContentMD) != generated[index].StoryboardMD {
				state, stateErr := service.GetComicState(ctx, chapterUUID)
				if stateErr != nil {
					return nil, stateErr
				}
				return nil, generatedSectionsConflict("章节已有不同 Comic Sections", "现有分镜与任务快照结果不同，未执行覆盖。", len(existing), len(generated), state.Revision)
			}
		}
		return existing, nil
	}

	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, _, err := service.ensureComicState(ctx, tx, chapterUUID)
		if err != nil {
			return err
		}
		_, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		now := service.now().UTC()
		var max int
		if err := tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND deleted_at IS NULL", state.ID).Select("COALESCE(MAX(section_no),0)").Scan(&max).Error; err != nil {
			return err
		}
		created := make([]comicSectionRecord, 0, len(generated))
		for index, item := range generated {
			sectionUUID, uuidErr := newUUIDv7()
			if uuidErr != nil {
				return uuidErr
			}
			row := comicSectionRecord{UUID: sectionUUID, ChapterComicStateID: state.ID, ActorID: actor.ID, SectionNo: max + index + 1, PageRole: PageRoleBody, Title: item.Title, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			variant, err := createStoryboardTx(tx, row, actor.ID, item.StoryboardMD, "generated", now)
			if err != nil {
				return err
			}
			if err := tx.Model(&row).Update("current_storyboard_variant_id", variant.ID).Error; err != nil {
				return err
			}
			created = append(created, row)
		}
		if err := normalizeSectionOrderTx(tx, state.ID, now); err != nil {
			return err
		}
		for _, createdRow := range created {
			if err := tx.First(&createdRow, createdRow.ID).Error; err != nil {
				return err
			}
			if err := appendSectionEvent(tx, createdRow.ID, "section_created", map[string]any{"section_uuid": createdRow.UUID, "section_no": createdRow.SectionNo, "page_role": createdRow.PageRole, "source_type": "generated"}, now); err != nil {
				return err
			}
		}
		if err := updateComicStateTx(tx, state.ID, now); err != nil {
			return err
		}
		return service.createChapterSnapshotTx(ctx, tx, state.ID, actor.ID, "storyboard_generated")
	})
	if err != nil {
		return nil, err
	}
	items, err := service.listSectionsByPageRole(ctx, chapterUUID, PageRoleBody)
	if err == nil {
		service.emit("comic:section_changed", map[string]any{"chapter_uuid": chapterUUID, "generated": true})
	}
	return items, err
}

// ReplaceGeneratedSections atomically replaces the active storyboard after an
// explicit user confirmation. Existing sections are soft-deleted and remain
// recoverable through the snapshots created immediately before and after the
// replacement. Repeating an already-applied result is idempotent.
func (service *Service) ReplaceGeneratedSections(ctx context.Context, chapterUUID string, generated []GeneratedComicSection, expectedStateRevision int64) ([]ComicSection, error) {
	if err := normalizeGeneratedSections(generated); err != nil {
		return nil, err
	}
	if expectedStateRevision < 0 {
		return nil, domainError(CodeValidation, "Comic state revision 无效", "expected_comic_state_revision 必须是非负整数。", nil)
	}
	changed := false
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, _, err := service.ensureComicState(ctx, tx, chapterUUID)
		if err != nil {
			return err
		}
		var existing []comicSectionRecord
		if err := tx.Where("chapter_comic_state_id = ? AND page_role = ? AND deleted_at IS NULL", state.ID, PageRoleBody).Order("section_no ASC").Find(&existing).Error; err != nil {
			return err
		}
		matches, err := generatedSectionsMatchTx(tx, existing, generated)
		if err != nil {
			return err
		}
		if matches {
			return nil
		}
		if state.Revision != expectedStateRevision {
			return domainError(CodeStateConflict, "Comic Sections 已发生变化", "请刷新并重新确认后再覆盖。", nil)
		}
		_, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		now := service.now().UTC()
		if len(existing) > 0 {
			if err := service.createChapterSnapshotTx(ctx, tx, state.ID, actor.ID, "before_storyboard_overwrite"); err != nil {
				return err
			}
			if err := tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND page_role = ? AND deleted_at IS NULL", state.ID, PageRoleBody).Updates(map[string]any{
				"deleted_at": now, "revision": gorm.Expr("revision + 1"), "updated_at": now,
			}).Error; err != nil {
				return err
			}
			for _, row := range existing {
				if err := appendSectionEvent(tx, row.ID, "section_deleted", map[string]any{"section_uuid": row.UUID, "reason": "storyboard_overwrite"}, now); err != nil {
					return err
				}
			}
		}
		var max int
		if err := tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND deleted_at IS NULL", state.ID).Select("COALESCE(MAX(section_no),0)").Scan(&max).Error; err != nil {
			return err
		}
		created := make([]comicSectionRecord, 0, len(generated))
		for index, item := range generated {
			sectionUUID, uuidErr := newUUIDv7()
			if uuidErr != nil {
				return uuidErr
			}
			row := comicSectionRecord{UUID: sectionUUID, ChapterComicStateID: state.ID, ActorID: actor.ID, SectionNo: max + index + 1, PageRole: PageRoleBody, Title: item.Title, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			variant, err := createStoryboardTx(tx, row, actor.ID, item.StoryboardMD, "generated", now)
			if err != nil {
				return err
			}
			if err := tx.Model(&row).Update("current_storyboard_variant_id", variant.ID).Error; err != nil {
				return err
			}
			created = append(created, row)
		}
		if err := normalizeSectionOrderTx(tx, state.ID, now); err != nil {
			return err
		}
		for _, createdRow := range created {
			if err := tx.First(&createdRow, createdRow.ID).Error; err != nil {
				return err
			}
			if err := appendSectionEvent(tx, createdRow.ID, "section_created", map[string]any{"section_uuid": createdRow.UUID, "section_no": createdRow.SectionNo, "page_role": createdRow.PageRole, "source_type": "generated", "reason": "storyboard_overwrite"}, now); err != nil {
				return err
			}
		}
		if err := updateComicStateTx(tx, state.ID, now); err != nil {
			return err
		}
		if err := service.createChapterSnapshotTx(ctx, tx, state.ID, actor.ID, "storyboard_overwritten"); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	items, err := service.listSectionsByPageRole(ctx, chapterUUID, PageRoleBody)
	if err == nil && changed {
		service.emit("comic:section_changed", map[string]any{"chapter_uuid": chapterUUID, "generated": true, "overwritten": true})
	}
	return items, err
}

func normalizeGeneratedSections(generated []GeneratedComicSection) error {
	if len(generated) < 1 || len(generated) > MaxGeneratedComicSections {
		return domainError(CodeValidation, "Comic storyboard 数量无效", fmt.Sprintf("sections 必须包含 1 到 %d 项。", MaxGeneratedComicSections), nil)
	}
	for index := range generated {
		generated[index].Title = strings.TrimSpace(generated[index].Title)
		generated[index].StoryboardMD = strings.TrimSpace(generated[index].StoryboardMD)
		if generated[index].Title == "" || generated[index].StoryboardMD == "" || len([]rune(generated[index].Title)) > 160 || len([]rune(generated[index].StoryboardMD)) > 262144 {
			return domainError(CodeValidation, "Comic storyboard 内容无效", "每个 section 都需要有效 title 和 storyboard。", nil)
		}
	}
	return nil
}

func generatedSectionsMatchTx(tx *gorm.DB, existing []comicSectionRecord, generated []GeneratedComicSection) (bool, error) {
	if len(existing) != len(generated) {
		return false, nil
	}
	for index, row := range existing {
		if row.Title != generated[index].Title || row.CurrentStoryboardVariantID == nil {
			return false, nil
		}
		var storyboard storyboardRecord
		if err := tx.Where("id = ?", *row.CurrentStoryboardVariantID).First(&storyboard).Error; err != nil {
			return false, err
		}
		if strings.TrimSpace(storyboard.ContentMD) != generated[index].StoryboardMD {
			return false, nil
		}
	}
	return true, nil
}

func (service *Service) UpdateSection(ctx context.Context, chapterUUID, sectionUUID string, input UpdateSectionInput) (ComicSection, error) {
	var pageRole *string
	if input.PageRole != nil {
		normalized, err := service.normalizePageRole(*input.PageRole, false)
		if err != nil {
			return ComicSection{}, err
		}
		pageRole = &normalized
	}
	var row comicSectionRecord
	var actorID int64
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, _, err := service.ensureComicState(ctx, tx, chapterUUID)
		if err != nil {
			return err
		}
		_, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		actorID = actor.ID
		if err := tx.Where("uuid = ? AND chapter_comic_state_id = ? AND deleted_at IS NULL", sectionUUID, state.ID).First(&row).Error; err != nil {
			return notFound(err, "Comic section 不存在")
		}
		if service.protectsLastBodyPage() && pageRole != nil && row.PageRole == PageRoleBody && *pageRole != PageRoleBody {
			if err := ensureBodyPageRemainsTx(tx, state.ID, row.ID); err != nil {
				return err
			}
		}
		updates := map[string]any{"revision": gorm.Expr("revision + 1"), "updated_at": service.now().UTC()}
		if input.Title != nil {
			value := strings.TrimSpace(*input.Title)
			if len([]rune(value)) > 160 {
				return domainError(CodeValidation, "Section title 过长", "title 最多 160 个字符。", nil)
			}
			updates["title"] = value
		}
		if input.DescriptionMD != nil {
			value := strings.TrimSpace(*input.DescriptionMD)
			if len([]rune(value)) > 262144 {
				return domainError(CodeValidation, "Section description 过长", "description_md 最多 262144 个字符。", nil)
			}
			updates["description_md"] = value
		}
		if pageRole != nil {
			if err := ensureSpecialPageRoleAvailableTx(tx, state.ID, *pageRole, row.ID); err != nil {
				return err
			}
			updates["page_role"] = *pageRole
		}
		result := tx.Model(&row).Where("revision = ?", input.ExpectedRevision).Updates(updates)
		if result.Error != nil {
			return conflictErr(result.Error)
		}
		if result.RowsAffected != 1 {
			return domainError(CodeConflict, "Section 已被修改", "刷新后重试。", nil)
		}
		now := service.now().UTC()
		if err := normalizeSectionOrderTx(tx, state.ID, now); err != nil {
			return err
		}
		if err := tx.First(&row, row.ID).Error; err != nil {
			return err
		}
		if err := appendSectionEvent(tx, row.ID, "section_updated", map[string]any{"section_uuid": row.UUID, "section_no": row.SectionNo, "page_role": row.PageRole}, now); err != nil {
			return err
		}
		if err := updateComicStateTx(tx, state.ID, now); err != nil {
			return err
		}
		return service.createChapterSnapshotTx(ctx, tx, state.ID, actorID, "section_updated")
	})
	if err != nil {
		return ComicSection{}, err
	}
	dto, err := service.GetSection(ctx, chapterUUID, sectionUUID)
	if err == nil {
		service.emit("comic:section_changed", map[string]any{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID})
	}
	return dto, err
}

func (service *Service) DeleteSection(ctx context.Context, chapterUUID, sectionUUID string, expectedRevision int64) error {
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, _, err := service.ensureComicState(ctx, tx, chapterUUID)
		if err != nil {
			return err
		}
		_, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		var row comicSectionRecord
		if err := tx.Where("uuid = ? AND chapter_comic_state_id = ? AND deleted_at IS NULL", sectionUUID, state.ID).First(&row).Error; err != nil {
			return notFound(err, "Comic section 不存在")
		}
		if service.protectsLastBodyPage() && row.PageRole == PageRoleBody {
			if err := ensureBodyPageRemainsTx(tx, state.ID, row.ID); err != nil {
				return err
			}
		}
		now := service.now().UTC()
		result := tx.Model(&row).Where("revision = ?", expectedRevision).Updates(map[string]any{"deleted_at": now, "revision": gorm.Expr("revision + 1"), "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainError(CodeConflict, "Section 已被修改", "刷新后重试。", nil)
		}
		if err := normalizeSectionOrderTx(tx, state.ID, now); err != nil {
			return err
		}
		if err := appendSectionEvent(tx, row.ID, "section_deleted", map[string]any{"section_uuid": row.UUID}, now); err != nil {
			return err
		}
		if err := updateComicStateTx(tx, state.ID, now); err != nil {
			return err
		}
		return service.createChapterSnapshotTx(ctx, tx, state.ID, actor.ID, "section_deleted")
	})
	if err == nil {
		service.emit("comic:section_changed", map[string]any{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID, "deleted": true})
	}
	return err
}

func (service *Service) RestoreSection(ctx context.Context, chapterUUID, sectionUUID string, expectedRevision int64) (ComicSection, error) {
	var row comicSectionRecord
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, _, err := service.ensureComicState(ctx, tx, chapterUUID)
		if err != nil {
			return err
		}
		_, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		if err := tx.Where("uuid = ? AND chapter_comic_state_id = ?", sectionUUID, state.ID).First(&row).Error; err != nil {
			return notFound(err, "Comic section 不存在")
		}
		if row.DeletedAt == nil {
			return domainError(CodeStateConflict, "页面不在回收站", "只有已移入回收站的页面可以恢复。", nil)
		}
		if service.protectsLastBodyPage() && row.PageRole != PageRoleBody {
			if err := ensureBodyPageExistsTx(tx, state.ID); err != nil {
				return err
			}
		}
		if err := ensureSpecialPageRoleAvailableTx(tx, state.ID, row.PageRole, row.ID); err != nil {
			return err
		}
		var maxSectionNo int
		if err := tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND deleted_at IS NULL", state.ID).Select("COALESCE(MAX(section_no),0)").Scan(&maxSectionNo).Error; err != nil {
			return err
		}
		now := service.now().UTC()
		result := tx.Model(&row).Where("revision = ? AND deleted_at IS NOT NULL", expectedRevision).Updates(map[string]any{
			"deleted_at": nil,
			"section_no": maxSectionNo + 1,
			"revision":   gorm.Expr("revision + 1"),
			"updated_at": now,
		})
		if result.Error != nil {
			return conflictErr(result.Error)
		}
		if result.RowsAffected != 1 {
			return domainError(CodeConflict, "页面已被修改", "刷新回收站后重试。", nil)
		}
		if err := normalizeSectionOrderTx(tx, state.ID, now); err != nil {
			return err
		}
		if err := tx.First(&row, row.ID).Error; err != nil {
			return err
		}
		if err := appendSectionEvent(tx, row.ID, "section_restored", map[string]any{"section_uuid": row.UUID, "section_no": row.SectionNo, "page_role": row.PageRole}, now); err != nil {
			return err
		}
		if err := updateComicStateTx(tx, state.ID, now); err != nil {
			return err
		}
		return service.createChapterSnapshotTx(ctx, tx, state.ID, actor.ID, "section_restored")
	})
	if err != nil {
		return ComicSection{}, err
	}
	dto, err := service.GetSection(ctx, chapterUUID, sectionUUID)
	if err == nil {
		service.emit("comic:section_changed", map[string]any{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID, "restored": true})
	}
	return dto, err
}

func (service *Service) ReorderSections(ctx context.Context, chapterUUID string, orderedUUIDs []string) ([]ComicSection, error) {
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, _, err := service.ensureComicState(ctx, tx, chapterUUID)
		if err != nil {
			return err
		}
		_, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		var rows []comicSectionRecord
		if err := tx.Where("chapter_comic_state_id = ? AND deleted_at IS NULL", state.ID).Order("section_no").Find(&rows).Error; err != nil {
			return err
		}
		bodyRows := make([]comicSectionRecord, 0, len(rows))
		bodyByUUID := map[string]comicSectionRecord{}
		activeByUUID := make(map[string]comicSectionRecord, len(rows))
		for _, row := range rows {
			activeByUUID[row.UUID] = row
			if row.PageRole == PageRoleBody {
				bodyRows = append(bodyRows, row)
				bodyByUUID[row.UUID] = row
			}
		}
		orderedBodyUUIDs := make([]string, 0, len(bodyRows))
		seen := make(map[string]struct{}, len(orderedUUIDs))
		switch {
		case len(orderedUUIDs) == len(bodyRows):
			for _, uuid := range orderedUUIDs {
				if _, ok := bodyByUUID[uuid]; !ok {
					return domainError(CodeValidation, "正文页顺序包含未知 UUID", "body-only 顺序只能包含当前章节的 active body 页面。", nil)
				}
				if _, ok := seen[uuid]; ok {
					return domainError(CodeValidation, "正文页顺序包含重复 UUID", "每个 active body 页面必须且只能出现一次。", nil)
				}
				seen[uuid] = struct{}{}
				orderedBodyUUIDs = append(orderedBodyUUIDs, uuid)
			}
		case len(orderedUUIDs) == len(rows):
			for _, uuid := range orderedUUIDs {
				row, ok := activeByUUID[uuid]
				if !ok {
					return domainError(CodeValidation, "页面顺序包含未知 UUID", "full-list 顺序只能包含当前章节的 active 页面。", nil)
				}
				if _, ok := seen[uuid]; ok {
					return domainError(CodeValidation, "页面顺序包含重复 UUID", "每个 active 页面必须且只能出现一次。", nil)
				}
				seen[uuid] = struct{}{}
				if row.PageRole == PageRoleBody {
					orderedBodyUUIDs = append(orderedBodyUUIDs, uuid)
				}
			}
		default:
			return domainError(CodeValidation, "正文页顺序不完整", "section_uuids 必须包含全部 active body 页面，或兼容性地包含全部 active 页面。", nil)
		}
		now := service.now().UTC()
		orderedRows := make([]comicSectionRecord, 0, len(rows))
		for _, row := range rows {
			if row.PageRole == PageRoleFrontCover {
				orderedRows = append(orderedRows, row)
			}
		}
		incrementRevision := make(map[int64]struct{}, len(orderedBodyUUIDs))
		for _, uuid := range orderedBodyUUIDs {
			row := bodyByUUID[uuid]
			orderedRows = append(orderedRows, row)
			incrementRevision[row.ID] = struct{}{}
		}
		for _, row := range rows {
			if row.PageRole == PageRoleBackCover {
				orderedRows = append(orderedRows, row)
			}
		}
		if err := writeSectionOrderTx(tx, orderedRows, incrementRevision, now); err != nil {
			return err
		}
		if err := updateComicStateTx(tx, state.ID, now); err != nil {
			return err
		}
		return service.createChapterSnapshotTx(ctx, tx, state.ID, actor.ID, "sections_reordered")
	})
	if err != nil {
		return nil, err
	}
	service.emit("comic:sections_reordered", map[string]any{"chapter_uuid": chapterUUID})
	return service.ListSections(ctx, chapterUUID)
}

func (service *Service) SetSectionPremiseAssets(ctx context.Context, chapterUUID, sectionUUID string, orderedAssetUUIDs []string, expectedRevision int64) (ComicSection, error) {
	if len(orderedAssetUUIDs) > MaxSectionPremiseAssetSelections {
		return ComicSection{}, domainError(CodeValidation, "页面设定引用过多", fmt.Sprintf("premise_asset_uuids 最多包含 %d 个设定项。", MaxSectionPremiseAssetSelections), nil)
	}
	seen := make(map[string]struct{}, len(orderedAssetUUIDs))
	for _, assetUUID := range orderedAssetUUIDs {
		if !isUUIDv7(assetUUID) {
			return ComicSection{}, domainError(CodeValidation, "设定引用 UUID 无效", "premise_asset_uuids 只能包含 UUIDv7。", nil)
		}
		if _, exists := seen[assetUUID]; exists {
			return ComicSection{}, domainError(CodeValidation, "设定引用重复", "premise_asset_uuids 不得包含重复值。", nil)
		}
		seen[assetUUID] = struct{}{}
	}
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, _, err := service.ensureComicState(ctx, tx, chapterUUID)
		if err != nil {
			return err
		}
		var section comicSectionRecord
		if err := tx.Where("uuid = ? AND chapter_comic_state_id = ? AND deleted_at IS NULL", sectionUUID, state.ID).First(&section).Error; err != nil {
			return notFound(err, "Comic section 不存在")
		}
		if section.Revision != expectedRevision {
			return domainError(CodeConflict, "Section 已被修改", "刷新后重试。", nil)
		}
		assetIDs := make([]int64, 0, len(orderedAssetUUIDs))
		for _, assetUUID := range orderedAssetUUIDs {
			var asset premiseAssetRecord
			if err := tx.Where("uuid = ? AND project_id=(SELECT id FROM projects WHERE uuid=?) AND deleted_at IS NULL AND current_variant_id IS NOT NULL", assetUUID, service.store.ProjectUUID()).First(&asset).Error; err != nil {
				return notFound(err, "设定引用不存在或尚无图片")
			}
			assetIDs = append(assetIDs, asset.ID)
		}
		if err := tx.Where("comic_section_id = ?", section.ID).Delete(&comicSectionPremiseAssetSelectionRecord{}).Error; err != nil {
			return err
		}
		now := service.now().UTC()
		for index, assetID := range assetIDs {
			selection := comicSectionPremiseAssetSelectionRecord{ComicSectionID: section.ID, PremiseAssetID: assetID, SortOrder: index + 1, CreatedAt: now}
			if err := tx.Create(&selection).Error; err != nil {
				return conflictErr(err)
			}
		}
		result := tx.Model(&section).Where("revision = ?", expectedRevision).Updates(map[string]any{"revision": gorm.Expr("revision + 1"), "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainError(CodeConflict, "Section 已被修改", "刷新后重试。", nil)
		}
		if err := appendSectionEvent(tx, section.ID, "premise_assets_selected", map[string]any{"section_uuid": section.UUID, "premise_asset_uuids": orderedAssetUUIDs}, now); err != nil {
			return err
		}
		return updateComicStateTx(tx, state.ID, now)
	})
	if err != nil {
		return ComicSection{}, err
	}
	section, err := service.GetSection(ctx, chapterUUID, sectionUUID)
	if err == nil {
		service.emit("comic:section_changed", map[string]any{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID})
	}
	return section, err
}

func (service *Service) CreateStoryboard(ctx context.Context, chapterUUID, sectionUUID, content, sourceType string, expectedRevision int64) (ComicSection, error) {
	content = strings.TrimSpace(content)
	if content == "" || len([]rune(content)) > 262144 {
		return ComicSection{}, domainError(CodeValidation, "Storyboard 无效", "content_md 必须包含内容且最多 262144 个字符。", nil)
	}
	if sourceType == "" {
		sourceType = "manual"
	}
	if sourceType != "manual" && sourceType != "generated" && sourceType != "restore" {
		return ComicSection{}, domainError(CodeValidation, "Storyboard source_type 无效", "source_type 不受支持。", nil)
	}
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, _, err := service.ensureComicState(ctx, tx, chapterUUID)
		if err != nil {
			return err
		}
		_, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		var row comicSectionRecord
		if err := tx.Where("uuid = ? AND chapter_comic_state_id = ? AND deleted_at IS NULL", sectionUUID, state.ID).First(&row).Error; err != nil {
			return notFound(err, "Comic section 不存在")
		}
		if row.Revision != expectedRevision {
			return domainError(CodeConflict, "Section 已被修改", "刷新后重试。", nil)
		}
		variant, err := createStoryboardTx(tx, row, actor.ID, content, sourceType, service.now().UTC())
		if err != nil {
			return err
		}
		result := tx.Model(&row).Where("revision = ?", expectedRevision).Updates(map[string]any{"current_storyboard_variant_id": variant.ID, "revision": gorm.Expr("revision + 1"), "updated_at": service.now().UTC()})
		if result.Error != nil || result.RowsAffected != 1 {
			return domainError(CodeConflict, "Section 已被修改", "刷新后重试。", result.Error)
		}
		if err := appendSectionEvent(tx, row.ID, "storyboard_created", map[string]any{"section_uuid": row.UUID, "storyboard_uuid": variant.UUID}, service.now().UTC()); err != nil {
			return err
		}
		if err := updateComicStateTx(tx, state.ID, service.now().UTC()); err != nil {
			return err
		}
		return service.createChapterSnapshotTx(ctx, tx, state.ID, actor.ID, "storyboard_created")
	})
	if err != nil {
		return ComicSection{}, err
	}
	dto, err := service.GetSection(ctx, chapterUUID, sectionUUID)
	if err == nil {
		service.emit("comic:section_changed", map[string]any{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID})
	}
	return dto, err
}

func (service *Service) ListStoryboards(ctx context.Context, chapterUUID, sectionUUID string) ([]StoryboardVariant, error) {
	section, err := service.GetSection(ctx, chapterUUID, sectionUUID)
	if err != nil {
		return nil, err
	}
	var row comicSectionRecord
	_ = service.store.DB().WithContext(ctx).Where("uuid = ?", section.UUID).First(&row).Error
	var variants []storyboardRecord
	if err := service.store.DB().WithContext(ctx).Where("comic_section_id = ?", row.ID).Order("version_no DESC").Find(&variants).Error; err != nil {
		return nil, err
	}
	result := make([]StoryboardVariant, 0, len(variants))
	for _, item := range variants {
		result = append(result, storyboardDTO(item))
	}
	return result, nil
}

func (service *Service) SelectStoryboard(ctx context.Context, chapterUUID, sectionUUID, variantUUID string, expectedRevision int64) (ComicSection, error) {
	var content string
	var source storyboardRecord
	section, err := service.GetSection(ctx, chapterUUID, sectionUUID)
	if err != nil {
		return ComicSection{}, err
	}
	var sectionRow comicSectionRecord
	_ = service.store.DB().WithContext(ctx).Where("uuid = ?", section.UUID).First(&sectionRow).Error
	if err := service.store.DB().WithContext(ctx).Where("uuid = ? AND comic_section_id = ?", variantUUID, sectionRow.ID).First(&source).Error; err != nil {
		return ComicSection{}, notFound(err, "Storyboard 版本不存在")
	}
	content = source.ContentMD
	return service.CreateStoryboard(ctx, chapterUUID, sectionUUID, content, "restore", expectedRevision)
}

func (service *Service) ImportSectionImage(ctx context.Context, chapterUUID, sectionUUID, uploadUUID string, expectedRevision int64) (ComicSection, error) {
	variantUUID, err := newUUIDv7()
	if err != nil {
		return ComicSection{}, err
	}
	_, err = service.files.FinalizeUploadWithBind(ctx, uploadUUID, "comic_section_image", func(tx *gorm.DB, fileID int64) error {
		state, _, err := service.ensureComicState(ctx, tx, chapterUUID)
		if err != nil {
			return err
		}
		_, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		var section comicSectionRecord
		if err := tx.Where("uuid = ? AND chapter_comic_state_id = ? AND deleted_at IS NULL", sectionUUID, state.ID).First(&section).Error; err != nil {
			return notFound(err, "Comic section 不存在")
		}
		if section.Revision != expectedRevision {
			return domainError(CodeConflict, "Section 已被修改", "刷新后重试。", nil)
		}
		var version int
		if err := tx.Model(&imageVariantRecord{}).Where("comic_section_id = ?", section.ID).Select("COALESCE(MAX(version_no),0)+1").Scan(&version).Error; err != nil {
			return err
		}
		now := service.now().UTC()
		variant := imageVariantRecord{UUID: variantUUID, ComicSectionID: section.ID, FileID: fileID, ActorID: actor.ID, VersionNo: version, SourceType: "manual", InputSnapshot: "{}", CreatedAt: now}
		if err := tx.Create(&variant).Error; err != nil {
			return err
		}
		result := tx.Model(&section).Where("revision = ?", expectedRevision).Updates(map[string]any{"current_image_variant_id": variant.ID, "revision": gorm.Expr("revision + 1"), "updated_at": now})
		if result.Error != nil || result.RowsAffected != 1 {
			return domainError(CodeConflict, "Section 已被修改", "刷新后重试。", result.Error)
		}
		if err := appendSectionEvent(tx, section.ID, "image_imported", map[string]any{"section_uuid": section.UUID, "image_variant_uuid": variant.UUID}, now); err != nil {
			return err
		}
		if err := updateComicStateTx(tx, state.ID, now); err != nil {
			return err
		}
		return service.createChapterSnapshotTx(ctx, tx, state.ID, actor.ID, "image_imported")
	})
	if err != nil {
		return ComicSection{}, err
	}
	dto, err := service.GetSection(ctx, chapterUUID, sectionUUID)
	if err == nil {
		service.emit("comic:section_changed", map[string]any{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID})
	}
	return dto, err
}

func (service *Service) CommitGeneratedSectionImage(ctx context.Context, chapterUUID, sectionUUID, generationUUID string, snapshot json.RawMessage, reader filesReader) (ComicSection, error) {
	expectedPageRole, enforcePageRole, err := service.generatedImageSnapshotPageRole(snapshot)
	if err != nil {
		return ComicSection{}, err
	}
	variantUUID, err := newUUIDv7()
	if err != nil {
		return ComicSection{}, err
	}
	_, err = service.files.CommitReader(ctx, files.CommitInput{Purpose: "comic_section_image", OriginalFilename: "generated-section.png", DisplayName: "Generated comic section", SourceType: "generated", Metadata: map[string]any{"section_uuid": sectionUUID, "variant": variantUUID}, Reader: reader, Bind: func(tx *gorm.DB, fileID int64) error {
		state, _, err := service.ensureComicState(ctx, tx, chapterUUID)
		if err != nil {
			return err
		}
		_, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		var section comicSectionRecord
		if err := tx.Where("uuid = ? AND chapter_comic_state_id = ? AND deleted_at IS NULL", sectionUUID, state.ID).First(&section).Error; err != nil {
			return notFound(err, "Comic section 不存在")
		}
		if enforcePageRole && section.PageRole != expectedPageRole {
			return domainError(CodeConflict, "Section 页面角色已变化", "图片生成期间 page_role 已变化，请基于当前页面角色重新发起生成。", nil)
		}
		var generationID *int64
		if generationUUID != "" {
			var generation struct {
				ID       int64
				TaskUUID string
			}
			if err := tx.Table("comic_image_generations").Select("id,task_uuid").Where("uuid = ? AND comic_section_id = ?", generationUUID, section.ID).Scan(&generation).Error; err != nil {
				return err
			}
			if generation.ID > 0 {
				if err := ensureProductionTaskRunning(tx, generation.TaskUUID); err != nil {
					return err
				}
				generationID = &generation.ID
			}
		}
		var version int
		if err := tx.Model(&imageVariantRecord{}).Where("comic_section_id = ?", section.ID).Select("COALESCE(MAX(version_no),0)+1").Scan(&version).Error; err != nil {
			return err
		}
		now := service.now().UTC()
		variant := imageVariantRecord{UUID: variantUUID, ComicSectionID: section.ID, FileID: fileID, ImageGenerationID: generationID, ActorID: actor.ID, VersionNo: version, SourceType: "generated", InputSnapshot: string(snapshot), CreatedAt: now}
		if err := tx.Create(&variant).Error; err != nil {
			return err
		}
		if err := tx.Model(&section).Updates(map[string]any{"current_image_variant_id": variant.ID, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		if generationID != nil {
			_ = tx.Table("comic_image_generations").Where("id = ?", *generationID).Updates(map[string]any{"status": "completed", "completed_at": now}).Error
		}
		if err := appendSectionEvent(tx, section.ID, "image_generated", map[string]any{"section_uuid": section.UUID, "image_variant_uuid": variant.UUID, "generation_uuid": generationUUID}, now); err != nil {
			return err
		}
		if err := updateComicStateTx(tx, state.ID, now); err != nil {
			return err
		}
		return service.createChapterSnapshotTx(ctx, tx, state.ID, actor.ID, "image_generated")
	}})
	if err != nil {
		return ComicSection{}, err
	}
	dto, err := service.GetSection(ctx, chapterUUID, sectionUUID)
	if err == nil {
		service.emit("comic:section_changed", map[string]any{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID})
	}
	return dto, err
}

// generatedImageSnapshotPageRole keeps v1-v4 image tasks compatible: those
// durable snapshots predate page roles and therefore cannot participate in the
// role-drift check. Version 5 and later must freeze one valid page role.
func (service *Service) generatedImageSnapshotPageRole(snapshot json.RawMessage) (string, bool, error) {
	if strings.TrimSpace(string(snapshot)) == "" {
		return "", false, nil
	}
	var frozen GenerationSnapshot
	if err := json.Unmarshal(snapshot, &frozen); err != nil {
		return "", false, domainError(CodeValidation, "图片生成快照已损坏", "input_snapshot 不是有效 JSON。", err)
	}
	if frozen.Version < 5 {
		return "", false, nil
	}
	pageRole, err := service.normalizePageRole(frozen.PageRole, false)
	if err != nil {
		return "", false, domainError(CodeValidation, "图片生成快照页面角色无效", "v5 图片生成快照必须包含有效的 page_role。", err)
	}
	return pageRole, true, nil
}

func (service *Service) ListImageVariants(ctx context.Context, chapterUUID, sectionUUID string) ([]ImageVariant, error) {
	section, err := service.GetSection(ctx, chapterUUID, sectionUUID)
	if err != nil {
		return nil, err
	}
	var row comicSectionRecord
	_ = service.store.DB().WithContext(ctx).Where("uuid = ?", section.UUID).First(&row).Error
	var variants []imageVariantRecord
	if err := service.store.DB().WithContext(ctx).Where("comic_section_id = ?", row.ID).Order("version_no DESC").Find(&variants).Error; err != nil {
		return nil, err
	}
	result := make([]ImageVariant, 0, len(variants))
	for _, item := range variants {
		dto, err := service.imageVariantDTO(ctx, item)
		if err != nil {
			return nil, err
		}
		result = append(result, dto)
	}
	return result, nil
}

func (service *Service) SelectImageVariant(ctx context.Context, chapterUUID, sectionUUID, variantUUID string, expectedRevision int64) (ComicSection, error) {
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, _, err := service.ensureComicState(ctx, tx, chapterUUID)
		if err != nil {
			return err
		}
		_, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		var section comicSectionRecord
		if err := tx.Where("uuid = ? AND chapter_comic_state_id = ? AND deleted_at IS NULL", sectionUUID, state.ID).First(&section).Error; err != nil {
			return notFound(err, "Comic section 不存在")
		}
		var variant imageVariantRecord
		if err := tx.Where("uuid = ? AND comic_section_id = ?", variantUUID, section.ID).First(&variant).Error; err != nil {
			return notFound(err, "图片版本不存在")
		}
		result := tx.Model(&section).Where("revision = ?", expectedRevision).Updates(map[string]any{"current_image_variant_id": variant.ID, "revision": gorm.Expr("revision + 1"), "updated_at": service.now().UTC()})
		if result.Error != nil || result.RowsAffected != 1 {
			return domainError(CodeConflict, "Section 已被修改", "刷新后重试。", result.Error)
		}
		if err := appendSectionEvent(tx, section.ID, "image_selected", map[string]any{"section_uuid": section.UUID, "image_variant_uuid": variant.UUID}, service.now().UTC()); err != nil {
			return err
		}
		if err := updateComicStateTx(tx, state.ID, service.now().UTC()); err != nil {
			return err
		}
		return service.createChapterSnapshotTx(ctx, tx, state.ID, actor.ID, "image_selected")
	})
	if err != nil {
		return ComicSection{}, err
	}
	dto, err := service.GetSection(ctx, chapterUUID, sectionUUID)
	if err == nil {
		service.emit("comic:section_changed", map[string]any{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID})
	}
	return dto, err
}

func (service *Service) ListChapterSnapshots(ctx context.Context, chapterUUID string) ([]ChapterSnapshot, error) {
	state, _, err := service.ensureComicState(ctx, service.store.DB(), chapterUUID)
	if err != nil {
		return nil, err
	}
	var rows []chapterSnapshotRecord
	if err := service.store.DB().WithContext(ctx).Where("chapter_comic_state_id = ?", state.ID).Order("version_no DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]ChapterSnapshot, 0, len(rows))
	for _, row := range rows {
		var payload chapterSnapshotPayload
		_ = json.Unmarshal([]byte(row.SnapshotJSON), &payload)
		result = append(result, chapterSnapshotSummary(row, len(payload.Sections)))
	}
	return result, nil
}

func (service *Service) GetChapterSnapshot(ctx context.Context, chapterUUID, snapshotUUID string) (ChapterSnapshotDetail, error) {
	if !isUUIDv7(snapshotUUID) {
		return ChapterSnapshotDetail{}, domainError(CodeValidation, "快照 UUID 无效", "snapshot_uuid 必须是 UUIDv7。", nil)
	}
	state, chapter, err := service.ensureComicState(ctx, service.store.DB(), chapterUUID)
	if err != nil {
		return ChapterSnapshotDetail{}, err
	}
	var row chapterSnapshotRecord
	if err := service.store.DB().WithContext(ctx).Where("uuid = ? AND chapter_comic_state_id = ?", snapshotUUID, state.ID).First(&row).Error; err != nil {
		return ChapterSnapshotDetail{}, notFound(err, "章节快照不存在")
	}
	var payload chapterSnapshotPayload
	if err := json.Unmarshal([]byte(row.SnapshotJSON), &payload); err != nil {
		return ChapterSnapshotDetail{}, domainError(CodeSnapshotInvalid, "章节快照损坏", "快照详情无法读取。", err)
	}
	if payload.Version < 1 {
		payload.Version = 1
	}
	previewChapter := payload.Chapter
	if previewChapter.UUID == "" {
		previewChapter = ChapterSnapshotChapter{UUID: chapter.UUID, ChapterCode: chapter.ChapterCode, Title: chapter.Title}
	}
	sections := append([]snapshotSection(nil), payload.Sections...)
	sort.SliceStable(sections, func(left, right int) bool {
		leftNo, rightNo := sections[left].SectionNo, sections[right].SectionNo
		if leftNo == rightNo {
			return false
		}
		if leftNo <= 0 {
			return false
		}
		if rightNo <= 0 {
			return true
		}
		return leftNo < rightNo
	})
	detailSections := make([]ChapterSnapshotSection, 0, len(sections))
	for index, item := range sections {
		pageRole, roleErr := service.normalizePageRole(item.PageRole, true)
		if roleErr != nil {
			return ChapterSnapshotDetail{}, domainError(CodeSnapshotInvalid, "章节快照页面角色无效", "快照 page_role 无法读取。", roleErr)
		}
		storyboard := strings.TrimSpace(item.StoryboardMD)
		if storyboard == "" && item.StoryboardUUID != "" {
			var variant storyboardRecord
			if err := service.store.DB().WithContext(ctx).Where("uuid = ?", item.StoryboardUUID).First(&variant).Error; err == nil {
				storyboard = variant.ContentMD
			}
		}
		if storyboard == "" {
			storyboard = item.DescriptionMD
		}
		currentImage, premiseReference := service.resolveSnapshotMedia(ctx, item.ImageVariantUUID)
		sectionNo := item.SectionNo
		if sectionNo <= 0 {
			sectionNo = index + 1
		}
		detailSections = append(detailSections, ChapterSnapshotSection{
			UUID: item.UUID, SectionNo: sectionNo, PageRole: pageRole, Title: item.Title, StoryboardMD: storyboard,
			CurrentImage: currentImage, PremiseReference: premiseReference,
		})
	}
	return ChapterSnapshotDetail{
		ChapterSnapshot: chapterSnapshotSummary(row, len(detailSections)), SchemaVersion: payload.Version,
		Chapter: previewChapter, Sections: detailSections,
	}, nil
}

func chapterSnapshotSummary(row chapterSnapshotRecord, sectionCount int) ChapterSnapshot {
	return ChapterSnapshot{UUID: row.UUID, VersionNo: row.VersionNo, Reason: row.Reason, Source: chapterSnapshotSource(row.Reason), SectionCount: sectionCount, CreatedAt: row.CreatedAt}
}

func chapterSnapshotSource(reason string) string {
	switch reason {
	case "storyboard_generated", "storyboard_overwritten", "image_generated":
		return "generated"
	case "snapshot_restored":
		return "restore"
	default:
		return "manual"
	}
}

func (service *Service) resolveSnapshotMedia(ctx context.Context, imageVariantUUID string) (ChapterSnapshotMedia, ChapterSnapshotMedia) {
	current := ChapterSnapshotMedia{Status: "none", VariantUUID: imageVariantUUID}
	reference := ChapterSnapshotMedia{Status: "none"}
	if imageVariantUUID == "" {
		return current, reference
	}
	var variant imageVariantRecord
	if err := service.store.DB().WithContext(ctx).Where("uuid = ?", imageVariantUUID).First(&variant).Error; err != nil {
		current.Status = "missing"
		return current, reference
	}
	current = service.resolveSnapshotFile(ctx, variant.FileID)
	current.VariantUUID = imageVariantUUID
	if variant.ImageGenerationID == nil {
		return current, reference
	}
	var generation struct{ PremiseFileID *int64 }
	if err := service.store.DB().WithContext(ctx).Table("comic_image_generations").Select("premise_file_id").Where("id = ?", *variant.ImageGenerationID).Scan(&generation).Error; err != nil {
		reference.Status = "missing"
		return current, reference
	}
	if generation.PremiseFileID != nil {
		reference = service.resolveSnapshotFile(ctx, *generation.PremiseFileID)
	}
	return current, reference
}

func (service *Service) resolveSnapshotFile(ctx context.Context, fileID int64) ChapterSnapshotMedia {
	media := ChapterSnapshotMedia{Status: "missing"}
	var row struct {
		UUID      string
		DeletedAt *time.Time
	}
	if err := service.store.DB().WithContext(ctx).Table("files").Select("uuid,deleted_at").Where("id = ?", fileID).Scan(&row).Error; err != nil || row.UUID == "" {
		return media
	}
	media.AssetUUID = row.UUID
	if row.DeletedAt != nil {
		media.Status = "deleted"
		return media
	}
	asset, err := service.files.GetAsset(ctx, row.UUID, false)
	if err != nil {
		media.Status = "unavailable"
		return media
	}
	media.Status = asset.Status
	media.Asset = &asset
	return media
}

func (service *Service) RestoreChapterSnapshot(ctx context.Context, chapterUUID, snapshotUUID string) ([]ComicSection, error) {
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, _, err := service.ensureComicState(ctx, tx, chapterUUID)
		if err != nil {
			return err
		}
		_, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		var snapshot chapterSnapshotRecord
		if err := tx.Where("uuid = ? AND chapter_comic_state_id = ?", snapshotUUID, state.ID).First(&snapshot).Error; err != nil {
			return notFound(err, "章节快照不存在")
		}
		if err := ensureSnapshotRestoreIdle(ctx, tx, state.ID, chapterUUID); err != nil {
			return err
		}
		var value chapterSnapshotPayload
		if err := json.Unmarshal([]byte(snapshot.SnapshotJSON), &value); err != nil {
			return domainError(CodeSnapshotInvalid, "章节快照损坏", "快照无法恢复。", err)
		}
		pageRoles := make([]string, len(value.Sections))
		seenSections := make(map[string]struct{}, len(value.Sections))
		frontCoverCount, bodyCount, backCoverCount := 0, 0, 0
		for index, item := range value.Sections {
			pageRole, roleErr := service.normalizePageRole(item.PageRole, true)
			if roleErr != nil {
				return domainError(CodeSnapshotInvalid, "章节快照页面角色无效", "快照 page_role 无法恢复。", roleErr)
			}
			if _, exists := seenSections[item.UUID]; exists {
				return domainError(CodeSnapshotInvalid, "章节快照包含重复 Section", "同一个 Section 不能在页面序列中出现多次。", nil)
			}
			seenSections[item.UUID] = struct{}{}
			pageRoles[index] = pageRole
			if pageRole == PageRoleFrontCover {
				frontCoverCount++
			}
			if pageRole == PageRoleBody {
				bodyCount++
			}
			if pageRole == PageRoleBackCover {
				backCoverCount++
			}
		}
		if frontCoverCount > 1 || backCoverCount > 1 {
			return domainError(CodeSnapshotInvalid, "章节快照包含重复特殊页面", "快照最多只能包含一个封面和一个封底。", nil)
		}
		if service.protectsLastBodyPage() && bodyCount == 0 {
			return domainError(CodeSnapshotInvalid, "章节快照缺少正文页", "普通绘本快照至少需要包含一个 body 页面。", nil)
		}
		var all []comicSectionRecord
		if err := tx.Where("chapter_comic_state_id = ?", state.ID).Find(&all).Error; err != nil {
			return err
		}
		byUUID := map[string]comicSectionRecord{}
		for _, row := range all {
			byUUID[row.UUID] = row
		}
		now := service.now().UTC()
		if err := tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND deleted_at IS NULL", state.ID).Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		for index, item := range value.Sections {
			row, ok := byUUID[item.UUID]
			if !ok {
				return domainError(CodeSnapshotInvalid, "章节快照引用已永久移除的 Section", "无法安全恢复该快照。", nil)
			}
			updates := map[string]any{"section_no": index + 1, "page_role": pageRoles[index], "title": item.Title, "description_md": item.DescriptionMD, "deleted_at": nil, "revision": gorm.Expr("revision + 1"), "updated_at": now, "current_storyboard_variant_id": nil, "current_image_variant_id": nil}
			if item.StoryboardUUID != "" {
				var variant storyboardRecord
				if err := tx.Where("uuid = ? AND comic_section_id = ?", item.StoryboardUUID, row.ID).First(&variant).Error; err != nil {
					return domainError(CodeSnapshotInvalid, "快照 Storyboard 不存在", "无法安全恢复该快照。", err)
				}
				updates["current_storyboard_variant_id"] = variant.ID
			}
			if item.ImageVariantUUID != "" {
				var variant imageVariantRecord
				if err := tx.Where("uuid = ? AND comic_section_id = ?", item.ImageVariantUUID, row.ID).First(&variant).Error; err != nil {
					return domainError(CodeSnapshotInvalid, "快照图片版本不存在", "无法安全恢复该快照。", err)
				}
				updates["current_image_variant_id"] = variant.ID
			}
			if err := tx.Model(&row).Updates(updates).Error; err != nil {
				return err
			}
		}
		if err := normalizeSectionOrderTx(tx, state.ID, now); err != nil {
			return err
		}
		if err := updateComicStateTx(tx, state.ID, now); err != nil {
			return err
		}
		return service.createChapterSnapshotTx(ctx, tx, state.ID, actor.ID, "snapshot_restored")
	})
	if err != nil {
		return nil, err
	}
	service.emit("comic:snapshot_restored", map[string]any{"chapter_uuid": chapterUUID, "snapshot_uuid": snapshotUUID})
	return service.ListSections(ctx, chapterUUID)
}

type snapshotSection struct {
	UUID             string `json:"uuid"`
	PageRole         string `json:"page_role,omitempty"`
	Title            string `json:"title"`
	DescriptionMD    string `json:"description_md"`
	StoryboardUUID   string `json:"storyboard_uuid,omitempty"`
	StoryboardMD     string `json:"storyboard_md,omitempty"`
	ImageVariantUUID string `json:"image_variant_uuid,omitempty"`
	SectionNo        int    `json:"section_no"`
}
type chapterSnapshotPayload struct {
	Version  int                    `json:"version"`
	Chapter  ChapterSnapshotChapter `json:"chapter,omitempty"`
	Sections []snapshotSection      `json:"sections"`
}

func (service *Service) createChapterSnapshotTx(ctx context.Context, tx *gorm.DB, stateID, actorID int64, reason string) error {
	var rows []comicSectionRecord
	if err := tx.WithContext(ctx).Where("chapter_comic_state_id = ? AND deleted_at IS NULL", stateID).Order("section_no").Find(&rows).Error; err != nil {
		return err
	}
	var chapter ChapterSnapshotChapter
	if err := tx.WithContext(ctx).Table("chapters AS chapters").Select("chapters.uuid,chapters.chapter_code,chapters.title").Joins("JOIN chapter_comic_states AS states ON states.chapter_id=chapters.id").Where("states.id = ?", stateID).Scan(&chapter).Error; err != nil {
		return err
	}
	payload := chapterSnapshotPayload{Version: 3, Chapter: chapter, Sections: make([]snapshotSection, 0, len(rows))}
	for _, row := range rows {
		item := snapshotSection{UUID: row.UUID, SectionNo: row.SectionNo, PageRole: row.PageRole, Title: row.Title, DescriptionMD: row.DescriptionMD}
		if row.CurrentStoryboardVariantID != nil {
			var storyboard storyboardRecord
			if err := tx.Where("id = ?", *row.CurrentStoryboardVariantID).First(&storyboard).Error; err != nil {
				return err
			}
			item.StoryboardUUID, item.StoryboardMD = storyboard.UUID, storyboard.ContentMD
		}
		if row.CurrentImageVariantID != nil {
			_ = tx.Table("comic_image_variants").Where("id = ?", *row.CurrentImageVariantID).Pluck("uuid", &item.ImageVariantUUID).Error
		}
		payload.Sections = append(payload.Sections, item)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var version int
	if err := tx.Model(&chapterSnapshotRecord{}).Where("chapter_comic_state_id = ?", stateID).Select("COALESCE(MAX(version_no),0)+1").Scan(&version).Error; err != nil {
		return err
	}
	uuid, err := newUUIDv7()
	if err != nil {
		return err
	}
	return tx.Create(&chapterSnapshotRecord{UUID: uuid, ChapterComicStateID: stateID, ActorID: actorID, VersionNo: version, Reason: reason, SnapshotJSON: string(encoded), SnapshotHash: hashJSON(encoded), CreatedAt: service.now().UTC()}).Error
}

func ensureSnapshotRestoreIdle(ctx context.Context, tx *gorm.DB, stateID int64, chapterUUID string) error {
	var activeCount int64
	if err := tx.WithContext(ctx).Raw(`SELECT
		(SELECT COUNT(*) FROM task_runs WHERE resource_uuid=? AND kind IN ('story_chapter_generation','comic_storyboard_generation') AND status IN ('queued','running')) +
		(SELECT COUNT(*) FROM production_task_runs AS tasks JOIN comic_sections AS sections ON sections.uuid=tasks.resource_uuid WHERE sections.chapter_comic_state_id=? AND tasks.kind='comic_image_generation' AND tasks.status IN ('queued','running')) +
		(SELECT COUNT(*) FROM workflows WHERE project_id=(SELECT chapters.project_id FROM chapter_comic_states JOIN chapters ON chapters.id=chapter_comic_states.chapter_id WHERE chapter_comic_states.id=?) AND kind='yolo_project_initialization' AND status IN ('queued','running','interrupted'))`, chapterUUID, stateID, stateID).Scan(&activeCount).Error; err != nil {
		return err
	}
	if activeCount > 0 {
		return domainError(CodeSnapshotBusy, "章节正在生成，无法恢复快照", "请等待 Yolo、章节正文、漫画脚本或页面图片生成任务结束后再恢复。", nil)
	}
	return nil
}

func ensureSpecialPageRoleAvailableTx(tx *gorm.DB, stateID int64, pageRole string, excludeSectionID int64) error {
	if pageRole != PageRoleFrontCover && pageRole != PageRoleBackCover {
		return nil
	}
	query := tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND page_role = ? AND deleted_at IS NULL", stateID, pageRole)
	if excludeSectionID > 0 {
		query = query.Where("id <> ?", excludeSectionID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return domainError(CodeConflict, "特殊页面已存在", fmt.Sprintf("当前绘本最多只能有一个 active %s 页面。", pageRole), nil)
	}
	return nil
}

func ensureBodyPageRemainsTx(tx *gorm.DB, stateID, excludedSectionID int64) error {
	var count int64
	if err := tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND page_role = ? AND deleted_at IS NULL AND id <> ?", stateID, PageRoleBody, excludedSectionID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return domainError(CodeConflict, "绘本必须保留正文页", "已有正文页的绘本至少需要保留一个 active body 页面。", nil)
	}
	return nil
}

func ensureBodyPageExistsTx(tx *gorm.DB, stateID int64) error {
	var count int64
	if err := tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND page_role = ? AND deleted_at IS NULL", stateID, PageRoleBody).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return domainError(CodeValidation, "请先创建正文页", "普通绘本创建封面或封底前，至少需要一个 active body 页面。", nil)
	}
	return nil
}

func pageRoleOrder(pageRole string) int {
	switch pageRole {
	case PageRoleFrontCover:
		return 0
	case PageRoleBackCover:
		return 2
	default:
		return 1
	}
}

func normalizeSectionOrderTx(tx *gorm.DB, stateID int64, now time.Time) error {
	var rows []comicSectionRecord
	if err := tx.Where("chapter_comic_state_id = ? AND deleted_at IS NULL", stateID).Order("section_no,id").Find(&rows).Error; err != nil {
		return err
	}
	sort.SliceStable(rows, func(left, right int) bool {
		return pageRoleOrder(rows[left].PageRole) < pageRoleOrder(rows[right].PageRole)
	})
	return writeSectionOrderTx(tx, rows, nil, now)
}

func writeSectionOrderTx(tx *gorm.DB, rows []comicSectionRecord, incrementRevision map[int64]struct{}, now time.Time) error {
	maxSectionNo := 0
	for _, row := range rows {
		if row.SectionNo > maxSectionNo {
			maxSectionNo = row.SectionNo
		}
	}
	temporaryStart := maxSectionNo + len(rows) + 1
	for index, row := range rows {
		if err := tx.Model(&comicSectionRecord{}).Where("id = ?", row.ID).Updates(map[string]any{"section_no": temporaryStart + index, "updated_at": now}).Error; err != nil {
			return err
		}
	}
	for index, row := range rows {
		updates := map[string]any{"section_no": index + 1, "updated_at": now}
		if _, ok := incrementRevision[row.ID]; ok {
			updates["revision"] = gorm.Expr("revision + 1")
		}
		if err := tx.Model(&comicSectionRecord{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

func createStoryboardTx(tx *gorm.DB, section comicSectionRecord, actorID int64, content, sourceType string, now time.Time) (storyboardRecord, error) {
	var version int
	if err := tx.Model(&storyboardRecord{}).Where("comic_section_id = ?", section.ID).Select("COALESCE(MAX(version_no),0)+1").Scan(&version).Error; err != nil {
		return storyboardRecord{}, err
	}
	uuid, err := newUUIDv7()
	if err != nil {
		return storyboardRecord{}, err
	}
	record := storyboardRecord{UUID: uuid, ComicSectionID: section.ID, ActorID: actorID, VersionNo: version, ContentMD: strings.TrimSpace(content), SourceType: sourceType, CreatedAt: now}
	return record, tx.Create(&record).Error
}
func updateComicStateTx(tx *gorm.DB, stateID int64, now time.Time) error {
	var sections, bodySections, storyboards, images int64
	if err := tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND deleted_at IS NULL", stateID).Count(&sections).Error; err != nil {
		return err
	}
	if err := tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND page_role = ? AND deleted_at IS NULL", stateID, PageRoleBody).Count(&bodySections).Error; err != nil {
		return err
	}
	if err := tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND deleted_at IS NULL AND current_storyboard_variant_id IS NOT NULL", stateID).Count(&storyboards).Error; err != nil {
		return err
	}
	if err := tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND deleted_at IS NULL AND current_image_variant_id IS NOT NULL", stateID).Count(&images).Error; err != nil {
		return err
	}
	status := "empty"
	if sections > 0 {
		status = "draft"
	}
	if bodySections > 0 && storyboards == sections {
		status = "storyboarded"
	}
	if bodySections > 0 && images == sections {
		status = "ready"
	}
	return tx.Model(&comicStateRecord{}).Where("id = ?", stateID).Updates(map[string]any{"status": status, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error
}
func appendSectionEvent(tx *gorm.DB, sectionID int64, eventType string, payload any, now time.Time) error {
	uuid, err := newUUIDv7()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Exec("INSERT INTO comic_section_events (uuid,comic_section_id,sequence,event_type,payload,created_at) SELECT ?,?,COALESCE(MAX(sequence),0)+1,?,?,? FROM comic_section_events WHERE comic_section_id=?", uuid, sectionID, eventType, string(encoded), now, sectionID).Error
}
func storyboardDTO(row storyboardRecord) StoryboardVariant {
	return StoryboardVariant{UUID: row.UUID, VersionNo: row.VersionNo, ContentMD: row.ContentMD, SourceType: row.SourceType, CreatedAt: row.CreatedAt}
}
func (service *Service) imageVariantDTO(ctx context.Context, row imageVariantRecord) (ImageVariant, error) {
	var fileUUID string
	if err := service.store.DB().WithContext(ctx).Table("files").Where("id = ?", row.FileID).Pluck("uuid", &fileUUID).Error; err != nil {
		return ImageVariant{}, err
	}
	asset, err := service.files.GetAsset(ctx, fileUUID, false)
	if err != nil {
		return ImageVariant{}, err
	}
	var generationUUID string
	var generationSummary *ImageGenerationSummary
	var sectionPremise *SectionPremise
	if row.ImageGenerationID != nil {
		var generation struct {
			ID              int64
			UUID            string
			Status          string
			InputSnapshot   string
			ComicSectionID  int64
			PremiseFileID   *int64
			PremiseMetadata string
		}
		if err := service.store.DB().WithContext(ctx).Table("comic_image_generations").Select("id,uuid,status,input_snapshot,comic_section_id,premise_file_id,premise_metadata").Where("id = ?", *row.ImageGenerationID).Scan(&generation).Error; err != nil {
			return ImageVariant{}, err
		}
		generationUUID = generation.UUID
		var snapshot GenerationSnapshot
		if strings.TrimSpace(generation.InputSnapshot) != "" {
			if err := json.Unmarshal([]byte(generation.InputSnapshot), &snapshot); err != nil {
				return ImageVariant{}, domainError(CodeValidation, "图片生成快照已损坏", "input_snapshot 不是有效 JSON。", err)
			}
		}
		generationSummary = &ImageGenerationSummary{UUID: generation.UUID, Status: generation.Status, ProviderUUID: snapshot.ProviderUUID, ProviderType: snapshot.ProviderType, Model: snapshot.Model, ModelSource: snapshot.ModelSource}
		sectionPremise, err = service.sectionPremiseDTO(ctx, generation.PremiseFileID, generation.PremiseMetadata)
		if err != nil {
			return ImageVariant{}, err
		}
	}
	return ImageVariant{UUID: row.UUID, VersionNo: row.VersionNo, SourceType: row.SourceType, GenerationUUID: generationUUID, InputSnapshot: json.RawMessage(row.InputSnapshot), Generation: generationSummary, Asset: asset, SectionPremise: sectionPremise, CreatedAt: row.CreatedAt}, nil
}
func (service *Service) sectionDTO(ctx context.Context, row comicSectionRecord, chapterUUID string) (ComicSection, error) {
	result := ComicSection{UUID: row.UUID, ChapterUUID: chapterUUID, SectionNo: row.SectionNo, PageRole: row.PageRole, Title: row.Title, DescriptionMD: row.DescriptionMD, PremiseAssets: []PremiseAssetReference{}, Revision: row.Revision, DeletedAt: row.DeletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	var references []struct {
		AssetUUID, VariantUUID, FileUUID, Title string
	}
	if err := service.store.DB().WithContext(ctx).
		Table("comic_section_premise_asset_selections AS selections").
		Select("assets.uuid AS asset_uuid, variants.uuid AS variant_uuid, files.uuid AS file_uuid, assets.title").
		Joins("JOIN premise_assets AS assets ON assets.id=selections.premise_asset_id AND assets.deleted_at IS NULL").
		Joins("JOIN premise_asset_variants AS variants ON variants.id=assets.current_variant_id").
		Joins("JOIN files ON files.id=variants.file_id AND files.deleted_at IS NULL").
		Where("selections.comic_section_id = ?", row.ID).
		Order("selections.sort_order ASC").Scan(&references).Error; err != nil {
		return result, err
	}
	for _, reference := range references {
		result.PremiseAssets = append(result.PremiseAssets, PremiseAssetReference{AssetUUID: reference.AssetUUID, VariantUUID: reference.VariantUUID, FileUUID: reference.FileUUID, Title: reference.Title})
	}
	if row.CurrentStoryboardVariantID != nil {
		var variant storyboardRecord
		if err := service.store.DB().WithContext(ctx).First(&variant, *row.CurrentStoryboardVariantID).Error; err == nil {
			dto := storyboardDTO(variant)
			result.CurrentStoryboard = &dto
		}
	}
	if row.CurrentImageVariantID != nil {
		var variant imageVariantRecord
		if err := service.store.DB().WithContext(ctx).First(&variant, *row.CurrentImageVariantID).Error; err == nil {
			dto, err := service.imageVariantDTO(ctx, variant)
			if err != nil {
				return result, err
			}
			result.CurrentImage = &dto
		}
	}
	return result, nil
}
