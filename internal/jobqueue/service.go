package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"lumi/internal/modelsettings"
	"lumi/internal/project"
	"lumi/internal/promptcatalog"
	"lumi/internal/provider"
	"lumi/internal/story"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"gorm.io/gorm"
)

func (manager *Manager) CreateChapterGeneration(ctx context.Context, projectUUID string, input CreateGenerationInput) (Task, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return Task{}, err
	}
	generationLanguage, err := loadProjectGenerationLanguage(ctx, runtime.store)
	if err != nil {
		return Task{}, taskError(CodeTaskPersistenceFailed, "无法读取项目生成语言", "任务尚未创建。", err)
	}
	input.ChapterUUID = strings.TrimSpace(input.ChapterUUID)
	input.ProviderUUID = strings.TrimSpace(input.ProviderUUID)
	input.Model = strings.TrimSpace(input.Model)
	input.PromptKey = strings.ToLower(strings.TrimSpace(input.PromptKey))
	if input.PromptKey == "" {
		input.PromptKey = "story_chapter"
	}
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if !isUUIDv7(input.ChapterUUID) || (input.ProviderUUID != "" && !isUUIDv7(input.ProviderUUID)) || (input.PromptKey != "story_chapter" && input.PromptKey != "next_story_chapter") || input.Prompt == "" || len(input.Prompt) > 256<<10 || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 255 {
		return Task{}, taskError(CodeInvalidTask, "生成任务输入无效", "chapter_uuid、prompt 和 idempotency_key 必须有效且符合长度限制。", nil)
	}
	if input.Parameters.Temperature != nil && (*input.Parameters.Temperature < 0 || *input.Parameters.Temperature > 2) {
		return Task{}, taskError(CodeInvalidTask, "生成参数无效", "temperature 必须在 0 到 2 之间。", nil)
	}
	if input.Parameters.MaxTokens < 0 || input.Parameters.MaxTokens > 200000 {
		return Task{}, taskError(CodeInvalidTask, "生成参数无效", "max_tokens 超出安全范围。", nil)
	}
	resolved, model, modelSource, err := manager.resolveProjectModel(ctx, runtime.store, modelsettings.StoryText, modelsettings.KindText, input.ProviderUUID, input.Model)
	if err != nil {
		return Task{}, err
	}
	storyService := story.NewService(runtime.store)
	chapter, err := storyService.GetChapter(ctx, input.ChapterUUID)
	if err != nil {
		return Task{}, err
	}
	if chapter.TrashedAt != nil {
		return Task{}, taskError(CodeInvalidTask, "章节已在回收站", "不能为回收站章节创建生成任务。", nil)
	}
	var storyUUID, inputContent string
	if chapter.CurrentStory != nil {
		storyUUID, inputContent = chapter.CurrentStory.UUID, chapter.CurrentStory.Content
	}
	promptTemplate, err := storyService.EffectivePrompt(ctx, promptcatalog.GroupStory, input.PromptKey)
	if err != nil {
		return Task{}, err
	}
	systemPrompt, err := storyService.EffectivePrompt(ctx, promptcatalog.GroupStory, "json_system")
	if err != nil {
		return Task{}, err
	}
	languageInstruction, err := storyService.EffectiveLanguageInstruction(ctx)
	if err != nil {
		return Task{}, err
	}
	systemPrompt = promptcatalog.WithInstruction(systemPrompt, languageInstruction)
	finalPrompt, err := buildStoryChapterPrompt(ctx, storyService, chapter, input.PromptKey, input.Prompt, promptTemplate)
	if err != nil {
		return Task{}, taskError(CodeInvalidTask, "章节提示词无法渲染", "请检查项目提示词是否保留全部规范占位符。", err)
	}
	snapshot := storyGenerationSnapshot{Version: 2, ProjectUUID: projectUUID, GenerationLanguage: generationLanguage, ChapterUUID: chapter.UUID, ChapterCode: chapter.ChapterCode, ChapterStoryUUID: storyUUID,
		ChapterRevision: chapter.Revision, InputContent: inputContent, PromptKey: input.PromptKey, PromptTemplate: promptTemplate,
		SystemPrompt: systemPrompt, Prompt: finalPrompt,
		ProviderUUID: resolved.UUID, ProviderType: resolved.ProviderType, ProviderBaseURL: resolved.BaseURL, Model: model, ModelSource: modelSource, Parameters: input.Parameters}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return Task{}, taskError(CodeTaskPersistenceFailed, "无法固化生成输入", "任务尚未创建。", err)
	}
	taskUUID, err := newUUIDv7()
	if err != nil {
		return Task{}, err
	}
	now := manager.now().UTC()
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, taskError(CodeTaskPersistenceFailed, "无法创建任务事务", "任务尚未创建。", err)
	}
	defer tx.Rollback()
	if existing, found, err := findTaskTx(ctx, tx, runtime.projectID, KindStoryChapterGeneration, input.IdempotencyKey); err != nil {
		return Task{}, err
	} else if found {
		if err := tx.Commit(); err != nil {
			return Task{}, err
		}
		return existing.DTO(), nil
	}
	if active, found, err := findActiveResourceTaskTx(ctx, tx, runtime.projectID, input.ChapterUUID); err != nil {
		return Task{}, err
	} else if found {
		return Task{}, taskError(CodeTaskConflict, "章节已有进行中的生成任务", "请等待任务 "+active.UUID+" 完成或先取消它。", nil)
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO task_runs
		(uuid, project_id, river_job_id, kind, resource_uuid, input_version, input_snapshot, status, idempotency_key, retryable, provider_uuid, model, model_source, progress, attempt, max_attempts, error_code, error_message, created_at, updated_at)
		VALUES (?, ?, NULL, ?, ?, 2, ?, ?, ?, 1, ?, ?, ?, 0, 0, 3, '', '', ?, ?)`,
		taskUUID, runtime.projectID, KindStoryChapterGeneration, input.ChapterUUID, string(snapshotJSON), StatusQueued, input.IdempotencyKey, resolved.UUID, model, modelSource, now, now)
	if err != nil {
		return Task{}, taskError(CodeTaskPersistenceFailed, "无法持久化生成任务", "任务与队列 job 均未创建。", err)
	}
	taskID, err := result.LastInsertId()
	if err != nil {
		return Task{}, err
	}
	if err := appendTaskEventTx(ctx, tx, taskID, "task_queued", map[string]any{"project_uuid": projectUUID, "task_uuid": taskUUID, "chapter_uuid": input.ChapterUUID, "status": StatusQueued, "progress": 0}, now); err != nil {
		return Task{}, err
	}
	threadID, runID, err := createAgentAuditTx(ctx, tx, runtime.projectID, taskID, taskUUID, input.ChapterUUID, resolved.UUID, model, input.Prompt, now)
	if err != nil {
		return Task{}, err
	}
	_ = threadID
	_ = runID
	inserted, err := runtime.client.InsertTx(ctx, tx, riverArgs{Version: 1, ProjectUUID: projectUUID, TaskUUID: taskUUID, TaskKind: KindStoryChapterGeneration, ResourceUUID: input.ChapterUUID}, &river.InsertOpts{
		Queue: QueueStory, MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateRetryable, rivertype.JobStateScheduled}},
	})
	if err != nil {
		return Task{}, taskError(CodeTaskPersistenceFailed, "无法插入队列任务", "产品任务与 River job 已一并回滚。", err)
	}
	if inserted.UniqueSkippedAsDuplicate {
		return Task{}, taskError(CodeTaskConflict, "章节已有互斥生成任务", "River unique job 拒绝了重复任务。", nil)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE task_runs SET river_job_id = ? WHERE id = ?", inserted.Job.ID, taskID); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, taskError(CodeTaskPersistenceFailed, "无法提交生成任务", "产品任务与 River job 未提交。", err)
	}
	task, err := manager.GetTask(ctx, projectUUID, taskUUID)
	if err == nil {
		runtime.broadcast("task:queued", task)
	}
	return task, err
}

func loadProjectGenerationLanguage(ctx context.Context, store *project.Store) (string, error) {
	var value string
	if err := store.DB().WithContext(ctx).Model(&project.Project{}).Where("uuid = ?", store.ProjectUUID()).Pluck("generation_language", &value).Error; err != nil {
		return "", err
	}
	language, valid := project.NormalizeGenerationLanguage(value)
	if !valid {
		return "", fmt.Errorf("unsupported project generation language %q", value)
	}
	return language, nil
}

// resolveProvider uses the global active Provider for public requests. Internal
// workflow continuations pass their already-frozen Provider UUID explicitly.
func (manager *Manager) resolveProvider(ctx context.Context, providerUUID string) (provider.Resolved, error) {
	providerUUID = strings.TrimSpace(providerUUID)
	if providerUUID == "" {
		return manager.providers.Active(ctx)
	}
	if !isUUIDv7(providerUUID) {
		return provider.Resolved{}, taskError(CodeInvalidTask, "Provider UUID 无效", "内部 Provider 快照必须是 UUIDv7。", nil)
	}
	return manager.providers.Resolve(ctx, providerUUID)
}

// resolveProjectModel resolves new public requests through the project model
// hierarchy. A non-empty Provider UUID is an internal, already-frozen
// continuation and deliberately bypasses current option discovery so retries do
// not drift when site settings change.
func (manager *Manager) resolveProjectModel(ctx context.Context, store *project.Store, settingKey, kind, providerUUID, model string) (provider.Resolved, string, string, error) {
	providerUUID = strings.TrimSpace(providerUUID)
	model = strings.TrimSpace(model)
	if providerUUID != "" {
		item, err := manager.resolveProvider(ctx, providerUUID)
		if err != nil {
			return provider.Resolved{}, "", "", err
		}
		if model == "" {
			model = item.DefaultModel
			if kind == modelsettings.KindImage {
				model = item.DefaultImageModel
			}
		}
		if model == "" || len([]rune(model)) > 512 {
			return provider.Resolved{}, "", "", taskError(CodeInvalidTask, "模型无效", "model 不能为空且最多 512 个字符。", nil)
		}
		return item, model, modelsettings.SourceExplicitTask, nil
	}
	choice, err := manager.models.Resolve(ctx, store, settingKey, kind, "", model)
	if err != nil {
		var settingsErr *modelsettings.Error
		if errors.As(err, &settingsErr) {
			return provider.Resolved{}, "", "", taskError(CodeInvalidTask, settingsErr.Message, settingsErr.Details, err)
		}
		return provider.Resolved{}, "", "", err
	}
	return choice.Provider, choice.Model, choice.Source, nil
}

func (manager *Manager) ListTasks(ctx context.Context, projectUUID, status string, limit int) ([]Task, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	query := runtime.store.DB().WithContext(ctx).Where("project_id = ?", runtime.projectID)
	if status != "" {
		if !validStatus(status) {
			return nil, taskError(CodeInvalidTask, "任务状态无效", "status 不是受支持的产品任务状态。", nil)
		}
		query = query.Where("status = ?", status)
	}
	var records []taskRecord
	if err := query.Order("created_at DESC, id DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	items := make([]Task, 0, len(records))
	for _, record := range records {
		items = append(items, record.DTO())
	}
	return items, nil
}

func (manager *Manager) GetTask(ctx context.Context, projectUUID, taskUUID string) (Task, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return Task{}, err
	}
	record, err := getTaskRecord(ctx, runtime.store.DB(), runtime.projectID, taskUUID)
	if err != nil {
		return Task{}, err
	}
	return record.DTO(), nil
}

func (manager *Manager) ListTaskEvents(ctx context.Context, projectUUID, taskUUID string, before, after int64, limit int) ([]TaskEvent, CursorPagination, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return nil, CursorPagination{}, err
	}
	task, err := getTaskRecord(ctx, runtime.store.DB(), runtime.projectID, taskUUID)
	if err != nil {
		return nil, CursorPagination{}, err
	}
	if before < 0 || after < 0 || (before > 0 && after > 0) {
		return nil, CursorPagination{}, taskError(CodeInvalidTask, "事件游标无效", "before 与 after 必须是非负 sequence，且不能同时使用。", nil)
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	query := runtime.store.DB().WithContext(ctx).Where("task_run_id = ?", task.ID)
	order := "sequence ASC"
	if before > 0 {
		query = query.Where("sequence < ?", before)
		order = "sequence DESC"
	} else if after > 0 {
		query = query.Where("sequence > ?", after)
	}
	var records []taskEventRecord
	if err := query.Order(order).Limit(limit + 1).Find(&records).Error; err != nil {
		return nil, CursorPagination{}, err
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	if before > 0 {
		for left, right := 0, len(records)-1; left < right; left, right = left+1, right-1 {
			records[left], records[right] = records[right], records[left]
		}
	}
	items := make([]TaskEvent, 0, len(records))
	for _, record := range records {
		items = append(items, TaskEvent{UUID: record.UUID, Sequence: record.Sequence, EventType: record.EventType, Payload: json.RawMessage(record.Payload), CreatedAt: record.CreatedAt})
	}
	var next, previous *string
	if len(items) > 0 {
		firstCursor := fmt.Sprintf("%d", items[0].Sequence)
		lastCursor := fmt.Sprintf("%d", items[len(items)-1].Sequence)
		if before > 0 {
			next = &lastCursor
			if hasMore {
				previous = &firstCursor
			}
		} else {
			if hasMore {
				next = &lastCursor
			}
			if after > 0 {
				previous = &firstCursor
			}
		}
	}
	return items, CursorPagination{PerPage: limit, NextCursor: next, PrevCursor: previous, HasMore: hasMore}, nil
}

func (manager *Manager) CancelTask(ctx context.Context, projectUUID, taskUUID string) (Task, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return Task{}, err
	}
	// Cancel the application-level provider context before waiting for the
	// single SQLite connection. This lets an in-flight result transaction
	// observe cancellation and roll back instead of winning the DB lock race.
	runtime.cancelWork(taskUUID)
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	record, found, err := findTaskByUUIDTx(ctx, tx, runtime.projectID, taskUUID)
	if err != nil {
		return Task{}, err
	}
	if !found {
		return Task{}, taskError(CodeTaskNotFound, "任务不存在", "该任务可能已经清理。", nil)
	}
	if record.Status == StatusCompleted || record.Status == StatusFailed || record.Status == StatusCancelled || record.Status == StatusInterrupted {
		if err := tx.Commit(); err != nil {
			return Task{}, err
		}
		return record.DTO(), nil
	}
	var resultCommitted bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM story_generation_results WHERE task_run_id = ?
		UNION ALL
		SELECT 1 FROM story_prompt_results WHERE task_run_id = ?
	)`, record.ID, record.ID).Scan(&resultCommitted); err != nil {
		return Task{}, err
	}
	if resultCommitted {
		// The append-only Story mutation is already durable. The worker (or its
		// idempotent retry after restart) owns the remaining completion projection;
		// reporting cancellation here would make product state contradict content.
		if err := tx.Commit(); err != nil {
			return Task{}, err
		}
		return record.DTO(), nil
	}
	if record.RiverJobID == nil {
		return Task{}, taskError(CodeTaskPersistenceFailed, "任务缺少队列关联", "任务无法安全取消。", nil)
	}
	now := manager.now().UTC()
	if _, err := runtime.client.JobCancelTx(ctx, tx, *record.RiverJobID); err != nil {
		return Task{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE task_runs SET status = ?, cancel_requested_at = ?, completed_at = ?, updated_at = ?, error_code = '', error_message = '' WHERE id = ?`, StatusCancelled, now, now, now, record.ID); err != nil {
		return Task{}, err
	}
	if err := appendTaskEventTx(ctx, tx, record.ID, "cancel_requested", map[string]any{"project_uuid": projectUUID, "task_uuid": taskUUID, "status": StatusCancelled}, now); err != nil {
		return Task{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_runs SET status = ?, completed_at = ?, updated_at = ? WHERE task_run_id = ?;`, StatusCancelled, now, now, record.ID); err != nil {
		return Task{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_threads SET status = ?, updated_at = ? WHERE id IN (SELECT agent_thread_id FROM agent_runs WHERE task_run_id = ?)`, StatusCancelled, now, record.ID); err != nil {
		return Task{}, err
	}
	if record.Kind == KindComicStoryboardGeneration {
		if err := cancelComicStoryboardWorkflowTx(ctx, tx, record.UUID, now); err != nil {
			return Task{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	_, _ = runtime.client.JobCancel(context.WithoutCancel(ctx), *record.RiverJobID)
	task, err := manager.GetTask(ctx, projectUUID, taskUUID)
	if err == nil {
		runtime.broadcast("task:cancelled", task)
		if record.Kind == KindComicStoryboardGeneration {
			runtime.broadcastComicStoryboardWorkflow("workflow:cancelled", record.UUID)
		}
	}
	return task, err
}

func (manager *Manager) RetryTask(ctx context.Context, projectUUID, taskUUID string) (Task, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return Task{}, err
	}
	preflight, err := getTaskRecord(ctx, runtime.store.DB(), runtime.projectID, taskUUID)
	if err != nil {
		return Task{}, err
	}
	if !preflight.Retryable || (preflight.Status != StatusFailed && preflight.Status != StatusInterrupted && preflight.Status != StatusCancelled) {
		return Task{}, taskError(CodeTaskStateConflict, "任务当前不能重试", "只有声明可重试的 failed、interrupted 或 cancelled 任务可以显式重试。", nil)
	}
	if preflight.RiverJobID == nil {
		return Task{}, taskError(CodeTaskPersistenceFailed, "任务缺少队列关联", "任务无法安全重试。", nil)
	}
	if err := waitForStoryTaskRetryBoundary(ctx, runtime, *preflight.RiverJobID); err != nil {
		return Task{}, err
	}
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	record, found, err := findTaskByUUIDTx(ctx, tx, runtime.projectID, taskUUID)
	if err != nil {
		return Task{}, err
	}
	if !found {
		return Task{}, taskError(CodeTaskNotFound, "任务不存在", "该任务可能已经清理。", nil)
	}
	if !record.Retryable || (record.Status != StatusFailed && record.Status != StatusInterrupted && record.Status != StatusCancelled) {
		return Task{}, taskError(CodeTaskStateConflict, "任务当前不能重试", "只有声明可重试的 failed、interrupted 或 cancelled 任务可以显式重试。", nil)
	}
	if record.RiverJobID == nil {
		return Task{}, taskError(CodeTaskPersistenceFailed, "任务缺少队列关联", "任务无法安全重试。", nil)
	}
	if _, err := runtime.client.JobRetryTx(ctx, tx, *record.RiverJobID); err != nil {
		return Task{}, err
	}
	now := manager.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE task_runs SET status = ?, progress = 0, error_code = '', error_message = '', cancel_requested_at = NULL, completed_at = NULL, updated_at = ? WHERE id = ?`, StatusQueued, now, record.ID); err != nil {
		return Task{}, taskConflict(err)
	}
	if err := appendTaskEventTx(ctx, tx, record.ID, "retry_requested", map[string]any{"project_uuid": projectUUID, "task_uuid": taskUUID, "status": StatusQueued}, now); err != nil {
		return Task{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_runs SET status = ?, error_code = '', error_message = '', completed_at = NULL, updated_at = ? WHERE task_run_id = ?`, StatusQueued, now, record.ID); err != nil {
		return Task{}, err
	}
	if record.Kind == KindComicStoryboardGeneration {
		if err := queueComicStoryboardWorkflowTx(ctx, tx, record.UUID, now); err != nil {
			return Task{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Task{}, taskConflict(err)
	}
	task, err := manager.GetTask(ctx, projectUUID, taskUUID)
	if err == nil {
		runtime.broadcast("task:queued", task)
		if record.Kind == KindComicStoryboardGeneration {
			runtime.broadcastComicStoryboardWorkflow("workflow:queued", record.UUID)
		}
	}
	return task, err
}

func waitForStoryTaskRetryBoundary(ctx context.Context, runtime *projectRuntime, riverJobID int64) error {
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		job, err := runtime.client.JobGet(ctx, riverJobID)
		if err != nil {
			return err
		}
		if job.State != rivertype.JobStateRunning {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return taskError(CodeTaskStateConflict, "上一轮任务仍在结束", "请稍后再次重试。", nil)
		case <-ticker.C:
		}
	}
}

func getTaskRecord(ctx context.Context, db *gorm.DB, projectID int64, taskUUID string) (taskRecord, error) {
	if !isUUIDv7(taskUUID) {
		return taskRecord{}, taskError(CodeInvalidTask, "任务 UUID 无效", "任务资源标识必须是 UUIDv7。", nil)
	}
	var record taskRecord
	err := db.WithContext(ctx).Where("project_id = ? AND uuid = ?", projectID, taskUUID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return taskRecord{}, taskError(CodeTaskNotFound, "任务不存在", "该任务可能已经清理。", err)
	}
	return record, err
}

func findTaskTx(ctx context.Context, tx *sql.Tx, projectID int64, kind, idempotencyKey string) (taskRecord, bool, error) {
	row := tx.QueryRowContext(ctx, taskSelectSQL+" WHERE project_id = ? AND kind = ? AND idempotency_key = ? LIMIT 1", projectID, kind, idempotencyKey)
	return scanTask(row)
}

func findActiveResourceTaskTx(ctx context.Context, tx *sql.Tx, projectID int64, resourceUUID string) (taskRecord, bool, error) {
	row := tx.QueryRowContext(ctx, taskSelectSQL+" WHERE project_id = ? AND kind = ? AND resource_uuid = ? AND status IN ('queued','running','waiting_for_input') LIMIT 1", projectID, KindStoryChapterGeneration, resourceUUID)
	return scanTask(row)
}

func findTaskByUUIDTx(ctx context.Context, tx *sql.Tx, projectID int64, taskUUID string) (taskRecord, bool, error) {
	if !isUUIDv7(taskUUID) {
		return taskRecord{}, false, taskError(CodeInvalidTask, "任务 UUID 无效", "任务资源标识必须是 UUIDv7。", nil)
	}
	return scanTask(tx.QueryRowContext(ctx, taskSelectSQL+" WHERE project_id = ? AND uuid = ? LIMIT 1", projectID, taskUUID))
}

const taskSelectSQL = `SELECT id, uuid, project_id, river_job_id, kind, resource_uuid, input_version, input_snapshot, status, idempotency_key, retryable, provider_uuid, model, model_source, progress, attempt, max_attempts, error_code, error_message, cancel_requested_at, started_at, completed_at, created_at, updated_at FROM task_runs`

type rowScanner interface{ Scan(...any) error }

func scanTask(row rowScanner) (taskRecord, bool, error) {
	var record taskRecord
	err := row.Scan(&record.ID, &record.UUID, &record.ProjectID, &record.RiverJobID, &record.Kind, &record.ResourceUUID, &record.InputVersion, &record.InputSnapshot, &record.Status, &record.IdempotencyKey, &record.Retryable, &record.ProviderUUID, &record.Model, &record.ModelSource, &record.Progress, &record.Attempt, &record.MaxAttempts, &record.ErrorCode, &record.ErrorMessage, &record.CancelRequestedAt, &record.StartedAt, &record.CompletedAt, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return taskRecord{}, false, nil
	}
	return record, err == nil, err
}

func appendTaskEventTx(ctx context.Context, tx *sql.Tx, taskID int64, eventType string, payload any, now time.Time) error {
	uuid, err := newUUIDv7()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO task_events (uuid, task_run_id, sequence, event_type, payload, created_at) SELECT ?, ?, COALESCE(MAX(sequence), 0) + 1, ?, ?, ? FROM task_events WHERE task_run_id = ?`, uuid, taskID, eventType, string(encoded), now, taskID)
	return err
}

func createAgentAuditTx(ctx context.Context, tx *sql.Tx, projectID, taskID int64, taskUUID, chapterUUID, providerUUID, model, inputSummary string, now time.Time) (int64, int64, error) {
	threadUUID, err := newUUIDv7()
	if err != nil {
		return 0, 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_threads (uuid, project_id, kind, subject_type, subject_uuid, status, provider_uuid, model, created_at, updated_at) VALUES (?, ?, 'story_generation', 'chapter', ?, 'idle', ?, ?, ?, ?) ON CONFLICT(project_id, kind, subject_type, subject_uuid) DO UPDATE SET provider_uuid = excluded.provider_uuid, model = excluded.model, updated_at = excluded.updated_at`, threadUUID, projectID, chapterUUID, providerUUID, model, now, now)
	if err != nil {
		return 0, 0, err
	}
	var threadID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM agent_threads WHERE project_id = ? AND kind = 'story_generation' AND subject_type = 'chapter' AND subject_uuid = ?`, projectID, chapterUUID).Scan(&threadID); err != nil {
		return 0, 0, err
	}
	runUUID, err := newUUIDv7()
	if err != nil {
		return 0, 0, err
	}
	summary := truncateSummary(inputSummary, 500)
	result, err := tx.ExecContext(ctx, `INSERT INTO agent_runs (uuid, agent_thread_id, task_run_id, trigger_type, status, input_summary, created_at, updated_at) VALUES (?, ?, ?, 'job_step', 'queued', ?, ?, ?)`, runUUID, threadID, taskID, summary, now, now)
	if err != nil {
		return 0, 0, err
	}
	runID, err := result.LastInsertId()
	if err != nil {
		return 0, 0, err
	}
	if err := appendAgentEventTx(ctx, tx, threadID, &runID, "run_queued", map[string]any{"task_uuid": taskUUID, "chapter_uuid": chapterUUID, "status": StatusQueued}, now); err != nil {
		return 0, 0, err
	}
	return threadID, runID, nil
}

func appendAgentEventTx(ctx context.Context, tx *sql.Tx, threadID int64, runID *int64, eventType string, payload any, now time.Time) error {
	uuid, err := newUUIDv7()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_events (uuid, agent_thread_id, agent_run_id, sequence, event_type, payload, created_at) SELECT ?, ?, ?, COALESCE(MAX(sequence), 0) + 1, ?, ?, ? FROM agent_events WHERE agent_thread_id = ?`, uuid, threadID, runID, eventType, string(encoded), now, threadID)
	return err
}

func buildStoryChapterPrompt(ctx context.Context, service *story.Service, chapter story.Chapter, promptKey, inputPrompt, template string) (string, error) {
	profile, err := service.GetStoryProfile(ctx)
	if err != nil {
		return "", err
	}
	chapters, err := service.ListChapters(ctx, "active")
	if err != nil {
		return "", err
	}
	if promptKey == "next_story_chapter" {
		var previous any
		for _, item := range chapters {
			if item.SortOrder >= chapter.SortOrder || item.CurrentStory == nil || strings.TrimSpace(item.CurrentStory.Content) == "" {
				continue
			}
			previous = map[string]any{"chapter_code": item.ChapterCode, "title": item.Title, "content": item.CurrentStory.Content, "content_format": item.CurrentStory.ContentFormat}
		}
		previousJSON, marshalErr := json.Marshal(previous)
		if marshalErr != nil {
			return "", marshalErr
		}
		rendered, renderErr := promptcatalog.Render(template, map[string]string{
			"story_md": profile.StoryMD, "previous_chapter_json": string(previousJSON),
			"guidance_prompt": strings.TrimSpace(inputPrompt), "next_chapter_code": chapter.ChapterCode,
		})
		if renderErr != nil {
			return "", renderErr
		}
		return rendered, nil
	}
	chapterPlan, err := json.Marshal(map[string]any{
		"chapter_code": chapter.ChapterCode,
		"title":        chapter.Title,
		"outline":      strings.TrimSpace(inputPrompt),
	})
	if err != nil {
		return "", err
	}
	summaries := make([]map[string]string, 0, len(chapters))
	for _, item := range chapters {
		if item.UUID == chapter.UUID || item.CurrentStory == nil || strings.TrimSpace(item.CurrentStory.Content) == "" {
			continue
		}
		summaries = append(summaries, map[string]string{
			"chapter_code": item.ChapterCode,
			"title":        item.Title,
			"summary":      truncateSummary(item.CurrentStory.Content, 600),
		})
	}
	encodedSummaries, err := json.Marshal(summaries)
	if err != nil {
		return "", err
	}
	rendered, err := promptcatalog.Render(template, map[string]string{
		"input_prompt":             strings.TrimSpace(inputPrompt),
		"story_md":                 strings.TrimSpace(profile.StoryMD),
		"chapter_plan_json":        string(chapterPlan),
		"generated_summaries_json": string(encodedSummaries),
		"chapter_code":             chapter.ChapterCode,
	})
	if err != nil {
		return "", err
	}
	return rendered, nil
}

func validStatus(value string) bool {
	switch value {
	case StatusQueued, StatusRunning, StatusWaitingForInput, StatusCompleted, StatusFailed, StatusCancelled, StatusInterrupted:
		return true
	}
	return false
}

func taskConflict(err error) error {
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint failed") {
		return taskError(CodeTaskConflict, "资源已有进行中的生成任务", "请等待现有任务结束后再重试。", err)
	}
	return err
}

func truncateSummary(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func newUUIDv7() (string, error) {
	value, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate UUIDv7: %w", err)
	}
	return value.String(), nil
}

func isUUIDv7(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.Version() == 7
}

var _ = project.Project{}
var _ = provider.Provider{}
