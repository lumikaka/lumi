package files

import (
	"context"
	"errors"
	"os"
	"time"

	"lumi/internal/project"

	"gorm.io/gorm"
)

const lightReconcileLimit = 100

func ReconcileOnOpen(ctx context.Context, store *project.Store) error {
	service := NewService(store, nil)
	now := service.now().UTC()
	// A full scan cannot continue across a process boundary. Release its
	// active uniqueness slot before River restores the owning maintenance
	// task, which will create a fresh, auditable scan attempt.
	if err := store.DB().WithContext(ctx).Model(&scanRecord{}).
		Where("project_id = (SELECT id FROM projects WHERE uuid = ?) AND status = ?", store.ProjectUUID(), "running").
		Updates(map[string]any{"status": "failed", "completed_at": now, "updated_at": now, "summary_json": `{"interrupted":true}`}).Error; err != nil {
		return err
	}
	_, err := service.Reconcile(ctx, lightReconcileLimit)
	return err
}

type ReconcileSummary struct {
	Recovered int `json:"recovered"`
	Missing   int `json:"missing"`
	Expired   int `json:"expired"`
}

type UploadCleanupSummary struct {
	ReconcileSummary
	Removed      int64 `json:"removed"`
	RetiredFiles int64 `json:"retired_files"`
}

func (service *Service) CleanupUploads(ctx context.Context, limit int, retention time.Duration) (UploadCleanupSummary, error) {
	if limit <= 0 || limit > 1000 {
		limit = lightReconcileLimit
	}
	if retention <= 0 {
		retention = 7 * 24 * time.Hour
	}
	reconciled, err := service.Reconcile(ctx, limit)
	if err != nil {
		return UploadCleanupSummary{}, err
	}
	cutoff := service.now().UTC().Add(-retention)
	var orphanFileIDs []int64
	if err := service.store.DB().WithContext(ctx).Table("files AS files").
		Where("files.project_id = (SELECT id FROM projects WHERE uuid = ?) AND files.purpose = 'project_chatbot_reference' AND files.deleted_at IS NULL AND files.created_at <= ?", service.store.ProjectUUID(), cutoff).
		Where("NOT EXISTS (SELECT 1 FROM chat_context_references refs WHERE refs.file_id=files.id OR refs.image_file_id=files.id)").
		Where("NOT EXISTS (SELECT 1 FROM project_creation_reference_files creation_refs WHERE creation_refs.file_id=files.id)").
		Order("files.id ASC").Limit(limit).Pluck("files.id", &orphanFileIDs).Error; err != nil {
		return UploadCleanupSummary{}, err
	}
	var ids []int64
	if err := service.store.DB().WithContext(ctx).Model(&uploadRecord{}).Where(`project_id = (SELECT id FROM projects WHERE uuid = ?) AND ((state = ? AND updated_at <= ? AND file_object_id IS NULL AND finalized_file_id IS NULL) OR (state = ? AND consumed_at <= ?))`, service.store.ProjectUUID(), StateExpired, cutoff, StateConsumed, cutoff).Order("id ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return UploadCleanupSummary{}, err
	}
	if len(ids) == 0 && len(orphanFileIDs) == 0 {
		return UploadCleanupSummary{ReconcileSummary: reconciled}, nil
	}
	now := service.now().UTC()
	var removed, retired int64
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(orphanFileIDs) > 0 {
			result := tx.Model(&fileRecord{}).
				Where("id IN ? AND purpose='project_chatbot_reference' AND deleted_at IS NULL", orphanFileIDs).
				Where("NOT EXISTS (SELECT 1 FROM chat_context_references refs WHERE refs.file_id=files.id OR refs.image_file_id=files.id)").
				Where("NOT EXISTS (SELECT 1 FROM project_creation_reference_files creation_refs WHERE creation_refs.file_id=files.id)").
				Update("deleted_at", now)
			if result.Error != nil {
				return result.Error
			}
			retired = result.RowsAffected
		}
		if len(ids) > 0 {
			result := tx.Where("id IN ?", ids).Delete(&uploadRecord{})
			if result.Error != nil {
				return result.Error
			}
			removed = result.RowsAffected
		}
		return nil
	})
	if err != nil {
		return UploadCleanupSummary{}, err
	}
	return UploadCleanupSummary{ReconcileSummary: reconciled, Removed: removed, RetiredFiles: retired}, nil
}

func (service *Service) Reconcile(ctx context.Context, limit int) (ReconcileSummary, error) {
	var summary ReconcileSummary
	err := service.store.WithFileCommit(func() error {
		var err error
		summary, err = service.reconcile(ctx, limit)
		return err
	})
	return summary, err
}

