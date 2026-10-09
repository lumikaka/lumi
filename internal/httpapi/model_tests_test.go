package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/modeltest"
	"lumi/internal/provider"

	"github.com/labstack/echo/v4"
)

func TestModelTestResourcesUseEnvelopesAndPublicUUIDs(t *testing.T) {
	dir := t.TempDir()
	app, err := appstore.Open(dir, config.SQLiteDSN(filepath.Join(dir, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	item, err := providers.Create(context.Background(), provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "openai/gpt-6-sol", APIKey: "never-public"})
	if err != nil {
		t.Fatal(err)
	}
	s := modeltest.NewService(app, providers, nil, nil, nil, nil)
	t.Cleanup(func() { _ = s.Close(); providers.Close(); _ = app.Close() })
	h := NewModelTestHandler(s)
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.POST("/api/v1/model-tests", h.Create)
	e.GET("/api/v1/model-tests", h.Index)
	e.GET("/api/v1/model-tests/:test_uuid", h.Show)
	e.PUT("/api/v1/model-tests/:test_uuid/execution", h.Execute)
	e.DELETE("/api/v1/model-tests/:test_uuid/execution", h.Stop)
	e.GET("/media/model-tests/:test_uuid/steps/:step_uuid/content", h.Content)
	invalid := requestJSON(t, e, "POST", "/api/v1/model-tests", map[string]any{})
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty selection created a test: %s", invalid.Body)
	}
	choice := map[string]any{"provider_uuid": item.UUID, "model": item.DefaultModel}
	input := map[string]any{"text_model": choice, "image_model": choice}
	created := requestJSON(t, e, "POST", "/api/v1/model-tests", input)
	var envelope struct {
		Success bool           `json:"success"`
		Data    modeltest.Test `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil || created.Code != http.StatusAccepted || !envelope.Success || envelope.Data.Status != "queued" || len(envelope.Data.Steps) != 3 {
		t.Fatalf("create=%s err=%v", created.Body, err)
	}
	path := "/api/v1/model-tests/" + envelope.Data.UUID
	for range 2 {
		started := requestJSON(t, e, "PUT", path+"/execution", input)
		if started.Code != http.StatusAccepted || !strings.Contains(started.Body.String(), `"status":"queued"`) {
			t.Fatalf("start=%d %s", started.Code, started.Body)
		}
	}
	input["text_model"] = map[string]any{"provider_uuid": item.UUID, "model": "different"}
	conflict := requestJSON(t, e, "PUT", path+"/execution", input)
	if conflict.Code != 409 || !strings.Contains(conflict.Body.String(), `"success":false,"data":null`) {
		t.Fatalf("conflict=%s", conflict.Body)
	}
	for _, path := range []string{path, "/api/v1/model-tests?page=1&per_page=20"} {
		read := requestJSON(t, e, "GET", path, nil)
		if read.Code != 200 {
			t.Fatal(read.Body)
		}
		for _, forbidden := range []string{`"id":`, `"model_test_id":`, "never-public", dir} {
			if strings.Contains(read.Body.String(), forbidden) {
				t.Fatalf("leaked %q", forbidden)
			}
		}
	}
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/api/v1/model-tests/1", 404}, {"/api/v1/model-tests?page=0", 422}, {"/api/v1/model-tests?per_page=101", 422},
		{"/api/v1/model-tests?page=nope", 422}, {"/media/model-tests/" + envelope.Data.UUID + "/steps/" + envelope.Data.Steps[0].UUID + "/content", 404},
	} {
		result := requestJSON(t, e, "GET", tc.path, nil)
		if result.Code != tc.status || !strings.Contains(result.Body.String(), `"data":null`) {
			t.Fatalf("path=%s response=%d %s", tc.path, result.Code, result.Body)
		}
	}
	unknown := requestJSON(t, e, "POST", "/api/v1/model-tests", map[string]any{"id": 123})
	if unknown.Code != 400 {
		t.Fatalf("internal field accepted: %s", unknown.Body)
	}
	for range 2 {
		stopped := requestJSON(t, e, "DELETE", path+"/execution", nil)
		if err := json.Unmarshal(stopped.Body.Bytes(), &envelope); err != nil || stopped.Code != http.StatusOK || !envelope.Success || envelope.Data.Status != "interrupted" {
			t.Fatalf("stop=%d %s err=%v", stopped.Code, stopped.Body, err)
		}
		for _, step := range envelope.Data.Steps {
			if step.Status != "skipped" {
				t.Fatalf("queued step not skipped: %+v", step)
			}
		}
		if strings.Contains(stopped.Body.String(), `"id":`) || strings.Contains(stopped.Body.String(), `"model_test_id":`) {
			t.Fatalf("stop exposed internal ids: %s", stopped.Body)
		}
	}
	missing := requestJSON(t, e, "DELETE", "/api/v1/model-tests/1/execution", nil)
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), `"success":false,"data":null`) {
		t.Fatalf("missing=%d %s", missing.Code, missing.Body)
	}
}
