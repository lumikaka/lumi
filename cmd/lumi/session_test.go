package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
	"lumi/internal/sessionimport"
)

func runSessionCLI(t *testing.T, opt options, root, rollout string) sessionimport.Result {
	t.Helper()
	cmd := exec.Command(os.Args[0], "session", "import", "--rollout", rollout, "--json", "--environment", opt.environment, "--data-dir", opt.dataDir, "--timeout", "20s")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		t.Fatalf("import: %v %s %s", err, b, stderr.String())
	}
	var result struct {
		Success bool                 `json:"success"`
		Data    sessionimport.Result `json:"data"`
	}
	if err := json.Unmarshal(b, &result); err != nil || !result.Success {
		t.Fatalf("%s %v", b, err)
	}
	return result.Data
}

func TestManualSessionCLIAndDesktopHistory(t *testing.T) {
	opt := cliOptions(t)
	root := t.TempDir()
	p := runProjectCLI(t, opt, root, "init", "--name", "Imported history")
	configPath := filepath.Join(root, ".codex", "config.toml")
	configBefore, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("../../internal/sessionimport/testdata/desktop-recorded.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	encodedRoot, _ := json.Marshal(root)
	fixture = bytes.ReplaceAll(fixture, []byte(`"PROJECT_ROOT"`), encodedRoot)
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, fixture, 0600); err != nil {
		t.Fatal(err)
	}
	status := cliRun(t, opt, "status", 0)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(status.Endpoint, "http")+"/api/v1/ws", http.Header{"Origin": []string{status.Endpoint}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(map[string]any{"topic": "project:" + p.UUID, "event": "phx_join", "payload": map[string]any{}, "ref": "1", "join_ref": "join-1"}); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var frame map[string]any
	if err := conn.ReadJSON(&frame); err != nil || frame["event"] != "phx_reply" {
		t.Fatalf("join: %v %v", frame, err)
	}
	first := runSessionCLI(t, opt, root, path)
	if first.Imported != 3 || first.ThreadUUID == "" {
		t.Fatal(first)
	}
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatal(err)
	}
	payload, _ := frame["payload"].(map[string]any)
	if frame["event"] != "codex_session:changed" || frame["join_ref"] != "join-1" || payload["thread_uuid"] != first.ThreadUUID || payload["project_uuid"] != p.UUID || len(payload) != 2 {
		t.Fatalf("unsafe event: %v", frame)
	}
	again := runSessionCLI(t, opt, root, path)
	if again.Imported != 0 || again.ThreadUUID != first.ThreadUUID {
		t.Fatal(again)
	}
	_ = conn.WriteJSON(map[string]any{"topic": "phoenix", "event": "heartbeat", "payload": map[string]any{}, "ref": "2"})
	if err := conn.ReadJSON(&frame); err != nil || frame["event"] != "phx_reply" {
		t.Fatalf("duplicate import emitted event: %v %v", frame, err)
	}
	_ = conn.Close()
	base := "/api/v1/projects/" + p.UUID + "/chat_threads"
	request := func(method, path, body string, want int) []byte {
		t.Helper()
		return apiRequest(t, status.Endpoint, method, path, body, "", "", "", want).data
	}
	threads := request(http.MethodGet, base, "", 200)
	if !bytes.Contains(threads, []byte(`"thread_type":"codex_session"`)) {
		t.Fatalf("missing imported thread: %s", threads)
	}
	items := request(http.MethodGet, base+"/"+first.ThreadUUID+"/codex_session_items?limit=2", "", 200)
	for _, private := range []string{`"id":`, "rollout_path", "source_offset", path} {
		if strings.Contains(string(items), private) {
			t.Fatalf("private field leaked: %s", private)
		}
	}
	request(http.MethodPost, base+"/"+first.ThreadUUID+"/turns", `{"input_text":"continue"}`, 422)
	request(http.MethodPost, base+"/"+first.ThreadUUID+"/follow_ups", `{"input_text":"continue"}`, 422)
	request(http.MethodGet, base+"/"+first.ThreadUUID+"/trajectory", "", 422)
	request(http.MethodGet, base+"/019a0000-0000-7000-8000-000000000099/codex_session_items", "", 404)
	request(http.MethodGet, base+"/"+first.ThreadUUID+"/codex_session_items?limit=bad", "", 422)
	if _, err := forwardSessionImport(t.Context(), instance{Endpoint: status.Endpoint, Nonce: "stale"}, sessionImportRequest{RootPath: root, Rollout: path}); err == nil {
		t.Fatal("stale CLI generation accepted")
	}
	apiRequest(t, status.Endpoint, "POST", sessionImportsRoute, `{}`, "https://evil.example", "", "", 403)
	configAfter, _ := os.ReadFile(configPath)
	if !bytes.Equal(configBefore, configAfter) {
		t.Fatal("manual import changed Codex configuration")
	}
	if _, err := os.Stat(filepath.Join(root, ".codex", "hooks.json")); !os.IsNotExist(err) {
		t.Fatal("manual import installed hooks")
	}
	cliRun(t, opt, "stop", 0)
	// The normal desktop project manager reopens the same format and data.
	app, err := appstore.Open(t.TempDir(), config.SQLiteDSN(filepath.Join(t.TempDir(), "desktop.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	desktop := project.NewManager(app)
	defer desktop.Close()
	opened, err := desktop.OpenSelected(t.Context(), project.ExplicitExistingDirectory(root))
	if err != nil {
		t.Fatal(err)
	}
	reader := &sessionimport.Service{Projects: desktop}
	page, err := reader.History(t.Context(), opened.UUID, first.ThreadUUID, "", "", 50)
	if err != nil || len(page.Items) != 3 {
		t.Fatalf("desktop: %+v %v", page, err)
	}
	// Appending alone never imports; imported records remain after CLI stops.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("{\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"role\":\"user\",\"content\":[{\"type\":\"input_text\",\"text\":\"not automatically imported\"}]}}\n")
	_ = f.Close()
	page, err = reader.History(t.Context(), opened.UUID, first.ThreadUUID, "", "", 50)
	if err != nil || len(page.Items) != 3 {
		t.Fatal("automatic import occurred")
	}
}

func TestSessionManualOnlyArgumentsAndMissingRollout(t *testing.T) {
	for _, args := range [][]string{{"session", "enable", "--json"}, {"session", "disable", "--json"}, {"session", "import", "--json"}} {
		var out, stderr bytes.Buffer
		if code := command(args, &out, &stderr); code != 2 || !strings.Contains(out.String(), `"success":false`) {
			t.Fatalf("%v: %d %s", args, code, out.String())
		}
	}
	opt := cliOptions(t)
	r := runSessionCLI(t, opt, t.TempDir(), filepath.Join(t.TempDir(), "missing.jsonl"))
	if !r.Skipped || len(r.Warnings) == 0 {
		t.Fatal(r)
	}
	cliRun(t, opt, "status", 3)
}
