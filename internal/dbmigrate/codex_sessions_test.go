package dbmigrate

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestCodexMigrationPreservesChatGraphAndRejectsLossyDowngrade(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "codex.sqlite") + "?_pragma=foreign_keys(1)"
	r, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.migrator.Migrate(20261029000049); err != nil {
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
	exec(`INSERT INTO chat_threads(id,uuid,project_id,title,provider_uuid,model,created_at,updated_at) VALUES(500,'01970000-0000-7000-8000-000000000002',1,'existing','01970000-0000-7000-8000-000000000003','model',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	exec(`INSERT INTO chat_turns(id,uuid,thread_id,queue_sequence,input_text,created_at,updated_at) VALUES(1,'01970000-0000-7000-8000-000000000004',500,1,'preserve me',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	exec(`INSERT INTO chat_runs(id,uuid,thread_id,turn_id,trigger_type,provider_uuid,model,created_at,updated_at) VALUES(1,'01970000-0000-7000-8000-000000000005',500,1,'prompt','01970000-0000-7000-8000-000000000003','model',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	exec(`INSERT INTO chat_items(uuid,thread_id,turn_id,run_id,sequence,item_type,role,content,created_at) VALUES('01970000-0000-7000-8000-000000000006',500,1,1,1,'user_message','user','preserve me',CURRENT_TIMESTAMP)`)
	if err := r.Up(); err != nil {
		t.Fatal(err)
	}
	// Empty imports allow a clean downgrade and another upgrade.
	if err := r.migrator.Migrate(20261029000049); err != nil {
		t.Fatal(err)
	}
	if err := r.Up(); err != nil {
		t.Fatal(err)
	}
	var content string
	if err := db.QueryRow(`SELECT i.content FROM chat_items i JOIN chat_runs r ON r.id=i.run_id JOIN chat_turns t ON t.id=r.turn_id JOIN chat_threads th ON th.id=t.thread_id WHERE th.id=500`).Scan(&content); err != nil || content != "preserve me" {
		t.Fatalf("chat graph changed: %s %v", content, err)
	}
	exec(`INSERT INTO chat_threads(uuid,project_id,title,thread_type,provider_uuid,model,model_source,created_at,updated_at) VALUES('01970000-0000-7000-8000-000000000007',1,'Imported','codex_session','','','',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	var id int64
	if err := db.QueryRow(`SELECT id FROM chat_threads WHERE thread_type='codex_session'`).Scan(&id); err != nil || id <= 500 {
		t.Fatalf("sequence=%d %v", id, err)
	}
	if _, err := db.Exec(`UPDATE chat_threads SET status='busy' WHERE thread_type='codex_session'`); err == nil {
		t.Fatal("imported run state accepted")
	}
	if _, err := db.Exec(`UPDATE chat_threads SET thread_type='conversation' WHERE thread_type='codex_session'`); err == nil {
		t.Fatal("model-less conversation accepted")
	}
	if err := r.migrator.Migrate(20261029000049); err == nil {
		t.Fatal("lossy downgrade accepted")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM chat_threads`).Scan(&count); err != nil || count != 2 {
		t.Fatal("failed downgrade changed threads")
	}
	if err := db.QueryRow(`SELECT count(*) FROM pragma_foreign_key_check`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid foreign keys: %d %v", count, err)
	}
}
