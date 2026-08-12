package files

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lumi/internal/project"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EventPublisher interface {
	Broadcast(topic, event string, payload any)
}

type Service struct {
	store  *project.Store
	events EventPublisher
	now    func() time.Time
}

func NewService(store *project.Store, events EventPublisher) *Service {
	return &Service{store: store, events: events, now: time.Now}
}

func newUUIDv7() (string, error) {
	value, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate UUIDv7: %w", err)
	}
	return value.String(), nil
}

func isUUIDv7(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.Version() == 7
}

func (service *Service) projectAndActor(ctx context.Context, db *gorm.DB) (project.Project, project.Actor, error) {
	var projectRecord project.Project
	if err := db.WithContext(ctx).Where("uuid = ?", service.store.ProjectUUID()).First(&projectRecord).Error; err != nil {
		return projectRecord, project.Actor{}, err
	}
	var actor project.Actor
	if err := db.WithContext(ctx).Where("kind = ?", "local_user").Order("id ASC").First(&actor).Error; err != nil {
		return projectRecord, actor, err
	}
	return projectRecord, actor, nil
}

func (service *Service) partPath(operationUUID string) (string, error) {
	if !isUUIDv7(operationUUID) {
		return "", fileError(CodeUnsafePath, "临时文件 UUID 无效", "临时文件只能由服务端 UUIDv7 定位。", nil)
	}
	resolved, err := service.store.ResolvePath(filepath.ToSlash(filepath.Join(".lumi", "tmp", operationUUID+".part")))
	if err != nil {
		return "", fileError(CodeUnsafePath, "上传暂存路径不安全", "项目临时目录必须位于受管根内且不能经过 symlink。", err)
	}
	return resolved, nil
}

func (service *Service) assetPath(keyPath string) (string, error) {
	if err := validateKeyPath(keyPath); err != nil {
		return "", err
	}
	resolved, err := service.store.ResolvePath(filepath.ToSlash(filepath.Join("assets", filepath.FromSlash(keyPath))))
	if err != nil {
		return "", fileError(CodeUnsafePath, "Asset 路径不安全", "正式对象路径必须位于 assets 根内且不能经过 symlink。", err)
	}
	return resolved, nil
}

func (service *Service) quarantinePath(objectUUID, ext string) (string, error) {
	if !isUUIDv7(objectUUID) {
		return "", fileError(CodeUnsafePath, "隔离对象 UUID 无效", "对象身份不是 UUIDv7。", nil)
	}
	name := objectUUID
	if ext != "" {
		name += "." + safeStem(ext, "bin")
	}
	resolved, err := service.store.ResolvePath(filepath.ToSlash(filepath.Join(".lumi", "quarantine", name)))
	if err != nil {
		return "", fileError(CodeUnsafePath, "隔离路径不安全", "quarantine 必须位于项目受管根内且不能经过 symlink。", err)
	}
	return resolved, nil
}

func (service *Service) emit(event string, payload map[string]any) {
	if service.events == nil {
		return
	}
	// Realtime is a refresh hint. A broken publisher must never unwind a
	// durable SQLite/filesystem commit that already succeeded.
	defer func() { _ = recover() }()
	payload["project_uuid"] = service.store.ProjectUUID()
	service.events.Broadcast("project:"+service.store.ProjectUUID(), event, payload)
}

func (service *Service) emitObjectAssets(ctx context.Context, objectID int64, event, status string) {
	if service.events == nil {
		return
	}
	var rows []fileRecord
	if err := service.store.DB().WithContext(ctx).Where("file_object_id = ?", objectID).Find(&rows).Error; err != nil {
		return
	}
	for _, row := range rows {
		service.emit(event, map[string]any{"asset_uuid": row.UUID, "status": status})
	}
}

