package dbmigrate

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestCindyMigrationPreservesCodexDataSequenceAndGuardsDowngrade(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "cindy.sqlite") + "?_pragma=foreign_keys(1)"
	r, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.migrator.Migrate(20261101000051); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(query string) {
		t.Helper()
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'01970000-0000-7000-8000-000000000001','test',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	exec(`INSERT INTO chat_threads(id,uuid,project_id,title,thread_type,provider_uuid,model,model_source,created_at,updated_at) VALUES(500,'01970000-0000-7000-8000-000000000002',1,'Codex original','codex_session','','','',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	exec(`INSERT INTO codex_sessions(id,thread_id,session_uuid,rollout_path,prefix_hash,read_offset,imported_at) VALUES(1,500,'01970000-0000-7000-8000-000000000003','/synthetic/rollout.jsonl','prefix',12,CURRENT_TIMESTAMP)`)
	exec(`INSERT INTO codex_session_items(uuid,session_id,source_offset,kind,text,recorded_at) VALUES('01970000-0000-7000-8000-000000000004',1,0,'user','preserve Codex body',CURRENT_TIMESTAMP)`)
	// A deleted high ID must never be reused by the table rebuild.
	exec(`INSERT INTO chat_threads(id,uuid,project_id,title,thread_type,provider_uuid,model,created_at,updated_at) VALUES(900,'01970000-0000-7000-8000-000000000005',1,'temporary','mcp','','',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	exec(`DELETE FROM chat_threads WHERE id=900`)
	if err := r.Up(); err != nil {
		t.Fatal(err)
	}
	if err := r.migrator.Migrate(20261101000051); err != nil {
		t.Fatal("empty Cindy downgrade:", err)
	}
	if err := r.Up(); err != nil {
		t.Fatal(err)
	}
	var text, path string
	if err := db.QueryRow(`SELECT i.text,s.rollout_path FROM codex_session_items i JOIN codex_sessions s ON s.id=i.session_id JOIN chat_threads t ON t.id=s.thread_id WHERE t.id=500`).Scan(&text, &path); err != nil || text != "preserve Codex body" || path != "/synthetic/rollout.jsonl" {
		t.Fatalf("Codex changed: %q %q %v", text, path, err)
	}
	exec(`INSERT INTO chat_threads(uuid,project_id,title,thread_type,provider_uuid,model,model_source,created_at,updated_at) VALUES('01970000-0000-7000-8000-000000000006',1,'Cindy','cindy_session','','','',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	var id int64
	if err := db.QueryRow(`SELECT id FROM chat_threads WHERE thread_type='cindy_session'`).Scan(&id); err != nil || id <= 900 {
		t.Fatalf("sequence=%d %v", id, err)
	}
	if _, err := db.Exec(`UPDATE chat_threads SET status='busy' WHERE thread_type='cindy_session'`); err == nil {
		t.Fatal("imported running status accepted")
	}
	if _, err := db.Exec(`UPDATE chat_threads SET thread_type='conversation' WHERE thread_type='cindy_session'`); err == nil {
		t.Fatal("model-less conversation accepted")
	}
	if err := r.migrator.Migrate(20261101000051); err == nil {
		t.Fatal("lossy Cindy downgrade accepted")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM chat_threads`).Scan(&count); err != nil || count != 2 {
		t.Fatal("failed downgrade changed threads")
	}
	if err := db.QueryRow(`SELECT count(*) FROM pragma_foreign_key_check`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("foreign keys=%d %v", count, err)
	}
}
