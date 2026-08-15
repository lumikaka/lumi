package production

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

type ExportContent struct {
	File         *os.File
	Filename     string
	ContentType  string
	ETag         string
	LastModified time.Time
	ExpiresAt    time.Time
	ByteSize     int64
}

func (service *Service) OpenExportContent(ctx context.Context, exportUUID string) (ExportContent, error) {
	if !isUUIDv7(exportUUID) {
		return ExportContent{}, domainError(CodeNotFound, "导出不存在", "export_uuid 必须是 UUIDv7。", nil)
	}
	p, _, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		return ExportContent{}, err
	}
	var record exportRecord
	err = service.store.DB().WithContext(ctx).Where("project_id = ? AND uuid = ?", p.ID, exportUUID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ExportContent{}, domainError(CodeNotFound, "导出不存在", "导出记录已清理或不属于当前项目。", err)
	}
	if err != nil {
		return ExportContent{}, err
	}
	now := service.now().UTC()
	if record.Status == "expired" || (record.ExpiresAt != nil && !record.ExpiresAt.After(now)) {
		return ExportContent{}, domainError(CodeExportExpired, "导出已过期", "导出文件已超过 7 天保留期，请重新导出。", nil)
	}
	if record.Status != "ready" || record.ExpiresAt == nil {
		return ExportContent{}, domainError(CodeExportUnavailable, "导出尚不可下载", "只有未过期的 ready 导出可以下载。", nil)
	}

	filename := filepath.Base(filepath.FromSlash(record.RelativePath))
	var file *os.File
	if target, pathErr := service.registeredExportPath(record); pathErr == nil {
		file, err = os.Open(target)
	} else {
		err = pathErr
	}
	if err != nil && record.OutputFileID != nil {
		var assetUUID string
		lookupErr := service.store.DB().WithContext(ctx).Table("files").Where("id = ? AND deleted_at IS NULL", *record.OutputFileID).Pluck("uuid", &assetUUID).Error
		if lookupErr == nil && assetUUID != "" {
			legacy, legacyErr := service.files.OpenContent(ctx, assetUUID)
			if legacyErr == nil {
				file, err = legacy.File, nil
				if filename == "." || filename == "" {
					filename = legacy.Filename
				}
			}
		}
	}
	if err != nil || file == nil {
		return ExportContent{}, domainError(CodeExportUnavailable, "导出文件不可用", "导出文件缺失或登记路径不安全，请重新导出。", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return ExportContent{}, err
	}
	if !info.Mode().IsRegular() || (record.ByteSize > 0 && info.Size() != record.ByteSize) {
		_ = file.Close()
		return ExportContent{}, domainError(CodeExportUnavailable, "导出文件状态异常", "磁盘导出文件与数据库登记大小不一致。", nil)
	}
	contentHash := record.ContentSHA256
	if !snapshotHashPattern.MatchString(contentHash) {
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			_ = file.Close()
			return ExportContent{}, err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			_ = file.Close()
			return ExportContent{}, err
		}
		contentHash = fmt.Sprintf("%x", hash.Sum(nil))
	}
	format, formatErr := NormalizeExportFormat(record.Format)
	if formatErr != nil {
		_ = file.Close()
		return ExportContent{}, domainError(CodeExportUnavailable, "导出格式无效", "数据库登记的导出格式无法下载。", formatErr)
	}
	extension := exportExtension(format)
	if filename == "." || filename == "" || !strings.HasSuffix(strings.ToLower(filename), "."+extension) {
		filename = "comic-export-" + record.UUID + "." + extension
	}
	contentType := "application/zip"
	if format == ExportFormatPDF {
		contentType = "application/pdf"
	}
	return ExportContent{
		File: file, Filename: filename, ContentType: contentType, ETag: fmt.Sprintf("\"sha256-%s\"", contentHash),
		LastModified: info.ModTime().UTC(), ExpiresAt: record.ExpiresAt.UTC(), ByteSize: info.Size(),
	}, nil
}

type ExportCleanupResult struct {
	ExpiredMarked      int `json:"expired_marked"`
	ExportsDeleted     int `json:"exports_deleted"`
	TasksDeleted       int `json:"tasks_deleted"`
	LegacyFilesPurged  int `json:"legacy_files_purged"`
	OrphanFilesDeleted int `json:"orphan_files_deleted"`
}

