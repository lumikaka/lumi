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
	"lumi/internal/cindyarchive"
	"lumi/internal/config"
	"lumi/internal/project"
)

func TestManualCindyImportCLIWebSocketReadOnlyAndDesktopReopen(t *testing.T) {
	opt := cliOptions(t)
	root := t.TempDir()
	p := runProjectCLI(t, opt, root, "init", "--name", "Synthetic Cindy history")
	configPath := filepath.Join(root, ".codex", "config.toml")
	configBefore, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../internal/cindyarchive/testdata/public-history.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "explicit-export.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	status := cliRun(t, opt, "status", 0)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(status.Endpoint, "http")+"/api/v1/ws", http.Header{"Origin": []string{status.Endpoint}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteJSON(map[string]any{"topic": "project:" + p.UUID, "event": "phx_join", "payload": map[string]any{}, "ref": "1", "join_ref": "cindy-join"}); err != nil {
		t.Fatal(err)
	}
	var frame map[string]any
	if err := conn.ReadJSON(&frame); err != nil || frame["event"] != "phx_reply" {
		t.Fatalf("join=%v %v", frame, err)
	}
	run := func() cindyarchive.Result {
		t.Helper()
		cmd := exec.Command(os.Args[0], "cindy-session", "import", "--file", path, "--source-session", "example-host-task", "--json", "--data-dir", opt.dataDir, "--environment", opt.environment, "--timeout", "20s")
		cmd.Dir = root
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("manual import=%v %s %s", err, output, stderr.String())
		}
		var envelope struct {
			Success bool                `json:"success"`
			Data    cindyarchive.Result `json:"data"`
		}
		if err := json.Unmarshal(output, &envelope); err != nil || !envelope.Success {
			t.Fatalf("response=%s %v", output, err)
		}
		if bytes.Contains(output, []byte("再写一页。")) || bytes.Contains(output, []byte("source_session_id")) {
			t.Fatal("CLI summary leaked source/body")
		}
		return envelope.Data
	}
	first := run()
	if first.Imported != 3 || first.ThreadUUID == "" || first.Replayed {
		t.Fatalf("first=%+v", first)
	}
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatal(err)
	}
	payload, _ := frame["payload"].(map[string]any)
	if frame["event"] != "cindy_session:changed" || frame["join_ref"] != "cindy-join" || len(payload) != 2 || payload["thread_uuid"] != first.ThreadUUID || payload["project_uuid"] != p.UUID {
		t.Fatalf("unsafe hint=%v", frame)
	}
	again := run()
	if !again.Replayed || again.Imported != 0 || again.ImportUUID != first.ImportUUID {
		t.Fatalf("replay=%+v", again)
	}
	changed := bytes.ReplaceAll(raw, []byte("再写一页。"), []byte("CONFIDENTIAL_ARCHIVE_BODY"))
	if bytes.Equal(changed, raw) {
		t.Fatal("conflict fixture did not change")
	}
	if _, err := forwardCindySessionImport(t.Context(), status.instance, cindySessionImportRequest{RootPath: root, SourceSessionID: "example-host-task", Export: changed}); err == nil || err.Error() != "cindy_archive_source_changed" {
		t.Fatalf("source rewrite=%v", err)
	}
	_ = conn.WriteJSON(map[string]any{"topic": "phoenix", "event": "heartbeat", "payload": map[string]any{}, "ref": "2"})
	if err := conn.ReadJSON(&frame); err != nil || frame["event"] != "phx_reply" {
		t.Fatalf("replay emitted a change=%v %v", frame, err)
	}
	base := "/api/v1/projects/" + p.UUID + "/chat_threads"
	request := func(method, path, body string, want int) []byte {
		t.Helper()
		return apiRequest(t, status.Endpoint, method, path, body, "", "", "", want).data
	}
	threads := request("GET", base, "", 200)
	if !bytes.Contains(threads, []byte(`"thread_type":"cindy_session"`)) {
		t.Fatal("imported thread missing")
	}
	items := request("GET", base+"/"+first.ThreadUUID+"/cindy_session_items?limit=2", "", 200)
	var page cindyarchive.Page
	if json.Unmarshal(items, &page) != nil || len(page.Items) != 2 || !page.CursorPagination.HasMore || page.Coverage.Kind != "manual_public_text" {
		t.Fatalf("history=%s", items)
	}
	for _, secret := range []string{`"id":`, "source_message_id", "source_text_sha256", "sort_at", "workingDir", "example-host-task", path} {
		if bytes.Contains(items, []byte(secret)) {
			t.Fatalf("private history field=%s", secret)
		}
	}
	request("POST", base+"/"+first.ThreadUUID+"/turns", `{"input_text":"continue"}`, 422)
	request("POST", base+"/"+first.ThreadUUID+"/follow_ups", `{"input_text":"continue"}`, 422)
	request("GET", base+"/"+first.ThreadUUID+"/trajectory", "", 422)
	request("GET", base+"/"+first.ThreadUUID+"/cindy_session_items?limit=bad", "", 422)
	request("GET", base+"/"+first.ThreadUUID+"/codex_session_items", "", 404)
	if _, err := forwardCindySessionImport(t.Context(), instance{Endpoint: status.Endpoint, Nonce: "stale"}, cindySessionImportRequest{RootPath: root, SourceSessionID: "example-host-task", Export: raw}); err == nil {
		t.Fatal("stale CLI generation accepted")
	}
	apiRequest(t, status.Endpoint, "POST", cindySessionImportsRoute, `{}`, "https://evil.example", "", "", 403)
	configAfter, _ := os.ReadFile(configPath)
	if !bytes.Equal(configBefore, configAfter) {
		t.Fatal("manual Cindy import changed Codex configuration")
	}
	if _, err := os.Stat(filepath.Join(root, ".codex", "hooks.json")); !os.IsNotExist(err) {
		t.Fatal("manual import installed hooks")
	}
	cliRun(t, opt, "stop", 0)
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
	reader := &cindyarchive.Service{Projects: desktop}
	saved, err := reader.History(t.Context(), opened.UUID, first.ThreadUUID, "", "", 100)
	if err != nil || len(saved.Items) != 3 {
		t.Fatalf("desktop reopen=%+v %v", saved, err)
	}
	// Replacing the file and merely reading history never collects more text.
	if err := os.WriteFile(path, []byte("invalid future format"), 0600); err != nil {
		t.Fatal(err)
	}
	saved, err = reader.History(t.Context(), opened.UUID, first.ThreadUUID, "", "", 100)
	if err != nil || len(saved.Items) != 3 {
		t.Fatal("manual-only history changed without import")
	}
	log, _ := os.ReadFile(opt.path(".log"))
	for _, secret := range []string{"再写一页。", "CONFIDENTIAL_ARCHIVE_BODY", "example-host-task", "source_message_id", "source_text_sha256", "/example/book"} {
		if bytes.Contains(log, []byte(secret)) {
			t.Fatalf("archive leaked into CLI log: %s", secret)
		}
	}
}

func TestCindyImportRequiresExplicitFileAndSourceWithoutStartingServer(t *testing.T) {
	opt := cliOptions(t)
	for _, args := range [][]string{
		{"cindy-session", "import"},
		{"cindy-session", "import", "--file", "missing.json"},
		{"cindy-session", "import", "--file", "missing.json", "--source-session", "explicit"},
	} {
		args = append(args, "--json", "--data-dir", opt.dataDir)
		var out, stderr bytes.Buffer
		if code := command(args, &out, &stderr); code == 0 || !bytes.Contains(out.Bytes(), []byte(`"success":false`)) {
			t.Fatalf("args=%v code=%d out=%s", args, code, out.String())
		}
	}
	cliRun(t, opt, "status", 3)
}
