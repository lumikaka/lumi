package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"gorm.io/gorm"
)

const defaultGCGracePeriod = 7 * 24 * time.Hour

type gcPlanRecord struct {
	ID             int64 `gorm:"primaryKey;autoIncrement"`
	UUID           string
	ProjectID      int64
	SnapshotHash   string
	Status         string
	EstimatedBytes int64
	CreatedAt      time.Time
	AppliedAt      *time.Time
}

func (gcPlanRecord) TableName() string { return "asset_gc_plans" }

type gcEntryRecord struct {
	ID               int64 `gorm:"primaryKey;autoIncrement"`
	GCPlanID         int64
	FileObjectID     *int64
	ObjectUUID       string
	SHA256           string
	SafeKeyPath      string
	ByteSize         int64
	ReferenceSummary string
}

func (gcEntryRecord) TableName() string { return "asset_gc_entries" }

func (service *Service) GCDryRun(ctx context.Context, grace time.Duration) (GCPlan, error) {
	if grace <= 0 {
		grace = defaultGCGracePeriod
	}
	projectRecord, _, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return GCPlan{}, err
	}
	cutoff := service.now().UTC().Add(-grace)
	objects, err := service.gcCandidates(ctx, projectRecord.ID, cutoff)
	if err != nil {
		return GCPlan{}, err
	}
	snapshot := gcSnapshot(objects)
	uuidValue, err := newUUIDv7()
	if err != nil {
		return GCPlan{}, err
	}
	now := service.now().UTC()
	plan := gcPlanRecord{UUID: uuidValue, ProjectID: projectRecord.ID, SnapshotHash: snapshot, Status: "dry_run", CreatedAt: now}
	referenceSummaries := make(map[int64]string, len(objects))
	for _, object := range objects {
		plan.EstimatedBytes += object.ByteSize
		summary, summaryErr := service.gcReferenceSummary(ctx, object.ID)
		if summaryErr != nil {
			return GCPlan{}, summaryErr
		}
		referenceSummaries[object.ID] = summary
	}
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&plan).Error; err != nil {
			return err
		}
		for _, object := range objects {
			objectID := object.ID
			entry := gcEntryRecord{GCPlanID: plan.ID, FileObjectID: &objectID, ObjectUUID: object.UUID, SHA256: object.SHA256, SafeKeyPath: object.KeyPath, ByteSize: object.ByteSize, ReferenceSummary: referenceSummaries[object.ID]}
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return GCPlan{}, err
	}
	return service.getGCPlan(ctx, plan.UUID)
}

func (service *Service) gcReferenceSummary(ctx context.Context, objectID int64) (string, error) {
	var counts struct {
		EligibleDeletedFiles int64
		ActiveFiles          int64
		BusinessRefs         int64
		DerivedRefs          int64
		PendingUploads       int64
	}
	err := service.store.DB().WithContext(ctx).Raw(`SELECT
		(SELECT COUNT(*) FROM files WHERE file_object_id=? AND deleted_at IS NOT NULL) AS eligible_deleted_files,
		(SELECT COUNT(*) FROM files WHERE file_object_id=? AND deleted_at IS NULL) AS active_files,
		(SELECT COUNT(*) FROM story_source_items WHERE file_id IN (SELECT id FROM files WHERE file_object_id=?)) AS business_refs,
		(SELECT COUNT(*) FROM files child JOIN files parent ON parent.id=child.source_file_id WHERE parent.file_object_id=?) AS derived_refs,
		(SELECT COUNT(*) FROM upload_stashed WHERE file_object_id=? AND state <> 'consumed') AS pending_uploads`, objectID, objectID, objectID, objectID, objectID).Scan(&counts).Error
	if err != nil {
		return "", err
	}
	var assetUUIDs []string
	if err := service.store.DB().WithContext(ctx).Table("files").Where("file_object_id = ?", objectID).Order("uuid ASC").Pluck("uuid", &assetUUIDs).Error; err != nil {
		return "", err
	}
	encoded, err := json.Marshal(map[string]any{"asset_uuids": assetUUIDs, "eligible_deleted_files": counts.EligibleDeletedFiles, "active_files": counts.ActiveFiles, "business_refs": counts.BusinessRefs, "derived_refs": counts.DerivedRefs, "pending_uploads": counts.PendingUploads})
	return string(encoded), err
}

