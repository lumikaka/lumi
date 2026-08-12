CREATE TABLE task_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    river_job_id INTEGER,
    kind TEXT NOT NULL,
    resource_uuid TEXT NOT NULL,
    input_version INTEGER NOT NULL,
    input_snapshot TEXT NOT NULL,
    status TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    retryable INTEGER NOT NULL DEFAULT 0,
    provider_uuid TEXT NOT NULL,
    model TEXT NOT NULL,
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
    CONSTRAINT task_runs_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT task_runs_kind_check CHECK (kind IN ('story_chapter_generation')),
    CONSTRAINT task_runs_resource_uuid_check CHECK (length(resource_uuid) = 36),
    CONSTRAINT task_runs_input_version_check CHECK (input_version > 0),
    CONSTRAINT task_runs_input_snapshot_check CHECK (json_valid(input_snapshot)),
    CONSTRAINT task_runs_status_check CHECK (status IN ('queued', 'running', 'waiting_for_input', 'completed', 'failed', 'cancelled', 'interrupted')),
    CONSTRAINT task_runs_idempotency_check CHECK (length(trim(idempotency_key)) BETWEEN 1 AND 255),
    CONSTRAINT task_runs_retryable_check CHECK (retryable IN (0, 1)),
    CONSTRAINT task_runs_provider_uuid_check CHECK (length(provider_uuid) = 36),
    CONSTRAINT task_runs_model_check CHECK (length(trim(model)) BETWEEN 1 AND 255),
    CONSTRAINT task_runs_progress_check CHECK (progress BETWEEN 0 AND 100),
    CONSTRAINT task_runs_attempt_check CHECK (attempt >= 0 AND max_attempts > 0),
    UNIQUE(project_id, kind, idempotency_key),
    UNIQUE(river_job_id)
);

CREATE INDEX task_runs_project_status_created_index
    ON task_runs(project_id, status, created_at DESC, id DESC);

CREATE UNIQUE INDEX task_runs_active_resource_unique
    ON task_runs(project_id, kind, resource_uuid)
    WHERE status IN ('queued', 'running', 'waiting_for_input');

CREATE TABLE task_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    task_run_id INTEGER NOT NULL,
    sequence INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT task_events_task_fk FOREIGN KEY (task_run_id) REFERENCES task_runs(id) ON DELETE CASCADE,
    CONSTRAINT task_events_sequence_check CHECK (sequence > 0),
    CONSTRAINT task_events_type_check CHECK (length(trim(event_type)) BETWEEN 1 AND 120),
    CONSTRAINT task_events_payload_check CHECK (json_valid(payload)),
    UNIQUE(task_run_id, sequence)
);

CREATE INDEX task_events_task_sequence_index ON task_events(task_run_id, sequence);

CREATE TRIGGER task_events_append_only_update
BEFORE UPDATE ON task_events
BEGIN
    SELECT RAISE(ABORT, 'task_events are append-only');
END;

CREATE TRIGGER task_events_append_only_delete
BEFORE DELETE ON task_events
BEGIN
    SELECT RAISE(ABORT, 'task_events are append-only');
END;

CREATE TABLE agent_threads (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    subject_type TEXT NOT NULL,
    subject_uuid TEXT NOT NULL,
    status TEXT NOT NULL,
    provider_uuid TEXT NOT NULL,
    model TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT agent_threads_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT agent_threads_kind_check CHECK (kind IN ('story_generation')),
    CONSTRAINT agent_threads_subject_check CHECK (subject_type IN ('chapter')),
    CONSTRAINT agent_threads_status_check CHECK (status IN ('idle', 'running', 'waiting_for_input', 'completed', 'failed', 'cancelled', 'interrupted')),
    UNIQUE(project_id, kind, subject_type, subject_uuid)
);

