package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"lumi/internal/production"
)

type CreateWebsiteInput struct {
	AllowMissingImages bool   `json:"allow_missing_images"`
	IdempotencyKey     string `json:"idempotency_key"`
}

func (manager *Manager) CreateWebsiteGeneration(ctx context.Context, projectUUID string, input CreateWebsiteInput) (ProductionTask, error) {
	return manager.createWebsiteGeneration(ctx, projectUUID, input, "", false)
}

// Plugin submissions never adopt an unrelated active desktop or plugin task.
// A supplied key identifies one invocation. With no key, the pending scope
// lives in the same transaction as the frozen snapshot and survives restarts.
func (manager *Manager) CreatePluginWebsiteGeneration(ctx context.Context, projectUUID, host string, allowMissing bool, key string) (ProductionTask, error) {
	prefix := websiteRequestScope(host, allowMissing)
	scope := ""
	if key == "" {
		scope = prefix
		generated, err := newUUIDv7()
		if err != nil {
			return ProductionTask{}, err
		}
		key = generated
	}
	return manager.createWebsiteGeneration(ctx, projectUUID, CreateWebsiteInput{AllowMissingImages: allowMissing, IdempotencyKey: prefix + key}, scope, true)
}

func (manager *Manager) createWebsiteGeneration(ctx context.Context, projectUUID string, input CreateWebsiteInput, scope string, plugin bool) (ProductionTask, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return ProductionTask{}, err
	}
	if err = runtime.store.RequireReady(); err != nil {
		return ProductionTask{}, err
	}
	lookup := func() (ProductionTask, bool, error) {
		if scope == "" {
			return manager.FindProductionTask(ctx, projectUUID, KindWebsiteGeneration, input.IdempotencyKey)
		}
		var taskUUID string
		err := runtime.sqlDB.QueryRowContext(ctx, `SELECT t.uuid FROM website_generations g JOIN production_task_runs t ON t.id=g.task_id WHERE g.project_id=? AND g.plugin_request_scope=? AND g.plugin_result_observed=0`, runtime.projectID, scope).Scan(&taskUUID)
		if errors.Is(err, sql.ErrNoRows) {
			return ProductionTask{}, false, nil
		}
		if err != nil {
			return ProductionTask{}, false, err
		}
		task, err := manager.GetProductionTask(ctx, projectUUID, taskUUID)
		return task, true, err
	}
	if plugin {
		if task, found, err := lookup(); found || err != nil {
			return task, err
		}
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
		details := "请补齐图片，或明确选择仅导出已有内容。"
		if plugin {
			details = fmt.Sprintf("共有 %d 页缺图。", readiness.MissingSectionCount) + details
		}
		return ProductionTask{}, &production.Error{Code: production.CodeExportIncomplete, Message: "作品仍有缺图", Details: details}
	}
	// Lost final reply: an argument-free replay can reuse only this host's
	// last successful task for the identical snapshot, still installed and
	// intact. A failed/newer task, edited book or damaged file forces a build.
	if scope != "" {
		uuid, err := service.ReplayableWebsite(ctx, scope, hash)
		if err != nil {
			return ProductionTask{}, err
		}
		if uuid != "" {
			return manager.GetProductionTask(ctx, projectUUID, uuid)
		}
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
		_, err = tx.ExecContext(ctx, `INSERT INTO website_generations(uuid,project_id,task_id,snapshot_json,snapshot_hash,created_at,plugin_request_scope) VALUES(?,?,?,?,?,?,NULLIF(?,''))`, uuid, runtime.projectID, taskID, string(encoded), hash, now, scope)
		return err
	})
	if plugin {
		if err != nil {
			// Another relay may have reserved our pending scope concurrently.
			if existing, found, lookupErr := lookup(); found || lookupErr != nil {
				return existing, lookupErr
			}
		}
		return task, err
	}
	var conflict *Error
	if errors.As(err, &conflict) && conflict.Code == CodeTaskConflict {
		var active productionTaskRecord
		if lookupErr := runtime.store.DB().WithContext(ctx).Where("project_id=? AND kind=? AND status IN ('queued','running')", runtime.projectID, KindWebsiteGeneration).First(&active).Error; lookupErr == nil {
			return active.DTO(), nil
		}
	}
	return task, err
}

// PluginWebsiteKey exposes the opaque caller key, not its storage namespace.
func PluginWebsiteKey(task ProductionTask, host string, allowMissing bool) string {
	return strings.TrimPrefix(task.IdempotencyKey, websiteRequestScope(host, allowMissing))
}

func websiteRequestScope(host string, allowMissing bool) string {
	return fmt.Sprintf("plugin:website:%s:%t:", host, allowMissing)
}
