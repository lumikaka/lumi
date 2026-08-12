package files

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"gorm.io/gorm"
)

// RepairContent restores the exact immutable bytes of an existing Asset.
// It accepts only content whose validated digest, size, and MIME type match
// the original object, so callers cannot use repair to mutate an Asset.
func (service *Service) RepairContent(ctx context.Context, assetUUID string, reader io.Reader) (Asset, error) {
	if reader == nil {
		return Asset{}, fileError(CodeValidationFailed, "Asset 修复内容为空", "repair 必须提供原始内容。", nil)
	}
	row, err := service.assetRowByUUID(ctx, assetUUID, false)
	if err != nil {
		return Asset{}, err
	}
	upload, err := service.CreateUpload(ctx, CreateUploadInput{
		Purpose: row.Purpose, OriginalFilename: canonicalFilename(assetUUID + "." + row.CanonicalExt), DisplayName: "Asset repair", Reader: reader,
	})
	if err != nil {
		return Asset{}, err
	}
	record, err := service.uploadRecord(ctx, upload.UUID)
	if err != nil {
		return Asset{}, err
	}
	if record.State != StateReady || record.SHA256 == nil || record.ByteSize == nil || record.MIMEType == nil {
		return Asset{}, fileError(CodeUploadNotReady, "Asset 修复内容尚未校验完成", "repair 暂存内容必须处于 ready 状态。", nil)
	}
	if *record.SHA256 != row.SHA256 || *record.ByteSize != row.ByteSize || *record.MIMEType != row.MIMEType {
		return Asset{}, fileError(CodeInvalidContent, "Asset 修复内容不匹配", "repair 只能恢复与原对象摘要、大小和 MIME 完全一致的内容。", nil)
	}

	err = service.store.WithFileCommit(func() error {
		var current assetRow
		if err := service.assetQuery(ctx).Where("f.uuid = ?", assetUUID).Scan(&current).Error; err != nil {
			return err
		}
		if current.ID == 0 {
			return fileError(CodeAssetNotFound, "Asset 不存在", "只能修复当前项目的正式 Asset。", gorm.ErrRecordNotFound)
		}
		if current.SHA256 != *record.SHA256 || current.ByteSize != *record.ByteSize || current.MIMEType != *record.MIMEType {
			return fileError(CodeInvalidContent, "Asset 修复内容不匹配", "对象在 repair 提交前已发生变化。", nil)
		}
		var currentObject objectRecord
		if err := service.store.DB().WithContext(ctx).First(&currentObject, current.FileObjectID).Error; err != nil {
			return err
		}
		part, err := service.partPath(record.UUID)
		if err != nil {
			return err
		}
		target, err := service.assetPath(current.KeyPath)
		if err != nil {
			return err
		}
		if info, statErr := os.Lstat(target); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return fileError(CodeUnsafePath, "Asset 修复目标不安全", "repair 目标必须是受管目录内的普通文件。", nil)
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fileError(CodeOperationUnavailable, "无法创建 Asset 修复目录", "repair 内容尚未提交。", err)
		}
		if err := os.Rename(part, target); err != nil {
			return fileError(CodeOperationUnavailable, "Asset 原子修复失败", "repair 暂存内容已保留，可安全重试。", err)
		}
		if err := syncDirectory(filepath.Dir(target)); err != nil {
			return fileError(CodeOperationUnavailable, "Asset 修复目录同步失败", "对象已恢复但尚未确认持久化。", err)
		}
		if err := service.verifyObjectFile(currentObject); err != nil {
			return err
		}
		now := service.now().UTC()
		return service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&objectRecord{}).Where("id = ?", current.FileObjectID).Updates(map[string]any{"state": ObjectReady, "verified_at": now}).Error; err != nil {
				return err
			}
			result := tx.Model(&uploadRecord{}).Where("id = ? AND state = ?", record.ID, StateReady).Updates(map[string]any{
				"state": StateConsumed, "file_object_id": current.FileObjectID, "finalized_file_id": current.ID, "consumed_at": now, "updated_at": now,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fileError(CodeInvalidState, "Asset repair 状态冲突", "修复提交没有被重复写入。", nil)
			}
			return nil
		})
	})
	if err != nil {
		return Asset{}, err
	}
	asset, err := service.GetAsset(ctx, assetUUID, false)
	if err == nil {
		service.emit("asset/repaired", map[string]any{"asset_uuid": asset.UUID, "status": asset.Status})
	}
	return asset, err
}
