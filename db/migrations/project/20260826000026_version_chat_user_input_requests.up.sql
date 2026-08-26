PRAGMA defer_foreign_keys = ON;

CREATE TABLE chat_user_input_requests_v4 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    thread_id INTEGER NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    run_id INTEGER NOT NULL REFERENCES chat_runs(id) ON DELETE CASCADE,
    turn_id INTEGER NOT NULL REFERENCES chat_turns(id) ON DELETE CASCADE,
    item_id INTEGER NOT NULL UNIQUE REFERENCES chat_items(id) ON DELETE CASCADE,
    tool_call_uuid TEXT NOT NULL,
    schema_version TEXT NOT NULL,
    request_json TEXT NOT NULL,
    response_json TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    answered_at DATETIME,
    resumed_at DATETIME,
    cancelled_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT chat_user_input_v4_tool_call_check CHECK (length(tool_call_uuid) = 36),
    CONSTRAINT chat_user_input_v4_schema_check CHECK (schema_version IN ('legacy_choice_v1', 'codex_questions_v1')),
    CONSTRAINT chat_user_input_v4_request_check CHECK (json_valid(request_json)),
    CONSTRAINT chat_user_input_v4_response_check CHECK (response_json IS NULL OR json_valid(response_json)),
    CONSTRAINT chat_user_input_v4_status_check CHECK (status IN ('pending', 'answered', 'resuming', 'resumed', 'cancelled')),
    UNIQUE(run_id, tool_call_uuid)
);

INSERT INTO chat_user_input_requests_v4(
    id,uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,schema_version,request_json,response_json,
    status,answered_at,resumed_at,cancelled_at,created_at,updated_at
)
SELECT
    id,uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,'legacy_choice_v1',
    json_object('input_type',input_type,'question',question,'options',json(options_json)),response_json,
    status,answered_at,resumed_at,cancelled_at,created_at,updated_at
FROM chat_user_input_requests;

DROP TABLE chat_user_input_requests;
ALTER TABLE chat_user_input_requests_v4 RENAME TO chat_user_input_requests;

CREATE INDEX chat_user_input_thread_status_index
    ON chat_user_input_requests(thread_id, status, created_at, id);
