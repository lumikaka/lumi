CREATE TABLE file_objects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    sha256 TEXT NOT NULL,
    key_path TEXT NOT NULL COLLATE NOCASE,
    mime_type TEXT NOT NULL,
    canonical_ext TEXT NOT NULL,
    byte_size INTEGER NOT NULL,
    width INTEGER,
    height INTEGER,
    duration_ms INTEGER,
    state TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    verified_at DATETIME,
    CONSTRAINT file_objects_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT file_objects_sha_check CHECK (length(sha256) = 64 AND sha256 = lower(sha256)),
    CONSTRAINT file_objects_key_check CHECK (length(trim(key_path)) BETWEEN 1 AND 700),
    CONSTRAINT file_objects_size_check CHECK (byte_size >= 0),
    CONSTRAINT file_objects_dimensions_check CHECK ((width IS NULL OR width > 0) AND (height IS NULL OR height > 0) AND (duration_ms IS NULL OR duration_ms >= 0)),
    CONSTRAINT file_objects_state_check CHECK (state IN ('pending', 'ready', 'missing', 'corrupt', 'quarantined')),
    UNIQUE(project_id, sha256),
    UNIQUE(project_id, key_path)
);

CREATE INDEX file_objects_project_state_created_index ON file_objects(project_id, state, created_at, id);

CREATE TRIGGER file_objects_ready_content_immutable
BEFORE UPDATE OF sha256, key_path, mime_type, canonical_ext, byte_size, width, height, duration_ms ON file_objects
WHEN OLD.state = 'ready'
BEGIN
    SELECT RAISE(ABORT, 'ready file object content is immutable');
END;

CREATE TABLE files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    file_object_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    purpose TEXT NOT NULL,
    original_filename TEXT,
    display_name TEXT,
    source_type TEXT NOT NULL,
    source_file_id INTEGER,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    actor_id INTEGER,
    created_at DATETIME NOT NULL,
    deleted_at DATETIME,
    CONSTRAINT files_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT files_object_fk FOREIGN KEY (file_object_id) REFERENCES file_objects(id) ON DELETE RESTRICT,
    CONSTRAINT files_source_fk FOREIGN KEY (source_file_id) REFERENCES files(id) ON DELETE RESTRICT,
    CONSTRAINT files_actor_fk FOREIGN KEY (actor_id) REFERENCES actors(id) ON DELETE SET NULL,
    CONSTRAINT files_kind_check CHECK (kind IN ('image', 'text', 'audio', 'video', 'archive', 'document', 'binary')),
    CONSTRAINT files_purpose_check CHECK (length(trim(purpose)) BETWEEN 1 AND 120),
    CONSTRAINT files_source_type_check CHECK (source_type IN ('imported', 'generated', 'derived', 'exported')),
    CONSTRAINT files_metadata_check CHECK (json_valid(metadata_json))
);

CREATE INDEX files_project_created_index ON files(project_id, created_at DESC, id DESC);
CREATE INDEX files_project_purpose_deleted_index ON files(project_id, purpose, deleted_at, created_at DESC);
CREATE INDEX files_object_index ON files(file_object_id);
CREATE INDEX files_source_index ON files(source_file_id);

CREATE TABLE upload_stashed (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    actor_id INTEGER NOT NULL,
    reserved_file_uuid TEXT NOT NULL UNIQUE,
    purpose TEXT NOT NULL,
    original_filename TEXT NOT NULL,
    display_name TEXT,
    mime_type TEXT,
    canonical_ext TEXT,
    byte_size INTEGER,
    sha256 TEXT,
    width INTEGER,
    height INTEGER,
    duration_ms INTEGER,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    state TEXT NOT NULL,
    file_object_id INTEGER,
    finalized_file_id INTEGER,
    error_code TEXT NOT NULL DEFAULT '',
    expires_at DATETIME NOT NULL,
    consumed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT upload_stashed_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT upload_stashed_actor_fk FOREIGN KEY (actor_id) REFERENCES actors(id) ON DELETE RESTRICT,
    CONSTRAINT upload_stashed_object_fk FOREIGN KEY (file_object_id) REFERENCES file_objects(id) ON DELETE RESTRICT,
    CONSTRAINT upload_stashed_file_fk FOREIGN KEY (finalized_file_id) REFERENCES files(id) ON DELETE RESTRICT,
    CONSTRAINT upload_stashed_state_check CHECK (state IN ('receiving', 'ready', 'consuming', 'consumed', 'failed', 'expired')),
    CONSTRAINT upload_stashed_metadata_check CHECK (json_valid(metadata_json)),
    CONSTRAINT upload_stashed_size_check CHECK (byte_size IS NULL OR byte_size >= 0),
    CONSTRAINT upload_stashed_sha_check CHECK (sha256 IS NULL OR (length(sha256) = 64 AND sha256 = lower(sha256))),
    CONSTRAINT upload_stashed_consumed_check CHECK ((state = 'consumed') = (consumed_at IS NOT NULL))
);

CREATE INDEX upload_stashed_project_state_expiry_index ON upload_stashed(project_id, state, expires_at, id);
CREATE INDEX upload_stashed_object_index ON upload_stashed(file_object_id);

