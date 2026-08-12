package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"lumi/internal/llm"
	"lumi/internal/llmlog"
	"lumi/internal/production"
	"lumi/internal/promptcatalog"
	"lumi/internal/provider"
	"lumi/internal/realtime"
	"lumi/internal/story"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type storyGenerationWorker struct {
	river.WorkerDefaults[riverArgs]
	runtime *projectRuntime
}

func (worker *storyGenerationWorker) Work(ctx context.Context, job *river.Job[riverArgs]) error {
	runtime := worker.runtime
	workCtx, workCancel := context.WithCancel(ctx)
	runtime.registerWork(job.Args.TaskUUID, workCancel)
	defer func() {
		workCancel()
		runtime.unregisterWork(job.Args.TaskUUID)
	}()
	if job.Args.Version != 1 || job.Args.ProjectUUID != runtime.projectUUID || !validStoryTaskKind(job.Args.TaskKind) || !isUUIDv7(job.Args.TaskUUID) || !isUUIDv7(job.Args.ResourceUUID) {
		return river.JobCancel(taskError(CodeInvalidTask, "River job 参数无效", "任务参数版本或 UUID 不受支持。", nil))
	}
	record, err := getTaskRecord(workCtx, runtime.store.DB(), runtime.projectID, job.Args.TaskUUID)
	if err != nil {
		return err
	}
	if record.Status == StatusCompleted || record.Status == StatusCancelled {
		return nil
	}
	var snapshot storyGenerationSnapshot
	if err := json.Unmarshal([]byte(record.InputSnapshot), &snapshot); err != nil {
		_ = runtime.failTask(context.WithoutCancel(ctx), record, "invalid_input_snapshot", "生成输入快照损坏。", false, job.Attempt)
		return river.JobCancel(err)
	}
	if (snapshot.Version != 1 && snapshot.Version != 2 && snapshot.Version != 3) || snapshot.ProjectUUID != runtime.projectUUID || (record.Kind == KindStoryChapterGeneration && snapshot.ChapterUUID != record.ResourceUUID) || (record.Kind != KindStoryChapterGeneration && (snapshot.WorkflowKind != record.Kind || job.Args.ResourceUUID != record.ResourceUUID)) {
		_ = runtime.failTask(context.WithoutCancel(ctx), record, "invalid_input_snapshot", "生成输入快照与任务资源不一致。", false, job.Attempt)
		return river.JobCancel(errors.New("task input snapshot mismatch"))
	}
	agentThreadID, agentRunID, err := runtime.markRunning(workCtx, record, snapshot, job.Attempt)
	if err != nil {
		return err
	}
	record.Attempt = job.Attempt
	resolved, err := runtime.manager.providers.Resolve(workCtx, snapshot.ProviderUUID)
	if err != nil {
		return runtime.handleWorkError(ctx, record, err, job.Attempt, job.MaxAttempts)
	}
	progress := 10
	systemPrompt := snapshot.SystemPrompt
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = storyGenerationSystemPrompt(snapshot.GenerationLanguage)
	}
	response := llm.Response{}
	if record.Kind != KindStoryChapterGeneration {
		if persisted, found, loadErr := runtime.loadStoryPromptResult(workCtx, record.ID); loadErr != nil {
			return runtime.handleWorkError(ctx, record, loadErr, job.Attempt, job.MaxAttempts)
		} else if found {
			response.Content = persisted
		}
	}
	if strings.TrimSpace(response.Content) == "" {
		request := llm.Request{BaseURL: snapshot.ProviderBaseURL, APIKey: resolved.APIKey, Model: snapshot.Model,
			SystemPrompt: systemPrompt,
			Prompt:       snapshot.Prompt, Temperature: snapshot.Parameters.Temperature, MaxTokens: snapshot.Parameters.MaxTokens}
		requestPayload, logErr := llmlog.EncodeTextRequest(request)
		if logErr != nil {
			return runtime.handleWorkError(ctx, record, logErr, job.Attempt, job.MaxAttempts)
		}
		logHandle, logErr := llmlog.Begin(workCtx, runtime.store, llmlog.StartInput{
			ProjectID: runtime.projectID, TaskRunID: record.ID, AgentThreadID: agentThreadID, AgentRunID: agentRunID,
			SourceType: llmlog.SourceStoryGeneration, Scenario: record.Kind, RequestType: llmlog.RequestText, Attempt: job.Attempt,
			ProviderUUID: snapshot.ProviderUUID, ProviderType: snapshot.ProviderType, Model: snapshot.Model, InputSummary: snapshot.Prompt,
			RequestPayload: requestPayload,
		})
		if logErr != nil {
			return runtime.handleWorkError(ctx, record, logErr, job.Attempt, job.MaxAttempts)
		}
		response, err = runtime.manager.llm.Generate(workCtx, request, func(_ string) error {
			progress += 5
			if progress > 90 {
				progress = 90
			}
			return runtime.recordProgress(workCtx, record, progress)
		})
		var responsePayload []byte
		if err == nil {
			responsePayload, logErr = llmlog.EncodeTextResponse(response, request.APIKey)
			if logErr != nil {
				err = logErr
			}
		}
		finishErr := llmlog.Finish(context.WithoutCancel(ctx), runtime.store, logHandle, llmlog.FinishInput{
			OutputSummary: response.Content, InputTokens: response.Usage.InputTokens, CachedInputTokens: response.Usage.CachedInputTokens, OutputTokens: response.Usage.OutputTokens,
			FinishReason: response.FinishReason, Response: responsePayload, Err: err,
		})
		if finishErr != nil {
			if err != nil {
				err = errors.Join(err, finishErr)
			} else {
				err = finishErr
			}
		}
		if err != nil {
			return runtime.handleWorkError(ctx, record, err, job.Attempt, job.MaxAttempts)
		}
	}
	if cancelled, checkErr := runtime.cancelRequested(workCtx, record.ID); checkErr != nil {
		return runtime.handleWorkError(ctx, record, checkErr, job.Attempt, job.MaxAttempts)
	} else if cancelled {
		err := context.Canceled
		return runtime.handleWorkError(ctx, record, err, job.Attempt, job.MaxAttempts)
	}
	if record.Kind == KindStoryChapterGeneration {
		content, title, contentFormat, parseErr := parseStoryChapterResponse(response.Content, snapshot)
		if parseErr != nil {
			return runtime.handleWorkError(ctx, record, parseErr, job.Attempt, job.MaxAttempts)
		}
		chapter, applyErr := story.NewService(runtime.store).ApplyGeneratedChapter(workCtx, record.UUID, snapshot.ChapterUUID, title, content, contentFormat, snapshot.ChapterRevision)
		if applyErr != nil {
			return runtime.handleWorkError(ctx, record, applyErr, job.Attempt, job.MaxAttempts)
		}
		if err := runtime.completeTask(workCtx, record, chapter); err != nil {
			return err
		}
	} else {
		if err := runtime.persistStoryPromptResult(workCtx, record, response.Content); err != nil {
			return runtime.handleWorkError(ctx, record, err, job.Attempt, job.MaxAttempts)
		}
		payload, applyErr := runtime.applyStoryWorkflowResponse(workCtx, record, snapshot, response.Content)
		if applyErr != nil {
			return runtime.handleWorkError(ctx, record, applyErr, job.Attempt, job.MaxAttempts)
		}
		if err := runtime.completeStoryWorkflowTask(workCtx, record, payload); err != nil {
			return err
		}
	}
	return nil
}

