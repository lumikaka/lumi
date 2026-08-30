package story

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"lumi/internal/durablefs"
	"lumi/internal/project"

	"gorm.io/gorm"
)

const DefaultStoryMD = "# STORY\n\n<!-- Lumi Goal 02 将在这里维护故事正文。 -->\n"

func ReconcileOnOpen(ctx context.Context, store *project.Store) error {
	if store.SetupStatus() == project.SetupStatusDraft {
		return nil
	}
	service := NewService(store)
	if _, err := service.GetStoryProfile(ctx); err != nil {
		return err
	}
	sourceType := "migration"
	if project.IsProjectCreation(ctx) {
		sourceType = "project_created"
	}
	return service.EnsurePromptCatalogVersions(ctx, sourceType)
}

func (service *Service) readStoryMD() (string, string, error) {
	path, err := service.store.ResolvePath("STORY.md")
	if err != nil {
		return "", "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxStoryMDBytes+1))
	if err != nil {
		return "", "", err
	}
	if len(content) > maxStoryMDBytes {
		return "", fmt.Sprintf("oversize:%d", len(content)), storyError(CodeValidationFailed, "STORY.md 过大", fmt.Sprintf("文件不能超过 %d 字节。", maxStoryMDBytes), nil)
	}
	value := string(content)
	if err := validateText(value, maxStoryMDBytes, "STORY.md"); err != nil {
		return value, contentHash(value), err
	}
	return value, contentHash(value), nil
}

func profileDTO(record storyProfileRecord) StoryProfile {
	return StoryProfile{UUID: record.UUID, VersionNo: record.VersionNo, Revision: record.Revision, StoryMD: record.StoryMD, ContentHash: record.ContentHash, SourceType: record.SourceType, ProjectionState: record.ProjectionState, ExportedRevision: record.ExportedRevision, ExportedHash: record.ExportedHash, ObservedFileHash: record.ObservedFileHash, CreatedAt: record.CreatedAt}
}

func (service *Service) ensureStoryProfile(ctx context.Context) (storyProfileRecord, error) {
	projectRecord, actor, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return storyProfileRecord{}, err
	}
	var current storyProfileRecord
	err = service.store.DB().WithContext(ctx).Where("project_id = ? AND is_current = 1", projectRecord.ID).First(&current).Error
	if err == nil {
		return current, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return current, err
	}
	fileHash := ""
	_, observedHash, readErr := service.readStoryMD()
	if observedHash != "" {
		fileHash = observedHash
	}
	defaultHash := contentHash(DefaultStoryMD)
	state := "conflict"
	if readErr == nil && fileHash == defaultHash {
		state = "synced"
	}
	profileUUID, err := newUUIDv7()
	if err != nil {
		return current, err
	}
	current = storyProfileRecord{UUID: profileUUID, ProjectID: projectRecord.ID, ActorID: actor.ID, VersionNo: 1, Revision: 1, IsCurrent: true, StoryMD: DefaultStoryMD, ContentHash: defaultHash, SourceType: "project_created", ProjectionState: state, ExportedRevision: 1, ExportedHash: defaultHash, ObservedFileHash: fileHash, CreatedAt: service.now().UTC()}
	if err := service.store.DB().WithContext(ctx).Create(&current).Error; err != nil {
		if uniqueConflict(err) {
			if lookupErr := service.store.DB().WithContext(ctx).Where("project_id = ? AND is_current = 1", projectRecord.ID).First(&current).Error; lookupErr == nil {
				return current, nil
			}
		}
		return current, err
	}
	return current, nil
}

func (service *Service) updateProjectionState(ctx context.Context, profileID int64, state string, exportedRevision int64, exportedHash, observedHash string) error {
	return service.store.DB().WithContext(ctx).Model(&storyProfileRecord{}).Where("id = ?", profileID).Updates(map[string]any{"projection_state": state, "exported_revision": exportedRevision, "exported_hash": exportedHash, "observed_file_hash": observedHash}).Error
}

func (service *Service) reconcileStoryProfile(ctx context.Context) (storyProfileRecord, error) {
	current, err := service.ensureStoryProfile(ctx)
	if err != nil {
		return current, err
	}
	_, fileHash, readErr := service.readStoryMD()
	if readErr != nil {
		if updateErr := service.updateProjectionState(ctx, current.ID, "conflict", current.ExportedRevision, current.ExportedHash, fileHash); updateErr != nil {
			return current, updateErr
		}
		current.ProjectionState = "conflict"
		current.ObservedFileHash = fileHash
		return current, nil
	}
	if fileHash == current.ContentHash {
		if current.ProjectionState != "synced" || current.ExportedRevision != current.Revision || current.ExportedHash != fileHash || current.ObservedFileHash != fileHash {
			if err := service.updateProjectionState(ctx, current.ID, "synced", current.Revision, fileHash, fileHash); err != nil {
				return current, err
			}
		}
		current.ProjectionState = "synced"
		current.ExportedRevision = current.Revision
		current.ExportedHash = fileHash
		current.ObservedFileHash = fileHash
		return current, nil
	}
	if current.ProjectionState == "pending" && current.ExportedHash != "" && fileHash == current.ExportedHash {
		if err := service.writeProjection(current.StoryMD); err != nil {
			// The original mutation already reported the projection failure. Reads
			// keep the project usable and expose the durable pending state so the UI
			// can offer an explicit retry after permissions or disk space recover.
			return current, nil
		}
		if err := service.updateProjectionState(ctx, current.ID, "synced", current.Revision, current.ContentHash, current.ContentHash); err != nil {
			return current, err
		}
		current.ProjectionState = "synced"
		current.ExportedRevision = current.Revision
		current.ExportedHash = current.ContentHash
		current.ObservedFileHash = current.ContentHash
		return current, nil
	}
	if err := service.updateProjectionState(ctx, current.ID, "conflict", current.ExportedRevision, current.ExportedHash, fileHash); err != nil {
		return current, err
	}
	current.ProjectionState = "conflict"
	current.ObservedFileHash = fileHash
	return current, nil
}

