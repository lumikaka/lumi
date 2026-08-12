CREATE TABLE llm_logs_next (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    task_run_id INTEGER REFERENCES task_runs(id) ON DELETE CASCADE,
    production_task_run_id INTEGER REFERENCES production_task_runs(id) ON DELETE CASCADE,
    agent_thread_id INTEGER REFERENCES agent_threads(id) ON DELETE SET NULL,
    agent_run_id INTEGER REFERENCES agent_runs(id) ON DELETE SET NULL,
    chat_thread_id INTEGER REFERENCES chat_threads(id) ON DELETE CASCADE,
    chat_run_id INTEGER REFERENCES chat_runs(id) ON DELETE CASCADE,
    workflow_id INTEGER REFERENCES workflows(id) ON DELETE CASCADE,
    workflow_step_id INTEGER REFERENCES workflow_steps(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL,
    scenario TEXT NOT NULL,
    request_type TEXT NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 0,
    provider_uuid TEXT NOT NULL,
    provider_type TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL,
    status TEXT NOT NULL,
    input_summary TEXT NOT NULL DEFAULT '',
    output_summary TEXT NOT NULL DEFAULT '',
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    finish_reason TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    http_status INTEGER NOT NULL DEFAULT 0,
    provider_error_code TEXT NOT NULL DEFAULT '',
    provider_request_id TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    completed_at DATETIME,
    CONSTRAINT llm_logs_source_check CHECK (source_type IN ('story_generation', 'project_chat', 'production', 'workflow')),
    CONSTRAINT llm_logs_scenario_check CHECK (length(trim(scenario)) BETWEEN 1 AND 120),
    CONSTRAINT llm_logs_request_type_check CHECK (request_type IN ('text', 'image')),
    CONSTRAINT llm_logs_attempt_check CHECK (attempt >= 0),
    CONSTRAINT llm_logs_status_check CHECK (status IN ('pending', 'completed', 'failed', 'cancelled')),
    CONSTRAINT llm_logs_usage_check CHECK (input_tokens >= 0 AND output_tokens >= 0 AND duration_ms >= 0 AND http_status >= 0),
    CONSTRAINT llm_logs_context_check CHECK (
        (source_type = 'story_generation' AND task_run_id IS NOT NULL AND production_task_run_id IS NULL AND chat_thread_id IS NULL AND chat_run_id IS NULL AND workflow_id IS NULL AND workflow_step_id IS NULL) OR
        (source_type = 'project_chat' AND task_run_id IS NULL AND production_task_run_id IS NULL AND chat_thread_id IS NOT NULL AND chat_run_id IS NOT NULL AND workflow_id IS NULL AND workflow_step_id IS NULL) OR
        (source_type = 'production' AND task_run_id IS NULL AND production_task_run_id IS NOT NULL AND chat_thread_id IS NULL AND chat_run_id IS NULL AND workflow_id IS NULL AND workflow_step_id IS NULL) OR
        (source_type = 'workflow' AND task_run_id IS NULL AND production_task_run_id IS NULL AND chat_thread_id IS NULL AND chat_run_id IS NULL AND workflow_id IS NOT NULL AND workflow_step_id IS NOT NULL)
    )
);

INSERT INTO llm_logs_next (
    uuid, project_id, task_run_id, agent_thread_id, agent_run_id,
    source_type, scenario, request_type, attempt,
    provider_uuid, provider_type, model, status,
    input_summary, output_summary, input_tokens, output_tokens, duration_ms,
    finish_reason, error_code, created_at, completed_at
)
SELECT
    uuid, project_id, task_run_id, agent_thread_id, agent_run_id,
    'story_generation', scenario, 'text', 0,
    provider_uuid, provider_type, model, status,
    input_summary, output_summary, input_tokens, output_tokens, duration_ms,
    finish_reason, error_code, created_at, completed_at
FROM llm_logs;

INSERT INTO llm_logs_next (
    uuid, project_id, chat_thread_id, chat_run_id,
    source_type, scenario, request_type, attempt,
    provider_uuid, provider_type, model, status,
    input_tokens, output_tokens, duration_ms, finish_reason, error_code,
    created_at, completed_at
)
SELECT
    calls.uuid, calls.project_id, calls.thread_id, calls.run_id,
    'project_chat', CASE WHEN threads.scene <> '' THEN threads.scene ELSE 'project_chat' END, 'text', 0,
    calls.provider_uuid, '', calls.model, calls.status,
    calls.input_tokens, calls.output_tokens, calls.duration_ms, calls.finish_reason, calls.error_code,
    calls.created_at, calls.created_at
FROM agent_model_calls AS calls
JOIN chat_threads AS threads ON threads.id = calls.thread_id;

DROP TABLE llm_logs;
DROP TABLE agent_model_calls;
ALTER TABLE llm_logs_next RENAME TO llm_logs;

CREATE INDEX llm_logs_project_created_index ON llm_logs(project_id, created_at DESC, id DESC);
CREATE INDEX llm_logs_task_index ON llm_logs(task_run_id, created_at DESC, id DESC);
CREATE INDEX llm_logs_production_task_index ON llm_logs(production_task_run_id, created_at DESC, id DESC);
CREATE INDEX llm_logs_chat_run_index ON llm_logs(chat_run_id, created_at, id);
CREATE INDEX llm_logs_workflow_step_index ON llm_logs(workflow_step_id, created_at, id);
CREATE INDEX llm_logs_project_status_index ON llm_logs(project_id, status, created_at DESC, id DESC);
