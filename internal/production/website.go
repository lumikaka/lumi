package production

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"time"

	"gorm.io/gorm"
)

const WebsiteProcessor = "width1600-jpeg92-white-catmullrom-v1"
const WebsiteWidth = 1600
const websiteTemplateVersion = 2

// WebsiteSnapshot is private queue input. Only the explicit presentation DTO in
// website_render.go is written to the publishable directory.
type WebsiteSnapshot struct {
	Version         int                    `json:"version"`
	TemplateVersion int                    `json:"template_version"`
	Title           string                 `json:"title"`
	Format          string                 `json:"format"`
	Chapters        []WebsiteChapter       `json:"chapters"`
	Missing         []ExportMissingSection `json:"missing"`
	Processor       string                 `json:"processor"`
}
type WebsiteChapter struct {
	UUID  string        `json:"uuid"`
	Code  string        `json:"code"`
	Title string        `json:"title"`
	Pages []WebsitePage `json:"pages"`
}
type WebsitePage struct {
	UUID      string `json:"uuid"`
	Title     string `json:"title"`
	Role      string `json:"role"`
	Number    int    `json:"number"`
	AssetUUID string `json:"asset_uuid"`
	SHA256    string `json:"sha256"`
}
type websiteGeneration struct {
	ID              int64
	UUID            string
	ProjectID       int64
	TaskID          int64
	SnapshotJSON    string
	SnapshotHash    string
	ManifestJSON    string
	ProcessedImages int
	ReusedImages    int
	CreatedAt       time.Time
	CompletedAt     *time.Time
}

func (websiteGeneration) TableName() string { return "website_generations" }

type WebsiteGeneration struct {
	UUID            string     `json:"uuid"`
	TaskUUID        string     `json:"task_uuid"`
	Status          string     `json:"status"`
	Progress        int        `json:"progress"`
	ErrorCode       string     `json:"error_code,omitempty"`
	ProcessedImages int        `json:"processed_images"`
	ReusedImages    int        `json:"reused_images"`
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}
type WebsiteStatus struct {
	Latest           *WebsiteGeneration `json:"latest"`
	LastSuccessfulAt *time.Time         `json:"last_successful_at"`
	HasUpdates       bool               `json:"has_updates"`
	Available        bool               `json:"available"`
	OutputDirectory  string             `json:"output_directory"`
	PreviewURL       string             `json:"preview_url,omitempty"`
	Readiness        ExportReadiness    `json:"readiness"`
}

func (s *Service) BuildWebsiteSnapshot(ctx context.Context) (WebsiteSnapshot, ExportReadiness, string, error) {
	db, err := s.store.DB().DB()
	if err != nil {
		return WebsiteSnapshot{}, ExportReadiness{}, "", err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return WebsiteSnapshot{}, ExportReadiness{}, "", err
	}
	defer tx.Rollback()
	return s.BuildWebsiteSnapshotTx(ctx, tx)
}

