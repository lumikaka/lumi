package httpapi

import (
	"context"
	"encoding/json"
	"github.com/labstack/echo/v4"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/pricing"
	"lumi/internal/project"
	"lumi/internal/provider"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelPriceAPIContracts(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	app, err := appstore.Open(dir, config.SQLiteDSN(filepath.Join(dir, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	manager := project.NewManager(app)
	t.Cleanup(func() { _ = manager.Close(); providers.Close(); _ = app.Close() })
	p, err := providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/model", DefaultImageModel: "test/image", APIKey: "secret-must-not-leak"})
	if err != nil {
		t.Fatal(err)
	}
	projectHeader, err := manager.Create(ctx, "Pricing API", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	h := NewModelPriceHandler(providers, manager, nil)
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.GET("/api/v1/model-prices", h.Index)
	e.POST("/api/v1/model-prices", h.Create)
	e.DELETE("/api/v1/model-prices/:price_uuid", h.Delete)
	e.GET("/api/v1/projects/:project_uuid/llm-cost-summary", h.Summary)
	e.POST("/api/v1/projects/:project_uuid/llm-cost-backfills", h.Preview)
	r := pricing.Rule{ProviderUUID: p.UUID, ProviderType: p.ProviderType, Model: "test/model", RequestType: "text", Mode: "tokens", Currency: "USD", Rates: []pricing.Rate{{Metric: "input_tokens", Price: "1"}, {Metric: "cached_input_tokens", Price: "0.1"}, {Metric: "output_tokens", Price: "2"}}}
	created := requestJSON(t, e, "POST", "/api/v1/model-prices", map[string]any{"rule": r})
	if created.Code != 201 {
		t.Fatalf("%d %s", created.Code, created.Body)
	}
	var envelope struct {
		Success bool          `json:"success"`
		Data    pricing.Price `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Success || !pricing.ValidUUID(envelope.Data.UUID) {
		t.Fatal(envelope)
	}
	for _, forbidden := range []string{`"id"`, `"provider_id"`, `"catalog_key"`, "secret-must-not-leak"} {
		if strings.Contains(created.Body.String(), forbidden) {
			t.Fatal(created.Body.String())
		}
	}
	conflict := requestJSON(t, e, "POST", "/api/v1/model-prices", map[string]any{"rule": r})
	if conflict.Code != 409 {
		t.Fatalf("%d %s", conflict.Code, conflict.Body)
	}
	listed := requestJSON(t, e, "GET", "/api/v1/model-prices", nil)
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), `"items":[`) {
		t.Fatal(listed.Body.String())
	}
	summary := requestJSON(t, e, "GET", "/api/v1/projects/"+projectHeader.UUID+"/llm-cost-summary", nil)
	if summary.Code != 200 || !strings.Contains(summary.Body.String(), `"totals":[]`) {
		t.Fatalf("%d %s", summary.Code, summary.Body)
	}
	invalid := requestJSON(t, e, "POST", "/api/v1/projects/"+projectHeader.UUID+"/llm-cost-backfills", map[string]any{"price_uuids": []string{"internal-id-1"}})
	if invalid.Code != 422 {
		t.Fatalf("%d %s", invalid.Code, invalid.Body)
	}
	deleted := requestJSON(t, e, "DELETE", "/api/v1/model-prices/"+envelope.Data.UUID, nil)
	if deleted.Code != 200 || !strings.Contains(deleted.Body.String(), `"data":null`) {
		t.Fatal(deleted.Body.String())
	}
}
