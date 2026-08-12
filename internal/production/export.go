package production

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"lumi/internal/files"
	"lumi/internal/project"

	"gorm.io/gorm"
)

type exportRecord struct {
	ID                                                          int64 `gorm:"primaryKey"`
	UUID                                                        string
	ProjectID                                                   int64
	ChapterID                                                   *int64
	TaskUUID, Scope, Format, Status, SnapshotJSON, SnapshotHash string
	OutputFileID                                                *int64
	RelativePath, ErrorCode                                     string
	CreatedAt                                                   time.Time
	CompletedAt                                                 *time.Time
}

type ExportFilter struct {
	Scope        string
	ChapterUUID  string
	TaskUUID     string
	SnapshotHash string
	Status       string
}

func (exportRecord) TableName() string { return "comic_exports" }

func (service *Service) BuildExportSnapshot(ctx context.Context, scope, chapterUUID string) (ExportSnapshot, string, error) {
	return service.BuildExportSnapshotWithOptions(ctx, scope, chapterUUID, false)
}

func (service *Service) ExportReadiness(ctx context.Context, scope, chapterUUID string) (ExportReadiness, error) {
	db, err := service.store.DB().DB()
	if err != nil {
		return ExportReadiness{}, err
	}
	p, _, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		return ExportReadiness{}, err
	}
	readiness, _, err := queryExportReadiness(ctx, db, p.ID, scope, chapterUUID)
	return readiness, err
}

func (service *Service) BuildExportSnapshotWithOptions(ctx context.Context, scope, chapterUUID string, allowMissingImages bool) (ExportSnapshot, string, error) {
	db, err := service.store.DB().DB()
	if err != nil {
		return ExportSnapshot{}, "", err
	}
	p, _, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		return ExportSnapshot{}, "", err
	}
	pictureBook := service.store.PictureBookProfile()
	return buildExportSnapshot(ctx, db, service.store.ProjectUUID(), p.ID, scope, chapterUUID, allowMissingImages, pictureBook)
}

// BuildExportSnapshotTx repeats export readiness inside the production task
// transaction. The task row has already acquired SQLite's writer lock, so a
// matching hash proves that the frozen export and its audit counts describe
// the same database state that is committed with the task.
func (service *Service) BuildExportSnapshotTx(ctx context.Context, tx *sql.Tx, scope, chapterUUID string, allowMissingImages bool) (ExportSnapshot, string, error) {
	var projectID int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM projects WHERE uuid = ?", service.store.ProjectUUID()).Scan(&projectID); err != nil {
		return ExportSnapshot{}, "", err
	}
	pictureBook := service.store.PictureBookProfile()
	return buildExportSnapshot(ctx, tx, service.store.ProjectUUID(), projectID, scope, chapterUUID, allowMissingImages, pictureBook)
}

type exportQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func buildExportSnapshot(ctx context.Context, queryer exportQueryer, projectUUID string, projectID int64, scope, chapterUUID string, allowMissingImages bool, pictureBook project.PictureBookProfile) (ExportSnapshot, string, error) {
	readiness, entries, err := queryExportReadiness(ctx, queryer, projectID, scope, chapterUUID)
	if err != nil {
		return ExportSnapshot{}, "", err
	}
	if !readiness.CanExport {
		return ExportSnapshot{}, "", domainError(CodeExportEmpty, "没有可导出的图片", "至少一个 active Section 必须有可用的 current image。", nil)
	}
	if !readiness.Complete && !allowMissingImages {
		return ExportSnapshot{}, "", domainError(CodeExportIncomplete, "漫画仍有缺图 Section", "请先补齐图片，或明确允许仅导出已有图片。", nil)
	}
	missingUUIDs := make([]string, 0, len(readiness.MissingSections))
	for _, item := range readiness.MissingSections {
		missingUUIDs = append(missingUUIDs, item.UUID)
	}
	snapshot := ExportSnapshot{
		Version: 3, ProjectUUID: projectUUID, Scope: readiness.Scope, ChapterUUID: readiness.ChapterUUID,
		AllowMissingImages: allowMissingImages,
		ActiveChapterCount: readiness.ActiveChapterCount, SectionCount: readiness.ActiveSectionCount,
		ExportedSectionCount: readiness.ImageSectionCount, MissingSectionCount: readiness.MissingSectionCount,
		MissingSectionUUIDs: missingUUIDs, Entries: entries, PictureBook: &pictureBook,
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return ExportSnapshot{}, "", err
	}
	return snapshot, hashJSON(encoded), nil
}

