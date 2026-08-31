DROP INDEX project_creation_reference_files_project_session_index;

ALTER TABLE project_creation_reference_files RENAME TO project_creation_reference_files_without_plans;

CREATE TABLE project_creation_reference_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    creation_session_uuid TEXT NOT NULL,
    reference_uuid TEXT NOT NULL UNIQUE,
    position INTEGER NOT NULL,
    file_id INTEGER NOT NULL UNIQUE,
    reference_role TEXT NOT NULL DEFAULT 'auto',
    title TEXT NOT NULL DEFAULT '',
    instruction TEXT NOT NULL DEFAULT '',
    include_in_yolo INTEGER NOT NULL DEFAULT 1,
    plan_source TEXT NOT NULL DEFAULT 'system_default',
    premise_asset_id INTEGER,
    imported_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT project_creation_reference_files_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT project_creation_reference_files_file_fk FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE RESTRICT,
    CONSTRAINT project_creation_reference_files_premise_asset_fk FOREIGN KEY (premise_asset_id) REFERENCES premise_assets(id) ON DELETE SET NULL,
    CONSTRAINT project_creation_reference_files_position_unique UNIQUE(creation_session_uuid, position),
    CONSTRAINT project_creation_reference_files_position_check CHECK (position BETWEEN 1 AND 16),
    CONSTRAINT project_creation_reference_files_role_check CHECK (reference_role IN ('auto','character','scene','prop','style')),
    CONSTRAINT project_creation_reference_files_title_check CHECK (length(title) <= 160),
    CONSTRAINT project_creation_reference_files_instruction_check CHECK (length(instruction) <= 2000),
    CONSTRAINT project_creation_reference_files_include_check CHECK (include_in_yolo IN (0,1)),
    CONSTRAINT project_creation_reference_files_source_check CHECK (plan_source IN ('system_default','agent_proposed','user_confirmed'))
);

INSERT INTO project_creation_reference_files (
    id, uuid, project_id, creation_session_uuid, reference_uuid, position, file_id,
    reference_role, title, instruction, include_in_yolo, plan_source,
    premise_asset_id, imported_at, created_at, updated_at
)
SELECT
    id, uuid, project_id, creation_session_uuid, reference_uuid, position, file_id,
    'auto', '', '', 1, 'system_default',
    NULL, NULL, created_at, created_at
FROM project_creation_reference_files_without_plans;

DROP TABLE project_creation_reference_files_without_plans;

CREATE INDEX project_creation_reference_files_project_session_index
    ON project_creation_reference_files(project_id, creation_session_uuid, position);

CREATE UNIQUE INDEX project_creation_reference_files_premise_asset_unique
    ON project_creation_reference_files(premise_asset_id)
    WHERE premise_asset_id IS NOT NULL;
