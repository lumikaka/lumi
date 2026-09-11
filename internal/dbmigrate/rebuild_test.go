package dbmigrate

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRebuildPreservesChildrenAcrossLineEndings(t *testing.T) {
	for _, tc := range []struct {
		name string
		eol  string
	}{
		{name: "LF", eol: "\n"},
		{name: "CRLF", eol: "\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const rebuild = `-- lumi: rebuild-without-foreign-key-actions
CREATE TEMP TABLE parents_backup AS SELECT * FROM parents;
DROP TABLE parents;
CREATE TABLE parents (id INTEGER PRIMARY KEY);
INSERT INTO parents SELECT * FROM parents_backup;
DROP TABLE parents_backup;
`
			migrationFS := fstest.MapFS{
				"000001_create.up.sql": {Data: []byte(`
CREATE TABLE parents (id INTEGER PRIMARY KEY);
CREATE TABLE children (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parents(id) ON DELETE CASCADE);
INSERT INTO parents VALUES (1);
INSERT INTO children VALUES (1, 1);
`)},
				"000002_rebuild.up.sql":   {Data: []byte(strings.ReplaceAll(rebuild, "\n", tc.eol))},
				"000002_rebuild.down.sql": {Data: []byte(strings.ReplaceAll(rebuild, "\n", tc.eol))},
				"000003_delete.up.sql":    {Data: []byte("DELETE FROM parents;")},
			}
			dsn := "file:" + filepath.Join(t.TempDir(), "rebuild.sqlite") + "?_pragma=foreign_keys(1)"
			runner, err := OpenWithFS(dsn, migrationFS, ".")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runner.Close() })
			db, err := sql.Open("sqlite", dsn)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			for _, step := range []struct {
				name string
				run  func() error
			}{
				{name: "up", run: func() error { return runner.migrator.Migrate(2) }},
				{name: "down", run: func() error { return runner.Down(1) }},
			} {
				if err := step.run(); err != nil {
					t.Fatalf("%s: %v", step.name, err)
				}
				var count int
				if err := db.QueryRow("SELECT COUNT(*) FROM children WHERE id=1 AND parent_id=1").Scan(&count); err != nil || count != 1 {
					t.Fatalf("%s rebuild lost child: count=%d err=%v", step.name, count, err)
				}
			}
			// An ordinary migration must still cascade on the migration connection.
			if err := runner.Up(); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM children").Scan(&count); err != nil || count != 0 {
				t.Fatalf("foreign key actions were not restored: count=%d err=%v", count, err)
			}
		})
	}
}