func queryExportReadiness(ctx context.Context, queryer exportQueryer, projectID int64, scope, chapterUUID string) (ExportReadiness, []ExportEntry, error) {
	scope = strings.TrimSpace(scope)
	if scope != "chapter" && scope != "project" {
		return ExportReadiness{}, nil, domainError(CodeValidation, "导出 scope 无效", "只支持 chapter/project。", nil)
	}
	chapterUUID = strings.TrimSpace(chapterUUID)
	if scope == "chapter" {
		var exists int
		if err := queryer.QueryRowContext(ctx, "SELECT 1 FROM chapters WHERE project_id = ? AND uuid = ? AND deleted_at IS NULL", projectID, chapterUUID).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return ExportReadiness{}, nil, domainError(CodeNotFound, "章节不存在", "只能检查当前项目的 active Chapter。", err)
			}
			return ExportReadiness{}, nil, err
		}
	}
	query := `
SELECT c.uuid,c.chapter_code,c.title,s.uuid,s.section_no,s.title,
       CASE WHEN iv.id IS NOT NULL AND f.deleted_at IS NULL AND fo.state='ready' THEN f.uuid ELSE NULL END,
       CASE WHEN iv.id IS NOT NULL AND f.deleted_at IS NULL AND fo.state='ready' THEN fo.canonical_ext ELSE NULL END
FROM chapters c
LEFT JOIN chapter_comic_states cs ON cs.chapter_id=c.id
LEFT JOIN comic_sections s ON s.chapter_comic_state_id=cs.id AND s.deleted_at IS NULL
LEFT JOIN comic_image_variants iv ON iv.id=s.current_image_variant_id
LEFT JOIN files f ON f.id=iv.file_id
LEFT JOIN file_objects fo ON fo.id=f.file_object_id
WHERE c.project_id=? AND c.deleted_at IS NULL`
	args := []any{projectID}
	if scope == "chapter" {
		query += " AND c.uuid=?"
		args = append(args, chapterUUID)
	}
	query += " ORDER BY c.sort_order ASC,c.id ASC,s.section_no ASC,s.id ASC"
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return ExportReadiness{}, nil, err
	}
	defer rows.Close()
	readiness := ExportReadiness{Scope: scope, MissingSections: []ExportMissingSection{}}
	if scope == "chapter" {
		readiness.ChapterUUID = chapterUUID
	}
	entries := []ExportEntry{}
	seenChapters := make(map[string]struct{})
	for rows.Next() {
		var rowChapterUUID, chapterCode, chapterTitle string
		var sectionUUID, sectionTitle, imageUUID, extension sql.NullString
		var sectionNo sql.NullInt64
		if err := rows.Scan(&rowChapterUUID, &chapterCode, &chapterTitle, &sectionUUID, &sectionNo, &sectionTitle, &imageUUID, &extension); err != nil {
			return ExportReadiness{}, nil, err
		}
		if _, exists := seenChapters[rowChapterUUID]; !exists {
			seenChapters[rowChapterUUID] = struct{}{}
			readiness.ActiveChapterCount++
		}
		if !sectionUUID.Valid {
			continue
		}
		readiness.ActiveSectionCount++
		if imageUUID.Valid {
			readiness.ImageSectionCount++
			entries = append(entries, ExportEntry{ChapterCode: chapterCode, ChapterTitle: chapterTitle, SectionNo: int(sectionNo.Int64), SectionUUID: sectionUUID.String, ImageAssetUUID: imageUUID.String, Extension: extension.String})
			continue
		}
		readiness.MissingSections = append(readiness.MissingSections, ExportMissingSection{UUID: sectionUUID.String, SectionNo: int(sectionNo.Int64), Title: sectionTitle.String, ChapterUUID: rowChapterUUID})
	}
	if err := rows.Err(); err != nil {
		return ExportReadiness{}, nil, err
	}
	readiness.MissingSectionCount = len(readiness.MissingSections)
	readiness.CanExport = readiness.ImageSectionCount > 0
	readiness.Complete = readiness.CanExport && readiness.MissingSectionCount == 0
	return readiness, entries, nil
}

