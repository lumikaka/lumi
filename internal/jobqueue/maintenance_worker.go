package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"lumi/internal/files"

	"github.com/riverqueue/river"
)

type assetMaintenanceWorker struct {
	river.WorkerDefaults[maintenanceArgs]
	runtime *projectRuntime
}

func (worker *assetMaintenanceWorker) Work(ctx context.Context, job *river.Job[maintenanceArgs]) error {
	if err := worker.runtime.store.RequireReady(); err != nil {
		return river.JobCancel(err)
	}
	runtime := worker.runtime
	workCtx, cancel := context.WithCancel(ctx)
	runtime.registerWork(job.Args.TaskUUID, cancel)
	defer func() { cancel(); runtime.unregisterWork(job.Args.TaskUUID) }()
	if job.Args.Version != 1 || job.Args.ProjectUUID != runtime.projectUUID || !validMaintenanceKind(job.Args.MaintenanceKind) || !isUUIDv7(job.Args.TaskUUID) || !isUUIDv7(job.Args.ResourceUUID) {
		return river.JobCancel(taskError(CodeInvalidTask, "Asset 维护 job 参数无效", "任务参数版本、kind 或 UUID 不受支持。", nil))
	}
	record, err := getMaintenanceRecord(workCtx, runtime.store.DB(), runtime.projectID, job.Args.TaskUUID)
	if err != nil {
		return err
	}
	if record.Status == StatusCompleted || record.Status == StatusCancelled {
		return nil
	}
	if err := runtime.startMaintenance(workCtx, record, job.Attempt); err != nil {
		return err
	}
	record.Attempt = job.Attempt
	var publisher files.EventPublisher
	if runtime.manager.hub != nil {
		publisher = runtime.manager.hub
	}
	service := files.NewService(runtime.store, publisher)
	resourceUUID := record.ResourceUUID
	switch record.Kind {
	case KindAssetReconcile:
		_, err = service.Reconcile(workCtx, 1000)
	case KindAssetUploadCleanup:
		_, err = service.CleanupUploads(workCtx, 1000, 0)
	case KindAssetIntegrityScan:
		var scan files.IntegrityScan
		scan, err = service.RunIntegrityScanWithProgress(workCtx, func(progress int) error {
			return runtime.progressMaintenance(workCtx, record, progress)
		})
		if err == nil {
			resourceUUID = scan.UUID
		}
	case KindAssetThumbnailRebuild:
		var assets []files.Asset
		assets, err = service.ListAssets(workCtx, files.AssetFilter{Kind: "image", Limit: 200})
		if err == nil {
			for index, asset := range assets {
				if workCtx.Err() != nil {
					err = workCtx.Err()
					break
				}
				if asset.Status != files.ObjectReady {
					continue
				}
				if _, thumbErr := service.EnsureThumbnail(workCtx, asset.UUID, "grid_256"); thumbErr != nil {
					err = thumbErr
					break
				}
				if _, thumbErr := service.EnsureThumbnail(workCtx, asset.UUID, "detail_1024"); thumbErr != nil {
					err = thumbErr
					break
				}
				_ = runtime.progressMaintenance(workCtx, record, (index+1)*90/maxInt(len(assets), 1))
			}
		}
	case KindAssetGCApply:
		var snapshot maintenanceSnapshot
		if unmarshalErr := json.Unmarshal([]byte(record.InputSnapshot), &snapshot); unmarshalErr != nil {
			err = unmarshalErr
		} else {
			_, err = service.GCApply(workCtx, snapshot.PlanUUID, time.Duration(snapshot.GraceHours)*time.Hour)
		}
	}
	if err != nil {
		_ = runtime.failMaintenance(context.WithoutCancel(ctx), record, err, job.Attempt)
		if errors.Is(err, context.Canceled) {
			return err
		}
		return err
	}
	return runtime.completeMaintenance(workCtx, record, resourceUUID)
}