func parseStoryChapterResponse(raw string, snapshot storyGenerationSnapshot) (content, title, contentFormat string, err error) {
	if snapshot.Version == 1 {
		return strings.TrimSpace(raw), "", "md", nil
	}
	var output struct {
		ChapterCode   string `json:"chapter_code"`
		Title         string `json:"title"`
		Content       string `json:"content"`
		ContentFormat string `json:"content_format"`
	}
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(raw)), &output); decodeErr != nil {
		return "", "", "", &llm.Error{Code: llm.CodeInvalidContent, SafeMessage: "模型没有返回规范的章节 JSON。", Retryable: false, Cause: decodeErr}
	}
	output.ChapterCode = strings.TrimSpace(output.ChapterCode)
	output.Title = strings.TrimSpace(output.Title)
	output.Content = strings.TrimSpace(output.Content)
	output.ContentFormat = strings.ToLower(strings.TrimSpace(output.ContentFormat))
	if output.ChapterCode == "" || output.ChapterCode != snapshot.ChapterCode {
		return "", "", "", &llm.Error{Code: llm.CodeInvalidContent, SafeMessage: "模型返回的 chapter_code 与任务快照不一致。", Retryable: false}
	}
	if output.Title == "" || output.Content == "" || output.ContentFormat != "txt" {
		return "", "", "", &llm.Error{Code: llm.CodeInvalidContent, SafeMessage: "模型返回的章节 JSON 字段不完整。", Retryable: false}
	}
	return output.Content, output.Title, output.ContentFormat, nil
}