func (service *Service) CreateExportRecord(ctx context.Context, taskUUID, scope, chapterUUID string, snapshot ExportSnapshot, snapshotHash string) (Export, error) {
	p, _, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		return Export{}, err
	}
	uuid, err := newUUIDv7()
	if err != nil {
		return Export{}, err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return Export{}, err
	}
	var chapterID *int64
	if scope == "chapter" {
		chapter, err := service.chapterByUUID(ctx, service.store.DB(), chapterUUID)
		if err != nil {
			return Export{}, err
		}
		chapterID = &chapter.ID
	}
	now := service.now().UTC()
	record := exportRecord{UUID: uuid, ProjectID: p.ID, ChapterID: chapterID, TaskUUID: taskUUID, Scope: scope, Format: "zip", Status: "queued", SnapshotJSON: string(encoded), SnapshotHash: snapshotHash, CreatedAt: now}
	if err := service.store.DB().WithContext(ctx).Create(&record).Error; err != nil {
		return Export{}, err
	}
	return service.exportDTO(ctx, record)
}

func (service *Service) ListExports(ctx context.Context) ([]Export, error) {
	p, _, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		return nil, err
	}
	var rows []exportRecord
	if err := service.store.DB().WithContext(ctx).Where("project_id = ?", p.ID).Order("created_at DESC,id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Export, 0, len(rows))
	for _, row := range rows {
		dto, err := service.exportDTO(ctx, row)
		if err != nil {
			return nil, err
		}
		result = append(result, dto)
	}
	return result, nil
}

func (service *Service) ListExportsPage(ctx context.Context, filter ExportFilter, page, perPage int) ([]Export, Pagination, error) {
	p, _, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		return nil, Pagination{}, err
	}
	filter.Scope = strings.ToLower(strings.TrimSpace(filter.Scope))
	filter.ChapterUUID = strings.TrimSpace(filter.ChapterUUID)
	filter.TaskUUID = strings.TrimSpace(filter.TaskUUID)
	filter.SnapshotHash = strings.ToLower(strings.TrimSpace(filter.SnapshotHash))
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	if filter.Scope != "" && filter.Scope != "project" && filter.Scope != "chapter" {
		return nil, Pagination{}, domainError(CodeValidation, "导出 scope 无效", "scope 只支持 project 或 chapter。", nil)
	}
	if filter.ChapterUUID != "" && !isUUIDv7(filter.ChapterUUID) {
		return nil, Pagination{}, domainError(CodeValidation, "章节 UUID 无效", "chapter_uuid 必须是 UUIDv7。", nil)
	}
	if filter.Scope == "project" && filter.ChapterUUID != "" {
		return nil, Pagination{}, domainError(CodeValidation, "导出筛选冲突", "project scope 不能同时指定 chapter_uuid。", nil)
	}
	if filter.TaskUUID != "" && !isUUIDv7(filter.TaskUUID) {
		return nil, Pagination{}, domainError(CodeValidation, "任务 UUID 无效", "task_uuid 必须是 UUIDv7。", nil)
	}
	if filter.SnapshotHash != "" && !snapshotHashPattern.MatchString(filter.SnapshotHash) {
		return nil, Pagination{}, domainError(CodeValidation, "导出快照 hash 无效", "snapshot_hash 必须是 64 位小写十六进制字符串。", nil)
	}
	if filter.Status != "" && !validExportStatus(filter.Status) {
		return nil, Pagination{}, domainError(CodeValidation, "导出状态无效", "status 只支持 queued、running、ready、failed 或 cancelled。", nil)
	}
	page, perPage = normalizePage(page, perPage, 20)
	query := service.store.DB().WithContext(ctx).Model(&exportRecord{}).Where("project_id = ?", p.ID)
	if filter.Scope != "" {
		query = query.Where("scope = ?", filter.Scope)
	}
	if filter.ChapterUUID != "" {
		query = query.Where("chapter_id = (SELECT id FROM chapters WHERE project_id = ? AND uuid = ?)", p.ID, filter.ChapterUUID)
	}
	if filter.TaskUUID != "" {
		query = query.Where("task_uuid = ?", filter.TaskUUID)
	}
	if filter.SnapshotHash != "" {
		query = query.Where("snapshot_hash = ?", filter.SnapshotHash)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, Pagination{}, err
	}
	var rows []exportRecord
	if err := query.Order("created_at DESC,id DESC").Limit(perPage).Offset((page - 1) * perPage).Find(&rows).Error; err != nil {
		return nil, Pagination{}, err
	}
	items := make([]Export, 0, len(rows))
	for _, row := range rows {
		dto, err := service.exportDTO(ctx, row)
		if err != nil {
			return nil, Pagination{}, err
		}
		items = append(items, dto)
	}
	return items, pagePagination(page, perPage, total), nil
}

