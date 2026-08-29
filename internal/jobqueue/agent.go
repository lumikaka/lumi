package jobqueue

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"lumi/internal/agent"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type agentArgs struct {
	Version      int    `json:"version"`
	ProjectUUID  string `json:"project_uuid" river:"unique"`
	JobKind      string `json:"job_kind" river:"unique"`
	ResourceUUID string `json:"resource_uuid" river:"unique"`
	ThreadUUID   string `json:"thread_uuid" river:"unique"`
	WakeupUUID   string `json:"wakeup_uuid,omitempty" river:"unique"`
}

func (agentArgs) Kind() string { return "lumi_project_agent_v1" }

type agentWorker struct {
	river.WorkerDefaults[agentArgs]
	runtime *projectRuntime
	service *agent.Service
}

func (worker *agentWorker) Work(ctx context.Context, job *river.Job[agentArgs]) error {
	if worker.service == nil || job.Args.Version != 1 || job.Args.ProjectUUID != worker.runtime.projectUUID || !isUUIDv7(job.Args.ResourceUUID) || !isUUIDv7(job.Args.ThreadUUID) {
		return river.JobCancel(taskError(CodeInvalidTask, "Agent River job 参数无效", "Job 只能引用当前项目公开 UUIDv7。", nil))
	}
	workCtx, cancel := context.WithCancel(ctx)
	worker.runtime.registerWork(job.Args.ResourceUUID, cancel)
	defer func() {
		cancel()
		worker.runtime.unregisterWork(job.Args.ResourceUUID)
	}()
	for {
		err := worker.service.ExecuteJob(workCtx, worker.runtime.store, agent.JobSpec{Version: job.Args.Version, ProjectUUID: job.Args.ProjectUUID, JobKind: job.Args.JobKind, ResourceUUID: job.Args.ResourceUUID, ThreadUUID: job.Args.ThreadUUID, WakeupUUID: job.Args.WakeupUUID})
		if errors.Is(err, agent.ErrJobNotReady) {
			// River promotes snoozed jobs on its maintenance cadence, which is too
			// coarse for an Agent polling a task running on another queue. Keep the
			// single Agent worker claimed and poll the persisted task state instead.
			select {
			case <-workCtx.Done():
				return workCtx.Err()
			case <-time.After(300 * time.Millisecond):
				continue
			}
		}
		if errors.Is(err, agent.ErrWaitingInput) || errors.Is(err, agent.ErrWaitingWorkflow) {
			return nil
		}
		if err == nil {
			return nil
		}
		var agentErr *agent.Error
		if errors.As(err, &agentErr) && !agentErr.Retryable {
			return river.JobCancel(err)
		}
		return err
	}
}

func (manager *Manager) EnqueueAgentTx(ctx context.Context, projectUUID string, tx *sql.Tx, spec agent.JobSpec) (int64, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return 0, err
	}
	inserted, err := runtime.client.InsertTx(ctx, tx, agentArgs{Version: spec.Version, ProjectUUID: projectUUID, JobKind: spec.JobKind, ResourceUUID: spec.ResourceUUID, ThreadUUID: spec.ThreadUUID, WakeupUUID: spec.WakeupUUID}, &river.InsertOpts{
		Queue: QueueAgent, MaxAttempts: 5,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateRetryable, rivertype.JobStateScheduled}},
	})
	if err != nil {
		return 0, err
	}
	if inserted.Job == nil {
		return 0, taskError(CodeTaskPersistenceFailed, "无法读取 Agent River job", "队列插入未返回 job。", nil)
	}
	return inserted.Job.ID, nil
}

func (manager *Manager) CancelAgentJob(ctx context.Context, projectUUID string, jobID int64) error {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return err
	}
	_, err = runtime.client.JobCancel(ctx, jobID)
	return err
}

func (manager *Manager) CancelAgentWork(projectUUID, workUUID string) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err == nil {
		runtime.cancelWork(workUUID)
	}
}

