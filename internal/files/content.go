package files

import (
	"context"
	"database/sql"
	"fmt"
	"os"
)

func (service *Service) OpenContent(ctx context.Context, assetUUID string) (Content, error) {
	row, err := service.assetRowByUUID(ctx, assetUUID, false)
	if err != nil {
		return Content{}, err
	}
	if row.ObjectState != ObjectReady {
		return Content{}, fileError(CodeObjectUnavailable, "Asset 内容不可用", "只有 ready 对象可以读取。", nil)
	}
	file, err := service.openObjectContent(row.KeyPath, row.ByteSize)
	if err != nil {
		return Content{}, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return Content{}, err
	}
	filename := row.UUID + "." + row.CanonicalExt
	if row.OriginalFilename != nil {
		filename = *row.OriginalFilename
	}
	return Content{File: file, Asset: service.assetDTO(row), ETag: fmt.Sprintf("\"sha256-%s\"", row.SHA256), LastModified: info.ModTime().UTC(), Filename: filename}, nil
}

// CheckContentTx checks physical availability using the caller's frozen SQLite
// transaction. It must not acquire another connection: project stores use one
// connection to serialize writes. Raw paths never leave the Asset Store.
func (service *Service) CheckContentTx(ctx context.Context, tx *sql.Tx, assetUUID string) error {
	var key string
	var size int64
	err := tx.QueryRowContext(ctx, `SELECT o.key_path,o.byte_size FROM files f JOIN file_objects o ON o.id=f.file_object_id WHERE f.uuid=? AND f.deleted_at IS NULL AND o.state='ready' AND f.project_id=(SELECT id FROM projects WHERE uuid=?)`, assetUUID, service.store.ProjectUUID()).Scan(&key, &size)
	if err != nil {
		return err
	}
	file, err := service.openObjectContent(key, size)
	if err != nil {
		return err
	}
	return file.Close()
}

func (service *Service) openObjectContent(key string, size int64) (*os.File, error) {
	path, err := service.assetPath(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fileError(CodeObjectUnavailable, "Asset 文件缺失", "请运行完整性扫描或重试生成。", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != size {
		file.Close()
		return nil, fileError(CodeObjectUnavailable, "Asset 文件状态异常", "磁盘文件与数据库摘要不一致。", nil)
	}
	return file, nil
}
