package story

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type CreateChapterInput struct {
	ChapterCode   string
	Title         string
	Content       string
	ContentFormat string
}

func (service *Service) CreateChapter(ctx context.Context, input CreateChapterInput) (Chapter, error) {
	code, volume, chapterNo, sortOrder, err := parseChapterCode(input.ChapterCode)
	if err != nil {
		return Chapter{}, err
	}
	input.Title = strings.TrimSpace(input.Title)
	if len([]rune(input.Title)) > 255 {
		return Chapter{}, storyError(CodeValidationFailed, "章节标题过长", "章节标题不能超过 255 个字符。", nil)
	}
	format := "txt"
	if input.Content != "" {
		if err := validateText(input.Content, maxChapterBytes, "章节正文"); err != nil {
			return Chapter{}, err
		}
		format, err = normalizeFormat(input.ContentFormat)
		if err != nil {
			return Chapter{}, err
		}
	}
	projectRecord, actor, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return Chapter{}, err
	}
	chapterUUID, err := newUUIDv7()
	if err != nil {
		return Chapter{}, err
	}
	now := service.now().UTC()
	record := chapterRecord{UUID: chapterUUID, ProjectID: projectRecord.ID, VolumeNo: volume, ChapterNo: chapterNo, ChapterCode: code, SortOrder: sortOrder, Title: input.Title, Revision: 0, CreatedAt: now, UpdatedAt: now}
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		if input.Content == "" {
			return nil
		}
		requestHash, err := randomRequestHash()
		if err != nil {
			return err
		}
		source, item, err := service.sourceAndItem(ctx, tx, projectRecord.ID, actor.ID, record.ID, 1, "manual_entry", requestHash, "", format, input.Content)
		if err != nil {
			return err
		}
		storyUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		story := chapterStoryRecord{UUID: storyUUID, ChapterID: record.ID, ActorID: actor.ID, StorySourceID: source.ID, StorySourceItemID: item.ID, VersionNo: 1, SourceType: "manual_entry", Content: input.Content, ContentFormat: format, ContentHash: contentHash(input.Content), CharCount: len([]rune(input.Content)), CreatedAt: now}
		if err := tx.Create(&story).Error; err != nil {
			return err
		}
		return tx.Model(&record).Updates(map[string]any{"current_story_id": story.ID, "revision": 1, "updated_at": now}).Error
	})
	if err != nil {
		if uniqueConflict(err) {
			return Chapter{}, storyError(CodeChapterConflict, "章节编号冲突", "同一项目中的 active 章节编号和排序必须唯一。", err)
		}
		return Chapter{}, err
	}
	return service.GetChapter(ctx, chapterUUID)
}

func (service *Service) findChapter(ctx context.Context, uuid string, includeTrashed bool) (chapterRecord, error) {
	if !isUUIDv7(uuid) {
		return chapterRecord{}, storyError(CodeValidationFailed, "章节 UUID 无效", "章节资源标识必须是 UUIDv7。", nil)
	}
	projectRecord, _, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return chapterRecord{}, err
	}
	query := service.store.DB().WithContext(ctx).Where("project_id = ? AND uuid = ?", projectRecord.ID, uuid)
	if !includeTrashed {
		query = query.Where("deleted_at IS NULL")
	}
	var record chapterRecord
	err = query.First(&record).Error
	if err != nil {
		return record, recordNotFound(err, CodeChapterNotFound, "章节不存在", "该章节不存在或已经进入回收站。")
	}
	return record, nil
}

