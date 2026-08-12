PRAGMA defer_foreign_keys = ON;

CREATE TEMP TABLE llm_logs_storyboard_workflow_backup AS SELECT * FROM llm_logs;
DROP TABLE llm_logs;
CREATE TEMP TABLE workflow_events_storyboard_workflow_backup AS SELECT * FROM workflow_events;
DROP TABLE workflow_events;
CREATE TEMP TABLE workflow_steps_storyboard_workflow_backup AS SELECT * FROM workflow_steps;
DROP TABLE workflow_steps;

CREATE TABLE workflows_next (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    thread_id INTEGER REFERENCES chat_threads(id) ON DELETE SET NULL,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    input_version INTEGER NOT NULL DEFAULT 1,
    input_snapshot TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    provider_uuid TEXT NOT NULL,
    model TEXT NOT NULL,
    model_source TEXT NOT NULL DEFAULT 'legacy_frozen',
    current_step_key TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    cancel_requested_at DATETIME,
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT workflows_kind_check CHECK (kind IN ('yolo_project_initialization', 'comic_section_image_generation', 'comic_storyboard_generation')),
    CONSTRAINT workflows_title_check CHECK (length(trim(title)) BETWEEN 1 AND 160),
    CONSTRAINT workflows_status_check CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled', 'interrupted')),
    CONSTRAINT workflows_input_check CHECK (input_version > 0 AND json_valid(input_snapshot)),
    CONSTRAINT workflows_key_check CHECK (length(trim(idempotency_key)) BETWEEN 8 AND 160),
    CONSTRAINT workflows_provider_uuid_check CHECK (length(provider_uuid) = 36),
    CONSTRAINT workflows_model_check CHECK (length(trim(model)) BETWEEN 1 AND 512),
    UNIQUE(project_id, kind, idempotency_key)
);

INSERT INTO workflows_next (
    id,uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,
    idempotency_key,provider_uuid,model,model_source,current_step_key,error_code,
    error_message,cancel_requested_at,started_at,completed_at,created_at,updated_at
)
SELECT
    id,uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,
    idempotency_key,provider_uuid,model,model_source,current_step_key,error_code,
    error_message,cancel_requested_at,started_at,completed_at,created_at,updated_at
FROM workflows;

DROP TABLE workflows;
ALTER TABLE workflows_next RENAME TO workflows;
CREATE INDEX workflows_project_status_created_index ON workflows(project_id,status,created_at DESC,id DESC);

CREATE TABLE workflow_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    workflow_id INTEGER NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    step_key TEXT NOT NULL,
    position INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    idempotency_key TEXT NOT NULL UNIQUE,
    river_job_id INTEGER,
    task_uuid TEXT NOT NULL DEFAULT '',
    resource_uuid TEXT NOT NULL DEFAULT '',
    input_json TEXT NOT NULL DEFAULT '{}',
    output_json TEXT NOT NULL DEFAULT '{}',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT workflow_steps_key_check CHECK (length(trim(step_key)) BETWEEN 1 AND 120),
    CONSTRAINT workflow_steps_position_check CHECK (position > 0),
    CONSTRAINT workflow_steps_status_check CHECK (status IN ('pending', 'queued', 'running', 'waiting', 'completed', 'failed', 'cancelled', 'interrupted')),
    CONSTRAINT workflow_steps_input_check CHECK (json_valid(input_json)),
    CONSTRAINT workflow_steps_output_check CHECK (json_valid(output_json)),
    UNIQUE(workflow_id,step_key),
    UNIQUE(workflow_id,position)
);
INSERT INTO workflow_steps (
    id,uuid,workflow_id,step_key,position,status,idempotency_key,river_job_id,
    task_uuid,resource_uuid,input_json,output_json,error_code,error_message,
    started_at,completed_at,created_at,updated_at
)
SELECT
    id,uuid,workflow_id,step_key,position,status,idempotency_key,river_job_id,
    task_uuid,resource_uuid,input_json,output_json,error_code,error_message,
    started_at,completed_at,created_at,updated_at
FROM workflow_steps_storyboard_workflow_backup;
DROP TABLE workflow_steps_storyboard_workflow_backup;
CREATE INDEX workflow_steps_workflow_position_index ON workflow_steps(workflow_id,position,id);

CREATE TABLE workflow_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    workflow_id INTEGER NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    step_id INTEGER REFERENCES workflow_steps(id) ON DELETE SET NULL,
    sequence INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL,
    CONSTRAINT workflow_events_sequence_check CHECK (sequence > 0),
    CONSTRAINT workflow_events_payload_check CHECK (json_valid(payload_json)),
    UNIQUE(workflow_id,sequence)
);
INSERT INTO workflow_events (id,uuid,workflow_id,step_id,sequence,event_type,payload_json,created_at)
SELECT id,uuid,workflow_id,step_id,sequence,event_type,payload_json,created_at
FROM workflow_events_storyboard_workflow_backup;
DROP TABLE workflow_events_storyboard_workflow_backup;
CREATE INDEX workflow_events_workflow_sequence_index ON workflow_events(workflow_id,sequence,id);

