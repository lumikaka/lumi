PRAGMA defer_foreign_keys = ON;

DROP TRIGGER project_prompt_versions_append_only;

CREATE TABLE project_prompt_versions_previous (
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
    CONSTRAINT project_prompt_versions_restore_fk FOREIGN KEY (restored_from_version_id) REFERENCES project_prompt_versions_previous(id),
    CONSTRAINT project_prompt_versions_group_check CHECK (prompt_group IN ('story', 'chapter', 'premise')),
    CONSTRAINT project_prompt_versions_key_check CHECK (length(trim(prompt_key)) BETWEEN 1 AND 120),
    CONSTRAINT project_prompt_versions_version_check CHECK (version_no > 0),
    CONSTRAINT project_prompt_versions_prompt_check CHECK (length(trim(prompt)) > 0),
    CONSTRAINT project_prompt_versions_hash_check CHECK (length(prompt_hash) = 64),
    CONSTRAINT project_prompt_versions_source_check CHECK (source_type IN ('manual_edit', 'version_restore')),
    UNIQUE(project_id, prompt_group, prompt_key, version_no)
);

-- Preserve every row on rollback. New premise_style records are namespaced into
-- the premise group, and migration-authored source types become manual edits.
INSERT INTO project_prompt_versions_previous (
    id, uuid, project_id, actor_id, restored_from_version_id, prompt_group, prompt_key,
    version_no, prompt, prompt_hash, source_type, created_at
)
SELECT
    id, uuid, project_id, actor_id, restored_from_version_id,
    CASE WHEN prompt_group = 'premise_style' THEN 'premise' ELSE prompt_group END,
    CASE WHEN prompt_group = 'premise_style' THEN 'premise_style.' || prompt_key ELSE prompt_key END,
    version_no, prompt, prompt_hash,
    CASE WHEN source_type = 'version_restore' THEN 'version_restore' ELSE 'manual_edit' END,
    created_at
FROM project_prompt_versions;

DROP TABLE project_prompt_versions;
ALTER TABLE project_prompt_versions_previous RENAME TO project_prompt_versions;

CREATE INDEX project_prompt_versions_history_index
    ON project_prompt_versions(project_id, prompt_group, prompt_key, version_no DESC);

CREATE TRIGGER project_prompt_versions_append_only
BEFORE UPDATE ON project_prompt_versions
BEGIN
    SELECT RAISE(ABORT, 'project_prompt_versions are append-only');
END;

