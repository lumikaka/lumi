package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"lumi/internal/appstore"
)

// Lock owner kinds describe which Lumi entry point holds a project. They are
// display hints for release guidance; mutual exclusion still comes only from
// the operating-system lock.
const (
	LockOwnerCLI     = "cli"
	LockOwnerDesktop = "desktop"
	LockOwnerWeb     = "web"
	LockOwnerUnknown = "unknown"
)

var processLockOwner atomic.Value // string

// SetLockOwner records this process's role in every project lock it acquires
// afterwards. Each Lumi process has exactly one role, so this is process-wide.
func SetLockOwner(kind string) {
	switch kind {
	case LockOwnerCLI, LockOwnerDesktop, LockOwnerWeb:
		processLockOwner.Store(kind)
	default:
		processLockOwner.Store("")
	}
}

func currentLockOwner() string {
	kind, _ := processLockOwner.Load().(string)
	return kind
}

type lockMetadata struct {
	PID         int       `json:"pid"`
	Hostname    string    `json:"hostname"`
	ProjectUUID string    `json:"project_uuid"`
	AcquiredAt  time.Time `json:"acquired_at"`
	Owner       string    `json:"owner,omitempty"`
}

type projectLock struct {
	file *os.File
}

func projectLockPath(root string) string { return filepath.Join(root, ".lumi", "project.lock") }

func acquireProjectLock(root, projectUUID string, now time.Time) (*projectLock, error) {
	file, err := os.OpenFile(projectLockPath(root), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, projectError(CodePermissionDenied, "无法创建项目锁", "请检查 .lumi 目录的写权限。", err)
	}
	if err := lockFile(file); err != nil {
		metadata := readLockMetadata(file)
		_ = file.Close()
		return nil, projectError(CodeLocked, "项目已被锁定", lockedDetails(metadata), err)
	}
	hostname, _ := os.Hostname()
	metadata := lockMetadata{PID: os.Getpid(), Hostname: hostname, ProjectUUID: projectUUID, AcquiredAt: now.UTC(), Owner: currentLockOwner()}
	content, err := json.Marshal(metadata)
	if err == nil {
		// Windows locks byte zero mandatorily. Keep it as JSON whitespace so
		// other handles can read the owner from byte one without moving the
		// lock range (which must still exclude older Lumi versions).
		content = append([]byte{'\n'}, content...)
		if truncateErr := file.Truncate(0); truncateErr == nil {
			_, _ = file.Seek(0, 0)
			_, _ = file.Write(content)
			_ = file.Sync()
		}
	}
	return &projectLock{file: file}, nil
}

func lockOwnerKind(metadata lockMetadata) string {
	switch metadata.Owner {
	case LockOwnerCLI, LockOwnerDesktop, LockOwnerWeb:
		return metadata.Owner
	}
	return LockOwnerUnknown
}

// lockedDetails tells the user the concrete next step for the current owner.
func lockedDetails(metadata lockMetadata) string {
	process := "另一个 Lumi 进程"
	if metadata.PID > 0 {
		process = fmt.Sprintf("%s 上的 Lumi 进程 %d", metadata.Hostname, metadata.PID)
	}
	switch lockOwnerKind(metadata) {
	case LockOwnerCLI:
		return fmt.Sprintf("项目正由 Agent（如 Codex）使用的 Lumi CLI 打开（%s）。在 Agent 中结束当前任务并关闭 Lumi 审阅界面后约 15 秒会自动释放；也可以在终端执行 lumi stop 立即释放。", process)
	case LockOwnerDesktop:
		return fmt.Sprintf("项目正由 Lumi 桌面版打开（%s）。请在桌面版中关闭该项目，或退出桌面版后重试。", process)
	case LockOwnerWeb:
		return fmt.Sprintf("项目正由 Lumi Web 服务打开（%s）。请在该服务中关闭项目或停止服务后重试。", process)
	}
	return fmt.Sprintf("项目正由%s使用；请在桌面版中关闭该项目，或等待 CLI 空闲回收 / 执行 lumi stop 释放。进程退出后锁会自动释放。", process)
}

func readLockMetadata(file *os.File) lockMetadata {
	// Start at zero for legacy metadata, then skip the reserved lock byte if
	// Windows denies reading it through a handle that does not own the lock.
	for _, offset := range []int64{0, 1} {
		if _, err := file.Seek(offset, 0); err != nil {
			return lockMetadata{}
		}
		var metadata lockMetadata
		if err := json.NewDecoder(file).Decode(&metadata); err == nil {
			return metadata
		}
	}
	return lockMetadata{}
}

func (lock *projectLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	return errors.Join(unlockFile(lock.file), lock.file.Close())
}

// LockOwner is the public view of who currently holds a project lock.
type LockOwner struct {
	Kind       string     `json:"kind"`
	PID        int        `json:"pid,omitempty"`
	Hostname   string     `json:"hostname,omitempty"`
	AcquiredAt *time.Time `json:"acquired_at,omitempty"`
}

// LockStatus answers "can this instance open the project now, and if not,
// who holds it". It never changes project state or the recent index.
type LockStatus struct {
	ProjectUUID           string     `json:"project_uuid"`
	Locked                bool       `json:"locked"`
	HeldByCurrentInstance bool       `json:"held_by_current_instance"`
	Owner                 *LockOwner `json:"owner"`
	ReleaseDetails        string     `json:"release_details"`
}

// LockStatus inspects the recent project's lock. A brief non-blocking probe
// distinguishes a live owner from metadata left behind by an exited process.
func (manager *Manager) LockStatus(ctx context.Context, projectUUID string) (LockStatus, error) {
	if !isUUIDv7(projectUUID) {
		return LockStatus{}, projectError(CodeInvalidUUID, "项目 UUID 无效", "项目 UUID 必须是 UUIDv7。", nil)
	}
	status := LockStatus{ProjectUUID: projectUUID}
	manager.mu.Lock()
	_, local := manager.projects[projectUUID]
	manager.mu.Unlock()
	if local {
		// Never probe a lock this process holds or is acquiring.
		hostname, _ := os.Hostname()
		kind := currentLockOwner()
		if kind == "" {
			kind = LockOwnerUnknown
		}
		status.Locked, status.HeldByCurrentInstance = true, true
		status.Owner = &LockOwner{Kind: kind, PID: os.Getpid(), Hostname: hostname}
		return status, nil
	}
	recent, err := manager.app.RecentProject(ctx, projectUUID)
	if err != nil {
		if errors.Is(err, appstore.ErrRecentProjectNotFound) {
			return LockStatus{}, projectError(CodeProjectNotFound, "最近项目不存在", "该项目可能已从最近列表移除。", err)
		}
		return LockStatus{}, err
	}
	file, err := os.OpenFile(projectLockPath(recent.RootPath), os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return LockStatus{}, projectError(CodePermissionDenied, "无法读取项目锁", "请检查 .lumi 目录的读写权限。", err)
	}
	defer file.Close()
	if lockFile(file) == nil {
		_ = unlockFile(file)
		return status, nil
	}
	metadata := readLockMetadata(file)
	owner := &LockOwner{Kind: lockOwnerKind(metadata), PID: metadata.PID, Hostname: metadata.Hostname}
	if !metadata.AcquiredAt.IsZero() {
		acquired := metadata.AcquiredAt.UTC()
		owner.AcquiredAt = &acquired
	}
	status.Locked, status.Owner, status.ReleaseDetails = true, owner, lockedDetails(metadata)
	return status, nil
}
