package files

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

type scanRecord struct {
	ID             int64 `gorm:"primaryKey;autoIncrement"`
	UUID           string
	ProjectID      int64
	Mode           string
	Status         string
	Progress       int
	CheckedObjects int
	FindingCount   int
	SummaryJSON    string
	StartedAt      *time.Time
	CompletedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (scanRecord) TableName() string { return "integrity_scans" }

type findingRecord struct {
	ID           int64 `gorm:"primaryKey;autoIncrement"`
	UUID         string
	ScanID       int64
	FileObjectID *int64
	Kind         string
	Severity     string
	SafePath     *string
	Summary      string
	Resolution   string
	ResultJSON   string
	CreatedAt    time.Time
	ResolvedAt   *time.Time
}

func (findingRecord) TableName() string { return "integrity_findings" }

func (service *Service) RunIntegrityScan(ctx context.Context) (IntegrityScan, error) {
	return service.RunIntegrityScanWithProgress(ctx, nil)
}

func (service *Service) RunIntegrityScanWithProgress(ctx context.Context, onProgress func(int) error) (IntegrityScan, error) {
	projectRecord, _, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return IntegrityScan{}, err
	}
	uuidValue, err := newUUIDv7()
	if err != nil {
		return IntegrityScan{}, err
	}
	now := service.now().UTC()
	record := scanRecord{UUID: uuidValue, ProjectID: projectRecord.ID, Mode: "full", Status: "running", Progress: 0, SummaryJSON: "{}", StartedAt: &now, CreatedAt: now, UpdatedAt: now}
	if err := service.store.DB().WithContext(ctx).Create(&record).Error; err != nil {
		return IntegrityScan{}, fileError(CodeInvalidState, "完整性扫描已在运行", "同一项目同一时间只能有一个 full scan。", err)
	}
	service.emit("integrity_scan/progress", map[string]any{"scan_uuid": record.UUID, "status": "running", "progress": 0})
	if _, err := service.Reconcile(ctx, 1000); err != nil {
		return service.failScan(ctx, record, err)
	}
	if onProgress != nil {
		if err := onProgress(10); err != nil {
			return service.failScan(ctx, record, err)
		}
	}
	var objects []objectRecord
	if err := service.store.DB().WithContext(ctx).Where("project_id = ?", projectRecord.ID).Order("id ASC").Find(&objects).Error; err != nil {
		return service.failScan(ctx, record, err)
	}
	counts := map[string]int{"pending": 0, "missing": 0, "corrupt": 0, "quarantined": 0, "orphan": 0, "thumbnail": 0}
	known := make(map[string]struct{}, len(objects))
	for index, object := range objects {
		known[filepath.ToSlash(object.KeyPath)] = struct{}{}
		kind, severity, summary := "", "warning", ""
		switch object.State {
		case ObjectPending:
			kind, summary = "pending", "对象提交尚未完成"
		case ObjectMissing:
			kind, severity, summary = "missing", "error", "数据库对象缺少对应磁盘文件"
		case ObjectCorrupt:
			kind, severity, summary = "corrupt", "error", "对象已标记为损坏"
		case ObjectQuarantined:
			kind, severity, summary = "quarantined", "error", "对象已隔离，等待人工处理"
		case ObjectReady:
			if verifyErr := service.verifyObjectFile(object); verifyErr != nil {
				kind, severity, summary = "corrupt", "error", "磁盘内容与 SHA-256、MIME 或尺寸摘要不一致"
				if errors.Is(verifyErr, os.ErrNotExist) {
					kind, summary = "missing", "数据库对象缺少对应磁盘文件"
					_ = service.store.DB().WithContext(ctx).Model(&objectRecord{}).Where("id = ?", object.ID).Update("state", ObjectMissing).Error
					service.emitObjectAssets(ctx, object.ID, "asset/missing", ObjectMissing)
				} else {
					if quarantineErr := service.quarantineObject(ctx, object); quarantineErr != nil {
						_ = service.store.DB().WithContext(ctx).Model(&objectRecord{}).Where("id = ?", object.ID).Update("state", ObjectCorrupt).Error
						service.emitObjectAssets(ctx, object.ID, "asset/updated", ObjectCorrupt)
					} else {
						service.emitObjectAssets(ctx, object.ID, "asset/updated", ObjectQuarantined)
					}
				}
			}
		}
		if kind != "" {
			counts[kind]++
			if err := service.createFinding(ctx, record.ID, &object, kind, severity, object.KeyPath, summary); err != nil {
				return service.failScan(ctx, record, err)
			}
		}
		if kind == "" && object.State == ObjectReady && strings.HasPrefix(object.MIMEType, "image/") {
			thumbnailFindings, thumbnailErr := service.scanThumbnailCache(ctx, record.ID, object)
			if thumbnailErr != nil {
				return service.failScan(ctx, record, thumbnailErr)
			}
			counts["thumbnail"] += thumbnailFindings
		}
		progress := 10
		if len(objects) > 0 {
			progress = 10 + (index+1)*70/len(objects)
		}
		if index%25 == 0 {
			_ = service.store.DB().WithContext(ctx).Model(&scanRecord{}).Where("id = ?", record.ID).Updates(map[string]any{"progress": progress, "checked_objects": index + 1, "updated_at": service.now().UTC()}).Error
			service.emit("integrity_scan/progress", map[string]any{"scan_uuid": record.UUID, "status": "running", "progress": progress})
			if onProgress != nil {
				if err := onProgress(progress); err != nil {
					return service.failScan(ctx, record, err)
				}
			}
		}
	}
	assetsRoot, err := service.store.ResolvePath("assets")
	if err != nil {
		return service.failScan(ctx, record, err)
	}
	var orphans []string
	walkErr := filepath.WalkDir(assetsRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == assetsRoot {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			rel, _ := filepath.Rel(assetsRoot, path)
			orphans = append(orphans, filepath.ToSlash(rel))
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(assetsRoot, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if _, ok := known[key]; !ok {
			orphans = append(orphans, key)
		}
		return nil
	})
	if walkErr != nil {
		return service.failScan(ctx, record, walkErr)
	}
	sort.Strings(orphans)
	for _, key := range orphans {
		counts["orphan"]++
		if err := service.createFinding(ctx, record.ID, nil, "orphan", "warning", key, "assets 中的文件没有 file_objects 记录，未自动导入或删除"); err != nil {
			return service.failScan(ctx, record, err)
		}
	}
	databaseFindings, err := service.scanDatabaseIntegrity(ctx, record.ID)
	if err != nil {
		return service.failScan(ctx, record, err)
	}
	counts["corrupt"] += databaseFindings
	if onProgress != nil {
		if err := onProgress(95); err != nil {
			return service.failScan(ctx, record, err)
		}
	}
	summaryJSON, _ := json.Marshal(counts)
	completed := service.now().UTC()
	total := 0
	for _, value := range counts {
		total += value
	}
	if err := service.store.DB().WithContext(ctx).Model(&scanRecord{}).Where("id = ?", record.ID).Updates(map[string]any{"status": "completed", "progress": 100, "checked_objects": len(objects), "finding_count": total, "summary_json": string(summaryJSON), "completed_at": completed, "updated_at": completed}).Error; err != nil {
		return service.failScan(ctx, record, err)
	}
	service.emit("integrity_scan/completed", map[string]any{"scan_uuid": record.UUID, "status": "completed", "progress": 100, "finding_count": total})
	return service.GetIntegrityScan(ctx, record.UUID)
}

func (service *Service) scanThumbnailCache(ctx context.Context, scanID int64, object objectRecord) (int, error) {
	findings := 0
	for _, profile := range []string{"grid_256", "detail_1024"} {
		relative := filepath.ToSlash(filepath.Join(".lumi", "thumbnails", object.SHA256, thumbnailProcessorVersion+"-"+profile+".jpg"))
		path, err := service.store.ResolvePath(relative)
		if err != nil {
			if findingErr := service.createFinding(ctx, scanID, &object, "thumbnail", "warning", relative, "缩略图缓存路径不安全，可重新构建"); findingErr != nil {
				return findings, findingErr
			}
			findings++
			continue
		}
		info, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		valid := statErr == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 && info.Size() > 0
		if valid {
			file, openErr := os.Open(path)
			if openErr != nil {
				valid = false
			} else {
				config, format, decodeErr := image.DecodeConfig(file)
				_ = file.Close()
				maxDimension := thumbnailProfiles[profile]
				valid = decodeErr == nil && format == "jpeg" && config.Width > 0 && config.Height > 0 && config.Width <= maxDimension && config.Height <= maxDimension
			}
		}
		if !valid {
			if findingErr := service.createFinding(ctx, scanID, &object, "thumbnail", "warning", relative, "缩略图缓存损坏或尺寸无效，可安全重新构建"); findingErr != nil {
				return findings, findingErr
			}
			findings++
		}
	}
	return findings, nil
}

func (service *Service) scanDatabaseIntegrity(ctx context.Context, scanID int64) (int, error) {
	findings := 0
	var integrity string
	if err := service.store.DB().WithContext(ctx).Raw("PRAGMA integrity_check").Scan(&integrity).Error; err != nil {
		return findings, err
	}
	if integrity != "ok" {
		if err := service.createFinding(ctx, scanID, nil, "corrupt", "error", "", "SQLite integrity_check 报告项目数据库异常"); err != nil {
			return findings, err
		}
		findings++
	}
	rows, err := service.store.DB().WithContext(ctx).Raw("PRAGMA foreign_key_check").Rows()
	if err != nil {
		return findings, err
	}
	defer rows.Close()
	for rows.Next() {
		var table, parent string
		var rowID any
		var foreignKeyID int64
		if err := rows.Scan(&table, &rowID, &parent, &foreignKeyID); err != nil {
			return findings, err
		}
		summary := "业务外键完整性异常：" + table + " → " + parent
		if err := service.createFinding(ctx, scanID, nil, "corrupt", "error", "", summary); err != nil {
			return findings, err
		}
		findings++
	}
	return findings, rows.Err()
}

func (service *Service) createFinding(ctx context.Context, scanID int64, object *objectRecord, kind, severity, safePath, summary string) error {
	uuidValue, err := newUUIDv7()
	if err != nil {
		return err
	}
	var objectID *int64
	if object != nil {
		value := object.ID
		objectID = &value
	}
	var path *string
	if safePath != "" {
		value := filepath.ToSlash(safePath)
		path = &value
	}
	return service.store.DB().WithContext(ctx).Create(&findingRecord{UUID: uuidValue, ScanID: scanID, FileObjectID: objectID, Kind: kind, Severity: severity, SafePath: path, Summary: summary, Resolution: "open", ResultJSON: "{}", CreatedAt: service.now().UTC()}).Error
}

func (service *Service) quarantineObject(ctx context.Context, object objectRecord) error {
	return service.store.WithFileCommit(func() error {
		source, err := service.assetPath(object.KeyPath)
		if err != nil {
			return err
		}
		target, err := service.quarantinePath(object.UUID, object.CanonicalExt)
		if err != nil {
			return err
		}
		if info, statErr := os.Lstat(target); statErr == nil {
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return fileError(CodeUnsafePath, "隔离目标不安全", "quarantine 目标必须是普通文件。", nil)
			}
			if _, sourceErr := os.Lstat(source); sourceErr == nil {
				return fileError(CodeInvalidState, "隔离目标已经存在", "不会覆盖现有隔离内容。", nil)
			} else if !errors.Is(sourceErr, os.ErrNotExist) {
				return sourceErr
			}
		} else if errors.Is(statErr, os.ErrNotExist) {
			if err := os.Rename(source, target); err != nil {
				return err
			}
			if err := syncDirectory(filepath.Dir(target)); err != nil {
				return err
			}
		} else {
			return statErr
		}
		return service.store.DB().WithContext(ctx).Model(&objectRecord{}).Where("id = ?", object.ID).Update("state", ObjectQuarantined).Error
	})
}

