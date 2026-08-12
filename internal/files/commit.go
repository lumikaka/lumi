package files

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"lumi/internal/durablefs"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (service *Service) FinalizeUpload(ctx context.Context, uploadUUID, expectedPurpose string) (Asset, error) {
	return service.finalizeUpload(ctx, uploadUUID, expectedPurpose, "imported", "", nil)
}

// FinalizeUploadWithBind is the domain-facing form of FinalizeUpload. Bind is
// executed in the same final SQLite transaction that creates the immutable
// File row. It lets a resource import become visible only after its bytes are
// durable, without allowing callers to bypass Asset Store.
func (service *Service) FinalizeUploadWithBind(ctx context.Context, uploadUUID, expectedPurpose string, bind func(*gorm.DB, int64) error) (Asset, error) {
	if bind == nil {
		return service.FinalizeUpload(ctx, uploadUUID, expectedPurpose)
	}
	return service.finalizeUpload(ctx, uploadUUID, expectedPurpose, "imported", "", bind)
}

// CommitReader sends generated and derived outputs through the same durable
// part/object/file commit protocol. Bind runs in the final SQLite transaction
// with File creation, so a domain reference cannot observe half a commit.
func (service *Service) CommitReader(ctx context.Context, input CommitInput) (Asset, error) {
	sourceType := strings.TrimSpace(input.SourceType)
	if sourceType == "" {
		sourceType = "generated"
	}
	if sourceType != "generated" && sourceType != "derived" && sourceType != "exported" && sourceType != "imported" {
		return Asset{}, fileError(CodeValidationFailed, "Asset source_type 无效", "source_type 必须来自服务端允许列表。", nil)
	}
	upload, err := service.CreateUpload(ctx, CreateUploadInput{Purpose: input.Purpose, OriginalFilename: input.OriginalFilename, DisplayName: input.DisplayName, Metadata: input.Metadata, Reader: input.Reader})
	if err != nil {
		return Asset{}, err
	}
	return service.finalizeUpload(ctx, upload.UUID, input.Purpose, sourceType, input.SourceAssetUUID, input.Bind)
}

func (service *Service) CreateDerived(ctx context.Context, sourceAssetUUID string, input CommitInput) (Asset, error) {
	if _, err := service.GetAsset(ctx, sourceAssetUUID, false); err != nil {
		return Asset{}, err
	}
	input.SourceType = "derived"
	input.SourceAssetUUID = sourceAssetUUID
	return service.CommitReader(ctx, input)
}

