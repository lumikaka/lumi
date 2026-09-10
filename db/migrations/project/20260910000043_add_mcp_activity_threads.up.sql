-- lumi: rebuild-without-foreign-key-actions
-- The migration runner disables FK actions outside the transaction, validates
-- every FK before commit, then restores enforcement on the same connection.
CREATE TEMP TABLE mcp_thread_sequence_backup AS SELECT seq FROM sqlite_sequence WHERE name='chat_threads';
CREATE TABLE chat_threads_new (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 uuid TEXT NOT NULL UNIQUE,
 project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 title TEXT NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 160),
 status TEXT NOT NULL DEFAULT 'idle' CHECK(status IN ('idle','busy','waiting_for_input','completed','failed','cancelled','interrupted')),
 provider_uuid TEXT NOT NULL,
 model TEXT NOT NULL,
 next_turn_sequence INTEGER NOT NULL DEFAULT 1 CHECK(next_turn_sequence>0),
 next_item_sequence INTEGER NOT NULL DEFAULT 1 CHECK(next_item_sequence>0),
 next_event_sequence INTEGER NOT NULL DEFAULT 1 CHECK(next_event_sequence>0),
 archived_at DATETIME,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 model_source TEXT NOT NULL DEFAULT 'legacy_frozen',
 thread_type TEXT NOT NULL DEFAULT 'conversation' CHECK(thread_type IN ('conversation','workflow','mcp')),
 CHECK((thread_type='mcp' AND status='idle' AND provider_uuid='' AND model='') OR (thread_type!='mcp' AND length(provider_uuid)=36 AND length(trim(model)) BETWEEN 1 AND 512))
);
INSERT INTO chat_threads_new (id,uuid,project_id,title,status,thread_type,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,archived_at,created_at,updated_at) SELECT id,uuid,project_id,title,status,thread_type,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,archived_at,created_at,updated_at FROM chat_threads;
DROP TABLE chat_threads;
ALTER TABLE chat_threads_new RENAME TO chat_threads;
UPDATE sqlite_sequence SET seq=max(seq,coalesce((SELECT max(seq) FROM mcp_thread_sequence_backup),0)) WHERE name='chat_threads';
DROP TABLE mcp_thread_sequence_backup;
CREATE INDEX chat_threads_project_updated_index ON chat_threads(project_id,archived_at,updated_at DESC,id DESC);

CREATE TABLE mcp_threads (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 uuid TEXT NOT NULL UNIQUE,
 thread_id INTEGER NOT NULL UNIQUE REFERENCES chat_threads(id) ON DELETE CASCADE,
 grant_uuid TEXT NOT NULL,
 client_name TEXT NOT NULL,
 started_at DATETIME NOT NULL,
 last_call_at DATETIME NOT NULL,
 next_sequence INTEGER NOT NULL DEFAULT 1,
 revision INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX mcp_threads_grant ON mcp_threads(grant_uuid,last_call_at DESC,id DESC);
CREATE TABLE mcp_thread_activities (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 uuid TEXT NOT NULL UNIQUE,
 mcp_thread_id INTEGER NOT NULL REFERENCES mcp_threads(id) ON DELETE CASCADE,
 sequence INTEGER NOT NULL,
 call_uuid TEXT UNIQUE,
 tool_name TEXT NOT NULL,
 action TEXT NOT NULL,
 resource_key TEXT NOT NULL DEFAULT '',
 label TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('executing','pending_confirmation','succeeded','failed','rejected','expired','interrupted')),
 error_message TEXT NOT NULL DEFAULT '',
 admitted_at DATETIME NOT NULL,
 UNIQUE(mcp_thread_id,sequence)
);
