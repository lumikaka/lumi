CREATE TABLE premise_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL UNIQUE,
    default_style TEXT NOT NULL DEFAULT '',
    current_source_id INTEGER,
    current_setting_image_id INTEGER,
    revision INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT premise_profiles_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT premise_profiles_source_fk FOREIGN KEY (current_source_id) REFERENCES premise_sources(id) ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT premise_profiles_setting_fk FOREIGN KEY (current_setting_image_id) REFERENCES premise_setting_images(id) ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT premise_profiles_revision_check CHECK (revision >= 0)
);

CREATE TABLE premise_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    actor_id INTEGER NOT NULL,
    source_type TEXT NOT NULL,
    source_text TEXT NOT NULL,
    style_snapshot TEXT NOT NULL,
    provider_uuid TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    parameters_json TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL,
    CONSTRAINT premise_sources_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT premise_sources_actor_fk FOREIGN KEY (actor_id) REFERENCES actors(id),
    CONSTRAINT premise_sources_type_check CHECK (source_type IN ('manual', 'generated')),
    CONSTRAINT premise_sources_text_check CHECK (length(trim(source_text)) BETWEEN 1 AND 262144),
    CONSTRAINT premise_sources_parameters_check CHECK (json_valid(parameters_json))
);

CREATE INDEX premise_sources_project_created_index ON premise_sources(project_id, created_at DESC, id DESC);
CREATE TRIGGER premise_sources_append_only BEFORE UPDATE ON premise_sources BEGIN SELECT RAISE(ABORT, 'premise_sources are append-only'); END;

CREATE TABLE premise_setting_images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    source_id INTEGER,
    file_id INTEGER NOT NULL,
    origin TEXT NOT NULL,
    prompt TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    CONSTRAINT premise_setting_images_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT premise_setting_images_source_fk FOREIGN KEY (source_id) REFERENCES premise_sources(id) ON DELETE SET NULL,
    CONSTRAINT premise_setting_images_file_fk FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE RESTRICT,
    CONSTRAINT premise_setting_images_origin_check CHECK (origin IN ('manual', 'generated'))
);

CREATE INDEX premise_setting_images_project_created_index ON premise_setting_images(project_id, created_at DESC, id DESC);

CREATE TABLE premise_generation_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    task_uuid TEXT NOT NULL UNIQUE,
    source_id INTEGER,
    setting_image_id INTEGER,
    step_type TEXT NOT NULL,
    status TEXT NOT NULL,
    input_snapshot TEXT NOT NULL,
    output_json TEXT NOT NULL DEFAULT '{}',
    error_code TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    completed_at DATETIME,
    CONSTRAINT premise_steps_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT premise_steps_source_fk FOREIGN KEY (source_id) REFERENCES premise_sources(id) ON DELETE SET NULL,
    CONSTRAINT premise_steps_setting_fk FOREIGN KEY (setting_image_id) REFERENCES premise_setting_images(id) ON DELETE SET NULL,
    CONSTRAINT premise_steps_type_check CHECK (step_type IN ('setting_generation', 'asset_breakdown')),
    CONSTRAINT premise_steps_status_check CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    CONSTRAINT premise_steps_input_check CHECK (json_valid(input_snapshot)),
    CONSTRAINT premise_steps_output_check CHECK (json_valid(output_json))
);

CREATE TABLE premise_assets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    actor_id INTEGER NOT NULL,
    current_variant_id INTEGER,
    asset_type TEXT NOT NULL,
    title TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    position_json TEXT NOT NULL DEFAULT '{}',
    crop_json TEXT NOT NULL DEFAULT '{}',
    revision INTEGER NOT NULL DEFAULT 0,
    deleted_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT premise_assets_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT premise_assets_actor_fk FOREIGN KEY (actor_id) REFERENCES actors(id),
    CONSTRAINT premise_assets_variant_fk FOREIGN KEY (current_variant_id) REFERENCES premise_asset_variants(id) ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT premise_assets_type_check CHECK (asset_type IN ('character', 'scene', 'prop', 'reference')),
    CONSTRAINT premise_assets_title_check CHECK (length(trim(title)) BETWEEN 1 AND 160),
    CONSTRAINT premise_assets_position_check CHECK (json_valid(position_json)),
    CONSTRAINT premise_assets_crop_check CHECK (json_valid(crop_json)),
    CONSTRAINT premise_assets_revision_check CHECK (revision >= 0)
);