func (manager *Manager) StartDomainTask(ctx context.Context, projectUUID string, request agent.DomainTaskRequest) (agent.DomainTask, error) {
	switch request.Kind {
	case KindStoryChapterGeneration:
		task, err := manager.CreateChapterGeneration(ctx, projectUUID, CreateGenerationInput{ChapterUUID: request.ResourceUUID, ProviderUUID: request.ProviderUUID, Model: request.Model, PromptKey: request.PromptKey, Prompt: request.Prompt, IdempotencyKey: request.IdempotencyKey, Invocation: request.Invocation})
		if err != nil {
			return storyDomainTask(task), err
		}
		if request.Invocation.AwaitCompletion {
			return storyDomainTask(task), agent.ErrWaitingWorkflow
		}
		return storyDomainTask(task), nil
	case KindPremiseSettingGeneration:
		task, err := manager.CreatePremiseSettingGeneration(ctx, projectUUID, request.ResourceUUID, CreateProductionGenerationInput{ProviderUUID: request.ProviderUUID, Model: request.Model, Prompt: request.Prompt, IdempotencyKey: request.IdempotencyKey})
		return productionDomainTask(task), err
	case KindPremiseAssetBreakdown:
		task, err := manager.CreatePremiseBreakdown(ctx, projectUUID, request.ResourceUUID, CreateProductionGenerationInput{ProviderUUID: request.ProviderUUID, Model: request.Model, Prompt: request.Prompt, IdempotencyKey: request.IdempotencyKey})
		return productionDomainTask(task), err
	case KindPremiseAssetGeneration:
		task, err := manager.CreatePremiseAssetGeneration(ctx, projectUUID, request.ResourceUUID, CreateProductionGenerationInput{
			ProviderUUID: request.ProviderUUID, Model: request.Model, Prompt: request.Prompt,
			AssetOperation: request.AssetOperation, AssetType: request.AssetType,
			AssetTitle: request.AssetTitle, AssetSummary: request.AssetSummary,
			AssetTags: request.AssetTags, IdempotencyKey: request.IdempotencyKey,
		})
		return productionDomainTask(task), err
	case KindComicImageGeneration:
		task, err := manager.createComicImageGeneration(ctx, projectUUID, request.ChapterUUID, request.ResourceUUID, CreateProductionGenerationInput{ProviderUUID: request.ProviderUUID, Model: request.Model, SelectionProviderUUID: request.SelectionProviderUUID, SelectionModel: request.SelectionModel, Prompt: request.Prompt, PremiseAssetUUIDs: request.PremiseAssetUUIDs, IdempotencyKey: request.IdempotencyKey}, false)
		return productionDomainTask(task), err
	case KindStoryProfileGeneration, KindStoryProfileFromChapters, KindStoryChapterBatchPlan, KindComicStoryboardGeneration:
		var maxSectionCount *int
		if request.MaxSectionCount > 0 {
			maxSectionCount = &request.MaxSectionCount
		}
		task, err := manager.CreateStoryWorkflow(ctx, projectUUID, request.Kind, request.ChapterUUID, CreateStoryWorkflowInput{
			ProviderUUID: request.ProviderUUID, Model: request.Model, Prompt: request.Prompt,
			ChapterCount: request.ChapterCount, MaxSectionCount: maxSectionCount, IdempotencyKey: request.IdempotencyKey,
			Invocation: request.Invocation,
		})
		if err != nil {
			return storyDomainTask(task), err
		}
		if request.Invocation.AwaitCompletion {
			return storyDomainTask(task), agent.ErrWaitingWorkflow
		}
		return storyDomainTask(task), nil
	case KindComicExport:
		operation, err := manager.CreateComicExport(ctx, projectUUID, CreateExportInput{
			Scope: request.Scope, ChapterUUID: request.ChapterUUID, Format: request.Format,
			AllowMissingImages: request.AllowMissingImages, IdempotencyKey: request.IdempotencyKey,
		})
		return productionDomainTask(operation.Task), err
	default:
		return agent.DomainTask{}, taskError(CodeInvalidTask, "Domain task 不在 allowlist", "Agent 只能启动已注册的生成任务。", nil)
	}
}

func (manager *Manager) StartDomainTaskBatch(ctx context.Context, projectUUID string, request agent.DomainTaskBatchRequest) (agent.DomainTaskBatch, error) {
	if request.Kind != KindComicImageGeneration {
		return agent.DomainTaskBatch{}, taskError(CodeInvalidTask, "Domain task batch 不在 allowlist", "Agent 只能批量启动已注册的图片生成任务。", nil)
	}
	batch, err := manager.createComicImageGenerationBatch(ctx, projectUUID, request.ChapterUUID, CreateComicImageGenerationBatchInput{
		SectionUUIDs: request.ResourceUUIDs, IdempotencyKey: request.IdempotencyKey,
	}, false)
	result := agent.DomainTaskBatch{
		ChapterUUID: batch.ChapterUUID, RequestedCount: batch.RequestedCount,
		AcceptedCount: batch.AcceptedCount, Tasks: make([]agent.DomainTask, 0, len(batch.Tasks)),
	}
	for _, task := range batch.Tasks {
		result.Tasks = append(result.Tasks, agent.DomainTask{
			UUID: task.UUID, Kind: task.Kind, ResourceUUID: task.ResourceUUID, Status: task.Status,
			ErrorCode: task.ErrorCode, ErrorMessage: task.ErrorMessage,
		})
	}
	return result, err
}

