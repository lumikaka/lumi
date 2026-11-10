package textmcp

import (
	"context"
	"fmt"
	"time"

	"lumi/internal/httpapi"
	"lumi/internal/jobqueue"
	"lumi/internal/production"
	"lumi/internal/project"
)

// Leave time for the stdio relay and Cindy's 25s transport to deliver a
// continuation. The next call resumes the SQLite task even after reconnect.
const websiteWait = 8 * time.Second

func (s *Service) exportWebsite(ctx context.Context, store *project.Store, prod *production.Service, a arguments) (any, error) {
	if s.WebsiteTasks == nil {
		return nil, httpapi.NewError(503, "website_generation_unavailable", "当前 CLI 实例不能生成网站", "使用随包 CLI 重启共享服务。", nil)
	}
	host := HostFromContext(ctx)
	task, err := s.WebsiteTasks.CreatePluginWebsiteGeneration(ctx, store.ProjectUUID(), host, a.AllowMissing, a.IdempotencyKey)
	if err != nil {
		return nil, httpapi.ProductionAPIError(err)
	}
	taskUUID := task.UUID
	key := jobqueue.PluginWebsiteKey(task, host, a.AllowMissing)
	current, err := s.waitProductionTask(ctx, store.ProjectUUID(), publicTask(task), websiteWait, func(ctx context.Context, projectUUID, uuid string) (HostTask, error) {
		got, err := s.WebsiteTasks.GetProductionTask(ctx, projectUUID, uuid)
		if err != nil {
			return HostTask{}, httpapi.ProductionAPIError(err)
		}
		return publicTask(got), nil
	})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !terminalTaskStatus(current.Status) {
		return map[string]any{"done": false, "task_uuid": taskUUID, "status": current.Status, "idempotency_key": key, "next_action": "保持缺图参数，使用返回的 idempotency_key 再次调用 export_website 继续本次任务；尚不能交付。"}, nil
	}
	var result any
	if current.Status != "completed" {
		code := current.ErrorCode
		if code == "" {
			code = "website_generation_failed"
		}
		err = httpapi.NewError(409, code, "本次网站生成未完成", fmt.Sprintf("任务 %s：%s。%s", taskUUID, current.Status, current.ErrorMessage), nil)
	} else {
		delivery, deliveryErr := prod.WebsiteDelivery(ctx, taskUUID, s.Endpoint, host == HostCindy)
		delivery.IdempotencyKey = key
		result, err = delivery, deliveryErr
	}
	// A cancelled/timed-out caller leaves its durable pending association
	// intact. Only release this invocation, never a later one in its scope.
	if ctx.Err() == nil {
		if clearErr := prod.ObserveWebsiteResult(ctx, taskUUID); clearErr != nil {
			return nil, clearErr
		}
	}
	return result, err
}

func publicTask(t jobqueue.ProductionTask) HostTask {
	return HostTask{UUID: t.UUID, Status: t.Status, Progress: t.Progress, ErrorCode: t.ErrorCode, ErrorMessage: t.ErrorMessage}
}
