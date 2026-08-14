PRAGMA defer_foreign_keys = ON;

DELETE FROM comic_exports WHERE status = 'expired';

CREATE TABLE comic_exports_previous (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    chapter_id INTEGER,
    task_uuid TEXT NOT NULL UNIQUE,
    scope TEXT NOT NULL,
    format TEXT NOT NULL,
    status TEXT NOT NULL,
    snapshot_json TEXT NOT NULL,
    snapshot_hash TEXT NOT NULL,
    output_file_id INTEGER,
    relative_path TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    completed_at DATETIME,
    CONSTRAINT comic_exports_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT comic_exports_chapter_fk FOREIGN KEY (chapter_id) REFERENCES chapters(id) ON DELETE CASCADE,
    CONSTRAINT comic_exports_file_fk FOREIGN KEY (output_file_id) REFERENCES files(id) ON DELETE RESTRICT,
    CONSTRAINT comic_exports_scope_check CHECK (scope IN ('chapter', 'project')),
    CONSTRAINT comic_exports_format_check CHECK (format IN ('zip')),
    CONSTRAINT comic_exports_status_check CHECK (status IN ('queued', 'running', 'ready', 'failed', 'cancelled')),
    CONSTRAINT comic_exports_snapshot_check CHECK (json_valid(snapshot_json)),
    CONSTRAINT comic_exports_hash_check CHECK (length(snapshot_hash) = 64)
);

INSERT INTO comic_exports_previous (
    id, uuid, project_id, chapter_id, task_uuid, scope, format, status,
    snapshot_json, snapshot_hash, output_file_id, relative_path, error_code,
    created_at, completed_at
)
SELECT
    id, uuid, project_id, chapter_id, task_uuid, scope, format, status,
    snapshot_json, snapshot_hash, output_file_id, relative_path, error_code,
    created_at, completed_at
FROM comic_exports;

DROP TABLE comic_exports;
ALTER TABLE comic_exports_previous RENAME TO comic_exports;

CREATE UNIQUE INDEX comic_exports_ready_snapshot_unique
    ON comic_exports(project_id, scope, ifnull(chapter_id, 0), format, snapshot_hash)
    WHERE status = 'ready';
CREATE INDEX comic_exports_project_created_index
    ON comic_exports(project_id, created_at DESC, id DESC);
