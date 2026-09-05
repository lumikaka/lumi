package dbmigrate

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestCostMigrationPreservesLegacyUsageAndRoundTrips(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "costs.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.migrator.Migrate(20260904000037); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, q := range []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000001','Costs',1,37,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO task_runs(id,uuid,project_id,kind,resource_uuid,input_version,input_snapshot,status,idempotency_key,provider_uuid,model,progress,attempt,max_attempts,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000002',1,'story_chapter_generation','01990000-0000-7000-8000-000000000001',1,'{}','running','cost-migration','01990000-0000-7000-8000-000000000003','model',0,1,3,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO llm_logs(uuid,project_id,task_run_id,source_type,scenario,request_type,attempt,provider_uuid,model,status,input_tokens,cached_input_tokens,output_tokens,created_at) VALUES('01990000-0000-7000-8000-000000000004',1,1,'story_generation','story_chapter_generation','text',1,'01990000-0000-7000-8000-000000000003','model','completed',123,0,45,CURRENT_TIMESTAMP)`,
		`INSERT INTO llm_logs(uuid,project_id,task_run_id,source_type,scenario,request_type,attempt,provider_uuid,model,status,created_at) VALUES('01990000-0000-7000-8000-000000000005',1,1,'story_generation','story_chapter_generation','text',2,'01990000-0000-7000-8000-000000000003','model','pending',CURRENT_TIMESTAMP)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	var status, reason string
	var amount sql.NullInt64
	var usage sql.NullString
	if err := db.QueryRow(`SELECT cost_status,cost_reason,cost_nanos,billing_usage FROM llm_logs WHERE attempt=1`).Scan(&status, &reason, &amount, &usage); err != nil {
		t.Fatal(err)
	}
	if status != "unpriced" || reason != "legacy_usage" || amount.Valid || usage.Valid {
		t.Fatalf("legacy cost guessed: %s %s %+v %+v", status, reason, amount, usage)
	}
	if err := db.QueryRow(`SELECT cost_status FROM llm_logs WHERE attempt=2`).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("pending cost %s %v", status, err)
	}
	for _, q := range []string{`UPDATE llm_logs SET cost_nanos=-1`, `UPDATE llm_logs SET billing_usage='broken'`, `UPDATE llm_logs SET cost_status='billed'`} {
		if _, err := db.Exec(q); err == nil {
			t.Fatalf("accepted invalid estimate: %s", q)
		}
	}
	if err := runner.migrator.Migrate(20260904000037); err != nil {
		t.Fatal(err)
	}
	if tableHasColumn(t, db, "llm_logs", "cost_nanos") || containsTable(tableNames(t, dsn), "llm_cost_backfills") {
		t.Fatal("cost schema survived rollback")
	}
	var input, cache, output int
	if err := db.QueryRow(`SELECT input_tokens,cached_input_tokens,output_tokens FROM llm_logs WHERE attempt=1`).Scan(&input, &cache, &output); err != nil || input != 123 || cache != 0 || output != 45 {
		t.Fatalf("legacy usage changed: %d %d %d %v", input, cache, output, err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
}