func (service *Service) failScan(ctx context.Context, record scanRecord, cause error) (IntegrityScan, error) {
	now := service.now().UTC()
	_ = service.store.DB().WithContext(context.WithoutCancel(ctx)).Model(&scanRecord{}).Where("id = ?", record.ID).Updates(map[string]any{"status": "failed", "completed_at": now, "updated_at": now}).Error
	return IntegrityScan{}, cause
}

func (service *Service) GetIntegrityScan(ctx context.Context, scanUUID string) (IntegrityScan, error) {
	if !isUUIDv7(scanUUID) {
		return IntegrityScan{}, fileError(CodeScanNotFound, "完整性扫描不存在", "scan_uuid 必须是 UUIDv7。", nil)
	}
	var record scanRecord
	err := service.store.DB().WithContext(ctx).Where("uuid = ? AND project_id = (SELECT id FROM projects WHERE uuid = ?)", scanUUID, service.store.ProjectUUID()).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return IntegrityScan{}, fileError(CodeScanNotFound, "完整性扫描不存在", "该扫描不属于当前项目。", err)
	}
	if err != nil {
		return IntegrityScan{}, err
	}
	var findings []findingRecord
	if err := service.store.DB().WithContext(ctx).Where("scan_id = ?", record.ID).Order("id ASC").Find(&findings).Error; err != nil {
		return IntegrityScan{}, err
	}
	return scanDTO(record, findings), nil
}

