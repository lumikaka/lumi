package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/httpapi"
	"lumi/internal/textmcp"
)

func TestLegacyCreationTransportReproducesUnboundSuccessAndBoundArgumentFailure(t *testing.T) {
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Historical daemons accepted these fields but rejected the newer
		// expected_project_uuid constraint before dispatching setup or reads.
		var legacy struct {
			RootPath  string          `json:"root_path"`
			Tool      string          `json:"tool"`
			Arguments json.RawMessage `json:"arguments"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&legacy); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(textmcp.Failure("invalid_arguments", "创作请求无效", "请求必须是有限大小的 JSON 对象。"))
			return
		}
		_ = json.NewEncoder(w).Encode(httpapi.Envelope{Success: true, Data: map[string]any{"synthetic": true}})
	}))
	defer web.Close()
	identity := instance{Endpoint: web.URL, Nonce: "test-generation"}
	request := creationRequest{RootPath: t.TempDir(), Tool: "project_overview", Arguments: json.RawMessage(`{}`)}
	if got := forwardCreation(t.Context(), identity, request); !got.Success {
		t.Fatalf("unbound request: %+v", got)
	}
	request.ExpectedProjectUUID = "01989abc-def0-7000-8000-000000000001"
	if got := forwardCreation(t.Context(), identity, request); got.Success || got.Error == nil || got.Error.Code != "invalid_arguments" {
		t.Fatalf("bound request: %+v", got)
	}
}

func TestMCPRejectsLegacyDaemonBeforeCreationWithoutStoppingIt(t *testing.T) {
	opt := options{environment: "production", dataDir: t.TempDir(), timeout: 5 * time.Second}
	root := t.TempDir()
	var identity instance
	var creations, deletes atomic.Int32
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == instanceRoute && r.Method == http.MethodGet {
			// v3 supports signed imports but accepts unreviewed setting images.
			_ = json.NewEncoder(w).Encode(httpapi.Envelope{Success: true, Data: instanceStatus{instance: identity, Running: true, CreationProtocolVersion: 3}})
			return
		}
		if r.Method == http.MethodDelete {
			deletes.Add(1)
		} else if r.URL.Path == creationRoute {
			creations.Add(1)
		}
		_ = json.NewEncoder(w).Encode(textmcp.Failure("invalid_arguments", "legacy", ""))
	}))
	t.Cleanup(web.Close)
	identity = instance{Endpoint: web.URL, PID: os.Getpid(), Environment: opt.environment, Nonce: strings.Repeat("a", 64)}
	cleanup, err := writeInstance(opt, identity)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	client := mcpClient(t, opt, root)
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"project_init", map[string]any{"idea": "Must not create any project"}},
		{"project_overview", map[string]any{}},
		{"project_setup", map[string]any{"action": "get"}},
	} {
		callText(t, client, call.name, call.args, "cli_upgrade_required")
	}
	if creations.Load() != 0 || deletes.Load() != 0 {
		t.Fatalf("legacy service received creation=%d stop=%d", creations.Load(), deletes.Load())
	}
	if files, err := os.ReadDir(root); err != nil || len(files) != 0 {
		t.Fatalf("preflight modified project directory: %v %v", files, err)
	}
	if live, err := inspectLiveInstance(t.Context(), opt); err != nil || live.instance != identity {
		t.Fatalf("shared service changed: %+v %v", live, err)
	}
}

func TestBoundSetupGetUpdateFinalizeAgainstCapableDaemon(t *testing.T) {
	opt := cliOptions(t)
	client := mcpClient(t, opt, t.TempDir())
	initial := callText(t, client, "project_init", map[string]any{"idea": "Synthetic paper boat fixture", "name": "Bound draft"}, "")
	projectUUID := field(initial, "project")["uuid"].(string)
	if status := cliRun(t, opt, "status", 0); status.CreationProtocolVersion != creationProtocolVersion {
		t.Fatalf("missing daemon capability: %+v", status)
	}
	bound := func(name string, args map[string]any) map[string]any {
		t.Helper()
		params := &mcp.CallToolParams{Name: name, Arguments: args}
		params.Meta = mcp.Meta{textmcp.ExpectedProjectUUIDMeta: projectUUID}
		result, err := client.CallTool(t.Context(), params)
		if err != nil {
			t.Fatal(err)
		}
		var envelope toolEnvelope
		if len(result.Content) != 1 {
			t.Fatalf("content=%+v", result.Content)
		}
		if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &envelope); err != nil || !envelope.Success || result.IsError {
			t.Fatalf("bound %s: %+v %v", name, envelope, err)
		}
		return envelope.Data
	}
	setup := bound("project_setup", map[string]any{"action": "get"})
	setup = bound("project_setup", map[string]any{"action": "update", "expected_revision": setup["revision"],
		"project_name": "正式版独立作品验收", "generation_language": "zh-Hans", "overall_style": "温暖水彩",
		"generation_brief": "三页合成验收作品，不调用付费媒体", "picture_book": map[string]any{
			"format": "classic_picture_book", "aspect_ratio": map[string]any{"mode": "landscape"},
		}})
	if got := bound("project_setup", map[string]any{"action": "finalize", "expected_revision": setup["revision"]}); got["setup_status"] != "ready" {
		t.Fatalf("finalize: %+v", got)
	}
	if got := bound("project_overview", map[string]any{}); field(got, "project")["uuid"] != projectUUID {
		t.Fatal("setup changed project identity")
	}
}
