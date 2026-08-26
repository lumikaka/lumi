package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"lumi/internal/files"
	"lumi/internal/project"

	"gorm.io/gorm"
)

type storedContextReference struct {
	ResourceType   string
	ResourceUUID   string
	SnapshotJSON   string
	FileID         *int64
	PremiseAssetID *int64
	ComicSectionID *int64
	ImageFileID    *int64
}

type referenceRow struct {
	ResourceType string
	ResourceUUID string
	Position     int
	SnapshotJSON string
	ImageFileID  sql.NullInt64
	ImageReady   bool
}

func normalizeReferenceInputs(values []ReferenceInput) ([]ReferenceInput, error) {
	if len(values) > MaxContextReferences {
		return nil, domainError(CodeReferenceLimit, "Reference 过多", "每条用户输入最多携带 16 个 Reference。", nil)
	}
	result := make([]ReferenceInput, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.ResourceType = strings.ToLower(strings.TrimSpace(value.ResourceType))
		value.ResourceUUID = strings.TrimSpace(value.ResourceUUID)
		switch value.ResourceType {
		case ReferenceTypeFile, ReferenceTypePremiseAsset, ReferenceTypeComicSection:
		default:
			return nil, domainError(CodeReferenceInvalidType, "Reference 类型无效", "resource_type 只支持 file、premise_asset 或 comic_section。", nil)
		}
		if !isUUIDv7(value.ResourceUUID) {
			return nil, domainError(CodeReferenceInvalidUUID, "Reference UUID 无效", "resource_uuid 必须是 UUIDv7。", nil)
		}
		key := value.ResourceType + ":" + value.ResourceUUID
		if _, exists := seen[key]; exists {
			return nil, domainError(CodeReferenceDuplicate, "Reference 重复", "同一条用户输入不能重复引用同一资源。", nil)
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func (service *Service) resolveContextReferences(ctx context.Context, store *project.Store, projectID int64, inputs []ReferenceInput) ([]storedContextReference, error) {
	normalized, err := normalizeReferenceInputs(inputs)
	if err != nil {
		return nil, err
	}
	result := make([]storedContextReference, 0, len(normalized))
	for _, input := range normalized {
		var resolved storedContextReference
		switch input.ResourceType {
		case ReferenceTypeFile:
			resolved, err = resolveFileReference(ctx, store, projectID, input.ResourceUUID)
		case ReferenceTypePremiseAsset:
			resolved, err = resolvePremiseAssetReference(ctx, store, projectID, input.ResourceUUID)
		case ReferenceTypeComicSection:
			resolved, err = resolveComicSectionReference(ctx, store, projectID, input.ResourceUUID)
		}
		if err != nil {
			return nil, err
		}
		result = append(result, resolved)
	}
	return result, nil
}

func resolveFileReference(ctx context.Context, store *project.Store, projectID int64, resourceUUID string) (storedContextReference, error) {
	var row struct {
		ID               int64
		ProjectID        int64
		UUID             string
		Kind             string
		OriginalFilename string
		DisplayName      string
		DeletedAt        *time.Time
		MIMEType         string
		ByteSize         int64
		Width            *int
		Height           *int
		ObjectState      string
	}
	err := store.DB().WithContext(ctx).Table("files AS files").
		Select("files.id,files.project_id,files.uuid,files.kind,COALESCE(files.original_filename,'') AS original_filename,COALESCE(files.display_name,'') AS display_name,files.deleted_at,objects.mime_type,objects.byte_size,objects.width,objects.height,objects.state AS object_state").
		Joins("JOIN file_objects AS objects ON objects.id=files.file_object_id").
		Where("files.uuid=?", resourceUUID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return storedContextReference{}, referenceNotFound(resourceUUID)
	}
	if err != nil {
		return storedContextReference{}, err
	}
	if row.ProjectID != projectID {
		return storedContextReference{}, domainError(CodeReferenceProject, "Reference 不属于当前项目", "不能跨项目引用文件。", nil)
	}
	if row.DeletedAt != nil || row.Kind != "image" || row.ObjectState != files.ObjectReady || !strings.HasPrefix(strings.ToLower(row.MIMEType), "image/") {
		return storedContextReference{}, referenceNotFound(resourceUUID)
	}
	snapshot := map[string]any{
		"resource_type": ReferenceTypeFile, "resource_uuid": row.UUID, "status": "available",
		"name": firstNonEmpty(row.DisplayName, row.OriginalFilename), "original_filename": row.OriginalFilename,
		"mime_type": row.MIMEType, "byte_size": row.ByteSize, "width": row.Width, "height": row.Height,
	}
	encoded, err := encodeReferenceSnapshot(snapshot, nil)
	if err != nil {
		return storedContextReference{}, err
	}
	id := row.ID
	return storedContextReference{ResourceType: ReferenceTypeFile, ResourceUUID: row.UUID, SnapshotJSON: encoded, FileID: &id, ImageFileID: &id}, nil
}

func resolvePremiseAssetReference(ctx context.Context, store *project.Store, projectID int64, resourceUUID string) (storedContextReference, error) {
	var row struct {
		ID                 int64
		ProjectID          int64
		UUID               string
		AssetType          string
		Title              string
		Summary            string
		Revision           int64
		DeletedAt          *time.Time
		CurrentVariantUUID string
		CurrentFileID      *int64
		CurrentFileUUID    string
	}
	err := store.DB().WithContext(ctx).Table("premise_assets AS assets").
		Select("assets.id,assets.project_id,assets.uuid,assets.asset_type,assets.title,assets.summary,assets.revision,assets.deleted_at,COALESCE(variants.uuid,'') AS current_variant_uuid,CASE WHEN files.deleted_at IS NULL AND objects.state='ready' THEN files.id ELSE NULL END AS current_file_id,CASE WHEN files.deleted_at IS NULL AND objects.state='ready' THEN COALESCE(files.uuid,'') ELSE '' END AS current_file_uuid").
		Joins("LEFT JOIN premise_asset_variants AS variants ON variants.id=assets.current_variant_id").
		Joins("LEFT JOIN files ON files.id=variants.file_id").
		Joins("LEFT JOIN file_objects AS objects ON objects.id=files.file_object_id").
		Where("assets.uuid=?", resourceUUID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return storedContextReference{}, referenceNotFound(resourceUUID)
	}
	if err != nil {
		return storedContextReference{}, err
	}
	if row.ProjectID != projectID {
		return storedContextReference{}, domainError(CodeReferenceProject, "Reference 不属于当前项目", "不能跨项目引用设定项。", nil)
	}
	if row.DeletedAt != nil {
		return storedContextReference{}, referenceNotFound(resourceUUID)
	}
	var tags []string
	if err := store.DB().WithContext(ctx).Table("premise_asset_tags").Where("premise_asset_id=?", row.ID).Order("tag").Pluck("tag", &tags).Error; err != nil {
		return storedContextReference{}, err
	}
	tags, tagsTruncated := compactReferenceTags(tags)
	snapshot := map[string]any{
		"resource_type": ReferenceTypePremiseAsset, "resource_uuid": row.UUID, "status": "available",
		"asset_type": row.AssetType, "title": row.Title, "summary": row.Summary, "tags": tags,
		"revision": row.Revision, "current_variant_uuid": row.CurrentVariantUUID, "current_file_uuid": row.CurrentFileUUID,
	}
	truncated := []string{}
	if tagsTruncated {
		truncated = append(truncated, "tags")
	}
	encoded, err := encodeReferenceSnapshot(snapshot, truncated)
	if err != nil {
		return storedContextReference{}, err
	}
	id := row.ID
	return storedContextReference{ResourceType: ReferenceTypePremiseAsset, ResourceUUID: row.UUID, SnapshotJSON: encoded, PremiseAssetID: &id, ImageFileID: row.CurrentFileID}, nil
}

func resolveComicSectionReference(ctx context.Context, store *project.Store, projectID int64, resourceUUID string) (storedContextReference, error) {
	var row struct {
		ID                    int64
		ProjectID             int64
		UUID                  string
		ChapterUUID           string
		SectionNo             int
		Title                 string
		Description           string
		Revision              int64
		DeletedAt             *time.Time
		ChapterDeletedAt      *time.Time
		CurrentStoryboardUUID string
		CurrentImageFileID    *int64
		CurrentImageFileUUID  string
	}
	err := store.DB().WithContext(ctx).Table("comic_sections AS sections").
		Select("sections.id,chapters.project_id,sections.uuid,chapters.uuid AS chapter_uuid,sections.section_no,sections.title,sections.description_md AS description,sections.revision,sections.deleted_at,chapters.deleted_at AS chapter_deleted_at,COALESCE(storyboards.uuid,'') AS current_storyboard_uuid,CASE WHEN files.deleted_at IS NULL AND objects.state='ready' THEN files.id ELSE NULL END AS current_image_file_id,CASE WHEN files.deleted_at IS NULL AND objects.state='ready' THEN COALESCE(files.uuid,'') ELSE '' END AS current_image_file_uuid").
		Joins("JOIN chapter_comic_states AS states ON states.id=sections.chapter_comic_state_id").
		Joins("JOIN chapters ON chapters.id=states.chapter_id").
		Joins("LEFT JOIN comic_storyboard_variants AS storyboards ON storyboards.id=sections.current_storyboard_variant_id").
		Joins("LEFT JOIN comic_image_variants AS images ON images.id=sections.current_image_variant_id").
		Joins("LEFT JOIN files ON files.id=images.file_id").
		Joins("LEFT JOIN file_objects AS objects ON objects.id=files.file_object_id").
		Where("sections.uuid=?", resourceUUID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return storedContextReference{}, referenceNotFound(resourceUUID)
	}
	if err != nil {
		return storedContextReference{}, err
	}
	if row.ProjectID != projectID {
		return storedContextReference{}, domainError(CodeReferenceProject, "Reference 不属于当前项目", "不能跨项目引用 Comic Section。", nil)
	}
	if row.DeletedAt != nil || row.ChapterDeletedAt != nil {
		return storedContextReference{}, referenceNotFound(resourceUUID)
	}
	snapshot := map[string]any{
		"resource_type": ReferenceTypeComicSection, "resource_uuid": row.UUID, "status": "available",
		"chapter_uuid": row.ChapterUUID, "section_no": row.SectionNo, "title": row.Title,
		"description": row.Description, "revision": row.Revision,
		"current_storyboard_uuid": row.CurrentStoryboardUUID, "current_image_file_uuid": row.CurrentImageFileUUID,
	}
	encoded, err := encodeReferenceSnapshot(snapshot, nil)
	if err != nil {
		return storedContextReference{}, err
	}
	id := row.ID
	return storedContextReference{ResourceType: ReferenceTypeComicSection, ResourceUUID: row.UUID, SnapshotJSON: encoded, ComicSectionID: &id, ImageFileID: row.CurrentImageFileID}, nil
}

func referenceNotFound(resourceUUID string) error {
	return domainError(CodeReferenceNotFound, "Reference 不可用", "资源不存在、已删除或当前不可引用："+resourceUUID, nil)
}

func encodeReferenceSnapshot(snapshot map[string]any, truncated []string) (string, error) {
	truncatedSet := map[string]bool{}
	for _, field := range truncated {
		truncatedSet[field] = true
	}
	for _, field := range []string{"summary", "description", "name", "original_filename", "title"} {
		value, ok := snapshot[field].(string)
		if !ok {
			continue
		}
		compact, changed := truncateUTF8Bytes(value, 3000)
		if changed {
			snapshot[field] = compact
			truncatedSet[field] = true
		}
	}
	for {
		truncated = truncated[:0]
		for _, field := range []string{"summary", "description", "tags", "name", "original_filename", "title"} {
			if truncatedSet[field] {
				truncated = append(truncated, field)
			}
		}
		snapshot["truncated_fields"] = truncated
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			return "", err
		}
		if len(encoded) <= MaxReferenceSnapshotBytes {
			return string(encoded), nil
		}
		shrunk := false
		for _, field := range []string{"summary", "description", "name", "original_filename", "title"} {
			value, ok := snapshot[field].(string)
			if !ok || value == "" {
				continue
			}
			limit := len(value) / 2
			if limit < 64 {
				limit = 64
			}
			compact, changed := truncateUTF8Bytes(value, limit)
			if changed {
				snapshot[field] = compact
				truncatedSet[field] = true
				shrunk = true
				break
			}
		}
		if !shrunk {
			return "", domainError(CodeReferenceSnapshot, "Reference 快照过大", "紧凑快照编码后不得超过 8 KiB。", nil)
		}
	}
}

func truncateUTF8Bytes(value string, limit int) (string, bool) {
	if limit < 1 || len(value) <= limit {
		return value, false
	}
	end := limit
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return strings.TrimSpace(value[:end]) + "…", true
}

func compactReferenceTags(tags []string) ([]string, bool) {
	result := make([]string, 0, len(tags))
	bytes := 0
	truncated := false
	for _, tag := range tags {
		compact, changed := truncateUTF8Bytes(tag, 128)
		if changed || len(result) >= 32 || bytes+len(compact) > 2048 {
			truncated = true
			if len(result) >= 32 || bytes+len(compact) > 2048 {
				break
			}
		}
		result = append(result, compact)
		bytes += len(compact)
	}
	return result, truncated
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func attachItemReferencesTx(ctx context.Context, tx *sql.Tx, itemID int64, references []storedContextReference, now time.Time) error {
	return attachReferencesTx(ctx, tx, &itemID, nil, references, now)
}

func attachFollowUpReferencesTx(ctx context.Context, tx *sql.Tx, followUpID int64, references []storedContextReference, now time.Time) error {
	return attachReferencesTx(ctx, tx, nil, &followUpID, references, now)
}

func attachReferencesTx(ctx context.Context, tx *sql.Tx, itemID, followUpID *int64, references []storedContextReference, now time.Time) error {
	for index, reference := range references {
		if _, err := tx.ExecContext(ctx, `INSERT INTO chat_context_references(chat_item_id,follow_up_id,position,resource_type,resource_uuid,snapshot_json,file_id,premise_asset_id,comic_section_id,image_file_id,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, itemID, followUpID, index+1, reference.ResourceType, reference.ResourceUUID, reference.SnapshotJSON, reference.FileID, reference.PremiseAssetID, reference.ComicSectionID, reference.ImageFileID, now); err != nil {
			return err
		}
	}
	return nil
}

func loadFollowUpReferencesTx(ctx context.Context, tx *sql.Tx, followUpID int64) ([]storedContextReference, error) {
	return loadStoredReferencesTx(ctx, tx, `follow_up_id=?`, followUpID)
}

func loadStoredReferencesTx(ctx context.Context, tx *sql.Tx, where string, ownerID int64) ([]storedContextReference, error) {
	rows, err := tx.QueryContext(ctx, `SELECT resource_type,resource_uuid,snapshot_json,file_id,premise_asset_id,comic_section_id,image_file_id FROM chat_context_references WHERE `+where+` ORDER BY position,id`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []storedContextReference{}
	for rows.Next() {
		var reference storedContextReference
		var fileID, premiseAssetID, comicSectionID, imageFileID sql.NullInt64
		if err := rows.Scan(&reference.ResourceType, &reference.ResourceUUID, &reference.SnapshotJSON, &fileID, &premiseAssetID, &comicSectionID, &imageFileID); err != nil {
			return nil, err
		}
		reference.FileID = nullableSQLInt64(fileID)
		reference.PremiseAssetID = nullableSQLInt64(premiseAssetID)
		reference.ComicSectionID = nullableSQLInt64(comicSectionID)
		reference.ImageFileID = nullableSQLInt64(imageFileID)
		result = append(result, reference)
	}
	return result, rows.Err()
}

func nullableSQLInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func (service *Service) itemReferences(ctx context.Context, store *project.Store, itemID int64) ([]Reference, error) {
	return service.queryReferences(ctx, store, "refs.chat_item_id=?", itemID)
}

func (service *Service) followUpReferences(ctx context.Context, store *project.Store, followUpID int64) ([]Reference, error) {
	return service.queryReferences(ctx, store, "refs.follow_up_id=?", followUpID)
}

func (service *Service) queryReferences(ctx context.Context, store *project.Store, where string, ownerID int64) ([]Reference, error) {
	var rows []referenceRow
	err := store.DB().WithContext(ctx).Table("chat_context_references AS refs").
		Select("refs.resource_type,refs.resource_uuid,refs.position,refs.snapshot_json,refs.image_file_id,(refs.image_file_id IS NOT NULL AND EXISTS (SELECT 1 FROM files f JOIN file_objects o ON o.id=f.file_object_id WHERE f.id=refs.image_file_id AND f.deleted_at IS NULL AND o.state='ready')) AS image_ready").
		Where(where, ownerID).Order("refs.position,refs.id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]Reference, 0, len(rows))
	for _, row := range rows {
		snapshot := json.RawMessage(row.SnapshotJSON)
		if !json.Valid(snapshot) {
			snapshot = json.RawMessage(`{"status":"unavailable"}`)
		}
		result = append(result, Reference{ResourceType: row.ResourceType, ResourceUUID: row.ResourceUUID, Position: row.Position, ImageAvailable: row.ImageReady, Snapshot: snapshot})
	}
	return result, nil
}
