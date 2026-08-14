package files

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"gorm.io/gorm"
)

// RetireExportFile invalidates a legacy Asset URL as soon as its owning comic
// export expires. It intentionally accepts an internal bigint: this API never
// crosses the HTTP boundary and is only used by the export lifecycle worker.
func (service *Service) RetireExportFile(ctx context.Context, fileID int64, retiredAt time.Time) error {
	if fileID <= 0 {
		return nil
	}
	result := service.store.DB().WithContext(ctx).Model(&fileRecord{}).
		Where("id = ? AND project_id = (SELECT id FROM projects WHERE uuid = ?) AND purpose = 'export'", fileID, service.store.ProjectUUID()).
		Update("deleted_at", gorm.Expr("COALESCE(deleted_at, ?)", retiredAt.UTC()))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var exists bool
	if err := service.store.DB().WithContext(ctx).Raw(`SELECT EXISTS(
		SELECT 1 FROM files WHERE id = ? AND project_id = (SELECT id FROM projects WHERE uuid = ?) AND purpose = 'export'
	)`, fileID, service.store.ProjectUUID()).Scan(&exists).Error; err != nil {
		return err
	}
	if exists {
		return nil
	}
	return fileError(CodeAssetNotFound, "旧导出 Asset 不存在", "只能回收当前项目中 purpose=export 的逻辑 File。", nil)
}

// PurgeRetiredExportFiles is a narrowly scoped, audited GC path for the old
// dual-written ZIP format. Shared objects are retained; only the retired
// logical export File is deleted in that case.
func (service *Service) PurgeRetiredExportFiles(ctx context.Context, limit int) (removed int, attempted int, err error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	var ids []int64
	err = service.store.DB().WithContext(ctx).Table("files AS files").
		Where("files.project_id = (SELECT id FROM projects WHERE uuid = ?) AND files.purpose = 'export' AND files.deleted_at IS NOT NULL", service.store.ProjectUUID()).
		Where("NOT EXISTS (SELECT 1 FROM comic_exports exports WHERE exports.output_file_id = files.id)").
		Order("files.deleted_at ASC, files.id ASC").Limit(limit).Pluck("files.id", &ids).Error
	if err != nil {
		return 0, 0, err
	}
	attempted = len(ids)
	var failures []error
	for _, fileID := range ids {
		if err := service.purgeRetiredExportFile(ctx, fileID); err != nil {
			failures = append(failures, err)
			continue
		}
		removed++
	}
	return removed, attempted, errors.Join(failures...)
}

type exportGCReferences struct {
	OtherFiles       int64
	StoryReferences  int64
	DerivedChildren  int64
	UploadReferences int64
	ExportReferences int64
}

func (service *Service) purgeRetiredExportFile(ctx context.Context, fileID int64) error {
	return service.store.WithFileCommit(func() error {
		var logical fileRecord
		if err := service.store.DB().WithContext(ctx).Where("id = ? AND purpose = 'export' AND deleted_at IS NOT NULL", fileID).First(&logical).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var object objectRecord
		if err := service.store.DB().WithContext(ctx).Where("id = ? AND project_id = ?", logical.FileObjectID, logical.ProjectID).First(&object).Error; err != nil {
			return err
		}
		refs, err := service.exportGCReferenceCounts(ctx, logical, object)
		if err != nil {
			return err
		}
		plan, err := service.createExportGCPlan(ctx, logical, object, refs)
		if err != nil {
			return err
		}
		if refs.ExportReferences > 0 || refs.StoryReferences > 0 || refs.DerivedChildren > 0 || refs.UploadReferences > 0 {
			service.markExportGCPlanStale(ctx, plan.ID)
			return fileError(CodeReferenced, "旧导出 Asset 仍被引用", "export-only GC 的事务前引用复检未通过。", nil)
		}
		if refs.OtherFiles > 0 {
			applyErr := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := lockExportGCPlan(tx, plan.ID); err != nil {
					return err
				}
				if err := service.recheckExportFileReferences(tx, logical, false); err != nil {
					return err
				}
				if err := tx.Where("state = 'consumed' AND finalized_file_id = ?", logical.ID).Delete(&uploadRecord{}).Error; err != nil {
					return err
				}
				if err := tx.Delete(&fileRecord{}, logical.ID).Error; err != nil {
					return err
				}
				appliedAt := service.now().UTC()
				return tx.Model(&gcPlanRecord{}).Where("id = ? AND status = 'dry_run'", plan.ID).Updates(map[string]any{"status": "applied", "applied_at": appliedAt}).Error
			})
			if applyErr != nil {
				service.markExportGCPlanStale(ctx, plan.ID)
			}
			return applyErr
		}
		path, err := service.assetPath(object.KeyPath)
		if object.State == ObjectQuarantined {
			path, err = service.quarantinePath(object.UUID, object.CanonicalExt)
		}
		if err != nil {
			return err
		}
		applyErr := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Acquire SQLite's write lock before the final reference check and
			// filesystem mutation. If removal or commit fails, the logical rows
			// remain eligible and the next cleanup pass can retry idempotently.
			if err := lockExportGCPlan(tx, plan.ID); err != nil {
				return err
			}
			if err := service.recheckExportFileReferences(tx, logical, true); err != nil {
				return err
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := tx.Where("state = 'consumed' AND (file_object_id = ? OR finalized_file_id = ?)", logical.FileObjectID, logical.ID).Delete(&uploadRecord{}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&fileRecord{}, logical.ID).Error; err != nil {
				return err
			}
			result := tx.Where("id = ? AND sha256 = ?", object.ID, object.SHA256).Delete(&objectRecord{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fileError(CodeGCPlanStale, "旧导出对象已变化", "export-only GC 未按审计 snapshot 删除对象。", nil)
			}
			appliedAt := service.now().UTC()
			return tx.Model(&gcPlanRecord{}).Where("id = ? AND status = 'dry_run'", plan.ID).Updates(map[string]any{"status": "applied", "applied_at": appliedAt}).Error
		})
		if applyErr != nil {
			service.markExportGCPlanStale(ctx, plan.ID)
		}
		return applyErr
	})
}

