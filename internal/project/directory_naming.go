package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lumi/internal/appstore"
)

type DirectoryNamePreview struct {
	RootPath string `json:"root_path"`
}

func directoryBaseName(name string) (string, error) {
	name, err := validateSetupText(name, 120, "name")
	if err != nil {
		return "", err
	}
	base := projectDirectoryName(name)
	if base == "" {
		return "", projectError(CodeInvalidPath, "项目名称不能用作目录名", "名称至少需要包含一个文字或数字。", nil)
	}
	return base, nil
}

func directoryCandidate(root, base string, number int) string {
	if number > 1 {
		base = fmt.Sprintf("%s-%d", base, number)
	}
	return filepath.Join(filepath.Dir(root), base)
}

func (manager *Manager) PreviewDirectoryName(ctx context.Context, projectUUID, name string) (DirectoryNamePreview, error) {
	if !isUUIDv7(projectUUID) {
		return DirectoryNamePreview{}, projectError(CodeInvalidUUID, "项目 UUID 无效", "项目必须使用公开 UUIDv7。", nil)
	}
	base, err := directoryBaseName(name)
	if err != nil {
		return DirectoryNamePreview{}, err
	}
	recent, err := manager.app.RecentProject(ctx, projectUUID)
	if err != nil {
		if errors.Is(err, appstore.ErrRecentProjectNotFound) {
			return DirectoryNamePreview{}, projectError(CodeProjectNotFound, "项目不存在", "该项目不在最近项目索引中。", err)
		}
		return DirectoryNamePreview{}, err
	}
	for number := 1; number <= maxProjectDirectoryNumber; number++ {
		candidate := directoryCandidate(recent.RootPath, base, number)
		if candidate == recent.RootPath {
			return DirectoryNamePreview{RootPath: candidate}, nil
		}
		_, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			return DirectoryNamePreview{RootPath: candidate}, nil
		}
		if err != nil {
			return DirectoryNamePreview{}, err
		}
	}
	return DirectoryNamePreview{}, projectError(CodeProjectDirectoryNameExhausted, "项目目录名称已用尽", "请更换项目名称。", nil)
}

// Called while the caller still owns its request lease. Moving the directory
// happens later, outside the Chat/HTTP request that requested the rename.
func (manager *Manager) SetDirectoryRename(ctx context.Context, projectUUID, name string, rename bool) error {
	manager.directoryMu.Lock()
	defer manager.directoryMu.Unlock()
	if rename {
		if _, err := directoryBaseName(name); err != nil {
			return err
		}
	} else {
		name = ""
	}
	return manager.app.SetPendingDirectoryName(ctx, projectUUID, strings.TrimSpace(name))
}

// Reuses the server's lifecycle pass. Presence does not prevent a rename, but
// active requests and background work do; no running job is interrupted.
func (manager *Manager) ApplyPendingDirectoryRenames(ctx context.Context) error {
	if !manager.directoryMu.TryLock() {
		return nil
	}
	defer manager.directoryMu.Unlock()
	items, err := manager.app.PendingDirectoryProjects(ctx)
	if err != nil {
		return err
	}
	var result error
	for _, item := range items {
		operationCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := manager.applyDirectoryRename(operationCtx, item)
		cancel()
		result = errors.Join(result, err)
	}
	return result
}

