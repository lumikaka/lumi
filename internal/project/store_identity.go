package project

import (
	"fmt"
	"os"
	"path/filepath"
)

// These identities belong to the opened store, not to a caller's path/UUID
// cache. A directory or database copy can have the same pathname and UUID
// while the store and its OS lock still refer to the original files.
type storeIdentity struct {
	root, database os.FileInfo
}

func nodeIdentity(path string, directory bool) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if (directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) {
		return nil, fmt.Errorf("unexpected project file type")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// File.Stat captures Windows file IDs immediately; Lstat may defer this
	// until SameFile, by which time the pathname could already be replaced.
	return file.Stat()
}

func captureStoreIdentity(root string) (storeIdentity, error) {
	directory, err := nodeIdentity(root, true)
	if err != nil {
		return storeIdentity{}, err
	}
	database, err := nodeIdentity(filepath.Join(root, "project.sqlite"), false)
	return storeIdentity{root: directory, database: database}, err
}

// RequireDirectoryIdentity is called while a Manager lease pins the store.
// It validates filesystem identity without reading or writing SQLite, and
// does not release or replace a lock when the original paths have changed.
func (store *Store) RequireDirectoryIdentity() error {
	current, err := captureStoreIdentity(store.root)
	if err != nil || store.identity.root == nil || store.identity.database == nil ||
		!os.SameFile(current.root, store.identity.root) || !os.SameFile(current.database, store.identity.database) {
		return projectError(CodeIdentityMismatch, "项目目录或数据库文件已被替换", "恢复原项目路径或协调原实例释放后重新打开；未复用旧连接执行操作。", err)
	}
	if store.lock == nil || store.lock.file == nil {
		return projectError(CodeIdentityMismatch, "项目锁身份已失效", "协调原实例释放后重新打开项目。", nil)
	}
	locked, err := store.lock.file.Stat()
	if err != nil {
		return projectError(CodeIdentityMismatch, "项目锁身份已失效", "协调原实例释放后重新打开项目。", err)
	}
	currentLock, err := nodeIdentity(filepath.Join(store.root, ".lumi", "project.lock"), false)
	if err != nil || !os.SameFile(locked, currentLock) {
		return projectError(CodeIdentityMismatch, "项目锁文件已被替换", "恢复原项目锁文件或协调原实例释放后重新打开；不要删除锁文件。", err)
	}
	return nil
}
