package dbmigrate

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestGlobalModelSettingsMigrationPreservesSiteSettings(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "global-settings.sqlite")
	runner, err := OpenApp(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.migrator.Migrate(20260905000007); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO site_settings(key,value,updated_at) VALUES('ai_provider.active','"aliyun_bailian"',CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	var revision int
	var settings string
	if err := db.QueryRow(`SELECT revision,settings FROM global_model_settings WHERE singleton=1`).Scan(&revision, &settings); err != nil {
		t.Fatal(err)
	}
	if revision != 0 || settings != "{}" {
		t.Fatalf("migration changed defaults: revision=%d settings=%s", revision, settings)
	}
	if _, err := db.Exec(`INSERT INTO global_model_settings(created_at,updated_at) VALUES(CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err == nil {
		t.Fatal("singleton constraint accepted a second row")
	}
	if err := runner.migrator.Migrate(20260905000007); err != nil {
		t.Fatal(err)
	}
	if containsTable(tableNames(t, dsn), "global_model_settings") {
		t.Fatal("down migration retained global model settings")
	}
	var active string
	if err := db.QueryRow(`SELECT value FROM site_settings WHERE key='ai_provider.active'`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != `"aliyun_bailian"` {
		t.Fatalf("site settings changed: %s", active)
	}
}
