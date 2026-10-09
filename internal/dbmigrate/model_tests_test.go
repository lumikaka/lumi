package dbmigrate

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestModelTestsMigrationConstraintsAndRollback(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "app.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenApp(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if err := runner.migrator.Migrate(20261003000012); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("INSERT INTO model_tests(uuid,created_at,updated_at) VALUES('test',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO model_test_steps(uuid,model_test_id,position,kind) VALUES('step',1,1,'connection')"); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		"INSERT INTO model_test_steps(uuid,model_test_id,position,kind) VALUES('orphan',2,2,'text')",
		"INSERT INTO model_test_steps(uuid,model_test_id,position,kind) VALUES('duplicate',1,1,'connection')",
		"UPDATE model_tests SET status='invalid'",
		"UPDATE model_test_steps SET status='invalid'",
	} {
		if _, err := db.Exec(query); err == nil {
			t.Fatalf("constraint missing: %s", query)
		}
	}
	if err := runner.migrator.Migrate(20261003000012); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"model_tests", "model_test_steps"} {
		if containsTable(tableNames(t, dsn), table) {
			t.Fatalf("down migration retained %s", table)
		}
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
}