func (manager *Manager) ListDomainTasks(ctx context.Context, projectUUID, domain, status string, limit int) ([]agent.DomainTask, error) {
	result := []agent.DomainTask{}
	switch domain {
	case "story":
		items, err := manager.ListTasks(ctx, projectUUID, status, limit)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			result = append(result, storyDomainTask(item))
		}
	case "production":
		items, err := manager.ListProductionTasks(ctx, projectUUID, status, limit)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			result = append(result, productionDomainTask(item))
		}
	default:
		return nil, taskError(CodeInvalidTask, "任务领域无效", "domain 只支持 story 或 production。", nil)
	}
	return result, nil
}

func (manager *Manager) ListDomainTaskEvents(ctx context.Context, projectUUID, domain, taskUUID string, before, after int64, limit int) ([]agent.DomainTaskEvent, agent.CursorPagination, error) {
	var items []TaskEvent
	var pagination CursorPagination
	var err error
	switch domain {
	case "story":
		items, pagination, err = manager.ListTaskEvents(ctx, projectUUID, taskUUID, before, after, limit)
	case "production":
		items, pagination, err = manager.ListProductionTaskEvents(ctx, projectUUID, taskUUID, before, after, limit)
	default:
		err = taskError(CodeInvalidTask, "任务领域无效", "domain 只支持 story 或 production。", nil)
	}
	if err != nil {
		return nil, agent.CursorPagination{}, err
	}
	publicItems := make([]agent.DomainTaskEvent, 0, len(items))
	for _, item := range items {
		publicItems = append(publicItems, agent.DomainTaskEvent{UUID: item.UUID, Sequence: item.Sequence, EventType: item.EventType, CreatedAt: item.CreatedAt})
	}
	publicPagination := agent.CursorPagination{PerPage: pagination.PerPage, HasMore: pagination.HasMore}
	if pagination.NextCursor != nil {
		publicPagination.NextCursor = *pagination.NextCursor
	}
	if pagination.PrevCursor != nil {
		publicPagination.PrevCursor = *pagination.PrevCursor
	}
	return publicItems, publicPagination, nil
}

func (manager *Manager) GetDomainTask(ctx context.Context, projectUUID, kind, taskUUID string) (agent.DomainTask, error) {
	if storyDomainTaskKind(kind) {
		task, err := manager.GetTask(ctx, projectUUID, taskUUID)
		return storyDomainTask(task), err
	}
	task, err := manager.GetProductionTask(ctx, projectUUID, taskUUID)
	return productionDomainTask(task), err
}

func (manager *Manager) CancelDomainTask(ctx context.Context, projectUUID, kind, taskUUID string) error {
	if storyDomainTaskKind(kind) {
		_, err := manager.CancelTask(ctx, projectUUID, taskUUID)
		return err
	}
	_, err := manager.CancelProductionTask(ctx, projectUUID, taskUUID)
	return err
}

func (manager *Manager) RetryDomainTask(ctx context.Context, projectUUID, kind, taskUUID string) (agent.DomainTask, error) {
	if storyDomainTaskKind(kind) {
		task, err := manager.RetryTask(ctx, projectUUID, taskUUID)
		return storyDomainTask(task), err
	}
	task, err := manager.RetryProductionTask(ctx, projectUUID, taskUUID)
	return productionDomainTask(task), err
}

func storyDomainTaskKind(kind string) bool {
	return kind == "story" || kind == KindStoryChapterGeneration || kind == KindStoryProfileGeneration || kind == KindStoryProfileFromChapters || kind == KindStoryChapterBatchPlan || kind == KindComicStoryboardGeneration
}

func storyDomainTask(task Task) agent.DomainTask {
	return agent.DomainTask{UUID: task.UUID, Kind: task.Kind, ResourceUUID: task.ResourceUUID, Status: task.Status, ErrorCode: task.ErrorCode, ErrorMessage: task.ErrorMessage}
}

func productionDomainTask(task ProductionTask) agent.DomainTask {
	return agent.DomainTask{UUID: task.UUID, Kind: task.Kind, ResourceUUID: task.ResourceUUID, Status: task.Status, ErrorCode: task.ErrorCode, ErrorMessage: task.ErrorMessage}
}
