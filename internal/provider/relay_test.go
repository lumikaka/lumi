package provider

import (
	"context"
	"encoding/json"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/sitesettings"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRelayConfigurationCredentialsAndVerification(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	app, err := appstore.Open(dir, config.SQLiteDSN(filepath.Join(dir, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	service := NewService(app, NewMemorySecretStore())
	defer service.Close()
	items, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var relay Provider
	for _, item := range items {
		if item.ProviderType == TypeOpenAIRelay {
			relay = item
		}
	}
	if !isUUIDv7(relay.UUID) || relay.Configured || relay.DefaultModel != "gpt-6.1-sol" || relay.DefaultImageModel != "gpt-6.1-sol" {
		t.Fatalf("initial=%+v", relay)
	}
	if !slices.Equal(SupportedTextModels(relay), RelayModels()) || !slices.Equal(SupportedImageModels(relay), RelayModels()) {
		t.Fatal("wrong model options")
	}
	update := func(values map[string]any) {
		t.Helper()
		if _, _, err := service.Settings().Update(ctx, values); err != nil {
			t.Fatal(err)
		}
	}
	update(map[string]any{sitesettings.RelayBaseURLKey: " https://text.example/v1/ ", sitesettings.RelayAPIKeyKey: "relay-text-secret"})
	partial, _ := service.Get(ctx, relay.UUID)
	if partial.Configured || partial.HasSecret {
		t.Fatal("partial configuration accepted")
	}
	if _, err := service.Activate(ctx, TypeOpenAIRelay); err == nil {
		t.Fatal("activated incomplete provider")
	}
	update(map[string]any{sitesettings.RelayImageBaseURLKey: "https://image.example/api/v1/", sitesettings.RelayImageAPIKeyKey: "relay-image-secret", sitesettings.RelayDefaultImageModelKey: "gpt-6.1-sol"})
	resolved, err := service.Resolve(ctx, relay.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.APIKey != "relay-text-secret" || resolved.ImageAPIKey != "relay-image-secret" || resolved.BaseURL != "https://text.example/v1" || resolved.ImageBaseURL != "https://image.example/api/v1" {
		t.Fatal("credential or endpoint mismatch")
	}
	public, _ := json.Marshal(resolved)
	settings, _ := service.Settings().List(ctx)
	settingsJSON, _ := json.Marshal(settings)
	for _, secret := range []string{resolved.APIKey, resolved.ImageAPIKey} {
		if strings.Contains(string(public)+string(settingsJSON), secret) {
			t.Fatal("secret exposed in public JSON")
		}
	}
	for _, key := range []string{sitesettings.RelayAPIKeyKey, sitesettings.RelayImageAPIKeyKey} {
		var raw string
		if err := app.DB().Raw("SELECT value FROM site_settings WHERE key = ?", key).Scan(&raw).Error; err != nil {
			t.Fatal(err)
		}
		if raw == "" || strings.Contains(raw, "relay-text-secret") || strings.Contains(raw, "relay-image-secret") {
			t.Fatal("secret not encrypted")
		}
	}
	if _, err := service.MarkVerified(ctx, relay.UUID, resolved.ConfigFingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(ctx, TypeOpenAIRelay); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{sitesettings.RelayBaseURLKey, sitesettings.RelayImageBaseURLKey, sitesettings.RelayAPIKeyKey, sitesettings.RelayImageAPIKeyKey, sitesettings.RelayDefaultModelKey, sitesettings.RelayDefaultImageModelKey} {
		before, err := service.Resolve(ctx, relay.UUID)
		if err != nil {
			t.Fatal(err)
		}
		value := "changed-secret"
		if strings.Contains(key, "base_url") {
			value = "https://changed.example/v1"
		}
		if strings.Contains(key, "model") {
			value = "gpt-5.6-sol"
		}
		update(map[string]any{key: value})
		after, err := service.MarkVerified(ctx, relay.UUID, before.ConfigFingerprint)
		if err != nil || after.Verified || after.Ready {
			t.Fatalf("stale check accepted after %s: %v", key, err)
		}
		if _, err := service.MarkVerified(ctx, relay.UUID); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{sitesettings.RelayAPIKeyKey, sitesettings.RelayImageAPIKeyKey} {
		if _, _, err := service.Settings().Reset(ctx, []string{key}); err != nil {
			t.Fatal(err)
		}
		current, _ := service.Get(ctx, relay.UUID)
		if current.HasSecret || current.Configured || current.Verified || current.Ready {
			t.Fatalf("reset did not invalidate: %+v", current)
		}
		other := sitesettings.RelayAPIKeyKey
		if key == other {
			other = sitesettings.RelayImageAPIKeyKey
		}
		remaining, _ := service.Settings().Secret(ctx, other)
		if remaining == "" {
			t.Fatal("reset cleared other group")
		}
		update(map[string]any{key: "restored-secret"})
	}
	for _, tc := range []struct{ key, value string }{
		{sitesettings.RelayBaseURLKey, "ftp://example.com"}, {sitesettings.RelayImageBaseURLKey, "https://key@example.com/v1"},
		{sitesettings.RelayBaseURLKey, "https://example.com/v1?key=x"}, {sitesettings.RelayImageBaseURLKey, "https://example.com/v1#x"},
		{sitesettings.RelayDefaultModelKey, "custom-model"}, {sitesettings.RelayDefaultImageModelKey, "openai/gpt-6"},
	} {
		if _, _, err := service.Settings().Update(ctx, map[string]any{tc.key: tc.value}); err == nil {
			t.Fatalf("accepted invalid %s", tc.value)
		}
	}
	for _, model := range RelayModels() {
		update(map[string]any{sitesettings.RelayDefaultModelKey: model, sitesettings.RelayDefaultImageModelKey: model})
	}
}

func TestRelayModePreservesLegacyFingerprintAndInvalidatesOnSwitch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	app, err := appstore.Open(dir, config.SQLiteDSN(filepath.Join(dir, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	service := NewService(app, NewMemorySecretStore())
	defer service.Close()
	settings := service.Settings()
	if _, _, err := settings.Update(ctx, map[string]any{sitesettings.RelayBaseURLKey: "https://text.example/v1", sitesettings.RelayImageBaseURLKey: "https://image.example/v1", sitesettings.RelayAPIKeyKey: "text-key", sitesettings.RelayImageAPIKeyKey: "image-key"}); err != nil {
		t.Fatal(err)
	}
	items, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var relay Provider
	for _, item := range items {
		if item.ProviderType == TypeOpenAIRelay {
			relay = item
		}
	}
	legacy, err := settings.Fingerprint(ctx, sitesettings.RelayBaseURLKey, sitesettings.RelayImageBaseURLKey, sitesettings.RelayAPIKeyKey, sitesettings.RelayImageAPIKeyKey, sitesettings.RelayDefaultModelKey, sitesettings.RelayDefaultImageModelKey)
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.providerFingerprint(ctx, TypeOpenAIRelay)
	if err != nil || current != legacy {
		t.Fatal("legacy verification fingerprint changed")
	}
	if _, err := service.MarkVerified(ctx, relay.UUID, legacy); err != nil {
		t.Fatal(err)
	}
	if _, _, err := settings.Update(ctx, map[string]any{sitesettings.RelayImageAPIModeKey: sitesettings.ImageAPIImages, sitesettings.RelayDefaultImageModelKey: "gpt-image-2"}); err != nil {
		t.Fatal(err)
	}
	relay, err = service.Get(ctx, relay.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if relay.Verified || relay.ImageAPIMode != sitesettings.ImageAPIImages || !slices.Equal(SupportedImageModels(relay), sitesettings.RelayImageModels(sitesettings.ImageAPIImages)) || !slices.Equal(SupportedTextModels(relay), RelayModels()) {
		t.Fatalf("provider=%+v", relay)
	}
	if stale, err := service.MarkVerified(ctx, relay.UUID, legacy); err != nil || stale.Verified {
		t.Fatalf("stale=%+v err=%v", stale, err)
	}
	if _, err := service.MarkVerified(ctx, relay.UUID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := settings.Update(ctx, map[string]any{sitesettings.RelayImageAPIModeKey: sitesettings.ImageAPIResponses, sitesettings.RelayDefaultImageModelKey: "gpt-6.1-sol"}); err != nil {
		t.Fatal(err)
	}
	relay, err = service.Get(ctx, relay.UUID)
	if err != nil || relay.Verified {
		t.Fatal("switch back retained verification")
	}
	current, err = service.providerFingerprint(ctx, TypeOpenAIRelay)
	if err != nil || current != legacy {
		t.Fatal("explicit Responses mode changed legacy fingerprint")
	}
}
