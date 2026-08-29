CREATE TABLE project_creation_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    idempotency_key TEXT NOT NULL UNIQUE,
    input_text TEXT NOT NULL,
    status TEXT NOT NULL,
    planned_project_uuid TEXT NOT NULL UNIQUE,
    planned_root_path TEXT NOT NULL DEFAULT '',
    recent_project_id INTEGER,
    thread_uuid TEXT NOT NULL DEFAULT '',
    turn_uuid TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    completed_at DATETIME,
    failed_at DATETIME,
    CONSTRAINT project_creation_sessions_recent_project_fk FOREIGN KEY (recent_project_id) REFERENCES recent_projects(id) ON DELETE SET NULL,
    CONSTRAINT project_creation_sessions_status_check CHECK (status IN ('pending','creating_project','creating_conversation','active','failed','cancelled')),
    CONSTRAINT project_creation_sessions_input_check CHECK (length(trim(input_text)) BETWEEN 1 AND 262144),
    CONSTRAINT project_creation_sessions_idempotency_check CHECK (length(trim(idempotency_key)) BETWEEN 1 AND 200),
    CONSTRAINT project_creation_sessions_attempt_check CHECK (attempt_count >= 0)
);

CREATE INDEX project_creation_sessions_status_updated_index
    ON project_creation_sessions(status, updated_at, id);
