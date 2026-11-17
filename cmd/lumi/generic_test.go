package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/provider"
	"lumi/internal/sitesettings"
)

// Real stdio -> daemon -> durable production task -> HTTP image provider ->
// current page -> review -> export. Only the remote provider is a fixture.
func TestGenericStdioCreationImageReviewAndExport(t *testing.T) {
	opt := cliOptions(t)
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 48, 48))); err != nil {
		t.Fatal(err)
	}
	var generations atomic.Int32
	imageAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if r.URL.Path != "/v1/images/generations" || r.Header.Get("Authorization") != "Bearer fixture-image-key" || json.NewDecoder(r.Body).Decode(&request) != nil || request["model"] != "gpt-image-2" || !strings.Contains(request["prompt"].(string), "Paper star") {
			t.Errorf("unexpected image request: %s %+v", r.URL.Path, request)
			w.WriteHeader(400)
			return
		}
		generations.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(picture.Bytes())}}})
	}))
	defer imageAPI.Close()
	// Configure the isolated application before the daemon opens it. No real
	// credentials, keychain, provider network or user configuration is involved.
	app, err := appstore.Open(opt.dataDir, config.SQLiteDSN(filepath.Join(opt.dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	key := base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	t.Setenv("LUMI_TEST_MASTER_KEY", key)
	keys := sitesettings.NewMemoryMasterKeyStore()
	if err := keys.Set(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, keys)
	_, _, err = providers.Settings().Update(t.Context(), map[string]any{
		sitesettings.RelayBaseURLKey: imageAPI.URL + "/v1", sitesettings.RelayAPIKeyKey: "fixture-text-key",
		sitesettings.RelayImageBaseURLKey: imageAPI.URL + "/v1", sitesettings.RelayImageAPIKeyKey: "fixture-image-key",
		sitesettings.RelayImageAPIModeKey: "images", sitesettings.RelayDefaultImageModelKey: "gpt-image-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := providers.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ProviderType == provider.TypeOpenAIRelay {
			if _, err := providers.MarkVerified(t.Context(), item.UUID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := providers.Activate(t.Context(), provider.TypeOpenAIRelay); err != nil {
		t.Fatal(err)
	}
	providers.Close()
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	client := mcpHostClient(t, opt, root, "generic")
	empty := map[string]any{}
	callText(t, client, "project_overview", empty, "project_not_initialized")
	init := callText(t, client, "project_init", map[string]any{"idea": "Paper star", "name": "Generic acceptance"}, "")
	projectUUID := field(init, "project")["uuid"].(string)
	setup := callText(t, client, "project_setup", map[string]any{"action": "update", "expected_revision": field(init, "setup")["revision"], "project_name": "Generic acceptance", "overall_style": "Paper collage", "picture_book": map[string]any{"format": "classic_picture_book", "aspect_ratio": map[string]any{"mode": "square"}}}, "")
	callText(t, client, "project_setup", map[string]any{"action": "finalize", "expected_revision": setup["revision"]}, "")
	overview := callText(t, client, "project_overview", empty, "")
	chapter := callText(t, client, "chapters", map[string]any{"action": "create", "title": "Paper star", "chapter_code": "vol01.ch01", "content": "A paper star finds its home.", "expected_revision": field(overview, "project")["revision"]}, "")
	state := callText(t, client, "pages", map[string]any{"action": "list", "chapter_uuid": chapter["uuid"]}, "")
	page := callText(t, client, "pages", map[string]any{"action": "create", "chapter_uuid": chapter["uuid"], "title": "Paper star", "description_md": "Home at last.", "storyboard_md": "Paper star above a little house.", "expected_revision": field(state, "state")["revision"]}, "")
	target := callText(t, client, "image_target", map[string]any{"target_uuid": page["uuid"], "slot": "page_image", "setting_uuids": []string{}}, "")
	args := map[string]any{"page_uuid": page["uuid"], "setting_uuids": []string{}, "setting_reviews": []any{}, "idempotency_key": "generic-fixture-1", "expected_revision": target["expected_revision"]}
	generated := callText(t, client, "generate_page_image", args, "")
	if generated["done"] != true || generated["is_current_image"] != true {
		t.Fatal(generated)
	}
	replay := callText(t, mcpHostClient(t, opt, root, "generic"), "generate_page_image", args, "")
	if field(replay, "task")["task_uuid"] != field(generated, "task")["task_uuid"] || generations.Load() != 1 {
		t.Fatal("reconnect duplicated generation", replay, generations.Load())
	}
	imageResult, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "read_image", Arguments: field(generated, "read_image")})
	if err != nil || imageResult.IsError || len(imageResult.Content) != 2 {
		t.Fatal("image unavailable", imageResult, err)
	}
	adopted := callText(t, client, "pages", map[string]any{"action": "get", "chapter_uuid": chapter["uuid"], "page_uuid": page["uuid"]}, "")
	callText(t, client, "pages", map[string]any{"action": "record_image_review", "page_uuid": page["uuid"], "chapter_uuid": chapter["uuid"], "variant_uuid": field(adopted, "current_image")["uuid"], "expected_revision": adopted["revision"], "verdict": "passed", "observation": "Synthetic fixture: image bytes and current variant verified."}, "")
	link := callText(t, client, "web_url", map[string]any{"page_uuid": page["uuid"]}, "")
	status := cliRun(t, opt, "status", 0)
	if link["url"] != status.Endpoint+"/projects/"+projectUUID+"/chapters/"+chapter["uuid"].(string)+"/sections/"+page["uuid"].(string) {
		t.Fatal("wrong review destination", link)
	}
	for _, format := range []string{"markdown", "zip", "pdf"} {
		export := callText(t, client, "export_book", map[string]any{"format": format, "allow_missing_images": false}, "")
		info, err := os.Stat(export["path"].(string))
		if err != nil || info.Size() == 0 || export["missing_image_count"] != float64(0) {
			t.Fatal("export unavailable", export, err)
		}
	}
	// A connection from another cwd cannot discover or switch to this project.
	callText(t, mcpHostClient(t, opt, t.TempDir(), "generic"), "project_overview", empty, "project_not_initialized")
}
