package jobqueue

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"lumi/internal/agent"
	"lumi/internal/modelsettings"
	"lumi/internal/production"
	"lumi/internal/promptcatalog"
	"lumi/internal/story"
)

// The second request is frozen before accepting the batch. Only its target
// image UUID is filled in when the first task commits its result.
type premiseBatchSnapshot struct {
	Version            int                           `json:"version"`
	SourceUUID         string                        `json:"source_uuid"`
	RequestFingerprint string                        `json:"request_fingerprint"`
	Breakdown          production.GenerationSnapshot `json:"breakdown"`
}

func premiseBatchFingerprint(sourceUUID string, input CreateProductionGenerationInput, invocation agent.DomainInvocationContext) string {
	encoded, _ := json.Marshal(struct {
		Source         string
		Input          CreateProductionGenerationInput
		Invocation     agent.DomainInvocationContext
		ProviderUUID   string
		EnableThinking *bool
		PromptExtend   *bool
		References     []production.GenerationReferenceFile
	}{sourceUUID, input, invocation, input.ProviderUUID, input.EnableThinking, input.PromptExtend, input.ReferenceFiles})
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func replayPremiseBatch(ctx context.Context, runtime *projectRuntime, sourceUUID string, input CreateProductionGenerationInput, invocation agent.DomainInvocationContext) (ProductionTask, bool, error) {
	var raw, taskUUID string
	err := runtime.sqlDB.QueryRowContext(ctx, `SELECT w.input_snapshot,t.uuid FROM production_task_runs t
		JOIN workflow_steps s ON s.task_uuid=t.uuid JOIN workflows w ON w.id=s.workflow_id
		WHERE t.project_id=? AND t.kind=? AND t.idempotency_key=? AND w.kind=?`, runtime.projectID, KindPremiseSettingGeneration, input.IdempotencyKey, agent.WorkflowPremiseBatch).Scan(&raw, &taskUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return ProductionTask{}, false, nil
	}
	if err != nil {
		return ProductionTask{}, false, err
	}
	var snapshot premiseBatchSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return ProductionTask{}, false, err
	}
	if snapshot.Version != 1 || snapshot.RequestFingerprint != premiseBatchFingerprint(sourceUUID, input, invocation) {
		return ProductionTask{}, false, taskError(CodeTaskConflict, "批量设定幂等输入冲突", "调用归属、来源或生成参数与已接受的批次不一致。", nil)
	}
	task, err := runtime.manager.GetProductionTask(ctx, runtime.projectUUID, taskUUID)
	return task, true, err
}

func (manager *Manager) preparePremiseBatch(ctx context.Context, runtime *projectRuntime, source production.PremiseSource, input CreateProductionGenerationInput, setting production.GenerationSnapshot, invocation agent.DomainInvocationContext) (premiseBatchSnapshot, error) {
	result := premiseBatchSnapshot{Version: 1, SourceUUID: source.UUID, RequestFingerprint: premiseBatchFingerprint(source.UUID, input, invocation)}
	resolved, model, modelSource, err := manager.resolveProductionProvider(ctx, runtime.store, modelsettings.ProjectText, "", "", false, nil, nil)
	if err != nil {
		return result, err
	}
	template, err := story.NewService(runtime.store).EffectivePrompt(ctx, promptcatalog.GroupPremise, premiseAssetBreakdownPromptKey)
	if err != nil {
		return result, err
	}
	result.Breakdown = production.GenerationSnapshot{
		Version: 2, Kind: KindPremiseAssetBreakdown, ProjectUUID: runtime.projectUUID,
		GenerationLanguage: setting.GenerationLanguage, SourceUUID: source.UUID,
		SourceText: source.SourceText, StyleSnapshot: source.StyleSnapshot, Prompt: template,
		PromptTemplate: template, LanguageInstruction: setting.LanguageInstruction,
		ProviderUUID: resolved.UUID, ProviderType: resolved.ProviderType, ProviderBaseURL: resolved.BaseURL,
		Model: model, ModelSource: modelSource, Parameters: json.RawMessage(`{}`),
	}
	return result, nil
}

