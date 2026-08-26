ALTER TABLE chat_threads
    ADD COLUMN thread_type TEXT NOT NULL DEFAULT 'conversation'
    CHECK (thread_type IN ('conversation', 'workflow'));

-- Historical Workflow-only Threads have no Chat Turns. Keep them explicitly
-- typed so their terminal status can continue to mirror their Workflow, while
-- normal conversation Threads use aggregate activity state.
UPDATE chat_threads
SET thread_type = 'workflow'
WHERE EXISTS (
    SELECT 1 FROM workflows WHERE workflows.thread_id = chat_threads.id
)
AND NOT EXISTS (
    SELECT 1 FROM chat_turns WHERE chat_turns.thread_id = chat_threads.id
);

CREATE TABLE workflow_awaits (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    workflow_id INTEGER NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    chat_thread_id INTEGER NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    chat_turn_id INTEGER NOT NULL REFERENCES chat_turns(id) ON DELETE CASCADE,
    chat_run_id INTEGER NOT NULL REFERENCES chat_runs(id) ON DELETE CASCADE,
    tool_execution_id INTEGER NOT NULL REFERENCES agent_tool_executions(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'waiting',
    river_job_id INTEGER,
    created_at DATETIME NOT NULL,
    ready_at DATETIME,
    resumed_at DATETIME,
    cancelled_at DATETIME,
    updated_at DATETIME NOT NULL,
    CONSTRAINT workflow_awaits_uuid_check CHECK (length(uuid) = 36),
    CONSTRAINT workflow_awaits_status_check CHECK (status IN ('waiting', 'ready', 'resuming', 'resumed', 'cancelled')),
    UNIQUE(workflow_id),
    UNIQUE(tool_execution_id)
);

CREATE INDEX workflow_awaits_workflow_status_index
    ON workflow_awaits(workflow_id, status, id);
CREATE INDEX workflow_awaits_run_status_index
    ON workflow_awaits(chat_run_id, status, id);
CREATE INDEX workflow_awaits_thread_status_index
    ON workflow_awaits(chat_thread_id, status, created_at, id);
