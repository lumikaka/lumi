package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"lumi/internal/production"
)

const comicStoryboardOverwriteActionRequired = "confirm_comic_storyboard_overwrite"

type comicStoryboardOverwriteRequest struct {
	ActionRequired             string `json:"action_required"`
	ExistingSectionCount       int    `json:"existing_section_count"`
	GeneratedSectionCount      int    `json:"generated_section_count"`
	ExpectedComicStateRevision int64  `json:"expected_comic_state_revision"`
}

func (runtime *projectRuntime) waitForComicStoryboardOverwrite(ctx context.Context, record taskRecord, conflict *production.GeneratedSectionsConflict, attempt int) error {
	request := comicStoryboardOverwriteRequest{
		ActionRequired:             comicStoryboardOverwriteActionRequired,
		ExistingSectionCount:       conflict.ExistingCount,
		GeneratedSectionCount:      conflict.GeneratedCount,
		ExpectedComicStateRevision: conflict.ComicStateRevision,
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return err
	}
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ref, found, err := storyTaskWorkflowRefTx(ctx, tx, record.UUID)
	if err != nil {
		return err
	}
	if !found || ref.WorkflowKind != KindComicStoryboardGeneration {
		return fmt.Errorf("comic storyboard workflow projection not found")
	}
	now := runtime.manager.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE task_runs SET status=?,progress=95,attempt=?,error_code='',error_message='',completed_at=NULL,updated_at=? WHERE id=? AND status='running' AND cancel_requested_at IS NULL`, StatusWaitingForInput, attempt, now, record.ID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return fmt.Errorf("comic storyboard task is not running")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET output_json=?,updated_at=? WHERE id=?`, string(encoded), now, ref.StepID); err != nil {
		return err
	}
	payload := map[string]any{
		"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "chapter_uuid": record.ResourceUUID,
		"status": StatusWaitingForInput, "progress": 95, "attempt": attempt,
		"action_required": request.ActionRequired, "existing_section_count": request.ExistingSectionCount,
		"generated_section_count": request.GeneratedSectionCount, "expected_comic_state_revision": request.ExpectedComicStateRevision,
	}
	if err := appendTaskEventTx(ctx, tx, record.ID, "task_waiting_for_input", payload, now); err != nil {
		return err
	}
	var agentThreadID, agentRunID int64
	if err := tx.QueryRowContext(ctx, `SELECT agent_thread_id,id FROM agent_runs WHERE task_run_id=?`, record.ID).Scan(&agentThreadID, &agentRunID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_runs SET status=?,updated_at=?,error_code='',error_message='' WHERE id=?`, StatusWaitingForInput, now, agentRunID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_threads SET status=?,updated_at=? WHERE id=?`, StatusWaitingForInput, now, agentThreadID); err != nil {
		return err
	}
	if err := appendAgentEventTx(ctx, tx, agentThreadID, &agentRunID, "run_waiting_for_input", payload, now); err != nil {
		return err
	}
	if err := markStoryTaskWorkflowWaitingTx(ctx, tx, record.UUID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status, task.Progress, task.Attempt, task.ErrorCode, task.ErrorMessage, task.CompletedAt, task.UpdatedAt = StatusWaitingForInput, 95, attempt, "", "", nil, now
	runtime.broadcast("task:waiting_for_input", task)
	runtime.broadcastStoryTaskWorkflow("workflow:step_changed", record.UUID)
	return nil
}