func (service *Service) storyDTO(ctx context.Context, id int64) (*ChapterStory, error) {
	var record chapterStoryRecord
	if err := service.store.DB().WithContext(ctx).First(&record, id).Error; err != nil {
		return nil, err
	}
	var source storySourceRecord
	if err := service.store.DB().WithContext(ctx).First(&source, record.StorySourceID).Error; err != nil {
		return nil, err
	}
	var item storySourceItemRecord
	if err := service.store.DB().WithContext(ctx).First(&item, record.StorySourceItemID).Error; err != nil {
		return nil, err
	}
	return &ChapterStory{UUID: record.UUID, VersionNo: record.VersionNo, SourceType: record.SourceType, SourceUUID: source.UUID, SourceItemUUID: item.UUID, Content: record.Content, ContentFormat: record.ContentFormat, ContentHash: record.ContentHash, CharCount: record.CharCount, CreatedAt: record.CreatedAt}, nil
}

func (service *Service) chapterDTO(ctx context.Context, record chapterRecord) (Chapter, error) {
	var current *ChapterStory
	if record.CurrentStoryID != nil {
		story, err := service.storyDTO(ctx, *record.CurrentStoryID)
		if err != nil {
			return Chapter{}, err
		}
		current = story
	}
	return Chapter{UUID: record.UUID, ChapterCode: record.ChapterCode, VolumeNo: record.VolumeNo, ChapterNo: record.ChapterNo, SortOrder: record.SortOrder, Title: record.Title, Revision: record.Revision, TrashedAt: record.DeletedAt, CurrentStory: current, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, nil
}

func (service *Service) GetChapter(ctx context.Context, uuid string) (Chapter, error) {
	record, err := service.findChapter(ctx, uuid, true)
	if err != nil {
		return Chapter{}, err
	}
	return service.chapterDTO(ctx, record)
}

func (service *Service) ListChapters(ctx context.Context, state string) ([]Chapter, error) {
	projectRecord, _, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return nil, err
	}
	query := service.store.DB().WithContext(ctx).Where("project_id = ?", projectRecord.ID)
	switch state {
	case "", "active":
		query = query.Where("deleted_at IS NULL").Order("sort_order ASC")
	case "trashed":
		query = query.Where("deleted_at IS NOT NULL").Order("deleted_at DESC, sort_order ASC")
	default:
		return nil, storyError(CodeValidationFailed, "章节列表状态无效", "state 只支持 active 或 trashed。", nil)
	}
	var records []chapterRecord
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	items := make([]Chapter, 0, len(records))
	for _, record := range records {
		item, err := service.chapterDTO(ctx, record)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

type UpdateChapterInput struct {
	Title            string
	ExpectedRevision int64
}

func (service *Service) UpdateChapter(ctx context.Context, uuid string, input UpdateChapterInput) (Chapter, error) {
	input.Title = strings.TrimSpace(input.Title)
	if len([]rune(input.Title)) > 255 {
		return Chapter{}, storyError(CodeValidationFailed, "章节标题过长", "章节标题不能超过 255 个字符。", nil)
	}
	record, err := service.findChapter(ctx, uuid, false)
	if err != nil {
		return Chapter{}, err
	}
	result := service.store.DB().WithContext(ctx).Model(&chapterRecord{}).
		Where("id = ? AND revision = ? AND deleted_at IS NULL", record.ID, input.ExpectedRevision).
		Updates(map[string]any{"title": input.Title, "revision": gorm.Expr("revision + 1"), "updated_at": service.now().UTC()})
	if result.Error != nil {
		return Chapter{}, result.Error
	}
	if result.RowsAffected != 1 {
		return Chapter{}, storyError(CodeChapterRevisionConflict, "章节版本冲突", "章节已被其他操作更新，请刷新后重试。", nil)
	}
	return service.GetChapter(ctx, uuid)
}

type UpdateStoryInput struct {
	Content          string
	ContentFormat    string
	ExpectedRevision int64
}

func (service *Service) UpdateStory(ctx context.Context, chapterUUID string, input UpdateStoryInput) (Chapter, error) {
	if err := validateText(input.Content, maxChapterBytes, "章节正文"); err != nil {
		return Chapter{}, err
	}
	format, err := normalizeFormat(input.ContentFormat)
	if err != nil {
		return Chapter{}, err
	}
	record, err := service.findChapter(ctx, chapterUUID, false)
	if err != nil {
		return Chapter{}, err
	}
	if record.Revision != input.ExpectedRevision {
		return Chapter{}, storyError(CodeChapterRevisionConflict, "正文版本冲突", "章节已被其他操作更新，请保留本地草稿并刷新后重试。", nil)
	}
	if record.CurrentStoryID != nil {
		current, err := service.storyDTO(ctx, *record.CurrentStoryID)
		if err != nil {
			return Chapter{}, err
		}
		if current.ContentHash == contentHash(input.Content) && current.ContentFormat == format {
			return service.chapterDTO(ctx, record)
		}
	}
	projectRecord, actor, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return Chapter{}, err
	}
	now := service.now().UTC()
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked chapterRecord
		if err := tx.Where("id = ? AND deleted_at IS NULL", record.ID).First(&locked).Error; err != nil {
			return err
		}
		if locked.Revision != input.ExpectedRevision {
			return storyError(CodeChapterRevisionConflict, "正文版本冲突", "章节已被其他操作更新，请刷新后重试。", nil)
		}
		requestHash, err := randomRequestHash()
		if err != nil {
			return err
		}
		source, item, err := service.sourceAndItem(ctx, tx, projectRecord.ID, actor.ID, locked.ID, 1, "manual_edit", requestHash, "", format, input.Content)
		if err != nil {
			return err
		}
		var versionNo int
		if err := tx.Model(&chapterStoryRecord{}).Where("chapter_id = ?", locked.ID).Select("COALESCE(MAX(version_no), 0)").Scan(&versionNo).Error; err != nil {
			return err
		}
		storyUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		story := chapterStoryRecord{UUID: storyUUID, ChapterID: locked.ID, ActorID: actor.ID, StorySourceID: source.ID, StorySourceItemID: item.ID, VersionNo: versionNo + 1, SourceType: "manual_edit", Content: input.Content, ContentFormat: format, ContentHash: contentHash(input.Content), CharCount: len([]rune(input.Content)), CreatedAt: now}
		if err := tx.Create(&story).Error; err != nil {
			return err
		}
		result := tx.Model(&chapterRecord{}).Where("id = ? AND revision = ? AND deleted_at IS NULL", locked.ID, input.ExpectedRevision).Updates(map[string]any{"current_story_id": story.ID, "revision": gorm.Expr("revision + 1"), "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return storyError(CodeChapterRevisionConflict, "正文版本冲突", "章节已被其他操作更新，请刷新后重试。", nil)
		}
		return nil
	})
	if err != nil {
		return Chapter{}, err
	}
	return service.GetChapter(ctx, chapterUUID)
}

