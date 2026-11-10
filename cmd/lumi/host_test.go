package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/httpapi"
	"lumi/internal/textmcp"
)

func TestMCPHostFlagRejectsUnknownValuesAtStartup(t *testing.T) {
	for _, args := range [][]string{{"mcp", "--host", "vscode"}, {"mcp", "--host="}, {"mcp", "--host", "Claude"}, {"init", "--host", "other"}} {
		var stdout, stderr bytes.Buffer
		if exit := command(args, &stdout, &stderr); exit != 2 || !strings.Contains(stdout.String()+stderr.String(), "--host") {
			t.Fatalf("%v exit=%d out=%s err=%s", args, exit, stdout.String(), stderr.String())
		}
	}
}

func mcpHostClient(t *testing.T, opt options, root, host string, flags ...string) *mcp.ClientSession {
	t.Helper()
	cmd := exec.Command(os.Args[0], "mcp", "--host", host, "--data-dir", opt.dataDir, "--environment", opt.environment, "--timeout", "20s")
	cmd.Args = append(cmd.Args, flags...)
	cmd.Dir = root
	client, err := mcp.NewClient(&mcp.Implementation{Name: "host-e2e", Version: "1"}, nil).Connect(t.Context(), &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// A v5 daemon serves every existing tool; only generate_page_image needs v6.
// The host is forwarded so the daemon can gate host-limited tools.
func TestClaudeHostForwardsHostAndRequiresV6OnlyForPageGeneration(t *testing.T) {
	opt := options{environment: "production", dataDir: t.TempDir(), timeout: 5 * time.Second}
	root := t.TempDir()
	var identity instance
	var mu sync.Mutex
	forwarded := []creationRequest{}
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == instanceRoute && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(httpapi.Envelope{Success: true, Data: instanceStatus{instance: identity, Running: true, CreationProtocolVersion: 5}})
			return
		}
		if r.URL.Path == creationRoute {
			var input creationRequest
			_ = json.NewDecoder(r.Body).Decode(&input)
			mu.Lock()
			forwarded = append(forwarded, input)
			mu.Unlock()
		}
		_ = json.NewEncoder(w).Encode(textmcp.Failure("project_not_initialized", "fake", ""))
	}))
	t.Cleanup(web.Close)
	identity = instance{Endpoint: web.URL, PID: os.Getpid(), Environment: opt.environment, Nonce: strings.Repeat("b", 64)}
	cleanup, err := writeInstance(opt, identity)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	client := mcpHostClient(t, opt, root, "claude")
	callText(t, client, "generate_page_image", map[string]any{"page_uuid": "01989abc-def0-7000-8000-000000000001", "setting_uuids": []string{}, "idempotency_key": "k", "expected_revision": 1}, "cli_upgrade_required")
	callText(t, client, "project_overview", map[string]any{}, "project_not_initialized")
	mu.Lock()
	defer mu.Unlock()
	if len(forwarded) != 1 || forwarded[0].Tool != "project_overview" || forwarded[0].Host != "claude" {
		t.Fatalf("forwarded=%+v", forwarded)
	}
}

func TestClaudeGuidanceCreatesImportOnlyAndNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	result := ensureGuidance(root)
	content, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil || string(content) != "@AGENTS.md\n" || result.Claude != "created" {
		t.Fatalf("claude=%q result=%+v err=%v", content, result, err)
	}
	again := guidanceResult{}
	ensureClaudeGuidance(root, &again)
	if again.Claude != "preserved" {
		t.Fatalf("again=%+v", again)
	}
	custom := []byte("# 我的规则\n保持简洁。\n")
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), custom, 0o644); err != nil {
		t.Fatal(err)
	}
	manual := guidanceResult{}
	ensureClaudeGuidance(root, &manual)
	after, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if manual.Claude != "manual" || !bytes.Equal(after, custom) || !strings.Contains(strings.Join(manual.Messages, ""), "@AGENTS.md") {
		t.Fatalf("manual=%+v after=%q", manual, after)
	}
}
