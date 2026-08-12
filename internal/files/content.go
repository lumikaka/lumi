package files

import (
	"context"
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
	path, err := service.assetPath(row.KeyPath)
	if err != nil {
		return Content{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return Content{}, fileError(CodeObjectUnavailable, "Asset 文件缺失", "请运行完整性扫描或重试生成。", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return Content{}, err
	}
	if !info.Mode().IsRegular() || info.Size() != row.ByteSize {
		_ = file.Close()
		return Content{}, fileError(CodeObjectUnavailable, "Asset 文件状态异常", "磁盘文件与数据库摘要不一致。", nil)
	}
	filename := row.UUID + "." + row.CanonicalExt
	if row.OriginalFilename != nil {
		filename = *row.OriginalFilename
	}
	return Content{File: file, Asset: service.assetDTO(row), ETag: fmt.Sprintf("\"sha256-%s\"", row.SHA256), LastModified: info.ModTime().UTC(), Filename: filename}, nil
}