func createPremiseBatchTx(ctx context.Context, tx *sql.Tx, runtime *projectRuntime, taskUUID string, snapshot premiseBatchSnapshot, invocation agent.DomainInvocationContext, now time.Time) error {
	owner, err := loadInlineWorkflowOwnerTx(ctx, tx, runtime.projectID, invocation)
	if err != nil {
		return err
	}
	workflowUUID, err := newUUIDv7()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO workflows(uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at)
		VALUES(?,?,?, ?,?,'queued',1,?,?,?,?,?,?,?,?)`, workflowUUID, runtime.projectID, owner.ThreadID, agent.WorkflowPremiseBatch, agent.WorkflowPremiseBatch, string(encoded), "premise-batch:"+taskUUID, snapshot.Breakdown.ProviderUUID, snapshot.Breakdown.Model, snapshot.Breakdown.ModelSource, agent.WorkflowStepGenerateSetting, now, now)
	if err != nil {
		return err
	}
	workflowID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	for index, key := range []string{agent.WorkflowStepGenerateSetting, agent.WorkflowStepBreakdownAssets} {
		stepUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		status, linkedTask, resource := "pending", "", ""
		if index == 0 {
			status, linkedTask, resource = "queued", taskUUID, snapshot.SourceUUID
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_steps(uuid,workflow_id,step_key,position,status,idempotency_key,task_uuid,resource_uuid,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, stepUUID, workflowID, key, index+1, status, workflowUUID+":"+key, linkedTask, resource, now, now); err != nil {
			return err
		}
	}
	awaitUUID, err := newUUIDv7()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_awaits(uuid,workflow_id,chat_thread_id,chat_turn_id,chat_run_id,tool_execution_id,status,created_at,updated_at) VALUES(?,?,?,?,?,?,'waiting',?,?)`, awaitUUID, workflowID, owner.ThreadID, owner.TurnID, owner.RunID, owner.ToolExecutionID, now, now); err != nil {
		return err
	}
	return appendComicWorkflowEventTx(ctx, tx, workflowID, "workflow_queued", map[string]any{
		"project_uuid": runtime.projectUUID, "workflow_uuid": workflowUUID, "thread_uuid": owner.ThreadUUID,
		"turn_uuid": owner.TurnUUID, "run_uuid": owner.RunUUID, "tool_call_uuid": owner.ToolCallUUID,
		"origin_item_uuid": owner.ToolItemUUID, "task_uuid": taskUUID, "status": agent.WorkflowQueued,
	}, now)
}

// All production lifecycle transitions run in their caller's transaction.
// Advancing a setting task and enqueueing its breakdown must commit together.
func syncProductionTaskWorkflowsTx(ctx context.Context, runtime *projectRuntime, tx *sql.Tx, projectUUID, taskUUID string, now time.Time) (comicImageBatchWorkflowTransition, error) {
	transition, err := syncComicImageBatchWorkflowTx(ctx, runtime, tx, projectUUID, taskUUID, now)
	if err != nil {
		return transition, err
	}
	return transition, syncPremiseBatchWorkflowTx(ctx, runtime, tx, taskUUID, now)
}

