package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"gorm.io/gorm"
)

type maintenanceSnapshot struct {
	Version    int    `json:"version"`
	PlanUUID   string `json:"plan_uuid,omitempty"`
	GraceHours int    `json:"grace_hours,omitempty"`
}

func validMaintenanceKind(kind string) bool {
	switch kind {
	case KindAssetReconcile, KindAssetIntegrityScan, KindAssetThumbnailRebuild, KindAssetUploadCleanup, KindAssetGCApply:
		return true
	default:
		return false
	}
}

func (manager *Manager) CreateMaintenanceTask(ctx context.Context, projectUUID string, input CreateMaintenanceInput) (MaintenanceTask, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return MaintenanceTask{}, err
	}
	input.Kind = strings.TrimSpace(input.Kind)
	input.PlanUUID = strings.TrimSpace(input.PlanUUID)
	if !validMaintenanceKind(input.Kind) {
		return MaintenanceTask{}, taskError(CodeInvalidTask, "维护任务类型无效", "kind 必须来自 Asset Store 维护任务允许列表。", nil)
	}
	if input.Kind == KindAssetGCApply && !isUUIDv7(input.PlanUUID) {
		return MaintenanceTask{}, taskError(CodeInvalidTask, "GC plan UUID 无效", "asset_gc_apply 必须引用公开 dry-run plan UUID。", nil)
	}
	if input.GraceHours < 0 || input.GraceHours > 24*365 {
		return MaintenanceTask{}, taskError(CodeInvalidTask, "GC grace period 无效", "grace_hours 必须在 0 到 8760 之间。", nil)
	}
	if input.Kind != KindAssetGCApply && input.GraceHours != 0 {
		return MaintenanceTask{}, taskError(CodeInvalidTask, "维护任务参数无效", "只有 asset_gc_apply 接受 grace_hours。", nil)
	}
	taskUUID, err := newUUIDv7()
	if err != nil {
		return MaintenanceTask{}, err
	}
	resourceUUID := taskUUID
	if input.PlanUUID != "" {
		resourceUUID = input.PlanUUID
	}
	snapshotJSON, err := json.Marshal(maintenanceSnapshot{Version: 1, PlanUUID: input.PlanUUID, GraceHours: input.GraceHours})
	if err != nil {
		return MaintenanceTask{}, err
	}
	now := manager.now().UTC()
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return MaintenanceTask{}, err
	}
	defer tx.Rollback()
	maxAttempts := 3
	if input.Kind == KindAssetGCApply {
		maxAttempts = 1
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO asset_maintenance_runs (uuid,project_id,river_job_id,kind,resource_uuid,input_version,input_snapshot,status,progress,attempt,max_attempts,error_code,error_message,created_at,updated_at) VALUES (?,?,NULL,?,?,1,?,'queued',0,0,?,'','',?,?)`, taskUUID, runtime.projectID, input.Kind, resourceUUID, string(snapshotJSON), maxAttempts, now, now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return MaintenanceTask{}, taskError(CodeTaskConflict, "同类维护任务已在运行", "每个项目每类 Asset 维护任务最多一个 active job。", err)
		}
		return MaintenanceTask{}, err
	}
	taskID, err := result.LastInsertId()
	if err != nil {
		return MaintenanceTask{}, err
	}
	if err := appendMaintenanceEventTx(ctx, tx, taskID, "task_queued", map[string]any{"project_uuid": projectUUID, "task_uuid": taskUUID, "kind": input.Kind, "resource_uuid": resourceUUID, "status": StatusQueued, "progress": 0}, now); err != nil {
		return MaintenanceTask{}, err
	}
	inserted, err := runtime.client.InsertTx(ctx, tx, maintenanceArgs{Version: 1, ProjectUUID: projectUUID, TaskUUID: taskUUID, MaintenanceKind: input.Kind, ResourceUUID: resourceUUID}, &river.InsertOpts{Queue: QueueAssetMaintenance, MaxAttempts: maxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateRetryable, rivertype.JobStateScheduled}}})
	if err != nil {
		return MaintenanceTask{}, err
	}
	if inserted.UniqueSkippedAsDuplicate {
		return MaintenanceTask{}, taskError(CodeTaskConflict, "同类维护任务已在队列中", "River unique job 拒绝了重复维护任务。", nil)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE asset_maintenance_runs SET river_job_id = ? WHERE id = ?", inserted.Job.ID, taskID); err != nil {
		return MaintenanceTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return MaintenanceTask{}, err
	}
	task, err := manager.GetMaintenanceTask(ctx, projectUUID, taskUUID)
	if err == nil {
		runtime.broadcastMaintenance("task:queued", task)
	}
	return task, err
}

func (manager *Manager) GetMaintenanceTask(ctx context.Context, projectUUID, taskUUID string) (MaintenanceTask, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return MaintenanceTask{}, err
	}
	record, err := getMaintenanceRecord(ctx, runtime.store.DB(), runtime.projectID, taskUUID)
	if err != nil {
		return MaintenanceTask{}, err
	}
	return record.DTO(), nil
}
func (manager *Manager) ListMaintenanceTasks(ctx context.Context, projectUUID string, limit int) ([]MaintenanceTask, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var records []maintenanceRecord
	if err := runtime.store.DB().WithContext(ctx).Where("project_id = ?", runtime.projectID).Order("created_at DESC,id DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	items := make([]MaintenanceTask, 0, len(records))
	for _, record := range records {
		items = append(items, record.DTO())
	}
	return items, nil
}

func (manager *Manager) ListMaintenanceTaskEvents(ctx context.Context, projectUUID, taskUUID string, before, after int64, limit int) ([]TaskEvent, CursorPagination, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return nil, CursorPagination{}, err
	}
	record, err := getMaintenanceRecord(ctx, runtime.store.DB(), runtime.projectID, taskUUID)
	if err != nil {
		return nil, CursorPagination{}, err
	}
	if before < 0 || after < 0 || (before > 0 && after > 0) {
		return nil, CursorPagination{}, taskError(CodeInvalidTask, "维护事件游标无效", "before 与 after 必须是非负 sequence，且不能同时使用。", nil)
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := runtime.store.DB().WithContext(ctx).Where("maintenance_run_id = ?", record.ID)
	order := "sequence ASC"
	if before > 0 {
		query = query.Where("sequence < ?", before)
		order = "sequence DESC"
	} else if after > 0 {
		query = query.Where("sequence > ?", after)
	}
	var rows []maintenanceEventRecord
	if err := query.Order(order).Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, CursorPagination{}, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	if before > 0 {
		for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
			rows[left], rows[right] = rows[right], rows[left]
		}
	}
	items := make([]TaskEvent, 0, len(rows))
	for _, row := range rows {
		items = append(items, TaskEvent{UUID: row.UUID, Sequence: row.Sequence, EventType: row.EventType, Payload: json.RawMessage(row.Payload), CreatedAt: row.CreatedAt})
	}
	var next, previous *string
	if len(items) > 0 {
		first := fmt.Sprintf("%d", items[0].Sequence)
		last := fmt.Sprintf("%d", items[len(items)-1].Sequence)
		if before > 0 {
			next = &last
			if hasMore {
				previous = &first
			}
		} else {
			if hasMore {
				next = &last
			}
			if after > 0 {
				previous = &first
			}
		}
	}
	return items, CursorPagination{PerPage: limit, NextCursor: next, PrevCursor: previous, HasMore: hasMore}, nil
}

func (manager *Manager) CancelMaintenanceTask(ctx context.Context, projectUUID, taskUUID string) (MaintenanceTask, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return MaintenanceTask{}, err
	}
	runtime.cancelWork(taskUUID)
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return MaintenanceTask{}, err
	}
	defer tx.Rollback()
	record, found, err := findMaintenanceTx(ctx, tx, runtime.projectID, taskUUID)
	if err != nil {
		return MaintenanceTask{}, err
	}
	if !found {
		return MaintenanceTask{}, taskError(CodeTaskNotFound, "维护任务不存在", "该任务可能已经清理。", nil)
	}
	if record.Status == StatusCompleted || record.Status == StatusFailed || record.Status == StatusCancelled || record.Status == StatusInterrupted {
		_ = tx.Commit()
		return record.DTO(), nil
	}
	if record.RiverJobID == nil {
		return MaintenanceTask{}, taskError(CodeTaskPersistenceFailed, "维护任务缺少队列关联", "任务无法安全取消。", nil)
	}
	now := manager.now().UTC()
	if _, err := runtime.client.JobCancelTx(ctx, tx, *record.RiverJobID); err != nil {
		return MaintenanceTask{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE asset_maintenance_runs SET status='cancelled',cancel_requested_at=?,completed_at=?,updated_at=? WHERE id=?`, now, now, now, record.ID); err != nil {
		return MaintenanceTask{}, err
	}
	if err := appendMaintenanceEventTx(ctx, tx, record.ID, "cancel_requested", map[string]any{"project_uuid": projectUUID, "task_uuid": taskUUID, "kind": record.Kind, "status": StatusCancelled}, now); err != nil {
		return MaintenanceTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return MaintenanceTask{}, err
	}
	_, _ = runtime.client.JobCancel(context.WithoutCancel(ctx), *record.RiverJobID)
	task, err := manager.GetMaintenanceTask(ctx, projectUUID, taskUUID)
	if err == nil {
		runtime.broadcastMaintenance("task:cancelled", task)
	}
	return task, err
}