func (service *Service) CreateUpload(ctx context.Context, input CreateUploadInput) (Upload, error) {
	if input.Reader == nil {
		return Upload{}, fileError(CodeValidationFailed, "上传内容为空", "multipart 请求必须包含 file。", nil)
	}
	policy, err := policyFor(input.Purpose)
	if err != nil {
		return Upload{}, err
	}
	projectRecord, actor, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return Upload{}, err
	}
	uploadUUID, err := newUUIDv7()
	if err != nil {
		return Upload{}, err
	}
	fileUUID, err := newUUIDv7()
	if err != nil {
		return Upload{}, err
	}
	now := service.now().UTC()
	filtered, err := filteredMetadata(policy, input.Metadata)
	if err != nil {
		return Upload{}, err
	}
	metadata, err := json.Marshal(filtered)
	if err != nil {
		return Upload{}, fileError(CodeValidationFailed, "Asset metadata 无效", "metadata 必须可编码为 JSON。", err)
	}
	record := uploadRecord{UUID: uploadUUID, ProjectID: projectRecord.ID, ActorID: actor.ID, ReservedFileUUID: fileUUID, Purpose: strings.TrimSpace(input.Purpose), OriginalFilename: canonicalFilename(input.OriginalFilename), DisplayName: strings.TrimSpace(input.DisplayName), MetadataJSON: string(metadata), State: StateReceiving, ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now, UpdatedAt: now}
	if err := service.store.DB().WithContext(ctx).Create(&record).Error; err != nil {
		return Upload{}, err
	}
	operationErr := service.store.WithFileCommit(func() error {
		path, err := service.partPath(record.UUID)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return fileError(CodeOperationUnavailable, "无法创建上传暂存文件", "请检查项目临时目录与可用空间。", err)
		}
		limited := &io.LimitedReader{R: input.Reader, N: policy.MaxBytes + 1}
		written, copyErr := io.Copy(file, limited)
		syncErr := file.Sync()
		closeErr := file.Close()
		if copyErr != nil || syncErr != nil || closeErr != nil {
			return fileError(CodeOperationUnavailable, "上传暂存写入失败", "文件尚未完成，稍后可安全重试。", errors.Join(copyErr, syncErr, closeErr))
		}
		if written > policy.MaxBytes {
			return fileError(CodeFileTooLarge, "Asset 超过大小限制", fmt.Sprintf("purpose %s 最大允许 %d 字节。", record.Purpose, policy.MaxBytes), nil)
		}
		inspection, err := inspectContent(path, record.OriginalFilename, policy)
		if err != nil {
			return err
		}
		return service.store.DB().WithContext(ctx).Model(&uploadRecord{}).Where("id = ? AND state = ?", record.ID, StateReceiving).Updates(map[string]any{
			"mime_type": inspection.MIMEType, "canonical_ext": inspection.Extension, "byte_size": inspection.ByteSize,
			"sha256": inspection.SHA256, "width": inspection.Width, "height": inspection.Height, "duration_ms": inspection.DurationMS,
			"state": StateReady, "updated_at": service.now().UTC(),
		}).Error
	})
	if operationErr != nil {
		var domainErr *Error
		code := CodeOperationUnavailable
		if errors.As(operationErr, &domainErr) {
			code = domainErr.Code
		}
		_ = service.store.DB().WithContext(context.WithoutCancel(ctx)).Model(&uploadRecord{}).Where("id = ?", record.ID).Updates(map[string]any{"state": StateFailed, "error_code": code, "updated_at": service.now().UTC()}).Error
		if path, pathErr := service.partPath(record.UUID); pathErr == nil {
			_ = os.Remove(path)
		}
		return Upload{}, operationErr
	}
	upload, err := service.GetUpload(ctx, record.UUID)
	if err == nil {
		service.emit("upload/completed", map[string]any{"upload_uuid": upload.UUID, "status": upload.State})
	}
	return upload, err
}

func (service *Service) GetUpload(ctx context.Context, uploadUUID string) (Upload, error) {
	if !isUUIDv7(uploadUUID) {
		return Upload{}, fileError(CodeUploadNotFound, "上传记录不存在", "upload_uuid 必须是 UUIDv7。", nil)
	}
	var record uploadRecord
	err := service.store.DB().WithContext(ctx).Where("project_id = (SELECT id FROM projects WHERE uuid = ?) AND uuid = ?", service.store.ProjectUUID(), uploadUUID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Upload{}, fileError(CodeUploadNotFound, "上传记录不存在", "该暂存上传不属于当前项目。", err)
	}
	if err != nil {
		return Upload{}, err
	}
	return service.uploadDTO(ctx, record)
}