func (manager *Manager) ResolveComicStoryboardConflict(ctx context.Context, projectUUID, workflowUUID string, input ResolveComicStoryboardConflictInput) (WorkflowConflictResolution, error) {
	action := strings.TrimSpace(input.Action)
	if !isUUIDv7(workflowUUID) || (action != ComicStoryboardConflictOverwrite && action != ComicStoryboardConflictKeepExisting) || input.ExpectedComicStateRevision == nil || *input.ExpectedComicStateRevision < 0 {
		return WorkflowConflictResolution{}, taskError(CodeInvalidTask, "冲突处理参数无效", "workflow_uuid、action 或 expected_comic_state_revision 不符合要求。", nil)
	}
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return WorkflowConflictResolution{}, err
	}
	runtime.conflictMu.Lock()
	defer runtime.conflictMu.Unlock()
	var taskUUID, threadUUID, workflowStatus, stepStatus, outputJSON string
	err = runtime.sqlDB.QueryRowContext(ctx, `SELECT tasks.uuid,threads.uuid,workflows.status,steps.status,steps.output_json
		FROM workflows
		JOIN chat_threads threads ON threads.id=workflows.thread_id
		JOIN workflow_steps steps ON steps.workflow_id=workflows.id
		JOIN task_runs tasks ON tasks.project_id=workflows.project_id AND tasks.uuid=steps.task_uuid
		WHERE workflows.project_id=? AND workflows.uuid=? AND workflows.kind=? AND steps.step_key=?`, runtime.projectID, workflowUUID, KindComicStoryboardGeneration, "comic_storyboard").Scan(&taskUUID, &threadUUID, &workflowStatus, &stepStatus, &outputJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WorkflowConflictResolution{}, taskError(CodeTaskNotFound, "Comic storyboard workflow 不存在", "该 workflow 可能已经清理。", err)
		}
		return WorkflowConflictResolution{}, err
	}
	resolution := WorkflowConflictResolution{WorkflowUUID: workflowUUID, ThreadUUID: threadUUID, TaskUUID: taskUUID, Action: action}
	if workflowStatus == StatusCompleted && action == ComicStoryboardConflictOverwrite && terminalComicStoryboardResolution(outputJSON, action) {
		resolution.Status = StatusCompleted
		return resolution, nil
	}
	if workflowStatus == StatusCancelled && action == ComicStoryboardConflictKeepExisting && terminalComicStoryboardResolution(outputJSON, action) {
		resolution.Status = StatusCancelled
		return resolution, nil
	}
	if workflowStatus != "running" || stepStatus != "waiting" {
		return WorkflowConflictResolution{}, taskError(CodeTaskStateConflict, "Workflow 当前不等待覆盖确认", "请刷新 workflow 状态后重试。", nil)
	}
	var pending comicStoryboardOverwriteRequest
	if err := json.Unmarshal([]byte(outputJSON), &pending); err != nil || pending.ActionRequired != comicStoryboardOverwriteActionRequired || pending.ExpectedComicStateRevision != *input.ExpectedComicStateRevision {
		return WorkflowConflictResolution{}, taskError(CodeTaskStateConflict, "覆盖确认已过期", "请刷新并使用最新的 Comic state revision 重新确认。", err)
	}
	record, err := getTaskRecord(ctx, runtime.store.DB(), runtime.projectID, taskUUID)
	if err != nil {
		return WorkflowConflictResolution{}, err
	}
	if record.Status != StatusWaitingForInput || record.Kind != KindComicStoryboardGeneration {
		return WorkflowConflictResolution{}, taskError(CodeTaskStateConflict, "任务当前不等待覆盖确认", "请刷新任务状态后重试。", nil)
	}
	if action == ComicStoryboardConflictKeepExisting {
		if err := runtime.declineComicStoryboardOverwrite(ctx, record); err != nil {
			return WorkflowConflictResolution{}, err
		}
		resolution.Status = StatusCancelled
		return resolution, nil
	}
	var snapshot storyGenerationSnapshot
	if err := json.Unmarshal([]byte(record.InputSnapshot), &snapshot); err != nil {
		return WorkflowConflictResolution{}, taskError(CodeTaskPersistenceFailed, "生成输入快照损坏", "无法安全应用已生成的 storyboard。", err)
	}
	raw, found, err := runtime.loadStoryPromptResult(ctx, record.ID)
	if err != nil || !found {
		return WorkflowConflictResolution{}, taskError(CodeTaskPersistenceFailed, "生成结果不存在", "无法读取等待确认的 storyboard 结果。", err)
	}
	generated, err := parseComicStoryboardResponse(raw, snapshot)
	if err != nil {
		return WorkflowConflictResolution{}, taskError(CodeTaskPersistenceFailed, "生成结果损坏", "无法安全解析等待确认的 storyboard 结果。", err)
	}
	productionService := production.NewService(runtime.store, manager.hub)
	sections, err := productionService.ReplaceGeneratedSections(ctx, snapshot.ChapterUUID, generated, *input.ExpectedComicStateRevision)
	if err != nil {
		var productionErr *production.Error
		if errors.As(err, &productionErr) && productionErr.Code == production.CodeStateConflict {
			if refreshErr := runtime.refreshComicStoryboardOverwrite(ctx, record, productionService, len(generated)); refreshErr != nil {
				return WorkflowConflictResolution{}, taskError(CodeTaskPersistenceFailed, "覆盖确认刷新失败", "Comic state 已变化，但无法保存新的确认版本。", errors.Join(err, refreshErr))
			}
		}
		return WorkflowConflictResolution{}, err
	}
	sectionUUIDs := make([]string, len(sections))
	for index := range sections {
		sectionUUIDs[index] = sections[index].UUID
	}
	payload := map[string]any{"project_uuid": projectUUID, "chapter_uuid": snapshot.ChapterUUID, "section_uuids": sectionUUIDs, "overwritten": true}
	if err := runtime.completeStoryWorkflowTask(ctx, record, payload); err != nil {
		return WorkflowConflictResolution{}, err
	}
	resolution.Status = StatusCompleted
	return resolution, nil
}

func terminalComicStoryboardResolution(outputJSON, action string) bool {
	var output struct {
		Overwritten bool   `json:"overwritten"`
		Resolution  string `json:"resolution"`
	}
	if json.Unmarshal([]byte(outputJSON), &output) != nil {
		return false
	}
	if action == ComicStoryboardConflictOverwrite {
		return output.Overwritten
	}
	return output.Resolution == ComicStoryboardConflictKeepExisting
}

