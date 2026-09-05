PRAGMA defer_foreign_keys = ON;

CREATE TEMP TABLE comic_image_batch_workflow_thread_ids AS
SELECT thread_id FROM workflows
WHERE kind='comic_image_generation_batch' AND thread_id IS NOT NULL
AND EXISTS(SELECT 1 FROM chat_threads WHERE id=workflows.thread_id AND thread_type='workflow');
DELETE FROM workflows WHERE kind='comic_image_generation_batch';
DELETE FROM chat_threads WHERE id IN (SELECT thread_id FROM comic_image_batch_workflow_thread_ids);
DROP TABLE comic_image_batch_workflow_thread_ids;

CREATE TEMP TABLE workflows_comic_image_batch_backup AS SELECT * FROM workflows;
CREATE TEMP TABLE workflow_steps_comic_image_batch_backup AS SELECT * FROM workflow_steps;
CREATE TEMP TABLE workflow_events_comic_image_batch_backup AS SELECT * FROM workflow_events;
CREATE TEMP TABLE workflow_awaits_comic_image_batch_backup AS SELECT * FROM workflow_awaits;
CREATE TEMP TABLE llm_logs_comic_image_batch_backup AS
SELECT * FROM llm_logs WHERE workflow_id IS NOT NULL OR workflow_step_id IS NOT NULL;

DELETE FROM llm_logs WHERE workflow_id IS NOT NULL OR workflow_step_id IS NOT NULL;
DELETE FROM workflow_awaits;
DELETE FROM workflow_events;
DELETE FROM workflow_steps;
DROP TABLE workflows;

CREATE TABLE workflows (
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
    CONSTRAINT workflows_kind_check CHECK (kind IN ('yolo_project_initialization', 'premise_asset_generation', 'comic_section_image_generation', 'comic_storyboard_generation', 'story_chapter_generation', 'story_chapter_batch_plan')),
    CONSTRAINT workflows_title_check CHECK (length(trim(title)) BETWEEN 1 AND 160),
    CONSTRAINT workflows_status_check CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled', 'interrupted')),
    CONSTRAINT workflows_input_check CHECK (input_version > 0 AND json_valid(input_snapshot)),
    CONSTRAINT workflows_key_check CHECK (length(trim(idempotency_key)) BETWEEN 8 AND 160),
    CONSTRAINT workflows_provider_uuid_check CHECK (length(provider_uuid) = 36),
    CONSTRAINT workflows_model_check CHECK (length(trim(model)) BETWEEN 1 AND 512),
    UNIQUE(project_id, kind, idempotency_key)
);

INSERT INTO workflows (
    id,uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,
    idempotency_key,provider_uuid,model,model_source,current_step_key,error_code,
    error_message,cancel_requested_at,started_at,completed_at,created_at,updated_at
)
SELECT
    id,uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,
    idempotency_key,provider_uuid,model,model_source,current_step_key,error_code,
    error_message,cancel_requested_at,started_at,completed_at,created_at,updated_at
FROM workflows_comic_image_batch_backup;
CREATE INDEX workflows_project_status_created_index ON workflows(project_id,status,created_at DESC,id DESC);

INSERT INTO workflow_steps (
    id,uuid,workflow_id,step_key,position,status,idempotency_key,river_job_id,
    task_uuid,resource_uuid,input_json,output_json,error_code,error_message,
    started_at,completed_at,created_at,updated_at
)
SELECT
    id,uuid,workflow_id,step_key,position,status,idempotency_key,river_job_id,
    task_uuid,resource_uuid,input_json,output_json,error_code,error_message,
    started_at,completed_at,created_at,updated_at
FROM workflow_steps_comic_image_batch_backup;

INSERT INTO workflow_events (id,uuid,workflow_id,step_id,sequence,event_type,payload_json,created_at)
SELECT id,uuid,workflow_id,step_id,sequence,event_type,payload_json,created_at
FROM workflow_events_comic_image_batch_backup;

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
FROM llm_logs_comic_image_batch_backup;

INSERT INTO workflow_awaits (
    id,uuid,workflow_id,chat_thread_id,chat_turn_id,chat_run_id,tool_execution_id,
    status,river_job_id,created_at,ready_at,resumed_at,cancelled_at,updated_at
)
SELECT
    id,uuid,workflow_id,chat_thread_id,chat_turn_id,chat_run_id,tool_execution_id,
    status,river_job_id,created_at,ready_at,resumed_at,cancelled_at,updated_at
FROM workflow_awaits_comic_image_batch_backup;

DROP TABLE workflows_comic_image_batch_backup;
DROP TABLE workflow_steps_comic_image_batch_backup;
DROP TABLE workflow_events_comic_image_batch_backup;
DROP TABLE workflow_awaits_comic_image_batch_backup;
DROP TABLE llm_logs_comic_image_batch_backup;
