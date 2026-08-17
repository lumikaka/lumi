package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/llm"
	"lumi/internal/provider"

	"github.com/labstack/echo/v4"
)

type providerCheckFake struct {
	baseURL string
	model   string
	key     string
	err     error
}

func (fake *providerCheckFake) Generate(_ context.Context, request llm.Request, _ func(string) error) (llm.Response, error) {
	fake.baseURL, fake.key, fake.model = request.BaseURL, request.APIKey, request.Model
	return llm.Response{}, fake.err
}

func TestProviderHandlersKeepSecretsOutOfAPIEnvelopes(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	service := provider.NewService(app, provider.NewMemorySecretStore())
	model := &providerCheckFake{}
	handler := NewProviderHandler(service, model)
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.GET("/api/v1/providers", handler.Index)
	e.POST("/api/v1/providers/:provider_uuid/connection-checks", handler.Check)

	secret := "handler-secret-must-not-return"
	created, err := service.Create(context.Background(), provider.CreateInput{ProviderType: "cloudflare_ai_gateway", AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/story-model", APIKey: secret})
	if err != nil {
		t.Fatal(err)
	}
	listed := requestJSON(t, e, "GET", "/api/v1/providers", nil)
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), `"data":{"items":[`) || strings.Contains(listed.Body.String(), secret) {
		t.Fatalf("list response = %d %s", listed.Code, listed.Body.String())
	}
	checked := requestJSON(t, e, "POST", "/api/v1/providers/"+created.UUID+"/connection-checks", map[string]any{})
	if checked.Code != 200 || model.model != "test/story-model" || model.key != secret || model.baseURL != "https://api.cloudflare.com/client/v4/accounts/0123456789abcdef0123456789abcdef/ai/v1" || strings.Contains(checked.Body.String(), secret) {
		t.Fatalf("check response = %d %s, model=%q base_url=%q", checked.Code, checked.Body.String(), model.model, model.baseURL)
	}
}

func TestProviderConnectionCheckActivatesFirstReadyProvider(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	service := provider.NewService(app, provider.NewMemorySecretStore())
	model := &providerCheckFake{}
	handler := NewProviderHandler(service, model)
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.POST("/api/v1/providers/:provider_uuid/connection-checks", handler.Check)

	if _, _, err := service.Settings().Update(context.Background(), map[string]any{
		"ai_providers.openai_compatible.account_id":          "0123456789abcdef0123456789abcdef",
		"ai_providers.openai_compatible.default_model":       "test/story-model",
		"ai_providers.openai_compatible.default_image_model": "test/image-model",
		"ai_providers.openai_compatible.api_key":             "check-secret",
	}); err != nil {
		t.Fatal(err)
	}
	items, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	checked := requestJSON(t, e, "POST", "/api/v1/providers/"+items[0].UUID+"/connection-checks", map[string]any{})
	if checked.Code != http.StatusOK || !strings.Contains(checked.Body.String(), `"activated":true`) {
		t.Fatalf("check response = %d %s", checked.Code, checked.Body.String())
	}
	active, err := service.Active(context.Background())
	if err != nil || active.ProviderType != provider.TypeCloudflareAIGateway || !active.Ready {
		t.Fatalf("active provider = %+v, error = %v", active.Provider, err)
	}
}