CREATE UNIQUE INDEX premise_assets_active_title_unique ON premise_assets(project_id, lower(title)) WHERE deleted_at IS NULL;
CREATE INDEX premise_assets_project_state_index ON premise_assets(project_id, deleted_at, updated_at DESC, id DESC);

CREATE TABLE premise_asset_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    premise_asset_id INTEGER NOT NULL,
    tag TEXT NOT NULL COLLATE NOCASE,
    created_at DATETIME NOT NULL,
    CONSTRAINT premise_asset_tags_asset_fk FOREIGN KEY (premise_asset_id) REFERENCES premise_assets(id) ON DELETE CASCADE,
    CONSTRAINT premise_asset_tags_value_check CHECK (tag = lower(trim(tag)) AND length(tag) BETWEEN 1 AND 64),
    UNIQUE(premise_asset_id, tag)
);

CREATE INDEX premise_asset_tags_tag_asset_index ON premise_asset_tags(tag, premise_asset_id);

CREATE TABLE premise_asset_variants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    premise_asset_id INTEGER NOT NULL,
    file_id INTEGER NOT NULL,
    source_setting_image_id INTEGER,
    version_no INTEGER NOT NULL,
    source_type TEXT NOT NULL,
    crop_json TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL,
    CONSTRAINT premise_asset_variants_asset_fk FOREIGN KEY (premise_asset_id) REFERENCES premise_assets(id) ON DELETE CASCADE,
    CONSTRAINT premise_asset_variants_file_fk FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE RESTRICT,
    CONSTRAINT premise_asset_variants_setting_fk FOREIGN KEY (source_setting_image_id) REFERENCES premise_setting_images(id) ON DELETE SET NULL,
    CONSTRAINT premise_asset_variants_version_check CHECK (version_no > 0),
    CONSTRAINT premise_asset_variants_source_check CHECK (source_type IN ('manual', 'breakdown', 'replacement')),
    CONSTRAINT premise_asset_variants_crop_check CHECK (json_valid(crop_json)),
    UNIQUE(premise_asset_id, version_no)
);

CREATE INDEX premise_asset_variants_asset_version_index ON premise_asset_variants(premise_asset_id, version_no DESC);

CREATE TRIGGER premise_asset_variants_append_only
BEFORE UPDATE ON premise_asset_variants
BEGIN
    SELECT RAISE(ABORT, 'premise_asset_variants are append-only');
END;

CREATE TABLE premise_asset_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    premise_asset_id INTEGER NOT NULL,
    sequence INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT premise_asset_events_asset_fk FOREIGN KEY (premise_asset_id) REFERENCES premise_assets(id) ON DELETE CASCADE,
    CONSTRAINT premise_asset_events_payload_check CHECK (json_valid(payload)),
    UNIQUE(premise_asset_id, sequence)
);

CREATE INDEX premise_asset_events_asset_sequence_index ON premise_asset_events(premise_asset_id, sequence);
CREATE TRIGGER premise_asset_events_append_only_update BEFORE UPDATE ON premise_asset_events BEGIN SELECT RAISE(ABORT, 'premise_asset_events are append-only'); END;
CREATE TRIGGER premise_asset_events_append_only_delete
BEFORE DELETE ON premise_asset_events
WHEN EXISTS (SELECT 1 FROM premise_assets WHERE id = OLD.premise_asset_id)
BEGIN SELECT RAISE(ABORT, 'premise_asset_events are append-only'); END;

CREATE TABLE chapter_comic_states (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    chapter_id INTEGER NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'empty',
    revision INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT comic_states_chapter_fk FOREIGN KEY (chapter_id) REFERENCES chapters(id) ON DELETE CASCADE,
    CONSTRAINT comic_states_status_check CHECK (status IN ('empty', 'draft', 'storyboarded', 'rendering', 'ready')),
    CONSTRAINT comic_states_revision_check CHECK (revision >= 0)
);

CREATE TABLE comic_sections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    chapter_comic_state_id INTEGER NOT NULL,
    actor_id INTEGER NOT NULL,
    section_no INTEGER NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    description_md TEXT NOT NULL DEFAULT '',
    current_storyboard_variant_id INTEGER,
    current_image_variant_id INTEGER,
    revision INTEGER NOT NULL DEFAULT 0,
    deleted_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT comic_sections_state_fk FOREIGN KEY (chapter_comic_state_id) REFERENCES chapter_comic_states(id) ON DELETE CASCADE,
    CONSTRAINT comic_sections_actor_fk FOREIGN KEY (actor_id) REFERENCES actors(id),
    CONSTRAINT comic_sections_storyboard_fk FOREIGN KEY (current_storyboard_variant_id) REFERENCES comic_storyboard_variants(id) ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT comic_sections_image_fk FOREIGN KEY (current_image_variant_id) REFERENCES comic_image_variants(id) ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT comic_sections_number_check CHECK (section_no > 0),
    CONSTRAINT comic_sections_revision_check CHECK (revision >= 0)
);

