package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/mcpbridge"
	"lumi/internal/project"
)

// The packaging CI runs this against the same embedded binary shipped in the
// desktop bundle. Ordinary unit tests do not recursively build another binary.
func TestMCPPackagedProcessReconnect(t *testing.T) {
	t.Run("dynamic_stdio", func(t *testing.T) { testMCPPackagedReconnect(t, false) })
	t.Run("stable_oauth", func(t *testing.T) { testMCPPackagedReconnect(t, true) })
}
func testMCPPackagedReconnect(t *testing.T, stable bool) {
	binary := os.Getenv("LUMI_MCP_TEST_BINARY")
	if binary == "" {
		t.Skip("set LUMI_MCP_TEST_BINARY to an absolute embed_frontend binary")
	}
	dir := t.TempDir()
	store, err := appstore.Open(dir, config.SQLiteDSN(filepath.Join(dir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	manager := project.NewManager(store)
	p, err := manager.Create(t.Context(), "MCP process integration", project.ExplicitNewProjectParent(dir))
	if err != nil {
		t.Fatal(err)
	}
	manager.Close()
	store.Close()
	stableAddress := ""
	if stable {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		stableAddress = listener.Addr().String()
		listener.Close()
	}
	start := func() (string, func()) {
		t.Helper()
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address := listener.Addr().String()
		listener.Close()
		if stable {
			address = stableAddress
		}
		cmd := exec.Command(binary)
		cmd.Dir = dir // do not load the developer's .env
		cmd.Env = append(os.Environ(), "APP_ENV=production", "APP_ADDRESS="+address, "FRONTEND_URL=http://"+address, "LUMI_DATA_DIR="+dir, "DATABASE_DSN=", "LUMI_DESKTOP_ACCESS_TOKEN=")
		log, err := os.Create(filepath.Join(t.TempDir(), "backend.log"))
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		stopped := false
		stop := func() {
			if !stopped {
				stopped = true
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				_ = log.Close()
			}
		}
		t.Cleanup(stop)
		client := &http.Client{Timeout: time.Second}
		base := "http://" + address
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			r, err := client.Get(base + "/api/v1/health")
			if err == nil {
				r.Body.Close()
				if r.StatusCode == 200 {
					return base, stop
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("packaged backend did not become healthy")
		return "", stop
	}
	rest := func(base, method, path string, body any) map[string]any {
		t.Helper()
		b, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(t.Context(), method, base+path, bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", base)
		req.Header.Set("Content-Type", "application/json")
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		b, _ = io.ReadAll(r.Body)
		var result map[string]any
		if json.Unmarshal(b, &result) != nil || result["success"] != true {
			t.Fatalf("REST status=%d: %s", r.StatusCode, b)
		}
		if result["data"] == nil {
			return nil
		}
		return result["data"].(map[string]any)
	}
	base, stop := start()
	if endpoint := rest(base, "GET", "/api/v1/mcp", nil)["endpoint"]; endpoint != base+"/mcp" {
		t.Fatalf("MCP is not on business listener: %v", endpoint)
	}
	rest(base, "PUT", "/api/v1/open-projects/"+p.UUID, nil)
	grant := rest(base, "POST", "/api/v1/projects/"+p.UUID+"/mcp-grants", map[string]any{"name": "packaged SDK", "permission": "edit"})
	cmd := exec.Command(binary, "--mcp", "--environment", "production", "--data-dir", dir)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LUMI_MCP_TOKEN="+grant["token"].(string))
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "packaged-process-test", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	list, err := client.ListTools(ctx, nil)
	if err != nil || len(list.Tools) != 4 {
		t.Fatalf("tools=%+v err=%v", list, err)
	}
	args := map[string]any{"method": "PATCH", "url": "/api/v1/projects/" + p.UUID, "request_body": map[string]any{"name": "MCP process updated", "description": "process integration", "expected_revision": 1}, "response_filter": ".data | {uuid,name,revision}", "idempotency_key": "process-edit"}
	first, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "request_api", Arguments: args})
	if err != nil || first.IsError {
		t.Fatalf("edit=%+v err=%v", first, err)
	}
	before, err := mcpbridge.Read(mcpbridge.Path(dir, "production"), "production")
	if err != nil {
		t.Fatal(err)
	}
	var oauthForm url.Values
	if stable {
		endpoint := rest(base, "GET", "/api/v1/mcp", nil)["endpoint"].(string)
		oauthBase := strings.TrimSuffix(endpoint, "/mcp")
		response, err := http.Post(oauthBase+"/oauth/register", "application/json", strings.NewReader(`{"client_name":"Packaged OAuth","redirect_uris":["http://127.0.0.1:49222/callback"],"token_endpoint_auth_method":"none"}`))
		if err != nil {
			t.Fatal(err)
		}
		var registered map[string]any
		err = json.NewDecoder(response.Body).Decode(&registered)
		response.Body.Close()
		if err != nil || response.StatusCode != 201 {
			t.Fatalf("registration: %+v %v", registered, err)
		}
		verifier := strings.Repeat("a", 43)
		digest := sha256.Sum256([]byte(verifier))
		params := url.Values{"client_id": {registered["client_id"].(string)}, "response_type": {"code"}, "resource": {endpoint}, "redirect_uri": {"http://127.0.0.1:49222/callback"}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}}
		browser := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		response, err = browser.Get(oauthBase + "/oauth/authorize?" + params.Encode())
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		location, err := url.Parse(response.Header.Get("Location"))
		if err != nil || response.StatusCode != 303 {
			t.Fatalf("authorize: %d %v", response.StatusCode, err)
		}
		decision := rest(base, "POST", "/api/v1/mcp-authorization-requests/"+location.Query().Get("request_uuid")+"/decisions", map[string]string{"decision": "approve", "project_uuid": p.UUID, "permission": "read"})
		callback, _ := url.Parse(decision["redirect_url"].(string))
		oauthForm = url.Values{"client_id": params["client_id"], "resource": {endpoint}, "redirect_uri": params["redirect_uri"], "grant_type": {"authorization_code"}, "code_verifier": {verifier}, "code": {callback.Query().Get("code")}}
	}
	stop() // crash leaves stale discovery; the bridge must survive and not replay
	if _, err := client.ListTools(ctx, nil); err == nil {
		t.Fatal("dead backend accepted request")
	}
	base, _ = start()
	after, err := mcpbridge.Read(mcpbridge.Path(dir, "production"), "production")
	if err != nil || before.Nonce == after.Nonce {
		t.Fatalf("instance not replaced: %v", err)
	}
	if stable {
		if before.Endpoint != after.Endpoint {
			t.Fatal("OAuth endpoint changed across restart")
		}
		response, err := http.PostForm(strings.TrimSuffix(after.Endpoint, "/mcp")+"/oauth/token", oauthForm)
		if err != nil {
			t.Fatal(err)
		}
		var token map[string]any
		err = json.NewDecoder(response.Body).Decode(&token)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 || token["access_token"] == nil {
			t.Fatalf("code did not survive process restart: %+v %v", token, err)
		}
		req, _ := http.NewRequest("POST", after.Endpoint, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
		req.Header.Set("Authorization", "Bearer "+token["access_token"].(string))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		response, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("restarted token: %d", response.StatusCode)
		}
	}
	closed, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "request_api", Arguments: map[string]any{"method": "GET", "url": "/api/v1/projects/" + p.UUID, "response_filter": ".data | {uuid,name}"}})
	if err != nil || !closed.IsError {
		t.Fatalf("closed=%+v err=%v", closed, err)
	}
	rest(base, "PUT", "/api/v1/open-projects/"+p.UUID, nil)
	again, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "request_api", Arguments: args})
	if err != nil || again.IsError {
		t.Fatalf("recovered=%+v err=%v", again, err)
	}
	b1, _ := json.Marshal(first.StructuredContent)
	b2, _ := json.Marshal(again.StructuredContent)
	if !bytes.Equal(b1, b2) {
		t.Fatal("restart did not deliver original durable result")
	}
	state := rest(base, "GET", "/api/v1/projects/"+p.UUID, nil)
	if state["name"] != "MCP process updated" || state["revision"] != float64(2) {
		t.Fatalf("unexpected state: %+v", state)
	}
}
