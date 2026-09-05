package server

import (
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestPriceRoutesAreRegisteredAndCatalogSeeded(t *testing.T) {
	dir := t.TempDir()
	app, err := appstore.Open(dir, config.SQLiteDSN(filepath.Join(dir, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	cfg := config.Config{Environment: "test", Address: ":0", FrontendURL: "http://localhost:5801", ViteDevServerURL: "http://127.0.0.1:5802", AppDataDir: dir, DatabaseDSN: config.SQLiteDSN(filepath.Join(dir, "app.sqlite"))}
	server, err := New(cfg, app, project.NewManager(app))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/model-prices", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "qwen3.7-plus") {
		t.Fatalf("%d %s", recorder.Code, recorder.Body)
	}
	routes := map[string]bool{}
	for _, r := range server.Routes() {
		routes[r.Method+" "+r.Path] = true
	}
	for _, path := range []string{"GET /api/v1/projects/:project_uuid/llm-cost-summary", "GET /api/v1/projects/:project_uuid/llm-cost-backfills", "POST /api/v1/projects/:project_uuid/llm-cost-backfills", "POST /api/v1/projects/:project_uuid/llm-cost-backfills/:backfill_uuid/applications"} {
		if !routes[path] {
			t.Fatal("missing route: " + path)
		}
	}
	for _, path := range []string{"/api/v1/projects/:project_uuid/llm-cost-backfills", "/api/v1/projects/:project_uuid/llm-cost-backfills/:backfill_uuid/applications"} {
		if !draftProjectRequestAllowed(http.MethodPost, path) {
			t.Fatal("draft pricing blocked")
		}
	}
}
