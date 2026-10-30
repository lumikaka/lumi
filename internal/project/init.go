package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"lumi/internal/promptcatalog"
)

type DirectoryInfo struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	RootPath string `json:"root_path"`
	Open     bool   `json:"open"`
}

type InitResult struct {
	DirectoryInfo
	Created bool `json:"created"`
}

// InspectDirectory reads identity without migrations, runtime startup, or
// recent-index updates. It briefly probes the OS lock without changing its
// metadata, and reports a lock held by either CLI or desktop as open.
func InspectDirectory(ctx context.Context, rawRoot string) (DirectoryInfo, error) {
	root, err := normalizeDirectory(rawRoot)
	if err != nil {
		return DirectoryInfo{}, err
	}
	if err := validateProjectContract(root); err != nil {
		return DirectoryInfo{}, err
	}
	header, open, err := inspectHeader(ctx, root)
	if err != nil {
		return DirectoryInfo{}, err
	}
	return DirectoryInfo{UUID: header.UUID, Name: header.Name, RootPath: root, Open: open}, nil
}

// SQLite mode=ro can create WAL/SHM files. When no Lumi writer owns the
// project, hold its OS lock without rewriting metadata and use immutable mode
// for this connection only. Never use immutable mode against a live writer:
// that connection must read SQLite's current WAL snapshot instead.
func inspectHeader(ctx context.Context, root string) (Header, bool, error) {
	path := filepath.Join(root, ".lumi", "project.lock")
	file, err := os.Open(path)
	created := false
	if errors.Is(err, os.ErrNotExist) {
		file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		created = err == nil
		if errors.Is(err, os.ErrExist) {
			file, err = os.Open(path)
		}
	}
	if err != nil {
		return Header{}, false, projectError(CodePermissionDenied, "无法检查项目锁", "请检查 .lumi/project.lock 的访问权限。", err)
	}
	defer file.Close()
	if err := lockFile(file); err != nil {
		header, readErr := readHeader(ctx, root)
		return header, true, readErr
	}
	defer unlockFile(file)
	if created {
		defer func() {
			owned, statErr := file.Stat()
			current, currentErr := os.Lstat(path)
			if statErr == nil && currentErr == nil && os.SameFile(owned, current) {
				_ = os.Remove(path)
			}
		}()
	}
	wal, err := os.Lstat(filepath.Join(root, "project.sqlite-wal"))
	if errors.Is(err, os.ErrNotExist) || (err == nil && wal.Mode().IsRegular() && wal.Size() == 0) {
		header, readErr := readHeaderDSN(ctx, projectReadOnlyDSN(root)+"&immutable=1")
		return header, false, readErr
	}
	if err != nil {
		return Header{}, false, err
	}
	// A crashed writer may have committed identity changes only in the WAL.
	// immutable=1 ignores WAL. Read a private snapshot of the DB + WAL while
	// holding the writer lock, so SQLite can rebuild SHM outside the project.
	snapshot, err := os.MkdirTemp("", "lumi-inspect-*")
	if err != nil {
		return Header{}, false, err
	}
	defer os.RemoveAll(snapshot)
	for _, name := range []string{"project.sqlite", "project.sqlite-wal"} {
		if err := copyInspectionFile(filepath.Join(root, name), filepath.Join(snapshot, name)); err != nil {
			return Header{}, false, err
		}
	}
	header, readErr := readHeader(ctx, snapshot)
	return header, false, readErr
}

func copyInspectionFile(source, target string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("项目快照源必须是普通文件：%s", filepath.Base(source))
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	return errors.Join(err, output.Close())
}

