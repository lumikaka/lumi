package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

var projectDirectories = []string{
	"assets",
	filepath.Join(".lumi", "cache"),
	filepath.Join(".lumi", "thumbnails"),
	filepath.Join(".lumi", "tmp"),
	filepath.Join(".lumi", "quarantine"),
	filepath.Join(".lumi", "backups"),
}

var legacyProjectDirectories = []string{
	"assets",
	filepath.Join(".lumi", "cache"),
	filepath.Join(".lumi", "thumbnails"),
	filepath.Join(".lumi", "tmp"),
	filepath.Join(".lumi", "backups"),
}

const maxProjectDirectoryNumber = 1000

func normalizeDirectory(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", projectError(CodeInvalidPath, "项目路径无效", "请选择一个本机绝对目录。", nil)
	}
	if !filepath.IsAbs(raw) {
		return "", projectError(CodeInvalidPath, "项目路径必须是绝对路径", "开发模式请粘贴本机目录的完整绝对路径。", nil)
	}
	cleaned := filepath.Clean(raw)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", projectError(CodeProjectNotFound, "项目目录不存在", "目录可能已移动，或所在磁盘暂时离线。", err)
		}
		if errors.Is(err, os.ErrPermission) {
			return "", projectError(CodePermissionDenied, "无法访问项目目录", "请检查目录与上级目录的读取权限。", err)
		}
		return "", projectError(CodeInvalidPath, "无法解析项目路径", "请确认路径和符号链接有效。", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", projectError(CodeInvalidPath, "无法读取项目目录", "请检查目录是否可用。", err)
	}
	if !info.IsDir() {
		return "", projectError(CodeInvalidPath, "项目路径不是目录", "请选择项目文件夹，而不是其中的文件。", nil)
	}
	return filepath.Clean(resolved), nil
}

func validateProjectContract(root string) error {
	for _, name := range []string{"README.md", "STORY.md", "project.sqlite"} {
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if err != nil {
			return projectError(CodeInvalidProject, "目录不是有效的 Lumi 项目", fmt.Sprintf("缺少项目文件 %s。", name), err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return projectError(CodeInvalidProject, "项目文件类型无效", fmt.Sprintf("%s 必须是项目根目录中的普通文件。", name), nil)
		}
	}
	// Only require the Goal 01 layout here. New managed directories are added
	// after the project lock is acquired, before migrations can use them.
	for _, name := range legacyProjectDirectories {
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return projectError(CodeInvalidProject, "项目目录结构不完整", fmt.Sprintf("%s 必须是项目内的真实目录。", filepath.ToSlash(name)), err)
		}
	}
	file, err := os.OpenFile(filepath.Join(root, "project.sqlite"), os.O_RDWR, 0)
	if err != nil {
		return projectError(CodePermissionDenied, "项目不可写", "请授予项目目录及 project.sqlite 的读写权限。", err)
	}
	return file.Close()
}

func ensureManagedProjectDirectories(root string) error {
	for _, directory := range projectDirectories {
		path := filepath.Join(root, directory)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			mode := os.FileMode(0o755)
			if strings.HasPrefix(directory, ".lumi") {
				mode = 0o700
			}
			if err := os.MkdirAll(path, mode); err != nil {
				return projectError(CodePermissionDenied, "无法补齐项目目录", fmt.Sprintf("无法创建 %s。", filepath.ToSlash(directory)), err)
			}
			continue
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return projectError(CodeInvalidProject, "项目目录结构无效", fmt.Sprintf("%s 必须是项目内的真实目录。", filepath.ToSlash(directory)), err)
		}
	}
	return nil
}

func projectDirectoryName(name string) string {
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.TrimSpace(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		case !lastDash:
			builder.WriteByte('-')
			lastDash = true
		}
		if builder.Len() >= 80 {
			break
		}
	}
	return strings.Trim(builder.String(), "-.")
}

func reserveNewProjectDirectory(parent, directoryName string) (string, error) {
	for number := 1; number <= maxProjectDirectoryNumber; number++ {
		candidateName := directoryName
		if number > 1 {
			candidateName = fmt.Sprintf("%s-%d", directoryName, number)
		}
		root := filepath.Join(parent, candidateName)
		if err := os.Mkdir(root, 0o755); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			if errors.Is(err, os.ErrPermission) {
				return "", projectError(CodePermissionDenied, "无法创建项目目录", "请检查父目录的写权限。", err)
			}
			return "", projectError(CodeInvalidPath, "无法创建项目目录", "请检查所选位置是否可用。", err)
		}
		return root, nil
	}
	return "", projectError(
		CodeProjectDirectoryNameExhausted,
		"项目目录名称已用尽",
		"基础目录名及自动编号 -2 至 -1000 均已被占用，请更换项目名称或父目录。",
		nil,
	)
}

func createProjectLayout(root, name string) error {
	for _, directory := range projectDirectories {
		mode := os.FileMode(0o755)
		if strings.HasPrefix(directory, ".lumi") {
			mode = 0o700
		}
		if err := os.MkdirAll(filepath.Join(root, directory), mode); err != nil {
			return fmt.Errorf("create %s: %w", directory, err)
		}
	}
	readme := "# " + name + "\n\n这是一个自包含的 Lumi 本地绘本项目。请复制整个目录进行移动或备份。\n"
	story := "# STORY\n\n<!-- Lumi Goal 02 将在这里维护故事正文。 -->\n"
	for filename, content := range map[string]string{"README.md": readme, "STORY.md": story} {
		if err := writeExclusive(filepath.Join(root, filename), []byte(content), 0o644); err != nil {
			return err
		}
	}
	file, err := os.OpenFile(filepath.Join(root, "project.sqlite"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("create project.sqlite: %w", err)
	}
	return file.Close()
}

func writeExclusive(path string, content []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync %s: %w", filepath.Base(path), err)
	}
	return file.Close()
}

// ResolveRelativePath is the project-scoped filesystem boundary reusable by
// later stores. It never accepts or returns a path outside the project root.
func ResolveRelativePath(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || strings.ContainsRune(relative, 0) {
		return "", projectError(CodeInvalidPath, "项目内部路径无效", "项目内部路径必须是规范化相对路径。", nil)
	}
	cleaned := filepath.Clean(filepath.FromSlash(relative))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", projectError(CodeInvalidPath, "项目内部路径越界", "路径不能离开项目根目录。", nil)
	}
	root = filepath.Clean(root)
	joined := filepath.Join(root, cleaned)
	rel, err := filepath.Rel(root, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", projectError(CodeInvalidPath, "项目内部路径越界", "路径不能离开项目根目录。", err)
	}
	current := root
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return "", projectError(CodeInvalidPath, "无法验证项目内部路径", "请检查项目目录权限。", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", projectError(CodeInvalidPath, "项目内部路径包含符号链接", "为防止路径越界，项目数据路径不能经过符号链接。", nil)
		}
	}
	return joined, nil
}
