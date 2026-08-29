package production

import (
	"archive/zip"
	"context"
	"crypto/sha256"
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
	"unicode"

	"lumi/internal/durablefs"
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
	RetentionDays                                               int
	ExpiresAt                                                   *time.Time
	ByteSize                                                    int64
	ContentSHA256                                               string
	CreatedAt                                                   time.Time
	CompletedAt                                                 *time.Time
}

const (
	ExportFormatZIP     = "zip"
	ExportFormatPDF     = "pdf"
	ExportRetentionDays = 7
	exportSnapshotV6    = 6
	exportCleanupLimit  = 1000
	exportRetention     = time.Duration(ExportRetentionDays) * 24 * time.Hour
)

const (
	ExportPDFPageSizeA4Portrait = "a4_portrait"
	ExportPDFTwoUpStacked       = "two_up_stacked"
	ExportPDFTwoUpColumns       = "two_up_columns"
	ExportPDFOneUp              = "one_up"
)

func NormalizeExportFormat(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ExportFormatZIP, nil
	}
	if value != ExportFormatZIP && value != ExportFormatPDF {
		return "", domainError(CodeValidation, "导出格式无效", "format 只支持 zip 或 pdf。", nil)
	}
	return value, nil
}

func ExportExpiresAt(completedAt time.Time) time.Time {
	return completedAt.UTC().Add(exportRetention)
}

type ExportFilter struct {
	Scope        string
	ChapterUUID  string
	TaskUUID     string
	SnapshotHash string
	Format       string
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
	return service.BuildExportSnapshotForFormat(ctx, scope, chapterUUID, allowMissingImages, ExportFormatZIP)
}

func (service *Service) BuildExportSnapshotForFormat(ctx context.Context, scope, chapterUUID string, allowMissingImages bool, format string) (ExportSnapshot, string, error) {
	format, err := NormalizeExportFormat(format)
	if err != nil {
		return ExportSnapshot{}, "", err
	}
	db, err := service.store.DB().DB()
	if err != nil {
		return ExportSnapshot{}, "", err
	}
	p, _, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		return ExportSnapshot{}, "", err
	}
	pictureBook, err := service.store.RequirePictureBookProfile()
	if err != nil {
		return ExportSnapshot{}, "", err
	}
	return buildExportSnapshot(ctx, db, service.store.ProjectUUID(), p.Name, p.ID, scope, chapterUUID, allowMissingImages, format, pictureBook)
}

// BuildExportSnapshotTx repeats export readiness inside the production task
// transaction. The task row has already acquired SQLite's writer lock, so a
// matching hash proves that the frozen export and its audit counts describe
// the same database state that is committed with the task.
func (service *Service) BuildExportSnapshotTx(ctx context.Context, tx *sql.Tx, scope, chapterUUID string, allowMissingImages bool) (ExportSnapshot, string, error) {
	return service.BuildExportSnapshotTxForFormat(ctx, tx, scope, chapterUUID, allowMissingImages, ExportFormatZIP)
}

func (service *Service) BuildExportSnapshotTxForFormat(ctx context.Context, tx *sql.Tx, scope, chapterUUID string, allowMissingImages bool, format string) (ExportSnapshot, string, error) {
	format, err := NormalizeExportFormat(format)
	if err != nil {
		return ExportSnapshot{}, "", err
	}
	var projectID int64
	var projectTitle string
	if err := tx.QueryRowContext(ctx, "SELECT id,name FROM projects WHERE uuid = ?", service.store.ProjectUUID()).Scan(&projectID, &projectTitle); err != nil {
		return ExportSnapshot{}, "", err
	}
	pictureBook, err := service.store.RequirePictureBookProfile()
	if err != nil {
		return ExportSnapshot{}, "", err
	}
	return buildExportSnapshot(ctx, tx, service.store.ProjectUUID(), projectTitle, projectID, scope, chapterUUID, allowMissingImages, format, pictureBook)
}

type exportQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func buildExportSnapshot(ctx context.Context, queryer exportQueryer, projectUUID, projectTitle string, projectID int64, scope, chapterUUID string, allowMissingImages bool, format string, pictureBook project.PictureBookProfile) (ExportSnapshot, string, error) {
	readiness, entries, err := queryExportReadiness(ctx, queryer, projectID, scope, chapterUUID)
	if err != nil {
		return ExportSnapshot{}, "", err
	}
	if !readiness.CanExport {
		return ExportSnapshot{}, "", domainError(CodeExportEmpty, "没有可导出的正文图片", "至少一个 active 正文页必须有可用的 current image。", nil)
	}
	if !readiness.Complete && !allowMissingImages {
		return ExportSnapshot{}, "", domainError(CodeExportIncomplete, "漫画仍有缺图 Section", "请先补齐图片，或明确允许仅导出已有图片。", nil)
	}
	missingUUIDs := make([]string, 0, len(readiness.MissingSections))
	for _, item := range readiness.MissingSections {
		missingUUIDs = append(missingUUIDs, item.UUID)
	}
	snapshot := ExportSnapshot{
		Version: exportSnapshotV6, ProjectUUID: projectUUID, Scope: readiness.Scope, ChapterUUID: readiness.ChapterUUID,
		AllowMissingImages: allowMissingImages,
		ActiveChapterCount: readiness.ActiveChapterCount, SectionCount: readiness.ActiveSectionCount,
		ExportedSectionCount: readiness.ImageSectionCount, MissingSectionCount: readiness.MissingSectionCount,
		MissingSectionUUIDs: missingUUIDs, MissingSections: readiness.MissingSections,
		Entries: entries, PictureBook: &pictureBook,
	}
	if format == ExportFormatPDF {
		for _, entry := range entries {
			if !supportedExportPDFMIME(entry.MIMEType) || entry.Width <= 0 || entry.Height <= 0 {
				return ExportSnapshot{}, "", domainError(CodeExportUnavailable, "PDF 图片元数据不完整", "所有导出图片都必须具有受支持的 MIME 和有效尺寸。", nil)
			}
		}
		snapshot.Format = ExportFormatPDF
		snapshot.ProjectTitle = strings.TrimSpace(projectTitle)
		layout := pdfLayoutForPictureBook(pictureBook)
		snapshot.PDFLayout = &layout
	} else {
		// PDF-only metadata is removed before marshaling ZIP snapshots. PageRole
		// remains frozen because it is part of the v6 export ordering contract.
		for index := range snapshot.Entries {
			snapshot.Entries[index].ChapterUUID = ""
			snapshot.Entries[index].MIMEType = ""
			snapshot.Entries[index].Width = 0
			snapshot.Entries[index].Height = 0
		}
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
SELECT c.uuid,c.chapter_code,c.title,s.uuid,s.section_no,s.title,s.page_role,
       CASE WHEN s.page_role='body' THEN
         SUM(CASE WHEN s.page_role='body' THEN 1 ELSE 0 END) OVER (
           PARTITION BY c.id ORDER BY s.section_no,s.id ROWS UNBOUNDED PRECEDING
         )
       ELSE NULL END,
       CASE WHEN iv.id IS NOT NULL AND f.deleted_at IS NULL AND fo.state='ready' THEN f.uuid ELSE NULL END,
       CASE WHEN iv.id IS NOT NULL AND f.deleted_at IS NULL AND fo.state='ready' THEN fo.canonical_ext ELSE NULL END,
       CASE WHEN iv.id IS NOT NULL AND f.deleted_at IS NULL AND fo.state='ready' THEN fo.mime_type ELSE NULL END,
       CASE WHEN iv.id IS NOT NULL AND f.deleted_at IS NULL AND fo.state='ready' THEN fo.width ELSE NULL END,
       CASE WHEN iv.id IS NOT NULL AND f.deleted_at IS NULL AND fo.state='ready' THEN fo.height ELSE NULL END
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
		var sectionUUID, sectionTitle, pageRole, imageUUID, extension, mimeType sql.NullString
		var sectionNo, bodyPageNo, width, height sql.NullInt64
		if err := rows.Scan(&rowChapterUUID, &chapterCode, &chapterTitle, &sectionUUID, &sectionNo, &sectionTitle, &pageRole, &bodyPageNo, &imageUUID, &extension, &mimeType, &width, &height); err != nil {
			return ExportReadiness{}, nil, err
		}
		if _, exists := seenChapters[rowChapterUUID]; !exists {
			seenChapters[rowChapterUUID] = struct{}{}
			readiness.ActiveChapterCount++
		}
		if !sectionUUID.Valid {
			continue
		}
		role := strings.TrimSpace(pageRole.String)
		if role == "" {
			role = PageRoleBody
		}
		readiness.ActiveSectionCount++
		if imageUUID.Valid {
			readiness.ImageSectionCount++
			entries = append(entries, ExportEntry{ChapterUUID: rowChapterUUID, ChapterCode: chapterCode, ChapterTitle: chapterTitle, SectionNo: int(sectionNo.Int64), SectionUUID: sectionUUID.String, PageRole: role, BodyPageNo: int(bodyPageNo.Int64), ImageAssetUUID: imageUUID.String, Extension: extension.String, MIMEType: mimeType.String, Width: int(width.Int64), Height: int(height.Int64)})
			if role == PageRoleBody {
				readiness.CanExport = true
			}
			continue
		}
		readiness.MissingSections = append(readiness.MissingSections, ExportMissingSection{UUID: sectionUUID.String, SectionNo: int(sectionNo.Int64), Title: sectionTitle.String, ChapterUUID: rowChapterUUID, PageRole: role, BodyPageNo: int(bodyPageNo.Int64)})
	}
	if err := rows.Err(); err != nil {
		return ExportReadiness{}, nil, err
	}
	readiness.MissingSectionCount = len(readiness.MissingSections)
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
	relative := ExportRelativePath(uuid, scope, chapterUUID, snapshotHash, snapshot)
	format := exportFormatForSnapshot(snapshot)
	record := exportRecord{UUID: uuid, ProjectID: p.ID, ChapterID: chapterID, TaskUUID: taskUUID, Scope: scope, Format: format, Status: "queued", SnapshotJSON: string(encoded), SnapshotHash: snapshotHash, RelativePath: relative, RetentionDays: ExportRetentionDays, CreatedAt: now}
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
	now := service.now().UTC()
	if err := service.store.DB().WithContext(ctx).Where("project_id = ? AND status <> 'expired' AND (expires_at IS NULL OR expires_at > ?)", p.ID, now).Order("created_at DESC,id DESC").Find(&rows).Error; err != nil {
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
	filter.Format = strings.ToLower(strings.TrimSpace(filter.Format))
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
	if filter.Format != "" && filter.Format != ExportFormatZIP && filter.Format != ExportFormatPDF {
		return nil, Pagination{}, domainError(CodeValidation, "导出格式无效", "format 只支持 zip 或 pdf。", nil)
	}
	if filter.Status != "" && !validExportStatus(filter.Status) {
		return nil, Pagination{}, domainError(CodeValidation, "导出状态无效", "status 只支持 queued、running、ready、failed 或 cancelled。", nil)
	}
	page, perPage = normalizePage(page, perPage, 20)
	now := service.now().UTC()
	query := service.store.DB().WithContext(ctx).Model(&exportRecord{}).Where("project_id = ? AND status <> 'expired' AND (expires_at IS NULL OR expires_at > ?)", p.ID, now)
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
	if filter.Format != "" {
		query = query.Where("format = ?", filter.Format)
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

func (service *Service) ExportForTaskOrReadySnapshot(ctx context.Context, taskUUID, snapshotHash, requestedFormat string) (Export, error) {
	var record exportRecord
	now := service.now().UTC()
	format, err := NormalizeExportFormat(requestedFormat)
	if err != nil {
		return Export{}, err
	}
	err = service.store.DB().WithContext(ctx).Where("task_uuid = ? AND format = ? AND status <> 'expired' AND (expires_at IS NULL OR expires_at > ?)", taskUUID, format, now).First(&record).Error
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
	err = service.store.DB().WithContext(ctx).Where("project_id = ? AND format = ? AND snapshot_hash = ? AND status = 'ready' AND expires_at > ?", p.ID, format, snapshotHash, now).Order("created_at DESC,id DESC").First(&record).Error
	if err != nil {
		return Export{}, notFound(err, "导出记录不存在")
	}
	return service.exportDTO(ctx, record)
}

func (service *Service) MarkExportRunning(ctx context.Context, taskUUID string) error {
	return service.store.DB().WithContext(ctx).Model(&exportRecord{}).Where("task_uuid = ? AND status = 'queued'", taskUUID).Updates(map[string]any{"status": "running", "expires_at": nil}).Error
}
func (service *Service) FailExport(ctx context.Context, taskUUID, code string, cancelled bool) error {
	status := "failed"
	if cancelled {
		status = "cancelled"
	}
	now := service.now().UTC()
	return service.store.DB().WithContext(ctx).Model(&exportRecord{}).Where("task_uuid = ? AND status IN ('queued','running')", taskUUID).Updates(map[string]any{"status": status, "error_code": code, "completed_at": now, "expires_at": now.Add(exportRetention)}).Error
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
		if record.ExpiresAt == nil {
			return Export{}, domainError(CodeExportUnavailable, "导出缺少到期时间", "ready 导出必须登记 7 天保留边界。", nil)
		}
		if !record.ExpiresAt.After(service.now().UTC()) {
			return Export{}, domainError(CodeExportExpired, "导出已过期", "已到期的 ready 导出不能在恢复任务时重新使用。", nil)
		}
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
	now := service.now().UTC()
	var ready exportRecord
	canonicalErr := service.store.DB().WithContext(ctx).Where("project_id = ? AND scope = ? AND ifnull(chapter_id,0)=ifnull(?,0) AND format = ? AND snapshot_hash = ? AND status = 'ready' AND expires_at > ?", record.ProjectID, record.Scope, record.ChapterID, record.Format, record.SnapshotHash, now).First(&ready).Error
	if canonicalErr == nil {
		// The first ready row is the canonical idempotent result for the
		// immutable snapshot. Reuse never extends its expiration time.
		if err := notifyExportProgress(reportProgress, 95); err != nil {
			return Export{}, err
		}
		if err := service.store.DB().WithContext(ctx).Delete(&record).Error; err != nil {
			return Export{}, err
		}
		return service.exportDTO(ctx, ready)
	}
	if !errors.Is(canonicalErr, gorm.ErrRecordNotFound) {
		return Export{}, canonicalErr
	}
	var snapshot ExportSnapshot
	if err := json.Unmarshal([]byte(record.SnapshotJSON), &snapshot); err != nil {
		return Export{}, domainError(CodeSnapshotInvalid, "导出快照损坏", "无法安全导出。", err)
	}
	relative := record.RelativePath
	if relative == "" {
		relative = ExportRelativePath(record.UUID, record.Scope, snapshot.ChapterUUID, record.SnapshotHash, snapshot)
		if err := service.store.DB().WithContext(ctx).Model(&exportRecord{}).Where("id = ? AND status = 'running'", record.ID).Update("relative_path", relative).Error; err != nil {
			return Export{}, err
		}
	}
	byteSize, contentSHA256, target, err := service.renderExportToPath(ctx, relative, record.Format, snapshot, reportProgress)
	if err != nil {
		return Export{}, err
	}
	now = service.now().UTC()
	expiresAt := now.Add(exportRetention)
	canonicalReused := false
	if err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureProductionTaskRunning(tx, taskUUID); err != nil {
			return err
		}
		if err := tx.Model(&exportRecord{}).
			Where("project_id = ? AND scope = ? AND ifnull(chapter_id,0)=ifnull(?,0) AND format = ? AND snapshot_hash = ? AND status = 'ready' AND expires_at <= ?", record.ProjectID, record.Scope, record.ChapterID, record.Format, record.SnapshotHash, now).
			Update("status", "expired").Error; err != nil {
			return err
		}
		ready = exportRecord{}
		canonicalErr := tx.Where("project_id = ? AND scope = ? AND ifnull(chapter_id,0)=ifnull(?,0) AND format = ? AND snapshot_hash = ? AND status = 'ready' AND expires_at > ?", record.ProjectID, record.Scope, record.ChapterID, record.Format, record.SnapshotHash, now).First(&ready).Error
		if canonicalErr == nil {
			result := tx.Where("id = ? AND status = 'running'", record.ID).Delete(&exportRecord{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return domainError(CodeStateConflict, "导出状态已变化", "canonical 复用未能移除临时导出记录。", nil)
			}
			canonicalReused = true
			return nil
		}
		if !errors.Is(canonicalErr, gorm.ErrRecordNotFound) {
			return canonicalErr
		}
		result := tx.Model(&exportRecord{}).Where("id = ? AND status = 'running'", record.ID).Updates(map[string]any{
			"status": "ready", "output_file_id": nil, "relative_path": relative,
			"retention_days": ExportRetentionDays, "expires_at": expiresAt,
			"byte_size": byteSize, "content_sha256": contentSHA256,
			"completed_at": now, "error_code": "",
		})
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
	if canonicalReused {
		// This task lost a race to another unexpired canonical artifact. Its
		// UUID-scoped path cannot be shared, so deleting it cannot affect ready.
		_ = os.Remove(target)
		return service.exportDTO(ctx, ready)
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
	format := ExportFormatZIP
	var exportSnapshot ExportSnapshot
	if json.Unmarshal(frozen.Parameters, &exportSnapshot) == nil {
		format = exportFormatForSnapshot(exportSnapshot)
	}
	var ready exportRecord
	if err := service.store.DB().WithContext(ctx).Where("project_id = ? AND format = ? AND snapshot_hash = ? AND status = 'ready' AND expires_at > ?", p.ID, format, frozen.Prompt, service.now().UTC()).Order("created_at DESC,id DESC").First(&ready).Error; err != nil {
		return Export{}, notFound(err, "导出记录不存在")
	}
	if err := notifyExportProgress(reportProgress, 95); err != nil {
		return Export{}, err
	}
	return service.exportDTO(ctx, ready)
}

func (service *Service) writeZip(ctx context.Context, output io.Writer, snapshot ExportSnapshot, reportProgress func(int) error) error {
	archive := zip.NewWriter(output)
	lastProgress := 10
	for index, entry := range snapshot.Entries {
		select {
		case <-ctx.Done():
			_ = archive.Close()
			return ctx.Err()
		default:
		}
		content, err := service.files.OpenContent(ctx, entry.ImageAssetUUID)
		if err != nil {
			_ = archive.Close()
			return err
		}
		name, err := exportZIPEntryName(snapshot.Version, entry)
		if err != nil {
			content.File.Close()
			_ = archive.Close()
			return err
		}
		writer, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err == nil {
			_, err = io.Copy(writer, content.File)
		}
		content.File.Close()
		if err != nil {
			_ = archive.Close()
			return err
		}
		progress := 10 + ((index + 1) * 70 / len(snapshot.Entries))
		if progress > lastProgress {
			if err := notifyExportProgress(reportProgress, progress); err != nil {
				_ = archive.Close()
				return err
			}
			lastProgress = progress
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	return nil
}

func exportZIPEntryName(snapshotVersion int, entry ExportEntry) (string, error) {
	chapter := safeSegment(entry.ChapterCode)
	extension := safeExtension(entry.Extension)
	if snapshotVersion < exportSnapshotV6 {
		return fmt.Sprintf("%s/sections/section-%03d.%s", chapter, entry.SectionNo, extension), nil
	}
	switch entry.PageRole {
	case PageRoleFrontCover:
		return fmt.Sprintf("%s/front-cover.%s", chapter, extension), nil
	case PageRoleBody:
		if entry.BodyPageNo <= 0 {
			return "", domainError(CodeSnapshotInvalid, "正文页码无效", "v6 ZIP 的 body 条目必须冻结正数 body_page_no。", nil)
		}
		return fmt.Sprintf("%s/pages/page-%03d.%s", chapter, entry.BodyPageNo, extension), nil
	case PageRoleBackCover:
		return fmt.Sprintf("%s/back-cover.%s", chapter, extension), nil
	default:
		return "", domainError(CodeSnapshotInvalid, "页面角色无效", "v6 ZIP 的每个图片条目都必须冻结有效的 page_role。", nil)
	}
}

type exportCountingWriter struct {
	writer  io.Writer
	written int64
}

func (writer *exportCountingWriter) Write(value []byte) (int, error) {
	written, err := writer.writer.Write(value)
	writer.written += int64(written)
	return written, err
}

func (service *Service) renderZipToExportPath(ctx context.Context, relative string, snapshot ExportSnapshot, reportProgress func(int) error) (int64, string, string, error) {
	return service.renderExportToPath(ctx, relative, ExportFormatZIP, snapshot, reportProgress)
}

func (service *Service) renderExportToPath(ctx context.Context, relative, format string, snapshot ExportSnapshot, reportProgress func(int) error) (int64, string, string, error) {
	format, err := NormalizeExportFormat(format)
	if err != nil {
		return 0, "", "", err
	}
	if exportFormatForSnapshot(snapshot) != format {
		return 0, "", "", domainError(CodeSnapshotInvalid, "导出格式不一致", "数据库登记格式与冻结快照格式不一致。", nil)
	}
	target, err := service.store.ResolvePath(relative)
	if err != nil {
		return 0, "", "", err
	}
	var byteSize int64
	var contentSHA256 string
	err = service.store.WithFileCommit(func() error {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		part := target + ".part"
		confirmedTarget, err := service.store.ResolvePath(relative)
		if err != nil || confirmedTarget != target {
			return domainError(CodeExportUnavailable, "导出路径不安全", "导出目标在写入前未通过项目根路径复检。", err)
		}
		confirmedPart, err := service.store.ResolvePath(relative + ".part")
		if err != nil || confirmedPart != part {
			return domainError(CodeExportUnavailable, "导出临时路径不安全", "导出 .part 在写入前未通过项目根路径复检。", err)
		}
		output, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		hash := sha256.New()
		counter := &exportCountingWriter{writer: io.MultiWriter(output, hash)}
		var writeErr error
		if format == ExportFormatPDF {
			writeErr = service.writePDF(ctx, counter, snapshot, reportProgress)
		} else {
			writeErr = service.writeZip(ctx, counter, snapshot, reportProgress)
		}
		syncErr := output.Sync()
		closeErr := output.Close()
		if operationErr := errors.Join(writeErr, syncErr, closeErr); operationErr != nil {
			_ = os.Remove(part)
			return operationErr
		}
		if err := notifyExportProgress(reportProgress, 90); err != nil {
			_ = os.Remove(part)
			return err
		}
		if info, statErr := os.Lstat(target); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				_ = os.Remove(part)
				return domainError(CodeExportUnavailable, "导出目标路径不安全", "已存在的导出目标必须是项目内普通文件。", nil)
			}
			// A process may have stopped after rename but before the ready DB
			// commit. Removing that unpublished, Export-UUID-scoped target makes
			// the retry portable to platforms where Rename cannot replace files.
			if err := os.Remove(target); err != nil {
				_ = os.Remove(part)
				return err
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			_ = os.Remove(part)
			return statErr
		}
		confirmedTarget, err = service.store.ResolvePath(relative)
		if err != nil || confirmedTarget != target {
			_ = os.Remove(part)
			return domainError(CodeExportUnavailable, "导出路径不安全", "导出目标在发布前未通过项目根路径复检。", err)
		}
		confirmedPart, err = service.store.ResolvePath(relative + ".part")
		if err != nil || confirmedPart != part {
			_ = os.Remove(part)
			return domainError(CodeExportUnavailable, "导出临时路径不安全", "导出 .part 在发布前未通过项目根路径复检。", err)
		}
		if err := os.Rename(part, target); err != nil {
			_ = os.Remove(part)
			return err
		}
		if err := durablefs.SyncDirectory(filepath.Dir(target)); err != nil {
			return err
		}
		byteSize = counter.written
		contentSHA256 = fmt.Sprintf("%x", hash.Sum(nil))
		return notifyExportProgress(reportProgress, 95)
	})
	return byteSize, contentSHA256, target, err
}

func notifyExportProgress(callback func(int) error, progress int) error {
	if callback == nil {
		return nil
	}
	return callback(progress)
}
func (service *Service) exportDTO(ctx context.Context, row exportRecord) (Export, error) {
	var chapterUUID, chapterCode string
	if row.ChapterID != nil {
		var chapter struct {
			UUID        string
			ChapterCode string
		}
		_ = service.store.DB().WithContext(ctx).Table("chapters").Select("uuid,chapter_code").Where("id = ?", *row.ChapterID).Scan(&chapter).Error
		chapterUUID, chapterCode = chapter.UUID, chapter.ChapterCode
	}
	var snapshot ExportSnapshot
	_ = json.Unmarshal([]byte(row.SnapshotJSON), &snapshot)
	extension := exportExtension(row.Format)
	filename := safeExportNameForSnapshot(row.Scope, chapterUUID, row.SnapshotHash, snapshot) + "." + extension
	if base := filepath.Base(filepath.FromSlash(row.RelativePath)); base != "." && base != "" && strings.HasSuffix(strings.ToLower(base), "."+extension) {
		filename = base
	}
	if row.Format == ExportFormatPDF {
		filename = exportPDFDownloadFilename(snapshot, service.store.ProjectName(), chapterCode)
	}
	result := Export{
		UUID: row.UUID, TaskUUID: row.TaskUUID, Scope: row.Scope, ChapterUUID: chapterUUID,
		Format: row.Format, Filename: filename, Status: row.Status,
		Snapshot: json.RawMessage(row.SnapshotJSON), SnapshotHash: row.SnapshotHash,
		ExpiresAt: row.ExpiresAt, RetentionDays: row.RetentionDays, ByteSize: row.ByteSize,
		ContentSHA256: row.ContentSHA256, RelativePath: row.RelativePath,
		ErrorCode: row.ErrorCode, CreatedAt: row.CreatedAt, CompletedAt: row.CompletedAt,
	}
	if result.RetentionDays == 0 {
		result.RetentionDays = ExportRetentionDays
	}
	if row.Status == "ready" && row.ExpiresAt != nil && row.ExpiresAt.After(service.now().UTC()) {
		result.DownloadURL = "/media/projects/" + service.store.ProjectUUID() + "/comic-exports/" + row.UUID + "/content"
	}
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

func exportFormatForSnapshot(snapshot ExportSnapshot) string {
	if strings.EqualFold(strings.TrimSpace(snapshot.Format), ExportFormatPDF) {
		return ExportFormatPDF
	}
	return ExportFormatZIP
}

func exportExtension(format string) string {
	if strings.EqualFold(strings.TrimSpace(format), ExportFormatPDF) {
		return ExportFormatPDF
	}
	return ExportFormatZIP
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

const (
	exportPDFDownloadStemMaxBytes = 180
	exportPDFChapterCodeMaxBytes  = 48
)

func exportPDFDownloadFilename(snapshot ExportSnapshot, currentProjectTitle, currentChapterCode string) string {
	projectTitle := strings.TrimSpace(snapshot.ProjectTitle)
	if projectTitle == "" {
		projectTitle = currentProjectTitle
	}
	projectTitle = safeExportPDFTitle(projectTitle)
	chapterCode := ""
	if snapshot.Scope == "chapter" {
		if len(snapshot.Entries) > 0 {
			chapterCode = snapshot.Entries[0].ChapterCode
		}
		if strings.TrimSpace(chapterCode) == "" {
			chapterCode = currentChapterCode
		}
		chapterCode = safeExportPDFChapterCode(chapterCode)
		chapterCode = truncateUTF8Bytes(chapterCode, exportPDFChapterCodeMaxBytes)
	}
	suffix := ""
	if chapterCode != "" {
		suffix = "-" + chapterCode
	}
	projectTitle = truncateUTF8Bytes(projectTitle, max(1, exportPDFDownloadStemMaxBytes-len(suffix)))
	projectTitle = strings.TrimRight(strings.TrimSpace(projectTitle), ". ")
	if projectTitle == "" {
		projectTitle = "untitled"
	}
	return projectTitle + suffix + ".pdf"
}

func safeExportPDFTitle(value string) string {
	var result strings.Builder
	lastSeparator := false
	lastSpace := false
	for _, char := range strings.TrimSpace(value) {
		if unicode.IsControl(char) || strings.ContainsRune(`/\\:*?"<>|`, char) {
			if result.Len() > 0 && !lastSeparator {
				result.WriteByte('-')
			}
			lastSeparator, lastSpace = true, false
			continue
		}
		if unicode.IsSpace(char) {
			if result.Len() > 0 && !lastSpace && !lastSeparator {
				result.WriteByte(' ')
			}
			lastSpace = true
			continue
		}
		result.WriteRune(char)
		lastSeparator, lastSpace = false, false
	}
	cleaned := strings.Trim(strings.TrimSpace(result.String()), ".-")
	if cleaned == "" {
		return "untitled"
	}
	return cleaned
}

func safeExportPDFChapterCode(value string) string {
	var result strings.Builder
	lastSeparator := false
	for _, char := range strings.TrimSpace(value) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			result.WriteRune(char)
			lastSeparator = false
			continue
		}
		if result.Len() > 0 && !lastSeparator {
			result.WriteByte('-')
			lastSeparator = true
		}
	}
	return strings.Trim(result.String(), "-")
}

func truncateUTF8Bytes(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	used := 0
	for index, char := range value {
		charBytes := len(string(char))
		if used+charBytes > limit {
			return strings.TrimSpace(value[:index])
		}
		used += charBytes
	}
	return value
}

// ExportRelativePath is the only storage naming rule used for new exports. The
// Export UUID makes paths generation-specific, so a cleanup job can never
// race a later export of the same immutable snapshot.
func ExportRelativePath(exportUUID, scope, chapterUUID, hash string, snapshot ExportSnapshot) string {
	filename := safeExportNameForSnapshot(scope, chapterUUID, hash, snapshot) + "-" + exportUUID + "." + exportExtension(exportFormatForSnapshot(snapshot))
	return filepath.ToSlash(filepath.Join("exports", filename))
}