func (service *Service) GetStoryProfile(ctx context.Context) (StoryProfile, error) {
	current, err := service.reconcileStoryProfile(ctx)
	if err != nil {
		return StoryProfile{}, err
	}
	return profileDTO(current), nil
}

func (service *Service) createStoryProfileVersion(ctx context.Context, current storyProfileRecord, storyMD, sourceType, state string, exportedRevision int64, exportedHash, observedHash string) (storyProfileRecord, error) {
	projectRecord, actor, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return storyProfileRecord{}, err
	}
	profileUUID, err := newUUIDv7()
	if err != nil {
		return storyProfileRecord{}, err
	}
	next := storyProfileRecord{UUID: profileUUID, ProjectID: projectRecord.ID, ActorID: actor.ID, VersionNo: current.VersionNo + 1, Revision: current.Revision + 1, IsCurrent: true, StoryMD: storyMD, ContentHash: contentHash(storyMD), SourceType: sourceType, ProjectionState: state, ExportedRevision: exportedRevision, ExportedHash: exportedHash, ObservedFileHash: observedHash, CreatedAt: service.now().UTC()}
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&storyProfileRecord{}).Where("id = ? AND revision = ? AND is_current = 1", current.ID, current.Revision).Update("is_current", false)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return storyError(CodeStoryProfileConflict, "STORY.md 版本冲突", "数据库中的 Story Profile 已更新，请刷新后重试。", nil)
		}
		return tx.Create(&next).Error
	})
	return next, err
}

func (service *Service) UpdateStoryProfile(ctx context.Context, storyMD string, expectedRevision int64) (StoryProfile, error) {
	return service.updateStoryProfile(ctx, storyMD, expectedRevision, "manual_edit")
}

// ApplyGeneratedStoryProfile appends an AI-generated profile version while
// keeping the same optimistic concurrency and STORY.md projection guarantees
// as a manual edit.
func (service *Service) ApplyGeneratedStoryProfile(ctx context.Context, storyMD string, expectedRevision int64) (StoryProfile, error) {
	// Existing project files use the historical manual_edit source enum. The
	// durable task/result records carry the generated provenance without
	// rewriting that append-only table's compatibility constraint.
	return service.updateStoryProfile(ctx, storyMD, expectedRevision, "manual_edit")
}

func (service *Service) updateStoryProfile(ctx context.Context, storyMD string, expectedRevision int64, sourceType string) (StoryProfile, error) {
	if err := validateText(storyMD, maxStoryMDBytes, "STORY.md"); err != nil {
		return StoryProfile{}, err
	}
	current, err := service.reconcileStoryProfile(ctx)
	if err != nil {
		return StoryProfile{}, err
	}
	if current.ContentHash == contentHash(storyMD) {
		return profileDTO(current), nil
	}
	if current.Revision != expectedRevision {
		return StoryProfile{}, storyError(CodeStoryProfileConflict, "STORY.md 版本冲突", "Story Profile 已被更新，请保留本地草稿并刷新后重试。", nil)
	}
	if current.ProjectionState == "conflict" {
		return StoryProfile{}, storyError(CodeStoryMDConflict, "检测到外部 STORY.md 修改", "请先选择导入外部文件或用数据库版本重新生成文件。", nil)
	}
	next, err := service.createStoryProfileVersion(ctx, current, storyMD, sourceType, "pending", current.ExportedRevision, current.ExportedHash, current.ObservedFileHash)
	if err != nil {
		return StoryProfile{}, err
	}
	if err := service.writeProjection(storyMD); err != nil {
		return StoryProfile{}, storyError(CodeStoryProjectionFailed, "Story Profile 已保存，但 STORY.md 写入失败", "数据库版本处于 pending；修复文件权限后可重新生成或在下次打开时恢复。", err)
	}
	if err := service.updateProjectionState(ctx, next.ID, "synced", next.Revision, next.ContentHash, next.ContentHash); err != nil {
		return StoryProfile{}, storyError(CodeStoryProjectionFailed, "STORY.md 已写入，但同步状态记录失败", "重新读取 Story Profile 会自动核对并恢复状态。", err)
	}
	next.ProjectionState = "synced"
	next.ExportedRevision = next.Revision
	next.ExportedHash = next.ContentHash
	next.ObservedFileHash = next.ContentHash
	profile := profileDTO(next)
	service.emit("story:profile_changed", map[string]any{"story_profile_uuid": profile.UUID, "revision": profile.Revision})
	return profile, nil
}

