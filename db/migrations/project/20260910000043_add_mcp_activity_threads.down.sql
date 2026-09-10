-- lumi: rebuild-without-foreign-key-actions
-- Refuse a lossy downgrade once MCP history exists.
CREATE TEMP TABLE mcp_down_guard (n INTEGER CHECK(n=0));
INSERT INTO mcp_down_guard SELECT count(*) FROM chat_threads WHERE thread_type='mcp';
DROP TABLE mcp_down_guard;
DROP TABLE mcp_thread_activities;
DROP TABLE mcp_threads;
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
 thread_type TEXT NOT NULL DEFAULT 'conversation' CHECK(thread_type IN ('conversation','workflow')),
 CHECK(length(provider_uuid)=36 AND length(trim(model)) BETWEEN 1 AND 512)
);
INSERT INTO chat_threads_new (id,uuid,project_id,title,status,thread_type,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,archived_at,created_at,updated_at) SELECT id,uuid,project_id,title,status,thread_type,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,archived_at,created_at,updated_at FROM chat_threads;
DROP TABLE chat_threads;
ALTER TABLE chat_threads_new RENAME TO chat_threads;
UPDATE sqlite_sequence SET seq=max(seq,coalesce((SELECT max(seq) FROM mcp_thread_sequence_backup),0)) WHERE name='chat_threads';
DROP TABLE mcp_thread_sequence_backup;
CREATE INDEX chat_threads_project_updated_index ON chat_threads(project_id,archived_at,updated_at DESC,id DESC);
