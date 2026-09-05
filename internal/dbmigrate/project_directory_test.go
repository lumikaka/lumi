package dbmigrate

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestDirectoryNamingMigrationDoesNotOptInHistoricalProjects(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "app.sqlite")
	runner, err := OpenApp(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.migrator.Migrate(20260831000005); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO recent_projects (uuid,name,root_path,created_at,updated_at,last_opened_at) VALUES ('019c0000-0000-7000-8000-000000000001','勇敢的小火车','/projects/Lumi-Draft-16',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	var automatic bool
	var pending, root string
	if err := db.QueryRow(`SELECT auto_name_directory,pending_directory_name,root_path FROM recent_projects`).Scan(&automatic, &pending, &root); err != nil {
		t.Fatal(err)
	}
	if automatic || pending != "" || root != "/projects/Lumi-Draft-16" {
		t.Fatalf("historical project changed: auto=%v pending=%q root=%q", automatic, pending, root)
	}
	if err := runner.migrator.Migrate(20260831000005); err != nil {
		t.Fatal(err)
	}
	if tableHasColumn(t, db, "recent_projects", "pending_directory_name") || tableHasColumn(t, db, "recent_projects", "auto_name_directory") {
		t.Fatal("directory naming fields survived rollback")
	}
	if err := db.QueryRow(`SELECT root_path FROM recent_projects`).Scan(&root); err != nil || root != "/projects/Lumi-Draft-16" {
		t.Fatalf("rollback root=%q err=%v", root, err)
	}
}
