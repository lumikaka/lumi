PRAGMA defer_foreign_keys = ON;

DROP INDEX IF EXISTS chat_threads_project_scope_updated_index;
ALTER TABLE chat_threads DROP COLUMN subject_uuid;
ALTER TABLE chat_threads DROP COLUMN scene;
ALTER TABLE chat_threads DROP COLUMN scope;

DROP INDEX IF EXISTS premise_sources_project_ignored_index;
DROP TRIGGER premise_sources_append_only;
ALTER TABLE premise_sources DROP COLUMN revision;
ALTER TABLE premise_sources DROP COLUMN ignored_at;
CREATE TRIGGER premise_sources_append_only
BEFORE UPDATE ON premise_sources
BEGIN
    SELECT RAISE(ABORT, 'premise_sources are append-only');
END;

DROP TRIGGER project_prompt_versions_append_only;
CREATE TABLE project_prompt_versions_previous (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    actor_id INTEGER NOT NULL,
    restored_from_version_id INTEGER,
    prompt_group TEXT NOT NULL,
    prompt_key TEXT NOT NULL,
    version_no INTEGER NOT NULL,
    prompt TEXT NOT NULL,
    prompt_hash TEXT NOT NULL,
    source_type TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT project_prompt_versions_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT project_prompt_versions_actor_fk FOREIGN KEY (actor_id) REFERENCES actors(id),
    CONSTRAINT project_prompt_versions_restore_fk FOREIGN KEY (restored_from_version_id) REFERENCES project_prompt_versions_previous(id),
    CONSTRAINT project_prompt_versions_group_check CHECK (prompt_group IN ('story', 'chapter')),
    CONSTRAINT project_prompt_versions_key_check CHECK (length(trim(prompt_key)) BETWEEN 1 AND 120),
    CONSTRAINT project_prompt_versions_version_check CHECK (version_no > 0),
    CONSTRAINT project_prompt_versions_prompt_check CHECK (length(trim(prompt)) > 0),
    CONSTRAINT project_prompt_versions_hash_check CHECK (length(prompt_hash) = 64),
    CONSTRAINT project_prompt_versions_source_check CHECK (source_type IN ('manual_edit', 'version_restore')),
    UNIQUE(project_id, prompt_group, prompt_key, version_no)
);
INSERT INTO project_prompt_versions_previous
SELECT * FROM project_prompt_versions WHERE prompt_group IN ('story', 'chapter');
DROP TABLE project_prompt_versions;
ALTER TABLE project_prompt_versions_previous RENAME TO project_prompt_versions;
CREATE INDEX project_prompt_versions_history_index
    ON project_prompt_versions(project_id, prompt_group, prompt_key, version_no DESC);
CREATE TRIGGER project_prompt_versions_append_only
BEFORE UPDATE ON project_prompt_versions
BEGIN
    SELECT RAISE(ABORT, 'project_prompt_versions are append-only');
END;

DROP TRIGGER production_task_events_append_only_delete;

CREATE TABLE production_task_events_backup AS
SELECT id, uuid, production_task_run_id, sequence, event_type, payload, created_at
FROM production_task_events;
DROP TABLE production_task_events;

CREATE TABLE production_task_runs_previous (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    river_job_id INTEGER UNIQUE,
    kind TEXT NOT NULL,
    resource_uuid TEXT NOT NULL,
    input_snapshot TEXT NOT NULL,
    status TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    provider_uuid TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    progress INTEGER NOT NULL DEFAULT 0,
    attempt INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 1,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    cancel_requested_at DATETIME,
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT production_tasks_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT production_tasks_kind_check CHECK (kind IN ('premise_setting_generation', 'premise_asset_breakdown', 'comic_image_generation', 'comic_export')),
    CONSTRAINT production_tasks_snapshot_check CHECK (json_valid(input_snapshot)),
    CONSTRAINT production_tasks_status_check CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled', 'interrupted')),
    CONSTRAINT production_tasks_progress_check CHECK (progress BETWEEN 0 AND 100),
    UNIQUE(project_id, kind, idempotency_key)
);
INSERT INTO production_task_runs_previous
SELECT * FROM production_task_runs WHERE kind <> 'premise_asset_generation';
DROP TABLE production_task_runs;
ALTER TABLE production_task_runs_previous RENAME TO production_task_runs;
CREATE UNIQUE INDEX production_tasks_active_resource_unique
    ON production_task_runs(project_id, kind, resource_uuid)
    WHERE status IN ('queued', 'running');
CREATE INDEX production_tasks_project_status_index
    ON production_task_runs(project_id, status, created_at DESC, id DESC);

CREATE TABLE production_task_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    production_task_run_id INTEGER NOT NULL,
    sequence INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT production_task_events_task_fk FOREIGN KEY (production_task_run_id) REFERENCES production_task_runs(id) ON DELETE CASCADE,
    CONSTRAINT production_task_events_payload_check CHECK (json_valid(payload)),
    UNIQUE(production_task_run_id, sequence)
);
INSERT INTO production_task_events(id, uuid, production_task_run_id, sequence, event_type, payload, created_at)
SELECT backup.id, backup.uuid, backup.production_task_run_id, backup.sequence, backup.event_type, backup.payload, backup.created_at
FROM production_task_events_backup AS backup
JOIN production_task_runs AS tasks ON tasks.id = backup.production_task_run_id;
DROP TABLE production_task_events_backup;
CREATE INDEX production_task_events_task_sequence_index
    ON production_task_events(production_task_run_id, sequence);
CREATE TRIGGER production_task_events_append_only_update
BEFORE UPDATE ON production_task_events
BEGIN
    SELECT RAISE(ABORT, 'production_task_events are append-only');
END;

CREATE TRIGGER production_task_events_append_only_delete
BEFORE DELETE ON production_task_events
WHEN EXISTS (SELECT 1 FROM production_task_runs WHERE id = OLD.production_task_run_id)
BEGIN
    SELECT RAISE(ABORT, 'production_task_events are append-only');
END;
