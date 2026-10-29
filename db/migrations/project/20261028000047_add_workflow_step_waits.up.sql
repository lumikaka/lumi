-- A Workflow step may suspend more than once while its domain tasks run.
-- Each durable wait has its own wake-up UUID; dependencies use internal IDs.
CREATE TABLE workflow_step_waits (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    workflow_step_id INTEGER NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
    phase_key TEXT NOT NULL,
    generation INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'waiting',
    river_job_id INTEGER,
    execution_uuid TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    ready_at DATETIME,
    consumed_at DATETIME,
    cancelled_at DATETIME,
    CONSTRAINT workflow_step_waits_uuid_check CHECK (length(uuid) = 36),
    CONSTRAINT workflow_step_waits_generation_check CHECK (generation > 0),
    CONSTRAINT workflow_step_waits_status_check CHECK (status IN ('waiting', 'ready', 'consumed', 'cancelled')),
    UNIQUE(workflow_step_id, generation)
);

CREATE UNIQUE INDEX workflow_step_waits_active_step_unique
    ON workflow_step_waits(workflow_step_id)
    WHERE status IN ('waiting', 'ready');

CREATE TABLE workflow_step_wait_dependencies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    wait_id INTEGER NOT NULL REFERENCES workflow_step_waits(id) ON DELETE CASCADE,
    task_run_id INTEGER REFERENCES task_runs(id) ON DELETE RESTRICT,
    production_task_run_id INTEGER REFERENCES production_task_runs(id) ON DELETE RESTRICT,
    CONSTRAINT workflow_step_wait_dependencies_task_check CHECK (
        (task_run_id IS NOT NULL AND production_task_run_id IS NULL)
        OR (task_run_id IS NULL AND production_task_run_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX workflow_step_wait_dependencies_story_unique
    ON workflow_step_wait_dependencies(wait_id, task_run_id)
    WHERE task_run_id IS NOT NULL;
CREATE UNIQUE INDEX workflow_step_wait_dependencies_production_unique
    ON workflow_step_wait_dependencies(wait_id, production_task_run_id)
    WHERE production_task_run_id IS NOT NULL;
CREATE INDEX workflow_step_wait_dependencies_story_index
    ON workflow_step_wait_dependencies(task_run_id, wait_id)
    WHERE task_run_id IS NOT NULL;
CREATE INDEX workflow_step_wait_dependencies_production_index
    ON workflow_step_wait_dependencies(production_task_run_id, wait_id)
    WHERE production_task_run_id IS NOT NULL;

ALTER TABLE workflow_steps
    ADD COLUMN current_wait_id INTEGER REFERENCES workflow_step_waits(id) ON DELETE SET NULL;
