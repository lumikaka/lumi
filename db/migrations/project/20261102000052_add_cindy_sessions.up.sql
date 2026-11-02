-- lumi: rebuild-without-foreign-key-actions
-- The migration runner disables FK actions outside the transaction, validates
-- every FK before commit, then restores enforcement on the same connection.
CREATE TEMP TABLE cindy_thread_sequence_backup AS SELECT seq FROM sqlite_sequence WHERE name='chat_threads';
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
 thread_type TEXT NOT NULL DEFAULT 'conversation' CHECK(thread_type IN ('conversation','workflow','mcp','codex_session','cindy_session')),
 CHECK((thread_type IN ('mcp','codex_session','cindy_session') AND status='idle' AND provider_uuid='' AND model='') OR (thread_type NOT IN ('mcp','codex_session','cindy_session') AND length(provider_uuid)=36 AND length(trim(model)) BETWEEN 1 AND 512))
);
INSERT INTO chat_threads_new (id,uuid,project_id,title,status,thread_type,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,archived_at,created_at,updated_at) SELECT id,uuid,project_id,title,status,thread_type,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,archived_at,created_at,updated_at FROM chat_threads;
DROP TABLE chat_threads;
ALTER TABLE chat_threads_new RENAME TO chat_threads;
UPDATE sqlite_sequence SET seq=max(seq,coalesce((SELECT max(seq) FROM cindy_thread_sequence_backup),0)) WHERE name='chat_threads';
DROP TABLE cindy_thread_sequence_backup;
CREATE INDEX chat_threads_project_updated_index ON chat_threads(project_id,archived_at,updated_at DESC,id DESC);

CREATE TABLE cindy_sessions (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 thread_id INTEGER NOT NULL UNIQUE REFERENCES chat_threads(id) ON DELETE CASCADE,
 source_session_id TEXT NOT NULL UNIQUE,
 captured_at DATETIME NOT NULL,
 cutoff_at DATETIME NOT NULL,
 imported_at DATETIME NOT NULL,
 warning_codes TEXT NOT NULL,
 omitted_messages INTEGER NOT NULL CHECK(omitted_messages>=0)
);
CREATE TABLE cindy_session_items (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 uuid TEXT NOT NULL UNIQUE,
 session_id INTEGER NOT NULL REFERENCES cindy_sessions(id) ON DELETE CASCADE,
 source_message_id TEXT NOT NULL,
 source_text_sha256 TEXT NOT NULL CHECK(length(source_text_sha256)=64),
 kind TEXT NOT NULL CHECK(kind IN ('user','assistant')),
 text TEXT NOT NULL,
 recorded_at DATETIME NOT NULL,
 sort_at TEXT NOT NULL,
 UNIQUE(session_id,source_message_id)
);
CREATE INDEX cindy_session_items_order_index ON cindy_session_items(session_id,sort_at,source_message_id);
CREATE TABLE cindy_session_imports (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 uuid TEXT NOT NULL UNIQUE,
 session_id INTEGER NOT NULL REFERENCES cindy_sessions(id) ON DELETE CASCADE,
 export_sha256 TEXT NOT NULL CHECK(length(export_sha256)=64),
 captured_at DATETIME NOT NULL,
 cutoff_at DATETIME NOT NULL,
 imported_at DATETIME NOT NULL,
 imported_count INTEGER NOT NULL CHECK(imported_count>=0),
 omitted_messages INTEGER NOT NULL CHECK(omitted_messages>=0),
 duplicate_messages INTEGER NOT NULL CHECK(duplicate_messages>=0),
 warning_codes TEXT NOT NULL,
 UNIQUE(session_id,export_sha256)
);
