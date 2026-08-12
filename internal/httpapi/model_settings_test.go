package httpapi

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/modelsettings"
	"lumi/internal/project"
	"lumi/internal/provider"

	"github.com/labstack/echo/v4"
)

type modelSettingsEventFake struct {
	topic, event string
	payload      any
}

func (fake *modelSettingsEventFake) Broadcast(topic, event string, payload any) {
	fake.topic, fake.event, fake.payload = topic, event, payload
}

func TestModelSettingsHandlerExposesRevisionedPublicResource(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	createdProvider, err := providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "api/text", DefaultImageModel: "api/image", APIKey: "private-secret"})
	if err != nil {
		t.Fatal(err)
	}
	projects := project.NewManager(app)
	createdProject, err := projects.Create(ctx, "API model settings", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = projects.Close(); providers.Close(); _ = app.Close() })

	events := &modelSettingsEventFake{}
	handler := NewModelSettingsHandler(projects, modelsettings.NewResolver(providers), events)
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.GET("/api/v1/projects/:project_uuid/model-settings", handler.Show)
	e.PATCH("/api/v1/projects/:project_uuid/model-settings", handler.Update)
	base := "/api/v1/projects/" + createdProject.UUID + "/model-settings"

	shown := requestJSON(t, e, "GET", base, nil)
	if shown.Code != 200 || strings.Contains(shown.Body.String(), "private-secret") || strings.Contains(shown.Body.String(), `"id"`) || !strings.Contains(shown.Body.String(), `"source":"global_provider_default"`) {
		t.Fatalf("show=%d %s", shown.Code, shown.Body.String())
	}
	updated := requestJSON(t, e, "PATCH", base, map[string]any{
		"expected_revision": 0,
		"overrides":         map[string]any{"story_text": map[string]any{"provider_uuid": createdProvider.UUID, "model": "api/text"}},
	})
	if updated.Code != 200 || !strings.Contains(updated.Body.String(), `"revision":1`) || !strings.Contains(updated.Body.String(), `"source":"scenario_override"`) {
		t.Fatalf("update=%d %s", updated.Code, updated.Body.String())
	}
	if events.topic != "project:"+createdProject.UUID || events.event != "project:model_settings_changed" || strings.Contains(strings.TrimSpace(updated.Body.String()), "private-secret") {
		t.Fatalf("event=%s %s %#v", events.topic, events.event, events.payload)
	}
	conflict := requestJSON(t, e, "PATCH", base, map[string]any{"expected_revision": 0, "overrides": map[string]any{"story_text": nil}})
	if conflict.Code != 409 || !strings.Contains(conflict.Body.String(), `"code":"project_model_settings_conflict"`) {
		t.Fatalf("conflict=%d %s", conflict.Code, conflict.Body.String())
	}
	invalid := requestJSON(t, e, "PATCH", base, map[string]any{
		"expected_revision": 1,
		"overrides":         map[string]any{"project_image": map[string]any{"provider_uuid": createdProvider.UUID, "model": "api/text"}},
	})
	if invalid.Code != 422 || !strings.Contains(invalid.Body.String(), `"code":"project_model_settings_invalid"`) {
		t.Fatalf("invalid=%d %s", invalid.Code, invalid.Body.String())
	}
	cleared := requestJSON(t, e, "PATCH", base, map[string]any{"expected_revision": 1, "overrides": map[string]any{"story_text": nil}})
	if cleared.Code != 200 || !strings.Contains(cleared.Body.String(), `"revision":2`) || !strings.Contains(cleared.Body.String(), `"override":null`) {
		t.Fatalf("clear=%d %s", cleared.Code, cleared.Body.String())
	}
}
