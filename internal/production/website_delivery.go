package production

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"lumi/internal/project"
)

type WebsiteArchive struct {
	UUID     string `json:"uuid"`
	Format   string `json:"format"`
	Path     string `json:"path"`
	ByteSize int64  `json:"byte_size"`
	SHA256   string `json:"sha256"`
}

type WebsiteDelivery struct {
	IdempotencyKey  string          `json:"idempotency_key"`
	TaskUUID        string          `json:"task_uuid"`
	GenerationUUID  string          `json:"generation_uuid"`
	Done            bool            `json:"done"`
	OutputDirectory string          `json:"output_directory,omitempty"`
	PreviewURL      string          `json:"preview_url,omitempty"`
	Archive         *WebsiteArchive `json:"archive,omitempty"`
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
func (s *Service) WebsiteDelivery(ctx context.Context, taskUUID, endpoint string, archive bool) (WebsiteDelivery, error) {
	result := WebsiteDelivery{TaskUUID: taskUUID}
	err := s.store.WithFileCommit(func() error {
		db, err := s.store.DB().DB()
		if err != nil {
			return err
		}
		var manifestJSON string
		err = db.QueryRowContext(ctx, `SELECT g.uuid,g.manifest_json FROM website_generations g
            JOIN project_websites w ON w.generation_id=g.id AND w.project_id=g.project_id
            JOIN production_task_runs t ON t.id=g.task_id
            WHERE g.project_id=(SELECT id FROM projects WHERE uuid=?) AND t.uuid=? AND t.status='completed' AND g.completed_at IS NOT NULL`, s.store.ProjectUUID(), taskUUID).Scan(&result.GenerationUUID, &manifestJSON)
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
		if archive {
			result.Archive, err = s.archiveWebsite(ctx, root, result.GenerationUUID, m)
			return err
		}
		if endpoint == "" || !websiteFilesPresent(root, m) {
			return domainError(CodeExportUnavailable, "本地网站预览不可用", "请检查网站文件及 CLI 实例。", nil)
		}
		result.OutputDirectory = root
		result.PreviewURL = strings.TrimRight(endpoint, "/") + "/media/projects/" + s.store.ProjectUUID() + "/website/index.html"
		return nil
	})
	result.Done = err == nil
	return result, err
}

func (s *Service) archiveWebsite(ctx context.Context, root, generationUUID string, manifest websiteManifest) (*WebsiteArchive, error) {
	dest, err := s.store.ResolvePath(".lumi/exports/website-" + generationUUID + ".zip")
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	part, err := os.CreateTemp(dir, ".website-*.zip")
	if err != nil {
		return nil, err
	}
	defer os.Remove(part.Name())
	defer part.Close()
	sum := sha256.New()
	zw := zip.NewWriter(io.MultiWriter(part, sum))
	for _, name := range slices.Sorted(maps.Keys(manifest.Files)) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") {
			return nil, fmt.Errorf("invalid website manifest path")
		}
		// Only manifest entries, in stable order. Verify the exact bytes put
		// into the ZIP; a user edit during delivery cannot slip through.
		path, err := project.ResolveRelativePath(root, name)
		if err != nil {
			return nil, err
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			return nil, err
		}
		if err = copyWebsiteFile(w, path, manifest.Files[name]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	if err := part.Sync(); err != nil {
		return nil, err
	}
	stat, err := part.Stat()
	if err != nil {
		return nil, err
	}
	if err := part.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(part.Name(), dest); err != nil {
		return nil, err
	}
	return &WebsiteArchive{UUID: generationUUID, Format: "zip", Path: dest, ByteSize: stat.Size(), SHA256: fmt.Sprintf("%x", sum.Sum(nil))}, nil
}

func copyWebsiteFile(w io.Writer, path string, meta websiteFile) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() || stat.Size() != meta.Size {
		return fmt.Errorf("website manifest size mismatch")
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(f, meta.Size+1))
	if err != nil {
		return err
	}
	if n != meta.Size || fmt.Sprintf("%x", h.Sum(nil)) != meta.SHA256 {
		return domainError(CodeExportChanged, "网站文件与本次生成清单不符", "未交付 ZIP；请检查改动并重新生成。", nil)
	}
	return nil
}
