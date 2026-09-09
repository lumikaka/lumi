package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
)

func TestGlobalModelSettingsRoutesWithoutAConfiguredProvider(t *testing.T) {
	dir := t.TempDir()
	dsn := config.SQLiteDSN(filepath.Join(dir, "app.sqlite"))
	store, err := appstore.Open(dir, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	application, err := New(config.Config{Environment: "test", Address: ":0", FrontendURL: "http://localhost:5801", ViteDevServerURL: "http://127.0.0.1:5802", AppDataDir: dir, DatabaseDSN: dsn}, store, project.NewManager(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		body := ""
		if method == http.MethodPatch {
			body = `{"expected_revision":0,"overrides":{"project_image":null}}`
		}
		req := httptest.NewRequest(method, "/api/v1/model-settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:5801")
		recorder := httptest.NewRecorder()
		application.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"effective":null`) || strings.Contains(recorder.Body.String(), `"id"`) {
			t.Fatalf("%s status=%d body=%s", method, recorder.Code, recorder.Body.String())
		}
	}
}