func (service *Service) CleanupExpiredExports(ctx context.Context, limit int) (ExportCleanupResult, error) {
	if limit <= 0 || limit > exportCleanupLimit {
		limit = exportCleanupLimit
	}
	p, _, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		return ExportCleanupResult{}, err
	}
	now := service.now().UTC()
	var candidates []exportRecord
	if err := service.store.DB().WithContext(ctx).
		Where("project_id = ? AND status IN ('ready','failed','cancelled','expired') AND expires_at IS NOT NULL AND expires_at <= ?", p.ID, now).
		Order("expires_at ASC,id ASC").Limit(limit).Find(&candidates).Error; err != nil {
		return ExportCleanupResult{}, err
	}
	result := ExportCleanupResult{}
	processed := 0
	var failures []error
	for _, candidate := range candidates {
		processed++
		claimed, claimErr := service.claimExpiredExport(ctx, candidate, now)
		if claimErr != nil {
			failures = append(failures, claimErr)
			continue
		}
		if !claimed {
			continue
		}
		if candidate.Status != "expired" {
			result.ExpiredMarked++
		}
		candidate.Status = "expired"
		if candidate.OutputFileID != nil {
			if err := service.files.RetireExportFile(ctx, *candidate.OutputFileID, now); err != nil {
				failures = append(failures, fmt.Errorf("retire legacy export %s: %w", candidate.UUID, err))
				continue
			}
		}
		if err := service.removeRegisteredExportArtifacts(candidate); err != nil {
			failures = append(failures, fmt.Errorf("remove export %s: %w", candidate.UUID, err))
			continue
		}
		deletedExport, deletedTask, err := service.deleteClaimedExport(ctx, candidate, now)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		result.ExportsDeleted += deletedExport
		result.TasksDeleted += deletedTask
	}

	remaining := limit - processed
	if remaining > 0 {
		purged, attempted, purgeErr := service.files.PurgeRetiredExportFiles(ctx, remaining)
		result.LegacyFilesPurged += purged
		remaining -= attempted
		if purgeErr != nil {
			failures = append(failures, purgeErr)
		}
	}
	if remaining > 0 {
		deleted, deleteErr := service.deleteOrphanExportTasks(ctx, p.ID, now, remaining)
		result.TasksDeleted += deleted
		remaining -= deleted
		if deleteErr != nil {
			failures = append(failures, deleteErr)
		}
	}
	if remaining > 0 {
		deleted, _, orphanErr := service.cleanupOrphanExportFiles(ctx, now, remaining)
		result.OrphanFilesDeleted += deleted
		if orphanErr != nil {
			failures = append(failures, orphanErr)
		}
	}
	if result.ExpiredMarked > 0 || result.ExportsDeleted > 0 || result.TasksDeleted > 0 || result.OrphanFilesDeleted > 0 {
		service.emit("comic:exports_changed", map[string]any{
			"expired": result.ExpiredMarked, "exports_deleted": result.ExportsDeleted,
			"tasks_deleted": result.TasksDeleted,
		})
	}
	return result, errors.Join(failures...)
}

func (service *Service) claimExpiredExport(ctx context.Context, candidate exportRecord, now time.Time) (bool, error) {
	claimed := false
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current exportRecord
		if err := tx.Where("id = ? AND project_id = ?", candidate.ID, candidate.ProjectID).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if current.ExpiresAt == nil || current.ExpiresAt.After(now) || (current.Status != "ready" && current.Status != "failed" && current.Status != "cancelled" && current.Status != "expired") {
			return nil
		}
		var taskStatus sql.NullString
		if err := tx.Table("production_task_runs").Select("status").Where("project_id = ? AND uuid = ?", current.ProjectID, current.TaskUUID).Row().Scan(&taskStatus); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if taskStatus.Valid && (taskStatus.String == "queued" || taskStatus.String == "running") {
			return nil
		}
		if current.Status != "expired" {
			result := tx.Model(&exportRecord{}).Where("id = ? AND status = ? AND expires_at <= ?", current.ID, current.Status, now).Update("status", "expired")
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return nil
			}
		}
		claimed = true
		return nil
	})
	return claimed, err
}

func (service *Service) deleteClaimedExport(ctx context.Context, candidate exportRecord, now time.Time) (int, int, error) {
	deletedExport, deletedTask := 0, 0
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var active int64
		if err := tx.Raw("SELECT COUNT(*) FROM production_task_runs WHERE project_id = ? AND uuid = ? AND status IN ('queued','running')", candidate.ProjectID, candidate.TaskUUID).Scan(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return nil
		}
		result := tx.Where("id = ? AND project_id = ? AND status = 'expired' AND expires_at <= ?", candidate.ID, candidate.ProjectID, now).Delete(&exportRecord{})
		if result.Error != nil {
			return result.Error
		}
		deletedExport = int(result.RowsAffected)
		if deletedExport == 0 {
			return nil
		}
		result = tx.Exec("DELETE FROM production_task_runs WHERE project_id = ? AND uuid = ? AND kind = 'comic_export' AND status IN ('completed','failed','cancelled','interrupted')", candidate.ProjectID, candidate.TaskUUID)
		if result.Error != nil {
			return result.Error
		}
		deletedTask = int(result.RowsAffected)
		return nil
	})
	return deletedExport, deletedTask, err
}

