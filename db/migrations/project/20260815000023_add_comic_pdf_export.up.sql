PRAGMA defer_foreign_keys = ON;

CREATE TABLE comic_exports_pdf (
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
    retention_days INTEGER NOT NULL DEFAULT 7,
    expires_at DATETIME,
    byte_size INTEGER NOT NULL DEFAULT 0,
    content_sha256 TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    completed_at DATETIME,
    CONSTRAINT comic_exports_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT comic_exports_chapter_fk FOREIGN KEY (chapter_id) REFERENCES chapters(id) ON DELETE CASCADE,
    CONSTRAINT comic_exports_file_fk FOREIGN KEY (output_file_id) REFERENCES files(id) ON DELETE RESTRICT,
    CONSTRAINT comic_exports_scope_check CHECK (scope IN ('chapter', 'project')),
    CONSTRAINT comic_exports_format_check CHECK (format IN ('zip', 'pdf')),
    CONSTRAINT comic_exports_status_check CHECK (status IN ('queued', 'running', 'ready', 'failed', 'cancelled', 'expired')),
    CONSTRAINT comic_exports_snapshot_check CHECK (json_valid(snapshot_json)),
    CONSTRAINT comic_exports_hash_check CHECK (length(snapshot_hash) = 64),
    CONSTRAINT comic_exports_retention_check CHECK (retention_days = 7),
    CONSTRAINT comic_exports_size_check CHECK (byte_size >= 0),
    CONSTRAINT comic_exports_content_hash_check CHECK (content_sha256 = '' OR (length(content_sha256) = 64 AND content_sha256 = lower(content_sha256)))
);

INSERT INTO comic_exports_pdf (
    id, uuid, project_id, chapter_id, task_uuid, scope, format, status,
    snapshot_json, snapshot_hash, output_file_id, relative_path,
    retention_days, expires_at, byte_size, content_sha256, error_code,
    created_at, completed_at
)
SELECT
    id, uuid, project_id, chapter_id, task_uuid, scope, format, status,
    snapshot_json, snapshot_hash, output_file_id, relative_path,
    retention_days, expires_at, byte_size, content_sha256, error_code,
    created_at, completed_at
FROM comic_exports;

DROP TABLE comic_exports;
ALTER TABLE comic_exports_pdf RENAME TO comic_exports;

CREATE UNIQUE INDEX comic_exports_ready_snapshot_unique
    ON comic_exports(project_id, scope, ifnull(chapter_id, 0), format, snapshot_hash)
    WHERE status = 'ready';
CREATE INDEX comic_exports_project_created_index
    ON comic_exports(project_id, created_at DESC, id DESC);
CREATE INDEX comic_exports_project_status_expiry_index
    ON comic_exports(project_id, status, expires_at, id);
CREATE INDEX comic_exports_legacy_output_file_index
    ON comic_exports(output_file_id)
    WHERE output_file_id IS NOT NULL;