func syncPremiseBatchWorkflowTx(ctx context.Context, runtime *projectRuntime, tx *sql.Tx, taskUUID string, now time.Time) error {
	var workflowID, stepID, threadID int64
	var workflowUUID, threadUUID, stepUUID, key, raw, oldStatus, taskStatus, code, message string
	var cancelled sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT w.id,w.uuid,w.thread_id,th.uuid,w.input_snapshot,w.status,w.cancel_requested_at,s.id,s.uuid,s.step_key,t.status,t.error_code,t.error_message
		FROM workflows w JOIN chat_threads th ON th.id=w.thread_id JOIN workflow_steps s ON s.workflow_id=w.id JOIN production_task_runs t ON t.uuid=s.task_uuid
		WHERE w.kind=? AND t.uuid=?`, agent.WorkflowPremiseBatch, taskUUID).Scan(&workflowID, &workflowUUID, &threadID, &threadUUID, &raw, &oldStatus, &cancelled, &stepID, &stepUUID, &key, &taskStatus, &code, &message)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// Chat abort and Workflow cancellation set this marker transactionally
	// before cancelling jobs. Explicit retries clear it without reviving chat.
	if cancelled.Valid {
		taskStatus, code, message = StatusCancelled, "cancelled", "用户已取消。"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status=?,error_code=?,error_message=?,started_at=(SELECT started_at FROM production_task_runs WHERE uuid=?),completed_at=CASE WHEN ? IN ('completed','failed','cancelled','interrupted') THEN COALESCE(completed_at,?) ELSE NULL END,updated_at=? WHERE id=?`, taskStatus, code, message, taskUUID, taskStatus, now, now, stepID); err != nil {
		return err
	}
	status, currentKey := taskStatus, key
	if taskStatus == StatusCompleted {
		var output string
		if err := tx.QueryRowContext(ctx, `SELECT output_json FROM premise_generation_steps WHERE task_uuid=? AND status='completed'`, taskUUID).Scan(&output); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET output_json=? WHERE id=?`, output, stepID); err != nil {
			return err
		}
		if key == agent.WorkflowStepGenerateSetting {
			var result struct {
				SettingUUID string `json:"setting_image_uuid"`
			}
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				return err
			}
			if !isUUIDv7(result.SettingUUID) {
				return taskError(CodeTaskPersistenceFailed, "设定总览图结果缺失", "无法确定本批次的拆分目标。", nil)
			}
			var snapshot premiseBatchSnapshot
			if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
				return err
			}
			if snapshot.Version != 1 {
				return taskError(CodeTaskPersistenceFailed, "批量设定快照无效", "无法恢复拆分输入。", nil)
			}
			snapshot.Breakdown.ResourceUUID = result.SettingUUID
			// This primitive reuses the same task on retry/replay.
			task, _, err := runtime.manager.insertProductionTaskTx(ctx, runtime, tx, snapshot.Breakdown, workflowUUID+":breakdown", func(tx *sql.Tx, _ int64, taskUUID string, encoded []byte, now time.Time) error {
				stepUUID, err := newUUIDv7()
				if err != nil {
					return err
				}
				_, err = tx.ExecContext(ctx, `INSERT INTO premise_generation_steps(uuid,project_id,task_uuid,source_id,setting_image_id,step_type,status,input_snapshot,created_at) VALUES(?,?,?,(SELECT id FROM premise_sources WHERE uuid=?),(SELECT id FROM premise_setting_images WHERE uuid=?),'asset_breakdown','queued',?,?)`, stepUUID, runtime.projectID, taskUUID, snapshot.SourceUUID, result.SettingUUID, string(encoded), now)
				return err
			}, now)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET task_uuid=?,resource_uuid=?,status=?,error_code='',error_message='',completed_at=NULL,updated_at=? WHERE workflow_id=? AND step_key=? AND task_uuid=''`, task.UUID, result.SettingUUID, task.Status, now, workflowID, agent.WorkflowStepBreakdownAssets); err != nil {
				return err
			}
			// A replay of a completed first task must not regress an already
			// finished second task or wake the chat with a partial result.
			if err := tx.QueryRowContext(ctx, `SELECT status,error_code,error_message FROM production_task_runs WHERE uuid=?`, task.UUID).Scan(&status, &code, &message); err != nil {
				return err
			}
			if status == StatusQueued {
				status = StatusRunning
			}
			currentKey = agent.WorkflowStepBreakdownAssets
		}
	}
	terminal := status == StatusCompleted || status == StatusFailed || status == StatusCancelled || status == StatusInterrupted
	if status == StatusCancelled {
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='cancelled',completed_at=COALESCE(completed_at,?),updated_at=? WHERE workflow_id=? AND status='pending'`, now, now, workflowID); err != nil {
			return err
		}
	}
	if status == StatusQueued && oldStatus != StatusQueued {
		// Explicit retry revives only unfinished, unstarted dependent steps.
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='pending',completed_at=NULL,updated_at=? WHERE workflow_id=? AND task_uuid=''`, now, workflowID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status=?,current_step_key=?,error_code=?,error_message=?,started_at=CASE WHEN ?='running' THEN COALESCE(started_at,?) ELSE started_at END,completed_at=CASE WHEN ? THEN COALESCE(completed_at,?) ELSE NULL END,updated_at=? WHERE id=?`, status, currentKey, code, message, status, now, terminal, now, now, workflowID); err != nil {
		return err
	}
	if _, err := agent.RecomputeThreadStatusTx(ctx, tx, threadID, now); err != nil {
		return err
	}
	event := "workflow_step_changed"
	if terminal && oldStatus != status {
		event = "workflow_" + status
	}
	if err := appendComicWorkflowEventTx(ctx, tx, workflowID, event, map[string]any{"project_uuid": runtime.projectUUID, "workflow_uuid": workflowUUID, "thread_uuid": threadUUID, "step_uuid": stepUUID, "step_key": key, "current_step_key": currentKey, "task_uuid": taskUUID, "status": status}, now); err != nil {
		return err
	}
	if terminal {
		return readyWorkflowAwaitsTx(ctx, runtime, tx, taskUUID, now)
	}
	return nil
}

func reconcilePremiseBatchWorkflows(ctx context.Context, db *sql.DB, projectID int64, now time.Time) error {
	// Every stage enqueue and terminal await wakeup is transactional. Recovery
	// only needs to project the task statuses reset by reconcileProductTasks;
	// River recovers the already persisted jobs.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status=(SELECT t.status FROM production_task_runs t WHERE t.uuid=workflow_steps.task_uuid),updated_at=? WHERE workflow_id IN (SELECT id FROM workflows WHERE project_id=? AND kind=? AND status IN ('queued','running')) AND task_uuid<>''`, now, projectID, agent.WorkflowPremiseBatch); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='queued',updated_at=? WHERE project_id=? AND kind=? AND status='running' AND EXISTS(SELECT 1 FROM workflow_steps s WHERE s.workflow_id=workflows.id AND s.status='queued')`, now, projectID, agent.WorkflowPremiseBatch); err != nil {
		return err
	}
	return tx.Commit()
}
