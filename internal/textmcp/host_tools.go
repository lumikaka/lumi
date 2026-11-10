package textmcp

import (
	"context"
	"strings"
	"time"

	"lumi/internal/httpapi"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/story"
)

// PageImageRequest asks the CLI instance to create the existing comic image
// production task for a host page. HostPagePrompt is the exact prompt that
// prepare_page_image renders; the task is refused when its snapshot differs.
type PageImageRequest struct {
	ProjectUUID, ChapterUUID, PageUUID string
	SettingUUIDs                       []string
	AdditionalDirection, Model         string
	HostPagePrompt, IdempotencyKey     string
}

// HostTask is the public task state shared by host waiters; no internal ID or snapshot.
type HostTask struct {
	UUID         string `json:"task_uuid"`
	ResourceUUID string `json:"-"`
	Status       string `json:"status"`
	Stage        string `json:"stage,omitempty"`
	Progress     int    `json:"progress"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type PageImageTask = HostTask

// PageImageTasks is implemented in the CLI instance by the shared production
// task manager. textmcp itself never calls a provider.
type PageImageTasks interface {
	CreatePageImage(context.Context, PageImageRequest) (PageImageTask, error)
	FindPageImage(ctx context.Context, projectUUID, key string) (PageImageTask, bool, error)
	PageImage(ctx context.Context, projectUUID, taskUUID string) (PageImageTask, error)
}

// PageImageWait bounds how long one generate_page_image call waits for the
// task. lumi mcp extends its transport timeout by this amount, so a slow
// provider returns a running status instead of a transport error.
var PageImageWait = 2 * time.Minute

const pageImageKeyPrefix = "mcp-host-page-image:"

func terminalTaskStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled", "interrupted":
		return true
	}
	return false
}

// generatePageImage validates inside a project lease, then waits for the task
// outside it so the worker is never blocked by this call.
func (s *Service) generatePageImage(ctx context.Context, withStore func(context.Context, string, func(*project.Store) error) error, guard func(*project.Store) error, projectUUID string, a arguments) httpapi.Envelope {
	if s.PageImages == nil {
		return Failure("page_image_generation_unavailable", "当前 CLI 实例未提供服务端生图", "使用与插件同版本的 lumi CLI 重启共享后台后重试。")
	}
	key := pageImageKeyPrefix + strings.TrimSpace(a.IdempotencyKey)
	var task PageImageTask
	err := withStore(ctx, projectUUID, func(store *project.Store) error {
		if err := guard(store); err != nil {
			return err
		}
		if err := store.RequireReady(); err != nil {
			return err
		}
		if err := store.RequireInitializationAvailable(ctx); err != nil {
			return err
		}
		existing, found, err := s.PageImages.FindPageImage(ctx, projectUUID, key)
		if err != nil {
			return err
		}
		if found {
			// A replay never re-validates: a completed task already advanced
			// the page revision. It must target the same page.
			if existing.ResourceUUID != a.PageUUID {
				return httpapi.NewError(409, "idempotency_key_conflict", "idempotency_key 已用于其他页面", "同一 key 只能重放同一次生图；新的生图使用新的 key。", nil)
			}
			task = existing
			return nil
		}
		prod := production.NewService(store, s.Events)
		prepared, err := prod.ValidatePageImageInputs(ctx, production.PreparePageImageInput{PageUUID: a.PageUUID, SettingUUIDs: value(a.ImageSettings), AdditionalDirection: a.AdditionalDirection, SettingReviews: a.SettingReviews})
		if err != nil {
			return err
		}
		if value(a.ExpectedRevision) != prepared.ImageWriteBack.ExpectedRevision {
			return httpapi.NewError(409, production.CodeConflict, "页面已被修改", "页面 revision 已变化（可能在 Lumi Web 中被编辑）。重新 image_target 读取最新内容与 revision，必要时重新核对设定后再生图；不要只替换 revision。", nil)
		}
		task, err = s.PageImages.CreatePageImage(ctx, PageImageRequest{ProjectUUID: projectUUID, ChapterUUID: prepared.Target.ChapterUUID(), PageUUID: a.PageUUID, SettingUUIDs: value(a.ImageSettings), AdditionalDirection: a.AdditionalDirection, Model: a.Model, HostPagePrompt: prepared.GenerationPrompt, IdempotencyKey: key})
		return err
	})
	if err != nil {
		return publicError(err)
	}
	task, err = s.waitProductionTask(ctx, projectUUID, task, PageImageWait, s.PageImages.PageImage)
	if err != nil {
		return publicError(err)
	}
	switch task.Status {
	case "completed":
	case "failed", "cancelled", "interrupted":
		code := task.ErrorCode
		if code == "" {
			code = "page_image_generation_failed"
		}
		message := "页面生图任务未完成"
		if task.ErrorMessage != "" {
			message += "：" + task.ErrorMessage
		}
		return Failure(code, message, "任务 "+task.UUID+" 状态为 "+task.Status+"，页面图片未改变。同一 idempotency_key 只会返回这次结果；修正原因（服务商配置、额度或限流）后，用新的 idempotency_key 重新 image_target 并生图。")
	default:
		return httpapi.Envelope{Success: true, Data: map[string]any{"task": task, "page_uuid": a.PageUUID, "done": false,
			"next_action": "生图仍在进行。用完全相同的参数和 idempotency_key 再次调用 generate_page_image 继续等待；不要换新 key（会重复生成并计费）。"}}
	}
	var data map[string]any
	err = withStore(ctx, projectUUID, func(store *project.Store) error {
		if err := guard(store); err != nil {
			return err
		}
		prod := production.NewService(store, s.Events)
		target, err := prod.HostImageTarget(ctx, a.PageUUID, "page_image")
		if err != nil {
			return err
		}
		page, err := prod.GetSection(ctx, target.ChapterUUID(), a.PageUUID)
		if err != nil {
			return err
		}
		image, found, err := prod.SectionImageByTask(ctx, a.PageUUID, task.UUID)
		if err != nil {
			return err
		}
		if !found {
			return httpapi.NewError(409, "page_image_missing", "任务已完成但未找到生成的页面图片", "重新 pages get 读取页面当前图片。", nil)
		}
		current := page.CurrentImage != nil && page.CurrentImage.UUID == image.VariantUUID
		data = map[string]any{"task": task, "done": true, "page_uuid": page.UUID, "chapter_uuid": page.ChapterUUID, "revision": page.Revision,
			"image": image, "is_current_image": current, "read_image": map[string]any{"asset_uuid": image.AssetUUID},
			"next_action": "read_image 查看 asset_uuid，对照已保存分镜与用户文本核对后，用 pages record_image_review 记录结论（使用返回的 variant_uuid 与 revision）。"}
		return nil
	})
	if err != nil {
		return publicError(err)
	}
	return httpapi.Envelope{Success: true, Data: data}
}

// waitProductionTask blocks on task change hints from the in-process realtime hub
// and rereads SQLite on each hint; there is no timed polling.
func (s *Service) waitProductionTask(ctx context.Context, projectUUID string, task HostTask, wait time.Duration, read func(context.Context, string, string) (HostTask, error)) (HostTask, error) {
	if terminalTaskStatus(task.Status) {
		return task, nil
	}
	uuid := task.UUID
	signal := make(chan struct{}, 1)
	stop := s.Events.Observe("project:"+projectUUID, func(event string, payload any) {
		if !strings.HasPrefix(event, "production_task:") {
			return
		}
		if fields, ok := payload.(map[string]any); ok && fields["task_uuid"] != uuid {
			return
		}
		select {
		case signal <- struct{}{}:
		default:
		}
	})
	defer stop()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		// Reread after subscribing so a transition before Observe is not missed.
		current, err := read(ctx, projectUUID, uuid)
		if err != nil {
			return task, err
		}
		task = current
		if terminalTaskStatus(task.Status) {
			return task, nil
		}
		select {
		case <-signal:
		case <-timer.C:
			return task, nil
		case <-ctx.Done():
			return task, nil
		}
	}
}

// webURL returns a link to an existing Web route. page_uuid is the Web
// section UUID and setting_uuid the premise asset UUID.
func (s *Service) webURL(ctx context.Context, store *project.Store, st *story.Service, prod *production.Service, a arguments) (any, error) {
	if strings.TrimSpace(s.Endpoint) == "" {
		return nil, httpapi.NewError(503, "web_unavailable", "CLI 实例未提供 Web 地址", "执行 lumi open --json 查看地址。", nil)
	}
	scopes := 0
	for _, set := range []bool{a.PageUUID != "", a.SettingUUID != "", a.ReviewView != ""} {
		if set {
			scopes++
		}
	}
	if scopes > 1 || (a.ChapterUUID != "" && (a.SettingUUID != "" || a.ReviewView != "")) {
		return nil, httpapi.NewError(400, "invalid_arguments", "链接范围冲突", "只能指定一个范围：page_uuid（可带 chapter_uuid）、chapter_uuid、setting_uuid 或 view。", nil)
	}
	base := s.Endpoint + "/projects/" + store.ProjectUUID()
	result := map[string]any{"project_uuid": store.ProjectUUID(), "url": base + "/", "scope": "book"}
	switch {
	case a.PageUUID != "":
		target, err := prod.HostImageTarget(ctx, a.PageUUID, "page_image")
		if err != nil {
			return nil, err
		}
		if a.ChapterUUID != "" && a.ChapterUUID != target.ChapterUUID() {
			return nil, httpapi.NewError(400, "invalid_arguments", "页面不属于该章节", "省略 chapter_uuid 或使用 book_outline 返回的页面所属章节。", nil)
		}
		result["chapter_uuid"], result["page_uuid"] = target.ChapterUUID(), a.PageUUID
		result["url"], result["scope"] = base+"/chapters/"+target.ChapterUUID()+"/sections/"+a.PageUUID, "page"
	case a.ChapterUUID != "":
		chapter, err := st.GetChapter(ctx, a.ChapterUUID)
		if err != nil {
			return nil, err
		}
		if chapter.TrashedAt != nil {
			return nil, httpapi.NewError(400, "invalid_arguments", "指定章节已删除", "", nil)
		}
		result["chapter_uuid"], result["url"], result["scope"] = chapter.UUID, base+"/chapters/"+chapter.UUID, "chapter"
	case a.SettingUUID != "":
		setting, err := prod.GetPremiseAsset(ctx, a.SettingUUID)
		if err != nil {
			return nil, err
		}
		if setting.DeletedAt != nil {
			return nil, httpapi.NewError(400, "invalid_arguments", "指定设定已删除", "", nil)
		}
		result["setting_uuid"], result["url"], result["scope"] = setting.UUID, base+"/premise/assets/"+setting.UUID, "setting"
	case a.ReviewView == "chapters":
		result["url"], result["scope"] = base+"/chapters", "chapters"
	case a.ReviewView == "settings":
		result["url"], result["scope"] = base+"/premise", "settings"
	case a.ReviewView == "exports":
		result["url"], result["scope"] = base+"/exports", "exports"
	case a.ReviewView == "providers":
		result["url"], result["scope"] = s.Endpoint+"/settings/providers", "providers"
	}
	result["note"] = "地址来自当前运行的 CLI 后台，重启后端口可能变化；每次交付前重新调用 web_url，不要写入文件。"
	return result, nil
}
