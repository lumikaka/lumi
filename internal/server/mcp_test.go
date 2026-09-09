package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/files"
	"lumi/internal/mcpbridge"
	"lumi/internal/mcpserver"
	"lumi/internal/project"
	"lumi/internal/realtime"
)

type mcpHarness struct {
	a         *Application
	store     *appstore.Store
	p         string
	grant     mcpserver.Grant
	token     string
	client    *mcp.ClientSession
	discovery string
	backend   *httptest.Server
}

func newMCPHarness(t *testing.T) *mcpHarness {
	t.Helper()
	dir := t.TempDir()
	dsn := config.SQLiteDSN(filepath.Join(dir, "app.sqlite"))
	store, err := appstore.Open(dir, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	manager := project.NewManager(store)
	a, err := New(config.Config{Environment: "development", Address: "127.0.0.1:0", FrontendURL: "http://localhost:5801", ViteDevServerURL: "http://localhost:5802", AppDataDir: dir, DatabaseDSN: dsn}, store, manager)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	p, err := manager.Create(t.Context(), "MCP test", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	grant, token, err := a.mcpService.CreateGrant(t.Context(), p.UUID, "SDK client", "edit")
	if err != nil {
		t.Fatal(err)
	}
	h := &mcpHarness{a: a, store: store, p: p.UUID, grant: grant, token: token, discovery: mcpbridge.Path(dir, "development")}
	h.restartBackend(t)
	h.client = connectMCPBridge(t, h.discovery, token)
	return h
}

func connectMCPBridge(t *testing.T, discovery, token string) *mcp.ClientSession {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go func() {
		err := mcpbridge.Run(ctx, inR, outW, discovery, "development", token)
		outW.CloseWithError(err)
	}()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "official-sdk-integration", Version: "1"}, nil).Connect(t.Context(), &mcp.IOTransport{Reader: outR, Writer: inW}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close(); inR.Close(); outW.Close() })
	return client
}
func (h *mcpHarness) restartBackend(t *testing.T) {
	if h.backend != nil {
		h.backend.Close()
	}
	h.backend = httptest.NewServer(h.a.mcpService.Protocol())
	t.Cleanup(h.backend.Close)
	_, err := mcpbridge.Write(h.discovery, mcpbridge.Instance{Endpoint: h.backend.URL + "/mcp", Environment: "development", Nonce: strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal(err)
	}
}
func (h *mcpHarness) tool(t *testing.T, name string, args map[string]any, wantError bool) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	r, err := h.client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r.StructuredContent)
	var result map[string]any
	if json.Unmarshal(b, &result) != nil {
		t.Fatalf("result=%s", b)
	}
	if r.IsError != wantError {
		t.Fatalf("isError=%v want %v: %s", r.IsError, wantError, b)
	}
	if strings.Contains(string(b), `"id":`) || strings.Contains(string(b), `"token_hash":`) {
		t.Fatalf("internal data leaked: %s", b)
	}
	return result
}
func (h *mcpHarness) request(t *testing.T, method, path string, body map[string]any, key string, wantError bool) map[string]any {
	args := map[string]any{"method": method, "url": "/api/v1/projects/" + h.p + path, "response_filter": ".data | {uuid,name,revision}"}
	if strings.HasPrefix(path, "/chapters") {
		args["response_filter"] = ".data | {uuid,title,revision,trashed_at}"
	}
	if body != nil {
		args["request_body"] = body
	}
	if key != "" {
		args["idempotency_key"] = key
	}
	return h.tool(t, "request_api", args, wantError)
}
func business(result map[string]any) map[string]any {
	return result["data"].(map[string]any)["result"].(map[string]any)["data"].(map[string]any)
}
func callInfo(result map[string]any) map[string]any {
	b, _ := json.Marshal(result["data"].(map[string]any)["call"])
	var c map[string]any
	_ = json.Unmarshal(b, &c)
	return c
}
func TestMCPRealSDKReadEditAuthorizationAndRestart(t *testing.T) {
	h := newMCPHarness(t)
	list, err := h.client.ListTools(t.Context(), nil)
	if err != nil || len(list.Tools) != 4 {
		t.Fatalf("tools=%+v err=%v", list, err)
	}
	h.tool(t, "read_agent_doc", map[string]any{"path": "/api/v1/agent-docs/overview.md"}, false)
	h.tool(t, "read_agent_doc", map[string]any{"path": "/api/v1/agent-docs/../secrets.md"}, true)
	got := h.request(t, "GET", "", nil, "", false)
	if business(got)["uuid"] != h.p {
		t.Fatalf("%+v", got)
	}
	for _, path := range []string{"https://example.com/", "/api/v1/projects/01970000-0000-7000-8000-000000000001", "/api/v1/projects/" + h.p + "/../providers", "/api/v1/projects/" + h.p + "/%2e%2e/providers", "/api/v1/projects/" + h.p + "/llm-logs"} {
		h.tool(t, "request_api", map[string]any{"method": "GET", "url": path, "response_filter": ".data | {uuid,name,revision}"}, true)
	}
	for _, key := range []string{"confirmed", "thread_uuid", "invocation", "tool_execution_uuid", "project_uuid"} {
		h.request(t, "PATCH", "", map[string]any{"name": "bad", "expected_revision": float64(1), key: true}, "bad-"+key, true)
	}
	edit := map[string]any{"name": "Edited by MCP", "description": "hello", "expected_revision": float64(1)}
	first := h.request(t, "PATCH", "", edit, "edit-1", false)
	if business(first)["name"] != "Edited by MCP" {
		t.Fatalf("%+v", first)
	}
	second := h.request(t, "PATCH", "", edit, "edit-1", false)
	if callInfo(second)["uuid"] != callInfo(first)["uuid"] {
		t.Fatal("duplicate write")
	}
	h.request(t, "PATCH", "", edit, "edit-conflict", true)
	conflict := h.request(t, "PATCH", "", map[string]any{"name": "tampered", "description": "changed", "expected_revision": float64(2)}, "edit-1", true)
	if conflict["error"].(map[string]any)["code"] != "mcp_idempotency_conflict" {
		t.Fatalf("wrong idempotency error: %+v", conflict)
	}
	h.restartBackend(t)
	h.tool(t, "get_call", map[string]any{"call_uuid": callInfo(first)["uuid"]}, false)
	h.request(t, "GET", "", nil, "", false)
	readGrant, readToken, err := h.a.mcpService.CreateGrant(t.Context(), h.p, "read only", "read")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.a.mcpService.Authenticate(t.Context(), readToken); err != nil {
		t.Fatal(err)
	}
	readClient := connectMCPBridge(t, h.discovery, readToken)
	readResult, err := readClient.CallTool(t.Context(), &mcp.CallToolParams{Name: "request_api", Arguments: map[string]any{"method": "GET", "url": "/api/v1/projects/" + h.p, "response_filter": ".data | {uuid,name,revision}"}})
	if err != nil || readResult.IsError {
		t.Fatalf("read grant cannot read: %+v %v", readResult, err)
	}
	writeResult, err := readClient.CallTool(t.Context(), &mcp.CallToolParams{Name: "request_api", Arguments: map[string]any{"method": "PATCH", "url": "/api/v1/projects/" + h.p, "response_filter": ".data | {uuid,name,revision}", "request_body": edit, "idempotency_key": "read-write"}})
	encoded, _ := json.Marshal(writeResult)
	if err != nil || !writeResult.IsError || !strings.Contains(string(encoded), "mcp_read_only") {
		t.Fatalf("read-only policy was not enforced: %s %v", encoded, err)
	}
	for _, token := range []string{"", "invalid", strings.Repeat("x", 73)} {
		if _, err = h.a.mcpService.Authenticate(t.Context(), token); err == nil {
			t.Fatal("invalid credential accepted")
		}
	}
	if err = h.a.mcpService.Revoke(t.Context(), h.p, readGrant.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.a.mcpService.Authenticate(t.Context(), readToken); err == nil {
		t.Fatal("revoked accepted")
	}
	if _, err = readClient.ListTools(t.Context(), nil); err == nil {
		t.Fatal("connected client retained revoked authorization")
	}
	if _, err = h.a.projects.CloseProject(t.Context(), h.p); err != nil {
		t.Fatal(err)
	}
	h.request(t, "GET", "", nil, "", true)
	h.request(t, "DELETE", "/chapters/01970000-0000-7000-8000-000000000001", map[string]any{"expected_revision": float64(1)}, "closed-dangerous", true)
}
func TestMCPDangerousConfirmationOriginalRequestAndVersions(t *testing.T) {
	h := newMCPHarness(t)
	created := h.request(t, "POST", "/chapters", map[string]any{"title": "Chapter", "chapter_code": "vol01.ch01"}, "create-chapter", false)
	chapter := business(created)
	u := chapter["uuid"].(string)
	args := map[string]any{"expected_revision": chapter["revision"]}
	pending := h.request(t, "DELETE", "/chapters/"+u, args, "trash", false)
	call := callInfo(pending)
	if call["status"] != "pending_confirmation" {
		t.Fatalf("%+v", pending)
	}
	read := business(h.request(t, "GET", "/chapters/"+u, nil, "", false))
	if read["trashed_at"] != nil {
		t.Fatal("executed without confirmation")
	}
	bad := h.a.mcpService.Decide(t.Context(), h.p, call["uuid"].(string), "bad", "approve")
	if bad["success"] != false {
		t.Fatal("fingerprint accepted")
	}
	approved := h.a.mcpService.Decide(t.Context(), h.p, call["uuid"].(string), call["fingerprint"].(string), "approve")
	if callInfo(approved)["status"] != "succeeded" {
		t.Fatalf("approved: %+v", approved)
	}
	h.tool(t, "get_call", map[string]any{"call_uuid": call["uuid"]}, false)
	// Persisted immutable input survives result retrieval and repeated decisions.
	again := h.a.mcpService.Decide(t.Context(), h.p, call["uuid"].(string), call["fingerprint"].(string), "approve")
	if callInfo(again)["status"] != "succeeded" {
		t.Fatal(again)
	}
	for _, action := range []string{"reject", "expire", "tamper", "conflict"} {
		current := business(h.request(t, "GET", "/chapters/"+u, nil, "", false))
		p := h.tool(t, "request_api", map[string]any{"method": "DELETE", "url": "/api/v1/projects/" + h.p + "/chapters/" + u + "/permanent", "query": map[string]any{"expected_revision": current["revision"]}, "response_filter": ".data", "idempotency_key": action}, false)
		c := callInfo(p)
		switch action {
		case "expire":
			h.store.DB().Model(&mcpserver.Call{}).Where("uuid = ?", c["uuid"]).Update("expires_at", time.Now().Add(-time.Minute))
		case "tamper":
			h.store.DB().Model(&mcpserver.Call{}).Where("uuid = ?", c["uuid"]).Update("arguments", `{}`)
		case "conflict":
			h.request(t, "POST", "/chapters/"+u+"/restorations", map[string]any{"expected_revision": current["revision"]}, "restore", false)
		}
		decision := "approve"
		if action == "reject" {
			decision = "reject"
		}
		r := h.a.mcpService.Decide(t.Context(), h.p, c["uuid"].(string), c["fingerprint"].(string), decision)
		if action == "tamper" {
			if r["success"] != false {
				t.Fatal(r)
			}
		} else {
			want := map[string]string{"reject": "rejected", "expire": "expired", "conflict": "failed"}[action]
			if callInfo(r)["status"] != want {
				t.Fatalf("%s: %+v", action, r)
			}
		}
	}
}
func TestMCPMediaScopeAndHTTPAuthentication(t *testing.T) {
	h := newMCPHarness(t)
	var fileUUID string
	var original bytes.Buffer
	if err := png.Encode(&original, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	err := h.a.projects.WithStore(t.Context(), h.p, func(store *project.Store) error {
		asset, err := files.NewService(store, nil).CommitReader(t.Context(), files.CommitInput{Purpose: "premise_asset", OriginalFilename: "test.png", SourceType: "generated", Reader: bytes.NewReader(original.Bytes())})
		fileUUID = asset.UUID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	media, err := h.client.CallTool(t.Context(), &mcp.CallToolParams{Name: "read_media", Arguments: map[string]any{"file_uuid": fileUUID}})
	if err != nil || media.IsError || len(media.Content) != 2 {
		t.Fatalf("media=%+v err=%v", media, err)
	}
	img, ok := media.Content[1].(*mcp.ImageContent)
	if !ok || !bytes.Equal(img.Data, original.Bytes()) {
		t.Fatal("image content mismatch")
	}
	h.tool(t, "read_media", map[string]any{"file_uuid": "../test"}, true)
	other, err := h.a.projects.Create(t.Context(), "Other media", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	var otherFile string
	err = h.a.projects.WithStore(t.Context(), other.UUID, func(store *project.Store) error {
		asset, err := files.NewService(store, nil).CommitReader(t.Context(), files.CommitInput{Purpose: "premise_asset", OriginalFilename: "other.png", SourceType: "generated", Reader: bytes.NewReader(original.Bytes())})
		otherFile = asset.UUID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	h.tool(t, "read_media", map[string]any{"file_uuid": otherFile}, true)
	for _, test := range []struct {
		token, origin string
		status        int
	}{{"", "", 401}, {"invalid", "", 401}, {h.token, "https://evil.test", 403}} {
		req, _ := http.NewRequest("POST", h.backend.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
		req.Header.Set("Authorization", "Bearer "+test.token)
		req.Header.Set("Origin", test.origin)
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != test.status {
			t.Fatalf("status %d", r.StatusCode)
		}
	}
}

func TestMCPWriteEmitsWebsocketThenRESTReadsTruth(t *testing.T) {
	h := newMCPHarness(t)
	web := httptest.NewServer(h.a)
	defer web.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(web.URL, "http")+"/api/v1/ws", http.Header{"Origin": []string{"http://localhost:5801"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ref := "1"
	if err = conn.WriteJSON(realtime.Frame{Topic: realtime.ProjectTopic(h.p), Event: realtime.EventJoin, Payload: map[string]any{}, Ref: &ref, JoinRef: &ref}); err != nil {
		t.Fatal(err)
	}
	var frame realtime.Frame
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err = conn.ReadJSON(&frame); err != nil {
		t.Fatal(err)
	}
	h.request(t, "PATCH", "", map[string]any{"name": "Websocket edit", "description": "", "expected_revision": float64(1)}, "ws-edit", false)
	seenBusiness, seenMCP := false, false
	for i := 0; i < 5 && (!seenBusiness || !seenMCP); i++ {
		if err = conn.ReadJSON(&frame); err != nil {
			t.Fatal(err)
		}
		if frame.Event == "mcp:changed" {
			seenMCP = true
		}
		if strings.HasPrefix(frame.Event, "story:") {
			seenBusiness = true
		}
		encoded, _ := json.Marshal(frame.Payload)
		if strings.Contains(string(encoded), `"id":`) {
			t.Fatal("internal ID in realtime")
		}
	}
	if !seenBusiness || !seenMCP {
		t.Fatalf("business=%v MCP=%v", seenBusiness, seenMCP)
	}
	response, err := http.Get(web.URL + "/api/v1/projects/" + h.p)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result map[string]any
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["data"].(map[string]any)["name"] != "Websocket edit" {
		t.Fatal(result)
	}
}
func TestMCPGrantManagementEnvelopeAndSecretOnlyOnce(t *testing.T) {
	h := newMCPHarness(t)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1/projects/"+h.p+path, strings.NewReader(body))
		r.Header.Set("Origin", "http://localhost:5801")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.a.ServeHTTP(w, r)
		return w
	}
	r := request("POST", "/mcp-grants", `{"name":"REST test","permission":"read"}`)
	if r.Code != 201 {
		t.Fatal(r.Body.String())
	}
	var created struct {
		Data struct {
			Token  string          `json:"token"`
			Grant  mcpserver.Grant `json:"grant"`
			Config map[string]any  `json:"client_config"`
		} `json:"data"`
	}
	if json.Unmarshal(r.Body.Bytes(), &created) != nil || created.Data.Token == "" || len(created.Data.Config) == 0 {
		t.Fatal(r.Body.String())
	}
	list := request("GET", "/mcp-grants", "")
	if strings.Contains(list.Body.String(), created.Data.Token) || strings.Contains(list.Body.String(), "token_hash") {
		t.Fatal("secret re-exposed")
	}
	if r = request("DELETE", "/mcp-grants/"+created.Data.Grant.UUID, ""); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if _, err := h.a.mcpService.Authenticate(t.Context(), created.Data.Token); err == nil {
		t.Fatal("revocation did not take effect")
	}
}

func TestMCPLegacyInitializeAndProtocolErrors(t *testing.T) {
	h := newMCPHarness(t)
	if got := h.client.InitializeResult().ProtocolVersion; got != "2026-07-28" {
		t.Fatalf("SDK did not negotiate current version: %s", got)
	}
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"legacy","version":"1"},"capabilities":{}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"not/a/method","params":{}}` + "\n"
	var out bytes.Buffer
	if err := mcpbridge.Run(t.Context(), strings.NewReader(input), &out, h.discovery, "development", h.token); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatal(out.String())
	}
	var init map[string]any
	if json.Unmarshal([]byte(lines[0]), &init) != nil || init["result"].(map[string]any)["protocolVersion"] != "2025-11-25" {
		t.Fatal(lines[0])
	}
	var missing struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal([]byte(lines[2]), &missing)
	if missing.Error.Code != -32601 {
		t.Fatal(lines[2])
	}
}