func (service *Service) reconcile(ctx context.Context, limit int) (ReconcileSummary, error) {
	if limit <= 0 || limit > 1000 {
		limit = lightReconcileLimit
	}
	summary := ReconcileSummary{}
	// A crash after a deduplicated ready object was claimed but before the
	// final File transaction leaves only the upload in consuming. Reset that
	// journal to ready after re-verifying the immutable object so finalize is
	// safely retryable with the same upload UUID.
	var consuming []uploadRecord
	if err := service.store.DB().WithContext(ctx).Table("upload_stashed AS u").Select("u.*").
		Joins("JOIN file_objects AS o ON o.id = u.file_object_id").
		Where("u.project_id = (SELECT id FROM projects WHERE uuid = ?) AND u.state = ? AND o.state = ?", service.store.ProjectUUID(), StateConsuming, ObjectReady).
		Order("u.id ASC").Limit(limit).Scan(&consuming).Error; err != nil {
		return summary, err
	}
	for _, upload := range consuming {
		var object objectRecord
		if err := service.store.DB().WithContext(ctx).First(&object, *upload.FileObjectID).Error; err != nil {
			return summary, err
		}
		if verifyErr := service.verifyObjectFile(object); verifyErr != nil {
			state := ObjectCorrupt
			if errors.Is(verifyErr, os.ErrNotExist) {
				state = ObjectMissing
			}
			now := service.now().UTC()
			if err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := tx.Model(&objectRecord{}).Where("id = ? AND state = ?", object.ID, ObjectReady).Update("state", state).Error; err != nil {
					return err
				}
				return tx.Model(&uploadRecord{}).Where("id = ? AND state = ?", upload.ID, StateConsuming).Updates(map[string]any{"state": StateFailed, "error_code": CodeObjectUnavailable, "updated_at": now}).Error
			}); err != nil {
				return summary, err
			}
			if state == ObjectMissing {
				service.emitObjectAssets(ctx, object.ID, "asset/missing", ObjectMissing)
			}
			continue
		}
		now := service.now().UTC()
		result := service.store.DB().WithContext(ctx).Model(&uploadRecord{}).Where("id = ? AND state = ?", upload.ID, StateConsuming).Updates(map[string]any{"state": StateReady, "updated_at": now})
		if result.Error != nil {
			return summary, result.Error
		}
		if result.RowsAffected == 1 {
			summary.Recovered++
			service.emit("asset/reconciled", map[string]any{"upload_uuid": upload.UUID, "status": StateReady})
		}
	}
	remainingLimit := limit - len(consuming)
	if remainingLimit < 0 {
		remainingLimit = 0
	}
	var pending []objectRecord
	if err := service.store.DB().WithContext(ctx).Where("project_id = (SELECT id FROM projects WHERE uuid = ?) AND state = ?", service.store.ProjectUUID(), ObjectPending).Order("id ASC").Limit(remainingLimit).Find(&pending).Error; err != nil {
		return summary, err
	}
	for _, object := range pending {
		var upload uploadRecord
		uploadErr := service.store.DB().WithContext(ctx).Where("file_object_id = ? AND state = ?", object.ID, StateConsuming).Order("id ASC").First(&upload).Error
		target, pathErr := service.assetPath(object.KeyPath)
		if pathErr != nil {
			now := service.now().UTC()
			if err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := tx.Model(&objectRecord{}).Where("id = ? AND state = ?", object.ID, ObjectPending).Update("state", ObjectCorrupt).Error; err != nil {
					return err
				}
				return tx.Model(&uploadRecord{}).Where("file_object_id = ? AND state = ?", object.ID, StateConsuming).Updates(map[string]any{"state": StateFailed, "error_code": CodeUnsafePath, "updated_at": now}).Error
			}); err != nil {
				return summary, err
			}
			continue
		}
		_, statErr := os.Lstat(target)
		if errors.Is(statErr, os.ErrNotExist) && uploadErr == nil {
			if part, partErr := service.partPath(upload.UUID); partErr == nil {
				if _, partStatErr := os.Lstat(part); partStatErr == nil {
					if publishErr := service.publishObject(upload, object); publishErr != nil {
						return summary, publishErr
					}
					_, statErr = os.Lstat(target)
				}
			}
		}
		if statErr == nil && service.verifyObjectFile(object) == nil {
			now := service.now().UTC()
			err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := tx.Model(&objectRecord{}).Where("id = ? AND state = ?", object.ID, ObjectPending).Updates(map[string]any{"state": ObjectReady, "verified_at": now}).Error; err != nil {
					return err
				}
				return tx.Model(&uploadRecord{}).Where("file_object_id = ? AND state = ?", object.ID, StateConsuming).Updates(map[string]any{"state": StateReady, "updated_at": now}).Error
			})
			if err != nil {
				return summary, err
			}
			summary.Recovered++
			service.emitObjectAssets(ctx, object.ID, "asset/reconciled", ObjectReady)
			if uploadErr == nil {
				service.emit("asset/reconciled", map[string]any{"upload_uuid": upload.UUID, "status": ObjectReady})
			}
			continue
		}
		now := service.now().UTC()
		if err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&objectRecord{}).Where("id = ? AND state = ?", object.ID, ObjectPending).Update("state", ObjectMissing).Error; err != nil {
				return err
			}
			return tx.Model(&uploadRecord{}).Where("file_object_id = ? AND state = ?", object.ID, StateConsuming).Updates(map[string]any{"state": StateFailed, "error_code": CodeObjectUnavailable, "updated_at": now}).Error
		}); err != nil {
			return summary, err
		}
		summary.Missing++
	}
	remaining := remainingLimit - len(pending)
	if remaining < 0 {
		remaining = 0
	}
	var ready []objectRecord
	if remaining > 0 {
		if err := service.store.DB().WithContext(ctx).Where("project_id = (SELECT id FROM projects WHERE uuid = ?) AND state = ?", service.store.ProjectUUID(), ObjectReady).Order("verified_at ASC, id ASC").Limit(remaining).Find(&ready).Error; err != nil {
			return summary, err
		}
	}
	for _, object := range ready {
		path, err := service.assetPath(object.KeyPath)
		if err != nil {
			if updateErr := service.store.DB().WithContext(ctx).Model(&objectRecord{}).Where("id = ? AND state = ?", object.ID, ObjectReady).Update("state", ObjectCorrupt).Error; updateErr != nil {
				return summary, updateErr
			}
			continue
		}
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			quarantine, quarantineErr := service.quarantinePath(object.UUID, object.CanonicalExt)
			if quarantineErr == nil {
				if quarantineInfo, statErr := os.Lstat(quarantine); statErr == nil && quarantineInfo.Mode().IsRegular() && quarantineInfo.Mode()&os.ModeSymlink == 0 {
					if updateErr := service.store.DB().WithContext(ctx).Model(&objectRecord{}).Where("id = ? AND state = ?", object.ID, ObjectReady).Update("state", ObjectQuarantined).Error; updateErr != nil {
						return summary, updateErr
					}
					service.emitObjectAssets(ctx, object.ID, "asset/updated", ObjectQuarantined)
					continue
				}
			}
			if updateErr := service.store.DB().WithContext(ctx).Model(&objectRecord{}).Where("id = ? AND state = ?", object.ID, ObjectReady).Update("state", ObjectMissing).Error; updateErr != nil {
				return summary, updateErr
			}
			summary.Missing++
			service.emitObjectAssets(ctx, object.ID, "asset/missing", ObjectMissing)
		} else if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != object.ByteSize {
			if updateErr := service.store.DB().WithContext(ctx).Model(&objectRecord{}).Where("id = ? AND state = ?", object.ID, ObjectReady).Update("state", ObjectCorrupt).Error; updateErr != nil {
				return summary, updateErr
			}
			service.emitObjectAssets(ctx, object.ID, "asset/updated", ObjectCorrupt)
		}
	}
	now := service.now().UTC()
	var expired []uploadRecord
	if err := service.store.DB().WithContext(ctx).Where("project_id = (SELECT id FROM projects WHERE uuid = ?) AND state IN ? AND expires_at <= ?", service.store.ProjectUUID(), []string{StateReceiving, StateReady, StateFailed}, now).Order("expires_at ASC,id ASC").Limit(limit).Find(&expired).Error; err != nil {
		return summary, err
	}
	for _, upload := range expired {
		result := service.store.DB().WithContext(ctx).Model(&uploadRecord{}).Where("id = ? AND state IN ?", upload.ID, []string{StateReceiving, StateReady, StateFailed}).Updates(map[string]any{"state": StateExpired, "updated_at": now})
		if result.Error != nil {
			return summary, result.Error
		}
		if result.RowsAffected == 1 {
			if part, err := service.partPath(upload.UUID); err == nil {
				_ = os.Remove(part)
			}
			summary.Expired++
			service.emit("upload/expired", map[string]any{"upload_uuid": upload.UUID, "status": StateExpired})
		}
	}
	return summary, nil
}
