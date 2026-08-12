package story

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type generationResultRecord struct {
	ID             int64 `gorm:"primaryKey;autoIncrement"`
	UUID           string
	TaskRunID      int64
	ChapterID      int64
	ChapterStoryID int64
	CreatedAt      time.Time
}

func (generationResultRecord) TableName() string { return "story_generation_results" }

// ApplyGeneratedChapter is the idempotent domain boundary used by queue
// workers. It preserves the same append-only chapter story rules as manual
// edits and refuses to replace content when the snapshotted revision is stale.
func (service *Service) ApplyGeneratedChapter(ctx context.Context, taskUUID, chapterUUID, title, content, contentFormat string, expectedRevision int64) (Chapter, error) {
	if !isUUIDv7(taskUUID) {
		return Chapter{}, storyError(CodeValidationFailed, "任务 UUID 无效", "AI 生成结果必须关联公开 UUIDv7 任务。", nil)
	}
	if err := validateText(content, maxChapterBytes, "AI 生成正文"); err != nil {
		return Chapter{}, err
	}
	format, err := normalizeFormat(contentFormat)
	if err != nil {
		return Chapter{}, err
	}
	record, err := service.findChapter(ctx, chapterUUID, false)
	if err != nil {
		return Chapter{}, err
	}
	var existing generationResultRecord
	result := service.store.DB().WithContext(ctx).Table("story_generation_results AS r").
		Select("r.*").Joins("JOIN task_runs t ON t.id = r.task_run_id").
		Where("t.uuid = ?", taskUUID).Limit(1).Find(&existing)
	if result.Error != nil {
		return Chapter{}, result.Error
	}
	if result.RowsAffected == 1 {
		return service.GetChapter(ctx, chapterUUID)
	}
	projectRecord, actor, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return Chapter{}, err
	}
	now := service.now().UTC()
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task struct {
			ID                int64
			Status            string
			CancelRequestedAt *time.Time
		}
		if err := tx.Raw("SELECT id, status, cancel_requested_at FROM task_runs WHERE uuid = ? AND project_id = ?", taskUUID, projectRecord.ID).Scan(&task).Error; err != nil {
			return err
		}
		if task.ID == 0 {
			return fmt.Errorf("task run not found")
		}
		if task.Status == "cancelled" || task.CancelRequestedAt != nil {
			return context.Canceled
		}
		var count int64
		if err := tx.Model(&generationResultRecord{}).Where("task_run_id = ?", task.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		var locked chapterRecord
		if err := tx.Where("id = ? AND deleted_at IS NULL", record.ID).First(&locked).Error; err != nil {
			return err
		}
		if locked.Revision != expectedRevision {
			return storyError(CodeChapterRevisionConflict, "生成输入版本已过期", "章节在生成期间已被修改；生成结果未覆盖当前正文。", nil)
		}
		// Goal 02 constrains the source enum to manual values. Generation
		// provenance lives in task_runs/story_generation_results, while the
		// append-only Story mutation still goes through the same source boundary.
		source, item, err := service.sourceAndItem(ctx, tx, projectRecord.ID, actor.ID, locked.ID, 1, "manual_edit", contentHash("ai-generation:"+taskUUID), "", format, content)
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
		story := chapterStoryRecord{UUID: storyUUID, ChapterID: locked.ID, ActorID: actor.ID, StorySourceID: source.ID, StorySourceItemID: item.ID, VersionNo: versionNo + 1, SourceType: "manual_edit", Content: content, ContentFormat: format, ContentHash: contentHash(content), CharCount: len([]rune(content)), CreatedAt: now}
		if err := tx.Create(&story).Error; err != nil {
			return err
		}
		updates := map[string]any{"current_story_id": story.ID, "revision": gorm.Expr("revision + 1"), "updated_at": now}
		if strings.TrimSpace(title) != "" {
			updates["title"] = strings.TrimSpace(title)
		}
		updated := tx.Model(&chapterRecord{}).Where("id = ? AND revision = ? AND deleted_at IS NULL", locked.ID, expectedRevision).Updates(updates)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return storyError(CodeChapterRevisionConflict, "生成输入版本已过期", "章节在生成期间已被修改；生成结果未覆盖当前正文。", nil)
		}
		resultUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		return tx.Create(&generationResultRecord{UUID: resultUUID, TaskRunID: task.ID, ChapterID: locked.ID, ChapterStoryID: story.ID, CreatedAt: now}).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(strings.ToLower(err.Error()), "task run not found") {
			return Chapter{}, storyError(CodeValidationFailed, "生成任务不存在", "无法提交未持久化任务的生成结果。", err)
		}
		return Chapter{}, err
	}
	return service.GetChapter(ctx, chapterUUID)
}