func (service *Service) ListChapterStories(ctx context.Context, chapterUUID string) ([]ChapterStory, error) {
	record, err := service.findChapter(ctx, chapterUUID, true)
	if err != nil {
		return nil, err
	}
	var records []chapterStoryRecord
	if err := service.store.DB().WithContext(ctx).Where("chapter_id = ?", record.ID).Order("version_no DESC").Limit(200).Find(&records).Error; err != nil {
		return nil, err
	}
	items := make([]ChapterStory, 0, len(records))
	for _, storyRecord := range records {
		item, err := service.storyDTO(ctx, storyRecord.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, nil
}

func (service *Service) TrashChapter(ctx context.Context, uuid string, expectedRevision int64) (Chapter, error) {
	record, err := service.findChapter(ctx, uuid, true)
	if err != nil {
		return Chapter{}, err
	}
	if record.DeletedAt != nil {
		return Chapter{}, storyError(CodeChapterStateConflict, "章节已在回收站", "无需重复回收该章节。", nil)
	}
	now := service.now().UTC()
	result := service.store.DB().WithContext(ctx).Model(&chapterRecord{}).Where("id = ? AND revision = ? AND deleted_at IS NULL", record.ID, expectedRevision).Updates(map[string]any{"deleted_at": now, "revision": gorm.Expr("revision + 1"), "updated_at": now})
	if result.Error != nil {
		return Chapter{}, result.Error
	}
	if result.RowsAffected != 1 {
		return Chapter{}, storyError(CodeChapterRevisionConflict, "回收操作版本冲突", "章节已被其他操作更新，请刷新后重试。", nil)
	}
	return service.GetChapter(ctx, uuid)
}

func (service *Service) RestoreChapter(ctx context.Context, uuid string, expectedRevision int64) (Chapter, error) {
	record, err := service.findChapter(ctx, uuid, true)
	if err != nil {
		return Chapter{}, err
	}
	if record.DeletedAt == nil {
		return Chapter{}, storyError(CodeChapterStateConflict, "章节不在回收站", "只有回收站中的章节可以恢复。", nil)
	}
	now := service.now().UTC()
	result := service.store.DB().WithContext(ctx).Model(&chapterRecord{}).Where("id = ? AND revision = ? AND deleted_at IS NOT NULL", record.ID, expectedRevision).Updates(map[string]any{"deleted_at": nil, "revision": gorm.Expr("revision + 1"), "updated_at": now})
	if result.Error != nil {
		if uniqueConflict(result.Error) {
			return Chapter{}, storyError(CodeChapterConflict, "无法恢复章节", "active 章节中已有相同编号或排序，请先处理冲突。", result.Error)
		}
		return Chapter{}, result.Error
	}
	if result.RowsAffected != 1 {
		return Chapter{}, storyError(CodeChapterRevisionConflict, "恢复操作版本冲突", "章节已被其他操作更新，请刷新后重试。", nil)
	}
	return service.GetChapter(ctx, uuid)
}

func (service *Service) PermanentlyDeleteChapter(ctx context.Context, uuid string, expectedRevision int64) error {
	record, err := service.findChapter(ctx, uuid, true)
	if err != nil {
		return err
	}
	if record.DeletedAt == nil {
		return storyError(CodeChapterStateConflict, "章节尚未进入回收站", "永久删除前必须先把章节移入回收站。", nil)
	}
	return service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		blocked, err := chapterDeleteBlocked(ctx, tx, record)
		if err != nil {
			return err
		}
		if blocked {
			return storyError(CodeChapterDeleteBlocked, "章节仍被活动任务使用", "等待相关生成或会话结束后重试。", nil)
		}
		return deleteChapterTx(tx, record, &expectedRevision)
	})
}

