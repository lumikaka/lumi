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

	"github.com/google/uuid"
)

func appStoreTestUUID(t *testing.T) string {
	t.Helper()
	value, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return value.String()
}

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

func TestProjectCreationSessionPersistsOrderedReferenceManifestAtomically(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "app-data")
	store, err := Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	session := ProjectCreationSession{
		UUID: appStoreTestUUID(t), IdempotencyKey: "home-reference-manifest-0001", InputText: "Create from two images",
		Status: "pending", PlannedProjectUUID: appStoreTestUUID(t), CreatedAt: now, UpdatedAt: now,
	}
	references := []ProjectCreationReference{
		{UUID: appStoreTestUUID(t), Position: 1, UploadUUID: appStoreTestUUID(t), FileUUID: appStoreTestUUID(t), OriginalFilename: "first.png", DeclaredMIMEType: "image/png", DeclaredByteSize: 123, Status: "pending", CreatedAt: now, UpdatedAt: now},
		{UUID: appStoreTestUUID(t), Position: 2, UploadUUID: appStoreTestUUID(t), FileUUID: appStoreTestUUID(t), OriginalFilename: "second.webp", DeclaredMIMEType: "image/webp", DeclaredByteSize: 456, Status: "pending", CreatedAt: now, UpdatedAt: now},
	}

	created, wasCreated, err := store.CreateOrGetProjectCreationSession(ctx, session, references)
	if err != nil || !wasCreated || created.ID == 0 {
		t.Fatalf("created=%+v was_created=%v err=%v", created, wasCreated, err)
	}
	stored, err := store.ProjectCreationReferences(ctx, created.ID)
	if err != nil || len(stored) != 2 || stored[0].UUID != references[0].UUID || stored[1].UUID != references[1].UUID || stored[0].Position != 1 || stored[1].Position != 2 {
		t.Fatalf("stored references=%+v err=%v", stored, err)
	}
	replayed, wasCreated, err := store.CreateOrGetProjectCreationSession(ctx, ProjectCreationSession{IdempotencyKey: session.IdempotencyKey}, nil)
	if err != nil || wasCreated || replayed.UUID != session.UUID {
		t.Fatalf("replayed=%+v was_created=%v err=%v", replayed, wasCreated, err)
	}

	foundSession, found, err := store.ProjectCreationReference(ctx, session.UUID, references[1].UUID)
	if err != nil || foundSession.ID != created.ID || found.Position != 2 {
		t.Fatalf("found session=%+v reference=%+v err=%v", foundSession, found, err)
	}
	updated, err := store.UpdateProjectCreationReference(ctx, found.ID, map[string]any{"status": "ready", "updated_at": now.Add(time.Minute)})
	if err != nil || updated.Status != "ready" || updated.FileUUID != references[1].FileUUID {
		t.Fatalf("updated reference=%+v err=%v", updated, err)
	}

	if err := store.DB().WithContext(ctx).Delete(&created).Error; err != nil {
		t.Fatal(err)
	}
	var remaining int64
	if err := store.DB().WithContext(ctx).Model(&ProjectCreationReference{}).Where("project_creation_session_id = ?", created.ID).Count(&remaining).Error; err != nil || remaining != 0 {
		t.Fatalf("remaining references=%d err=%v", remaining, err)
	}
}
