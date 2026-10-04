package appstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"lumi/internal/config"
	"lumi/internal/dbmigrate"
)

func TestCloudflareProviderRateLimitDefaultSeedsOnce(t *testing.T) {
	directory := t.TempDir()
	dsn := config.SQLiteDSN(filepath.Join(directory, "app.sqlite"))
	store, err := Open(directory, dsn)
	if err != nil {
		t.Fatal(err)
	}
	row, err := store.GlobalModelSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	limit, ok := row.ProviderRateLimits["cloudflare_ai_gateway"]
	if !ok || limit.RPM == nil || *limit.RPM != 200 || limit.RPS != nil || row.Revision != 0 {
		t.Fatalf("default limit=%+v present=%v revision=%d", limit, ok, row.Revision)
	}
	if _, ok := row.ProviderRateLimits["aliyun_bailian"]; ok {
		t.Fatal("Bailian has no published provider-wide limit to seed")
	}
	if _, err := store.PatchGlobalModelSettings(context.Background(), 0, nil, nil, map[string]*GlobalModelRateLimit{
		"cloudflare_ai_gateway": nil,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(directory, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	row, err = store.GlobalModelSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := row.ProviderRateLimits["cloudflare_ai_gateway"]; ok {
		t.Fatal("deleted default limit was recreated after restart")
	}
}

func TestCloudflareProviderRateLimitMigrationPreservesExistingRule(t *testing.T) {
	directory := t.TempDir()
	dsn := config.SQLiteDSN(filepath.Join(directory, "app.sqlite"))
	store, err := Open(directory, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	runner, err := dbmigrate.OpenApp(dsn)
	if err != nil {
		t.Fatal(err)
	}
	for {
		version, _, _, err := runner.Version()
		if err != nil {
			t.Fatal(err)
		}
		if version <= 20261003000013 {
			break
		}
		if err := runner.Down(1); err != nil {
			t.Fatal(err)
		}
	}
	if err := runner.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE global_model_settings SET provider_rate_limits = json_set(provider_rate_limits, '$.cloudflare_ai_gateway', json('{"rpm":77,"rps":2}')) WHERE singleton = 1`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(directory, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	row, err := store.GlobalModelSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if row.Revision != 0 {
		t.Fatalf("migration changed revision to %d", row.Revision)
	}
	limit := row.ProviderRateLimits["cloudflare_ai_gateway"]
	if limit.RPM == nil || *limit.RPM != 77 || limit.RPS == nil || *limit.RPS != 2 {
		t.Fatalf("migration overwrote existing rule: %+v", limit)
	}
}
