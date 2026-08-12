package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOnlineBackupAndRestoreIncludesWALContent(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "source.sqlite")
	dsn := "file:" + sourcePath + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE notes (value TEXT NOT NULL); INSERT INTO notes VALUES ('before');"); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(directory, "backups", "snapshot.sqlite")
	if err := OnlineBackup(context.Background(), dsn, backupPath); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DELETE FROM notes; INSERT INTO notes VALUES ('after');"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := OnlineRestore(context.Background(), dsn, backupPath); err != nil {
		t.Fatal(err)
	}
	restored, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var value string
	if err := restored.QueryRow("SELECT value FROM notes").Scan(&value); err != nil || value != "before" {
		t.Fatalf("restored value = %q, error = %v", value, err)
	}
}
