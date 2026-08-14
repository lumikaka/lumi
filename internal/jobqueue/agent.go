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
		err := worker.service.ExecuteJob(workCtx, worker.runtime.store, agent.JobSpec{Version: job.Args.Version, ProjectUUID: job.Args.ProjectUUID, JobKind: job.Args.JobKind, ResourceUUID: job.Args.ResourceUUID, ThreadUUID: job.Args.ThreadUUID})
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
		if errors.Is(err, agent.ErrWaitingInput) {
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
	inserted, err := runtime.client.InsertTx(ctx, tx, agentArgs{Version: spec.Version, ProjectUUID: projectUUID, JobKind: spec.JobKind, ResourceUUID: spec.ResourceUUID, ThreadUUID: spec.ThreadUUID}, &river.InsertOpts{
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
		task, err := manager.CreateChapterGeneration(ctx, projectUUID, CreateGenerationInput{ChapterUUID: request.ResourceUUID, ProviderUUID: request.ProviderUUID, Model: request.Model, Prompt: request.Prompt, IdempotencyKey: request.IdempotencyKey})
		return storyDomainTask(task), err
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
	default:
		return agent.DomainTask{}, taskError(CodeInvalidTask, "Domain task 不在 allowlist", "Agent 只能启动已注册的生成任务。", nil)
	}
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
	return kind == KindStoryChapterGeneration || kind == KindStoryChapterBatchPlan || kind == KindComicStoryboardGeneration
}

func storyDomainTask(task Task) agent.DomainTask {
	return agent.DomainTask{UUID: task.UUID, Kind: task.Kind, ResourceUUID: task.ResourceUUID, Status: task.Status, ErrorCode: task.ErrorCode, ErrorMessage: task.ErrorMessage}
}

func productionDomainTask(task ProductionTask) agent.DomainTask {
	return agent.DomainTask{UUID: task.UUID, Kind: task.Kind, ResourceUUID: task.ResourceUUID, Status: task.Status, ErrorCode: task.ErrorCode, ErrorMessage: task.ErrorMessage}
}
