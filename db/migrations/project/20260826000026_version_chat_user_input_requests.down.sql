PRAGMA defer_foreign_keys = ON;

-- The historical table cannot represent a multi-question request. Refuse the
-- downgrade before changing schema when any v4 row exists.
CREATE TEMP TABLE chat_user_input_down_guard (
    ok INTEGER NOT NULL CHECK (ok=1)
);
INSERT INTO chat_user_input_down_guard(ok)
SELECT 0 WHERE EXISTS (
    SELECT 1 FROM chat_user_input_requests WHERE schema_version<>'legacy_choice_v1'
);
DROP TABLE chat_user_input_down_guard;

CREATE TABLE chat_user_input_requests_legacy (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    thread_id INTEGER NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    run_id INTEGER NOT NULL REFERENCES chat_runs(id) ON DELETE CASCADE,
    turn_id INTEGER NOT NULL REFERENCES chat_turns(id) ON DELETE CASCADE,
    item_id INTEGER NOT NULL UNIQUE REFERENCES chat_items(id) ON DELETE CASCADE,
    tool_call_uuid TEXT NOT NULL,
    input_type TEXT NOT NULL,
    question TEXT NOT NULL,
    options_json TEXT NOT NULL,
    response_json TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    answered_at DATETIME,
    resumed_at DATETIME,
    cancelled_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT chat_user_input_tool_call_check CHECK (length(tool_call_uuid) = 36),
    CONSTRAINT chat_user_input_type_check CHECK (input_type IN ('single_choice', 'multiple_choice')),
    CONSTRAINT chat_user_input_question_check CHECK (length(trim(question)) BETWEEN 1 AND 4000),
    CONSTRAINT chat_user_input_options_check CHECK (json_valid(options_json)),
    CONSTRAINT chat_user_input_response_check CHECK (response_json IS NULL OR json_valid(response_json)),
    CONSTRAINT chat_user_input_status_check CHECK (status IN ('pending', 'answered', 'resuming', 'resumed', 'cancelled')),
    UNIQUE(run_id, tool_call_uuid)
);

INSERT INTO chat_user_input_requests_legacy(
    id,uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,input_type,question,options_json,response_json,
    status,answered_at,resumed_at,cancelled_at,created_at,updated_at
)
SELECT
    id,uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,
    json_extract(request_json,'$.input_type'),json_extract(request_json,'$.question'),json_extract(request_json,'$.options'),response_json,
    status,answered_at,resumed_at,cancelled_at,created_at,updated_at
FROM chat_user_input_requests;

DROP TABLE chat_user_input_requests;
ALTER TABLE chat_user_input_requests_legacy RENAME TO chat_user_input_requests;

CREATE INDEX chat_user_input_thread_status_index
    ON chat_user_input_requests(thread_id, status, created_at, id);