func (service *Service) registeredExportPath(record exportRecord) (string, error) {
	if strings.TrimSpace(record.RelativePath) == "" {
		return "", domainError(CodeExportUnavailable, "导出路径缺失", "导出记录没有登记文件路径。", nil)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(record.RelativePath)))
	if clean != record.RelativePath || filepath.ToSlash(filepath.Dir(filepath.FromSlash(clean))) != "exports" {
		return "", domainError(CodeExportUnavailable, "导出路径不安全", "导出文件必须直接位于项目 exports/ 目录。", nil)
	}
	format, err := NormalizeExportFormat(record.Format)
	if err != nil {
		return "", domainError(CodeExportUnavailable, "导出格式无效", "数据库登记的导出格式不受支持。", err)
	}
	var snapshot ExportSnapshot
	if err := jsonUnmarshalExportSnapshot(record.SnapshotJSON, &snapshot); err != nil {
		return "", err
	}
	if exportFormatForSnapshot(snapshot) != format {
		return "", domainError(CodeExportUnavailable, "导出格式不一致", "导出记录与冻结快照的格式不一致。", nil)
	}
	newPath := ExportRelativePath(record.UUID, record.Scope, snapshot.ChapterUUID, record.SnapshotHash, snapshot)
	legacyPath := filepath.ToSlash(filepath.Join("exports", safeExportNameForSnapshot(record.Scope, snapshot.ChapterUUID, record.SnapshotHash, snapshot)+".zip"))
	legacyAllowed := format == ExportFormatZIP && record.OutputFileID != nil && clean == legacyPath
	if clean != newPath && !legacyAllowed {
		return "", domainError(CodeExportUnavailable, "导出路径不受管理", "后台只处理 Lumi 登记的 ZIP/PDF 命名。", nil)
	}
	return service.store.ResolvePath(clean)
}

func jsonUnmarshalExportSnapshot(encoded string, snapshot *ExportSnapshot) error {
	if err := json.Unmarshal([]byte(encoded), snapshot); err != nil {
		return domainError(CodeSnapshotInvalid, "导出快照损坏", "无法验证导出文件路径。", err)
	}
	return nil
}

func (service *Service) removeRegisteredExportArtifacts(record exportRecord) error {
	if record.RelativePath == "" {
		return nil
	}
	path, err := service.registeredExportPath(record)
	if err != nil {
		return err
	}
	return service.store.WithFileCommit(func() error {
		var failures []error
		for _, candidate := range []string{path, path + ".part"} {
			if err := os.Remove(candidate); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, err)
			}
		}
		return errors.Join(failures...)
	})
}

func (service *Service) deleteOrphanExportTasks(ctx context.Context, projectID int64, now time.Time, limit int) (int, error) {
	cutoff := now.Add(-exportRetention)
	result := service.store.DB().WithContext(ctx).Exec(`DELETE FROM production_task_runs WHERE id IN (
		SELECT tasks.id FROM production_task_runs tasks
		WHERE tasks.project_id = ? AND tasks.kind = 'comic_export'
		  AND tasks.status IN ('completed','failed','cancelled','interrupted')
		  AND COALESCE(tasks.completed_at, tasks.created_at) <= ?
		  AND NOT EXISTS (SELECT 1 FROM comic_exports exports WHERE exports.task_uuid = tasks.uuid)
		ORDER BY COALESCE(tasks.completed_at, tasks.created_at) ASC, tasks.id ASC LIMIT ?
	)`, projectID, cutoff, limit)
	return int(result.RowsAffected), result.Error
}

func (service *Service) cleanupOrphanExportFiles(ctx context.Context, now time.Time, limit int) (int, int, error) {
	directory, err := service.store.ResolvePath("exports")
	if err != nil {
		return 0, 0, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	cutoff := now.Add(-exportRetention)
	deleted := 0
	attempted := 0
	var failures []error
	for _, entry := range entries {
		if attempted >= limit {
			break
		}
		if entry.IsDir() {
			continue
		}
		exportUUID, managed := exportUUIDFromManagedFilename(entry.Name())
		if !managed {
			continue
		}
		var exists bool
		if err := service.store.DB().WithContext(ctx).Raw("SELECT EXISTS(SELECT 1 FROM comic_exports WHERE uuid = ?)", exportUUID).Scan(&exists).Error; err != nil {
			failures = append(failures, err)
			continue
		}
		if exists {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().UTC().After(cutoff) {
			if err != nil {
				failures = append(failures, err)
			}
			continue
		}
		attempted++
		path, err := service.store.ResolvePath(filepath.ToSlash(filepath.Join("exports", entry.Name())))
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if err := service.store.WithFileCommit(func() error { return os.Remove(path) }); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, err)
			continue
		}
		deleted++
	}
	return deleted, attempted, errors.Join(failures...)
}

func exportUUIDFromManagedFilename(filename string) (string, bool) {
	base := filename
	if strings.HasSuffix(base, ".part") {
		base = strings.TrimSuffix(base, ".part")
	}
	lower := strings.ToLower(base)
	if !strings.HasSuffix(lower, ".zip") && !strings.HasSuffix(lower, ".pdf") {
		return "", false
	}
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if len(stem) < 37 || stem[len(stem)-37] != '-' {
		return "", false
	}
	uuidValue := stem[len(stem)-36:]
	prefix := stem[:len(stem)-37]
	if (!strings.HasPrefix(prefix, "comic-") && !strings.HasPrefix(prefix, "picture-book-")) || !isUUIDv7(uuidValue) {
		return "", false
	}
	return uuidValue, true
}