func (service *Service) GCApply(ctx context.Context, planUUID string, grace time.Duration) (GCPlan, error) {
	var plan GCPlan
	err := service.store.WithFileCommit(func() error {
		var err error
		plan, err = service.gcApply(ctx, planUUID, grace)
		return err
	})
	return plan, err
}

func (service *Service) gcApply(ctx context.Context, planUUID string, grace time.Duration) (GCPlan, error) {
	if grace <= 0 {
		grace = defaultGCGracePeriod
	}
	plan, record, entries, err := service.gcPlanRecords(ctx, planUUID)
	if err != nil {
		return GCPlan{}, err
	}
	if record.Status != "dry_run" {
		return GCPlan{}, fileError(CodeInvalidState, "GC plan 已经处理", "只有 dry_run plan 可以 apply。", nil)
	}
	cutoff := service.now().UTC().Add(-grace)
	current, err := service.gcCandidates(ctx, record.ProjectID, cutoff)
	if err != nil {
		return GCPlan{}, err
	}
	if gcSnapshot(current) != record.SnapshotHash {
		_ = service.store.DB().WithContext(ctx).Model(&gcPlanRecord{}).Where("id = ?", record.ID).Update("status", "stale").Error
		return GCPlan{}, fileError(CodeGCPlanStale, "GC dry-run 已过期", "引用或候选对象发生变化，请重新 dry-run。", nil)
	}
	byUUID := map[string]objectRecord{}
	for _, object := range current {
		byUUID[object.UUID] = object
	}
	for _, entry := range entries {
		object, ok := byUUID[entry.ObjectUUID]
		if !ok || object.SHA256 != entry.SHA256 || object.KeyPath != entry.SafeKeyPath {
			return GCPlan{}, fileError(CodeGCPlanStale, "GC 候选已变化", "对象摘要与 dry-run snapshot 不一致。", nil)
		}
		path, err := service.assetPath(object.KeyPath)
		if object.State == ObjectQuarantined {
			path, err = service.quarantinePath(object.UUID, object.CanonicalExt)
		}
		if err != nil {
			return GCPlan{}, err
		}
		err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var eligible int64
			if err := tx.Raw(`SELECT COUNT(*) FROM file_objects o WHERE o.id=? AND o.project_id=? AND o.state IN ('ready','missing','corrupt','quarantined') AND EXISTS (SELECT 1 FROM files f WHERE f.file_object_id=o.id) AND NOT EXISTS (SELECT 1 FROM files f WHERE f.file_object_id=o.id AND (f.deleted_at IS NULL OR f.deleted_at > ?)) AND NOT EXISTS (SELECT 1 FROM story_source_items ssi JOIN files f ON f.id=ssi.file_id WHERE f.file_object_id=o.id) AND NOT EXISTS (SELECT 1 FROM files child JOIN files parent ON parent.id=child.source_file_id WHERE parent.file_object_id=o.id) AND NOT EXISTS (SELECT 1 FROM upload_stashed u WHERE u.file_object_id=o.id AND u.state <> 'consumed')`, object.ID, record.ProjectID, cutoff).Scan(&eligible).Error; err != nil {
				return err
			}
			if eligible != 1 {
				return fileError(CodeGCPlanStale, "GC 对象引用已变化", "apply 前的事务内引用复检未通过。", nil)
			}
			var refs int64
			if err := tx.Table("story_source_items").Where("file_id IN (SELECT id FROM files WHERE file_object_id = ?)", object.ID).Count(&refs).Error; err != nil {
				return err
			}
			if refs > 0 {
				return fileError(CodeReferenced, "Asset 仍被业务引用", "Story import 引用阻止物理清理。", nil)
			}
			if err := tx.Where("state = ? AND finalized_file_id IN (SELECT id FROM files WHERE file_object_id = ?)", StateConsumed, object.ID).Delete(&uploadRecord{}).Error; err != nil {
				return err
			}
			if err := tx.Where("file_object_id = ? AND deleted_at IS NOT NULL", object.ID).Delete(&fileRecord{}).Error; err != nil {
				return err
			}
			result := tx.Where("id = ? AND sha256 = ?", object.ID, object.SHA256).Delete(&objectRecord{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fileError(CodeGCPlanStale, "GC 对象已变化", "对象未按 snapshot 删除。", nil)
			}
			return nil
		})
		if err != nil {
			return GCPlan{}, err
		}
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return GCPlan{}, removeErr
		}
	}
	applied := service.now().UTC()
	if err := service.store.DB().WithContext(ctx).Model(&gcPlanRecord{}).Where("id = ? AND status = ?", record.ID, "dry_run").Updates(map[string]any{"status": "applied", "applied_at": applied}).Error; err != nil {
		return GCPlan{}, err
	}
	plan.Status = "applied"
	plan.AppliedAt = &applied
	return plan, nil
}