func (service *Service) RestoreStoryProfile(ctx context.Context, versionUUID string, expectedRevision int64) (StoryProfile, error) {
	if !isUUIDv7(versionUUID) {
		return StoryProfile{}, storyError(CodeValidationFailed, "Story Profile 版本 UUID 无效", "version_uuid 必须是 UUIDv7。", nil)
	}
	projectRecord, _, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return StoryProfile{}, err
	}
	var target storyProfileRecord
	if err := service.store.DB().WithContext(ctx).Where("project_id = ? AND uuid = ?", projectRecord.ID, versionUUID).First(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return StoryProfile{}, storyError(CodeStoryProfileNotFound, "Story Profile 版本不存在", "目标版本不存在或不属于当前项目。", err)
		}
		return StoryProfile{}, err
	}
	return service.updateStoryProfile(ctx, target.StoryMD, expectedRevision, "manual_edit")
}

func (service *Service) ImportExternalStoryMD(ctx context.Context, expectedRevision int64) (StoryProfile, error) {
	current, err := service.reconcileStoryProfile(ctx)
	if err != nil {
		return StoryProfile{}, err
	}
	if current.Revision != expectedRevision {
		return StoryProfile{}, storyError(CodeStoryProfileConflict, "STORY.md 版本冲突", "Story Profile 已被更新，请刷新后重试。", nil)
	}
	if current.ProjectionState != "conflict" {
		return StoryProfile{}, storyError(CodeStoryMDConflict, "没有待导入的外部修改", "只有检测到 STORY.md 冲突时才能执行外部导入。", nil)
	}
	storyMD, fileHash, err := service.readStoryMD()
	if err != nil {
		return StoryProfile{}, err
	}
	next, err := service.createStoryProfileVersion(ctx, current, storyMD, "external_import", "synced", current.Revision+1, fileHash, fileHash)
	if err != nil {
		return StoryProfile{}, err
	}
	profile := profileDTO(next)
	service.emit("story:profile_changed", map[string]any{"story_profile_uuid": profile.UUID, "revision": profile.Revision, "external_import": true})
	return profile, nil
}

func (service *Service) RegenerateStoryMD(ctx context.Context, expectedRevision int64) (StoryProfile, error) {
	current, err := service.ensureStoryProfile(ctx)
	if err != nil {
		return StoryProfile{}, err
	}
	if current.Revision != expectedRevision {
		return StoryProfile{}, storyError(CodeStoryProfileConflict, "STORY.md 版本冲突", "Story Profile 已被更新，请刷新后重试。", nil)
	}
	if err := service.writeProjection(current.StoryMD); err != nil {
		_ = service.updateProjectionState(ctx, current.ID, "pending", current.ExportedRevision, current.ExportedHash, current.ObservedFileHash)
		return StoryProfile{}, storyError(CodeStoryProjectionFailed, "无法重新生成 STORY.md", "数据库版本未丢失；修复文件权限后可以重试。", err)
	}
	if err := service.updateProjectionState(ctx, current.ID, "synced", current.Revision, current.ContentHash, current.ContentHash); err != nil {
		return StoryProfile{}, err
	}
	current.ProjectionState = "synced"
	current.ExportedRevision = current.Revision
	current.ExportedHash = current.ContentHash
	current.ObservedFileHash = current.ContentHash
	profile := profileDTO(current)
	service.emit("story:profile_changed", map[string]any{"story_profile_uuid": profile.UUID, "revision": profile.Revision, "projection_regenerated": true})
	return profile, nil
}

func (service *Service) ListStoryProfiles(ctx context.Context) ([]StoryProfile, error) {
	projectRecord, _, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return nil, err
	}
	var records []storyProfileRecord
	if err := service.store.DB().WithContext(ctx).Where("project_id = ?", projectRecord.ID).Order("version_no DESC").Limit(200).Find(&records).Error; err != nil {
		return nil, err
	}
	items := make([]StoryProfile, 0, len(records))
	for _, record := range records {
		items = append(items, profileDTO(record))
	}
	return items, nil
}

func (service *Service) atomicWriteStoryMD(content string) error {
	target, err := service.store.ResolvePath("STORY.md")
	if err != nil {
		return err
	}
	tempDirectory, err := service.store.ResolvePath(".lumi/tmp")
	if err != nil {
		return err
	}
	operationUUID, err := newUUIDv7()
	if err != nil {
		return err
	}
	temporary := filepath.Join(tempDirectory, "story-projection-"+operationUUID+".tmp")
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(temporary)
		}
	}()
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		return err
	}
	cleanup = false
	return durablefs.SyncDirectory(service.store.Root())
}