CREATE TABLE agent_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    agent_thread_id INTEGER NOT NULL,
    task_run_id INTEGER NOT NULL UNIQUE,
    trigger_type TEXT NOT NULL,
    status TEXT NOT NULL,
    input_summary TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT agent_runs_thread_fk FOREIGN KEY (agent_thread_id) REFERENCES agent_threads(id) ON DELETE CASCADE,
    CONSTRAINT agent_runs_task_fk FOREIGN KEY (task_run_id) REFERENCES task_runs(id) ON DELETE CASCADE,
    CONSTRAINT agent_runs_trigger_check CHECK (trigger_type IN ('job_step')),
    CONSTRAINT agent_runs_status_check CHECK (status IN ('queued', 'running', 'waiting_for_input', 'completed', 'failed', 'cancelled', 'interrupted'))
);

CREATE INDEX agent_runs_thread_created_index ON agent_runs(agent_thread_id, created_at DESC, id DESC);

CREATE TABLE agent_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    agent_thread_id INTEGER NOT NULL,
    agent_run_id INTEGER,
    sequence INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT agent_events_thread_fk FOREIGN KEY (agent_thread_id) REFERENCES agent_threads(id) ON DELETE CASCADE,
    CONSTRAINT agent_events_run_fk FOREIGN KEY (agent_run_id) REFERENCES agent_runs(id) ON DELETE CASCADE,
    CONSTRAINT agent_events_sequence_check CHECK (sequence > 0),
    CONSTRAINT agent_events_payload_check CHECK (json_valid(payload)),
    UNIQUE(agent_thread_id, sequence)
);

CREATE INDEX agent_events_thread_sequence_index ON agent_events(agent_thread_id, sequence);

CREATE TRIGGER agent_events_append_only_update
BEFORE UPDATE ON agent_events
BEGIN
    SELECT RAISE(ABORT, 'agent_events are append-only');
END;

CREATE TRIGGER agent_events_append_only_delete
BEFORE DELETE ON agent_events
BEGIN
    SELECT RAISE(ABORT, 'agent_events are append-only');
END;

CREATE TABLE llm_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    task_run_id INTEGER NOT NULL,
    agent_thread_id INTEGER,
    agent_run_id INTEGER,
    scenario TEXT NOT NULL,
    provider_uuid TEXT NOT NULL,
    provider_type TEXT NOT NULL,
    model TEXT NOT NULL,
    status TEXT NOT NULL,
    input_summary TEXT NOT NULL DEFAULT '',
    output_summary TEXT NOT NULL DEFAULT '',
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    finish_reason TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    completed_at DATETIME,
    CONSTRAINT llm_logs_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT llm_logs_task_fk FOREIGN KEY (task_run_id) REFERENCES task_runs(id) ON DELETE CASCADE,
    CONSTRAINT llm_logs_thread_fk FOREIGN KEY (agent_thread_id) REFERENCES agent_threads(id) ON DELETE SET NULL,
    CONSTRAINT llm_logs_run_fk FOREIGN KEY (agent_run_id) REFERENCES agent_runs(id) ON DELETE SET NULL,
    CONSTRAINT llm_logs_scenario_check CHECK (scenario IN ('story_chapter_generation')),
    CONSTRAINT llm_logs_status_check CHECK (status IN ('pending', 'completed', 'failed', 'cancelled')),
    CONSTRAINT llm_logs_usage_check CHECK (input_tokens >= 0 AND output_tokens >= 0 AND duration_ms >= 0)
);

CREATE INDEX llm_logs_project_created_index ON llm_logs(project_id, created_at DESC, id DESC);
CREATE INDEX llm_logs_task_index ON llm_logs(task_run_id, created_at DESC, id DESC);

CREATE TABLE story_generation_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    task_run_id INTEGER NOT NULL UNIQUE,
    chapter_id INTEGER NOT NULL,
    chapter_story_id INTEGER NOT NULL UNIQUE,
    created_at DATETIME NOT NULL,
    CONSTRAINT story_generation_results_task_fk FOREIGN KEY (task_run_id) REFERENCES task_runs(id) ON DELETE CASCADE,
    CONSTRAINT story_generation_results_chapter_fk FOREIGN KEY (chapter_id) REFERENCES chapters(id) ON DELETE CASCADE,
    CONSTRAINT story_generation_results_story_fk FOREIGN KEY (chapter_story_id) REFERENCES chapter_stories(id) ON DELETE CASCADE
);
