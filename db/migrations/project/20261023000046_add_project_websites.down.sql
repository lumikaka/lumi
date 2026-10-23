-- lumi: rebuild-without-foreign-key-actions
-- Refuse a lossy downgrade once a website generation exists.
CREATE TEMP TABLE website_down_guard (count INTEGER CHECK(count=0));
INSERT INTO website_down_guard SELECT COUNT(*) FROM website_generations;
DROP TABLE website_down_guard;
DROP TABLE project_websites;
DROP TABLE website_generations;
CREATE TABLE production_task_runs_next (
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
    model_source TEXT NOT NULL DEFAULT 'legacy_frozen',
    stage TEXT NOT NULL DEFAULT '',
    stage_started_at DATETIME,
    execution_uuid TEXT,
    CONSTRAINT production_tasks_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT production_tasks_kind_check CHECK (kind IN ('premise_setting_generation', 'premise_asset_breakdown', 'premise_asset_generation', 'comic_image_generation', 'comic_export')),
    CONSTRAINT production_tasks_snapshot_check CHECK (json_valid(input_snapshot)),
    CONSTRAINT production_tasks_status_check CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled', 'interrupted')),
    CONSTRAINT production_tasks_progress_check CHECK (progress BETWEEN 0 AND 100),
    UNIQUE(project_id, kind, idempotency_key)
);

INSERT INTO production_task_runs_next (
    id, uuid, project_id, river_job_id, kind, resource_uuid, input_snapshot, status,
    idempotency_key, provider_uuid, model, progress, attempt, max_attempts, error_code,
    error_message, cancel_requested_at, started_at, completed_at, created_at, updated_at, model_source, stage, stage_started_at, execution_uuid
)
SELECT
    id, uuid, project_id, river_job_id, kind, resource_uuid, input_snapshot, status,
    idempotency_key, provider_uuid, model, progress, attempt, max_attempts, error_code,
    error_message, cancel_requested_at, started_at, completed_at, created_at, updated_at, model_source, stage, stage_started_at, execution_uuid
FROM production_task_runs;

DROP TRIGGER production_task_events_append_only_delete;
DROP TABLE production_task_runs;
ALTER TABLE production_task_runs_next RENAME TO production_task_runs;
CREATE TRIGGER production_task_events_append_only_delete
BEFORE DELETE ON production_task_events
WHEN EXISTS (SELECT 1 FROM production_task_runs WHERE id = OLD.production_task_run_id)
BEGIN SELECT RAISE(ABORT, 'production_task_events are append-only'); END;

CREATE UNIQUE INDEX production_tasks_active_resource_unique
    ON production_task_runs(project_id, kind, resource_uuid)
    WHERE status IN ('queued', 'running');
CREATE INDEX production_tasks_project_status_index
    ON production_task_runs(project_id, status, created_at DESC, id DESC);