CREATE TABLE asset_maintenance_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    river_job_id INTEGER UNIQUE,
    kind TEXT NOT NULL,
    resource_uuid TEXT NOT NULL,
    input_version INTEGER NOT NULL DEFAULT 1,
    input_snapshot TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL,
    progress INTEGER NOT NULL DEFAULT 0,
    attempt INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    cancel_requested_at DATETIME,
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT asset_maintenance_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT asset_maintenance_kind_check CHECK (kind IN ('asset_reconcile', 'asset_integrity_scan', 'asset_thumbnail_rebuild', 'asset_upload_cleanup', 'asset_gc_apply')),
    CONSTRAINT asset_maintenance_status_check CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled', 'interrupted')),
    CONSTRAINT asset_maintenance_progress_check CHECK (progress BETWEEN 0 AND 100),
    CONSTRAINT asset_maintenance_snapshot_check CHECK (json_valid(input_snapshot))
);

CREATE UNIQUE INDEX asset_maintenance_one_active_kind_index
    ON asset_maintenance_runs(project_id, kind)
    WHERE status IN ('queued', 'running');
CREATE INDEX asset_maintenance_project_created_index ON asset_maintenance_runs(project_id, created_at DESC, id DESC);

CREATE TABLE asset_maintenance_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    maintenance_run_id INTEGER NOT NULL,
    sequence INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT asset_maintenance_events_run_fk FOREIGN KEY (maintenance_run_id) REFERENCES asset_maintenance_runs(id) ON DELETE CASCADE,
    CONSTRAINT asset_maintenance_events_sequence_check CHECK (sequence > 0),
    CONSTRAINT asset_maintenance_events_payload_check CHECK (json_valid(payload)),
    UNIQUE(maintenance_run_id, sequence)
);

CREATE TRIGGER asset_maintenance_events_append_only_update
BEFORE UPDATE ON asset_maintenance_events
BEGIN
    SELECT RAISE(ABORT, 'asset maintenance events are append-only');
END;

CREATE TRIGGER asset_maintenance_events_append_only_delete
BEFORE DELETE ON asset_maintenance_events
BEGIN
    SELECT RAISE(ABORT, 'asset maintenance events are append-only');
END;

CREATE TABLE integrity_scans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    mode TEXT NOT NULL,
    status TEXT NOT NULL,
    progress INTEGER NOT NULL DEFAULT 0,
    checked_objects INTEGER NOT NULL DEFAULT 0,
    finding_count INTEGER NOT NULL DEFAULT 0,
    summary_json TEXT NOT NULL DEFAULT '{}',
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT integrity_scans_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT integrity_scans_mode_check CHECK (mode IN ('light', 'full')),
    CONSTRAINT integrity_scans_status_check CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    CONSTRAINT integrity_scans_progress_check CHECK (progress BETWEEN 0 AND 100),
    CONSTRAINT integrity_scans_summary_check CHECK (json_valid(summary_json))
);

CREATE UNIQUE INDEX integrity_scans_one_active_index ON integrity_scans(project_id, mode) WHERE status IN ('queued', 'running');
CREATE INDEX integrity_scans_project_created_index ON integrity_scans(project_id, created_at DESC, id DESC);

CREATE TABLE integrity_findings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    scan_id INTEGER NOT NULL,
    file_object_id INTEGER,
    kind TEXT NOT NULL,
    severity TEXT NOT NULL,
    safe_path TEXT,
    summary TEXT NOT NULL,
    resolution TEXT NOT NULL DEFAULT 'open',
    result_json TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL,
    resolved_at DATETIME,
    CONSTRAINT integrity_findings_scan_fk FOREIGN KEY (scan_id) REFERENCES integrity_scans(id) ON DELETE CASCADE,
    CONSTRAINT integrity_findings_object_fk FOREIGN KEY (file_object_id) REFERENCES file_objects(id) ON DELETE SET NULL,
    CONSTRAINT integrity_findings_kind_check CHECK (kind IN ('pending', 'missing', 'corrupt', 'quarantined', 'orphan', 'thumbnail')),
    CONSTRAINT integrity_findings_severity_check CHECK (severity IN ('info', 'warning', 'error')),
    CONSTRAINT integrity_findings_resolution_check CHECK (resolution IN ('open', 'reconciled', 'quarantined', 'ignored', 'removed')),
    CONSTRAINT integrity_findings_result_check CHECK (json_valid(result_json))
);

CREATE INDEX integrity_findings_scan_kind_index ON integrity_findings(scan_id, kind, id);

CREATE TABLE asset_gc_plans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    snapshot_hash TEXT NOT NULL,
    status TEXT NOT NULL,
    estimated_bytes INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    applied_at DATETIME,
    CONSTRAINT asset_gc_plans_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT asset_gc_plans_hash_check CHECK (length(snapshot_hash) = 64),
    CONSTRAINT asset_gc_plans_status_check CHECK (status IN ('dry_run', 'applied', 'stale'))
);

CREATE TABLE asset_gc_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    gc_plan_id INTEGER NOT NULL,
    file_object_id INTEGER,
    object_uuid TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    safe_key_path TEXT NOT NULL,
    byte_size INTEGER NOT NULL,
    reference_summary TEXT NOT NULL DEFAULT '{}',
    CONSTRAINT asset_gc_entries_plan_fk FOREIGN KEY (gc_plan_id) REFERENCES asset_gc_plans(id) ON DELETE CASCADE,
    CONSTRAINT asset_gc_entries_object_fk FOREIGN KEY (file_object_id) REFERENCES file_objects(id) ON DELETE SET NULL,
    CONSTRAINT asset_gc_entries_summary_check CHECK (json_valid(reference_summary)),
    UNIQUE(gc_plan_id, object_uuid)
);

ALTER TABLE story_source_items ADD COLUMN file_id INTEGER REFERENCES files(id) ON DELETE RESTRICT;
CREATE INDEX story_source_items_file_index ON story_source_items(file_id);