func (runtime *projectRuntime) startMaintenance(ctx context.Context, record maintenanceRecord, attempt int) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE asset_maintenance_runs SET status='running',progress=1,attempt=?,started_at=COALESCE(started_at,?),updated_at=?,error_code='',error_message='' WHERE id=? AND cancel_requested_at IS NULL AND status NOT IN ('completed','cancelled')`, attempt, now, now, record.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return context.Canceled
	}
	if err := appendMaintenanceEventTx(ctx, tx, record.ID, "task_started", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "kind": record.Kind, "resource_uuid": record.ResourceUUID, "status": StatusRunning, "progress": 1, "attempt": attempt}, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status = StatusRunning
	task.Progress = 1
	task.Attempt = attempt
	task.StartedAt = &now
	task.UpdatedAt = now
	runtime.broadcastMaintenance("task:running", task)
	return nil
}
func (runtime *projectRuntime) progressMaintenance(ctx context.Context, record maintenanceRecord, progress int) error {
	if progress < 1 {
		progress = 1
	}
	if progress > 95 {
		progress = 95
	}
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE asset_maintenance_runs SET progress=?,updated_at=? WHERE id=? AND status='running' AND cancel_requested_at IS NULL`, progress, now, record.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return context.Canceled
	}
	if err := appendMaintenanceEventTx(ctx, tx, record.ID, "task_progress", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "kind": record.Kind, "resource_uuid": record.ResourceUUID, "status": StatusRunning, "progress": progress}, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status = StatusRunning
	task.Progress = progress
	task.UpdatedAt = now
	runtime.broadcastMaintenance("task:progress", task)
	return nil
}
func (runtime *projectRuntime) completeMaintenance(ctx context.Context, record maintenanceRecord, resourceUUID string) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE asset_maintenance_runs SET status='completed',resource_uuid=?,progress=100,completed_at=?,updated_at=?,error_code='',error_message='' WHERE id=? AND cancel_requested_at IS NULL AND status='running'`, resourceUUID, now, now, record.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return context.Canceled
	}
	if err := appendMaintenanceEventTx(ctx, tx, record.ID, "task_completed", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "kind": record.Kind, "resource_uuid": resourceUUID, "status": StatusCompleted, "progress": 100}, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status = StatusCompleted
	task.ResourceUUID = resourceUUID
	task.Progress = 100
	task.CompletedAt = &now
	task.UpdatedAt = now
	runtime.broadcastMaintenance("task:completed", task)
	return nil
}
func (runtime *projectRuntime) failMaintenance(ctx context.Context, record maintenanceRecord, cause error, attempt int) error {
	var persisted maintenanceRecord
	if err := runtime.store.DB().WithContext(ctx).Where("id = ?", record.ID).First(&persisted).Error; err != nil {
		return err
	}
	if persisted.Status == StatusCancelled {
		return nil
	}
	record = persisted
	code, message := "asset_maintenance_failed", "Asset 维护任务失败，可安全重试。"
	var domain *files.Error
	if errors.As(cause, &domain) {
		code, message = domain.Code, domain.Message
	}
	if errors.Is(cause, context.Canceled) {
		code, message = "cancelled", "Asset 维护任务已取消。"
	}
	now := runtime.manager.now().UTC()
	status, eventType := StatusFailed, "task_failed"
	var completedAt any = now
	if record.CancelRequestedAt != nil {
		status, eventType = StatusCancelled, "task_cancelled"
	} else if errors.Is(cause, context.Canceled) || attempt < record.MaxAttempts {
		// Keep the database uniqueness guard active while River owns a retry.
		// A shutdown cancellation is recoverable and must not become a product
		// cancellation unless the user explicitly requested it.
		status, eventType, completedAt = StatusQueued, "task_retrying", nil
	}
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE asset_maintenance_runs SET status=?,progress=0,attempt=?,error_code=?,error_message=?,completed_at=?,updated_at=? WHERE id=? AND status <> 'completed'`, status, attempt, code, message, completedAt, now, record.ID); err != nil {
		return err
	}
	if err := appendMaintenanceEventTx(ctx, tx, record.ID, eventType, map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "kind": record.Kind, "resource_uuid": record.ResourceUUID, "status": status, "progress": 0, "attempt": attempt, "error_code": code, "error_message": message}, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status = status
	task.Attempt = attempt
	task.ErrorCode = code
	task.ErrorMessage = message
	if completedAt != nil {
		task.CompletedAt = &now
	} else {
		task.CompletedAt = nil
	}
	task.UpdatedAt = now
	broadcastEvent := "task:" + status
	if status == StatusQueued {
		broadcastEvent = "task:retrying"
	}
	runtime.broadcastMaintenance(broadcastEvent, task)
	return nil
}
func (runtime *projectRuntime) broadcastMaintenance(event string, task MaintenanceTask) {
	if runtime.manager.hub == nil {
		return
	}
	defer func() { _ = recover() }()
	runtime.manager.hub.Broadcast("project:"+runtime.projectUUID, event, map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": task.UUID, "kind": task.Kind, "resource_uuid": task.ResourceUUID, "status": task.Status, "progress": task.Progress, "attempt": task.Attempt, "error_code": task.ErrorCode, "error_message": task.ErrorMessage})
}
func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
