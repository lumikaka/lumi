CREATE TABLE chat_threads (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'idle',
    provider_uuid TEXT NOT NULL,
    model TEXT NOT NULL,
    next_turn_sequence INTEGER NOT NULL DEFAULT 1,
    next_item_sequence INTEGER NOT NULL DEFAULT 1,
    next_event_sequence INTEGER NOT NULL DEFAULT 1,
    archived_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT chat_threads_title_check CHECK (length(trim(title)) BETWEEN 1 AND 160),
    CONSTRAINT chat_threads_status_check CHECK (status IN ('idle', 'busy', 'waiting_for_input', 'completed', 'failed', 'cancelled', 'interrupted')),
    CONSTRAINT chat_threads_provider_uuid_check CHECK (length(provider_uuid) = 36),
    CONSTRAINT chat_threads_model_check CHECK (length(trim(model)) BETWEEN 1 AND 512),
    CONSTRAINT chat_threads_sequences_check CHECK (next_turn_sequence > 0 AND next_item_sequence > 0 AND next_event_sequence > 0)
);

CREATE INDEX chat_threads_project_updated_index
    ON chat_threads(project_id, archived_at, updated_at DESC, id DESC);

CREATE TABLE chat_turns (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    thread_id INTEGER NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL DEFAULT 'prompt',
    source_follow_up_id INTEGER,
    queue_sequence INTEGER NOT NULL,
    input_text TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    river_job_id INTEGER,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    cancel_requested_at DATETIME,
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT chat_turns_source_check CHECK (source_type IN ('prompt', 'follow_up')),
    CONSTRAINT chat_turns_queue_check CHECK (queue_sequence > 0),
    CONSTRAINT chat_turns_input_check CHECK (length(trim(input_text)) BETWEEN 1 AND 262144),
    CONSTRAINT chat_turns_status_check CHECK (status IN ('queued', 'in_progress', 'waiting_for_input', 'completed', 'failed', 'cancelled', 'interrupted')),
    UNIQUE(thread_id, queue_sequence)
);

CREATE INDEX chat_turns_thread_status_queue_index
    ON chat_turns(thread_id, status, queue_sequence, id);

CREATE TABLE chat_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    thread_id INTEGER NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    turn_id INTEGER NOT NULL UNIQUE REFERENCES chat_turns(id) ON DELETE CASCADE,
    trigger_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    step_count INTEGER NOT NULL DEFAULT 0,
    max_steps INTEGER NOT NULL DEFAULT 12,
    provider_uuid TEXT NOT NULL,
    model TEXT NOT NULL,
    context_bytes INTEGER NOT NULL DEFAULT 0,
    cancel_requested_at DATETIME,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT chat_runs_trigger_check CHECK (trigger_type IN ('prompt', 'follow_up')),
    CONSTRAINT chat_runs_status_check CHECK (status IN ('queued', 'in_progress', 'waiting_for_input', 'completed', 'failed', 'cancelled', 'interrupted')),
    CONSTRAINT chat_runs_steps_check CHECK (step_count >= 0 AND max_steps BETWEEN 1 AND 64),
    CONSTRAINT chat_runs_provider_uuid_check CHECK (length(provider_uuid) = 36),
    CONSTRAINT chat_runs_model_check CHECK (length(trim(model)) BETWEEN 1 AND 512),
    CONSTRAINT chat_runs_context_check CHECK (context_bytes >= 0)
);

CREATE INDEX chat_runs_thread_status_created_index
    ON chat_runs(thread_id, status, created_at DESC, id DESC);

CREATE TABLE chat_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    thread_id INTEGER NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    turn_id INTEGER REFERENCES chat_turns(id) ON DELETE CASCADE,
    run_id INTEGER REFERENCES chat_runs(id) ON DELETE SET NULL,
    sequence INTEGER NOT NULL,
    item_type TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    content_format TEXT NOT NULL DEFAULT 'text',
    status TEXT NOT NULL DEFAULT 'completed',
    remote_item_uuid TEXT NOT NULL DEFAULT '',
    tool_name TEXT NOT NULL DEFAULT '',
    target_uuid TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL,
    CONSTRAINT chat_items_sequence_check CHECK (sequence > 0),
    CONSTRAINT chat_items_type_check CHECK (item_type IN ('user_message', 'assistant_message', 'tool_call', 'tool_result', 'error', 'user_input_request', 'context_summary')),
    CONSTRAINT chat_items_role_check CHECK (role IN ('user', 'assistant', 'tool', 'system')),
    CONSTRAINT chat_items_format_check CHECK (content_format IN ('text', 'json')),
    CONSTRAINT chat_items_status_check CHECK (status IN ('pending', 'in_progress', 'completed', 'failed', 'cancelled')),
    CONSTRAINT chat_items_metadata_check CHECK (json_valid(metadata_json)),
    UNIQUE(thread_id, sequence)
);

CREATE INDEX chat_items_thread_sequence_index ON chat_items(thread_id, sequence, id);
CREATE INDEX chat_items_turn_sequence_index ON chat_items(turn_id, sequence, id);

CREATE TABLE chat_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    thread_id INTEGER NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    run_id INTEGER REFERENCES chat_runs(id) ON DELETE SET NULL,
    sequence INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL,
    CONSTRAINT chat_events_sequence_check CHECK (sequence > 0),
    CONSTRAINT chat_events_type_check CHECK (length(trim(event_type)) BETWEEN 1 AND 120),
    CONSTRAINT chat_events_payload_check CHECK (json_valid(payload_json)),
    UNIQUE(thread_id, sequence)
);

