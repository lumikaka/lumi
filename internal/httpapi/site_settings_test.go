package httpapi

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/provider"
	"lumi/internal/realtime"

	"github.com/labstack/echo/v4"
)

func TestSiteSettingsAPIEncryptsSecretsValidatesKeysAndResets(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(directory, config.SQLiteDSN(filepath.Join(directory, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	hub := realtime.NewHub(realtime.ProjectTopics{})
	t.Cleanup(func() { providers.Close(); _ = hub.Close(); _ = app.Close() })
	handler := NewSiteSettingsHandler(providers, hub)
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.GET("/api/v1/site-settings", handler.Index)
	e.PATCH("/api/v1/site-settings", handler.Update)
	e.POST("/api/v1/site-settings/resets", handler.Reset)

	secret := "api-setting-secret-never-return"
	updated := requestJSON(t, e, "PATCH", "/api/v1/site-settings", map[string]any{"settings": map[string]any{
		"ai_providers.openai_compatible.account_id":          "0123456789ABCDEF0123456789ABCDEF",
		"ai_providers.openai_compatible.default_model":       "test/model",
		"ai_providers.openai_compatible.default_image_model": "test/image-model",
		"ai_providers.openai_compatible.api_key":             secret,
	}})
	if updated.Code != 200 || strings.Contains(updated.Body.String(), secret) || !strings.Contains(updated.Body.String(), `"value":null`) || !strings.Contains(updated.Body.String(), `"secret_state":"available"`) {
		t.Fatalf("update response=%d %s", updated.Code, updated.Body.String())
	}
	unknown := requestJSON(t, e, "PATCH", "/api/v1/site-settings", map[string]any{"settings": map[string]any{"unknown.key": true}})
	if unknown.Code != 422 || !strings.Contains(unknown.Body.String(), `"code":"unknown_site_setting"`) {
		t.Fatalf("unknown response=%d %s", unknown.Code, unknown.Body.String())
	}
	genericBaseURL := requestJSON(t, e, "PATCH", "/api/v1/site-settings", map[string]any{"settings": map[string]any{"ai_providers.openai_compatible.base_url": "https://api.openai.com/v1"}})
	if genericBaseURL.Code != 422 || !strings.Contains(genericBaseURL.Body.String(), `"code":"unknown_site_setting"`) {
		t.Fatalf("generic base URL response=%d %s", genericBaseURL.Code, genericBaseURL.Body.String())
	}
	invalidActive := requestJSON(t, e, "PATCH", "/api/v1/site-settings", map[string]any{"settings": map[string]any{"ai_provider.active": "not-a-provider"}})
	if invalidActive.Code != 422 || !strings.Contains(invalidActive.Body.String(), `"code":"invalid_site_setting"`) {
		t.Fatalf("invalid active response=%d %s", invalidActive.Code, invalidActive.Body.String())
	}
	unverifiedActive := requestJSON(t, e, "PATCH", "/api/v1/site-settings", map[string]any{"settings": map[string]any{"ai_provider.active": "cloudflare_ai_gateway"}})
	if unverifiedActive.Code != 409 || !strings.Contains(unverifiedActive.Body.String(), `"code":"provider_not_ready"`) {
		t.Fatalf("unverified activation=%d %s", unverifiedActive.Code, unverifiedActive.Body.String())
	}
	reset := requestJSON(t, e, "POST", "/api/v1/site-settings/resets", map[string]any{"keys": []string{"ai_providers.openai_compatible.api_key"}})
	if reset.Code != 200 || strings.Contains(reset.Body.String(), secret) || !strings.Contains(reset.Body.String(), `"secret_state":"empty"`) {
		t.Fatalf("reset response=%d %s", reset.Code, reset.Body.String())
	}
	if _, err := providers.Settings().Secret(context.Background(), "ai_providers.openai_compatible.api_key"); err != nil {
		t.Fatal(err)
	}
}
