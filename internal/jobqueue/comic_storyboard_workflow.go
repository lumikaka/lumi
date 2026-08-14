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

type storyTaskWorkflowConfig struct {
	WorkflowKind      string
	StepKey           string
	Title             string
	IdempotencyPrefix string
}

type storyTaskWorkflowSnapshot struct {
	ProjectUUID        string   `json:"project_uuid"`
	TaskUUID           string   `json:"task_uuid"`
	ChapterUUID        string   `json:"chapter_uuid,omitempty"`
	ChapterCode        string   `json:"chapter_code,omitempty"`
	PromptKey          string   `json:"prompt_key,omitempty"`
	ChapterCount       int      `json:"chapter_count,omitempty"`
	TargetChapterCodes []string `json:"target_chapter_codes,omitempty"`
	MaxSectionCount    int      `json:"max_section_count,omitempty"`
	ModelSource        string   `json:"model_source"`
}

type comicStoryboardWorkflowSnapshot struct {
	Version         int    `json:"version"`
	ProjectUUID     string `json:"project_uuid"`
	ChapterUUID     string `json:"chapter_uuid"`
	StoryTaskUUID   string `json:"story_task_uuid"`
	MaxSectionCount int    `json:"max_section_count"`
	ModelSource     string `json:"model_source"`
}

type storyTaskWorkflowRef struct {
	ID           int64
	ThreadID     int64
	StepID       int64
	UUID         string
	ThreadUUID   string
	StepUUID     string
	ResourceUUID string
	WorkflowKind string
	StepKey      string
	Status       string
	StepStatus   string
}

func projectedStoryTaskWorkflowConfig(taskKind string) (storyTaskWorkflowConfig, bool) {
	switch taskKind {
	case KindStoryChapterGeneration:
		return storyTaskWorkflowConfig{WorkflowKind: agent.WorkflowStoryChapter, StepKey: agent.WorkflowStepStoryChapter, Title: agent.WorkflowStoryChapter, IdempotencyPrefix: "story-task:"}, true
	case KindStoryChapterBatchPlan:
		return storyTaskWorkflowConfig{WorkflowKind: agent.WorkflowStoryChapterBatchPlan, StepKey: agent.WorkflowStepChapterBatchPlan, Title: agent.WorkflowStoryChapterBatchPlan, IdempotencyPrefix: "story-task:"}, true
	case KindComicStoryboardGeneration:
		return storyTaskWorkflowConfig{WorkflowKind: agent.WorkflowComicStoryboard, StepKey: agent.WorkflowStepComicStoryboard, Title: agent.WorkflowComicStoryboard, IdempotencyPrefix: "comic-storyboard:"}, true
	default:
		return storyTaskWorkflowConfig{}, false
	}
}

func isProjectedStoryTaskWorkflow(taskKind string) bool {
	_, ok := projectedStoryTaskWorkflowConfig(taskKind)
	return ok
}

