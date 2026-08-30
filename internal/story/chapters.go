package story

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"lumi/internal/agentcheckpoint"

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
	chapter, err := service.GetChapter(ctx, chapterUUID)
	if err == nil {
		service.emit("story:chapter_changed", map[string]any{"chapter_uuid": chapter.UUID, "revision": chapter.Revision})
	}
	return chapter, err
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
	chapter, err := service.GetChapter(ctx, uuid)
	if err == nil {
		service.emit("story:chapter_changed", map[string]any{"chapter_uuid": chapter.UUID, "revision": chapter.Revision})
	}
	return chapter, err
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
	chapter, err := service.GetChapter(ctx, chapterUUID)
	if err == nil {
		service.emit("story:chapter_changed", map[string]any{"chapter_uuid": chapter.UUID, "revision": chapter.Revision, "story_changed": true})
	}
	return chapter, err
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
	chapter, err := service.GetChapter(ctx, uuid)
	if err == nil {
		service.emit("story:chapter_changed", map[string]any{"chapter_uuid": chapter.UUID, "revision": chapter.Revision, "trashed": true})
	}
	return chapter, err
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
	chapter, err := service.GetChapter(ctx, uuid)
	if err == nil {
		service.emit("story:chapter_changed", map[string]any{"chapter_uuid": chapter.UUID, "revision": chapter.Revision, "restored": true})
	}
	return chapter, err
}