func (service *Service) finalizeUpload(ctx context.Context, uploadUUID, expectedPurpose, sourceType, sourceAssetUUID string, bind func(*gorm.DB, int64) error) (Asset, error) {
	record, err := service.uploadRecord(ctx, uploadUUID)
	if err != nil {
		return Asset{}, err
	}
	if expectedPurpose != "" && record.Purpose != expectedPurpose {
		return Asset{}, fileError(CodePurposeMismatch, "上传 purpose 不匹配", "暂存上传不能被其他业务用途消费。", nil)
	}
	_, actor, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return Asset{}, err
	}
	if actor.ID != record.ActorID {
		return Asset{}, fileError(CodeActorMismatch, "上传 actor 不匹配", "暂存上传只能由创建它的本地 actor 消费。", nil)
	}
	if record.State == StateConsumed && record.FinalizedFileID != nil {
		var file fileRecord
		if err := service.store.DB().WithContext(ctx).First(&file, *record.FinalizedFileID).Error; err != nil {
			return Asset{}, err
		}
		return service.GetAsset(ctx, file.UUID, true)
	}
	if service.now().UTC().After(record.ExpiresAt) && record.State != StateConsuming {
		now := service.now().UTC()
		_ = service.store.DB().WithContext(ctx).Model(&uploadRecord{}).Where("id = ? AND state IN ?", record.ID, []string{StateReceiving, StateReady, StateFailed}).Updates(map[string]any{"state": StateExpired, "updated_at": now}).Error
		if part, pathErr := service.partPath(record.UUID); pathErr == nil {
			_ = os.Remove(part)
		}
		service.emit("upload/expired", map[string]any{"upload_uuid": record.UUID, "status": StateExpired})
		return Asset{}, fileError(CodeUploadExpired, "上传已经过期", "请重新上传后再 finalize。", nil)
	}
	if record.State != StateReady && record.State != StateConsuming {
		return Asset{}, fileError(CodeUploadNotReady, "上传尚未完成", "只有 ready 上传可以 finalize。", nil)
	}
	if record.SHA256 == nil || record.MIMEType == nil || record.CanonicalExt == nil || record.ByteSize == nil {
		return Asset{}, fileError(CodeUploadNotReady, "上传校验尚未完成", "暂存记录缺少完整内容摘要。", nil)
	}
	policy, err := policyFor(record.Purpose)
	if err != nil {
		return Asset{}, err
	}
	var object objectRecord
	err = service.store.WithFileCommit(func() error {
		if err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var locked uploadRecord
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, record.ID).Error; err != nil {
				return err
			}
			if locked.State == StateConsumed {
				object.ID = 0
				record = locked
				return nil
			}
			if locked.State != StateReady && locked.State != StateConsuming {
				return fileError(CodeInvalidState, "上传状态已变化", "请刷新后重试 finalize。", nil)
			}
			err := tx.Where("project_id = ? AND sha256 = ?", locked.ProjectID, *locked.SHA256).First(&object).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				objectUUID, uuidErr := newUUIDv7()
				if uuidErr != nil {
					return uuidErr
				}
				stem := safeStem(locked.DisplayName, safeStem(locked.OriginalFilename, kindForMIME(*locked.MIMEType)))
				keyPath := policy.Namespace + "/" + stem + "--" + locked.ReservedFileUUID + "." + *locked.CanonicalExt
				if err := validateKeyPath(keyPath); err != nil {
					return err
				}
				object = objectRecord{UUID: objectUUID, ProjectID: locked.ProjectID, SHA256: *locked.SHA256, KeyPath: keyPath, MIMEType: *locked.MIMEType, CanonicalExt: *locked.CanonicalExt, ByteSize: *locked.ByteSize, Width: locked.Width, Height: locked.Height, DurationMS: locked.DurationMS, State: ObjectPending, CreatedAt: service.now().UTC()}
				if err := tx.Create(&object).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
			if object.State == ObjectMissing || object.State == ObjectCorrupt || object.State == ObjectQuarantined {
				return fileError(CodeObjectUnavailable, "相同内容对象不可用", "完整性扫描已将同 hash 对象标记为不可读取，请先修复或隔离。", nil)
			}
			record = locked
			return tx.Model(&uploadRecord{}).Where("id = ?", locked.ID).Updates(map[string]any{"state": StateConsuming, "file_object_id": object.ID, "updated_at": service.now().UTC()}).Error
		}); err != nil {
			return err
		}
		if record.State == StateConsumed {
			return nil
		}
		if object.State == ObjectPending {
			if err := service.publishObject(record, object); err != nil {
				return err
			}
		}
		return service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var currentObject objectRecord
			if err := tx.First(&currentObject, object.ID).Error; err != nil {
				return err
			}
			if err := service.verifyObjectFile(currentObject); err != nil {
				return err
			}
			now := service.now().UTC()
			if currentObject.State != ObjectReady {
				if err := tx.Model(&objectRecord{}).Where("id = ? AND state = ?", currentObject.ID, ObjectPending).Updates(map[string]any{"state": ObjectReady, "verified_at": now}).Error; err != nil {
					return err
				}
			}
			var sourceFileID *int64
			if sourceAssetUUID != "" {
				if !isUUIDv7(sourceAssetUUID) {
					return fileError(CodeAssetNotFound, "来源 Asset 无效", "source_asset_uuid 必须是 UUIDv7。", nil)
				}
				var source fileRecord
				if err := tx.Where("project_id = ? AND uuid = ?", record.ProjectID, sourceAssetUUID).First(&source).Error; err != nil {
					return fileError(CodeAssetNotFound, "来源 Asset 不存在", "派生文件必须引用当前项目的正式 Asset。", err)
				}
				sourceFileID = &source.ID
			}
			var finalized fileRecord
			fileErr := tx.Where("uuid = ?", record.ReservedFileUUID).First(&finalized).Error
			if errors.Is(fileErr, gorm.ErrRecordNotFound) {
				original := record.OriginalFilename
				var display *string
				if strings.TrimSpace(record.DisplayName) != "" {
					value := strings.TrimSpace(record.DisplayName)
					display = &value
				}
				actorID := record.ActorID
				finalized = fileRecord{UUID: record.ReservedFileUUID, ProjectID: record.ProjectID, FileObjectID: currentObject.ID, Kind: kindForMIME(currentObject.MIMEType), Purpose: record.Purpose, OriginalFilename: &original, DisplayName: display, SourceType: sourceType, SourceFileID: sourceFileID, MetadataJSON: record.MetadataJSON, ActorID: &actorID, CreatedAt: now}
				if err := tx.Create(&finalized).Error; err != nil {
					return err
				}
			} else if fileErr != nil {
				return fileErr
			}
			if bind != nil {
				if err := bind(tx, finalized.ID); err != nil {
					return err
				}
			}
			result := tx.Model(&uploadRecord{}).Where("id = ? AND state = ?", record.ID, StateConsuming).Updates(map[string]any{"state": StateConsumed, "finalized_file_id": finalized.ID, "consumed_at": now, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fileError(CodeInvalidState, "上传 finalize 冲突", "提交事实没有被重复写入。", nil)
			}
			return nil
		})
	})
	if err != nil {
		return Asset{}, err
	}
	latest, err := service.uploadRecord(ctx, uploadUUID)
	if err != nil {
		return Asset{}, err
	}
	if latest.FinalizedFileID == nil {
		return Asset{}, fileError(CodeOperationUnavailable, "Asset 提交尚未完成", "reconcile 将保留并恢复此提交。", nil)
	}
	var finalized fileRecord
	if err := service.store.DB().WithContext(ctx).First(&finalized, *latest.FinalizedFileID).Error; err != nil {
		return Asset{}, err
	}
	asset, err := service.GetAsset(ctx, finalized.UUID, true)
	if err == nil {
		if part, pathErr := service.partPath(uploadUUID); pathErr == nil {
			_ = os.Remove(part)
		}
		service.emit("asset/created", map[string]any{"asset_uuid": asset.UUID, "upload_uuid": uploadUUID, "status": asset.Status})
	}
	return asset, err
}

