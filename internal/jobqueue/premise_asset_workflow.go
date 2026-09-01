package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"lumi/internal/agent"
)

const premiseAssetWorkflowTitle = agent.WorkflowPremiseAsset

type premiseAssetWorkflowSnapshot struct {
	Version            int    `json:"version"`
	ProjectUUID        string `json:"project_uuid"`
	PremiseAssetUUID   string `json:"premise_asset_uuid,omitempty"`
	AssetOperation     string `json:"asset_operation"`
	AssetType          string `json:"asset_type"`
	AssetTitle         string `json:"asset_title"`
	ProductionTaskUUID string `json:"production_task_uuid"`
	ModelSource        string `json:"model_source"`
}

type premiseAssetWorkflowRef struct {
	ID           int64
	ThreadID     int64
	StepID       int64
	UUID         string
	ThreadUUID   string
	StepUUID     string
	ResourceUUID string
}

func createPremiseAssetWorkflowTx(ctx context.Context, tx *sql.Tx, projectID int64, projectUUID, resourceUUID, taskUUID, operation, assetType, assetTitle, providerUUID, model, modelSource string, now time.Time) error {
	threadUUID, err := newUUIDv7()
	if err != nil {
		return err
	}
	workflowUUID, err := newUUIDv7()
	if err != nil {
		return err
	}
	stepUUID, err := newUUIDv7()
	if err != nil {
		return err
	}
	threadResult, err := tx.ExecContext(ctx, `INSERT INTO chat_threads(uuid,project_id,title,status,thread_type,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,created_at,updated_at) VALUES(?,?,?,'busy','workflow',?,?,?,1,1,1,?,?)`, threadUUID, projectID, premiseAssetWorkflowTitle, providerUUID, model, modelSource, now, now)
	if err != nil {
		return err
	}
	threadID, err := threadResult.LastInsertId()
	if err != nil {
		return err
	}
	snapshot := premiseAssetWorkflowSnapshot{
		Version: 1, ProjectUUID: projectUUID, AssetOperation: operation, AssetType: assetType,
		AssetTitle: assetTitle, ProductionTaskUUID: taskUUID, ModelSource: modelSource,
	}
	if operation == "variant" {
		snapshot.PremiseAssetUUID = resourceUUID
	}
	encodedSnapshot, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	workflowResult, err := tx.ExecContext(ctx, `INSERT INTO workflows(uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES(?,?,?, ?,?,'queued',1,?,?,?,?,?,?,?,?)`, workflowUUID, projectID, threadID, agent.WorkflowPremiseAsset, premiseAssetWorkflowTitle, string(encodedSnapshot), "premise-asset:"+taskUUID, providerUUID, model, modelSource, agent.WorkflowStepGeneratePremiseAsset, now, now)
	if err != nil {
		return err
	}
	workflowID, err := workflowResult.LastInsertId()
	if err != nil {
		return err
	}
	stepInput, err := json.Marshal(map[string]any{
		"asset_operation": operation,
		"asset_type":      assetType,
		"asset_title":     assetTitle,
	})
	if err != nil {
		return err
	}
	stepResult, err := tx.ExecContext(ctx, `INSERT INTO workflow_steps(uuid,workflow_id,step_key,position,status,idempotency_key,task_uuid,resource_uuid,input_json,output_json,created_at,updated_at) VALUES(?,?,?,1,'queued',?,?,?,?,'{}',?,?)`, stepUUID, workflowID, agent.WorkflowStepGeneratePremiseAsset, workflowUUID+":"+agent.WorkflowStepGeneratePremiseAsset, taskUUID, resourceUUID, string(stepInput), now, now)
	if err != nil {
		return err
	}
	stepID, err := stepResult.LastInsertId()
	if err != nil {
		return err
	}
	return appendPremiseAssetWorkflowEventTx(ctx, tx, workflowID, &stepID, "workflow_queued", map[string]any{
		"project_uuid": projectUUID, "workflow_uuid": workflowUUID, "thread_uuid": threadUUID,
		"step_uuid": stepUUID, "task_uuid": taskUUID, "resource_uuid": resourceUUID,
		"status": agent.WorkflowQueued,
	}, now)
}

