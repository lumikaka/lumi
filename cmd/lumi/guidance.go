package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const agentsGuidance = `# Lumi 项目

这是一个 Lumi 项目，与 Lumi 桌面版共用项目格式。
在 Codex 中，所有项目内容的读取与修改必须通过 Lumi 插件工具完成。
禁止直接读取或写入 project.sqlite（包括 -wal、-shm、-journal）、.lumi/、assets/。
STORY.md 是只读投影，可读取但不得直接修改；正文修改使用 Lumi 插件工具。

生图前调用 image_target 查询比例和构图建议；角色、场景参考先用 read_image 放入对话。
使用宿主自带生图能力，生成后立即把工具结果中实际提供的绝对路径传给 import_image。
图片保留期限未知；不得假设 savedPath、猜测文件名或扫描固定目录。比例偏差默认允许导入，
可按用户意愿选择 keep、居中 crop 或白色 pad；不要因比例偏差阻塞创作。

这些要求是行为指引。项目级 .codex/config.toml 的 permission profile 只限制
命令沙箱的写入，不限制读取；须先由用户信任项目，且可能被更高优先级配置覆盖。
不要替用户设置信任。Codex App 中的实际保护仍需用户人工确认。
`

const protectionSelection = "default_permissions = \"lumi-protect\"\n"
const protectionProfile = `
[permissions.lumi-protect]
extends = ":workspace"

[permissions.lumi-protect.filesystem.":workspace_roots"]
"project.sqlite" = "read"
"project.sqlite-wal" = "read"
"project.sqlite-shm" = "read"
"project.sqlite-journal" = "read"
".lumi" = "read"
"assets" = "read"
`

type guidanceResult struct {
	Agents   string   `json:"agents"`
	Config   string   `json:"config"`
	Messages []string `json:"messages"`
}

type protectionInfo struct {
	Configured bool     `json:"configured"`
	Effective  string   `json:"effective"`
	Reason     string   `json:"reason"`
	Warnings   []string `json:"warnings"`
}

const protectionCaveat = "写保护实际生效状态未知：请由用户在 Codex 中信任此项目并重新打开会话，核对当前权限与写保护效果；Lumi 不设置信任。旧 sandbox_mode / --sandbox 或更高优先级设置可能使 profile 失效。配置允许读取；禁止直接读取属于 AGENTS.md 行为指引。"

func ensureGuidance(root string) guidanceResult {
	result := guidanceResult{Agents: "preserved", Config: "manual", Messages: []string{}}
	path := filepath.Join(root, "AGENTS.md")
	if err := writeGuidanceExclusive(path, []byte(agentsGuidance), 0o644); err == nil {
		result.Agents = "created"
		result.Messages = append(result.Messages, "已创建 AGENTS.md。")
	} else if errors.Is(err, os.ErrExist) {
		result.Messages = append(result.Messages, "AGENTS.md 已存在，保留原文；请确认其中包含 Lumi 插件读写指引。")
	} else {
		result.Agents = "manual"
		result.Messages = append(result.Messages, "无法生成 AGENTS.md，请手工补齐 Lumi 插件读写指引："+err.Error())
	}
	configDir := filepath.Join(root, ".codex")
	if err := os.Mkdir(configDir, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		result.Messages = append(result.Messages, "无法创建 .codex，请手工添加 Lumi profile："+err.Error())
		return result
	}
	info, err := os.Lstat(configDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		result.Messages = append(result.Messages, ".codex 不是项目内的真实目录；未修改配置，请手工添加 Lumi profile。")
		return result
	}
	path = filepath.Join(configDir, "config.toml")
	info, err = os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		err = writeGuidanceExclusive(path, []byte(protectionSelection+protectionProfile), 0o644)
		if err == nil {
			result.Config = "created"
			result.Messages = append(result.Messages, "已创建 .codex/config.toml 的 lumi-protect profile。")
			return result
		}
	}
	if err != nil || !info.Mode().IsRegular() {
		result.Messages = append(result.Messages, ".codex/config.toml 不可安全写入（可能是符号链接或并发创建）；保留原文件，请手工合并 Lumi profile。")
		return result
	}
	original, err := os.ReadFile(path)
	if err != nil {
		result.Messages = append(result.Messages, "无法读取 .codex/config.toml；请手工合并 Lumi profile："+err.Error())
		return result
	}
	merged, warnings, err := mergeProtectionConfig(original)
	result.Messages = append(result.Messages, warnings...)
	if err != nil {
		result.Messages = append(result.Messages, "未修改 .codex/config.toml，请手工处理："+err.Error()+"。配置示例见 docs/features/codex-plugins/cli.md。")
		return result
	}
	if bytes.Equal(original, merged) {
		result.Config = "preserved"
		result.Messages = append(result.Messages, ".codex/config.toml 已包含 Lumi profile，保留原文。")
		return result
	}
	if err := replaceGuidanceConfig(path, original, merged, info); err != nil {
		result.Messages = append(result.Messages, "配置合并未完成，请手工合并 Lumi profile："+err.Error())
		return result
	}
	result.Config = "merged"
	result.Messages = append(result.Messages, "已合并 Lumi profile，保留 .codex/config.toml 的已有内容和注释。")
	return result
}

