DROP INDEX project_creation_session_references_session_status_index;

ALTER TABLE project_creation_session_references RENAME TO project_creation_session_references_without_plans;

CREATE TABLE project_creation_session_references (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_creation_session_id INTEGER NOT NULL,
    position INTEGER NOT NULL,
    upload_uuid TEXT NOT NULL UNIQUE,
    file_uuid TEXT NOT NULL UNIQUE,
    original_filename TEXT NOT NULL,
    declared_mime_type TEXT NOT NULL,
    declared_byte_size INTEGER NOT NULL,
    reference_role TEXT NOT NULL DEFAULT 'auto',
    title TEXT NOT NULL DEFAULT '',
    instruction TEXT NOT NULL DEFAULT '',
    include_in_yolo INTEGER NOT NULL DEFAULT 1,
    plan_source TEXT NOT NULL DEFAULT 'system_default',
    status TEXT NOT NULL DEFAULT 'pending',
    error_code TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT project_creation_session_references_session_fk FOREIGN KEY (project_creation_session_id) REFERENCES project_creation_sessions(id) ON DELETE CASCADE,
    CONSTRAINT project_creation_session_references_position_unique UNIQUE(project_creation_session_id, position),
    CONSTRAINT project_creation_session_references_position_check CHECK (position BETWEEN 1 AND 16),
    CONSTRAINT project_creation_session_references_size_check CHECK (declared_byte_size BETWEEN 1 AND 33554432),
    CONSTRAINT project_creation_session_references_mime_check CHECK (declared_mime_type IN ('image/png','image/jpeg','image/webp')),
    CONSTRAINT project_creation_session_references_role_check CHECK (reference_role IN ('auto','character','scene','prop','style')),
    CONSTRAINT project_creation_session_references_title_check CHECK (length(title) <= 160),
    CONSTRAINT project_creation_session_references_instruction_check CHECK (length(instruction) <= 2000),
    CONSTRAINT project_creation_session_references_include_check CHECK (include_in_yolo IN (0,1)),
    CONSTRAINT project_creation_session_references_source_check CHECK (plan_source IN ('system_default','agent_proposed','user_confirmed')),
    CONSTRAINT project_creation_session_references_status_check CHECK (status IN ('pending','uploading','ready','failed'))
);

INSERT INTO project_creation_session_references (
    id, uuid, project_creation_session_id, position, upload_uuid, file_uuid,
    original_filename, declared_mime_type, declared_byte_size,
    reference_role, title, instruction, include_in_yolo, plan_source,
    status, error_code, created_at, updated_at
)
SELECT
    id, uuid, project_creation_session_id, position, upload_uuid, file_uuid,
    original_filename, declared_mime_type, declared_byte_size,
    'auto', '', '', 1, 'system_default',
    status, error_code, created_at, updated_at
FROM project_creation_session_references_without_plans;

DROP TABLE project_creation_session_references_without_plans;

CREATE INDEX project_creation_session_references_session_status_index
    ON project_creation_session_references(project_creation_session_id, status, position);
