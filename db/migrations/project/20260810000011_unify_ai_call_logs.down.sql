CREATE TABLE agent_model_calls (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    thread_id INTEGER NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    run_id INTEGER NOT NULL REFERENCES chat_runs(id) ON DELETE CASCADE,
    provider_uuid TEXT NOT NULL,
    model TEXT NOT NULL,
    status TEXT NOT NULL,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    finish_reason TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    CONSTRAINT agent_model_calls_status_check CHECK (status IN ('completed', 'failed', 'cancelled')),
    CONSTRAINT agent_model_calls_usage_check CHECK (input_tokens >= 0 AND output_tokens >= 0 AND duration_ms >= 0)
);

INSERT INTO agent_model_calls (
    uuid, project_id, thread_id, run_id, provider_uuid, model, status,
    input_tokens, output_tokens, duration_ms, finish_reason, error_code, created_at
)
SELECT
    uuid, project_id, chat_thread_id, chat_run_id, provider_uuid, model,
    CASE WHEN status = 'pending' THEN 'failed' ELSE status END,
    input_tokens, output_tokens, duration_ms, finish_reason,
    CASE WHEN status = 'pending' THEN 'provider_call_interrupted' ELSE error_code END,
    created_at
FROM llm_logs
WHERE source_type = 'project_chat';

CREATE INDEX agent_model_calls_run_created_index ON agent_model_calls(run_id, created_at, id);

CREATE TABLE llm_logs_previous (
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

INSERT INTO llm_logs_previous (
    uuid, project_id, task_run_id, agent_thread_id, agent_run_id, scenario,
    provider_uuid, provider_type, model, status, input_summary, output_summary,
    input_tokens, output_tokens, duration_ms, finish_reason, error_code,
    created_at, completed_at
)
SELECT
    uuid, project_id, task_run_id, agent_thread_id, agent_run_id, scenario,
    provider_uuid, provider_type, model, status, input_summary, output_summary,
    input_tokens, output_tokens, duration_ms, finish_reason, error_code,
    created_at, completed_at
FROM llm_logs
WHERE source_type = 'story_generation'
  AND scenario IN ('story_chapter_generation', 'story_profile_generation', 'story_profile_from_chapters', 'story_chapter_batch_plan', 'comic_storyboard_generation');

DROP TABLE llm_logs;
ALTER TABLE llm_logs_previous RENAME TO llm_logs;
CREATE INDEX llm_logs_project_created_index ON llm_logs(project_id, created_at DESC, id DESC);
CREATE INDEX llm_logs_task_index ON llm_logs(task_run_id, created_at DESC, id DESC);