CREATE INDEX comic_sections_state_order_index ON comic_sections(chapter_comic_state_id, section_no);
CREATE UNIQUE INDEX comic_sections_active_order_unique ON comic_sections(chapter_comic_state_id, section_no) WHERE deleted_at IS NULL;

CREATE TABLE comic_storyboard_variants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    comic_section_id INTEGER NOT NULL,
    actor_id INTEGER NOT NULL,
    version_no INTEGER NOT NULL,
    content_md TEXT NOT NULL,
    source_type TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT comic_storyboards_section_fk FOREIGN KEY (comic_section_id) REFERENCES comic_sections(id) ON DELETE CASCADE,
    CONSTRAINT comic_storyboards_actor_fk FOREIGN KEY (actor_id) REFERENCES actors(id),
    CONSTRAINT comic_storyboards_version_check CHECK (version_no > 0),
    CONSTRAINT comic_storyboards_content_check CHECK (length(trim(content_md)) > 0),
    CONSTRAINT comic_storyboards_source_check CHECK (source_type IN ('manual', 'generated', 'restore')),
    UNIQUE(comic_section_id, version_no)
);

CREATE INDEX comic_storyboard_section_version_index ON comic_storyboard_variants(comic_section_id, version_no DESC);
CREATE TRIGGER comic_storyboard_variants_append_only BEFORE UPDATE ON comic_storyboard_variants BEGIN SELECT RAISE(ABORT, 'comic_storyboard_variants are append-only'); END;

CREATE TABLE comic_image_generations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    comic_section_id INTEGER NOT NULL,
    task_uuid TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,
    input_snapshot TEXT NOT NULL,
    error_code TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    completed_at DATETIME,
    CONSTRAINT comic_generations_section_fk FOREIGN KEY (comic_section_id) REFERENCES comic_sections(id) ON DELETE CASCADE,
    CONSTRAINT comic_generations_status_check CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    CONSTRAINT comic_generations_snapshot_check CHECK (json_valid(input_snapshot))
);

CREATE INDEX comic_generations_section_created_index ON comic_image_generations(comic_section_id, created_at DESC, id DESC);

CREATE TABLE comic_image_variants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    comic_section_id INTEGER NOT NULL,
    file_id INTEGER NOT NULL,
    image_generation_id INTEGER,
    actor_id INTEGER NOT NULL,
    version_no INTEGER NOT NULL,
    source_type TEXT NOT NULL,
    input_snapshot TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL,
    CONSTRAINT comic_images_section_fk FOREIGN KEY (comic_section_id) REFERENCES comic_sections(id) ON DELETE CASCADE,
    CONSTRAINT comic_images_file_fk FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE RESTRICT,
    CONSTRAINT comic_images_generation_fk FOREIGN KEY (image_generation_id) REFERENCES comic_image_generations(id) ON DELETE SET NULL,
    CONSTRAINT comic_images_actor_fk FOREIGN KEY (actor_id) REFERENCES actors(id),
    CONSTRAINT comic_images_version_check CHECK (version_no > 0),
    CONSTRAINT comic_images_source_check CHECK (source_type IN ('manual', 'generated', 'replacement', 'restore')),
    CONSTRAINT comic_images_snapshot_check CHECK (json_valid(input_snapshot)),
    UNIQUE(comic_section_id, version_no)
);

CREATE INDEX comic_image_section_version_index ON comic_image_variants(comic_section_id, version_no DESC);
CREATE TRIGGER comic_image_variants_append_only BEFORE UPDATE ON comic_image_variants BEGIN SELECT RAISE(ABORT, 'comic_image_variants are append-only'); END;

CREATE TABLE comic_section_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    comic_section_id INTEGER NOT NULL,
    sequence INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT comic_events_section_fk FOREIGN KEY (comic_section_id) REFERENCES comic_sections(id) ON DELETE CASCADE,
    CONSTRAINT comic_events_payload_check CHECK (json_valid(payload)),
    UNIQUE(comic_section_id, sequence)
);

