package agent

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"lumi/internal/files"
	"lumi/internal/project"

	"gorm.io/gorm"
)

const maxChatImageReferences = 4

type storedImageReference struct {
	FileID   int64
	UploadID *int64
}

type imageReferenceRow struct {
	UploadUUID       string
	FileUUID         string
	OriginalFilename string
	MIMEType         string
	ByteSize         int64
	Width            *int
	Height           *int
}

func threadAllowsImageReferences(thread threadRecord) bool {
	definition, ok := sceneDefinitionForThread(thread)
	return ok && definition.ImageReferencePolicy != ImageReferenceNone
}

func (service *Service) finalizeChatImageReferences(ctx context.Context, store *project.Store, thread threadRecord, uploadUUIDs []string) ([]storedImageReference, error) {
	uploadUUIDs, err := normalizeUploadUUIDs(uploadUUIDs)
	if err != nil {
		return nil, err
	}
	if len(uploadUUIDs) == 0 {
		return nil, nil
	}
	if !threadAllowsImageReferences(thread) {
		return nil, domainError(CodeImageReferenceUnsupported, "当前场景不支持图片附件", "图片附件只支持 AI 生成设定项和设定项引用会话。", nil)
	}
	fileService := files.NewService(store, service.hub)
	for _, uploadUUID := range uploadUUIDs {
		var owner struct {
			ProjectID int64
		}
		lookup := store.DB().WithContext(ctx).Table("upload_stashed").Select("project_id").Where("uuid=?", uploadUUID).Take(&owner)
		if errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
			return nil, domainError(CodeImageReferenceNotFound, "图片附件不存在", "附件必须来自当前项目的有效上传。", lookup.Error)
		}
		if lookup.Error != nil {
			return nil, lookup.Error
		}
		if owner.ProjectID != thread.ProjectID {
			return nil, domainError(CodeImageReferenceProject, "图片附件不属于当前项目", "不能跨项目引用上传文件。", nil)
		}
		upload, getErr := fileService.GetUpload(ctx, uploadUUID)
		if getErr != nil {
			return nil, domainError(CodeImageReferenceNotFound, "图片附件不存在", "附件必须来自当前项目的有效上传。", getErr)
		}
		if upload.Purpose != "project_chatbot_reference" || (upload.State != files.StateReady && upload.State != files.StateConsumed) {
			return nil, domainError(CodeImageReferenceIncomplete, "图片附件尚未就绪", "附件必须是 ready 的 project_chatbot_reference 上传。", nil)
		}
		mimeType := strings.ToLower(strings.TrimSpace(upload.MIMEType))
		if mimeType != "image/png" && mimeType != "image/jpeg" && mimeType != "image/webp" {
			return nil, domainError(CodeImageReferenceInvalidMIME, "图片附件格式无效", "附件只支持 PNG、JPEG 或 WebP 图片。", nil)
		}
	}

	references := make([]storedImageReference, 0, len(uploadUUIDs))
	for _, uploadUUID := range uploadUUIDs {
		asset, finalizeErr := fileService.FinalizeUpload(ctx, uploadUUID, "project_chatbot_reference")
		if finalizeErr != nil {
			return nil, domainError(CodeImageReferenceIncomplete, "图片附件无法持久化", "附件状态已变化，请重新选择图片。", finalizeErr)
		}
		var fileID, uploadID int64
		if err := store.DB().WithContext(ctx).Raw(`SELECT f.id,u.id FROM upload_stashed u JOIN files f ON f.id=u.finalized_file_id WHERE u.project_id=? AND u.uuid=? AND f.uuid=? AND f.deleted_at IS NULL`, thread.ProjectID, uploadUUID, asset.UUID).Row().Scan(&fileID, &uploadID); err != nil {
			return nil, domainError(CodeImageReferenceNotFound, "图片附件文件不可用", "附件对应文件已删除或不再属于当前项目。", err)
		}
		references = append(references, storedImageReference{FileID: fileID, UploadID: &uploadID})
	}
	return references, nil
}

func normalizeUploadUUIDs(values []string) ([]string, error) {
	if len(values) > maxChatImageReferences {
		return nil, domainError(CodeImageReferenceLimit, "图片附件过多", "每条消息最多附加 4 张图片。", nil)
	}
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !isUUIDv7(value) {
			return nil, domainError(CodeImageReferenceInvalidUUID, "图片附件 UUID 无效", "upload_uuids 必须是 UUIDv7 数组。", nil)
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result, nil
}

func attachItemImageReferencesTx(ctx context.Context, tx *sql.Tx, itemID int64, references []storedImageReference, now time.Time) error {
	for index, reference := range references {
		uuid, err := newUUIDv7()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO chat_item_file_references(uuid,chat_item_id,file_id,upload_stashed_id,position,created_at) VALUES(?,?,?,?,?,?)`, uuid, itemID, reference.FileID, reference.UploadID, index+1, now); err != nil {
			return err
		}
	}
	return nil
}

func attachFollowUpImageReferencesTx(ctx context.Context, tx *sql.Tx, followUpID int64, references []storedImageReference, now time.Time) error {
	for index, reference := range references {
		uuid, err := newUUIDv7()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO chat_follow_up_file_references(uuid,follow_up_id,file_id,upload_stashed_id,position,created_at) VALUES(?,?,?,?,?,?)`, uuid, followUpID, reference.FileID, reference.UploadID, index+1, now); err != nil {
			return err
		}
	}
	return nil
}

