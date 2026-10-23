package dbmigrate

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestWebsiteMigrationPreservesTaskChildrenAndGuardsLossyDown(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "website.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if err = runner.migrator.Migrate(20260921000045); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'01900000-0000-7000-8000-000000000201','Migration',1,45,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO production_task_runs(id,uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,model_source,stage,execution_uuid,created_at,updated_at) VALUES(1,'01900000-0000-7000-8000-000000000202',1,'comic_export','01900000-0000-7000-8000-000000000201','{}','completed','migration','custom','rendering','01900000-0000-7000-8000-000000000203',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO production_task_events(uuid,production_task_run_id,sequence,event_type,payload,created_at) VALUES('01900000-0000-7000-8000-000000000204',1,1,'task_completed','{}',CURRENT_TIMESTAMP)`,
	} {
		if _, err = db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for _, migrate := range []func() error{runner.Up, func() error { return runner.Down(1) }, runner.Up} {
		if err = migrate(); err != nil {
			t.Fatal(err)
		}
		var count int
		if err = db.QueryRow(`SELECT COUNT(*) FROM production_task_events e JOIN production_task_runs t ON t.id=e.production_task_run_id WHERE t.model_source='custom' AND t.stage='rendering' AND t.execution_uuid='01900000-0000-7000-8000-000000000203'`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("children=%d %v", count, err)
		}
	}
	if _, err = db.Exec(`INSERT INTO website_generations(uuid,project_id,task_id,snapshot_json,snapshot_hash,created_at) VALUES('01900000-0000-7000-8000-000000000205',1,1,'{}','hash',CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if err = runner.Down(1); err == nil {
		t.Fatal("lossy down accepted")
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM website_generations`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("guard lost website history: %d %v", count, err)
	}
}
