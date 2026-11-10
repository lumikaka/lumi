package production

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"lumi/web/websiteprompt"
)

type WebsiteDelivery struct {
	IdempotencyKey  string `json:"idempotency_key"`
	TaskUUID        string `json:"task_uuid"`
	GenerationUUID  string `json:"generation_uuid"`
	Done            bool   `json:"done"`
	OutputDirectory string `json:"output_directory,omitempty"`
	PreviewURL      string `json:"preview_url,omitempty"`
	PublishPrompt   string `json:"publish_prompt,omitempty"`
}

// ObserveWebsiteResult releases only this invocation's anonymous pending slot.
func (s *Service) ObserveWebsiteResult(ctx context.Context, taskUUID string) error {
	return s.store.DB().WithContext(ctx).Exec(`UPDATE website_generations SET plugin_result_observed=1
        WHERE task_id=(SELECT id FROM production_task_runs WHERE uuid=?) AND plugin_request_scope IS NOT NULL AND plugin_result_observed=0`, taskUUID).Error
}

// ReplayableWebsite is only for unkeyed retries after a terminal response may
// have been lost. Check the most recent attempted generation, not just the
// last success, before returning the exact durable task identity.
func (s *Service) ReplayableWebsite(ctx context.Context, scope, hash string) (string, error) {
	var uuid string
	err := s.store.WithFileCommit(func() error {
		var row struct{ TaskUUID, ManifestJSON string }
		err := s.store.DB().WithContext(ctx).Raw(`SELECT t.uuid AS task_uuid,g.manifest_json
            FROM website_generations g JOIN production_task_runs t ON t.id=g.task_id
            JOIN project_websites w ON w.generation_id=g.id AND w.project_id=g.project_id
            WHERE g.id=(SELECT MAX(id) FROM website_generations WHERE project_id=g.project_id)
              AND g.project_id=(SELECT id FROM projects WHERE uuid=?)
              AND g.plugin_request_scope=? AND g.plugin_result_observed=1 AND g.snapshot_hash=?
              AND t.status='completed' AND g.completed_at IS NOT NULL`, s.store.ProjectUUID(), scope, hash).Scan(&row).Error
		if err != nil || row.TaskUUID == "" {
			return err
		}
		root, err := s.store.ResolvePath("website")
		if err != nil {
			return err
		}
		var manifest websiteManifest
		if json.Unmarshal([]byte(row.ManifestJSON), &manifest) == nil && websiteValidate(root, manifest) == nil {
			uuid = row.TaskUUID
		}
		return ctx.Err()
	})
	return uuid, err
}

// Resolve the task, current publication and managed bytes under the same
// lock as the directory swap. Never substitute a previous or newer website.
func (s *Service) WebsiteDelivery(ctx context.Context, taskUUID, endpoint string) (WebsiteDelivery, error) {
	result := WebsiteDelivery{TaskUUID: taskUUID}
	err := s.store.WithFileCommit(func() error {
		db, err := s.store.DB().DB()
		if err != nil {
			return err
		}
		var manifestJSON, language string
		err = db.QueryRowContext(ctx, `SELECT g.uuid,g.manifest_json,p.generation_language FROM website_generations g
            JOIN projects p ON p.id=g.project_id
            JOIN project_websites w ON w.generation_id=g.id AND w.project_id=g.project_id
            JOIN production_task_runs t ON t.id=g.task_id
            WHERE p.uuid=? AND t.uuid=? AND t.status='completed' AND g.completed_at IS NOT NULL`, s.store.ProjectUUID(), taskUUID).Scan(&result.GenerationUUID, &manifestJSON, &language)
		if errors.Is(err, sql.ErrNoRows) {
			return domainError(CodeExportChanged, "本次任务不是当前成功发布的网站", "未交付其他版本。需要当前内容时重新生成。", nil)
		}
		if err != nil {
			return err
		}
		var m websiteManifest
		if err = json.Unmarshal([]byte(manifestJSON), &m); err != nil {
			return err
		}
		root, err := s.store.ResolvePath("website")
		if err != nil {
			return err
		}
		if _, ok := m.Files["index.html"]; !ok {
			return domainError(CodeExportUnavailable, "网站清单缺少首页", "请重新生成网站。", nil)
		}
		if endpoint == "" || !websiteFilesPresent(root, m) {
			return domainError(CodeExportUnavailable, "本地网站预览不可用", "请检查网站文件及 CLI 实例。", nil)
		}
		result.OutputDirectory = root
		result.PreviewURL = strings.TrimRight(endpoint, "/") + "/media/projects/" + s.store.ProjectUUID() + "/website/index.html"
		result.PublishPrompt = websiteprompt.Render(language, root)
		return nil
	})
	result.Done = err == nil
	return result, err
}