func createStoryTaskWorkflowTx(ctx context.Context, tx *sql.Tx, projectID int64, projectUUID, taskKind, resourceUUID, taskUUID, providerUUID, model, modelSource string, taskSnapshot []byte, now time.Time) error {
	config, ok := projectedStoryTaskWorkflowConfig(taskKind)
	if !ok {
		return nil
	}
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
	threadResult, err := tx.ExecContext(ctx, `INSERT INTO chat_threads(uuid,project_id,title,status,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,scope,scene,subject_uuid,created_at,updated_at) VALUES(?,?,?,'busy',?,?,?,1,1,1,'project','',?,?,?)`, threadUUID, projectID, config.Title, providerUUID, model, modelSource, resourceUUID, now, now)
	if err != nil {
		return err
	}
	threadID, err := threadResult.LastInsertId()
	if err != nil {
		return err
	}
	var workflowSnapshot []byte
	if taskKind == KindComicStoryboardGeneration {
		workflowSnapshot, err = json.Marshal(comicStoryboardWorkflowSnapshot{
			Version: 1, ProjectUUID: projectUUID, ChapterUUID: resourceUUID, StoryTaskUUID: taskUUID,
			MaxSectionCount: frozen.MaxSectionCount, ModelSource: modelSource,
		})
	} else {
		workflowSnapshot, err = json.Marshal(storyTaskWorkflowSnapshot{
			ProjectUUID: projectUUID, TaskUUID: taskUUID,
			ChapterUUID: frozen.ChapterUUID, ChapterCode: frozen.ChapterCode, PromptKey: frozen.PromptKey,
			ChapterCount: frozen.ChapterCount, TargetChapterCodes: frozen.TargetChapterCodes, ModelSource: modelSource,
		})
	}
	if err != nil {
		return err
	}
	workflowResult, err := tx.ExecContext(ctx, `INSERT INTO workflows(uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES(?,?,?, ?,?,'queued',1,?,?,?,?,?,?,?,?)`, workflowUUID, projectID, threadID, config.WorkflowKind, config.Title, string(workflowSnapshot), config.IdempotencyPrefix+taskUUID, providerUUID, model, modelSource, config.StepKey, now, now)
	if err != nil {
		return err
	}
	workflowID, err := workflowResult.LastInsertId()
	if err != nil {
		return err
	}
	stepInput := map[string]any{}
	if taskKind == KindComicStoryboardGeneration {
		stepInput["chapter_uuid"] = resourceUUID
		stepInput["max_section_count"] = frozen.MaxSectionCount
	} else {
		stepInput["kind"] = taskKind
		stepInput["resource_uuid"] = resourceUUID
	}
	if taskKind == KindStoryChapterGeneration {
		stepInput["chapter_uuid"] = frozen.ChapterUUID
		stepInput["chapter_code"] = frozen.ChapterCode
		stepInput["prompt_key"] = frozen.PromptKey
	} else if taskKind == KindStoryChapterBatchPlan {
		stepInput["chapter_count"] = frozen.ChapterCount
		stepInput["target_chapter_codes"] = frozen.TargetChapterCodes
	}
	encodedInput, err := json.Marshal(stepInput)
	if err != nil {
		return err
	}
	stepResult, err := tx.ExecContext(ctx, `INSERT INTO workflow_steps(uuid,workflow_id,step_key,position,status,idempotency_key,task_uuid,resource_uuid,input_json,output_json,created_at,updated_at) VALUES(?,?,?,1,'queued',?,?,?,?,'{}',?,?)`, stepUUID, workflowID, config.StepKey, workflowUUID+":"+config.StepKey, taskUUID, resourceUUID, string(encodedInput), now, now)
	if err != nil {
		return err
	}
	stepID, err := stepResult.LastInsertId()
	if err != nil {
		return err
	}
	return appendStoryTaskWorkflowEventTx(ctx, tx, workflowID, &stepID, "workflow_queued", map[string]any{
		"project_uuid": projectUUID, "workflow_uuid": workflowUUID, "thread_uuid": threadUUID,
		"step_uuid": stepUUID, "task_uuid": taskUUID, "resource_uuid": resourceUUID, "status": agent.WorkflowQueued,
	}, now)
}

func createComicStoryboardWorkflowTx(ctx context.Context, tx *sql.Tx, projectID int64, projectUUID, chapterUUID, taskUUID, providerUUID, model, modelSource string, taskSnapshot []byte, now time.Time) error {
	return createStoryTaskWorkflowTx(ctx, tx, projectID, projectUUID, KindComicStoryboardGeneration, chapterUUID, taskUUID, providerUUID, model, modelSource, taskSnapshot, now)
}

func storyTaskWorkflowRefTx(ctx context.Context, tx *sql.Tx, taskUUID string) (storyTaskWorkflowRef, bool, error) {
	var ref storyTaskWorkflowRef
	err := tx.QueryRowContext(ctx, `SELECT w.id,w.thread_id,s.id,w.uuid,t.uuid,s.uuid,s.resource_uuid,w.kind,s.step_key,w.status,s.status FROM workflows w JOIN chat_threads t ON t.id=w.thread_id JOIN workflow_steps s ON s.workflow_id=w.id WHERE s.task_uuid=? AND w.kind IN (?,?,?) LIMIT 1`, taskUUID, agent.WorkflowStoryChapter, agent.WorkflowStoryChapterBatchPlan, agent.WorkflowComicStoryboard).Scan(&ref.ID, &ref.ThreadID, &ref.StepID, &ref.UUID, &ref.ThreadUUID, &ref.StepUUID, &ref.ResourceUUID, &ref.WorkflowKind, &ref.StepKey, &ref.Status, &ref.StepStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return ref, false, nil
	}
	return ref, err == nil, err
}

func markStoryTaskWorkflowRunningTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := storyTaskWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='running',started_at=COALESCE(started_at,?),completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, now, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='running',current_step_key=?,started_at=COALESCE(started_at,?),completed_at=NULL,cancel_requested_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, ref.StepKey, now, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='busy',updated_at=? WHERE id=?`, now, ref.ThreadID); err != nil {
		return err
	}
	if ref.Status == agent.WorkflowRunning && ref.StepStatus == agent.WorkflowRunning {
		return nil
	}
	return appendStoryTaskWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "step_started", storyTaskWorkflowPayload(ref, taskUUID, agent.WorkflowRunning), now)
}

func markStoryTaskWorkflowWaitingTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := storyTaskWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='waiting',updated_at=? WHERE id=?`, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='running',current_step_key=?,updated_at=? WHERE id=?`, ref.StepKey, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='waiting_for_input',updated_at=? WHERE id=?`, now, ref.ThreadID); err != nil {
		return err
	}
	if ref.StepStatus == "waiting" {
		return nil
	}
	return appendStoryTaskWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "step_waiting_for_input", storyTaskWorkflowPayload(ref, taskUUID, StatusWaitingForInput), now)
}

func completeStoryTaskWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID string, output map[string]any, now time.Time) error {
	ref, found, err := storyTaskWorkflowRefTx(ctx, tx, taskUUID)
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
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='completed',current_step_key=?,completed_at=COALESCE(completed_at,?),cancel_requested_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, ref.StepKey, now, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='completed',updated_at=? WHERE id=?`, now, ref.ThreadID); err != nil {
		return err
	}
	if ref.Status == agent.WorkflowCompleted && ref.StepStatus == agent.WorkflowCompleted {
		return nil
	}
	payload := storyTaskWorkflowPayload(ref, taskUUID, agent.WorkflowCompleted)
	if err := appendStoryTaskWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "step_completed", payload, now); err != nil {
		return err
	}
	return appendStoryTaskWorkflowEventTx(ctx, tx, ref.ID, nil, "workflow_completed", payload, now)
}

func failStoryTaskWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID, code, message string, now time.Time) error {
	ref, found, err := storyTaskWorkflowRefTx(ctx, tx, taskUUID)
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
	payload := storyTaskWorkflowPayload(ref, taskUUID, agent.WorkflowFailed)
	payload["error_code"] = code
	return appendStoryTaskWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "step_failed", payload, now)
}

func cancelStoryTaskWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := storyTaskWorkflowRefTx(ctx, tx, taskUUID)
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
	return appendStoryTaskWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "workflow_cancelled", storyTaskWorkflowPayload(ref, taskUUID, agent.WorkflowCancelled), now)
}

func queueStoryTaskWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := storyTaskWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='queued',started_at=NULL,completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='queued',current_step_key=?,cancel_requested_at=NULL,started_at=NULL,completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, ref.StepKey, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='busy',updated_at=? WHERE id=?`, now, ref.ThreadID); err != nil {
		return err
	}
	if ref.Status == agent.WorkflowQueued && ref.StepStatus == agent.WorkflowQueued {
		return nil
	}
	return appendStoryTaskWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "workflow_retried", storyTaskWorkflowPayload(ref, taskUUID, agent.WorkflowQueued), now)
}

func interruptStoryTaskWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID, code, message string, now time.Time) error {
	ref, found, err := storyTaskWorkflowRefTx(ctx, tx, taskUUID)
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
	payload := storyTaskWorkflowPayload(ref, taskUUID, agent.WorkflowInterrupted)
	payload["error_code"] = code
	return appendStoryTaskWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "workflow_interrupted", payload, now)
}

