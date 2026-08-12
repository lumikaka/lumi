package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/sitesettings"
)

func TestProviderPersistsOnlyEncryptedSiteSetting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	databasePath := filepath.Join(dataDir, "lumi.sqlite")
	store, err := appstore.Open(dataDir, config.SQLiteDSN(databasePath))
	if err != nil {
		t.Fatal(err)
	}
	secrets := NewMemorySecretStore()
	service := NewService(store, secrets)
	apiKey := "lumi-test-secret-never-in-sqlite-7f828181"
	created, err := service.Create(ctx, CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/story-model", APIKey: apiKey})
	if err != nil {
		t.Fatal(err)
	}
	if created.UUID == "" || !created.HasSecret || created.ProviderType != TypeCloudflareAIGateway || created.AccountID != "0123456789abcdef0123456789abcdef" || created.BaseURL != "https://api.cloudflare.com/client/v4/accounts/0123456789abcdef0123456789abcdef/ai/v1" {
		t.Fatalf("created provider = %+v", created)
	}
	resolved, err := service.Resolve(ctx, created.UUID)
	if err != nil || resolved.APIKey != apiKey {
		t.Fatalf("resolved = %+v, error = %v", resolved, err)
	}
	resolvedJSON, err := json.Marshal(resolved)
	if err != nil || bytes.Contains(resolvedJSON, []byte(apiKey)) || bytes.Contains(resolvedJSON, []byte("APIKey")) {
		t.Fatalf("resolved provider JSON leaked secret: %s, error = %v", resolvedJSON, err)
	}

	encoded, err := json.Marshal(created)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"api_key", "secret_ref", `"id"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("public provider JSON contains %q: %s", forbidden, encoded)
		}
	}
	var ciphertext string
	if err := store.DB().WithContext(ctx).Raw("SELECT value FROM site_settings WHERE key = ?", "ai_providers.openai_compatible.api_key").Scan(&ciphertext).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ciphertext, `"algorithm":"AES-256-GCM"`) || strings.Contains(ciphertext, apiKey) {
		t.Fatalf("encrypted setting = %q", ciphertext)
	}

	if err := store.DB().Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{databasePath, databasePath + "-wal"} {
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			t.Fatal(readErr)
		}
		if bytes.Contains(contents, []byte(apiKey)) {
			t.Fatalf("API key leaked into %s", path)
		}
	}
}

func TestProviderSettingUpdateInvalidatesVerification(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	store, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store, NewMemorySecretStore())
	created, err := service.Create(ctx, CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/first", APIKey: "old-secret"})
	if err != nil {
		t.Fatal(err)
	}
	replacement := "replacement-secret"
	_, _, err = service.Settings().Update(ctx, map[string]any{
		"ai_providers.openai_compatible.default_model": "test/second",
		"ai_providers.openai_compatible.api_key":       replacement,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.Get(ctx, created.UUID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(updated)
	if bytes.Contains(encoded, []byte(replacement)) || updated.DisplayName != "Cloudflare AI Gateway" || updated.DefaultModel != "test/second" || !updated.HasSecret || updated.Verified {
		t.Fatalf("updated provider = %s", encoded)
	}
	resolved, err := service.Resolve(ctx, created.UUID)
	if err != nil || resolved.APIKey != replacement {
		t.Fatalf("resolved replacement = %+v, error = %v", resolved, err)
	}
	if _, err := service.Activate(ctx, TypeCloudflareAIGateway); err == nil {
		t.Fatal("activation succeeded before the updated configuration was verified")
	}
}

func TestBailianEndpointsAndSingleActiveProvider(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	store, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store, NewMemorySecretStore())
	if _, err := service.Create(ctx, CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/text", APIKey: "cloudflare-secret"}); err != nil {
		t.Fatal(err)
	}

	regions := []string{"cn-beijing", "ap-southeast-1", "eu-central-1", "ap-northeast-1"}
	var bailian Provider
	for _, region := range regions {
		if _, _, err := service.Settings().Update(ctx, map[string]any{
			"ai_providers.aliyun_bailian.workspace_id": "ws-123",
			"ai_providers.aliyun_bailian.region":       region,
			"ai_providers.aliyun_bailian.api_key":      "bailian-secret",
		}); err != nil {
			t.Fatal(err)
		}
		items, err := service.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		bailian = items[1]
		host := "https://ws-123." + region + ".maas.aliyuncs.com"
		if bailian.BaseURL != host+"/compatible-mode/v1" || bailian.ImageBaseURL != host+"/api/v1/services/aigc/multimodal-generation/generation" || bailian.DefaultModel != "qwen3.7-plus" || bailian.DefaultImageModel != "qwen-image-3.0" {
			t.Fatalf("region %s provider=%+v", region, bailian)
		}
	}
	if _, err := service.MarkVerified(ctx, bailian.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(ctx, TypeAliyunBailian); err != nil {
		t.Fatal(err)
	}
	items, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, item := range items {
		if item.Active {
			active++
			if item.ProviderType != TypeAliyunBailian {
				t.Fatalf("wrong active provider: %+v", item)
			}
		}
	}
	if active != 1 {
		t.Fatalf("active provider count=%d items=%+v", active, items)
	}
}

func TestConnectionCheckFingerprintCannotVerifyAChangedConfiguration(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	store, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store, NewMemorySecretStore())
	created, err := service.Create(ctx, CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/old-model", APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	checked, err := service.Resolve(ctx, created.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Settings().Update(ctx, map[string]any{sitesettings.CloudflareDefaultModelKey: "test/new-model"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.MarkVerified(ctx, created.UUID, checked.ConfigFingerprint); err != nil {
		t.Fatal(err)
	}
	current, err := service.Get(ctx, created.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Verified || current.Ready {
		t.Fatalf("stale connection check verified changed config: %+v", current)
	}
	if _, err := service.Activate(ctx, TypeCloudflareAIGateway); err == nil {
		t.Fatal("changed configuration activated using a stale connection check")
	}
}
