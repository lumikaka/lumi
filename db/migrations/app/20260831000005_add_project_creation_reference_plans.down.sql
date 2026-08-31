DROP INDEX project_creation_session_references_session_status_index;

ALTER TABLE project_creation_session_references RENAME TO project_creation_session_references_with_plans;

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
    status TEXT NOT NULL DEFAULT 'pending',
    error_code TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT project_creation_session_references_session_fk FOREIGN KEY (project_creation_session_id) REFERENCES project_creation_sessions(id) ON DELETE CASCADE,
    CONSTRAINT project_creation_session_references_position_unique UNIQUE(project_creation_session_id, position),
    CONSTRAINT project_creation_session_references_position_check CHECK (position BETWEEN 1 AND 16),
    CONSTRAINT project_creation_session_references_size_check CHECK (declared_byte_size BETWEEN 1 AND 33554432),
    CONSTRAINT project_creation_session_references_mime_check CHECK (declared_mime_type IN ('image/png','image/jpeg','image/webp')),
    CONSTRAINT project_creation_session_references_status_check CHECK (status IN ('pending','uploading','ready','failed'))
);

INSERT INTO project_creation_session_references (
    id, uuid, project_creation_session_id, position, upload_uuid, file_uuid,
    original_filename, declared_mime_type, declared_byte_size,
    status, error_code, created_at, updated_at
)
SELECT
    id, uuid, project_creation_session_id, position, upload_uuid, file_uuid,
    original_filename, declared_mime_type, declared_byte_size,
    status, error_code, created_at, updated_at
FROM project_creation_session_references_with_plans;

DROP TABLE project_creation_session_references_with_plans;

CREATE INDEX project_creation_session_references_session_status_index
    ON project_creation_session_references(project_creation_session_id, status, position);
