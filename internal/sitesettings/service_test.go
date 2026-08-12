package sitesettings

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
)

func testService(t *testing.T) (*appstore.Store, *MemoryMasterKeyStore, *Service) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "app")
	store, err := appstore.Open(directory, config.SQLiteDSN(filepath.Join(directory, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	keys := NewMemoryMasterKeyStore()
	service := NewService(store, keys)
	t.Cleanup(func() { service.Close(); _ = store.Close() })
	return store, keys, service
}

func TestSettingsSnapshotIsReadThroughAndMutationRefreshesIt(t *testing.T) {
	ctx := context.Background()
	store, _, service := testService(t)
	first, err := service.Value(ctx, CloudflareDefaultModelKey)
	if err != nil || first != "deepseek/deepseek-v4-pro" {
		t.Fatalf("first value=%v error=%v", first, err)
	}
	// A direct database write is deliberately invisible: all legitimate writes
	// must pass through Service.Update.
	if err := store.DB().Exec(`INSERT INTO site_settings(key,value,updated_at) VALUES(?,?,CURRENT_TIMESTAMP)`, CloudflareDefaultModelKey, `"bypassed/model"`).Error; err != nil {
		t.Fatal(err)
	}
	stillCached, _ := service.Value(ctx, CloudflareDefaultModelKey)
	if stillCached != "deepseek/deepseek-v4-pro" {
		t.Fatalf("read-through cache was reloaded unexpectedly: %v", stillCached)
	}
	if _, _, err := service.Update(ctx, map[string]any{CloudflareDefaultModelKey: "test/updated"}); err != nil {
		t.Fatal(err)
	}
	updated, _ := service.Value(ctx, CloudflareDefaultModelKey)
	if updated != "test/updated" {
		t.Fatalf("updated cache=%v", updated)
	}
	if err := store.DB().Exec(`DROP TABLE site_settings`).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Update(ctx, map[string]any{CloudflareDefaultModelKey: "test/must-not-stick"}); err == nil {
		t.Fatal("database failure did not fail the update")
	}
	unchanged, err := service.Value(ctx, CloudflareDefaultModelKey)
	if err != nil || unchanged != "test/updated" {
		t.Fatalf("failed transaction polluted cache: value=%v error=%v", unchanged, err)
	}
}

func TestSecretEnvelopeUsesRandomNonceAADAndNeverReturnsPlaintext(t *testing.T) {
	ctx := context.Background()
	store, keys, service := testService(t)
	secret := "lumi-plaintext-must-not-be-stored"
	if _, _, err := service.Update(ctx, map[string]any{CloudflareAPITokenKey: secret}); err != nil {
		t.Fatal(err)
	}
	var first string
	if err := store.DB().Raw(`SELECT value FROM site_settings WHERE key=?`, CloudflareAPITokenKey).Scan(&first).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(first, secret) || !strings.Contains(first, `"algorithm":"AES-256-GCM"`) {
		t.Fatalf("invalid secret envelope: %s", first)
	}
	if _, _, err := service.Update(ctx, map[string]any{CloudflareAPITokenKey: secret}); err != nil {
		t.Fatal(err)
	}
	var second string
	_ = store.DB().Raw(`SELECT value FROM site_settings WHERE key=?`, CloudflareAPITokenKey).Scan(&second).Error
	if first == second {
		t.Fatal("re-encryption reused a nonce")
	}
	response, err := service.List(ctx)
	if err != nil || response.Settings[CloudflareAPITokenKey] != nil {
		t.Fatalf("secret response=%+v error=%v", response.Settings, err)
	}
	var item Item
	for _, candidate := range response.Items {
		if candidate.Key == CloudflareAPITokenKey {
			item = candidate
		}
	}
	if item.Value != nil || item.HasValue == nil || !*item.HasValue || item.MaskedValue == nil || item.SecretState != "available" || strings.Contains(*item.MaskedValue, secret) {
		t.Fatalf("public secret item=%+v", item)
	}
	if keys.GetCount != 1 || keys.SetCount != 1 {
		t.Fatalf("master key accesses get=%d set=%d", keys.GetCount, keys.SetCount)
	}

	if _, _, err := service.Update(ctx, map[string]any{BailianAPIKeyKey: "another-secret"}); err != nil {
		t.Fatal(err)
	}
	var openAIEnvelope, bailianEnvelope string
	_ = store.DB().Raw(`SELECT value FROM site_settings WHERE key=?`, CloudflareAPITokenKey).Scan(&openAIEnvelope).Error
	_ = store.DB().Raw(`SELECT value FROM site_settings WHERE key=?`, BailianAPIKeyKey).Scan(&bailianEnvelope).Error
	if err := store.DB().Exec(`UPDATE site_settings SET value=CASE key WHEN ? THEN ? WHEN ? THEN ? END WHERE key IN (?,?)`, CloudflareAPITokenKey, bailianEnvelope, BailianAPIKeyKey, openAIEnvelope, CloudflareAPITokenKey, BailianAPIKeyKey).Error; err != nil {
		t.Fatal(err)
	}
	fresh := NewService(store, keys)
	t.Cleanup(fresh.Close)
	if _, err := fresh.Secret(ctx, CloudflareAPITokenKey); err == nil {
		t.Fatal("ciphertext was accepted under a different setting key; AAD was not enforced")
	}
}

func TestMasterKeyCacheConcurrentLoadRetryAndMissingKeyProtection(t *testing.T) {
	ctx := context.Background()
	store, keys, service := testService(t)
	if _, _, err := service.Update(ctx, map[string]any{CloudflareAPITokenKey: "cached-secret"}); err != nil {
		t.Fatal(err)
	}
	keys.GetCount = 0
	fresh := NewService(store, keys)
	t.Cleanup(fresh.Close)
	var wg sync.WaitGroup
	errorsSeen := make(chan error, 24)
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := fresh.Secret(ctx, CloudflareAPITokenKey)
			if err != nil || value != "cached-secret" {
				errorsSeen <- errors.Join(err, errors.New("unexpected secret value"))
			}
		}()
	}
	wg.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Fatal(err)
	}
	if keys.GetCount != 1 {
		t.Fatalf("concurrent first load accessed key store %d times", keys.GetCount)
	}

	retrying := NewService(store, keys)
	t.Cleanup(retrying.Close)
	keys.GetError = errors.New("temporary keychain outage")
	if _, err := retrying.Secret(ctx, CloudflareAPITokenKey); err == nil {
		t.Fatal("temporary keychain failure was ignored")
	}
	keys.GetError = nil
	if value, err := retrying.Secret(ctx, CloudflareAPITokenKey); err != nil || value != "cached-secret" {
		t.Fatalf("keychain retry value=%q error=%v", value, err)
	}

	keys.Clear()
	missing := NewService(store, keys)
	t.Cleanup(missing.Close)
	setCount := keys.SetCount
	if _, _, err := missing.Update(ctx, map[string]any{BailianAPIKeyKey: "must-not-generate-a-new-root"}); err == nil {
		t.Fatal("missing keychain key was regenerated while ciphertext existed")
	}
	if keys.SetCount != setCount {
		t.Fatalf("missing root key was overwritten: set count %d -> %d", setCount, keys.SetCount)
	}
}
