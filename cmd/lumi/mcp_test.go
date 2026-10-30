package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/realtime"
	"lumi/internal/sitesettings"
	"lumi/internal/textmcp"
)

func mcpClient(t *testing.T, opt options, root string) *mcp.ClientSession {
	t.Helper()
	cmd := exec.Command(os.Args[0], "mcp", "--data-dir", opt.dataDir, "--environment", opt.environment, "--timeout", "20s")
	cmd.Dir = root
	// Workspace-like env values and an inherited desktop token cannot select
	// a project or change this into a desktop MCP connection.
	cmd.Env = append(os.Environ(), "PWD=/wrong", "CODEX_WORKSPACE_ROOT=/wrong", "LUMI_DESKTOP_ACCESS_TOKEN=unused")
	client, err := mcp.NewClient(&mcp.Implementation{Name: "text-e2e", Version: "1"}, nil).Connect(t.Context(), &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

type toolEnvelope struct {
	Success bool                                    `json:"success"`
	Data    map[string]any                          `json:"data"`
	Error   struct{ Code, Message, Details string } `json:"error"`
}

func callText(t *testing.T, session *mcp.ClientSession, name string, args map[string]any, wantCode string) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("%s result=%+v", name, result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s content=%T", name, result.Content[0])
	}
	if len(text.Text) > textmcp.MaxBytes {
		t.Fatalf("oversize result: %d", len(text.Text))
	}
	var envelope toolEnvelope
	if err := json.Unmarshal([]byte(text.Text), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != wantCode || envelope.Success != (wantCode == "") || result.IsError != (wantCode != "") {
		t.Fatalf("%s(%v): %s", name, args, text.Text)
	}
	if wantCode != "" {
		return map[string]any{"code": envelope.Error.Code, "message": envelope.Error.Message, "details": envelope.Error.Details}
	}
	assertPublicFields(t, envelope.Data)
	return envelope.Data
}

func assertPublicFields(t *testing.T, value any) {
	t.Helper()
	switch x := value.(type) {
	case map[string]any:
		for key, v := range x {
			if key == "id" || strings.HasSuffix(key, "_id") {
				t.Fatalf("internal id leaked: %s", key)
			}
			assertPublicFields(t, v)
		}
	case []any:
		for _, v := range x {
			assertPublicFields(t, v)
		}
	}
}
func field(v map[string]any, key string) map[string]any { return v[key].(map[string]any) }

func joinProject(t *testing.T, endpoint, uuid string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(endpoint, "http")+"/api/v1/ws", http.Header{"Origin": []string{endpoint}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ref := "authoring-ui"
	if err := conn.WriteJSON(realtime.Frame{Topic: realtime.ProjectTopic(uuid), Event: realtime.EventJoin, Payload: map[string]any{}, Ref: &ref, JoinRef: &ref}); err != nil {
		t.Fatal(err)
	}
	var frame realtime.Frame
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatal(err)
	}
	if frame.Event != realtime.EventReply || frame.Topic != realtime.ProjectTopic(uuid) {
		t.Fatalf("join=%+v", frame)
	}
	return conn
}

func expectHint(t *testing.T, conn *websocket.Conn, event string) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var frame realtime.Frame
		if err := conn.ReadJSON(&frame); err != nil {
			t.Fatal(err)
		}
		assertPublicFields(t, frame.Payload)
		if frame.Event == event {
			if frame.JoinRef == nil || *frame.JoinRef != "authoring-ui" {
				t.Fatalf("bad envelope=%+v", frame)
			}
			return
		}
	}
}