func storyGenerationSystemPrompt(language string) string {
	definition, _ := promptcatalog.Lookup(promptcatalog.GroupStory, "json_system", language)
	return promptcatalog.WithLanguageInstruction(definition.DefaultValue, language)
}

func (runtime *projectRuntime) markRunning(ctx context.Context, record taskRecord, snapshot storyGenerationSnapshot, attempt int) (int64, int64, error) {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE task_runs SET status = ?, progress = 5, attempt = ?, started_at = COALESCE(started_at, ?), updated_at = ?, error_code = '', error_message = '' WHERE id = ? AND cancel_requested_at IS NULL AND status NOT IN ('completed','cancelled')`, StatusRunning, attempt, now, now, record.ID)
	if err != nil {
		return 0, 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, 0, err
	}
	if rows != 1 {
		return 0, 0, context.Canceled
	}
	if err := appendTaskEventTx(ctx, tx, record.ID, "task_started", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "chapter_uuid": record.ResourceUUID, "status": StatusRunning, "progress": 5, "attempt": attempt}, now); err != nil {
		return 0, 0, err
	}
	var threadID, runID int64
	if err := tx.QueryRowContext(ctx, `SELECT agent_thread_id, id FROM agent_runs WHERE task_run_id = ?`, record.ID).Scan(&threadID, &runID); err != nil {
		return 0, 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_runs SET status = 'running', started_at = COALESCE(started_at, ?), updated_at = ?, error_code = '', error_message = '' WHERE id = ?`, now, now, runID); err != nil {
		return 0, 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_threads SET status = 'running', provider_uuid = ?, model = ?, updated_at = ? WHERE id = ?`, snapshot.ProviderUUID, snapshot.Model, now, threadID); err != nil {
		return 0, 0, err
	}
	if err := appendAgentEventTx(ctx, tx, threadID, &runID, "run_started", map[string]any{"task_uuid": record.UUID, "chapter_uuid": record.ResourceUUID, "status": StatusRunning, "attempt": attempt}, now); err != nil {
		return 0, 0, err
	}
	if record.Kind == KindComicStoryboardGeneration {
		if err := markComicStoryboardWorkflowRunningTx(ctx, tx, record.UUID, now); err != nil {
			return 0, 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	updated := record.DTO()
	updated.Status, updated.Progress, updated.Attempt, updated.StartedAt, updated.UpdatedAt = StatusRunning, 5, attempt, &now, now
	runtime.broadcast("task:running", updated)
	if record.Kind == KindComicStoryboardGeneration {
		runtime.broadcastComicStoryboardWorkflow("workflow:step_changed", record.UUID)
	}
	return threadID, runID, nil
}

func (runtime *projectRuntime) recordProgress(ctx context.Context, record taskRecord, progress int) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	var cancelled bool
	if err := tx.QueryRowContext(ctx, `SELECT cancel_requested_at IS NOT NULL OR status = 'cancelled' FROM task_runs WHERE id = ?`, record.ID).Scan(&cancelled); err != nil {
		return err
	}
	if cancelled {
		return context.Canceled
	}
	if _, err := tx.ExecContext(ctx, `UPDATE task_runs SET progress = CASE WHEN progress < ? THEN ? ELSE progress END, updated_at = ? WHERE id = ? AND status = 'running'`, progress, progress, now, record.ID); err != nil {
		return err
	}
	if err := appendTaskEventTx(ctx, tx, record.ID, "task_progress", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "chapter_uuid": record.ResourceUUID, "status": StatusRunning, "progress": progress}, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status, task.Progress, task.UpdatedAt = StatusRunning, progress, now
	runtime.broadcast("task:progress", task)
	return nil
}

func (runtime *projectRuntime) completeTask(ctx context.Context, record taskRecord, chapter story.Chapter) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE task_runs SET status = 'completed', progress = 100, completed_at = ?, updated_at = ?, error_code = '', error_message = '' WHERE id = ? AND cancel_requested_at IS NULL AND status <> 'cancelled'`, now, now, record.ID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return context.Canceled
	}
	payload := map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "chapter_uuid": chapter.UUID, "status": StatusCompleted, "progress": 100}
	if chapter.CurrentStory != nil {
		payload["chapter_story_uuid"] = chapter.CurrentStory.UUID
	}
	if err := appendTaskEventTx(ctx, tx, record.ID, "task_completed", payload, now); err != nil {
		return err
	}
	var threadID, runID int64
	if err := tx.QueryRowContext(ctx, `SELECT agent_thread_id, id FROM agent_runs WHERE task_run_id = ?`, record.ID).Scan(&threadID, &runID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_runs SET status = 'completed', completed_at = ?, updated_at = ? WHERE id = ?`, now, now, runID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_threads SET status = 'completed', updated_at = ? WHERE id = ?`, now, threadID); err != nil {
		return err
	}
	if err := appendAgentEventTx(ctx, tx, threadID, &runID, "run_completed", payload, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status, task.Progress, task.CompletedAt, task.UpdatedAt = StatusCompleted, 100, &now, now
	runtime.broadcast("task:completed", task)
	if runtime.manager.hub != nil {
		runtime.manager.hub.Broadcast(realtime.ProjectTopic(runtime.projectUUID), "story:chapter_changed", payload)
	}
	return nil
}