func (service *Service) EmptyChapterTrash(ctx context.Context) (EmptyChapterTrashResult, error) {
	projectRecord, _, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return EmptyChapterTrashResult{}, err
	}
	result := EmptyChapterTrashResult{BlockedItems: []ChapterTrashBlockedItem{}}
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []chapterRecord
		if err := tx.Where("project_id = ? AND deleted_at IS NOT NULL", projectRecord.ID).Order("deleted_at ASC,id ASC").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			blocked, err := chapterDeleteBlocked(ctx, tx, row)
			if err != nil {
				return err
			}
			if blocked {
				result.BlockedItems = append(result.BlockedItems, ChapterTrashBlockedItem{UUID: row.UUID, ChapterCode: row.ChapterCode, ErrorCode: CodeChapterDeleteBlocked})
				continue
			}
			if err := deleteChapterTx(tx, row, nil); err != nil {
				return err
			}
			result.DeletedCount++
		}
		return nil
	})
	return result, err
}

func deleteChapterTx(tx *gorm.DB, record chapterRecord, expectedRevision *int64) error {
	query := tx.Model(&chapterRecord{}).Where("id = ? AND deleted_at IS NOT NULL", record.ID)
	if expectedRevision != nil {
		query = query.Where("revision = ?", *expectedRevision)
	}
	result := query.Update("current_story_id", nil)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		if expectedRevision != nil {
			return storyError(CodeChapterRevisionConflict, "永久删除版本冲突", "章节已被其他操作更新，请刷新后重试。", nil)
		}
		return storyError(CodeChapterNotFound, "章节不存在", "该章节可能已经被永久删除。", nil)
	}
	result = tx.Where("id = ? AND deleted_at IS NOT NULL", record.ID).Delete(&chapterRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return storyError(CodeChapterNotFound, "章节不存在", "该章节可能已经被永久删除。", nil)
	}
	return nil
}