func (service *Service) markExportGCPlanStale(ctx context.Context, planID int64) {
	_ = service.store.DB().WithContext(ctx).Model(&gcPlanRecord{}).Where("id = ? AND status = 'dry_run'", planID).Update("status", "stale").Error
}

func lockExportGCPlan(tx *gorm.DB, planID int64) error {
	result := tx.Model(&gcPlanRecord{}).Where("id = ? AND status = 'dry_run'", planID).Update("status", "dry_run")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fileError(CodeGCPlanStale, "旧导出 GC 审计已变化", "export-only GC 无法锁定当前 dry-run plan。", nil)
	}
	return nil
}

func (service *Service) exportGCReferenceCounts(ctx context.Context, logical fileRecord, object objectRecord) (exportGCReferences, error) {
	var refs exportGCReferences
	err := service.store.DB().WithContext(ctx).Raw(`SELECT
		(SELECT COUNT(*) FROM files WHERE file_object_id = ? AND id <> ?) AS other_files,
		(SELECT COUNT(*) FROM story_source_items WHERE file_id = ?) AS story_references,
		(SELECT COUNT(*) FROM files WHERE source_file_id = ?) AS derived_children,
		(SELECT COUNT(*) FROM upload_stashed WHERE state <> 'consumed' AND (file_object_id = ? OR finalized_file_id = ?)) AS upload_references,
		(SELECT COUNT(*) FROM comic_exports WHERE output_file_id = ?) AS export_references`,
		object.ID, logical.ID, logical.ID, logical.ID, object.ID, logical.ID, logical.ID).Scan(&refs).Error
	return refs, err
}

func (service *Service) recheckExportFileReferences(tx *gorm.DB, logical fileRecord, requireExclusiveObject bool) error {
	var refs exportGCReferences
	if err := tx.Raw(`SELECT
		(SELECT COUNT(*) FROM files WHERE file_object_id = ? AND id <> ?) AS other_files,
		(SELECT COUNT(*) FROM story_source_items WHERE file_id = ?) AS story_references,
		(SELECT COUNT(*) FROM files WHERE source_file_id = ?) AS derived_children,
		(SELECT COUNT(*) FROM upload_stashed WHERE state <> 'consumed' AND (file_object_id = ? OR finalized_file_id = ?)) AS upload_references,
		(SELECT COUNT(*) FROM comic_exports WHERE output_file_id = ?) AS export_references`,
		logical.FileObjectID, logical.ID, logical.ID, logical.ID, logical.FileObjectID, logical.ID, logical.ID).Scan(&refs).Error; err != nil {
		return err
	}
	if refs.ExportReferences > 0 || refs.StoryReferences > 0 || refs.DerivedChildren > 0 || refs.UploadReferences > 0 || (requireExclusiveObject && refs.OtherFiles > 0) {
		return fileError(CodeGCPlanStale, "旧导出引用已变化", "export-only GC apply 前的事务内引用复检未通过。", nil)
	}
	return nil
}

func (service *Service) createExportGCPlan(ctx context.Context, logical fileRecord, object objectRecord, refs exportGCReferences) (gcPlanRecord, error) {
	uuidValue, err := newUUIDv7()
	if err != nil {
		return gcPlanRecord{}, err
	}
	summary, err := json.Marshal(map[string]any{
		"mode": "expired_comic_export", "file_uuid": logical.UUID,
		"other_files": refs.OtherFiles, "story_references": refs.StoryReferences,
		"derived_children": refs.DerivedChildren, "upload_references": refs.UploadReferences,
		"export_references": refs.ExportReferences,
	})
	if err != nil {
		return gcPlanRecord{}, err
	}
	now := service.now().UTC()
	plan := gcPlanRecord{UUID: uuidValue, ProjectID: logical.ProjectID, SnapshotHash: gcSnapshot([]objectRecord{object}), Status: "dry_run", EstimatedBytes: object.ByteSize, CreatedAt: now}
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&plan).Error; err != nil {
			return err
		}
		objectID := object.ID
		entry := gcEntryRecord{GCPlanID: plan.ID, FileObjectID: &objectID, ObjectUUID: object.UUID, SHA256: object.SHA256, SafeKeyPath: object.KeyPath, ByteSize: object.ByteSize, ReferenceSummary: string(summary)}
		return tx.Create(&entry).Error
	})
	if err != nil {
		return gcPlanRecord{}, fmt.Errorf("create expired export GC audit: %w", err)
	}
	return plan, nil
}
