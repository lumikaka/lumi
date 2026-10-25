package agentevals

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/llm"
	"lumi/internal/provider"
)

type Config struct {
	ProviderType   string   `json:"provider_type"`
	BaseURL        string   `json:"base_url"`
	Model          string   `json:"model"`
	APIKey         string   `json:"-"`
	Temperature    *float64 `json:"temperature"`
	MaxTokens      int      `json:"max_tokens"`
	MaxRequests    int      `json:"max_requests_per_turn"`
	TimeoutSeconds int      `json:"timeout_seconds_per_case"`
	Repetitions    int      `json:"repetitions"`
}

func (c Config) Validate() error {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("base URL must be HTTP(S) without credentials, query or fragment")
	}
	if c.Model == "" || c.APIKey == "" {
		return fmt.Errorf("model and API key are required")
	}
	switch c.ProviderType {
	case provider.LegacyTypeOpenAICompatible, provider.TypeAliyunBailian, provider.TypeCloudflareAIGateway, provider.TypeOpenAIRelay:
	default:
		return fmt.Errorf("unsupported provider type")
	}
	if c.Repetitions < 1 || c.Repetitions > 100 || c.MaxRequests < 2 || c.MaxRequests > 100 || c.MaxTokens < 1 || c.TimeoutSeconds < 1 || c.TimeoutSeconds > 3600 {
		return fmt.Errorf("invalid repetitions, token budget, request budget or timeout")
	}
	if c.Temperature != nil && (math.IsNaN(*c.Temperature) || math.IsInf(*c.Temperature, 0) || *c.Temperature < 0 || *c.Temperature > 2) {
		return fmt.Errorf("temperature must be in [0,2]")
	}
	return nil
}
func (c Config) request() llm.ChatRequest {
	return llm.ChatRequest{ProviderType: c.ProviderType, BaseURL: c.BaseURL, APIKey: c.APIKey, Model: c.Model, Temperature: c.Temperature, MaxTokens: c.MaxTokens, Timeout: time.Duration(c.TimeoutSeconds) * time.Second}
}

// ReadLumiProvider copies only provider-related settings from a read-only app
// connection into a temporary store, then uses Lumi's existing secret codec.
// No project, conversation or business data is loaded; the source is not migrated.
func ReadLumiProvider(ctx context.Context, directory, providerType string) (provider.Resolved, error) {
	source, err := sql.Open("sqlite", config.SQLiteFileURI(filepath.Join(directory, "lumi.sqlite"))+"?mode=ro")
	if err != nil {
		return provider.Resolved{}, err
	}
	defer source.Close()
	rows, err := source.QueryContext(ctx, "SELECT key,value FROM site_settings WHERE key LIKE 'ai_providers.%' OR key='ai_provider.active'")
	if err != nil {
		return provider.Resolved{}, err
	}
	defer rows.Close()
	tmp, err := os.MkdirTemp("", "lumi-eval-provider-")
	if err != nil {
		return provider.Resolved{}, err
	}
	defer os.RemoveAll(tmp)
	app, err := appstore.Open(tmp, config.SQLiteDSN(filepath.Join(tmp, "lumi.sqlite")))
	if err != nil {
		return provider.Resolved{}, err
	}
	defer app.Close()
	for rows.Next() {
		var k, v string
		if err = rows.Scan(&k, &v); err != nil {
			return provider.Resolved{}, err
		}
		if err = app.DB().Exec("INSERT OR REPLACE INTO site_settings(key,value,updated_at) VALUES(?,?,?)", k, v, time.Now().UTC()).Error; err != nil {
			return provider.Resolved{}, err
		}
	}
	if err = rows.Err(); err != nil {
		return provider.Resolved{}, err
	}
	svc := provider.NewService(app, provider.NewOSSecretStore())
	defer svc.Close()
	if strings.TrimSpace(providerType) == "" {
		return svc.Active(ctx)
	}
	choices, err := svc.List(ctx)
	if err != nil {
		return provider.Resolved{}, err
	}
	for _, p := range choices {
		if p.ProviderType == providerType {
			return svc.Resolve(ctx, p.UUID)
		}
	}
	return provider.Resolved{}, fmt.Errorf("requested provider type is not configured")
}
