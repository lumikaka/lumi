package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lumi/internal/appstore"
)

type RecentProjectCover struct {
	File         *os.File
	MIMEType     string
	ByteSize     int64
	ETag         string
	LastModified time.Time
	Filename     string
}

type recentProjectCoverReference struct {
	AssetUUID string
	KeyPath   string
	MIMEType  string
	ByteSize  int64
	SHA256    string
	Filename  string
}

const recentProjectCoverQuery = `
SELECT files.uuid, objects.key_path, objects.mime_type, objects.byte_size, objects.sha256,
       COALESCE(files.original_filename, files.display_name, '')
FROM projects
JOIN chapters ON chapters.project_id = projects.id AND chapters.deleted_at IS NULL
JOIN chapter_comic_states ON chapter_comic_states.chapter_id = chapters.id
JOIN comic_sections ON comic_sections.chapter_comic_state_id = chapter_comic_states.id AND comic_sections.deleted_at IS NULL
JOIN comic_image_variants ON comic_image_variants.id = comic_sections.current_image_variant_id
JOIN files ON files.id = comic_image_variants.file_id AND files.project_id = projects.id AND files.deleted_at IS NULL
JOIN file_objects AS objects ON objects.id = files.file_object_id AND objects.project_id = projects.id AND objects.state = 'ready'
WHERE projects.uuid = ?
ORDER BY chapters.sort_order, chapters.id, comic_sections.section_no, comic_sections.id
LIMIT 1`

func loadRecentProjectCoverReference(ctx context.Context, projectUUID, root string) (recentProjectCoverReference, error) {
	if !isUUIDv7(projectUUID) {
		return recentProjectCoverReference{}, projectError(CodeInvalidUUID, "项目 UUID 无效", "项目 UUID 必须是 UUIDv7。", nil)
	}
	header, err := readHeader(ctx, root)
	if err != nil {
		return recentProjectCoverReference{}, err
	}
	if header.UUID != projectUUID {
		return recentProjectCoverReference{}, projectError(CodeIdentityMismatch, "项目身份不匹配", "最近项目路径中的 UUID 与索引不一致。", nil)
	}
	db, err := sql.Open("sqlite", projectReadOnlyDSN(root))
	if err != nil {
		return recentProjectCoverReference{}, projectError(CodeInvalidProject, "无法打开项目封面", "project.sqlite 不是可读取的 SQLite 数据库。", err)
	}
	defer db.Close()
	var cover recentProjectCoverReference
	if err := db.QueryRowContext(ctx, recentProjectCoverQuery, projectUUID).Scan(
		&cover.AssetUUID,
		&cover.KeyPath,
		&cover.MIMEType,
		&cover.ByteSize,
		&cover.SHA256,
		&cover.Filename,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return recentProjectCoverReference{}, projectError(CodeProjectCoverNotFound, "项目暂无封面", "项目的第一份绘本内容还没有可用画面。", err)
		}
		return recentProjectCoverReference{}, projectError(CodeInvalidProject, "无法读取项目封面", "项目数据库中的绘本画面记录不可读取。", err)
	}
	if !isUUIDv7(cover.AssetUUID) || !strings.HasPrefix(strings.ToLower(cover.MIMEType), "image/") || cover.ByteSize < 0 || len(cover.SHA256) != 64 {
		return recentProjectCoverReference{}, projectError(CodeInvalidProject, "项目封面记录无效", "第一张绘本画面的文件记录不完整。", nil)
	}
	return cover, nil
}

func (manager *Manager) OpenRecentProjectCover(ctx context.Context, projectUUID string) (RecentProjectCover, error) {
	recent, err := manager.app.RecentProject(ctx, projectUUID)
	if err != nil {
		if errors.Is(err, appstore.ErrRecentProjectNotFound) {
			return RecentProjectCover{}, projectError(CodeProjectNotFound, "最近项目不存在", "该项目可能已从最近列表移除。", err)
		}
		return RecentProjectCover{}, err
	}
	cover, err := loadRecentProjectCoverReference(ctx, recent.UUID, recent.RootPath)
	if err != nil {
		return RecentProjectCover{}, err
	}
	path, err := ResolveRelativePath(recent.RootPath, filepath.ToSlash(filepath.Join("assets", filepath.FromSlash(cover.KeyPath))))
	if err != nil {
		return RecentProjectCover{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return RecentProjectCover{}, projectError(CodeProjectCoverNotFound, "项目封面不可用", "第一张绘本画面的本地文件不存在或不可读取。", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return RecentProjectCover{}, projectError(CodeProjectCoverNotFound, "项目封面不可用", "无法读取第一张绘本画面的文件信息。", err)
	}
	if !info.Mode().IsRegular() || info.Size() != cover.ByteSize {
		_ = file.Close()
		return RecentProjectCover{}, projectError(CodeProjectCoverNotFound, "项目封面不可用", "第一张绘本画面的本地文件与数据库记录不一致。", nil)
	}
	filename := strings.TrimSpace(cover.Filename)
	if filename == "" {
		filename = fmt.Sprintf("cover-%s", cover.AssetUUID)
	}
	return RecentProjectCover{
		File: file, MIMEType: cover.MIMEType, ByteSize: cover.ByteSize,
		ETag: `"` + cover.SHA256 + `"`, LastModified: info.ModTime().UTC(), Filename: filename,
	}, nil
}