func (service *Service) ExportForTaskOrReadySnapshot(ctx context.Context, taskUUID, snapshotHash string) (Export, error) {
	var record exportRecord
	err := service.store.DB().WithContext(ctx).Where("task_uuid = ?", taskUUID).First(&record).Error
	if err == nil {
		return service.exportDTO(ctx, record)
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return Export{}, err
	}
	if snapshotHash == "" {
		return Export{}, notFound(err, "导出记录不存在")
	}
	p, _, projectErr := service.projectActor(ctx, service.store.DB())
	if projectErr != nil {
		return Export{}, projectErr
	}
	err = service.store.DB().WithContext(ctx).Where("project_id = ? AND snapshot_hash = ? AND status = 'ready'", p.ID, snapshotHash).Order("created_at DESC,id DESC").First(&record).Error
	if err != nil {
		return Export{}, notFound(err, "导出记录不存在")
	}
	return service.exportDTO(ctx, record)
}

func (service *Service) MarkExportRunning(ctx context.Context, taskUUID string) error {
	return service.store.DB().WithContext(ctx).Model(&exportRecord{}).Where("task_uuid = ? AND status = 'queued'", taskUUID).Update("status", "running").Error
}
func (service *Service) FailExport(ctx context.Context, taskUUID, code string, cancelled bool) error {
	status := "failed"
	if cancelled {
		status = "cancelled"
	}
	now := service.now().UTC()
	return service.store.DB().WithContext(ctx).Model(&exportRecord{}).Where("task_uuid = ? AND status NOT IN ('ready','cancelled')", taskUUID).Updates(map[string]any{"status": status, "error_code": code, "completed_at": now}).Error
}

