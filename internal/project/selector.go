package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"lumi/internal/platformpath"
)

const DefaultNewProjectParentDisplayPath = "~/Documents/Lumi/"

var resolveDefaultProjectParentDir = platformpath.DefaultProjectParentDir

// ExistingDirectorySelector is implemented by trusted desktop platform
// adapters. A browser upload control is intentionally not an implementation.
type ExistingDirectorySelector interface {
	SelectExistingDirectory(context.Context) (string, error)
}

// NewProjectParentSelector is separate because choosing a parent directory
// grants different authority from choosing an existing project.
type NewProjectParentSelector interface {
	SelectNewProjectParent(context.Context) (string, error)
}

// ExplicitExistingDirectory is the development adapter used by the local API.
type ExplicitExistingDirectory string

func (path ExplicitExistingDirectory) SelectExistingDirectory(context.Context) (string, error) {
	return normalizeDirectory(string(path))
}

// ExplicitNewProjectParent is the development adapter used by the local API.
type ExplicitNewProjectParent string

func (path ExplicitNewProjectParent) SelectNewProjectParent(ctx context.Context) (string, error) {
	raw := strings.TrimSpace(string(path))
	if isDefaultNewProjectParent(raw) {
		return DefaultNewProjectParent{}.SelectNewProjectParent(ctx)
	}
	if defaultPath, err := DefaultNewProjectParentPath(); err == nil && sameDirectoryPath(raw, defaultPath) {
		return ensureDefaultNewProjectParent(defaultPath)
	}
	return normalizeDirectory(raw)
}

func isDefaultNewProjectParent(raw string) bool {
	return raw == "" || strings.TrimRight(filepath.ToSlash(raw), "/") == strings.TrimRight(DefaultNewProjectParentDisplayPath, "/")
}

// DefaultNewProjectParent resolves the platform Documents directory and creates
// its Lumi child on first use. Explicit non-default parent paths must already
// exist so a typo cannot silently create an unrelated directory tree.
type DefaultNewProjectParent struct{}

func (DefaultNewProjectParent) SelectNewProjectParent(context.Context) (string, error) {
	parent, err := DefaultNewProjectParentPath()
	if err != nil {
		return "", err
	}
	return ensureDefaultNewProjectParent(parent)
}

// DefaultNewProjectParentPath resolves the default without creating it.
func DefaultNewProjectParentPath() (string, error) {
	parent, err := resolveDefaultProjectParentDir()
	if err != nil {
		return "", projectError(CodeDefaultProjectParentUnavailable, "无法确定默认项目目录", "请检查系统 Documents 目录是否可用。", err)
	}
	if strings.TrimSpace(parent) == "" || !filepath.IsAbs(parent) {
		return "", projectError(CodeDefaultProjectParentUnavailable, "无法确定默认项目目录", "系统返回的 Documents 目录不是有效的绝对路径。", nil)
	}
	return filepath.Clean(parent), nil
}

func ensureDefaultNewProjectParent(parent string) (string, error) {
	if err := os.MkdirAll(parent, 0o755); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return "", projectError(CodePermissionDenied, "无法创建默认项目目录", "请检查 Documents 目录的写权限。", err)
		}
		return "", projectError(CodeInvalidPath, "无法创建默认项目目录", "请手动选择新项目的父目录。", err)
	}
	return normalizeDirectory(parent)
}

func sameDirectoryPath(left, right string) bool {
	left = filepath.Clean(strings.TrimSpace(left))
	right = filepath.Clean(strings.TrimSpace(right))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