func (service *Service) uploadDTO(ctx context.Context, record uploadRecord) (Upload, error) {
	result := Upload{UUID: record.UUID, Purpose: record.Purpose, OriginalFilename: record.OriginalFilename, DisplayName: record.DisplayName, ByteSize: record.ByteSize, Width: record.Width, Height: record.Height, DurationMS: record.DurationMS, State: record.State, ErrorCode: record.ErrorCode, ExpiresAt: record.ExpiresAt, ConsumedAt: record.ConsumedAt, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
	if record.MIMEType != nil {
		result.MIMEType = *record.MIMEType
	}
	if record.SHA256 != nil {
		result.SHA256 = *record.SHA256
	}
	if record.FinalizedFileID != nil {
		var file fileRecord
		if err := service.store.DB().WithContext(ctx).First(&file, *record.FinalizedFileID).Error; err != nil {
			return Upload{}, err
		}
		result.AssetUUID = file.UUID
	}
	return result, nil
}

func (service *Service) CancelUpload(ctx context.Context, uploadUUID string) error {
	record, err := service.uploadRecord(ctx, uploadUUID)
	if err != nil {
		return err
	}
	if record.State == StateConsumed || record.State == StateConsuming {
		return fileError(CodeUploadConsumed, "上传正在或已经被消费", "正式 Asset 不会被取消上传操作删除。", nil)
	}
	now := service.now().UTC()
	result := service.store.DB().WithContext(ctx).Model(&uploadRecord{}).Where("id = ? AND state IN ?", record.ID, []string{StateReceiving, StateReady, StateFailed}).Updates(map[string]any{"state": StateExpired, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fileError(CodeInvalidState, "上传状态已变化", "请刷新上传状态后重试。", nil)
	}
	if part, pathErr := service.partPath(record.UUID); pathErr == nil {
		_ = os.Remove(part)
	}
	service.emit("upload/expired", map[string]any{"upload_uuid": record.UUID, "status": StateExpired})
	return nil
}

func (service *Service) uploadRecord(ctx context.Context, uploadUUID string) (uploadRecord, error) {
	if !isUUIDv7(uploadUUID) {
		return uploadRecord{}, fileError(CodeUploadNotFound, "上传记录不存在", "upload_uuid 必须是 UUIDv7。", nil)
	}
	var record uploadRecord
	err := service.store.DB().WithContext(ctx).Where("uuid = ? AND project_id = (SELECT id FROM projects WHERE uuid = ?)", uploadUUID, service.store.ProjectUUID()).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, fileError(CodeUploadNotFound, "上传记录不存在", "该上传不属于当前项目。", err)
	}
	return record, err
}

func (service *Service) assetRowByUUID(ctx context.Context, assetUUID string, includeDeleted bool) (assetRow, error) {
	if !isUUIDv7(assetUUID) {
		return assetRow{}, fileError(CodeAssetNotFound, "Asset 不存在", "asset_uuid 必须是 UUIDv7。", nil)
	}
	query := service.assetQuery(ctx).Where("f.uuid = ?", assetUUID)
	if !includeDeleted {
		query = query.Where("f.deleted_at IS NULL")
	}
	var row assetRow
	err := query.Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, fileError(CodeAssetNotFound, "Asset 不存在", "该 Asset 不属于当前项目或已进入回收站。", err)
	}
	return row, err
}

func (service *Service) assetQuery(ctx context.Context) *gorm.DB {
	return service.store.DB().WithContext(ctx).Table("files AS f").Select(`f.*, o.uuid AS object_uuid, o.sha256, o.key_path, o.mime_type, o.canonical_ext, o.byte_size, o.width, o.height, o.duration_ms, o.state AS object_state, o.verified_at, COALESCE(sf.uuid, '') AS source_asset_uuid`).Joins("JOIN file_objects AS o ON o.id = f.file_object_id").Joins("LEFT JOIN files AS sf ON sf.id = f.source_file_id").Where("f.project_id = (SELECT id FROM projects WHERE uuid = ?)", service.store.ProjectUUID())
}

func (service *Service) assetDTO(row assetRow) Asset {
	asset := Asset{UUID: row.UUID, Kind: row.Kind, Purpose: row.Purpose, SourceType: row.SourceType, SourceAssetUUID: row.SourceAssetUUID, MIMEType: row.MIMEType, ByteSize: row.ByteSize, Width: row.Width, Height: row.Height, DurationMS: row.DurationMS, Status: row.ObjectState, Metadata: decodeMetadata(row.MetadataJSON), ContentURL: "/media/projects/" + service.store.ProjectUUID() + "/assets/" + row.UUID + "/content", DeletedAt: row.DeletedAt, CreatedAt: row.CreatedAt}
	if row.OriginalFilename != nil {
		asset.OriginalFilename = *row.OriginalFilename
	}
	if row.DisplayName != nil {
		asset.DisplayName = *row.DisplayName
	}
	return asset
}

