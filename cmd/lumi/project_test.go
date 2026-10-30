package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
)

func runProjectCLI(t *testing.T, opt options, root string, args ...string) commandProjectInfo {
	t.Helper()
	args = append(args, "--json", "--environment", opt.environment, "--data-dir", opt.dataDir, "--timeout", "20s")
	cmd := exec.Command(os.Args[0], args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		log, _ := os.ReadFile(opt.path(".log"))
		t.Fatalf("project command %v: %v stdout=%s stderr=%s log=%s", args, err, output, stderr.String(), log)
	}
	var envelope struct {
		Success bool               `json:"success"`
		Data    commandProjectInfo `json:"data"`
	}
	if err := json.Unmarshal(output, &envelope); err != nil || !envelope.Success {
		t.Fatalf("JSON=%s err=%v", output, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(output, &raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(output), `"id":`) {
		t.Fatalf("exposed internal id: %s", output)
	}
	return envelope.Data
}

type fileSnapshot struct {
	Mode fs.FileMode
	Hash [32]byte
	Size int64
}

func projectFiles(t *testing.T, root string) map[string]fileSnapshot {
	t.Helper()
	result := map[string]fileSnapshot{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := fileSnapshot{Mode: info.Mode()}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value.Hash, value.Size = sha256.Sum256(data), info.Size()
		}
		result[relative] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestInitCLIStartsInstanceOpensProjectAndSharesDesktopFormat(t *testing.T) {
	opt := cliOptions(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := runProjectCLI(t, opt, root, "init", "--name", "CLI 项目", "--language", "en")
	if result.Created == nil || !*result.Created || !result.Open || result.Name != "CLI 项目" || !result.WriteProtection.Configured || result.WriteProtection.Effective != "unknown" {
		t.Fatalf("init=%+v", result)
	}
	status := cliRun(t, opt, "status", 0)
	if len(status.OpenProjects) != 1 || status.OpenProjects[0].UUID != result.UUID {
		t.Fatalf("status=%+v", status)
	}
	info := runProjectCLI(t, opt, root, "project", "info")
	if info.UUID != result.UUID || !info.Open || info.Created != nil {
		t.Fatalf("info=%+v", info)
	}
	// Exercise the actual daemon resource's generation check.
	input, _ := json.Marshal(initializationRequest{RootPath: root})
	if _, err := initializeThroughInstance(t.Context(), instance{Endpoint: status.Endpoint, Nonce: "stale"}, initializationRequest{RootPath: root}); err == nil {
		t.Fatal("stale generation accepted")
	}
	apiRequest(t, status.Endpoint, "POST", initializationRoute, string(input), "https://evil.example", "", "", 403)
	cliRun(t, opt, "stop", 0)
	before := projectFiles(t, root)
	again := runProjectCLI(t, opt, root, "init", "--name", "ignored", "--language", "invalid")
	if again.Created == nil || *again.Created || again.Open || again.UUID != result.UUID {
		t.Fatalf("again=%+v", again)
	}
	if !reflect.DeepEqual(before, projectFiles(t, root)) {
		t.Fatal("idempotent CLI init changed project files")
	}
	cliRun(t, opt, "status", 3) // An existing project never starts a daemon.
	closed := runProjectCLI(t, opt, root, "project", "info")
	if closed.Open || !reflect.DeepEqual(before, projectFiles(t, root)) {
		t.Fatal("closed info modified or opened project")
	}
	app, err := appstore.Open(opt.dataDir, config.SQLiteDSN(filepath.Join(opt.dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	recent, err := app.RecentProject(t.Context(), result.UUID)
	if err != nil || recent.RootPath != result.RootPath {
		t.Fatalf("recent=%+v err=%v", recent, err)
	}
	desktop := project.NewManager(app)
	defer desktop.Close()
	opened, err := desktop.OpenSelected(t.Context(), project.ExplicitExistingDirectory(root))
	if err != nil || opened.UUID != result.UUID || !opened.Open {
		t.Fatalf("desktop open=%+v err=%v", opened, err)
	}
	// Info also reports a desktop-owned project as open, using the OS lock.
	desktopInfo := runProjectCLI(t, opt, root, "project", "info")
	if !desktopInfo.Open {
		t.Fatal("desktop-owned project reported closed")
	}
}

func TestDesktopProjectInitAddsOnlyOptionalGuidance(t *testing.T) {
	opt := cliOptions(t)
	app, err := appstore.Open(opt.dataDir, config.SQLiteDSN(filepath.Join(opt.dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	desktop := project.NewManager(app)
	defer desktop.Close()
	created, err := desktop.Create(t.Context(), "桌面项目", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := desktop.CloseProject(t.Context(), created.UUID); err != nil {
		t.Fatal(err)
	}
	before := projectFiles(t, created.RootPath)
	withoutGuidance := runProjectCLI(t, opt, created.RootPath, "project", "info")
	if withoutGuidance.WriteProtection.Configured || withoutGuidance.WriteProtection.Effective != "unknown" {
		t.Fatalf("protection without guidance=%+v", withoutGuidance.WriteProtection)
	}
	if !reflect.DeepEqual(before, projectFiles(t, created.RootPath)) {
		t.Fatal("info without guidance changed desktop project")
	}
	recentBefore, err := app.RecentProject(t.Context(), created.UUID)
	if err != nil {
		t.Fatal(err)
	}
	result := runProjectCLI(t, opt, created.RootPath, "init", "--name", "ignored")
	if result.UUID != created.UUID || *result.Created || result.Guidance.Agents != "created" || result.Guidance.Config != "created" {
		t.Fatalf("result=%+v", result)
	}
	after := projectFiles(t, created.RootPath)
	for name, value := range before {
		if after[name] != value {
			t.Fatalf("changed desktop file %s", name)
		}
	}
	for name := range after {
		if _, exists := before[name]; !exists && name != "AGENTS.md" && name != ".codex" && name != filepath.Join(".codex", "config.toml") {
			t.Fatalf("unexpected added file %s", name)
		}
	}
	recentAfter, err := app.RecentProject(t.Context(), created.UUID)
	if err != nil || !reflect.DeepEqual(recentBefore, recentAfter) {
		t.Fatal("existing init updated recent index")
	}
	cliRun(t, opt, "status", 3)
	if _, err := desktop.OpenSelected(t.Context(), project.ExplicitExistingDirectory(created.RootPath)); err != nil {
		t.Fatal(err)
	}
	// Existing init succeeds without disturbing a desktop-held project lock.
	result = runProjectCLI(t, opt, created.RootPath, "init")
	if !result.Open || *result.Created {
		t.Fatalf("desktop-held init=%+v", result)
	}
}

func TestInitCLIConflictAndNonProjectInfo(t *testing.T) {
	opt := cliOptions(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := projectFiles(t, root)
	for _, args := range [][]string{{"init"}, {"project", "info"}} {
		cmd := exec.Command(os.Args[0], append(args, "--json", "--data-dir", opt.dataDir, "--timeout", "20s")...)
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected error: %s", output)
		}
		var envelope struct {
			Success bool `json:"success"`
			Data    any  `json:"data"`
			Error   struct {
				Code    string `json:"code"`
				Details string `json:"details"`
			} `json:"error"`
		}
		if err := json.Unmarshal(output, &envelope); err != nil || envelope.Success || envelope.Data != nil {
			t.Fatalf("error JSON=%s err=%v", output, err)
		}
		if args[0] == "init" && (envelope.Error.Code != project.CodeInitializationConflict || envelope.Error.Details == "") {
			t.Fatalf("error=%s", output)
		}
		if !reflect.DeepEqual(before, projectFiles(t, root)) {
			t.Fatal("failure modified cwd")
		}
	}
}

func TestInitCLIWithExistingCodexConfiguration(t *testing.T) {
	opt := cliOptions(t)
	for _, test := range []struct{ config, want string }{
		{"# user comment\nmodel = 'user-model'\n", "merged"},
		{"default_permissions = ':read-only'\n", "manual"},
	} {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".codex"), 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, ".codex", "config.toml")
		if err := os.WriteFile(path, []byte(test.config), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("user instructions"), 0o644); err != nil {
			t.Fatal(err)
		}
		result := runProjectCLI(t, opt, root, "init")
		if result.Guidance.Config != test.want || result.Guidance.Agents != "preserved" {
			t.Fatalf("guidance=%+v", result.Guidance)
		}
		content, _ := os.ReadFile(path)
		if !bytes.Contains(content, []byte(test.config)) || (test.want == "manual" && string(content) != test.config) {
			t.Fatal("lost existing config")
		}
		if test.want == "manual" && !strings.Contains(strings.Join(result.Guidance.Messages, "\n"), protectionSelection) {
			t.Fatal("missing manual snippet")
		}
		agents, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
		if string(agents) != "user instructions" {
			t.Fatal("overwrote user AGENTS")
		}
	}
}
