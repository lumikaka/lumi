DROP INDEX project_creation_reference_files_premise_asset_unique;
DROP INDEX project_creation_reference_files_project_session_index;

ALTER TABLE project_creation_reference_files RENAME TO project_creation_reference_files_with_plans;

CREATE TABLE project_creation_reference_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    creation_session_uuid TEXT NOT NULL,
    reference_uuid TEXT NOT NULL UNIQUE,
    position INTEGER NOT NULL,
    file_id INTEGER NOT NULL UNIQUE,
    created_at DATETIME NOT NULL,
    CONSTRAINT project_creation_reference_files_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT project_creation_reference_files_file_fk FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE RESTRICT,
    CONSTRAINT project_creation_reference_files_position_unique UNIQUE(creation_session_uuid, position),
    CONSTRAINT project_creation_reference_files_position_check CHECK (position BETWEEN 1 AND 16)
);

INSERT INTO project_creation_reference_files (
    id, uuid, project_id, creation_session_uuid, reference_uuid, position, file_id, created_at
)
SELECT
    id, uuid, project_id, creation_session_uuid, reference_uuid, position, file_id, created_at
FROM project_creation_reference_files_with_plans;

DROP TABLE project_creation_reference_files_with_plans;

CREATE INDEX project_creation_reference_files_project_session_index
    ON project_creation_reference_files(project_id, creation_session_uuid, position);