func (service *Service) RenderAndCommitExport(ctx context.Context, taskUUID string, reportProgress func(int) error) (Export, error) {
	if err := ensureProductionTaskRunning(service.store.DB().WithContext(ctx), taskUUID); err != nil {
		return Export{}, err
	}
	var record exportRecord
	if err := service.store.DB().WithContext(ctx).Where("task_uuid = ?", taskUUID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return service.readyExportForDeletedTask(ctx, taskUUID, reportProgress)
		}
		return Export{}, notFound(err, "导出记录不存在")
	}
	if record.Status == "ready" {
		return service.exportDTO(ctx, record)
	}
	if record.Status == "queued" {
		result := service.store.DB().WithContext(ctx).Model(&exportRecord{}).Where("id = ? AND status = 'queued'", record.ID).Update("status", "running")
		if result.Error != nil {
			return Export{}, result.Error
		}
		if result.RowsAffected != 1 {
			return Export{}, domainError(CodeStateConflict, "导出状态已变化", "导出未进入 running。", nil)
		}
		record.Status = "running"
	}
	if record.Status != "running" {
		return Export{}, domainError(CodeStateConflict, "导出状态不可执行", "只有 queued/running 导出可以继续。", nil)
	}
	var ready exportRecord
	if err := service.store.DB().WithContext(ctx).Where("project_id = ? AND scope = ? AND ifnull(chapter_id,0)=ifnull(?,0) AND format = ? AND snapshot_hash = ? AND status = 'ready'", record.ProjectID, record.Scope, record.ChapterID, record.Format, record.SnapshotHash).First(&ready).Error; err == nil {
		// The first ready row is the canonical idempotent result for the
		// immutable snapshot. A later product task completes against it.
		if err := notifyExportProgress(reportProgress, 95); err != nil {
			return Export{}, err
		}
		if err := service.store.DB().WithContext(ctx).Delete(&record).Error; err != nil {
			return Export{}, err
		}
		return service.exportDTO(ctx, ready)
	}
	var snapshot ExportSnapshot
	if err := json.Unmarshal([]byte(record.SnapshotJSON), &snapshot); err != nil {
		return Export{}, domainError(CodeSnapshotInvalid, "导出快照损坏", "无法安全导出。", err)
	}
	archive, err := service.renderZip(ctx, snapshot, reportProgress)
	if err != nil {
		return Export{}, err
	}
	filename := safeExportNameForSnapshot(record.Scope, snapshot.ChapterUUID, record.SnapshotHash, snapshot) + ".zip"
	relative := filepath.ToSlash(filepath.Join("exports", filename))
	asset, err := service.files.CommitReader(ctx, files.CommitInput{Purpose: "export", OriginalFilename: filename, DisplayName: filename, SourceType: "exported", Metadata: map[string]any{"format": "zip", "snapshot_uuid": record.UUID}, Reader: bytes.NewReader(archive), Bind: func(tx *gorm.DB, fileID int64) error {
		if err := ensureProductionTaskRunning(tx, taskUUID); err != nil {
			return err
		}
		return tx.Model(&exportRecord{}).Where("id = ?", record.ID).Update("output_file_id", fileID).Error
	}})
	if err != nil {
		return Export{}, err
	}
	if err := notifyExportProgress(reportProgress, 90); err != nil {
		return Export{}, err
	}
	content, err := service.files.OpenContent(ctx, asset.UUID)
	if err != nil {
		return Export{}, err
	}
	defer content.File.Close()
	target, err := service.store.ResolvePath(relative)
	if err != nil {
		return Export{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return Export{}, err
	}
	temp := target + ".part"
	output, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return Export{}, err
	}
	_, copyErr := io.Copy(output, content.File)
	syncErr := output.Sync()
	closeErr := output.Close()
	if err := errorsJoin(copyErr, syncErr, closeErr); err != nil {
		_ = os.Remove(temp)
		return Export{}, err
	}
	if err := os.Rename(temp, target); err != nil {
		_ = os.Remove(temp)
		return Export{}, err
	}
	if err := notifyExportProgress(reportProgress, 95); err != nil {
		return Export{}, err
	}
	now := service.now().UTC()
	if err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureProductionTaskRunning(tx, taskUUID); err != nil {
			return err
		}
		result := tx.Model(&exportRecord{}).Where("id = ? AND status = 'running'", record.ID).Updates(map[string]any{"status": "ready", "relative_path": relative, "completed_at": now, "error_code": ""})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainError(CodeStateConflict, "导出状态已变化", "导出未被标记为 ready。", nil)
		}
		return nil
	}); err != nil {
		return Export{}, err
	}
	if err := service.store.DB().WithContext(ctx).First(&record, record.ID).Error; err != nil {
		return Export{}, err
	}
	return service.exportDTO(ctx, record)
}

// A canonical-reuse attempt deletes its transient Export row once the ready
// artifact has been resolved. If the process stops before the task reaches its
// terminal state, a durable retry can still recover the hash from the frozen
// task snapshot and finish against the same canonical artifact.
func (service *Service) readyExportForDeletedTask(ctx context.Context, taskUUID string, reportProgress func(int) error) (Export, error) {
	p, _, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		return Export{}, err
	}
	var inputSnapshot string
	err = service.store.DB().WithContext(ctx).Table("production_task_runs").Select("input_snapshot").Where("project_id = ? AND uuid = ?", p.ID, taskUUID).Row().Scan(&inputSnapshot)
	if err != nil {
		return Export{}, notFound(err, "导出记录不存在")
	}
	var frozen GenerationSnapshot
	if err := json.Unmarshal([]byte(inputSnapshot), &frozen); err != nil || frozen.Kind != "comic_export" || !snapshotHashPattern.MatchString(frozen.Prompt) {
		return Export{}, domainError(CodeSnapshotInvalid, "导出快照损坏", "无法解析 canonical 导出产物。", err)
	}
	var ready exportRecord
	if err := service.store.DB().WithContext(ctx).Where("project_id = ? AND snapshot_hash = ? AND status = 'ready'", p.ID, frozen.Prompt).Order("created_at DESC,id DESC").First(&ready).Error; err != nil {
		return Export{}, notFound(err, "导出记录不存在")
	}
	if err := notifyExportProgress(reportProgress, 95); err != nil {
		return Export{}, err
	}
	return service.exportDTO(ctx, ready)
}