func (service *Service) GetAsset(ctx context.Context, assetUUID string, includeDeleted bool) (Asset, error) {
	row, err := service.assetRowByUUID(ctx, assetUUID, includeDeleted)
	if err != nil {
		return Asset{}, err
	}
	return service.assetDTO(row), nil
}

func (service *Service) ListAssets(ctx context.Context, filter AssetFilter) ([]Asset, error) {
	query := service.assetQuery(ctx)
	if filter.TrashedOnly {
		query = query.Where("f.deleted_at IS NOT NULL")
	} else if !filter.IncludeTrashed {
		query = query.Where("f.deleted_at IS NULL")
	}
	if filter.Purpose != "" {
		if _, err := policyFor(filter.Purpose); err != nil {
			return nil, err
		}
		query = query.Where("f.purpose = ?", filter.Purpose)
	}
	if filter.Kind != "" {
		query = query.Where("f.kind = ?", filter.Kind)
	}
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 100
	}
	var rows []assetRow
	if err := query.Order("f.created_at DESC, f.id DESC").Limit(filter.Limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]Asset, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.assetDTO(row))
	}
	return items, nil
}

func (service *Service) SoftDelete(ctx context.Context, assetUUID string) (Asset, error) {
	row, err := service.assetRowByUUID(ctx, assetUUID, true)
	if err != nil {
		return Asset{}, err
	}
	if row.DeletedAt == nil {
		now := service.now().UTC()
		if err := service.store.DB().WithContext(ctx).Model(&fileRecord{}).Where("id = ? AND deleted_at IS NULL", row.ID).Update("deleted_at", now).Error; err != nil {
			return Asset{}, err
		}
	}
	asset, err := service.GetAsset(ctx, assetUUID, true)
	if err == nil {
		service.emit("asset/trashed", map[string]any{"asset_uuid": asset.UUID, "status": "trashed"})
	}
	return asset, err
}

func (service *Service) Restore(ctx context.Context, assetUUID string) (Asset, error) {
	row, err := service.assetRowByUUID(ctx, assetUUID, true)
	if err != nil {
		return Asset{}, err
	}
	if row.DeletedAt != nil {
		if err := service.store.DB().WithContext(ctx).Model(&fileRecord{}).Where("id = ?", row.ID).Update("deleted_at", nil).Error; err != nil {
			return Asset{}, err
		}
	}
	asset, err := service.GetAsset(ctx, assetUUID, true)
	if err == nil {
		service.emit("asset/restored", map[string]any{"asset_uuid": asset.UUID, "status": asset.Status})
	}
	return asset, err
}

func (service *Service) UpdateAsset(ctx context.Context, assetUUID string, input UpdateAssetInput) (Asset, error) {
	row, err := service.assetRowByUUID(ctx, assetUUID, false)
	if err != nil {
		return Asset{}, err
	}
	updates := map[string]any{}
	if input.DisplayName != nil {
		value := strings.TrimSpace(*input.DisplayName)
		if len([]rune(value)) > 255 {
			return Asset{}, fileError(CodeValidationFailed, "Asset 展示名称过长", "display_name 不能超过 255 个字符。", nil)
		}
		updates["display_name"] = value
	}
	if input.Metadata != nil {
		policy, policyErr := policyFor(row.Purpose)
		if policyErr != nil {
			return Asset{}, policyErr
		}
		filtered, filterErr := filteredMetadata(policy, input.Metadata)
		if filterErr != nil {
			return Asset{}, filterErr
		}
		encoded, encodeErr := json.Marshal(filtered)
		if encodeErr != nil {
			return Asset{}, fileError(CodeValidationFailed, "Asset metadata 无效", "metadata 必须可编码为 JSON。", encodeErr)
		}
		updates["metadata_json"] = string(encoded)
	}
	if len(updates) > 0 {
		if err := service.store.DB().WithContext(ctx).Model(&fileRecord{}).Where("id = ? AND deleted_at IS NULL", row.ID).Updates(updates).Error; err != nil {
			return Asset{}, err
		}
	}
	asset, err := service.GetAsset(ctx, assetUUID, false)
	if err == nil {
		service.emit("asset/updated", map[string]any{"asset_uuid": asset.UUID, "status": asset.Status})
	}
	return asset, err
}
