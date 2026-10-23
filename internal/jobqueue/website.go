package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"lumi/internal/production"
)

type CreateWebsiteInput struct {
	AllowMissingImages bool   `json:"allow_missing_images"`
	IdempotencyKey     string `json:"idempotency_key"`
}

func (manager *Manager) CreateWebsiteGeneration(ctx context.Context, projectUUID string, input CreateWebsiteInput) (ProductionTask, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return ProductionTask{}, err
	}
	if err = runtime.store.RequireReady(); err != nil {
		return ProductionTask{}, err
	}
	service := production.NewService(runtime.store, manager.hub)
	snapshot, readiness, hash, err := service.BuildWebsiteSnapshot(ctx)
	if err != nil {
		return ProductionTask{}, err
	}
	if !readiness.CanExport {
		return ProductionTask{}, &production.Error{Code: production.CodeExportEmpty, Message: "没有可导出的网站内容", Details: "至少需要一张有效的正文图片。"}
	}
	if !readiness.Complete && !input.AllowMissingImages {
		return ProductionTask{}, &production.Error{Code: production.CodeExportIncomplete, Message: "作品仍有缺图", Details: "请补齐图片，或明确选择仅导出已有内容。"}
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return ProductionTask{}, err
	}
	generation := production.GenerationSnapshot{Version: 1, Kind: KindWebsiteGeneration, ProjectUUID: projectUUID, ResourceUUID: projectUUID, Parameters: encoded}
	task, err := manager.createProductionTask(ctx, runtime, generation, input.IdempotencyKey, func(tx *sql.Tx, taskID int64, taskUUID string, _ []byte, now time.Time) error {
		frozen, _, frozenHash, err := service.BuildWebsiteSnapshotTx(ctx, tx)
		if err != nil {
			return err
		}
		if hash != frozenHash {
			return &production.Error{Code: production.CodeExportChanged, Message: "网站内容已变化", Details: "请重新检查完整性并生成。"}
		}
		encoded, err := json.Marshal(frozen)
		if err != nil {
			return err
		}
		uuid, err := newUUIDv7()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO website_generations(uuid,project_id,task_id,snapshot_json,snapshot_hash,created_at) VALUES(?,?,?,?,?,?)`, uuid, runtime.projectID, taskID, string(encoded), hash, now)
		return err
	})
	var conflict *Error
	if errors.As(err, &conflict) && conflict.Code == CodeTaskConflict {
		var active productionTaskRecord
		if lookupErr := runtime.store.DB().WithContext(ctx).Where("project_id=? AND kind=? AND status IN ('queued','running')", runtime.projectID, KindWebsiteGeneration).First(&active).Error; lookupErr == nil {
			return active.DTO(), nil
		}
	}
	return task, err
}