func (runtime *projectRuntime) refreshComicStoryboardOverwrite(ctx context.Context, record taskRecord, service *production.Service, generatedCount int) error {
	sections, err := service.ListSections(ctx, record.ResourceUUID)
	if err != nil {
		return err
	}
	state, err := service.GetComicState(ctx, record.ResourceUUID)
	if err != nil {
		return err
	}
	request := comicStoryboardOverwriteRequest{
		ActionRequired:             comicStoryboardOverwriteActionRequired,
		ExistingSectionCount:       len(sections),
		GeneratedSectionCount:      generatedCount,
		ExpectedComicStateRevision: state.Revision,
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return err
	}
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ref, found, err := storyTaskWorkflowRefTx(ctx, tx, record.UUID)
	if err != nil {
		return err
	}
	if !found || ref.Status != StatusRunning || ref.StepStatus != "waiting" {
		return taskError(CodeTaskStateConflict, "Workflow 当前不等待覆盖确认", "请刷新 workflow 状态后重试。", nil)
	}
	var taskStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM task_runs WHERE id=?`, record.ID).Scan(&taskStatus); err != nil {
		return err
	}
	if taskStatus != StatusWaitingForInput {
		return taskError(CodeTaskStateConflict, "任务当前不等待覆盖确认", "请刷新任务状态后重试。", nil)
	}
	now := runtime.manager.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET output_json=?,updated_at=? WHERE id=?`, string(encoded), now, ref.StepID); err != nil {
		return err
	}
	payload := storyTaskWorkflowPayload(ref, record.UUID, StatusWaitingForInput)
	payload["action_required"] = request.ActionRequired
	payload["existing_section_count"] = request.ExistingSectionCount
	payload["generated_section_count"] = request.GeneratedSectionCount
	payload["expected_comic_state_revision"] = request.ExpectedComicStateRevision
	if err := appendTaskEventTx(ctx, tx, record.ID, "overwrite_confirmation_refreshed", payload, now); err != nil {
		return err
	}
	if err := appendStoryTaskWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "conflict_confirmation_refreshed", payload, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	runtime.broadcastStoryTaskWorkflow("workflow:step_changed", record.UUID)
	return nil
}

func (runtime *projectRuntime) declineComicStoryboardOverwrite(ctx context.Context, record taskRecord) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE task_runs SET status=?,progress=0,error_code='',error_message='',completed_at=?,updated_at=? WHERE id=? AND status=?`, StatusCancelled, now, now, record.ID, StatusWaitingForInput)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return taskError(CodeTaskStateConflict, "任务已不再等待确认", "请刷新后重试。", nil)
	}
	payload := map[string]any{
		"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "chapter_uuid": record.ResourceUUID,
		"status": StatusCancelled, "resolution": ComicStoryboardConflictKeepExisting,
	}
	if err := appendTaskEventTx(ctx, tx, record.ID, "overwrite_declined", payload, now); err != nil {
		return err
	}
	var agentThreadID, agentRunID int64
	if err := tx.QueryRowContext(ctx, `SELECT agent_thread_id,id FROM agent_runs WHERE task_run_id=?`, record.ID).Scan(&agentThreadID, &agentRunID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_runs SET status=?,completed_at=?,updated_at=?,error_code='',error_message='' WHERE id=?`, StatusCancelled, now, now, agentRunID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_threads SET status=?,updated_at=? WHERE id=?`, StatusCancelled, now, agentThreadID); err != nil {
		return err
	}
	if err := appendAgentEventTx(ctx, tx, agentThreadID, &agentRunID, "run_cancelled", payload, now); err != nil {
		return err
	}
	ref, found, err := storyTaskWorkflowRefTx(ctx, tx, record.UUID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("comic storyboard workflow projection not found")
	}
	output, _ := json.Marshal(map[string]any{"resolution": ComicStoryboardConflictKeepExisting})
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='cancelled',output_json=?,completed_at=?,error_code='',error_message='',updated_at=? WHERE id=?`, string(output), now, now, ref.StepID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='cancelled',completed_at=?,cancel_requested_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, now, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='cancelled',updated_at=? WHERE id=?`, now, ref.ThreadID); err != nil {
		return err
	}
	workflowPayload := storyTaskWorkflowPayload(ref, record.UUID, StatusCancelled)
	workflowPayload["resolution"] = ComicStoryboardConflictKeepExisting
	if err := appendStoryTaskWorkflowEventTx(ctx, tx, ref.ID, &ref.StepID, "workflow_cancelled", workflowPayload, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status, task.Progress, task.ErrorCode, task.ErrorMessage, task.CompletedAt, task.UpdatedAt = StatusCancelled, 0, "", "", &now, now
	runtime.broadcast("task:cancelled", task)
	runtime.broadcastStoryTaskWorkflow("workflow:cancelled", record.UUID)
	return nil
}
