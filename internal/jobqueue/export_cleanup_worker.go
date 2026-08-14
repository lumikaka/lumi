package jobqueue

import (
	"context"
	"time"

	"lumi/internal/production"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const comicExportCleanupPeriod = time.Hour

func comicExportCleanupInsert(projectUUID string) (river.JobArgs, *river.InsertOpts) {
	return exportCleanupArgs{Version: 1, ProjectUUID: projectUUID}, &river.InsertOpts{
		Queue: QueueAssetMaintenance, MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
			rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning,
			rivertype.JobStateRetryable, rivertype.JobStateScheduled,
		}},
	}
}

func comicExportCleanupPeriodicOptions() river.PeriodicJobOpts {
	return river.PeriodicJobOpts{ID: "comic_export_cleanup_v1", RunOnStart: true}
}

func comicExportCleanupPeriodicJob(projectUUID string) *river.PeriodicJob {
	options := comicExportCleanupPeriodicOptions()
	return river.NewPeriodicJob(river.PeriodicInterval(comicExportCleanupPeriod), func() (river.JobArgs, *river.InsertOpts) {
		return comicExportCleanupInsert(projectUUID)
	}, &options)
}

type comicExportCleanupWorker struct {
	river.WorkerDefaults[exportCleanupArgs]
	runtime *projectRuntime
}

func (worker *comicExportCleanupWorker) Work(ctx context.Context, job *river.Job[exportCleanupArgs]) error {
	if job.Args.Version != 1 || job.Args.ProjectUUID != worker.runtime.projectUUID || !isUUIDv7(job.Args.ProjectUUID) {
		return river.JobCancel(taskError(CodeInvalidTask, "导出清理任务参数无效", "项目 UUID 或参数版本不受支持。", nil))
	}
	service := production.NewService(worker.runtime.store, worker.runtime.manager.hub)
	_, err := service.CleanupExpiredExports(ctx, 1000)
	return err
}