func (s *Service) BuildWebsiteSnapshotTx(ctx context.Context, tx *sql.Tx) (WebsiteSnapshot, ExportReadiness, string, error) {
	var projectID int64
	var snapshot WebsiteSnapshot
	if err := tx.QueryRowContext(ctx, `SELECT id,name FROM projects WHERE uuid=?`, s.store.ProjectUUID()).Scan(&projectID, &snapshot.Title); err != nil {
		return snapshot, ExportReadiness{}, "", err
	}
	profile, err := s.store.RequirePictureBookProfile()
	if err != nil {
		return snapshot, ExportReadiness{}, "", err
	}
	readiness, entries, err := queryExportReadiness(ctx, tx, projectID, "project", "")
	if err != nil {
		return snapshot, readiness, "", err
	}
	snapshot.Version, snapshot.Format, snapshot.Processor = 1, profile.Format, WebsiteProcessor
	snapshot.TemplateVersion = websiteTemplateVersion
	snapshot.Chapters = []WebsiteChapter{}
	rows, err := tx.QueryContext(ctx, `SELECT uuid,chapter_code,title FROM chapters WHERE project_id=? AND deleted_at IS NULL ORDER BY sort_order,id`, projectID)
	if err != nil {
		return snapshot, readiness, "", err
	}
	for rows.Next() {
		c := WebsiteChapter{Pages: []WebsitePage{}}
		if err = rows.Scan(&c.UUID, &c.Code, &c.Title); err != nil {
			rows.Close()
			return snapshot, readiness, "", err
		}
		snapshot.Chapters = append(snapshot.Chapters, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return snapshot, readiness, "", err
	}
	readiness.CanExport = false
	for _, entry := range entries {
		page := WebsitePage{UUID: entry.SectionUUID, Role: entry.PageRole, Number: entry.BodyPageNo, AssetUUID: entry.ImageAssetUUID}
		if err = tx.QueryRowContext(ctx, `SELECT s.title,o.sha256 FROM comic_sections s JOIN files f ON f.uuid=? JOIN file_objects o ON o.id=f.file_object_id WHERE s.uuid=?`, entry.ImageAssetUUID, entry.SectionUUID).Scan(&page.Title, &page.SHA256); err != nil {
			return snapshot, readiness, "", err
		}
		// Read through Asset Store even for readiness: missing physical files
		// must be disclosed before the user opts into a partial website.
		openErr := s.files.CheckContentTx(ctx, tx, page.AssetUUID)
		if openErr != nil {
			readiness.ImageSectionCount--
			readiness.MissingSections = append(readiness.MissingSections, ExportMissingSection{UUID: page.UUID, ChapterUUID: entry.ChapterUUID, Title: page.Title, SectionNo: entry.SectionNo, PageRole: page.Role, BodyPageNo: page.Number})
			continue
		}
		for i := range snapshot.Chapters {
			if snapshot.Chapters[i].UUID == entry.ChapterUUID {
				snapshot.Chapters[i].Pages = append(snapshot.Chapters[i].Pages, page)
				break
			}
		}
		if page.Role == PageRoleBody {
			readiness.CanExport = true
		}
	}
	readiness.MissingSectionCount = len(readiness.MissingSections)
	readiness.Complete = readiness.CanExport && readiness.MissingSectionCount == 0
	snapshot.Missing = readiness.MissingSections
	encoded, err := json.Marshal(snapshot)
	return snapshot, readiness, hashJSON(encoded), err
}

func (s *Service) currentWebsite(ctx context.Context) (websiteGeneration, error) {
	var generation websiteGeneration
	err := s.store.DB().WithContext(ctx).Raw(`SELECT g.* FROM website_generations g JOIN project_websites w ON w.generation_id=g.id WHERE w.project_id=(SELECT id FROM projects WHERE uuid=?)`, s.store.ProjectUUID()).Scan(&generation).Error
	if err == nil && generation.ID == 0 {
		err = gorm.ErrRecordNotFound
	}
	return generation, err
}

func (s *Service) WebsiteStatus(ctx context.Context) (WebsiteStatus, error) {
	_, readiness, hash, err := s.BuildWebsiteSnapshot(ctx)
	if err != nil {
		return WebsiteStatus{}, err
	}
	result := WebsiteStatus{OutputDirectory: "website/", Readiness: readiness, HasUpdates: true}
	var latest WebsiteGeneration
	err = s.store.DB().WithContext(ctx).Raw(`SELECT g.uuid,t.uuid AS task_uuid,t.status,t.progress,t.error_code,g.processed_images,g.reused_images,g.created_at,g.completed_at FROM website_generations g JOIN production_task_runs t ON t.id=g.task_id WHERE g.project_id=(SELECT id FROM projects WHERE uuid=?) ORDER BY CASE WHEN t.status IN ('queued','running') THEN 0 ELSE 1 END,t.updated_at DESC,g.id DESC LIMIT 1`, s.store.ProjectUUID()).Scan(&latest).Error
	if err != nil {
		return result, err
	}
	if latest.UUID != "" {
		result.Latest = &latest
	}
	err = s.store.WithFileCommit(func() error {
		current, err := s.currentWebsite(ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		result.LastSuccessfulAt = current.CompletedAt
		result.HasUpdates = hash != current.SnapshotHash
		root, err := s.store.ResolvePath("website")
		if err != nil {
			return err
		}
		var manifest websiteManifest
		if json.Unmarshal([]byte(current.ManifestJSON), &manifest) == nil {
			result.Available = websiteFilesPresent(root, manifest)
		}
		result.HasUpdates = result.HasUpdates || !result.Available
		if result.Available {
			result.PreviewURL = "/media/projects/" + s.store.ProjectUUID() + "/website/index.html"
		}
		return nil
	})
	return result, err
}

// OpenWebsiteContent only serves files recorded in the successful manifest.
// Unrelated files in website/ are preserved but never exposed by local preview.
func (s *Service) OpenWebsiteContent(ctx context.Context, relative string) (*os.File, error) {
	var output *os.File
	err := s.store.WithFileCommit(func() error {
		current, err := s.currentWebsite(ctx)
		if err != nil {
			return err
		}
		var manifest websiteManifest
		if err := json.Unmarshal([]byte(current.ManifestJSON), &manifest); err != nil {
			return err
		}
		if _, ok := manifest.Files[relative]; !ok {
			return domainError(CodeNotFound, "网站文件不存在", "请先生成网站。", nil)
		}
		path, err := s.store.ResolvePath("website/" + relative)
		if err != nil {
			return err
		}
		output, err = os.Open(path)
		return err
	})
	return output, err
}
