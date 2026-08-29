DROP TABLE project_creation_session_references;

DROP INDEX project_creation_sessions_status_updated_index;

ALTER TABLE project_creation_sessions RENAME TO project_creation_sessions_with_references;

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

INSERT INTO project_creation_sessions (
    id, uuid, idempotency_key, input_text, status, planned_project_uuid,
    planned_root_path, recent_project_id, thread_uuid, turn_uuid, error_code,
    error_message, attempt_count, created_at, updated_at, completed_at, failed_at
)
SELECT
    id, uuid, idempotency_key, input_text,
    CASE WHEN status = 'awaiting_references' THEN 'creating_project' ELSE status END,
    planned_project_uuid, planned_root_path, recent_project_id, thread_uuid,
    turn_uuid, error_code, error_message, attempt_count, created_at, updated_at,
    completed_at, failed_at
FROM project_creation_sessions_with_references;

DROP TABLE project_creation_sessions_with_references;

CREATE INDEX project_creation_sessions_status_updated_index
    ON project_creation_sessions(status, updated_at, id);
