package dbmigrate

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMCPMigrationPreservesThreadSequenceAndModelConstraints(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "mcp.sqlite") + "?_pragma=foreign_keys(1)"
	r, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err = r.migrator.Migrate(20260909000042); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'01970000-0000-7000-8000-000000000001','test',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	exec(`INSERT INTO chat_threads(id,uuid,project_id,title,provider_uuid,model,created_at,updated_at) VALUES(500,'01970000-0000-7000-8000-000000000002',1,'test','01970000-0000-7000-8000-000000000003','model',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	exec(`DELETE FROM chat_threads WHERE id=500`)
	if err = r.Up(); err != nil {
		t.Fatal(err)
	}
	q := `INSERT INTO chat_threads(uuid,project_id,title,thread_type,provider_uuid,model,created_at,updated_at) VALUES('01970000-0000-7000-8000-000000000004',1,'MCP','mcp','','',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`
	exec(q)
	var id int64
	if err = db.QueryRow("SELECT id FROM chat_threads").Scan(&id); err != nil || id <= 500 {
		t.Fatalf("sequence reused: %d %v", id, err)
	}
	if _, err = db.Exec(`UPDATE chat_threads SET thread_type='conversation'`); err == nil {
		t.Fatal("model-less conversation accepted")
	}
	if _, err = db.Exec(`UPDATE chat_threads SET status='busy'`); err == nil {
		t.Fatal("MCP running status accepted")
	}
	if err = r.migrator.Migrate(20260909000042); err == nil {
		t.Fatal("lossy downgrade accepted")
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM chat_threads").Scan(&count); err != nil || count != 1 {
		t.Fatal("failed downgrade changed data")
	}
}