func (runtime *projectRuntime) handleWorkError(ctx context.Context, record taskRecord, err error, attempt, maxAttempts int) error {
	code, message, retryable, cancelled := classifyWorkError(err)
	if cancelled {
		requested, checkErr := runtime.cancelRequested(context.WithoutCancel(ctx), record.ID)
		if checkErr == nil && !requested {
			// River cancels worker contexts during bounded project shutdown too.
			// Preserve the retryable product task in queued state so reopening the
			// project can resume it; only an explicit persisted cancel request is
			// projected as a user cancellation.
			_ = runtime.pauseTask(context.WithoutCancel(ctx), record, attempt)
			return err
		}
		_ = runtime.cancelTaskProjection(context.WithoutCancel(ctx), record, code, message)
		return err
	}
	_ = runtime.failTask(context.WithoutCancel(ctx), record, code, message, retryable, attempt)
	if !retryable {
		return river.JobCancel(err)
	}
	if attempt >= maxAttempts {
		return err
	}
	return err
}

func (runtime *projectRuntime) pauseTask(ctx context.Context, record taskRecord, attempt int) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE task_runs SET status = 'queued', progress = 0, attempt = ?, updated_at = ?, error_code = '', error_message = '' WHERE id = ? AND status = 'running' AND cancel_requested_at IS NULL`, attempt, now, record.ID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return err
	}
	payload := map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "chapter_uuid": record.ResourceUUID, "status": StatusQueued, "attempt": attempt, "reason": "runtime_stopped"}
	if err := appendTaskEventTx(ctx, tx, record.ID, "task_paused", payload, now); err != nil {
		return err
	}
	if record.Kind == KindComicStoryboardGeneration {
		if err := queueComicStoryboardWorkflowTx(ctx, tx, record.UUID, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status, task.Progress, task.Attempt, task.UpdatedAt = StatusQueued, 0, attempt, now
	runtime.broadcast("task:queued", task)
	if record.Kind == KindComicStoryboardGeneration {
		runtime.broadcastComicStoryboardWorkflow("workflow:queued", record.UUID)
	}
	return nil
}

func (runtime *projectRuntime) failTask(ctx context.Context, record taskRecord, code, message string, retryable bool, attempt int) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE task_runs SET status = 'failed', progress = 0, attempt = ?, error_code = ?, error_message = ?, completed_at = ?, updated_at = ? WHERE id = ? AND status <> 'cancelled'`, attempt, code, message, now, now, record.ID); err != nil {
		return err
	}
	payload := map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "chapter_uuid": record.ResourceUUID, "status": StatusFailed, "progress": 0, "attempt": attempt, "error_code": code, "error_message": message, "retryable": retryable}
	if err := appendTaskEventTx(ctx, tx, record.ID, "task_failed", payload, now); err != nil {
		return err
	}
	if record.Kind == KindComicStoryboardGeneration {
		if err := failComicStoryboardWorkflowTx(ctx, tx, record.UUID, code, message, now); err != nil {
			return err
		}
	}
	var threadID, runID int64
	if err := tx.QueryRowContext(ctx, `SELECT agent_thread_id, id FROM agent_runs WHERE task_run_id = ?`, record.ID).Scan(&threadID, &runID); err == nil {
		_, _ = tx.ExecContext(ctx, `UPDATE agent_runs SET status = 'failed', error_code = ?, error_message = ?, completed_at = ?, updated_at = ? WHERE id = ?`, code, message, now, now, runID)
		_, _ = tx.ExecContext(ctx, `UPDATE agent_threads SET status = 'failed', updated_at = ? WHERE id = ?`, now, threadID)
		_ = appendAgentEventTx(ctx, tx, threadID, &runID, "run_failed", payload, now)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status, task.Progress, task.Attempt, task.ErrorCode, task.ErrorMessage, task.CompletedAt, task.UpdatedAt = StatusFailed, 0, attempt, code, message, &now, now
	runtime.broadcast("task:failed", task)
	if record.Kind == KindComicStoryboardGeneration {
		runtime.broadcastComicStoryboardWorkflow("workflow:failed", record.UUID)
	}
	return nil
}