func loadFollowUpImageReferencesTx(ctx context.Context, tx *sql.Tx, followUpID int64) ([]storedImageReference, error) {
	rows, err := tx.QueryContext(ctx, `SELECT file_id,upload_stashed_id FROM chat_follow_up_file_references WHERE follow_up_id=? ORDER BY position,id`, followUpID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStoredImageReferences(rows)
}

func loadLatestTurnImageReferencesTx(ctx context.Context, tx *sql.Tx, turnID int64) ([]storedImageReference, error) {
	rows, err := tx.QueryContext(ctx, `SELECT refs.file_id,refs.upload_stashed_id FROM chat_items items JOIN chat_item_file_references refs ON refs.chat_item_id=items.id WHERE items.turn_id=? AND items.item_type='user_message' AND items.id=(SELECT latest.id FROM chat_items latest WHERE latest.turn_id=? AND latest.item_type='user_message' AND EXISTS (SELECT 1 FROM chat_item_file_references latest_refs WHERE latest_refs.chat_item_id=latest.id) ORDER BY latest.sequence DESC,latest.id DESC LIMIT 1) ORDER BY refs.position,refs.id`, turnID, turnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStoredImageReferences(rows)
}

type storedImageReferenceScanner interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func scanStoredImageReferences(rows storedImageReferenceScanner) ([]storedImageReference, error) {
	result := []storedImageReference{}
	for rows.Next() {
		var fileID int64
		var uploadID sql.NullInt64
		if err := rows.Scan(&fileID, &uploadID); err != nil {
			return nil, err
		}
		var upload *int64
		if uploadID.Valid {
			value := uploadID.Int64
			upload = &value
		}
		result = append(result, storedImageReference{FileID: fileID, UploadID: upload})
	}
	return result, rows.Err()
}

func (service *Service) itemImageReferences(ctx context.Context, store *project.Store, itemID int64) ([]ImageReference, error) {
	return service.queryImageReferences(ctx, store, `SELECT COALESCE(u.uuid,''),f.uuid,COALESCE(f.original_filename,''),o.mime_type,o.byte_size,o.width,o.height FROM chat_item_file_references refs JOIN files f ON f.id=refs.file_id JOIN file_objects o ON o.id=f.file_object_id LEFT JOIN upload_stashed u ON u.id=refs.upload_stashed_id WHERE refs.chat_item_id=? ORDER BY refs.position,refs.id`, itemID)
}

func (service *Service) followUpImageReferences(ctx context.Context, store *project.Store, followUpID int64) ([]ImageReference, error) {
	return service.queryImageReferences(ctx, store, `SELECT COALESCE(u.uuid,''),f.uuid,COALESCE(f.original_filename,''),o.mime_type,o.byte_size,o.width,o.height FROM chat_follow_up_file_references refs JOIN files f ON f.id=refs.file_id JOIN file_objects o ON o.id=f.file_object_id LEFT JOIN upload_stashed u ON u.id=refs.upload_stashed_id WHERE refs.follow_up_id=? ORDER BY refs.position,refs.id`, followUpID)
}

func (service *Service) queryImageReferences(ctx context.Context, store *project.Store, query string, ownerID int64) ([]ImageReference, error) {
	rows, err := store.DB().WithContext(ctx).Raw(query, ownerID).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ImageReference{}
	for rows.Next() {
		var row imageReferenceRow
		if err := rows.Scan(&row.UploadUUID, &row.FileUUID, &row.OriginalFilename, &row.MIMEType, &row.ByteSize, &row.Width, &row.Height); err != nil {
			return nil, err
		}
		result = append(result, ImageReference{UploadUUID: row.UploadUUID, FileUUID: row.FileUUID, OriginalFilename: row.OriginalFilename, MIMEType: row.MIMEType, ByteSize: row.ByteSize, Width: row.Width, Height: row.Height, ContentURL: fmt.Sprintf("/media/projects/%s/assets/%s/content", store.ProjectUUID(), row.FileUUID)})
	}
	return result, rows.Err()
}
