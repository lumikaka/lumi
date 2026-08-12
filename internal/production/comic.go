package production

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"lumi/internal/files"

	"gorm.io/gorm"
)

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
	Title, DescriptionMD                              string
	CurrentStoryboardVariantID, CurrentImageVariantID *int64
	Revision                                          int64
	DeletedAt                                         *time.Time
	CreatedAt, UpdatedAt                              time.Time
}

func (comicSectionRecord) TableName() string { return "comic_sections" }

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
	state, chapter, err := service.ensureComicState(ctx, service.store.DB(), chapterUUID)
	if err != nil {
		return nil, err
	}
	var rows []comicSectionRecord
	if err := service.store.DB().WithContext(ctx).Where("chapter_comic_state_id = ? AND deleted_at IS NULL", state.ID).Order("section_no ASC").Find(&rows).Error; err != nil {
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
		var max int
		if err := tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND deleted_at IS NULL", state.ID).Select("COALESCE(MAX(section_no),0)").Scan(&max).Error; err != nil {
			return err
		}
		now := service.now().UTC()
		row = comicSectionRecord{UUID: uuid, ChapterComicStateID: state.ID, ActorID: actor.ID, SectionNo: max + 1, Title: title, DescriptionMD: description, CreatedAt: now, UpdatedAt: now}
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
		if err := updateComicStateTx(tx, state.ID, now); err != nil {
			return err
		}
		if err := appendSectionEvent(tx, row.ID, "section_created", map[string]any{"section_uuid": row.UUID, "section_no": row.SectionNo}, now); err != nil {
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
	if len(generated) < 1 || len(generated) > 6 {
		return nil, domainError(CodeValidation, "Comic storyboard 数量无效", "sections 必须包含 1 到 6 项。", nil)
	}
	for index := range generated {
		generated[index].Title = strings.TrimSpace(generated[index].Title)
		generated[index].StoryboardMD = strings.TrimSpace(generated[index].StoryboardMD)
		if generated[index].Title == "" || generated[index].StoryboardMD == "" || len([]rune(generated[index].Title)) > 160 || len([]rune(generated[index].StoryboardMD)) > 262144 {
			return nil, domainError(CodeValidation, "Comic storyboard 内容无效", "每个 section 都需要有效 title 和 storyboard。", nil)
		}
	}
	var existing []ComicSection
	existing, err := service.ListSections(ctx, chapterUUID)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		if len(existing) != len(generated) {
			return nil, domainError(CodeConflict, "章节已有 Comic Sections", "为避免覆盖手工内容，AI storyboard 只能写入空章节。", nil)
		}
		for index, section := range existing {
			if section.Title != generated[index].Title || section.CurrentStoryboard == nil || strings.TrimSpace(section.CurrentStoryboard.ContentMD) != generated[index].StoryboardMD {
				return nil, domainError(CodeConflict, "章节已有不同 Comic Sections", "现有分镜与任务快照结果不同，未执行覆盖。", nil)
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
		for index, item := range generated {
			sectionUUID, uuidErr := newUUIDv7()
			if uuidErr != nil {
				return uuidErr
			}
			row := comicSectionRecord{UUID: sectionUUID, ChapterComicStateID: state.ID, ActorID: actor.ID, SectionNo: index + 1, Title: item.Title, CreatedAt: now, UpdatedAt: now}
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
			if err := appendSectionEvent(tx, row.ID, "section_created", map[string]any{"section_uuid": row.UUID, "section_no": row.SectionNo, "source_type": "generated"}, now); err != nil {
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
	items, err := service.ListSections(ctx, chapterUUID)
	if err == nil {
		service.emit("comic:section_changed", map[string]any{"chapter_uuid": chapterUUID, "generated": true})
	}
	return items, err
}

func (service *Service) UpdateSection(ctx context.Context, chapterUUID, sectionUUID string, input UpdateSectionInput) (ComicSection, error) {
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
		result := tx.Model(&row).Where("revision = ?", input.ExpectedRevision).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainError(CodeConflict, "Section 已被修改", "刷新后重试。", nil)
		}
		if err := appendSectionEvent(tx, row.ID, "section_updated", map[string]any{"section_uuid": row.UUID}, service.now().UTC()); err != nil {
			return err
		}
		if err := updateComicStateTx(tx, state.ID, service.now().UTC()); err != nil {
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
		now := service.now().UTC()
		result := tx.Model(&row).Where("revision = ?", expectedRevision).Updates(map[string]any{"deleted_at": now, "revision": gorm.Expr("revision + 1"), "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainError(CodeConflict, "Section 已被修改", "刷新后重试。", nil)
		}
		if err := compactSectionOrderTx(tx, state.ID); err != nil {
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
		if len(rows) != len(orderedUUIDs) {
			return domainError(CodeValidation, "Section 顺序不完整", "必须提供当前章节全部 active section UUID。", nil)
		}
		byUUID := map[string]comicSectionRecord{}
		for _, row := range rows {
			byUUID[row.UUID] = row
		}
		for _, uuid := range orderedUUIDs {
			if _, ok := byUUID[uuid]; !ok {
				return domainError(CodeValidation, "Section 顺序包含未知 UUID", "只能排序当前章节的 active section。", nil)
			}
		}
		now := service.now().UTC()
		for index, uuid := range orderedUUIDs {
			if err := tx.Model(&comicSectionRecord{}).Where("id = ?", byUUID[uuid].ID).Updates(map[string]any{"section_no": 1_000_000 + index, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		for index, uuid := range orderedUUIDs {
			row := byUUID[uuid]
			if err := tx.Model(&comicSectionRecord{}).Where("id = ?", row.ID).Updates(map[string]any{"section_no": index + 1, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
				return err
			}
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
			UUID: item.UUID, SectionNo: sectionNo, Title: item.Title, StoryboardMD: storyboard,
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
	case "storyboard_generated", "image_generated":
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
			updates := map[string]any{"section_no": index + 1, "title": item.Title, "description_md": item.DescriptionMD, "deleted_at": nil, "revision": gorm.Expr("revision + 1"), "updated_at": now, "current_storyboard_variant_id": nil, "current_image_variant_id": nil}
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
	payload := chapterSnapshotPayload{Version: 2, Chapter: chapter, Sections: make([]snapshotSection, 0, len(rows))}
	for _, row := range rows {
		item := snapshotSection{UUID: row.UUID, SectionNo: row.SectionNo, Title: row.Title, DescriptionMD: row.DescriptionMD}
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
		(SELECT COUNT(*) FROM production_task_runs AS tasks JOIN comic_sections AS sections ON sections.uuid=tasks.resource_uuid WHERE sections.chapter_comic_state_id=? AND tasks.kind='comic_image_generation' AND tasks.status IN ('queued','running'))`, chapterUUID, stateID).Scan(&activeCount).Error; err != nil {
		return err
	}
	if activeCount > 0 {
		return domainError(CodeSnapshotBusy, "章节正在生成，无法恢复快照", "请等待章节正文、漫画脚本或 Section 图片生成任务结束后再恢复。", nil)
	}
	return nil
}

func compactSectionOrderTx(tx *gorm.DB, stateID int64) error {
	var rows []comicSectionRecord
	if err := tx.Where("chapter_comic_state_id = ? AND deleted_at IS NULL", stateID).Order("section_no").Find(&rows).Error; err != nil {
		return err
	}
	for index, row := range rows {
		if err := tx.Model(&row).Update("section_no", 1_000_000+index).Error; err != nil {
			return err
		}
	}
	for index, row := range rows {
		if err := tx.Model(&row).Update("section_no", index+1).Error; err != nil {
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
	var sections, storyboards, images int64
	tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND deleted_at IS NULL", stateID).Count(&sections)
	tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND deleted_at IS NULL AND current_storyboard_variant_id IS NOT NULL", stateID).Count(&storyboards)
	tx.Model(&comicSectionRecord{}).Where("chapter_comic_state_id = ? AND deleted_at IS NULL AND current_image_variant_id IS NOT NULL", stateID).Count(&images)
	status := "empty"
	if sections > 0 {
		status = "draft"
	}
	if sections > 0 && storyboards == sections {
		status = "storyboarded"
	}
	if sections > 0 && images == sections {
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
	result := ComicSection{UUID: row.UUID, ChapterUUID: chapterUUID, SectionNo: row.SectionNo, Title: row.Title, DescriptionMD: row.DescriptionMD, Revision: row.Revision, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
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