func (service *Service) renderZip(ctx context.Context, snapshot ExportSnapshot, reportProgress func(int) error) ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	lastProgress := 10
	for index, entry := range snapshot.Entries {
		select {
		case <-ctx.Done():
			_ = archive.Close()
			return nil, ctx.Err()
		default:
		}
		content, err := service.files.OpenContent(ctx, entry.ImageAssetUUID)
		if err != nil {
			_ = archive.Close()
			return nil, err
		}
		unit := "section"
		if snapshot.PictureBook != nil && snapshot.PictureBook.Format != project.PictureBookVertical {
			unit = "page"
		}
		name := fmt.Sprintf("%s/%s/%s-%03d.%s", safeSegment(entry.ChapterCode), safeSegment(entry.ChapterTitle), unit, entry.SectionNo, safeExtension(entry.Extension))
		writer, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err == nil {
			_, err = io.Copy(writer, content.File)
		}
		content.File.Close()
		if err != nil {
			_ = archive.Close()
			return nil, err
		}
		progress := 10 + ((index + 1) * 70 / len(snapshot.Entries))
		if progress > lastProgress {
			if err := notifyExportProgress(reportProgress, progress); err != nil {
				_ = archive.Close()
				return nil, err
			}
			lastProgress = progress
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func notifyExportProgress(callback func(int) error, progress int) error {
	if callback == nil {
		return nil
	}
	return callback(progress)
}
func (service *Service) exportDTO(ctx context.Context, row exportRecord) (Export, error) {
	var chapterUUID string
	if row.ChapterID != nil {
		_ = service.store.DB().WithContext(ctx).Table("chapters").Where("id = ?", *row.ChapterID).Pluck("uuid", &chapterUUID).Error
	}
	var snapshot ExportSnapshot
	_ = json.Unmarshal([]byte(row.SnapshotJSON), &snapshot)
	result := Export{UUID: row.UUID, TaskUUID: row.TaskUUID, Scope: row.Scope, ChapterUUID: chapterUUID, Format: row.Format, Filename: safeExportNameForSnapshot(row.Scope, chapterUUID, row.SnapshotHash, snapshot) + ".zip", Status: row.Status, Snapshot: json.RawMessage(row.SnapshotJSON), SnapshotHash: row.SnapshotHash, RelativePath: row.RelativePath, ErrorCode: row.ErrorCode, CreatedAt: row.CreatedAt, CompletedAt: row.CompletedAt}
	if row.OutputFileID != nil {
		var uuid string
		_ = service.store.DB().WithContext(ctx).Table("files").Where("id = ?", *row.OutputFileID).Pluck("uuid", &uuid).Error
		if uuid != "" {
			asset, err := service.files.GetAsset(ctx, uuid, false)
			if err == nil {
				result.OutputAsset = &asset
			}
		}
	}
	return result, nil
}

var (
	unsafeName          = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	snapshotHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

func validExportStatus(status string) bool {
	switch status {
	case "queued", "running", "ready", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func safeSegment(value string) string {
	value = strings.Trim(unsafeName.ReplaceAllString(strings.TrimSpace(value), "-"), "-._")
	if value == "" {
		return "untitled"
	}
	if len(value) > 80 {
		value = value[:80]
	}
	return value
}
func safeExtension(value string) string {
	value = strings.ToLower(strings.TrimPrefix(value, "."))
	switch value {
	case "png", "jpg", "jpeg", "gif", "webp":
		return value
	}
	return "bin"
}
func safeExportName(scope, chapterUUID, hash string) string {
	parts := []string{"comic", scope}
	if chapterUUID != "" {
		parts = append(parts, chapterUUID)
	}
	if len(hash) > 12 {
		hash = hash[:12]
	}
	parts = append(parts, hash)
	return strings.Join(parts, "-")
}

func safeExportNameForSnapshot(scope, chapterUUID, hash string, snapshot ExportSnapshot) string {
	if snapshot.PictureBook == nil || snapshot.PictureBook.Format == project.PictureBookVertical {
		return safeExportName(scope, chapterUUID, hash)
	}
	parts := []string{"picture-book", scope}
	if chapterUUID != "" {
		parts = append(parts, chapterUUID)
	}
	if len(hash) > 12 {
		hash = hash[:12]
	}
	return strings.Join(append(parts, hash), "-")
}
func errorsJoin(values ...error) error {
	var texts []string
	for _, err := range values {
		if err != nil {
			texts = append(texts, err.Error())
		}
	}
	if len(texts) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(texts, "; "))
}