func reconcileStoryTaskWorkflows(ctx context.Context, db *sql.DB, projectID int64, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT tasks.id,tasks.uuid,tasks.status,tasks.error_code,tasks.error_message,w.status,s.status,t.status FROM task_runs tasks JOIN workflow_steps s ON s.task_uuid=tasks.uuid JOIN workflows w ON w.id=s.workflow_id JOIN chat_threads t ON t.id=w.thread_id WHERE tasks.project_id=? AND tasks.kind=w.kind AND w.kind IN (?,?,?)`, projectID, agent.WorkflowStoryChapter, agent.WorkflowStoryChapterBatchPlan, agent.WorkflowComicStoryboard)
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
			if err := queueStoryTaskWorkflowTx(ctx, tx, item.taskUUID, now); err != nil {
				return err
			}
		case StatusRunning:
			if err := markStoryTaskWorkflowRunningTx(ctx, tx, item.taskUUID, now); err != nil {
				return err
			}
		case StatusWaitingForInput:
			if err := markStoryTaskWorkflowWaitingTx(ctx, tx, item.taskUUID, now); err != nil {
				return err
			}
		case StatusCompleted:
			if item.workflowStatus != agent.WorkflowCompleted || item.stepStatus != agent.WorkflowCompleted || item.threadStatus != agent.ThreadCompleted {
				output, err := storyTaskOutputTx(ctx, tx, item.taskID)
				if err != nil {
					return err
				}
				if err := completeStoryTaskWorkflowTx(ctx, tx, item.taskUUID, output, now); err != nil {
					return err
				}
			}
		case StatusFailed:
			if item.workflowStatus != agent.WorkflowFailed || item.stepStatus != agent.WorkflowFailed || item.threadStatus != agent.ThreadFailed {
				if err := failStoryTaskWorkflowTx(ctx, tx, item.taskUUID, item.errorCode, item.errorMessage, now); err != nil {
					return err
				}
			}
		case StatusCancelled:
			if item.workflowStatus != agent.WorkflowCancelled || item.stepStatus != agent.WorkflowCancelled || item.threadStatus != agent.ThreadCancelled {
				if err := cancelStoryTaskWorkflowTx(ctx, tx, item.taskUUID, now); err != nil {
					return err
				}
			}
		case StatusInterrupted:
			if item.workflowStatus != agent.WorkflowInterrupted || item.stepStatus != agent.WorkflowInterrupted || item.threadStatus != agent.ThreadInterrupted {
				if err := interruptStoryTaskWorkflowTx(ctx, tx, item.taskUUID, item.errorCode, item.errorMessage, now); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}

func reconcileComicStoryboardWorkflows(ctx context.Context, db *sql.DB, projectID int64, now time.Time) error {
	return reconcileStoryTaskWorkflows(ctx, db, projectID, now)
}

func storyTaskOutputTx(ctx context.Context, tx *sql.Tx, taskID int64) (map[string]any, error) {
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

func comicStoryboardTaskOutputTx(ctx context.Context, tx *sql.Tx, taskID int64) (map[string]any, error) {
	return storyTaskOutputTx(ctx, tx, taskID)
}

func storyTaskWorkflowPayload(ref storyTaskWorkflowRef, taskUUID, status string) map[string]any {
	return map[string]any{
		"workflow_uuid": ref.UUID, "thread_uuid": ref.ThreadUUID, "step_uuid": ref.StepUUID,
		"task_uuid": taskUUID, "resource_uuid": ref.ResourceUUID, "status": status,
	}
}

func appendStoryTaskWorkflowEventTx(ctx context.Context, tx *sql.Tx, workflowID int64, stepID *int64, eventType string, payload any, now time.Time) error {
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

func (runtime *projectRuntime) broadcastStoryTaskWorkflow(event, taskUUID string) {
	if runtime.manager.hub == nil {
		return
	}
	var workflowUUID, threadUUID, stepUUID, resourceUUID, status string
	var progress int
	err := runtime.sqlDB.QueryRowContext(context.Background(), `SELECT w.uuid,t.uuid,s.uuid,s.resource_uuid,w.status,tasks.progress FROM workflows w JOIN chat_threads t ON t.id=w.thread_id JOIN workflow_steps s ON s.workflow_id=w.id JOIN task_runs tasks ON tasks.project_id=w.project_id AND tasks.uuid=s.task_uuid WHERE s.task_uuid=? AND w.kind IN (?,?,?) LIMIT 1`, taskUUID, agent.WorkflowStoryChapter, agent.WorkflowStoryChapterBatchPlan, agent.WorkflowComicStoryboard).Scan(&workflowUUID, &threadUUID, &stepUUID, &resourceUUID, &status, &progress)
	if err != nil {
		return
	}
	payload, ok := storyTaskRealtimePayload(runtime.projectUUID, workflowUUID, threadUUID, stepUUID, taskUUID, resourceUUID, status, progress)
	if !ok {
		return
	}
	runtime.manager.hub.Broadcast(realtime.ProjectTopic(runtime.projectUUID), event, payload)
}

func (runtime *projectRuntime) broadcastComicStoryboardWorkflow(event, taskUUID string) {
	runtime.broadcastStoryTaskWorkflow(event, taskUUID)
}

func storyTaskRealtimePayload(projectUUID, workflowUUID, threadUUID, stepUUID, taskUUID, resourceUUID, status string, progress int) (map[string]any, bool) {
	for _, value := range []string{projectUUID, workflowUUID, threadUUID, stepUUID, taskUUID, resourceUUID} {
		if !isUUIDv7(value) {
			return nil, false
		}
	}
	if progress < 0 || progress > 100 {
		return nil, false
	}
	return map[string]any{
		"project_uuid": projectUUID, "workflow_uuid": workflowUUID, "thread_uuid": threadUUID,
		"step_uuid": stepUUID, "task_uuid": taskUUID, "resource_uuid": resourceUUID, "status": status, "progress": progress,
	}, true
}

func comicStoryboardRealtimePayload(projectUUID, workflowUUID, threadUUID, stepUUID, taskUUID, resourceUUID, status string) (map[string]any, bool) {
	payload, ok := storyTaskRealtimePayload(projectUUID, workflowUUID, threadUUID, stepUUID, taskUUID, resourceUUID, status, 0)
	delete(payload, "progress")
	return payload, ok
}