// InitAt initializes an existing directory. Existing projects are inspected
// only; the caller may independently add optional host guidance to them.
// Desktop creation deliberately continues to use CreateWithInput.
func (manager *Manager) InitAt(ctx context.Context, rawRoot string, input CreateInput) (result InitResult, err error) {
	root, err := normalizeDirectory(rawRoot)
	if err != nil {
		return result, err
	}
	if info, inspectErr := InspectDirectory(ctx, root); inspectErr == nil {
		return InitResult{DirectoryInfo: info}, nil
	} else {
		var pe *Error
		if errors.As(inspectErr, &pe) && pe.Code == CodeFormatTooNew {
			return result, inspectErr
		}
	}
	// Lstat also rejects dangling symlinks. Sidecars must never be adopted by
	// SQLite, even if the primary database does not yet exist.
	for _, name := range []string{"README.md", "STORY.md", "project.sqlite", "project.sqlite-wal", "project.sqlite-shm", "project.sqlite-journal", ".lumi", "assets"} {
		if _, statErr := os.Lstat(filepath.Join(root, name)); statErr == nil {
			return result, projectError(CodeInitializationConflict, "项目受管路径已存在："+name, "目录不是有效的 Lumi 项目；请移动冲突文件后重试。未覆盖任何已有文件。", nil)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return result, projectError(CodePermissionDenied, "无法检查项目路径", name, statErr)
		}
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = filepath.Base(root)
	}
	if len([]rune(name)) > 120 || name == "" {
		return result, projectError(CodeInvalidProject, "项目名称无效", "项目名称需包含 1 到 120 个字符。", nil)
	}
	language, valid := NormalizeGenerationLanguage(input.GenerationLanguage)
	if !valid {
		return result, projectError(CodeInvalidProject, "项目生成语言无效", "language 只支持 zh-Hans 或 en。", nil)
	}
	profile, err := NormalizePictureBookInput(nil)
	if err != nil {
		return result, err
	}
	projectUUID, err := newUUIDv7()
	if err != nil {
		return result, err
	}
	actorUUID, err := newUUIDv7()
	if err != nil {
		return result, err
	}
	owned := &initializationLayout{root: root}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, owned.cleanup())
		}
	}()
	if err := owned.create(name); err != nil {
		if errors.Is(err, os.ErrExist) {
			return result, projectError(CodeInitializationConflict, "无法原地创建项目文件", "路径可能被并发初始化占用；未覆盖已有文件。", err)
		}
		return result, projectError(CodePermissionDenied, "无法原地创建项目文件", "请检查目录写权限与磁盘可用空间。", err)
	}
	now := manager.now().UTC()
	lock, err := acquireProjectLock(root, projectUUID, now)
	owned.remember(".lumi/project.lock")
	if err != nil {
		return result, err
	}
	store, err := initializeStore(ctx, root, projectUUID, name, language, actorUUID, &profile, strings.TrimSpace(promptcatalog.DefaultProjectStyle(language)), nil, now, lock)
	if err != nil {
		_ = lock.Close()
		return result, err
	}
	entry, owner, err := manager.startOpening(ctx, projectUUID, root)
	if err != nil || !owner {
		_ = store.Close()
		if err == nil {
			err = fmt.Errorf("new project identity already registered")
		}
		return result, err
	}
	if err := manager.runOpenHooks(context.WithValue(ctx, openHookContextKey{}, true), store); err != nil {
		openErr, retained := manager.failOpening(ctx, entry, store, err)
		committed = retained
		return result, openErr
	}
	if err := manager.app.RecordProject(ctx, projectUUID, name, root, now); err != nil {
		openErr, retained := manager.failOpening(ctx, entry, store, err)
		committed = retained
		return result, openErr
	}
	manager.finishOpening(entry, store, nil)
	manager.notifyLifecycle(projectUUID, true)
	committed = true
	return InitResult{DirectoryInfo: DirectoryInfo{UUID: projectUUID, Name: name, RootPath: root, Open: true}, Created: true}, nil
}

type initializedNode struct {
	path string
	info os.FileInfo
}

type initializationLayout struct {
	root  string
	nodes []initializedNode
	db    bool
}

func (layout *initializationLayout) remember(relative string) {
	path := filepath.Join(layout.root, relative)
	if info, err := os.Lstat(path); err == nil {
		layout.nodes = append(layout.nodes, initializedNode{path, info})
	}
}

func (layout *initializationLayout) create(name string) error {
	// Exclusive creation of .lumi reserves initialization across processes.
	for _, directory := range append([]string{".lumi"}, projectDirectories...) {
		mode := os.FileMode(0o755)
		if strings.HasPrefix(directory, ".lumi") {
			mode = 0o700
		}
		if err := os.Mkdir(filepath.Join(layout.root, directory), mode); err != nil {
			return err
		}
		layout.remember(directory)
	}
	files := projectTextFiles(name)
	files["project.sqlite"] = ""
	for _, relative := range []string{"README.md", "STORY.md", "project.sqlite"} {
		mode := os.FileMode(0o644)
		if relative == "project.sqlite" {
			mode = 0o600
		}
		file, err := os.OpenFile(filepath.Join(layout.root, relative), os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		layout.remember(relative)
		layout.db = relative == "project.sqlite"
		_, writeErr := file.WriteString(files[relative])
		syncErr := file.Sync()
		if err := errors.Join(writeErr, syncErr, file.Close()); err != nil {
			return err
		}
	}
	return nil
}

func (layout *initializationLayout) cleanup() error {
	var result error
	if layout.db {
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			layout.remember("project.sqlite" + suffix)
		}
	}
	for i := len(layout.nodes) - 1; i >= 0; i-- {
		node := layout.nodes[i]
		info, err := os.Lstat(node.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if os.SameFile(info, node.info) {
			// Never recursively delete: concurrent user files in a newly created
			// directory must survive rollback, as must replaced nodes.
			result = errors.Join(result, os.Remove(node.path))
		}
	}
	return result
}