CREATE INDEX chat_events_thread_sequence_index ON chat_events(thread_id, sequence, id);

CREATE TABLE chat_follow_ups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    thread_id INTEGER NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    input_text TEXT NOT NULL,
    position INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    promoted_turn_id INTEGER REFERENCES chat_turns(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    deleted_at DATETIME,
    CONSTRAINT chat_follow_ups_input_check CHECK (length(trim(input_text)) BETWEEN 1 AND 262144),
    CONSTRAINT chat_follow_ups_position_check CHECK (position > 0),
    CONSTRAINT chat_follow_ups_status_check CHECK (status IN ('queued', 'promoted', 'deleted'))
);

CREATE INDEX chat_follow_ups_thread_position_index
    ON chat_follow_ups(thread_id, deleted_at, status, position, id);

CREATE TABLE chat_user_input_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    thread_id INTEGER NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    run_id INTEGER NOT NULL REFERENCES chat_runs(id) ON DELETE CASCADE,
    turn_id INTEGER NOT NULL REFERENCES chat_turns(id) ON DELETE CASCADE,
    item_id INTEGER NOT NULL UNIQUE REFERENCES chat_items(id) ON DELETE CASCADE,
    tool_call_uuid TEXT NOT NULL,
    input_type TEXT NOT NULL,
    question TEXT NOT NULL,
    options_json TEXT NOT NULL,
    response_json TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    answered_at DATETIME,
    resumed_at DATETIME,
    cancelled_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT chat_user_input_tool_call_check CHECK (length(tool_call_uuid) = 36),
    CONSTRAINT chat_user_input_type_check CHECK (input_type IN ('single_choice', 'multiple_choice')),
    CONSTRAINT chat_user_input_question_check CHECK (length(trim(question)) BETWEEN 1 AND 4000),
    CONSTRAINT chat_user_input_options_check CHECK (json_valid(options_json)),
    CONSTRAINT chat_user_input_response_check CHECK (response_json IS NULL OR json_valid(response_json)),
    CONSTRAINT chat_user_input_status_check CHECK (status IN ('pending', 'answered', 'resuming', 'resumed', 'cancelled')),
    UNIQUE(run_id, tool_call_uuid)
);

CREATE INDEX chat_user_input_thread_status_index
    ON chat_user_input_requests(thread_id, status, created_at, id);

CREATE TABLE agent_tool_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    thread_id INTEGER NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    run_id INTEGER NOT NULL REFERENCES chat_runs(id) ON DELETE CASCADE,
    turn_id INTEGER NOT NULL REFERENCES chat_turns(id) ON DELETE CASCADE,
    item_id INTEGER NOT NULL UNIQUE REFERENCES chat_items(id) ON DELETE CASCADE,
    tool_call_uuid TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    target_uuid TEXT NOT NULL DEFAULT '',
    arguments_json TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    state TEXT NOT NULL DEFAULT 'intent',
    result_json TEXT,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT agent_tool_call_uuid_check CHECK (length(tool_call_uuid) = 36),
    CONSTRAINT agent_tool_name_check CHECK (length(trim(tool_name)) BETWEEN 1 AND 120),
    CONSTRAINT agent_tool_arguments_check CHECK (json_valid(arguments_json)),
    CONSTRAINT agent_tool_result_check CHECK (result_json IS NULL OR json_valid(result_json)),
    CONSTRAINT agent_tool_state_check CHECK (state IN ('intent', 'executing', 'completed', 'failed')),
    UNIQUE(run_id, tool_call_uuid)
);

CREATE INDEX agent_tool_executions_run_state_index
    ON agent_tool_executions(run_id, state, created_at, id);

CREATE TABLE agent_context_summaries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    thread_id INTEGER NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    through_item_sequence INTEGER NOT NULL,
    summary TEXT NOT NULL,
    source_bytes INTEGER NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT agent_context_sequence_check CHECK (through_item_sequence > 0),
    CONSTRAINT agent_context_summary_check CHECK (length(trim(summary)) > 0),
    CONSTRAINT agent_context_bytes_check CHECK (source_bytes > 0),
    UNIQUE(thread_id, through_item_sequence)
);

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

CREATE INDEX agent_model_calls_run_created_index ON agent_model_calls(run_id, created_at, id);

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
    current_step_key TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    cancel_requested_at DATETIME,
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT workflows_kind_check CHECK (kind IN ('yolo_project_initialization')),
    CONSTRAINT workflows_title_check CHECK (length(trim(title)) BETWEEN 1 AND 160),
    CONSTRAINT workflows_status_check CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled', 'interrupted')),
    CONSTRAINT workflows_input_check CHECK (input_version > 0 AND json_valid(input_snapshot)),
    CONSTRAINT workflows_key_check CHECK (length(trim(idempotency_key)) BETWEEN 8 AND 160),
    CONSTRAINT workflows_provider_uuid_check CHECK (length(provider_uuid) = 36),
    CONSTRAINT workflows_model_check CHECK (length(trim(model)) BETWEEN 1 AND 512),
    UNIQUE(project_id, kind, idempotency_key)
);

CREATE INDEX workflows_project_status_created_index
    ON workflows(project_id, status, created_at DESC, id DESC);

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
    UNIQUE(workflow_id, step_key),
    UNIQUE(workflow_id, position)
);

CREATE INDEX workflow_steps_workflow_position_index ON workflow_steps(workflow_id, position, id);

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
    UNIQUE(workflow_id, sequence)
);

CREATE INDEX workflow_events_workflow_sequence_index ON workflow_events(workflow_id, sequence, id);
