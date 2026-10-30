package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/providerlimit"
)

func TestProviderModelRateLimitsRestoreFromSQLite(t *testing.T) {
	dir := t.TempDir()
	dsn := config.SQLiteDSN(filepath.Join(dir, "app.sqlite"))
	store, err := appstore.Open(dir, dsn)
	if err != nil {
		t.Fatal(err)
	}
	rps := 1
	_, err = store.PatchGlobalModelSettings(context.Background(), 0, nil, map[string]map[string]*appstore.GlobalModelRateLimit{
		provider.TypeCloudflareAIGateway: {"openai/gpt-6-sol": {RPS: &rps}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := appstore.Open(dir, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	limiter, err := restoredProviderLimiter(context.Background(), reopened)
	if err != nil {
		t.Fatal(err)
	}
	first, err := limiter.Acquire(context.Background(), provider.TypeCloudflareAIGateway, "https://example.com/ai/v1", "openai/gpt-6-sol", providerlimit.RequestText)
	if err != nil {
		t.Fatal(err)
	}
	first.Finish(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := limiter.Acquire(ctx, provider.TypeCloudflareAIGateway, "https://example.com/ai/v1", "openai/gpt-6-sol", providerlimit.RequestText); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("restored RPS limit did not block next request: %v", err)
	}
}

func TestProviderTotalRateLimitRestoresFromSQLite(t *testing.T) {
	dir := t.TempDir()
	dsn := config.SQLiteDSN(filepath.Join(dir, "app.sqlite"))
	store, err := appstore.Open(dir, dsn)
	if err != nil {
		t.Fatal(err)
	}
	rps := 1
	_, err = store.PatchGlobalModelSettings(context.Background(), 0, nil, nil, map[string]*appstore.GlobalModelRateLimit{
		provider.TypeAliyunBailian: {RPS: &rps},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := appstore.Open(dir, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	limiter, err := restoredProviderLimiter(context.Background(), reopened)
	if err != nil {
		t.Fatal(err)
	}
	first, err := limiter.Acquire(context.Background(), provider.TypeAliyunBailian, "https://dashscope.example/v1", "qwen3.7-plus", providerlimit.RequestText)
	if err != nil {
		t.Fatal(err)
	}
	first.Finish(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := limiter.Acquire(ctx, provider.TypeAliyunBailian, "https://dashscope.example/v1", "qwen-image-3.0", providerlimit.RequestImage); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("restored provider pool did not count another model: %v", err)
	}
}

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
		req := newLocalRequest(method, "/api/v1/model-settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:5801")
		recorder := httptest.NewRecorder()
		application.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"effective":null`) || strings.Contains(recorder.Body.String(), `"id"`) {
			t.Fatalf("%s status=%d body=%s", method, recorder.Code, recorder.Body.String())
		}
	}
}
