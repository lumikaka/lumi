package jobqueue

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"lumi/internal/agent"
)

const comicImageBatchWorkflowTitle = agent.WorkflowComicImageBatch

type comicImageBatchWorkflowSection struct {
	UUID     string `json:"uuid"`
	Title    string `json:"title"`
	Position int    `json:"position"`
}

type comicImageBatchWorkflowSnapshot struct {
	Version               int                              `json:"version"`
	ProjectUUID           string                           `json:"project_uuid"`
	ChapterUUID           string                           `json:"chapter_uuid"`
	Sections              []comicImageBatchWorkflowSection `json:"sections"`
	RequestFingerprint    string                           `json:"request_fingerprint"`
	ProviderUUID          string                           `json:"provider_uuid"`
	Model                 string                           `json:"model"`
	ModelSource           string                           `json:"model_source"`
	SelectionProviderUUID string                           `json:"selection_provider_uuid,omitempty"`
	SelectionModel        string                           `json:"selection_model,omitempty"`
	InvocationSource      agent.InvocationSource           `json:"invocation_source"`
	PresentationMode      agent.PresentationMode           `json:"presentation_mode"`
	IdempotencyKey        string                           `json:"idempotency_key"`
}

type comicImageBatchRequestIdentity struct {
	ProjectUUID           string                 `json:"project_uuid"`
	ChapterUUID           string                 `json:"chapter_uuid"`
	SectionUUIDs          []string               `json:"section_uuids"`
	ProviderUUID          string                 `json:"provider_uuid,omitempty"`
	Model                 string                 `json:"model,omitempty"`
	SelectionProviderUUID string                 `json:"selection_provider_uuid,omitempty"`
	SelectionModel        string                 `json:"selection_model,omitempty"`
	InvocationSource      agent.InvocationSource `json:"invocation_source"`
	PresentationMode      agent.PresentationMode `json:"presentation_mode"`
	ThreadUUID            string                 `json:"thread_uuid,omitempty"`
	TurnUUID              string                 `json:"turn_uuid,omitempty"`
	RunUUID               string                 `json:"run_uuid,omitempty"`
	ToolExecutionUUID     string                 `json:"tool_execution_uuid,omitempty"`
}

func newComicImageBatchWorkflowSnapshot(projectUUID, chapterUUID, batchKey string, input CreateComicImageGenerationBatchInput, invocation agent.DomainInvocationContext, prepared []preparedComicImageGeneration) (comicImageBatchWorkflowSnapshot, error) {
	if len(prepared) > 0 && len(prepared) != len(input.SectionUUIDs) {
		return comicImageBatchWorkflowSnapshot{}, taskError(CodeInvalidTask, "批量图片 Workflow 快照无效", "冻结的 Section 数量与有序输入不一致。", nil)
	}
	identity := comicImageBatchRequestIdentity{
		ProjectUUID: projectUUID, ChapterUUID: chapterUUID, SectionUUIDs: append([]string(nil), input.SectionUUIDs...),
		ProviderUUID: strings.TrimSpace(input.ProviderUUID), Model: strings.TrimSpace(input.Model),
		SelectionProviderUUID: strings.TrimSpace(input.SelectionProviderUUID), SelectionModel: strings.TrimSpace(input.SelectionModel),
		InvocationSource: invocation.Source, PresentationMode: invocation.PresentationMode,
		ThreadUUID: invocation.ThreadUUID, TurnUUID: invocation.TurnUUID,
		RunUUID: invocation.RunUUID, ToolExecutionUUID: invocation.ToolExecutionUUID,
	}
	encodedIdentity, err := json.Marshal(identity)
	if err != nil {
		return comicImageBatchWorkflowSnapshot{}, err
	}
	digest := sha256.Sum256(encodedIdentity)
	snapshot := comicImageBatchWorkflowSnapshot{
		Version: 1, ProjectUUID: projectUUID, ChapterUUID: chapterUUID,
		RequestFingerprint: fmt.Sprintf("%x", digest[:]), InvocationSource: invocation.Source,
		PresentationMode: invocation.PresentationMode, IdempotencyKey: batchKey,
	}
	snapshot.Sections = make([]comicImageBatchWorkflowSection, len(input.SectionUUIDs))
	for index, sectionUUID := range input.SectionUUIDs {
		snapshot.Sections[index] = comicImageBatchWorkflowSection{UUID: sectionUUID, Position: index + 1}
	}
	if len(prepared) > 0 {
		snapshot.ProviderUUID = prepared[0].Snapshot.ProviderUUID
		snapshot.Model = prepared[0].Snapshot.Model
		snapshot.ModelSource = prepared[0].Snapshot.ModelSource
		snapshot.SelectionProviderUUID = prepared[0].Snapshot.SelectionProviderUUID
		snapshot.SelectionModel = prepared[0].Snapshot.SelectionModel
	}
	for index, item := range prepared {
		snapshot.Sections[index] = comicImageBatchWorkflowSection{
			UUID: item.Section.UUID, Title: item.Section.Title, Position: index + 1,
		}
	}
	return snapshot, nil
}