CREATE TABLE llm_logs (
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
    request_payload TEXT CHECK (request_payload IS NULL OR json_valid(request_payload)),
    response TEXT CHECK (response IS NULL OR json_valid(response)),
    cached_input_tokens INTEGER CHECK (cached_input_tokens IS NULL OR cached_input_tokens >= 0),
    input_characters INTEGER CHECK (input_characters IS NULL OR input_characters >= 0),
    output_characters INTEGER CHECK (output_characters IS NULL OR output_characters >= 0),
    CONSTRAINT llm_logs_source_check CHECK (source_type IN ('story_generation', 'project_chat', 'production', 'workflow')),
    CONSTRAINT llm_logs_scenario_check CHECK (length(trim(scenario)) BETWEEN 1 AND 120),
    CONSTRAINT llm_logs_request_type_check CHECK (request_type IN ('text', 'image')),
    CONSTRAINT llm_logs_attempt_check CHECK (attempt >= 0),
    CONSTRAINT llm_logs_status_check CHECK (status IN ('pending', 'completed', 'failed', 'cancelled')),
    CONSTRAINT llm_logs_usage_check CHECK (input_tokens >= 0 AND output_tokens >= 0 AND duration_ms >= 0 AND http_status >= 0),
    CONSTRAINT llm_logs_context_check CHECK (
        (source_type='story_generation' AND task_run_id IS NOT NULL AND production_task_run_id IS NULL AND chat_thread_id IS NULL AND chat_run_id IS NULL AND workflow_id IS NULL AND workflow_step_id IS NULL) OR
        (source_type='project_chat' AND task_run_id IS NULL AND production_task_run_id IS NULL AND chat_thread_id IS NOT NULL AND chat_run_id IS NOT NULL AND workflow_id IS NULL AND workflow_step_id IS NULL) OR
        (source_type='production' AND task_run_id IS NULL AND production_task_run_id IS NOT NULL AND chat_thread_id IS NULL AND chat_run_id IS NULL AND workflow_id IS NULL AND workflow_step_id IS NULL) OR
        (source_type='workflow' AND task_run_id IS NULL AND production_task_run_id IS NULL AND chat_thread_id IS NULL AND chat_run_id IS NULL AND workflow_id IS NOT NULL AND workflow_step_id IS NOT NULL)
    )
);
INSERT INTO llm_logs (
    id,uuid,project_id,task_run_id,production_task_run_id,agent_thread_id,agent_run_id,
    chat_thread_id,chat_run_id,workflow_id,workflow_step_id,source_type,scenario,
    request_type,attempt,provider_uuid,provider_type,model,status,input_summary,
    output_summary,input_tokens,output_tokens,duration_ms,finish_reason,error_code,
    error_message,http_status,provider_error_code,provider_request_id,created_at,
    completed_at,request_payload,response,cached_input_tokens,input_characters,output_characters
)
SELECT
    id,uuid,project_id,task_run_id,production_task_run_id,agent_thread_id,agent_run_id,
    chat_thread_id,chat_run_id,workflow_id,workflow_step_id,source_type,scenario,
    request_type,attempt,provider_uuid,provider_type,model,status,input_summary,
    output_summary,input_tokens,output_tokens,duration_ms,finish_reason,error_code,
    error_message,http_status,provider_error_code,provider_request_id,created_at,
    completed_at,request_payload,response,cached_input_tokens,input_characters,output_characters
FROM llm_logs_storyboard_workflow_backup;
DROP TABLE llm_logs_storyboard_workflow_backup;
CREATE INDEX llm_logs_project_created_index ON llm_logs(project_id,created_at DESC,id DESC);
CREATE INDEX llm_logs_task_index ON llm_logs(task_run_id,created_at DESC,id DESC);
CREATE INDEX llm_logs_production_task_index ON llm_logs(production_task_run_id,created_at DESC,id DESC);
CREATE INDEX llm_logs_chat_run_index ON llm_logs(chat_run_id,created_at,id);
CREATE INDEX llm_logs_workflow_step_index ON llm_logs(workflow_step_id,created_at,id);
CREATE INDEX llm_logs_project_status_index ON llm_logs(project_id,status,created_at DESC,id DESC);
CREATE INDEX llm_logs_project_provider_index ON llm_logs(project_id,provider_uuid,created_at DESC,id DESC);
CREATE INDEX llm_logs_project_scenario_index ON llm_logs(project_id,scenario,created_at DESC,id DESC);
