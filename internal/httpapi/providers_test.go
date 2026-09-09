package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/llm"
	"lumi/internal/provider"
	"lumi/internal/sitesettings"

	"github.com/labstack/echo/v4"
)

type providerCheckFake struct {
	baseURL      string
	model        string
	key          string
	providerType string
	err          error
}

type providerCheckTransportFunc func(*http.Request) (*http.Response, error)

func (fn providerCheckTransportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestCloudflareCheckReturnsSanitizedUpstreamFailure(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	service := provider.NewService(app, provider.NewMemorySecretStore())
	t.Cleanup(service.Close)
	const secret = "test-cloudflare-secret"
	created, err := service.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "openai/gpt-5.6-sol", APIKey: secret})
	if err != nil {
		t.Fatal(err)
	}
	client := llm.NewOpenAICompatibleClient(&http.Client{Transport: providerCheckTransportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/client/v4/accounts/0123456789abcdef0123456789abcdef/ai/v1/responses" {
			t.Fatalf("path = %s", req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "Cf-Ray": []string{"test-ray"}},
			Body:       io.NopCloser(strings.NewReader(`{"success":false,"errors":[{"code":1001,"message":"Model request rejected. api_key=test-cloudflare-secret Bearer echoed-token"}]}`)),
		}, nil
	})})
	handler := NewProviderHandler(service, client)
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.POST("/api/v1/providers/:provider_uuid/connection-checks", handler.Check)
	checked := requestJSON(t, e, "POST", "/api/v1/providers/"+created.UUID+"/connection-checks", nil)
	var result Envelope
	if err := json.Unmarshal(checked.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if checked.Code != http.StatusBadGateway || result.Success || result.Data != nil || result.Error == nil || result.Error.Code != llm.CodeProviderResponse {
		t.Fatalf("response = %d %s", checked.Code, checked.Body.String())
	}
	for _, expected := range []string{"Provider HTTP status: 400", "Provider code: 1001", "Model request rejected.", "Provider request ID: test-ray", "[REDACTED]"} {
		if !strings.Contains(result.Error.Details, expected) {
			t.Fatalf("missing %q in %s", expected, result.Error.Details)
		}
	}
	for _, forbidden := range []string{secret, "echoed-token"} {
		if strings.Contains(checked.Body.String(), forbidden) {
			t.Fatal("response leaked credentials")
		}
	}
}

func (fake *providerCheckFake) CheckConnection(_ context.Context, providerType string, request llm.Request) error {
	fake.baseURL, fake.key, fake.model = request.BaseURL, request.APIKey, request.Model
	fake.providerType = providerType
	return fake.err
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
	if model.providerType != provider.TypeCloudflareAIGateway {
		t.Fatalf("checked provider type = %q", model.providerType)
	}
	if checked.Code != 200 || model.model != "test/story-model" || model.key != secret || model.baseURL != "https://api.cloudflare.com/client/v4/accounts/0123456789abcdef0123456789abcdef/ai/v1" || strings.Contains(checked.Body.String(), secret) {
		t.Fatalf("check response = %d %s, model=%q base_url=%q", checked.Code, checked.Body.String(), model.model, model.baseURL)
	}
}

func TestProviderChecksKeepVerificationStatesIndependent(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	service := provider.NewService(app, provider.NewMemorySecretStore())
	t.Cleanup(service.Close)
	if _, _, err := service.Settings().Update(ctx, map[string]any{
		sitesettings.CloudflareAccountIDKey: "0123456789abcdef0123456789abcdef",
		sitesettings.CloudflareAPITokenKey:  "cloudflare-secret",
		sitesettings.BailianWorkspaceKey:    "ws-123",
		sitesettings.BailianRegionKey:       "cn-beijing",
		sitesettings.BailianAPIKeyKey:       "bailian-secret",
	}); err != nil {
		t.Fatal(err)
	}
	items, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byType := map[string]provider.Provider{}
	for _, item := range items {
		byType[item.ProviderType] = item
	}
	model := &providerCheckFake{}
	handler := NewProviderHandler(service, model)
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.POST("/api/v1/providers/:provider_uuid/connection-checks", handler.Check)
	check := func(providerType string, status int) {
		t.Helper()
		checked := requestJSON(t, e, "POST", "/api/v1/providers/"+byType[providerType].UUID+"/connection-checks", nil)
		if checked.Code != status || model.providerType != providerType {
			t.Fatalf("check %s = %d %s, dispatched to %s", providerType, checked.Code, checked.Body.String(), model.providerType)
		}
	}
	check(provider.TypeAliyunBailian, 200)
	cloudflare, err := service.Get(ctx, byType[provider.TypeCloudflareAIGateway].UUID)
	if err != nil || cloudflare.Verified {
		t.Fatalf("Cloudflare verified by Bailian check: %+v, %v", cloudflare, err)
	}
	if _, err := service.Activate(ctx, provider.TypeAliyunBailian); err != nil {
		t.Fatal(err)
	}
	check(provider.TypeCloudflareAIGateway, 200)
	if _, _, err := service.Settings().Update(ctx, map[string]any{sitesettings.CloudflareAPITokenKey: "replacement-secret"}); err != nil {
		t.Fatal(err)
	}
	model.err = &llm.Error{Code: llm.CodeAuthentication, SafeMessage: "验证失败"}
	check(provider.TypeCloudflareAIGateway, 502)
	cloudflare, err = service.Get(ctx, byType[provider.TypeCloudflareAIGateway].UUID)
	if err != nil || cloudflare.Verified {
		t.Fatalf("failed Cloudflare check marked verified: %+v, %v", cloudflare, err)
	}
	bailian, err := service.Get(ctx, byType[provider.TypeAliyunBailian].UUID)
	if err != nil || !bailian.Verified || !bailian.Active || !bailian.Ready {
		t.Fatalf("Cloudflare check affected Bailian: %+v, %v", bailian, err)
	}
}