func (service *Service) gcCandidates(ctx context.Context, projectID int64, cutoff time.Time) ([]objectRecord, error) {
	var objects []objectRecord
	err := service.store.DB().WithContext(ctx).Raw(`SELECT o.* FROM file_objects o WHERE o.project_id = ? AND o.state IN ('ready','missing','corrupt','quarantined') AND EXISTS (SELECT 1 FROM files f WHERE f.file_object_id=o.id) AND NOT EXISTS (SELECT 1 FROM files f WHERE f.file_object_id=o.id AND (f.deleted_at IS NULL OR f.deleted_at > ?)) AND NOT EXISTS (SELECT 1 FROM story_source_items ssi JOIN files f ON f.id=ssi.file_id WHERE f.file_object_id=o.id) AND NOT EXISTS (SELECT 1 FROM files child JOIN files parent ON parent.id=child.source_file_id WHERE parent.file_object_id=o.id) AND NOT EXISTS (SELECT 1 FROM upload_stashed u WHERE u.file_object_id=o.id AND u.state <> 'consumed') ORDER BY o.uuid ASC`, projectID, cutoff).Scan(&objects).Error
	return objects, err
}
func gcSnapshot(objects []objectRecord) string {
	sort.Slice(objects, func(i, j int) bool { return objects[i].UUID < objects[j].UUID })
	hash := sha256.New()
	for _, object := range objects {
		fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%s\x00%d\n", object.UUID, object.SHA256, object.KeyPath, object.State, object.ByteSize)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (service *Service) getGCPlan(ctx context.Context, uuidValue string) (GCPlan, error) {
	plan, _, _, err := service.gcPlanRecords(ctx, uuidValue)
	return plan, err
}
func (service *Service) gcPlanRecords(ctx context.Context, uuidValue string) (GCPlan, gcPlanRecord, []gcEntryRecord, error) {
	if !isUUIDv7(uuidValue) {
		return GCPlan{}, gcPlanRecord{}, nil, fileError(CodeGCPlanNotFound, "GC plan 不存在", "plan_uuid 必须是 UUIDv7。", nil)
	}
	var record gcPlanRecord
	err := service.store.DB().WithContext(ctx).Where("uuid = ? AND project_id = (SELECT id FROM projects WHERE uuid = ?)", uuidValue, service.store.ProjectUUID()).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return GCPlan{}, record, nil, fileError(CodeGCPlanNotFound, "GC plan 不存在", "该 dry-run 不属于当前项目。", err)
	}
	if err != nil {
		return GCPlan{}, record, nil, err
	}
	var rows []gcEntryRecord
	if err := service.store.DB().WithContext(ctx).Where("gc_plan_id = ?", record.ID).Order("id ASC").Find(&rows).Error; err != nil {
		return GCPlan{}, record, nil, err
	}
	items := make([]GCEntry, 0, len(rows))
	for _, row := range rows {
		summary := map[string]any{}
		_ = json.Unmarshal([]byte(row.ReferenceSummary), &summary)
		items = append(items, GCEntry{ObjectUUID: row.ObjectUUID, SHA256: row.SHA256, SafeKeyPath: row.SafeKeyPath, ByteSize: row.ByteSize, ReferenceSummary: summary})
	}
	return GCPlan{UUID: record.UUID, SnapshotHash: record.SnapshotHash, Status: record.Status, EstimatedBytes: record.EstimatedBytes, Entries: items, CreatedAt: record.CreatedAt, AppliedAt: record.AppliedAt}, record, rows, nil
}
