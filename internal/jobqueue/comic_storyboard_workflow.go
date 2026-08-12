package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"lumi/internal/agent"
	"lumi/internal/realtime"
)

const comicStoryboardWorkflowTitle = agent.WorkflowComicStoryboard

type comicStoryboardWorkflowSnapshot struct {
	Version         int    `json:"version"`
	ProjectUUID     string `json:"project_uuid"`
	ChapterUUID     string `json:"chapter_uuid"`
	StoryTaskUUID   string `json:"story_task_uuid"`
	MaxSectionCount int    `json:"max_section_count"`
	ModelSource     string `json:"model_source"`
}

type comicStoryboardWorkflowRef struct {
	ID           int64
	ThreadID     int64
	StepID       int64
	UUID         string
	ThreadUUID   string
	StepUUID     string
	ResourceUUID string
	Status       string
	StepStatus   string
}

func createComicStoryboardWorkflowTx(ctx context.Context, tx *sql.Tx, projectID int64, projectUUID, chapterUUID, taskUUID, providerUUID, model, modelSource string, taskSnapshot []byte, now time.Time) error {
	var frozen storyGenerationSnapshot
	if err := json.Unmarshal(taskSnapshot, &frozen); err != nil {
		return err
	}
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
	threadResult, err := tx.ExecContext(ctx, `INSERT INTO chat_threads(uuid,project_id,title,status,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,scope,scene,subject_uuid,created_at,updated_at) VALUES(?,?,?,'busy',?,?,?,1,1,1,'project','',?,?,?)`, threadUUID, projectID, comicStoryboardWorkflowTitle, providerUUID, model, modelSource, chapterUUID, now, now)
	if err != nil {
		return err
	}
	threadID, err := threadResult.LastInsertId()
	if err != nil {
		return err
	}
	workflowSnapshot, err := json.Marshal(comicStoryboardWorkflowSnapshot{
		Version: 1, ProjectUUID: projectUUID, ChapterUUID: chapterUUID, StoryTaskUUID: taskUUID,
		MaxSectionCount: frozen.MaxSectionCount, ModelSource: modelSource,
	})
	if err != nil {
		return err
	}
	workflowResult, err := tx.ExecContext(ctx, `INSERT INTO workflows(uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES(?,?,?, ?,?,'queued',1,?,?,?,?,?,?,?,?)`, workflowUUID, projectID, threadID, agent.WorkflowComicStoryboard, comicStoryboardWorkflowTitle, string(workflowSnapshot), "comic-storyboard:"+taskUUID, providerUUID, model, modelSource, agent.WorkflowStepComicStoryboard, now, now)
	if err != nil {
		return err
	}
	workflowID, err := workflowResult.LastInsertId()
	if err != nil {
		return err
	}
	stepInput, err := json.Marshal(map[string]any{"chapter_uuid": chapterUUID, "max_section_count": frozen.MaxSectionCount})
	if err != nil {
		return err
	}
	stepResult, err := tx.ExecContext(ctx, `INSERT INTO workflow_steps(uuid,workflow_id,step_key,position,status,idempotency_key,task_uuid,resource_uuid,input_json,output_json,created_at,updated_at) VALUES(?,?,?,1,'queued',?,?,?,?,'{}',?,?)`, stepUUID, workflowID, agent.WorkflowStepComicStoryboard, workflowUUID+":"+agent.WorkflowStepComicStoryboard, taskUUID, chapterUUID, string(stepInput), now, now)
	if err != nil {
		return err
	}
	stepID, err := stepResult.LastInsertId()
	if err != nil {
		return err
	}
	return appendComicStoryboardWorkflowEventTx(ctx, tx, workflowID, &stepID, "workflow_queued", map[string]any{
		"project_uuid": projectUUID, "workflow_uuid": workflowUUID, "thread_uuid": threadUUID,
		"step_uuid": stepUUID, "task_uuid": taskUUID, "resource_uuid": chapterUUID, "status": agent.WorkflowQueued,
	}, now)
}

func comicStoryboardWorkflowRefTx(ctx context.Context, tx *sql.Tx, taskUUID string) (comicStoryboardWorkflowRef, bool, error) {
	var ref comicStoryboardWorkflowRef
	err := tx.QueryRowContext(ctx, `SELECT w.id,w.thread_id,s.id,w.uuid,t.uuid,s.uuid,s.resource_uuid,w.status,s.status FROM workflows w JOIN chat_threads t ON t.id=w.thread_id JOIN workflow_steps s ON s.workflow_id=w.id AND s.step_key=? WHERE w.kind=? AND s.task_uuid=? LIMIT 1`, agent.WorkflowStepComicStoryboard, agent.WorkflowComicStoryboard, taskUUID).Scan(&ref.ID, &ref.ThreadID, &ref.StepID, &ref.UUID, &ref.ThreadUUID, &ref.StepUUID, &ref.ResourceUUID, &ref.Status, &ref.StepStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return ref, false, nil
	}
	return ref, err == nil, err
}

func markComicStoryboardWorkflowRunningTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := comicStoryboardWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='running',started_at=COALESCE(started_at,?),completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, now, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='running',current_step_key=?,started_at=COALESCE(started_at,?),completed_at=NULL,cancel_requested_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, agent.WorkflowStepComicStoryboard, now, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='busy',updated_at=? WHERE id=?`, now, ref.ThreadID); err != nil {
		return err
	}
	if ref.Status == agent.WorkflowRunning && ref.StepStatus == agent.WorkflowRunning {
		return nil
	}
	return appendComicStoryboardWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "step_started", comicStoryboardWorkflowPayload(ref, taskUUID, agent.WorkflowRunning), now)
}

func completeComicStoryboardWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID string, output map[string]any, now time.Time) error {
	ref, found, err := comicStoryboardWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='completed',output_json=?,started_at=COALESCE(started_at,?),completed_at=COALESCE(completed_at,?),error_code='',error_message='',updated_at=? WHERE id=?`, string(encoded), now, now, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='completed',current_step_key=?,completed_at=COALESCE(completed_at,?),cancel_requested_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, agent.WorkflowStepComicStoryboard, now, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='completed',updated_at=? WHERE id=?`, now, ref.ThreadID); err != nil {
		return err
	}
	if ref.Status == agent.WorkflowCompleted && ref.StepStatus == agent.WorkflowCompleted {
		return nil
	}
	payload := comicStoryboardWorkflowPayload(ref, taskUUID, agent.WorkflowCompleted)
	if err := appendComicStoryboardWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "step_completed", payload, now); err != nil {
		return err
	}
	return appendComicStoryboardWorkflowEventTx(ctx, tx, ref.ID, nil, "workflow_completed", payload, now)
}

func failComicStoryboardWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID, code, message string, now time.Time) error {
	ref, found, err := comicStoryboardWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='failed',completed_at=?,error_code=?,error_message=?,updated_at=? WHERE id=?`, now, code, message, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='failed',completed_at=?,error_code=?,error_message=?,updated_at=? WHERE id=?`, now, code, message, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='failed',updated_at=? WHERE id=?`, now, ref.ThreadID); err != nil {
		return err
	}
	if ref.Status == agent.WorkflowFailed && ref.StepStatus == agent.WorkflowFailed {
		return nil
	}
	payload := comicStoryboardWorkflowPayload(ref, taskUUID, agent.WorkflowFailed)
	payload["error_code"] = code
	return appendComicStoryboardWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "step_failed", payload, now)
}

func cancelComicStoryboardWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := comicStoryboardWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='cancelled',completed_at=COALESCE(completed_at,?),error_code='cancelled',error_message='用户已取消。',updated_at=? WHERE id=? AND status<>'completed'`, now, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='cancelled',cancel_requested_at=?,completed_at=?,error_code='cancelled',error_message='用户已取消。',updated_at=? WHERE id=?`, now, now, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='cancelled',updated_at=? WHERE id=?`, now, ref.ThreadID); err != nil {
		return err
	}
	if ref.Status == agent.WorkflowCancelled {
		return nil
	}
	return appendComicStoryboardWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "workflow_cancelled", comicStoryboardWorkflowPayload(ref, taskUUID, agent.WorkflowCancelled), now)
}

func queueComicStoryboardWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := comicStoryboardWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='queued',started_at=NULL,completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='queued',current_step_key=?,cancel_requested_at=NULL,started_at=NULL,completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, agent.WorkflowStepComicStoryboard, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='busy',updated_at=? WHERE id=?`, now, ref.ThreadID); err != nil {
		return err
	}
	if ref.Status == agent.WorkflowQueued && ref.StepStatus == agent.WorkflowQueued {
		return nil
	}
	return appendComicStoryboardWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "workflow_retried", comicStoryboardWorkflowPayload(ref, taskUUID, agent.WorkflowQueued), now)
}

func interruptComicStoryboardWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID, code, message string, now time.Time) error {
	ref, found, err := comicStoryboardWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='interrupted',completed_at=?,error_code=?,error_message=?,updated_at=? WHERE id=?`, now, code, message, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='interrupted',completed_at=?,error_code=?,error_message=?,updated_at=? WHERE id=?`, now, code, message, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='interrupted',updated_at=? WHERE id=?`, now, ref.ThreadID); err != nil {
		return err
	}
	if ref.Status == agent.WorkflowInterrupted && ref.StepStatus == agent.WorkflowInterrupted {
		return nil
	}
	payload := comicStoryboardWorkflowPayload(ref, taskUUID, agent.WorkflowInterrupted)
	payload["error_code"] = code
	return appendComicStoryboardWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "workflow_interrupted", payload, now)
}

func reconcileComicStoryboardWorkflows(ctx context.Context, db *sql.DB, projectID int64, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT tasks.id,tasks.uuid,tasks.status,tasks.error_code,tasks.error_message,w.status,s.status,t.status FROM task_runs tasks JOIN workflow_steps s ON s.task_uuid=tasks.uuid JOIN workflows w ON w.id=s.workflow_id JOIN chat_threads t ON t.id=w.thread_id WHERE tasks.project_id=? AND tasks.kind=? AND w.kind=?`, projectID, KindComicStoryboardGeneration, agent.WorkflowComicStoryboard)
	if err != nil {
		return err
	}
	type reconciliationItem struct {
		taskID                                        int64
		taskUUID, taskStatus, errorCode, errorMessage string
		workflowStatus, stepStatus, threadStatus      string
	}
	var items []reconciliationItem
	for rows.Next() {
		var item reconciliationItem
		if err := rows.Scan(&item.taskID, &item.taskUUID, &item.taskStatus, &item.errorCode, &item.errorMessage, &item.workflowStatus, &item.stepStatus, &item.threadStatus); err != nil {
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
			if err := queueComicStoryboardWorkflowTx(ctx, tx, item.taskUUID, now); err != nil {
				return err
			}
		case StatusRunning:
			if err := markComicStoryboardWorkflowRunningTx(ctx, tx, item.taskUUID, now); err != nil {
				return err
			}
		case StatusCompleted:
			if item.workflowStatus != agent.WorkflowCompleted || item.stepStatus != agent.WorkflowCompleted || item.threadStatus != agent.ThreadCompleted {
				output, err := comicStoryboardTaskOutputTx(ctx, tx, item.taskID)
				if err != nil {
					return err
				}
				if err := completeComicStoryboardWorkflowTx(ctx, tx, item.taskUUID, output, now); err != nil {
					return err
				}
			}
		case StatusFailed:
			if item.workflowStatus != agent.WorkflowFailed || item.stepStatus != agent.WorkflowFailed || item.threadStatus != agent.ThreadFailed {
				if err := failComicStoryboardWorkflowTx(ctx, tx, item.taskUUID, item.errorCode, item.errorMessage, now); err != nil {
					return err
				}
			}
		case StatusCancelled:
			if item.workflowStatus != agent.WorkflowCancelled || item.stepStatus != agent.WorkflowCancelled || item.threadStatus != agent.ThreadCancelled {
				if err := cancelComicStoryboardWorkflowTx(ctx, tx, item.taskUUID, now); err != nil {
					return err
				}
			}
		case StatusInterrupted:
			if item.workflowStatus != agent.WorkflowInterrupted || item.stepStatus != agent.WorkflowInterrupted || item.threadStatus != agent.ThreadInterrupted {
				if err := interruptComicStoryboardWorkflowTx(ctx, tx, item.taskUUID, item.errorCode, item.errorMessage, now); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}

func comicStoryboardTaskOutputTx(ctx context.Context, tx *sql.Tx, taskID int64) (map[string]any, error) {
	var payload string
	err := tx.QueryRowContext(ctx, `SELECT payload FROM task_events WHERE task_run_id=? AND event_type='task_completed' ORDER BY sequence DESC LIMIT 1`, taskID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil || envelope.Result == nil {
		return map[string]any{}, nil
	}
	return envelope.Result, nil
}

func comicStoryboardWorkflowPayload(ref comicStoryboardWorkflowRef, taskUUID, status string) map[string]any {
	return map[string]any{
		"workflow_uuid": ref.UUID, "thread_uuid": ref.ThreadUUID, "step_uuid": ref.StepUUID,
		"task_uuid": taskUUID, "resource_uuid": ref.ResourceUUID, "status": status,
	}
}

func appendComicStoryboardWorkflowEventTx(ctx context.Context, tx *sql.Tx, workflowID int64, stepID *int64, eventType string, payload any, now time.Time) error {
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

func (runtime *projectRuntime) broadcastComicStoryboardWorkflow(event, taskUUID string) {
	if runtime.manager.hub == nil {
		return
	}
	var workflowUUID, threadUUID, stepUUID, resourceUUID, status string
	err := runtime.sqlDB.QueryRowContext(context.Background(), `SELECT w.uuid,t.uuid,s.uuid,s.resource_uuid,w.status FROM workflows w JOIN chat_threads t ON t.id=w.thread_id JOIN workflow_steps s ON s.workflow_id=w.id AND s.step_key=? WHERE w.kind=? AND s.task_uuid=? LIMIT 1`, agent.WorkflowStepComicStoryboard, agent.WorkflowComicStoryboard, taskUUID).Scan(&workflowUUID, &threadUUID, &stepUUID, &resourceUUID, &status)
	if err != nil {
		return
	}
	payload, ok := comicStoryboardRealtimePayload(runtime.projectUUID, workflowUUID, threadUUID, stepUUID, taskUUID, resourceUUID, status)
	if !ok {
		return
	}
	runtime.manager.hub.Broadcast(realtime.ProjectTopic(runtime.projectUUID), event, payload)
}

func comicStoryboardRealtimePayload(projectUUID, workflowUUID, threadUUID, stepUUID, taskUUID, resourceUUID, status string) (map[string]any, bool) {
	for _, value := range []string{projectUUID, workflowUUID, threadUUID, stepUUID, taskUUID, resourceUUID} {
		if !isUUIDv7(value) {
			return nil, false
		}
	}
	return map[string]any{
		"project_uuid": projectUUID, "workflow_uuid": workflowUUID, "thread_uuid": threadUUID,
		"step_uuid": stepUUID, "task_uuid": taskUUID, "resource_uuid": resourceUUID, "status": status,
	}, true
}
