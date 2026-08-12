PRAGMA defer_foreign_keys = ON;

DROP TRIGGER project_prompt_versions_append_only;

CREATE TABLE project_prompt_versions_agent (
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
    CONSTRAINT project_prompt_versions_restore_fk FOREIGN KEY (restored_from_version_id) REFERENCES project_prompt_versions_agent(id),
    CONSTRAINT project_prompt_versions_group_check CHECK (prompt_group IN ('story', 'chapter', 'premise', 'premise_style', 'agent')),
    CONSTRAINT project_prompt_versions_key_check CHECK (length(trim(prompt_key)) BETWEEN 1 AND 120),
    CONSTRAINT project_prompt_versions_version_check CHECK (version_no > 0),
    CONSTRAINT project_prompt_versions_prompt_check CHECK (length(trim(prompt)) > 0),
    CONSTRAINT project_prompt_versions_hash_check CHECK (length(prompt_hash) = 64),
    CONSTRAINT project_prompt_versions_source_check CHECK (source_type IN ('manual_edit', 'version_restore', 'project_language_changed', 'default_restore')),
    UNIQUE(project_id, prompt_group, prompt_key, version_no)
);

INSERT INTO project_prompt_versions_agent (
    id, uuid, project_id, actor_id, restored_from_version_id, prompt_group, prompt_key,
    version_no, prompt, prompt_hash, source_type, created_at
)
SELECT
    id, uuid, project_id, actor_id, restored_from_version_id,
    CASE WHEN prompt_group = 'premise' AND prompt_key LIKE 'agent.%' THEN 'agent' ELSE prompt_group END,
    CASE WHEN prompt_group = 'premise' AND prompt_key LIKE 'agent.%' THEN substr(prompt_key, 7) ELSE prompt_key END,
    version_no, prompt, prompt_hash, source_type, created_at
FROM project_prompt_versions;

DROP TABLE project_prompt_versions;
ALTER TABLE project_prompt_versions_agent RENAME TO project_prompt_versions;

CREATE INDEX project_prompt_versions_history_index
    ON project_prompt_versions(project_id, prompt_group, prompt_key, version_no DESC);

CREATE TRIGGER project_prompt_versions_append_only
BEFORE UPDATE ON project_prompt_versions
BEGIN
    SELECT RAISE(ABORT, 'project_prompt_versions are append-only');
END;

-- Rebuilding task_runs changes its kind CHECK. Back up every dependent row
-- before dropping the parent table: SQLite applies ON DELETE actions during a
-- DROP TABLE even when foreign-key checks are deferred.
CREATE TEMP TABLE migration_10_task_events AS SELECT * FROM task_events;
CREATE TEMP TABLE migration_10_agent_runs AS SELECT * FROM agent_runs;
CREATE TEMP TABLE migration_10_agent_events AS SELECT * FROM agent_events;
CREATE TEMP TABLE migration_10_llm_logs AS SELECT * FROM llm_logs;
CREATE TEMP TABLE migration_10_story_generation_results AS SELECT * FROM story_generation_results;

DROP TRIGGER task_events_append_only_update;
DROP TRIGGER task_events_append_only_delete;
DROP TRIGGER agent_events_append_only_update;
DROP TRIGGER agent_events_append_only_delete;

CREATE TABLE task_runs_next (
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
    CONSTRAINT task_runs_kind_check CHECK (kind IN ('story_chapter_generation', 'story_profile_generation', 'story_profile_from_chapters', 'story_chapter_batch_plan', 'comic_storyboard_generation')),
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

INSERT INTO task_runs_next SELECT * FROM task_runs;
DROP TABLE task_runs;
ALTER TABLE task_runs_next RENAME TO task_runs;

CREATE INDEX task_runs_project_status_created_index
    ON task_runs(project_id, status, created_at DESC, id DESC);
CREATE UNIQUE INDEX task_runs_active_resource_unique
    ON task_runs(project_id, kind, resource_uuid)
    WHERE status IN ('queued', 'running', 'waiting_for_input');

DROP TABLE llm_logs;
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
    CONSTRAINT llm_logs_scenario_check CHECK (scenario IN ('story_chapter_generation', 'story_profile_generation', 'story_profile_from_chapters', 'story_chapter_batch_plan', 'comic_storyboard_generation')),
    CONSTRAINT llm_logs_status_check CHECK (status IN ('pending', 'completed', 'failed', 'cancelled')),
    CONSTRAINT llm_logs_usage_check CHECK (input_tokens >= 0 AND output_tokens >= 0 AND duration_ms >= 0)
);
CREATE INDEX llm_logs_project_created_index ON llm_logs(project_id, created_at DESC, id DESC);
CREATE INDEX llm_logs_task_index ON llm_logs(task_run_id, created_at DESC, id DESC);

INSERT INTO task_events SELECT * FROM migration_10_task_events;
INSERT INTO agent_runs SELECT * FROM migration_10_agent_runs;
INSERT INTO agent_events SELECT * FROM migration_10_agent_events;
INSERT INTO llm_logs SELECT * FROM migration_10_llm_logs;
INSERT INTO story_generation_results SELECT * FROM migration_10_story_generation_results;

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

DROP TABLE migration_10_task_events;
DROP TABLE migration_10_agent_runs;
DROP TABLE migration_10_agent_events;
DROP TABLE migration_10_llm_logs;
DROP TABLE migration_10_story_generation_results;

CREATE TABLE story_prompt_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    task_run_id INTEGER NOT NULL UNIQUE,
    result_kind TEXT NOT NULL,
    output_json TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT story_prompt_results_task_fk FOREIGN KEY (task_run_id) REFERENCES task_runs(id) ON DELETE CASCADE,
    CONSTRAINT story_prompt_results_kind_check CHECK (result_kind IN ('story_profile_generation', 'story_profile_from_chapters', 'story_chapter_batch_plan', 'comic_storyboard_generation')),
    CONSTRAINT story_prompt_results_output_check CHECK (json_valid(output_json))
);