func TestStdioTextCreationEndToEndWithoutProviders(t *testing.T) {
	opt := cliOptions(t)
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	client := mcpClient(t, opt, root)
	tools, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 14 {
		t.Fatalf("tools=%d", len(tools.Tools))
	}
	for _, tool := range tools.Tools {
		b, _ := json.Marshal(tool.InputSchema)
		if strings.Contains(string(b), "root_path") {
			t.Fatal("model can choose root")
		}
	}
	callText(t, client, "project_overview", map[string]any{}, "project_not_initialized")
	init := callText(t, client, "project_init", map[string]any{"idea": "小狐狸在雨夜把迷路的星星送回天空", "name": "回家的星星"}, "")
	projectUUID := field(init, "project")["uuid"].(string)
	if field(init, "setup")["setup_status"] != "draft" {
		t.Fatalf("init=%v", init)
	}
	// A real web UI subscribes through this same socket, holding CLI presence.
	status := cliRun(t, opt, "status", 0)
	conn := joinProject(t, status.Endpoint, projectUUID)
	setup := callText(t, client, "project_setup", map[string]any{"action": "update", "expected_revision": field(init, "setup")["revision"], "project_name": "回家的星星", "generation_language": "zh-Hans", "overall_style": "温暖水彩，靛蓝雨夜", "generation_brief": "一只狐狸学会帮助别人，也找到自己的勇气。", "picture_book": map[string]any{"format": "classic_picture_book", "aspect_ratio": map[string]any{"mode": "landscape"}, "large_image_minimal_text": false}}, "")
	expectHint(t, conn, "project:setup_changed")
	callText(t, client, "project_setup", map[string]any{"action": "finalize", "expected_revision": setup["revision"]}, "")
	expectHint(t, conn, "project:setup_changed")
	overview := callText(t, client, "project_overview", map[string]any{}, "")
	if field(overview, "project")["setup_status"] != "ready" {
		t.Fatalf("overview=%v", overview)
	}
	// The complete prompt includes catalog output format, project facts and a
	// write target/revision; fixed text simulates Codex, never a Lumi provider.
	prompt := callText(t, client, "render_prompt", map[string]any{"prompt_group": "story", "prompt_key": "story_profile", "target": "story_profile", "variables": map[string]string{"chapter_count": "1"}}, "")
	promptText := prompt["prompt"].(string)
	for _, part := range []string{"小狐狸", "story_md", "chapter_plans", projectUUID, prompt["target_uuid"].(string), "expected_revision"} {
		if !strings.Contains(promptText, part) {
			t.Fatalf("prompt missing %q", part)
		}
	}
	profile := callText(t, client, "story_profile", map[string]any{"action": "write", "story_md": "# STORY.md\n\n小狐狸发现一颗迷路的星星，穿过森林送它回家。\n\n## 人物\n小狐狸：怕黑却乐于助人。", "expected_revision": prompt["expected_revision"]}, "")
	expectHint(t, conn, "story:profile_changed")
	callText(t, client, "story_profile", map[string]any{"action": "write", "story_md": "stale", "expected_revision": prompt["expected_revision"]}, "story_profile_revision_conflict")
	if profile["uuid"] == "" {
		t.Fatal("missing profile UUID")
	}
	chapter := callText(t, client, "chapters", map[string]any{"action": "create", "chapter_code": "vol01.ch01", "title": "雨夜的星星", "expected_revision": field(overview, "project")["revision"]}, "")
	chapterUUID := chapter["uuid"].(string)
	callText(t, client, "chapters", map[string]any{"action": "write", "chapter_uuid": chapterUUID, "content": "雨落下来时，小狐狸在草丛里找到星星。它撑起树叶伞，带星星走过森林，爬上山顶。天亮前，星星终于回到了天空。", "expected_revision": chapter["revision"]}, "")
	settingList := callText(t, client, "settings", map[string]any{"action": "list"}, "")
	setting := callText(t, client, "settings", map[string]any{"action": "create", "asset_type": "character", "title": "小狐狸", "summary": "橘红毛，白胸口，穿黄色雨衣；大尾巴，神情坚定。", "tags": []string{"主角"}, "expected_revision": field(settingList, "premise")["revision"]}, "")
	if setting["current_variant"] != nil {
		t.Fatal("text setting unexpectedly has image")
	}
	second := mcpClient(t, opt, root)
	pageUUIDs := []string{}
	for i := 1; i <= 3; i++ {
		list := callText(t, client, "pages", map[string]any{"action": "list", "chapter_uuid": chapterUUID}, "")
		rev := field(list, "state")["revision"]
		page := callText(t, client, "pages", map[string]any{"action": "create", "chapter_uuid": chapterUUID, "title": fmt.Sprintf("第%d页", i), "description_md": fmt.Sprintf("第%d页旁白：一起回家。", i), "storyboard_md": fmt.Sprintf("画面%d：雨夜森林，狐狸举起叶子伞，星星发出金光。宽景，柔和水彩，文字在上方留白。", i), "expected_revision": rev}, "")
		pageUUIDs = append(pageUUIDs, page["uuid"].(string))
		// Independent stdio connections share one instance and collection CAS.
		callText(t, second, "pages", map[string]any{"action": "create", "chapter_uuid": chapterUUID, "title": "旧计划", "description_md": "旧", "storyboard_md": "旧画面", "expected_revision": rev}, "production_conflict")
	}
	expectHint(t, conn, "comic:section_changed")
	page := callText(t, second, "pages", map[string]any{"action": "get", "chapter_uuid": chapterUUID, "page_uuid": pageUUIDs[0]}, "")
	updated := callText(t, second, "pages", map[string]any{"action": "update", "chapter_uuid": chapterUUID, "page_uuid": pageUUIDs[0], "description_md": "第一页旁白：我陪你回家。", "storyboard_md": "狐狸为星星撑伞，眼神温柔；金光照亮雨滴。", "expected_revision": page["revision"]}, "")
	callText(t, client, "pages", map[string]any{"action": "update", "chapter_uuid": chapterUUID, "page_uuid": pageUUIDs[0], "description_md": "不能覆盖", "storyboard_md": "不能覆盖画面", "expected_revision": page["revision"]}, "production_conflict")
	reread := callText(t, client, "pages", map[string]any{"action": "get", "chapter_uuid": chapterUUID, "page_uuid": pageUUIDs[0]}, "")
	if reread["revision"] != updated["revision"] || reread["description_md"] != "第一页旁白：我陪你回家。" || field(reread, "current_storyboard")["content_md"] != "狐狸为星星撑伞，眼神温柔；金光照亮雨滴。" {
		t.Fatalf("partial/stale page write=%v", reread)
	}
	structure := callText(t, client, "book_outline", map[string]any{"limit": 2}, "")
	if len(structure["items"].([]any)) != 2 || !field(structure, "cursor_pagination")["has_more"].(bool) {
		t.Fatalf("outline=%v", structure)
	}
	last := callText(t, client, "book_outline", map[string]any{"limit": 2, "after": field(structure, "cursor_pagination")["next_cursor"]}, "")
	if len(last["items"].([]any)) != 2 {
		t.Fatalf("last=%v", last)
	}
	callText(t, client, "export_book", map[string]any{"format": "zip", "allow_missing_images": false}, "comic_export_incomplete")
	exported := callText(t, client, "export_book", map[string]any{"format": "zip", "allow_missing_images": true}, "")
	path := exported["path"].(string)
	if info, err := os.Stat(path); err != nil || info.Size() == 0 || !strings.HasPrefix(path, root+string(os.PathSeparator)) {
		t.Fatalf("export=%v err=%v", exported, err)
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	var bookJSON, markdown string
	for _, f := range archive.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(r)
		r.Close()
		if f.Name == "book.md" {
			markdown = string(b)
		}
		if f.Name == "book.json" {
			bookJSON = string(b)
		}
	}
	for _, text := range []string{"STORY.md", "小狐狸", "黄色雨衣", "第一页旁白", "画面2", "画面3"} {
		if !strings.Contains(markdown, text) {
			t.Fatalf("export lost %q", text)
		}
	}
	var book map[string]any
	if err := json.Unmarshal([]byte(bookJSON), &book); err != nil {
		t.Fatal(err)
	}
	assertPublicFields(t, book)
	if len(field(book["chapters"].([]any)[0].(map[string]any), "chapter")) == 0 {
		t.Fatal("missing chapter")
	}
	callText(t, client, "export_book", map[string]any{"format": "markdown", "allow_missing_images": true}, "")
	// The host supplies actual paths. Import different ratios through real
	// stdio -> CLI -> Asset Store; no image provider is configured or invoked.
	lastHostPath := ""
	for i, size := range []image.Point{{450, 300}, {300, 300}, {300, 500}} {
		target := callText(t, client, "image_target", map[string]any{"target_uuid": pageUUIDs[i], "slot": "page_image"}, "")
		if field(target, "target_ratio")["width"] != float64(4) || field(target, "recommended_ratio")["width"] != float64(3) || target["suggested_fit"] != "crop" {
			t.Fatalf("target=%v", target)
		}
		fixture := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
		for y := 0; y < size.Y; y++ {
			for x := 0; x < size.X; x++ {
				fixture.SetRGBA(x, y, color.RGBA{R: uint8(x % 251), G: uint8(y % 251), B: 90, A: 255})
			}
		}
		var pngData bytes.Buffer
		if err := png.Encode(&pngData, fixture); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "host-result.jpeg")
		lastHostPath = path // really PNG
		if err := os.WriteFile(path, pngData.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		args := map[string]any{"path": path, "target_uuid": pageUUIDs[i], "slot": "page_image", "expected_revision": target["expected_revision"], "fit_mode": []string{"keep", "crop", "pad"}[i], "prompt": "同一只小狐狸，温暖水彩，保持黄色雨衣。"}
		imported := callText(t, client, "import_image", args, "")
		if imported["ratio_matches"] != false || len(imported["warnings"].([]any)) != 1 || field(field(imported, "asset"), "metadata")["origin"] != "host_generated" {
			t.Fatalf("import=%v", imported)
		}
		expectHint(t, conn, "comic:section_changed")
		callText(t, second, "import_image", args, "production_conflict")
		if i == 0 {
			args["expected_revision"] = imported["revision"]
			callText(t, client, "import_image", args, "")
			expectHint(t, conn, "comic:section_changed")
		}
		// Same REST history used by the web UI includes both versions/origin.
		url := "/api/v1/projects/" + projectUUID + "/chapters/" + chapterUUID + "/comic-sections/" + pageUUIDs[i] + "/image-variants"
		history := apiRequest(t, status.Endpoint, "GET", url, "", "", "", "", 200)
		var versions struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(history.data, &versions); err != nil {
			t.Fatal(err)
		}
		wantCount := 1
		if i == 0 {
			wantCount = 2
		}
		if len(versions.Items) != wantCount || versions.Items[0]["source_type"] != "host_generated" {
			t.Fatalf("UI history=%s", history.data)
		}
	}
	settingTarget := callText(t, client, "image_target", map[string]any{"target_uuid": setting["uuid"], "slot": "setting_image"}, "")
	callText(t, client, "import_image", map[string]any{"path": filepath.Join(root, "not-there.png"), "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": settingTarget["expected_revision"]}, "image_path_unavailable")
	settingArt := callText(t, client, "import_image", map[string]any{"path": lastHostPath, "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": settingTarget["expected_revision"]}, "")
	expectHint(t, conn, "premise:asset_changed")
	// Read the imported setting into the conversation as a generation reference.
	assetUUID := field(settingArt, "asset")["uuid"]
	preview, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "read_image", Arguments: map[string]any{"asset_uuid": assetUUID, "max_edge": 256}})
	if err != nil || preview == nil || preview.IsError || len(preview.Content) != 2 {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	imageContent, ok := preview.Content[1].(*mcp.ImageContent)
	if !ok || imageContent.MIMEType != "image/jpeg" {
		t.Fatalf("image content=%T", preview.Content[1])
	}
	previewConfig, _, err := image.DecodeConfig(bytes.NewReader(imageContent.Data))
	if err != nil || max(previewConfig.Width, previewConfig.Height) > 256 || bytes.Contains([]byte(preview.Content[0].(*mcp.TextContent).Text), []byte("image_data")) {
		t.Fatalf("preview geometry/text config=%v err=%v", previewConfig, err)
	}
	wire, _ := json.Marshal(preview)
	if len(wire) > textmcp.MaxBytes {
		t.Fatal("preview result exceeds MCP bound")
	}
	pdf := callText(t, client, "export_book", map[string]any{"format": "pdf", "allow_missing_images": false}, "")
	pdfData, err := os.ReadFile(pdf["path"].(string))
	if err != nil || !bytes.HasPrefix(pdfData, []byte("%PDF-")) || len(pdfData) < 1000 || pdf["missing_image_count"] != float64(0) {
		t.Fatalf("PDF=%v bytes=%d err=%v", pdf, len(pdfData), err)
	}
	// Read through the same REST API used by the UI after the socket hint.
	rest := apiRequest(t, status.Endpoint, "GET", "/api/v1/projects/"+projectUUID+"/chapters/"+chapterUUID+"/comic-sections/"+pageUUIDs[0], "", "", "", "", 200)
	if !strings.Contains(string(rest.data), "我陪你回家") {
		t.Fatalf("REST did not reread MCP facts: %s", rest.data)
	}
	// Stop the owner before audit inspection; the stdio process itself never
	// opens SQLite. No provider/task/log rows were created by this chain.
	_ = conn.Close()
	_ = client.Close()
	_ = second.Close()
	cliRun(t, opt, "stop", 0)
	app, err := appstore.Open(opt.dataDir, config.SQLiteDSN(filepath.Join(opt.dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	var count int64
	providers := provider.NewService(app, sitesettings.NewMemoryMasterKeyStore())
	defer providers.Close()
	items, err := providers.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Configured || item.Ready || item.HasSecret {
			t.Fatalf("unexpected configured provider: %+v", item)
		}
	}

	manager := project.NewManager(app)
	defer manager.Close()
	opened, err := manager.OpenSelected(t.Context(), project.ExplicitExistingDirectory(root))
	if err != nil {
		t.Fatal(err)
	}
	err = manager.WithStore(t.Context(), opened.UUID, func(store *project.Store) error {
		for _, table := range []string{"llm_logs", "task_runs", "production_task_runs", "comic_image_generations"} {
			if err := store.DB().Table(table).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return fmt.Errorf("unexpected provider activity: %s=%d", table, count)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMCPDesktopLockAndReleaseGuidance(t *testing.T) {
	opt := cliOptions(t)
	desktop := exec.Command(os.Args[0], "__desktop", opt.dataDir)
	desktop.Env = append(withoutDesktopToken(os.Environ()), "LUMI_DESKTOP_ACCESS_TOKEN=desktop-test-token", "APP_ENV=production", "LUMI_DATA_DIR="+opt.dataDir)
	log, err := os.Create(filepath.Join(opt.dataDir, "desktop-mcp-test.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	desktop.Stdout, desktop.Stderr = log, log
	if err := desktop.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = desktop.Process.Kill(); _ = desktop.Wait() })
	var endpoint string
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(filepath.Join(opt.dataDir, "desktop-ready")); err == nil {
			endpoint = string(b)
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if endpoint == "" {
		t.Fatal("desktop not ready")
	}
	session := apiRequest(t, endpoint, "POST", "/api/v1/desktop-sessions", `{"token":"desktop-test-token"}`, endpoint, "", "", 201)
	cookie := session.cookies[0].Name + "=" + session.cookies[0].Value
	created := apiRequest(t, endpoint, "POST", "/api/v1/projects", jsonTextForTest(map[string]any{"name": "Desktop owner", "parent_path": t.TempDir()}), endpoint, cookie, "", 201)
	var p project.Summary
	if err := json.Unmarshal(created.data, &p); err != nil {
		t.Fatal(err)
	}
	client := mcpClient(t, opt, p.RootPath)
	failure := callText(t, client, "project_overview", map[string]any{}, "project_locked")
	if !strings.Contains(failure["details"].(string), "桌面版") || !strings.Contains(failure["details"].(string), "lumi stop") {
		t.Fatalf("missing release advice: %v", failure)
	}
	callText(t, client, "project_init", map[string]any{"idea": "existing project"}, "project_locked")
	status := cliRun(t, opt, "status", 0)
	if status.Endpoint == endpoint || status.PID == desktop.Process.Pid {
		t.Fatal("MCP connected to desktop")
	}
	apiRequest(t, endpoint, "DELETE", "/api/v1/open-projects/"+p.UUID, "", endpoint, cookie, "", 200)
	opened := callText(t, client, "project_overview", map[string]any{}, "")
	if field(opened, "project")["uuid"] != p.UUID {
		t.Fatal("lost cwd binding after release")
	}
}

func jsonTextForTest(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestCreationForwardHasNoOriginOrCredentialsAndRejectsOversize(t *testing.T) {
	oversized := false
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, header := range []string{"Origin", "Authorization", "Cookie", "X-Lumi-Desktop-Access-Token"} {
			if r.Header.Get(header) != "" {
				t.Errorf("forwarded credential/origin %s", header)
			}
		}
		if r.Header.Get("If-Match") != "generation-marker" {
			t.Error("missing instance generation")
		}
		var request creationRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.RootPath != "/bound-cwd" {
			t.Errorf("root=%s", request.RootPath)
		}
		if oversized {
			_, _ = io.WriteString(w, strings.Repeat("x", textmcp.MaxBytes+1))
			return
		}
		_, _ = io.WriteString(w, `{"success":true,"data":{"uuid":"01989abc-def0-7000-8000-000000000001"}}`)
	}))
	defer web.Close()
	identity := instance{Endpoint: web.URL, Nonce: "generation-marker"}
	input := creationRequest{RootPath: "/bound-cwd", Tool: "project_overview", Arguments: json.RawMessage(`{}`)}
	if result := forwardCreation(t.Context(), identity, input); !result.Success {
		t.Fatalf("forward=%+v", result)
	}
	oversized = true
	if result := forwardCreation(t.Context(), identity, input); result.Error == nil || result.Error.Code != "result_too_large" {
		t.Fatalf("oversize=%+v", result)
	}
}