CREATE INDEX comic_events_section_sequence_index ON comic_section_events(comic_section_id, sequence);
CREATE TRIGGER comic_section_events_append_only_update BEFORE UPDATE ON comic_section_events BEGIN SELECT RAISE(ABORT, 'comic_section_events are append-only'); END;
CREATE TRIGGER comic_section_events_append_only_delete
BEFORE DELETE ON comic_section_events
WHEN EXISTS (SELECT 1 FROM comic_sections WHERE id = OLD.comic_section_id)
BEGIN SELECT RAISE(ABORT, 'comic_section_events are append-only'); END;

CREATE TABLE comic_chapter_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    chapter_comic_state_id INTEGER NOT NULL,
    actor_id INTEGER NOT NULL,
    version_no INTEGER NOT NULL,
    reason TEXT NOT NULL,
    snapshot_json TEXT NOT NULL,
    snapshot_hash TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT comic_snapshots_state_fk FOREIGN KEY (chapter_comic_state_id) REFERENCES chapter_comic_states(id) ON DELETE CASCADE,
    CONSTRAINT comic_snapshots_actor_fk FOREIGN KEY (actor_id) REFERENCES actors(id),
    CONSTRAINT comic_snapshots_json_check CHECK (json_valid(snapshot_json)),
    CONSTRAINT comic_snapshots_hash_check CHECK (length(snapshot_hash) = 64),
    UNIQUE(chapter_comic_state_id, version_no)
);

CREATE INDEX comic_snapshots_state_version_index ON comic_chapter_snapshots(chapter_comic_state_id, version_no DESC);
CREATE TRIGGER comic_chapter_snapshots_append_only BEFORE UPDATE ON comic_chapter_snapshots BEGIN SELECT RAISE(ABORT, 'comic_chapter_snapshots are append-only'); END;

CREATE TABLE comic_exports (
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

CREATE UNIQUE INDEX comic_exports_ready_snapshot_unique ON comic_exports(project_id, scope, ifnull(chapter_id, 0), format, snapshot_hash) WHERE status = 'ready';
CREATE INDEX comic_exports_project_created_index ON comic_exports(project_id, created_at DESC, id DESC);

CREATE TABLE production_task_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    river_job_id INTEGER UNIQUE,
    kind TEXT NOT NULL,
    resource_uuid TEXT NOT NULL,
    input_snapshot TEXT NOT NULL,
    status TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    provider_uuid TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    progress INTEGER NOT NULL DEFAULT 0,
    attempt INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 1,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    cancel_requested_at DATETIME,
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT production_tasks_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT production_tasks_kind_check CHECK (kind IN ('premise_setting_generation', 'premise_asset_breakdown', 'comic_image_generation', 'comic_export')),
    CONSTRAINT production_tasks_snapshot_check CHECK (json_valid(input_snapshot)),
    CONSTRAINT production_tasks_status_check CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled', 'interrupted')),
    CONSTRAINT production_tasks_progress_check CHECK (progress BETWEEN 0 AND 100),
    UNIQUE(project_id, kind, idempotency_key)
);

CREATE UNIQUE INDEX production_tasks_active_resource_unique ON production_task_runs(project_id, kind, resource_uuid) WHERE status IN ('queued', 'running');
CREATE INDEX production_tasks_project_status_index ON production_task_runs(project_id, status, created_at DESC, id DESC);

CREATE TABLE production_task_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    production_task_run_id INTEGER NOT NULL,
    sequence INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT production_task_events_task_fk FOREIGN KEY (production_task_run_id) REFERENCES production_task_runs(id) ON DELETE CASCADE,
    CONSTRAINT production_task_events_payload_check CHECK (json_valid(payload)),
    UNIQUE(production_task_run_id, sequence)
);

CREATE INDEX production_task_events_task_sequence_index ON production_task_events(production_task_run_id, sequence);
CREATE TRIGGER production_task_events_append_only_update BEFORE UPDATE ON production_task_events BEGIN SELECT RAISE(ABORT, 'production_task_events are append-only'); END;
CREATE TRIGGER production_task_events_append_only_delete
BEFORE DELETE ON production_task_events
WHEN EXISTS (SELECT 1 FROM production_task_runs WHERE id = OLD.production_task_run_id)
BEGIN SELECT RAISE(ABORT, 'production_task_events are append-only'); END;