func (service *Service) publishObject(upload uploadRecord, object objectRecord) error {
	part, err := service.partPath(upload.UUID)
	if err != nil {
		return err
	}
	target, err := service.assetPath(object.KeyPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fileError(CodeOperationUnavailable, "无法创建 Asset 目录", "正式对象尚未提交。", err)
	}
	if _, err := service.assetPath(object.KeyPath); err != nil {
		return err
	}
	if info, statErr := os.Lstat(target); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fileError(CodeUnsafePath, "Asset 目标路径不安全", "目标必须是项目内普通文件。", nil)
		}
		return service.verifyObjectFile(object)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if err := os.Link(part, target); err != nil {
		if errors.Is(err, os.ErrExist) {
			return service.verifyObjectFile(object)
		}
		return fileError(CodeOperationUnavailable, "Asset 原子提交失败", "暂存文件已保留，reconcile 可以安全重试。", err)
	}
	if err := syncDirectory(filepath.Dir(target)); err != nil {
		return fileError(CodeOperationUnavailable, "Asset 目录同步失败", "对象已写入但尚未确认持久化。", err)
	}
	_ = os.Remove(part)
	return nil
}

func syncDirectory(path string) error {
	return durablefs.SyncDirectory(path)
}

func (service *Service) verifyObjectFile(object objectRecord) error {
	policy, err := policyForObject(object)
	if err != nil {
		return err
	}
	path, err := service.assetPath(object.KeyPath)
	if err != nil {
		return err
	}
	actual, err := inspectContent(path, filepath.Base(object.KeyPath), policy)
	if err != nil {
		return err
	}
	if actual.SHA256 != object.SHA256 || actual.ByteSize != object.ByteSize || actual.MIMEType != object.MIMEType {
		return fileError(CodeInvalidContent, "Asset 完整性校验失败", "磁盘内容与 pending object 摘要不一致。", nil)
	}
	return nil
}

func policyForObject(object objectRecord) (purposePolicy, error) {
	for _, policy := range purposeRegistry {
		if strings.HasPrefix(object.KeyPath, policy.Namespace+"/") {
			return policy, nil
		}
	}
	return purposePolicy{}, fileError(CodeUnsafePath, "Asset namespace 未注册", "对象路径不属于任何允许的 purpose namespace。", nil)
}