func (runtime *projectRuntime) cancelTaskProjection(ctx context.Context, record taskRecord, code, message string) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE task_runs SET status = 'cancelled', completed_at = COALESCE(completed_at, ?), updated_at = ?, error_code = '', error_message = '' WHERE id = ? AND status <> 'completed'`, now, now, record.ID); err != nil {
		return err
	}
	if err := appendTaskEventTx(ctx, tx, record.ID, "task_cancelled", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "chapter_uuid": record.ResourceUUID, "status": StatusCancelled}, now); err != nil {
		return err
	}
	if record.Kind == KindComicStoryboardGeneration {
		if err := cancelComicStoryboardWorkflowTx(ctx, tx, record.UUID, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status, task.CompletedAt, task.UpdatedAt = StatusCancelled, &now, now
	runtime.broadcast("task:cancelled", task)
	if record.Kind == KindComicStoryboardGeneration {
		runtime.broadcastComicStoryboardWorkflow("workflow:cancelled", record.UUID)
	}
	return nil
}

func (runtime *projectRuntime) cancelRequested(ctx context.Context, taskID int64) (bool, error) {
	var cancelled bool
	err := runtime.sqlDB.QueryRowContext(ctx, `SELECT cancel_requested_at IS NOT NULL OR status = 'cancelled' FROM task_runs WHERE id = ?`, taskID).Scan(&cancelled)
	return cancelled, err
}

func (runtime *projectRuntime) projectRiverEvent(ctx context.Context, event *river.Event) error {
	var agentEnvelope agentArgs
	if err := json.Unmarshal(event.Job.EncodedArgs, &agentEnvelope); err == nil && agentEnvelope.JobKind != "" {
		// The Agent runtime owns its durable events and realtime projection.
		// Generic task projection must not interpret Agent UUIDs as story tasks.
		return nil
	}
	var maintenanceEnvelope struct {
		MaintenanceKind string `json:"maintenance_kind"`
	}
	if err := json.Unmarshal(event.Job.EncodedArgs, &maintenanceEnvelope); err == nil && maintenanceEnvelope.MaintenanceKind != "" {
		// The maintenance worker persists and broadcasts its own public projection.
		return nil
	}
	var productionEnvelope productionArgs
	if err := json.Unmarshal(event.Job.EncodedArgs, &productionEnvelope); err == nil && validProductionKind(productionEnvelope.TaskKind) {
		if productionEnvelope.ProjectUUID != runtime.projectUUID {
			return nil
		}
		return runtime.projectProductionRiverEvent(ctx, event, productionEnvelope)
	}
	var args riverArgs
	if err := json.Unmarshal(event.Job.EncodedArgs, &args); err != nil || args.ProjectUUID != runtime.projectUUID {
		return err
	}
	record, err := getTaskRecord(ctx, runtime.store.DB(), runtime.projectID, args.TaskUUID)
	if err != nil {
		return err
	}
	if record.Status == StatusCompleted || record.Status == StatusCancelled || (record.Status == StatusFailed && event.Kind == river.EventKindJobCancelled) {
		return nil
	}
	if event.Kind == river.EventKindJobCancelled && record.Status == StatusQueued && record.CancelRequestedAt == nil {
		// JobRetryTx can requeue a previously cancelled River row before its
		// cancellation event reaches this consumer. The durable task projection is
		// authoritative only when River also reports that the job has moved on.
		currentJob, currentErr := runtime.client.JobGet(ctx, event.Job.ID)
		if currentErr == nil && currentJob.State != rivertype.JobStateCancelled {
			return nil
		}
	}
	if event.Kind == river.EventKindJobFailed && event.Job.State != rivertype.JobStateDiscarded {
		tx, err := runtime.sqlDB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		now := runtime.manager.now().UTC()
		if _, err := tx.ExecContext(ctx, `UPDATE task_runs SET status = 'queued', progress = 0, updated_at = ? WHERE id = ? AND status = 'failed'`, now, record.ID); err != nil {
			return err
		}
		if err := appendTaskEventTx(ctx, tx, record.ID, "retry_scheduled", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "chapter_uuid": record.ResourceUUID, "status": StatusQueued, "attempt": event.Job.Attempt}, now); err != nil {
			return err
		}
		if record.Kind == KindComicStoryboardGeneration {
			if err := queueComicStoryboardWorkflowTx(ctx, tx, record.UUID, now); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		task := record.DTO()
		task.Status, task.Progress, task.Attempt, task.UpdatedAt = StatusQueued, 0, event.Job.Attempt, now
		runtime.broadcast("task:queued", task)
		if record.Kind == KindComicStoryboardGeneration {
			runtime.broadcastComicStoryboardWorkflow("workflow:queued", record.UUID)
		}
		return nil
	}
	if event.Kind == river.EventKindJobCancelled {
		return runtime.cancelTaskProjection(ctx, record, llm.CodeCancelled, "请求已取消。")
	}
	if event.Kind == river.EventKindJobFailed && event.Job.State == rivertype.JobStateDiscarded {
		return runtime.failTask(ctx, record, record.ErrorCode, safeFallback(record.ErrorMessage, "任务重试次数已用尽。"), false, event.Job.Attempt)
	}
	return nil
}

func classifyWorkError(err error) (code, message string, retryable, cancelled bool) {
	if errors.Is(err, context.Canceled) || errors.Is(err, river.ErrJobCancelledRemotely) {
		return llm.CodeCancelled, "请求已取消。", false, true
	}
	var llmErr *llm.Error
	if errors.As(err, &llmErr) {
		return llmErr.Code, llmErr.SafeMessage, llmErr.Retryable, llmErr.Code == llm.CodeCancelled
	}
	var providerErr *provider.Error
	if errors.As(err, &providerErr) {
		return providerErr.Code, providerErr.Message, false, false
	}
	var storyErr *story.Error
	if errors.As(err, &storyErr) {
		return storyErr.Code, storyErr.Message, false, false
	}
	var productionErr *production.Error
	if errors.As(err, &productionErr) {
		return productionErr.Code, productionErr.Message, false, false
	}
	return "local_persistence_error", "本地任务状态保存失败。", true, false
}

func workErrorCode(err error) string { code, _, _, _ := classifyWorkError(err); return code }

func safeFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