func (service *Service) ListIntegrityScans(ctx context.Context, limit int) ([]IntegrityScan, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var records []scanRecord
	if err := service.store.DB().WithContext(ctx).Where("project_id = (SELECT id FROM projects WHERE uuid = ?)", service.store.ProjectUUID()).Order("created_at DESC,id DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]IntegrityScan, 0, len(records))
	for _, record := range records {
		result = append(result, scanDTO(record, nil))
	}
	return result, nil
}

func scanDTO(record scanRecord, findings []findingRecord) IntegrityScan {
	summary := map[string]any{}
	_ = json.Unmarshal([]byte(record.SummaryJSON), &summary)
	items := make([]Finding, 0, len(findings))
	for _, item := range findings {
		safe := ""
		if item.SafePath != nil {
			safe = *item.SafePath
		}
		items = append(items, Finding{UUID: item.UUID, Kind: item.Kind, Severity: item.Severity, SafePath: safe, Summary: item.Summary, Resolution: item.Resolution, CreatedAt: item.CreatedAt, ResolvedAt: item.ResolvedAt})
	}
	return IntegrityScan{UUID: record.UUID, Mode: record.Mode, Status: record.Status, Progress: record.Progress, CheckedObjects: record.CheckedObjects, FindingCount: record.FindingCount, Summary: summary, Findings: items, StartedAt: record.StartedAt, CompletedAt: record.CompletedAt, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}