func getMaintenanceRecord(ctx context.Context, db *gorm.DB, projectID int64, taskUUID string) (maintenanceRecord, error) {
	if !isUUIDv7(taskUUID) {
		return maintenanceRecord{}, taskError(CodeInvalidTask, "维护任务 UUID 无效", "task_uuid 必须是 UUIDv7。", nil)
	}
	var record maintenanceRecord
	err := db.WithContext(ctx).Where("project_id = ? AND uuid = ?", projectID, taskUUID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, taskError(CodeTaskNotFound, "维护任务不存在", "该任务不属于当前项目。", err)
	}
	return record, err
}
func findMaintenanceTx(ctx context.Context, tx *sql.Tx, projectID int64, taskUUID string) (maintenanceRecord, bool, error) {
	var record maintenanceRecord
	err := tx.QueryRowContext(ctx, `SELECT id,uuid,project_id,river_job_id,kind,resource_uuid,input_version,input_snapshot,status,progress,attempt,max_attempts,error_code,error_message,cancel_requested_at,started_at,completed_at,created_at,updated_at FROM asset_maintenance_runs WHERE project_id=? AND uuid=?`, projectID, taskUUID).Scan(&record.ID, &record.UUID, &record.ProjectID, &record.RiverJobID, &record.Kind, &record.ResourceUUID, &record.InputVersion, &record.InputSnapshot, &record.Status, &record.Progress, &record.Attempt, &record.MaxAttempts, &record.ErrorCode, &record.ErrorMessage, &record.CancelRequestedAt, &record.StartedAt, &record.CompletedAt, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return record, false, nil
	}
	return record, err == nil, err
}

func appendMaintenanceEventTx(ctx context.Context, tx *sql.Tx, taskID int64, eventType string, payload any, now any) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	uuidValue, err := newUUIDv7()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO asset_maintenance_events (uuid,maintenance_run_id,sequence,event_type,payload,created_at) SELECT ?,?,COALESCE(MAX(sequence),0)+1,?,?,? FROM asset_maintenance_events WHERE maintenance_run_id=?`, uuidValue, taskID, eventType, string(payloadJSON), now, taskID)
	return err
}