func comicImageBatchWorkflowKey(batchKey string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(batchKey)))
	return fmt.Sprintf("comic-image-batch-workflow:%x", digest[:])
}

func comicImageBatchStepKey(position int) string {
	return fmt.Sprintf("%s:%03d", agent.WorkflowStepGenerateSectionImage, position)
}

func loadComicImageBatchReplayTx(ctx context.Context, tx *sql.Tx, projectID int64, expected comicImageBatchWorkflowSnapshot) (string, []productionTaskRecord, bool, error) {
	var workflowID int64
	var workflowUUID, raw string
	err := tx.QueryRowContext(ctx, `SELECT id,uuid,input_snapshot FROM workflows WHERE project_id=? AND kind=? AND idempotency_key=?`, projectID, agent.WorkflowComicImageBatch, comicImageBatchWorkflowKey(expected.IdempotencyKey)).Scan(&workflowID, &workflowUUID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	var frozen comicImageBatchWorkflowSnapshot
	if json.Unmarshal([]byte(raw), &frozen) != nil || frozen.Version != 1 || frozen.RequestFingerprint == "" {
		return "", nil, false, taskError(CodeTaskPersistenceFailed, "批量图片 Workflow 快照损坏", "无法安全重放已有批次。", nil)
	}
	if frozen.RequestFingerprint != expected.RequestFingerprint {
		return "", nil, false, taskError(CodeTaskConflict, "批量图片幂等输入冲突", "相同幂等键已用于不同的有序 Section 输入、模型参数或调用归属。", nil)
	}
	rows, err := tx.QueryContext(ctx, `SELECT t.id,t.uuid,t.project_id,t.river_job_id,t.kind,t.resource_uuid,t.input_snapshot,t.status,t.idempotency_key,t.provider_uuid,t.model,t.model_source,t.progress,t.attempt,t.max_attempts,t.error_code,t.error_message,t.cancel_requested_at,t.started_at,t.completed_at,t.created_at,t.updated_at
		FROM workflow_steps s JOIN production_task_runs t ON t.uuid=s.task_uuid
		WHERE s.workflow_id=? ORDER BY s.position,s.id`, workflowID)
	if err != nil {
		return "", nil, false, err
	}
	defer rows.Close()
	records := make([]productionTaskRecord, 0, len(expected.Sections))
	for rows.Next() {
		var row productionTaskRecord
		if err := rows.Scan(&row.ID, &row.UUID, &row.ProjectID, &row.RiverJobID, &row.Kind, &row.ResourceUUID, &row.InputSnapshot, &row.Status, &row.IdempotencyKey, &row.ProviderUUID, &row.Model, &row.ModelSource, &row.Progress, &row.Attempt, &row.MaxAttempts, &row.ErrorCode, &row.ErrorMessage, &row.CancelRequestedAt, &row.StartedAt, &row.CompletedAt, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return "", nil, false, err
		}
		records = append(records, row)
	}
	if err := rows.Err(); err != nil {
		return "", nil, false, err
	}
	if len(records) != len(expected.Sections) {
		return "", nil, false, taskError(CodeTaskPersistenceFailed, "批量图片 Workflow 任务不完整", "已有 Workflow 的 Step 与冻结输入数量不一致。", nil)
	}
	for index, record := range records {
		if record.Kind != KindComicImageGeneration || record.ResourceUUID != expected.Sections[index].UUID {
			return "", nil, false, taskError(CodeTaskPersistenceFailed, "批量图片 Workflow 任务顺序损坏", "已有 Workflow 的任务与冻结 Section 顺序不一致。", nil)
		}
	}
	return workflowUUID, records, true, nil
}

func createComicImageBatchWorkflowTx(ctx context.Context, runtime *projectRuntime, tx *sql.Tx, snapshot comicImageBatchWorkflowSnapshot, tasks []productionTaskRecord, invocation agent.DomainInvocationContext, now time.Time) (string, error) {
	if len(tasks) == 0 || len(tasks) != len(snapshot.Sections) {
		return "", taskError(CodeInvalidTask, "批量图片 Workflow 任务无效", "Workflow 必须为每个 Section 关联一个 Production Task。", nil)
	}
	var threadID *int64
	var threadUUID string
	var inlineOwner inlineWorkflowOwner
	var err error
	switch invocation.PresentationMode {
	case agent.PresentationDedicatedThread:
		threadUUID, err = newUUIDv7()
		if err != nil {
			return "", err
		}
		threadResult, err := tx.ExecContext(ctx, `INSERT INTO chat_threads(uuid,project_id,title,status,thread_type,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,created_at,updated_at) VALUES(?,?,?,'busy','workflow',?,?,?,1,1,1,?,?)`, threadUUID, runtime.projectID, comicImageBatchWorkflowTitle, snapshot.ProviderUUID, snapshot.Model, snapshot.ModelSource, now, now)
		if err != nil {
			return "", err
		}
		id, err := threadResult.LastInsertId()
		if err != nil {
			return "", err
		}
		threadID = &id
	case agent.PresentationInline:
		inlineOwner, err = loadInlineWorkflowOwnerTx(ctx, tx, runtime.projectID, invocation)
		if err != nil {
			return "", err
		}
		threadID, threadUUID = &inlineOwner.ThreadID, inlineOwner.ThreadUUID
	case agent.PresentationNone:
		// The parent Workflow owns presentation.
	default:
		return "", taskError(CodeInvalidTask, "Workflow 展示模式无效", "内部 invocation presentation_mode 不受支持。", nil)
	}
	workflowUUID, err := newUUIDv7()
	if err != nil {
		return "", err
	}
	encodedSnapshot, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	workflowResult, err := tx.ExecContext(ctx, `INSERT INTO workflows(uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES(?,?,?, ?,?,'queued',1,?,?,?,?,?,?,?,?)`, workflowUUID, runtime.projectID, threadID, agent.WorkflowComicImageBatch, comicImageBatchWorkflowTitle, string(encodedSnapshot), comicImageBatchWorkflowKey(snapshot.IdempotencyKey), snapshot.ProviderUUID, snapshot.Model, snapshot.ModelSource, comicImageBatchStepKey(1), now, now)
	if err != nil {
		return "", err
	}
	workflowID, err := workflowResult.LastInsertId()
	if err != nil {
		return "", err
	}
	var firstStepID int64
	var firstStepUUID string
	for index, task := range tasks {
		section := snapshot.Sections[index]
		stepUUID, err := newUUIDv7()
		if err != nil {
			return "", err
		}
		stepKey := comicImageBatchStepKey(index + 1)
		inputJSON, err := json.Marshal(map[string]any{
			"section_uuid": section.UUID, "section_title": section.Title, "request_position": section.Position,
		})
		if err != nil {
			return "", err
		}
		stepResult, err := tx.ExecContext(ctx, `INSERT INTO workflow_steps(uuid,workflow_id,step_key,position,status,idempotency_key,task_uuid,resource_uuid,input_json,output_json,error_code,error_message,started_at,completed_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,'{}',?,?,?,?,?,?)`, stepUUID, workflowID, stepKey, index+1, task.Status, workflowUUID+":"+stepKey, task.UUID, section.UUID, string(inputJSON), task.ErrorCode, task.ErrorMessage, task.StartedAt, task.CompletedAt, now, now)
		if err != nil {
			return "", err
		}
		if index == 0 {
			firstStepID, err = stepResult.LastInsertId()
			if err != nil {
				return "", err
			}
			firstStepUUID = stepUUID
		}
	}
	payload := map[string]any{
		"project_uuid": snapshot.ProjectUUID, "workflow_uuid": workflowUUID,
		"step_uuid": firstStepUUID, "task_uuid": tasks[0].UUID,
		"resource_uuid": tasks[0].ResourceUUID, "status": agent.WorkflowQueued,
	}
	if threadUUID != "" {
		payload["thread_uuid"] = threadUUID
	}
	if invocation.PresentationMode == agent.PresentationInline {
		awaitUUID, err := newUUIDv7()
		if err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_awaits(uuid,workflow_id,chat_thread_id,chat_turn_id,chat_run_id,tool_execution_id,status,created_at,updated_at) VALUES(?,?,?,?,?,?,'waiting',?,?)`, awaitUUID, workflowID, inlineOwner.ThreadID, inlineOwner.TurnID, inlineOwner.RunID, inlineOwner.ToolExecutionID, now, now); err != nil {
			return "", err
		}
		payload["turn_uuid"], payload["run_uuid"] = inlineOwner.TurnUUID, inlineOwner.RunUUID
		payload["tool_call_uuid"], payload["origin_item_uuid"] = inlineOwner.ToolCallUUID, inlineOwner.ToolItemUUID
	}
	if err := appendComicImageBatchWorkflowEventTx(ctx, tx, workflowID, &firstStepID, "workflow_queued", payload, now); err != nil {
		return "", err
	}
	for _, task := range tasks {
		if task.Status != StatusQueued {
			if _, err := syncComicImageBatchWorkflowTx(ctx, runtime, tx, snapshot.ProjectUUID, task.UUID, now); err != nil {
				return "", err
			}
			break
		}
	}
	return workflowUUID, nil
}

type comicImageBatchWorkflowTransition struct {
	Found        bool
	WorkflowUUID string
	ThreadUUID   string
	StepUUID     string
	ResourceUUID string
	Status       string
	Terminal     bool
}

type comicImageBatchTaskState struct {
	StepID                                                int64
	StepUUID, StepKey, TaskUUID, ResourceUUID, TaskStatus string
	Position, Progress                                    int
	ErrorCode, ErrorMessage                               string
	StartedAt, CompletedAt                                *time.Time
}

func syncComicImageBatchWorkflowTx(ctx context.Context, runtime *projectRuntime, tx *sql.Tx, projectUUID, taskUUID string, now time.Time) (comicImageBatchWorkflowTransition, error) {
	var transition comicImageBatchWorkflowTransition
	var workflowID, threadID int64
	var oldStatus string
	err := tx.QueryRowContext(ctx, `SELECT w.id,COALESCE(w.thread_id,0),w.uuid,COALESCE(th.uuid,''),s.uuid,s.resource_uuid,w.status
		FROM workflows w LEFT JOIN chat_threads th ON th.id=w.thread_id JOIN workflow_steps s ON s.workflow_id=w.id
		WHERE w.kind=? AND s.task_uuid=? LIMIT 1`, agent.WorkflowComicImageBatch, taskUUID).Scan(&workflowID, &threadID, &transition.WorkflowUUID, &transition.ThreadUUID, &transition.StepUUID, &transition.ResourceUUID, &oldStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return transition, nil
	}
	if err != nil {
		return transition, err
	}
	transition.Found = true
	rows, err := tx.QueryContext(ctx, `SELECT s.id,s.uuid,s.step_key,s.position,t.uuid,t.resource_uuid,t.status,t.progress,t.error_code,t.error_message,t.started_at,t.completed_at
		FROM workflow_steps s JOIN production_task_runs t ON t.uuid=s.task_uuid
		WHERE s.workflow_id=? ORDER BY s.position,s.id`, workflowID)
	if err != nil {
		return transition, err
	}
	var states []comicImageBatchTaskState
	for rows.Next() {
		var state comicImageBatchTaskState
		if err := rows.Scan(&state.StepID, &state.StepUUID, &state.StepKey, &state.Position, &state.TaskUUID, &state.ResourceUUID, &state.TaskStatus, &state.Progress, &state.ErrorCode, &state.ErrorMessage, &state.StartedAt, &state.CompletedAt); err != nil {
			rows.Close()
			return transition, err
		}
		states = append(states, state)
	}
	if err := rows.Close(); err != nil {
		return transition, err
	}
	if len(states) == 0 {
		return transition, taskError(CodeTaskPersistenceFailed, "批量图片 Workflow 没有步骤", "无法聚合空 Workflow。", nil)
	}
	for _, state := range states {
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status=?,error_code=?,error_message=?,started_at=?,completed_at=?,updated_at=? WHERE id=?`, state.TaskStatus, state.ErrorCode, state.ErrorMessage, state.StartedAt, state.CompletedAt, now, state.StepID); err != nil {
			return transition, err
		}
	}
	status, currentStep, errorCode, errorMessage, terminal := aggregateComicImageBatchState(states)
	transition.Status, transition.Terminal = status, terminal
	startedAt := any(nil)
	if status == agent.WorkflowRunning {
		startedAt = now
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status=?,current_step_key=?,started_at=CASE WHEN ? IS NULL THEN started_at ELSE COALESCE(started_at,?) END,completed_at=CASE WHEN ? THEN COALESCE(completed_at,?) ELSE NULL END,error_code=?,error_message=?,updated_at=? WHERE id=?`, status, currentStep, startedAt, startedAt, terminal, now, errorCode, errorMessage, now, workflowID); err != nil {
		return transition, err
	}
	if threadID > 0 {
		if _, err := agent.RecomputeThreadStatusTx(ctx, tx, threadID, now); err != nil {
			return transition, err
		}
	}
	var changed comicImageBatchTaskState
	for _, state := range states {
		if state.TaskUUID == taskUUID {
			changed = state
			break
		}
	}
	payload := map[string]any{
		"project_uuid": projectUUID, "workflow_uuid": transition.WorkflowUUID,
		"step_uuid": changed.StepUUID, "task_uuid": changed.TaskUUID,
		"resource_uuid": changed.ResourceUUID, "status": changed.TaskStatus, "progress": changed.Progress,
	}
	if transition.ThreadUUID != "" {
		payload["thread_uuid"] = transition.ThreadUUID
	}
	if err := appendComicImageBatchWorkflowEventTx(ctx, tx, workflowID, &changed.StepID, "workflow_step_changed", payload, now); err != nil {
		return transition, err
	}
	if terminal && oldStatus != status {
		terminalPayload := map[string]any{
			"project_uuid": projectUUID, "workflow_uuid": transition.WorkflowUUID,
			"step_uuid": changed.StepUUID, "task_uuid": changed.TaskUUID,
			"resource_uuid": changed.ResourceUUID, "status": status, "progress": changed.Progress,
		}
		if transition.ThreadUUID != "" {
			terminalPayload["thread_uuid"] = transition.ThreadUUID
		}
		if errorCode != "" {
			terminalPayload["error_code"] = errorCode
		}
		if err := appendComicImageBatchWorkflowEventTx(ctx, tx, workflowID, &changed.StepID, "workflow_"+status, terminalPayload, now); err != nil {
			return transition, err
		}
	}
	if terminal && runtime != nil {
		if err := readyWorkflowAwaitsTx(ctx, runtime, tx, taskUUID, now); err != nil {
			return transition, err
		}
	}
	return transition, nil
}

func aggregateComicImageBatchState(states []comicImageBatchTaskState) (status, currentStep, errorCode, errorMessage string, terminal bool) {
	active := false
	running := false
	anyTerminal := false
	for _, state := range states {
		switch state.TaskStatus {
		case StatusQueued:
			active = true
		case StatusRunning:
			active, running = true, true
		default:
			anyTerminal = true
		}
	}
	if active {
		status = agent.WorkflowQueued
		if running || anyTerminal {
			status = agent.WorkflowRunning
		}
		for _, state := range states {
			if state.TaskStatus == StatusQueued || state.TaskStatus == StatusRunning {
				return status, state.StepKey, "", "", false
			}
		}
	}
	for _, target := range []string{StatusFailed, StatusInterrupted, StatusCancelled} {
		for _, state := range states {
			if state.TaskStatus == target {
				code := strings.TrimSpace(state.ErrorCode)
				if code == "" {
					code = target
				}
				return target, state.StepKey, code, state.ErrorMessage, true
			}
		}
	}
	return agent.WorkflowCompleted, states[len(states)-1].StepKey, "", "", true
}

func appendComicImageBatchWorkflowEventTx(ctx context.Context, tx *sql.Tx, workflowID int64, stepID *int64, eventType string, payload any, now time.Time) error {
	eventUUID, err := newUUIDv7()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var sequence int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM workflow_events WHERE workflow_id=?`, workflowID).Scan(&sequence); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO workflow_events(uuid,workflow_id,step_id,sequence,event_type,payload_json,created_at) VALUES(?,?,?,?,?,?,?)`, eventUUID, workflowID, stepID, sequence, eventType, string(encoded), now)
	return err
}

func reconcileComicImageBatchWorkflows(ctx context.Context, db *sql.DB, projectID int64, projectUUID string, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	type cancellationTarget struct {
		ID                 int64
		UUID, ResourceUUID string
	}
	cancelRows, err := tx.QueryContext(ctx, `SELECT DISTINCT tasks.id,tasks.uuid,tasks.resource_uuid
		FROM workflows w
		JOIN workflow_steps s ON s.workflow_id=w.id
		JOIN production_task_runs tasks ON tasks.uuid=s.task_uuid
		WHERE w.project_id=? AND w.kind=? AND w.cancel_requested_at IS NOT NULL
		  AND tasks.status IN ('queued','running')
		ORDER BY s.position,s.id`, projectID, agent.WorkflowComicImageBatch)
	if err != nil {
		return err
	}
	var cancellations []cancellationTarget
	for cancelRows.Next() {
		var target cancellationTarget
		if err := cancelRows.Scan(&target.ID, &target.UUID, &target.ResourceUUID); err != nil {
			cancelRows.Close()
			return err
		}
		cancellations = append(cancellations, target)
	}
	if err := cancelRows.Close(); err != nil {
		return err
	}
	for _, target := range cancellations {
		if _, err := tx.ExecContext(ctx, `UPDATE production_task_runs SET status='cancelled',cancel_requested_at=COALESCE(cancel_requested_at,?),completed_at=COALESCE(completed_at,?),updated_at=? WHERE id=? AND status IN ('queued','running')`, now, now, now, target.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE comic_image_generations SET status='cancelled',completed_at=COALESCE(completed_at,?) WHERE task_uuid=? AND status<>'completed'`, now, target.UUID); err != nil {
			return err
		}
		if err := appendProductionEventTx(ctx, tx, target.ID, "task_cancelled", map[string]any{
			"project_uuid": projectUUID, "task_uuid": target.UUID, "resource_uuid": target.ResourceUUID, "status": StatusCancelled,
		}, now); err != nil {
			return err
		}
	}
	type workflowTarget struct {
		ID                                                     int64
		TaskUUID, Status, CurrentStep, ErrorCode, ErrorMessage string
		StartedAt, CompletedAt                                 *time.Time
	}
	rows, err := tx.QueryContext(ctx, `SELECT w.id,COALESCE(MIN(s.task_uuid),''),w.status,w.current_step_key,w.error_code,w.error_message,w.started_at,w.completed_at
		FROM workflows w LEFT JOIN workflow_steps s ON s.workflow_id=w.id
		WHERE w.project_id=? AND w.kind=?
		GROUP BY w.id,w.status,w.current_step_key,w.error_code,w.error_message,w.started_at,w.completed_at
		ORDER BY w.id`, projectID, agent.WorkflowComicImageBatch)
	if err != nil {
		return err
	}
	var targets []workflowTarget
	for rows.Next() {
		var target workflowTarget
		if err := rows.Scan(&target.ID, &target.TaskUUID, &target.Status, &target.CurrentStep, &target.ErrorCode, &target.ErrorMessage, &target.StartedAt, &target.CompletedAt); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, target)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, target := range targets {
		if target.TaskUUID == "" {
			return taskError(CodeTaskPersistenceFailed, "批量图片 Workflow 没有任务", "无法修复缺少 task_uuid 的批次 Workflow。", nil)
		}
		needsRepair, err := comicImageBatchWorkflowNeedsRepairTx(ctx, tx, target.ID, target.Status, target.CurrentStep, target.ErrorCode, target.ErrorMessage, target.StartedAt, target.CompletedAt)
		if err != nil {
			return err
		}
		if !needsRepair {
			continue
		}
		if _, err := syncComicImageBatchWorkflowTx(ctx, nil, tx, projectUUID, target.TaskUUID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func comicImageBatchWorkflowNeedsRepairTx(ctx context.Context, tx *sql.Tx, workflowID int64, workflowStatus, currentStep, workflowErrorCode, workflowErrorMessage string, workflowStartedAt, workflowCompletedAt *time.Time) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT s.step_key,s.position,s.status,s.error_code,s.error_message,s.started_at,s.completed_at,
		t.uuid,t.resource_uuid,t.status,t.progress,t.error_code,t.error_message,t.started_at,t.completed_at
		FROM workflow_steps s JOIN production_task_runs t ON t.uuid=s.task_uuid
		WHERE s.workflow_id=? ORDER BY s.position,s.id`, workflowID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	var states []comicImageBatchTaskState
	needsRepair := false
	for rows.Next() {
		var state comicImageBatchTaskState
		var stepStatus, stepErrorCode, stepErrorMessage string
		var stepStartedAt, stepCompletedAt *time.Time
		if err := rows.Scan(&state.StepKey, &state.Position, &stepStatus, &stepErrorCode, &stepErrorMessage, &stepStartedAt, &stepCompletedAt,
			&state.TaskUUID, &state.ResourceUUID, &state.TaskStatus, &state.Progress, &state.ErrorCode, &state.ErrorMessage, &state.StartedAt, &state.CompletedAt); err != nil {
			return false, err
		}
		if stepStatus != state.TaskStatus || stepErrorCode != state.ErrorCode || stepErrorMessage != state.ErrorMessage || !sameOptionalTime(stepStartedAt, state.StartedAt) || !sameOptionalTime(stepCompletedAt, state.CompletedAt) {
			needsRepair = true
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if len(states) == 0 {
		return true, nil
	}
	status, expectedCurrentStep, errorCode, errorMessage, terminal := aggregateComicImageBatchState(states)
	if workflowStatus != status || currentStep != expectedCurrentStep || workflowErrorCode != errorCode || workflowErrorMessage != errorMessage {
		needsRepair = true
	}
	if (terminal && workflowCompletedAt == nil) || (!terminal && workflowCompletedAt != nil) || (status == agent.WorkflowRunning && workflowStartedAt == nil) {
		needsRepair = true
	}
	return needsRepair, nil
}

func sameOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
