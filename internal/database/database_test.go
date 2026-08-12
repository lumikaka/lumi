package database

import (
	"path/filepath"
	"testing"
)

func TestOpenConfiguresSQLite(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "nested", "lumi.sqlite3")
	dsn := "file:" + databasePath + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
	db, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	var foreignKeys int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}

	var journalMode string
	if err := db.Raw("PRAGMA journal_mode").Scan(&journalMode).Error; err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	var synchronous int
	if err := db.Raw("PRAGMA synchronous").Scan(&synchronous).Error; err != nil {
		t.Fatal(err)
	}
	if synchronous != 1 {
		t.Fatalf("synchronous = %d, want NORMAL (1)", synchronous)
	}

	var busyTimeout int
	if err := db.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busyTimeout)
	}
	if sqlDB.Stats().MaxOpenConnections != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", sqlDB.Stats().MaxOpenConnections)
	}
}

func TestDatabaseFilePath(t *testing.T) {
	for _, scenario := range []struct {
		dsn  string
		path string
		ok   bool
	}{
		{dsn: "file:db/lumi.sqlite3?_pragma=foreign_keys(1)", path: "db/lumi.sqlite3", ok: true},
		{dsn: "file:/tmp/lumi%20test.sqlite3", path: "/tmp/lumi test.sqlite3", ok: true},
		{dsn: ":memory:", ok: false},
	} {
		path, ok, err := databaseFilePath(scenario.dsn)
		if err != nil || path != scenario.path || ok != scenario.ok {
			t.Fatalf("databaseFilePath(%q) = %q, %v, %v", scenario.dsn, path, ok, err)
		}
	}
}
