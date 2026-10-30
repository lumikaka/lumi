package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestGuidanceCreatesAndPreservesFiles(t *testing.T) {
	root := t.TempDir()
	first := ensureGuidance(root)
	if first.Agents != "created" || first.Config != "created" {
		t.Fatalf("first=%+v", first)
	}
	configPath := filepath.Join(root, ".codex", "config.toml")
	original, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"Lumi 插件工具", "禁止直接读取或写入", "STORY.md 是只读", "不限制读取", "人工确认"} {
		if !strings.Contains(string(agents), required) {
			t.Fatalf("missing guidance %q", required)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("user agents"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(configPath)
	second := ensureGuidance(root)
	if second.Agents != "preserved" || second.Config != "preserved" {
		t.Fatalf("second=%+v", second)
	}
	after, _ := os.ReadFile(configPath)
	afterInfo, _ := os.Stat(configPath)
	if !bytes.Equal(original, after) || !info.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatal("idempotent config changed")
	}
	agents, _ = os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if string(agents) != "user agents" {
		t.Fatal("overwrote user AGENTS")
	}
	protection := inspectProtection(root)
	if !protection.Configured || protection.Effective != "unknown" {
		t.Fatalf("protection=%+v", protection)
	}
	var config map[string]any
	if err := toml.Unmarshal(after, &config); err != nil {
		t.Fatal(err)
	}
	profile := config["permissions"].(map[string]any)["lumi-protect"].(map[string]any)
	rules := profile["filesystem"].(map[string]any)[":workspace_roots"].(map[string]any)
	for _, name := range []string{"project.sqlite", "project.sqlite-wal", "project.sqlite-shm", "project.sqlite-journal", ".lumi", "assets"} {
		if rules[name] != "read" {
			t.Fatalf("rule %s=%v", name, rules[name])
		}
	}
}

func TestGuidanceConfigMergeAndRefusal(t *testing.T) {
	for _, test := range []struct {
		name, original string
		want           string
		warning        bool
	}{
		{"comments-and-tables", "# user comment\nmodel = 'user-model'\n\n[mcp_servers.mine]\ncommand = 'user-command'\n", "merged", false},
		{"other-profile", "[permissions.user]\nextends = ':read-only'\n", "merged", false},
		{"same-selection", protectionSelection, "merged", false},
		{"same-profile-no-selection", protectionProfile, "merged", false},
		{"other-selection", "default_permissions = ':read-only'\n", "manual", false},
		{"named-profile-selection", "[profiles.user]\ndefault_permissions = ':read-only'\n", "manual", false},
		{"conflicting-profile", "[permissions.lumi-protect]\nextends = ':read-only'\n", "manual", false},
		{"invalid-toml", "[broken", "manual", false},
		{"sealed-inline-table", "permissions = {}\n", "manual", false},
		{"legacy-sandbox", "sandbox_mode = 'workspace-write'\n", "merged", true},
		{"legacy-named-profile", "[profiles.user]\nsandbox_mode = 'workspace-write'\n", "merged", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, ".codex"), 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, ".codex", "config.toml")
			if err := os.WriteFile(path, []byte(test.original), 0o600); err != nil {
				t.Fatal(err)
			}
			before, _ := os.Stat(path)
			result := ensureGuidance(root)
			if result.Config != test.want {
				t.Fatalf("result=%+v", result)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			afterInfo, _ := os.Stat(path)
			if test.want == "manual" {
				if string(after) != test.original || !before.ModTime().Equal(afterInfo.ModTime()) {
					t.Fatal("refusal changed config")
				}
				if !strings.Contains(strings.Join(result.Messages, "\n"), "手工") {
					t.Fatal("missing manual instructions")
				}
			} else {
				if !bytes.Contains(after, []byte(test.original)) || afterInfo.Mode().Perm() != 0o600 {
					t.Fatal("lost original content or permissions")
				}
				if !inspectProtection(root).Configured {
					t.Fatal("merged config not selected")
				}
				if again := ensureGuidance(root); again.Config != "preserved" {
					t.Fatalf("again=%+v", again)
				}
			}
			if test.warning && !strings.Contains(strings.Join(result.Messages, "\n"), "sandbox_mode") {
				t.Fatal("missing legacy warning")
			}
		})
	}
}

func TestGuidanceRejectsSymlinksAndConcurrentReplacement(t *testing.T) {
	for _, directoryLink := range []bool{true, false} {
		root, outside := t.TempDir(), t.TempDir()
		external := filepath.Join(outside, "config.toml")
		if err := os.WriteFile(external, []byte("# external"), 0o644); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(root, ".codex")
		target := outside
		if !directoryLink {
			if err := os.Mkdir(link, 0o755); err != nil {
				t.Fatal(err)
			}
			link = filepath.Join(link, "config.toml")
			target = external
		}
		if err := os.Symlink(target, link); err != nil {
			t.Skip(err)
		}
		if result := ensureGuidance(root); result.Config != "manual" {
			t.Fatalf("symlink result=%+v", result)
		}
		content, _ := os.ReadFile(external)
		if string(content) != "# external" {
			t.Fatal("wrote outside project")
		}
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("# original"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if err := os.WriteFile(path, []byte("# concurrent edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceGuidanceConfig(path, []byte("# original"), []byte(protectionSelection+protectionProfile), info); err == nil {
		t.Fatal("overwrote concurrent edit")
	}
	content, _ := os.ReadFile(path)
	if string(content) != "# concurrent edit" {
		t.Fatal("lost concurrent edit")
	}
}
