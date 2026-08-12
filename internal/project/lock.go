package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type lockMetadata struct {
	PID         int       `json:"pid"`
	Hostname    string    `json:"hostname"`
	ProjectUUID string    `json:"project_uuid"`
	AcquiredAt  time.Time `json:"acquired_at"`
}

type projectLock struct {
	file *os.File
}

func acquireProjectLock(root, projectUUID string, now time.Time) (*projectLock, error) {
	path := filepath.Join(root, ".lumi", "project.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, projectError(CodePermissionDenied, "无法创建项目锁", "请检查 .lumi 目录的写权限。", err)
	}
	if err := lockFile(file); err != nil {
		metadata := readLockMetadata(file)
		_ = file.Close()
		details := "该项目正由另一个 Lumi 实例使用，请先在另一个实例中关闭项目。"
		if metadata.Hostname != "" && metadata.PID > 0 {
			details = fmt.Sprintf("项目由 %s 上的 Lumi 进程 %d 使用；进程退出后锁会自动恢复。", metadata.Hostname, metadata.PID)
		}
		return nil, projectError(CodeLocked, "项目已被锁定", details, err)
	}
	hostname, _ := os.Hostname()
	metadata := lockMetadata{PID: os.Getpid(), Hostname: hostname, ProjectUUID: projectUUID, AcquiredAt: now.UTC()}
	content, err := json.Marshal(metadata)
	if err == nil {
		if truncateErr := file.Truncate(0); truncateErr == nil {
			_, _ = file.Seek(0, 0)
			_, _ = file.Write(content)
			_ = file.Sync()
		}
	}
	return &projectLock{file: file}, nil
}

func readLockMetadata(file *os.File) lockMetadata {
	if _, err := file.Seek(0, 0); err != nil {
		return lockMetadata{}
	}
	var metadata lockMetadata
	if err := json.NewDecoder(file).Decode(&metadata); err != nil && !errors.Is(err, os.ErrClosed) {
		return lockMetadata{}
	}
	return metadata
}

func (lock *projectLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	return errors.Join(unlockFile(lock.file), lock.file.Close())
}