func chapterDeleteBlocked(ctx context.Context, tx *gorm.DB, record chapterRecord) (bool, error) {
	var active int64
	err := tx.WithContext(ctx).Raw(`
SELECT
  (SELECT COUNT(*) FROM task_runs AS task WHERE task.project_id=? AND task.status IN ('queued','running','waiting_for_input') AND (
    task.resource_uuid=? OR task.resource_uuid IN (
      SELECT sections.uuid FROM comic_sections AS sections
      JOIN chapter_comic_states AS states ON states.id=sections.chapter_comic_state_id
      WHERE states.chapter_id=?
    ) OR (json_valid(task.input_snapshot) AND EXISTS (
      SELECT 1 FROM json_tree(task.input_snapshot) AS input
      WHERE input.value=? OR input.value IN (
        SELECT sections.uuid FROM comic_sections AS sections
        JOIN chapter_comic_states AS states ON states.id=sections.chapter_comic_state_id
        WHERE states.chapter_id=?
      )
    ))
  )) +
  (SELECT COUNT(*) FROM production_task_runs AS task WHERE task.project_id=? AND task.status IN ('queued','running') AND (
    task.resource_uuid=? OR task.resource_uuid IN (
      SELECT sections.uuid FROM comic_sections AS sections
      JOIN chapter_comic_states AS states ON states.id=sections.chapter_comic_state_id
      WHERE states.chapter_id=?
    ) OR (json_valid(task.input_snapshot) AND EXISTS (
      SELECT 1 FROM json_tree(task.input_snapshot) AS input
      WHERE input.value=? OR input.value IN (
        SELECT sections.uuid FROM comic_sections AS sections
        JOIN chapter_comic_states AS states ON states.id=sections.chapter_comic_state_id
        WHERE states.chapter_id=?
      )
    ))
  )) +
  (SELECT COUNT(*) FROM workflows AS workflow WHERE workflow.project_id=? AND workflow.status IN ('queued','running') AND json_valid(workflow.input_snapshot) AND EXISTS (
    SELECT 1 FROM json_tree(workflow.input_snapshot) AS input
    WHERE input.value=? OR input.value IN (
      SELECT sections.uuid FROM comic_sections AS sections
      JOIN chapter_comic_states AS states ON states.id=sections.chapter_comic_state_id
      WHERE states.chapter_id=?
    )
  )) +
  (SELECT COUNT(*) FROM chat_threads AS thread WHERE thread.project_id=? AND thread.status IN ('busy','waiting_for_input') AND (
    thread.subject_uuid=? OR thread.subject_uuid IN (
      SELECT sections.uuid FROM comic_sections AS sections
      JOIN chapter_comic_states AS states ON states.id=sections.chapter_comic_state_id
      WHERE states.chapter_id=?
    )
  ))`,
		record.ProjectID, record.UUID, record.ID, record.UUID, record.ID,
		record.ProjectID, record.UUID, record.ID, record.UUID, record.ID,
		record.ProjectID, record.UUID, record.ID,
		record.ProjectID, record.UUID, record.ID,
	).Scan(&active).Error
	return active > 0, err
}

func formatImportFailure(index int, filename string, err error) error {
	return storyError(CodeValidationFailed, "章节导入校验失败", fmt.Sprintf("第 %d 项 %s：%v", index+1, filename, err), err)
}