func (manager *Manager) applyDirectoryRename(ctx context.Context, item appstore.RecentProject) error {
	manager.mu.Lock()
	entry := manager.projects[item.UUID]
	manager.mu.Unlock()
	// Closed projects are handled when next opened. Never open an unattended
	// project (and restart its jobs) just to perform a directory rename.
	if entry == nil {
		return nil
	}
	busy, err := manager.runtime.HasActiveWork(ctx, item.UUID)
	if err != nil || busy {
		return err
	}
	manager.mu.Lock()
	if manager.projects[item.UUID] != entry || entry.state != projectOpen || entry.leases != 0 || entry.root != item.RootPath {
		manager.mu.Unlock()
		return nil
	}
	entry.state = projectDraining
	manager.signalLocked(entry)
	manager.mu.Unlock()
	busy, err = manager.runtime.HasActiveWork(ctx, item.UUID)
	if err != nil || busy {
		manager.restoreState(entry, projectOpen)
		return err
	}
	if err := manager.runtime.StopProject(ctx, item.UUID); err != nil {
		manager.restoreState(entry, projectOpen)
		return err
	}
	oldRoot := entry.root
	closeErr := entry.store.Close()
	root := oldRoot
	moveErr := closeErr
	if closeErr == nil {
		root, moveErr = moveProjectDirectory(oldRoot, item.PendingDirectoryName)
		if moveErr == nil {
			if err := manager.app.CompleteDirectoryRename(ctx, item.UUID, root, manager.now().UTC()); err != nil {
				moveErr = err
				if root != oldRoot {
					if rollbackErr := renameDirectoryNoReplace(root, oldRoot); rollbackErr != nil {
						moveErr = errors.Join(moveErr, rollbackErr)
					} else {
						root = oldRoot
					}
				}
			}
		}
	}
	// Keep the entry so all existing WebSocket presence leases remain valid.
	manager.mu.Lock()
	entry.root = root
	entry.state = projectOpening
	manager.signalLocked(entry)
	manager.mu.Unlock()
	header, openErr := readHeader(ctx, root)
	if openErr == nil && header.UUID != item.UUID {
		openErr = projectError(CodeIdentityMismatch, "项目目录身份不匹配", "重命名后的项目 UUID 与原项目不一致。", nil)
	}
	if openErr == nil {
		var lock *projectLock
		lock, openErr = acquireProjectLock(root, item.UUID, manager.now().UTC())
		if openErr == nil {
			var store *Store
			store, openErr = openStore(ctx, root, header, manager.now().UTC(), lock)
			if openErr != nil {
				_ = lock.Close()
			} else {
				if openErr = manager.runOpenHooks(ctx, store); openErr == nil {
					manager.finishOpening(entry, store, nil)
					manager.notifyLifecycle(item.UUID, true)
					return moveErr
				}
				openErr, _ = manager.failOpening(ctx, entry, store, openErr)
				return errors.Join(moveErr, openErr)
			}
		}
	}
	manager.finishOpening(entry, nil, openErr)
	manager.notifyLifecycle(item.UUID, false)
	return errors.Join(moveErr, openErr)
}

// The pending name also lets OpenRecent recover the small crash window
// between the filesystem rename and updating the local recent-project index.
func (manager *Manager) recoverRenamedDirectory(ctx context.Context, recent appstore.RecentProject) (string, error) {
	if recent.PendingDirectoryName == "" {
		return recent.RootPath, nil
	}
	if _, err := os.Lstat(recent.RootPath); !errors.Is(err, os.ErrNotExist) {
		return recent.RootPath, nil
	}
	base, err := directoryBaseName(recent.PendingDirectoryName)
	if err != nil {
		return recent.RootPath, err
	}
	for number := 1; number <= maxProjectDirectoryNumber; number++ {
		candidate := directoryCandidate(recent.RootPath, base, number)
		root, header, err := prepareExistingProject(ctx, ExplicitExistingDirectory(candidate), recent.UUID)
		if err == nil && header.UUID == recent.UUID {
			return root, manager.app.CompleteDirectoryRename(ctx, recent.UUID, root, manager.now().UTC())
		}
	}
	return recent.RootPath, nil
}

func moveProjectDirectory(root, name string) (string, error) {
	base, err := directoryBaseName(name)
	if err != nil {
		return root, err
	}
	for number := 1; number <= maxProjectDirectoryNumber; number++ {
		candidate := directoryCandidate(root, base, number)
		if candidate == root {
			return root, nil
		}
		if err := renameDirectoryNoReplace(root, candidate); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return root, err
		}
		return candidate, nil
	}
	return root, projectError(CodeProjectDirectoryNameExhausted, "项目目录名称已用尽", "请更换项目名称。", nil)
}
