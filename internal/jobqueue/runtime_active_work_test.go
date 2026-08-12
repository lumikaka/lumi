package jobqueue

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/libtnb/sqlite"
)

func TestHasActiveProjectWorkCoversRuntimeDomains(t *testing.T) {
	db, err := sql.Open("sqlite", "file:active-work-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, statement := range []string{
		`CREATE TABLE task_runs (project_id INTEGER, status TEXT)`,
		`CREATE TABLE asset_maintenance_runs (project_id INTEGER, status TEXT)`,
		`CREATE TABLE production_task_runs (project_id INTEGER, status TEXT)`,
		`CREATE TABLE chat_threads (id INTEGER PRIMARY KEY, project_id INTEGER)`,
		`CREATE TABLE chat_turns (thread_id INTEGER, status TEXT)`,
		`CREATE TABLE workflows (project_id INTEGER, status TEXT)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO chat_threads(id, project_id) VALUES (7, 42), (8, 99)`); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		insert string
	}{
		{name: "story queued", insert: `INSERT INTO task_runs VALUES (42, 'queued')`},
		{name: "story running", insert: `INSERT INTO task_runs VALUES (42, 'running')`},
		{name: "asset maintenance", insert: `INSERT INTO asset_maintenance_runs VALUES (42, 'running')`},
		{name: "production", insert: `INSERT INTO production_task_runs VALUES (42, 'queued')`},
		{name: "chat", insert: `INSERT INTO chat_turns VALUES (7, 'in_progress')`},
		{name: "workflow", insert: `INSERT INTO workflows VALUES (42, 'running')`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearActiveWorkTables(t, db)
			if _, err := db.Exec(test.insert); err != nil {
				t.Fatal(err)
			}
			active, err := hasActiveProjectWork(context.Background(), db, 42)
			if err != nil || !active {
				t.Fatalf("active = %t, error = %v", active, err)
			}
		})
	}
}

func TestHasActiveProjectWorkIgnoresWaitingTerminalAndOtherProjects(t *testing.T) {
	db, err := sql.Open("sqlite", "file:inactive-work-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, statement := range []string{
		`CREATE TABLE task_runs (project_id INTEGER, status TEXT)`,
		`CREATE TABLE asset_maintenance_runs (project_id INTEGER, status TEXT)`,
		`CREATE TABLE production_task_runs (project_id INTEGER, status TEXT)`,
		`CREATE TABLE chat_threads (id INTEGER PRIMARY KEY, project_id INTEGER)`,
		`CREATE TABLE chat_turns (thread_id INTEGER, status TEXT)`,
		`CREATE TABLE workflows (project_id INTEGER, status TEXT)`,
		`INSERT INTO task_runs VALUES (42, 'waiting_for_input'), (42, 'completed'), (99, 'running')`,
		`INSERT INTO asset_maintenance_runs VALUES (42, 'failed')`,
		`INSERT INTO production_task_runs VALUES (42, 'cancelled')`,
		`INSERT INTO chat_threads VALUES (7, 42), (8, 99)`,
		`INSERT INTO chat_turns VALUES (7, 'waiting_for_input'), (8, 'in_progress')`,
		`INSERT INTO workflows VALUES (42, 'completed'), (99, 'running')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	active, err := hasActiveProjectWork(context.Background(), db, 42)
	if err != nil || active {
		t.Fatalf("active = %t, error = %v", active, err)
	}
}

func clearActiveWorkTables(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, table := range []string{"task_runs", "asset_maintenance_runs", "production_task_runs", "chat_turns", "workflows"} {
		if _, err := db.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			t.Fatal(err)
		}
	}
}
