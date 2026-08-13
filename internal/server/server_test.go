package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/httpapi"
	"lumi/internal/project"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func TestRequestLogLevel(t *testing.T) {
	for _, scenario := range []struct {
		status int
		err    error
		want   slog.Level
	}{
		{status: http.StatusNoContent, want: slog.LevelInfo},
		{status: http.StatusFound, want: slog.LevelInfo},
		{status: http.StatusNotFound, want: slog.LevelWarn},
		{status: http.StatusUnprocessableEntity, want: slog.LevelWarn},
		{status: http.StatusInternalServerError, want: slog.LevelError},
		{status: http.StatusOK, err: errors.New("unexpected"), want: slog.LevelError},
	} {
		if got := requestLogLevel(scenario.status, scenario.err); got != scenario.want {
			t.Errorf("requestLogLevel(%d, %v) = %s, want %s", scenario.status, scenario.err, got, scenario.want)
		}
	}
}

func TestRequestLoggerFiltersAndClassifiesByLevel(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		threshold slog.Level
		status    int
		wantLog   bool
		wantLevel string
	}{
		{name: "info includes success", threshold: slog.LevelInfo, status: http.StatusNoContent, wantLog: true, wantLevel: "INFO"},
		{name: "warn excludes success", threshold: slog.LevelWarn, status: http.StatusNoContent},
		{name: "warn includes client error", threshold: slog.LevelWarn, status: http.StatusNotFound, wantLog: true, wantLevel: "WARN"},
		{name: "error excludes client error", threshold: slog.LevelError, status: http.StatusNotFound},
		{name: "error includes server error", threshold: slog.LevelError, status: http.StatusInternalServerError, wantLog: true, wantLevel: "ERROR"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: scenario.threshold}))
			e := echo.New()
			e.Use(middleware.RequestID())
			e.Use(requestLogger(logger))
			e.GET("/logging-test", func(c echo.Context) error { return c.NoContent(scenario.status) })

			request := httptest.NewRequest(http.MethodGet, "/logging-test", nil)
			request.Header.Set(echo.HeaderXRequestID, "test-request-id")
			e.ServeHTTP(httptest.NewRecorder(), request)

			if scenario.wantLog != (output.Len() > 0) {
				t.Fatalf("logged = %t, output = %q", output.Len() > 0, output.String())
			}
			if !scenario.wantLog {
				return
			}
			for _, fragment := range []string{
				`"level":"` + scenario.wantLevel + `"`,
				`"method":"GET"`,
				`"status":` + fmt.Sprint(scenario.status),
				`"request_id":"test-request-id"`,
				`"latency":`,
			} {
				if !strings.Contains(output.String(), fragment) {
					t.Errorf("log %q does not contain %q", output.String(), fragment)
				}
			}
		})
	}
}

func TestHealthAndUnknownAPI(t *testing.T) {
	dataDir := t.TempDir()
	store, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := config.Config{
		Environment: "test", Address: ":0", FrontendURL: "http://localhost:5801",
		ViteDevServerURL: "http://127.0.0.1:5802", AppDataDir: dataDir, DatabaseDSN: config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")),
	}
	application, err := New(cfg, store, project.NewManager(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })

	websocketRouteFound := false
	for _, route := range application.Routes() {
		if route.Method == http.MethodGet && route.Path == "/api/v1/ws" {
			websocketRouteFound = true
		}
	}
	if !websocketRouteFound || application.RealtimeHub() == nil {
		t.Fatal("application realtime endpoint was not initialized")
	}

	for _, scenario := range []struct {
		path   string
		status int
	}{
		{path: "/api/v1/health", status: http.StatusOK},
		{path: "/api/v1/projects/01989abc-def0-7000-8000-000000000001/tasks", status: http.StatusNotFound},
		{path: "/api/v1/missing", status: http.StatusNotFound},
		{path: "/api/missing", status: http.StatusNotFound},
	} {
		recorder := httptest.NewRecorder()
		application.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, scenario.path, nil))
		if recorder.Code != scenario.status {
			t.Fatalf("GET %s status = %d, body = %s", scenario.path, recorder.Code, recorder.Body.String())
		}
		var response httpapi.Envelope
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("GET %s returned invalid JSON: %v", scenario.path, err)
		}
	}
}

func TestWriteRequestsRequireTrustedOrigin(t *testing.T) {
	dataDir := t.TempDir()
	store, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := config.Config{
		Environment: "test", Address: ":0", FrontendURL: "http://localhost:5801",
		ViteDevServerURL: "http://127.0.0.1:5802", AppDataDir: dataDir, DatabaseDSN: config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")),
	}
	application, err := New(cfg, store, project.NewManager(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })

	for _, scenario := range []struct {
		origin string
		status int
	}{
		{origin: "", status: http.StatusForbidden},
		{origin: "https://evil.example", status: http.StatusForbidden},
		{origin: "http://127.0.0.1:5801", status: http.StatusUnprocessableEntity},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/open-projects", strings.NewReader(`{"root_path":""}`))
		request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		if scenario.origin != "" {
			request.Header.Set(echo.HeaderOrigin, scenario.origin)
		}
		recorder := httptest.NewRecorder()
		application.ServeHTTP(recorder, request)
		if recorder.Code != scenario.status {
			t.Fatalf("origin %q status = %d, body = %s", scenario.origin, recorder.Code, recorder.Body.String())
		}
	}
}
