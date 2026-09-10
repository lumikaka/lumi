-- lumi: rebuild-without-foreign-key-actions
-- Refuse rollback while batch workflows exist; preserve tasks and chat awaits.
CREATE TEMP TABLE premise_batch_rollback_guard (count INTEGER CHECK (count = 0));
INSERT INTO premise_batch_rollback_guard SELECT count(*) FROM workflows WHERE kind='premise_batch_generation';
DROP TABLE premise_batch_rollback_guard;

CREATE TEMP TABLE workflows_premise_batch_sequence AS SELECT seq FROM sqlite_sequence WHERE name='workflows';

CREATE TABLE workflows_premise_batch_new (
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
    CONSTRAINT workflows_kind_check CHECK (kind IN ('yolo_project_initialization', 'premise_asset_generation', 'comic_section_image_generation', 'comic_image_generation_batch', 'comic_storyboard_generation', 'story_chapter_generation', 'story_chapter_batch_plan', 'story_profile_generation', 'story_profile_from_chapters')),
    CONSTRAINT workflows_title_check CHECK (length(trim(title)) BETWEEN 1 AND 160),
    CONSTRAINT workflows_status_check CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled', 'interrupted')),
    CONSTRAINT workflows_input_check CHECK (input_version > 0 AND json_valid(input_snapshot)),
    CONSTRAINT workflows_key_check CHECK (length(trim(idempotency_key)) BETWEEN 8 AND 160),
    CONSTRAINT workflows_provider_uuid_check CHECK (length(provider_uuid) = 36),
    CONSTRAINT workflows_model_check CHECK (length(trim(model)) BETWEEN 1 AND 512),
    UNIQUE(project_id, kind, idempotency_key)
);

INSERT INTO workflows_premise_batch_new SELECT * FROM workflows;
DROP TABLE workflows;
ALTER TABLE workflows_premise_batch_new RENAME TO workflows;
CREATE INDEX workflows_project_status_created_index ON workflows(project_id,status,created_at DESC,id DESC);
UPDATE sqlite_sequence SET seq=MAX(seq,COALESCE((SELECT seq FROM workflows_premise_batch_sequence),0)) WHERE name='workflows';
INSERT INTO sqlite_sequence(name,seq) SELECT 'workflows',seq FROM workflows_premise_batch_sequence WHERE NOT EXISTS(SELECT 1 FROM sqlite_sequence WHERE name='workflows');
DROP TABLE workflows_premise_batch_sequence;