func (service *Service) ReorderChapters(ctx context.Context, orderedUUIDs []string) ([]Chapter, error) {
	if len(orderedUUIDs) == 0 {
		return nil, storyError(CodeValidationFailed, "章节顺序不能为空", "chapter_uuids 必须包含全部 active 章节。", nil)
	}
	seen := make(map[string]struct{}, len(orderedUUIDs))
	for _, chapterUUID := range orderedUUIDs {
		if !isUUIDv7(chapterUUID) {
			return nil, storyError(CodeValidationFailed, "章节 UUID 无效", "chapter_uuids 只能包含 UUIDv7。", nil)
		}
		if _, exists := seen[chapterUUID]; exists {
			return nil, storyError(CodeValidationFailed, "章节顺序包含重复 UUID", "每个 active 章节必须且只能出现一次。", nil)
		}
		seen[chapterUUID] = struct{}{}
	}
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		projectRecord, _, err := service.projectAndActor(ctx, tx)
		if err != nil {
			return err
		}
		var rows []chapterRecord
		if err := tx.Where("project_id = ? AND deleted_at IS NULL", projectRecord.ID).Order("sort_order ASC").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(orderedUUIDs) {
			return storyError(CodeValidationFailed, "章节顺序不完整", "chapter_uuids 必须包含全部 active 章节。", nil)
		}
		byUUID := make(map[string]chapterRecord, len(rows))
		maxOrder := len(rows)
		for _, row := range rows {
			byUUID[row.UUID] = row
			if row.SortOrder > maxOrder {
				maxOrder = row.SortOrder
			}
		}
		for _, chapterUUID := range orderedUUIDs {
			if _, exists := byUUID[chapterUUID]; !exists {
				return storyError(CodeValidationFailed, "章节顺序包含未知 UUID", "chapter_uuids 只能包含当前项目的 active 章节。", nil)
			}
		}
		now := service.now().UTC()
		temporaryBase := maxOrder + len(rows) + 1000
		for index, chapterUUID := range orderedUUIDs {
			row := byUUID[chapterUUID]
			if err := tx.Model(&chapterRecord{}).Where("id = ?", row.ID).Update("sort_order", temporaryBase+index).Error; err != nil {
				return err
			}
		}
		for index, chapterUUID := range orderedUUIDs {
			row := byUUID[chapterUUID]
			updates := map[string]any{"sort_order": index + 1, "updated_at": now}
			if row.SortOrder != index+1 {
				updates["revision"] = gorm.Expr("revision + 1")
			}
			if err := tx.Model(&chapterRecord{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	items, err := service.ListChapters(ctx, "active")
	if err == nil {
		service.emit("story:chapters_reordered", map[string]any{"chapter_uuids": orderedUUIDs})
	}
	return items, err
}

func (service *Service) PermanentlyDeleteChapter(ctx context.Context, uuid string, expectedRevision int64) error {
	return service.permanentlyDeleteChapter(ctx, uuid, expectedRevision, "")
}

// PermanentlyDeleteChapterFromTool atomically checkpoints a successful
// destructive request_api call with the Chapter deletion.
func (service *Service) PermanentlyDeleteChapterFromTool(ctx context.Context, uuid string, expectedRevision int64, toolExecutionUUID string) error {
	if !isUUIDv7(toolExecutionUUID) {
		return storyError(CodeValidationFailed, "工具执行 UUID 无效", "tool_execution_uuid 必须是 UUIDv7。", nil)
	}
	return service.permanentlyDeleteChapter(ctx, uuid, expectedRevision, toolExecutionUUID)
}

func (service *Service) permanentlyDeleteChapter(ctx context.Context, uuid string, expectedRevision int64, toolExecutionUUID string) error {
	if !isUUIDv7(uuid) {
		return storyError(CodeValidationFailed, "章节 UUID 无效", "章节资源标识必须是 UUIDv7。", nil)
	}
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if toolExecutionUUID != "" {
			data, found, err := agentcheckpoint.Read(ctx, tx, toolExecutionUUID, agentcheckpoint.RouteChapterPermanentDelete)
			if err != nil {
				return err
			}
			if found {
				if string(data) != "null" {
					return storyError(CodeChapterStateConflict, "永久删除恢复数据无效", "已提交的工具 checkpoint 与接口返回形状不匹配。", nil)
				}
				return nil
			}
		}
		projectRecord, _, err := service.projectAndActor(ctx, tx)
		if err != nil {
			return err
		}
		var record chapterRecord
		if err := tx.Where("project_id = ? AND uuid = ?", projectRecord.ID, uuid).First(&record).Error; err != nil {
			return recordNotFound(err, CodeChapterNotFound, "章节不存在", "该章节不存在或已经进入回收站。")
		}
		if record.DeletedAt == nil {
			return storyError(CodeChapterStateConflict, "章节尚未进入回收站", "永久删除前必须先把章节移入回收站。", nil)
		}
		blocked, err := chapterDeleteBlocked(ctx, tx, record)
		if err != nil {
			return err
		}
		if blocked {
			return storyError(CodeChapterDeleteBlocked, "章节仍被活动任务使用", "等待相关生成或会话结束后重试。", nil)
		}
		if err := deleteChapterTx(tx, record, &expectedRevision); err != nil {
			return err
		}
		if toolExecutionUUID != "" {
			return agentcheckpoint.Write(ctx, tx, toolExecutionUUID, agentcheckpoint.RouteChapterPermanentDelete, nil, service.now().UTC())
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Emit after the domain transaction commits. A checkpoint replay emits the
	// same idempotent invalidation hint so a crash between commit and broadcast
	// cannot leave another window displaying stale chapter facts.
	service.emit("story:chapter_permanently_deleted", map[string]any{"chapter_uuid": uuid})
	return nil
}

func (service *Service) EmptyChapterTrash(ctx context.Context) (EmptyChapterTrashResult, error) {
	return service.emptyChapterTrash(ctx, "")
}

// EmptyChapterTrashFromTool atomically checkpoints the exact deletion and
// blocker counts produced by a destructive request_api call.
func (service *Service) EmptyChapterTrashFromTool(ctx context.Context, toolExecutionUUID string) (EmptyChapterTrashResult, error) {
	if !isUUIDv7(toolExecutionUUID) {
		return EmptyChapterTrashResult{}, storyError(CodeValidationFailed, "工具执行 UUID 无效", "tool_execution_uuid 必须是 UUIDv7。", nil)
	}
	return service.emptyChapterTrash(ctx, toolExecutionUUID)
}

func (service *Service) emptyChapterTrash(ctx context.Context, toolExecutionUUID string) (EmptyChapterTrashResult, error) {
	result := EmptyChapterTrashResult{BlockedItems: []ChapterTrashBlockedItem{}}
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if toolExecutionUUID != "" {
			data, found, err := agentcheckpoint.Read(ctx, tx, toolExecutionUUID, agentcheckpoint.RouteChapterTrashEmpty)
			if err != nil {
				return err
			}
			if found {
				return json.Unmarshal(data, &result)
			}
		}
		projectRecord, _, err := service.projectAndActor(ctx, tx)
		if err != nil {
			return err
		}
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
		if toolExecutionUUID != "" {
			return agentcheckpoint.Write(ctx, tx, toolExecutionUUID, agentcheckpoint.RouteChapterTrashEmpty, result, service.now().UTC())
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	// This is intentionally an aggregate public hint: consumers invalidate the
	// chapter collection and re-read SQLite instead of treating the event as the
	// source of truth. Replays publish it again to close the post-commit crash
	// window without repeating the deletion itself.
	service.emit("story:chapter_trash_emptied", map[string]any{
		"deleted_count": result.DeletedCount,
		"blocked_count": len(result.BlockedItems),
	})
	return result, nil
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
	(SELECT COUNT(DISTINCT turns.id) FROM chat_turns AS turns
	 JOIN chat_threads AS thread ON thread.id=turns.thread_id
	 JOIN chat_items AS items ON items.turn_id=turns.id AND items.item_type='user_message'
	 JOIN chat_context_references AS refs ON refs.chat_item_id=items.id AND refs.resource_type='comic_section'
	 JOIN comic_sections AS sections ON sections.uuid=refs.resource_uuid
	 JOIN chapter_comic_states AS states ON states.id=sections.chapter_comic_state_id
	 WHERE thread.project_id=? AND turns.status IN ('queued','in_progress','waiting_for_input') AND states.chapter_id=?) +
	(SELECT COUNT(DISTINCT follow_ups.id) FROM chat_follow_ups AS follow_ups
	 JOIN chat_threads AS thread ON thread.id=follow_ups.thread_id
	 JOIN chat_context_references AS refs ON refs.follow_up_id=follow_ups.id AND refs.resource_type='comic_section'
	 JOIN comic_sections AS sections ON sections.uuid=refs.resource_uuid
	 JOIN chapter_comic_states AS states ON states.id=sections.chapter_comic_state_id
	 WHERE thread.project_id=? AND follow_ups.status='queued' AND follow_ups.deleted_at IS NULL AND states.chapter_id=?)`,
		record.ProjectID, record.UUID, record.ID, record.UUID, record.ID,
		record.ProjectID, record.UUID, record.ID, record.UUID, record.ID,
		record.ProjectID, record.UUID, record.ID,
		record.ProjectID, record.ID,
		record.ProjectID, record.ID,
	).Scan(&active).Error
	return active > 0, err
}

func formatImportFailure(index int, filename string, err error) error {
	return storyError(CodeValidationFailed, "章节导入校验失败", fmt.Sprintf("第 %d 项 %s：%v", index+1, filename, err), err)
}