func premiseAssetWorkflowRefTx(ctx context.Context, tx *sql.Tx, taskUUID string) (premiseAssetWorkflowRef, bool, error) {
	var ref premiseAssetWorkflowRef
	err := tx.QueryRowContext(ctx, `SELECT w.id,w.thread_id,s.id,w.uuid,t.uuid,s.uuid,s.resource_uuid FROM workflows w JOIN chat_threads t ON t.id=w.thread_id JOIN workflow_steps s ON s.workflow_id=w.id WHERE w.kind=? AND s.step_key=? AND s.task_uuid=? LIMIT 1`, agent.WorkflowPremiseAsset, agent.WorkflowStepGeneratePremiseAsset, taskUUID).
		Scan(&ref.ID, &ref.ThreadID, &ref.StepID, &ref.UUID, &ref.ThreadUUID, &ref.StepUUID, &ref.ResourceUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return ref, false, nil
	}
	return ref, err == nil, err
}

func markPremiseAssetWorkflowRunningTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := premiseAssetWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='running',started_at=COALESCE(started_at,?),completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=? AND status<>'completed'`, now, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='running',current_step_key=?,started_at=COALESCE(started_at,?),completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, agent.WorkflowStepGeneratePremiseAsset, now, now, ref.ID); err != nil {
		return err
	}
	if _, err := agent.RecomputeThreadStatusTx(ctx, tx, ref.ThreadID, now); err != nil {
		return err
	}
	return appendPremiseAssetWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "step_running", map[string]any{
		"workflow_uuid": ref.UUID, "thread_uuid": ref.ThreadUUID, "step_uuid": ref.StepUUID,
		"task_uuid": taskUUID, "resource_uuid": ref.ResourceUUID, "status": agent.WorkflowRunning,
	}, now)
}

func completePremiseAssetWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := premiseAssetWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='completed',started_at=COALESCE(started_at,?),completed_at=COALESCE(completed_at,?),error_code='',error_message='',updated_at=? WHERE id=?`, now, now, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='completed',current_step_key=?,completed_at=?,error_code='',error_message='',updated_at=? WHERE id=?`, agent.WorkflowStepGeneratePremiseAsset, now, now, ref.ID); err != nil {
		return err
	}
	if _, err := agent.RecomputeThreadStatusTx(ctx, tx, ref.ThreadID, now); err != nil {
		return err
	}
	return appendPremiseAssetWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "workflow_completed", map[string]any{
		"workflow_uuid": ref.UUID, "thread_uuid": ref.ThreadUUID, "step_uuid": ref.StepUUID,
		"task_uuid": taskUUID, "resource_uuid": ref.ResourceUUID, "status": agent.WorkflowCompleted,
	}, now)
}

func failPremiseAssetWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID, code, message string, now time.Time) error {
	ref, found, err := premiseAssetWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='failed',completed_at=?,error_code=?,error_message=?,updated_at=? WHERE id=?`, now, code, message, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='failed',completed_at=?,error_code=?,error_message=?,updated_at=? WHERE id=?`, now, code, message, now, ref.ID); err != nil {
		return err
	}
	if _, err := agent.RecomputeThreadStatusTx(ctx, tx, ref.ThreadID, now); err != nil {
		return err
	}
	return appendPremiseAssetWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "workflow_failed", map[string]any{
		"workflow_uuid": ref.UUID, "thread_uuid": ref.ThreadUUID, "step_uuid": ref.StepUUID,
		"task_uuid": taskUUID, "resource_uuid": ref.ResourceUUID, "status": agent.WorkflowFailed,
		"error_code": code,
	}, now)
}

func cancelPremiseAssetWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := premiseAssetWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='cancelled',completed_at=COALESCE(completed_at,?),error_code='cancelled',error_message='用户已取消。',updated_at=? WHERE id=? AND status<>'completed'`, now, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='cancelled',cancel_requested_at=?,completed_at=?,error_code='cancelled',error_message='用户已取消。',updated_at=? WHERE id=?`, now, now, now, ref.ID); err != nil {
		return err
	}
	if _, err := agent.RecomputeThreadStatusTx(ctx, tx, ref.ThreadID, now); err != nil {
		return err
	}
	return appendPremiseAssetWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "workflow_cancelled", map[string]any{
		"workflow_uuid": ref.UUID, "thread_uuid": ref.ThreadUUID, "step_uuid": ref.StepUUID,
		"task_uuid": taskUUID, "resource_uuid": ref.ResourceUUID, "status": agent.WorkflowCancelled,
	}, now)
}

func queuePremiseAssetWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := premiseAssetWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='queued',completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=? AND status<>'completed'`, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='queued',current_step_key=?,cancel_requested_at=NULL,completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, agent.WorkflowStepGeneratePremiseAsset, now, ref.ID); err != nil {
		return err
	}
	if _, err := agent.RecomputeThreadStatusTx(ctx, tx, ref.ThreadID, now); err != nil {
		return err
	}
	return appendPremiseAssetWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "workflow_queued", map[string]any{
		"workflow_uuid": ref.UUID, "thread_uuid": ref.ThreadUUID, "step_uuid": ref.StepUUID,
		"task_uuid": taskUUID, "resource_uuid": ref.ResourceUUID, "status": agent.WorkflowQueued,
	}, now)
}

func interruptPremiseAssetWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID, code, message string, now time.Time) error {
	ref, found, err := premiseAssetWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='interrupted',completed_at=?,error_code=?,error_message=?,updated_at=? WHERE id=? AND status<>'completed'`, now, code, message, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='interrupted',completed_at=?,error_code=?,error_message=?,updated_at=? WHERE id=?`, now, code, message, now, ref.ID); err != nil {
		return err
	}
	_, err = agent.RecomputeThreadStatusTx(ctx, tx, ref.ThreadID, now)
	return err
}

func reconcilePremiseAssetWorkflows(ctx context.Context, db *sql.DB, projectID int64, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT tasks.uuid,tasks.status,tasks.error_code,tasks.error_message,w.status FROM production_task_runs tasks JOIN workflow_steps s ON s.task_uuid=tasks.uuid JOIN workflows w ON w.id=s.workflow_id WHERE tasks.project_id=? AND tasks.kind=? AND w.kind=?`, projectID, KindPremiseAssetGeneration, agent.WorkflowPremiseAsset)
	if err != nil {
		return err
	}
	type row struct{ taskUUID, taskStatus, errorCode, errorMessage, workflowStatus string }
	var items []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.taskUUID, &item.taskStatus, &item.errorCode, &item.errorMessage, &item.workflowStatus); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range items {
		switch item.taskStatus {
		case StatusQueued:
			if item.workflowStatus != agent.WorkflowQueued {
				if err := queuePremiseAssetWorkflowTx(ctx, tx, item.taskUUID, now); err != nil {
					return err
				}
			}
		case StatusRunning:
			if item.workflowStatus != agent.WorkflowRunning {
				if err := markPremiseAssetWorkflowRunningTx(ctx, tx, item.taskUUID, now); err != nil {
					return err
				}
			}
		case StatusCompleted:
			if item.workflowStatus != agent.WorkflowCompleted {
				if err := completePremiseAssetWorkflowTx(ctx, tx, item.taskUUID, now); err != nil {
					return err
				}
			}
		case StatusFailed:
			if item.workflowStatus != agent.WorkflowFailed {
				if err := failPremiseAssetWorkflowTx(ctx, tx, item.taskUUID, item.errorCode, item.errorMessage, now); err != nil {
					return err
				}
			}
		case StatusCancelled:
			if item.workflowStatus != agent.WorkflowCancelled {
				if err := cancelPremiseAssetWorkflowTx(ctx, tx, item.taskUUID, now); err != nil {
					return err
				}
			}
		case StatusInterrupted:
			if item.workflowStatus != agent.WorkflowInterrupted {
				if err := interruptPremiseAssetWorkflowTx(ctx, tx, item.taskUUID, item.errorCode, item.errorMessage, now); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}

func appendPremiseAssetWorkflowEventTx(ctx context.Context, tx *sql.Tx, workflowID int64, stepID *int64, eventType string, payload any, now time.Time) error {
	eventUUID, err := newUUIDv7()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO workflow_events(uuid,workflow_id,step_id,sequence,event_type,payload_json,created_at) SELECT ?,?,?,COALESCE(MAX(sequence),0)+1,?,?,? FROM workflow_events WHERE workflow_id=?`, eventUUID, workflowID, stepID, eventType, string(encoded), now, workflowID)
	return err
}
