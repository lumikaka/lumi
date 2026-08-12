package appstore

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"lumi/internal/config"
)

func TestOpenRepairsLegacyRecentProjectTimestampTypes(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "app-data")
	databasePath := filepath.Join(dataDir, "lumi.sqlite")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dsn := config.SQLiteDSN(databasePath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE schema_migrations (version uint64, dirty bool)`,
		`INSERT INTO schema_migrations (version, dirty) VALUES (20260808000001, false)`,
		`CREATE TABLE recent_projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			root_path TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			last_opened_at TEXT NOT NULL
		)`,
		`CREATE INDEX recent_projects_last_opened_at_index ON recent_projects(last_opened_at DESC)`,
		`INSERT INTO recent_projects (
			id, uuid, name, root_path, created_at, updated_at, last_opened_at
		) VALUES (
			2,
			'019fe216-2a1e-748f-8c46-0cc0b2c455c0',
			'Moon',
			'/tmp/moon',
			'2026-08-08 15:55:31.230302+00:00',
			'2026-08-08 15:55:31.230302+00:00',
			'2026-08-08 15:55:31.230302+00:00'
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(dataDir, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	projects, err := store.RecentProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "Moon" {
		t.Fatalf("projects = %+v", projects)
	}
	wantTime := time.Date(2026, 8, 8, 15, 55, 31, 230302000, time.UTC)
	if !projects[0].CreatedAt.Equal(wantTime) || !projects[0].UpdatedAt.Equal(wantTime) || !projects[0].LastOpenedAt.Equal(wantTime) {
		t.Fatalf("migrated timestamps = %v, %v, %v", projects[0].CreatedAt, projects[0].UpdatedAt, projects[0].LastOpenedAt)
	}

	sqlDB, err := store.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"created_at", "updated_at", "last_opened_at"} {
		var columnType string
		if err := sqlDB.QueryRow(`SELECT type FROM pragma_table_info('recent_projects') WHERE name = ?`, column).Scan(&columnType); err != nil {
			t.Fatal(err)
		}
		if columnType != "DATETIME" {
			t.Fatalf("%s type = %q, want DATETIME", column, columnType)
		}
	}
}

func TestOpenMigratesAppStoreAndPersistsRecentProjects(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "app-data")
	store, err := Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 7, 1, 2, 3, 0, time.UTC)
	if err := store.RecordProject(context.Background(), "01989abc-def0-7000-8000-000000000001", "Moon", "/tmp/moon", now); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	projects, err := store.RecentProjects(context.Background())
	if err != nil || len(projects) != 1 || projects[0].Name != "Moon" {
		t.Fatalf("projects = %+v, error = %v", projects, err)
	}
	if err := store.UpdateProjectName(context.Background(), projects[0].UUID, "New Moon", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	renamed, err := store.RecentProject(context.Background(), projects[0].UUID)
	if err != nil || renamed.Name != "New Moon" || !renamed.LastOpenedAt.Equal(now) {
		t.Fatalf("renamed project = %+v, error = %v", renamed, err)
	}
	if info, err := os.Stat(filepath.Join(dataDir, "cache")); err != nil || !info.IsDir() {
		t.Fatalf("cache directory info = %+v, error = %v", info, err)
	}
	if err := store.ForgetProject(context.Background(), projects[0].UUID); err != nil {
		t.Fatal(err)
	}
	if err := store.ForgetProject(context.Background(), projects[0].UUID); !errors.Is(err, ErrRecentProjectNotFound) {
		t.Fatalf("second forget error = %v", err)
	}
}