// Parse for semantics, but preserve user bytes instead of round-tripping TOML.
// A second parse verifies that adding tables has not changed existing values.
func mergeProtectionConfig(original []byte) ([]byte, []string, error) {
	var config map[string]any
	if err := toml.Unmarshal(original, &config); err != nil {
		return nil, nil, fmt.Errorf("TOML 无效：%w", err)
	}
	warnings := legacySandboxWarnings(config)
	if hasOtherPermissionSelection(config) {
		return nil, warnings, fmt.Errorf("已有其他 default_permissions，不能替换用户权限选择")
	}
	_, hasSelection := config["default_permissions"]
	var expected map[string]any
	_ = toml.Unmarshal([]byte(protectionSelection+protectionProfile), &expected)
	permissions, hasPermissions := config["permissions"]
	var existingProfile any
	if hasPermissions {
		table, ok := permissions.(map[string]any)
		if !ok {
			return nil, warnings, fmt.Errorf("permissions 不是可合并的表")
		}
		existingProfile = table["lumi-protect"]
	}
	wanted := expected["permissions"].(map[string]any)["lumi-protect"]
	if existingProfile != nil && !reflect.DeepEqual(existingProfile, wanted) {
		return nil, warnings, fmt.Errorf("已有不同的 permissions.lumi-protect，不能覆盖")
	}
	merged := append([]byte{}, original...)
	if !hasSelection {
		merged = append([]byte(protectionSelection+"\n"), merged...)
	}
	if existingProfile == nil {
		merged = append(merged, []byte("\n"+protectionProfile)...)
	}
	var checked map[string]any
	if err := toml.Unmarshal(merged, &checked); err != nil {
		return nil, warnings, fmt.Errorf("现有 TOML 表无法安全扩展：%w", err)
	}
	// Verify every pre-existing setting is still present with the same value.
	if !containsConfig(checked, config) {
		return nil, warnings, fmt.Errorf("合并会改变现有配置")
	}
	return merged, warnings, nil
}

func hasOtherPermissionSelection(config map[string]any) bool {
	for key, value := range config {
		if key == "default_permissions" && value != "lumi-protect" {
			return true
		}
		if nested, ok := value.(map[string]any); ok && hasOtherPermissionSelection(nested) {
			return true
		}
	}
	return false
}

func containsConfig(merged, original map[string]any) bool {
	for key, value := range original {
		if table, ok := value.(map[string]any); ok {
			other, ok := merged[key].(map[string]any)
			if !ok || !containsConfig(other, table) {
				return false
			}
		} else if !reflect.DeepEqual(merged[key], value) {
			return false
		}
	}
	return true
}

func legacySandboxWarnings(config map[string]any) []string {
	var warnings []string
	var walk func(map[string]any, string)
	walk = func(table map[string]any, prefix string) {
		for key, value := range table {
			if key == "sandbox_mode" {
				warnings = append(warnings, "检测到旧设置 "+prefix+key+"；加载该 sandbox_mode 时会使 permission profile 失效，请用户手工迁移。")
			}
			if nested, ok := value.(map[string]any); ok {
				walk(nested, prefix+key+".")
			}
		}
	}
	walk(config, "")
	return warnings
}

func inspectProtection(root string) protectionInfo {
	result := protectionInfo{Effective: "unknown", Reason: protectionCaveat, Warnings: []string{}}
	path := filepath.Join(root, ".codex", "config.toml")
	if info, err := os.Lstat(filepath.Dir(path)); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		result.Warnings = append(result.Warnings, "缺少项目内的 .codex/config.toml。")
		return result
	}
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		result.Warnings = append(result.Warnings, "缺少普通文件 .codex/config.toml。")
		return result
	}
	content, err := os.ReadFile(path)
	var config map[string]any
	if err == nil {
		err = toml.Unmarshal(content, &config)
	}
	if err != nil {
		result.Warnings = append(result.Warnings, "无法读取有效的项目配置："+err.Error())
		return result
	}
	merged, warnings, mergeErr := mergeProtectionConfig(content)
	result.Warnings = append(result.Warnings, warnings...)
	result.Configured = mergeErr == nil && bytes.Equal(content, merged)
	if !result.Configured {
		result.Warnings = append(result.Warnings, "项目配置尚未完整选择 Lumi 写保护 profile。")
	}
	return result
}

func writeGuidanceExclusive(path string, content []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(content)
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err != nil {
		_ = os.Remove(path)
	}
	return err
}

func replaceGuidanceConfig(path string, original, merged []byte, info os.FileInfo) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".lumi-config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		_ = file.Close()
		return err
	}
	_, writeErr := file.Write(merged)
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return err
	}
	currentInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !os.SameFile(info, currentInfo) || !currentInfo.Mode().IsRegular() || !bytes.Equal(current, original) {
		return fmt.Errorf("配置已被其他程序修改；保留当前文件")
	}
	return os.Rename(file.Name(), path)
}

// Keep the full manual snippet available without depending on a repository
// checkout when the installed binary cannot safely merge user configuration.
func manualProtectionConfig() string {
	return strings.TrimSpace(protectionSelection + protectionProfile)
}
