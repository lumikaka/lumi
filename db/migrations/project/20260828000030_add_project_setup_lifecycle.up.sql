ALTER TABLE projects
    ADD COLUMN setup_status TEXT NOT NULL DEFAULT 'ready'
    CHECK (setup_status IN ('draft','ready'));

CREATE TABLE project_setup_drafts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'draft',
    revision INTEGER NOT NULL DEFAULT 1,
    original_input TEXT NOT NULL,
    project_name TEXT,
    generation_language TEXT,
    overall_style TEXT,
    format TEXT,
    aspect_ratio_mode TEXT,
    aspect_width INTEGER,
    aspect_height INTEGER,
    large_image_minimal_text INTEGER,
    interaction_mode TEXT,
    comic_layout TEXT,
    field_sources_json TEXT NOT NULL DEFAULT '{}',
    missing_fields_json TEXT NOT NULL DEFAULT '[]',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    finalized_revision INTEGER,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    finalized_at DATETIME,
    failed_at DATETIME,
    CONSTRAINT project_setup_drafts_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT project_setup_drafts_status_check CHECK (status IN ('draft','pending_confirmation','finalized','failed')),
    CONSTRAINT project_setup_drafts_revision_check CHECK (revision >= 1),
    CONSTRAINT project_setup_drafts_original_input_check CHECK (length(trim(original_input)) BETWEEN 1 AND 262144),
    CONSTRAINT project_setup_drafts_language_check CHECK (generation_language IS NULL OR generation_language IN ('zh-Hans','en')),
    CONSTRAINT project_setup_drafts_format_check CHECK (format IS NULL OR format IN ('classic_picture_book','wordless_picture_book','interactive_picture_book','comic_story','vertical_strip')),
    CONSTRAINT project_setup_drafts_aspect_mode_check CHECK (aspect_ratio_mode IS NULL OR aspect_ratio_mode IN ('landscape','square','portrait','custom','fixed')),
    CONSTRAINT project_setup_drafts_boolean_check CHECK (large_image_minimal_text IS NULL OR large_image_minimal_text IN (0,1)),
    CONSTRAINT project_setup_drafts_interaction_check CHECK (interaction_mode IS NULL OR interaction_mode IN ('find_it','make_a_choice','guess','follow_along')),
    CONSTRAINT project_setup_drafts_comic_layout_check CHECK (comic_layout IS NULL OR comic_layout IN ('four_panel','page_comic')),
    CONSTRAINT project_setup_drafts_sources_json_check CHECK (json_valid(field_sources_json) AND json_type(field_sources_json) = 'object'),
    CONSTRAINT project_setup_drafts_missing_json_check CHECK (json_valid(missing_fields_json) AND json_type(missing_fields_json) = 'array')
);

CREATE INDEX project_setup_drafts_project_status_index
    ON project_setup_drafts(project_id, status);

CREATE TABLE project_creation_bootstraps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    creation_session_uuid TEXT NOT NULL UNIQUE,
    thread_id INTEGER NOT NULL UNIQUE,
    turn_id INTEGER NOT NULL UNIQUE,
    created_at DATETIME NOT NULL,
    CONSTRAINT project_creation_bootstraps_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT project_creation_bootstraps_thread_fk FOREIGN KEY (thread_id) REFERENCES chat_threads(id) ON DELETE CASCADE,
    CONSTRAINT project_creation_bootstraps_turn_fk FOREIGN KEY (turn_id) REFERENCES chat_turns(id) ON DELETE CASCADE
);

CREATE TRIGGER project_picture_book_profiles_immutable_delete
BEFORE DELETE ON project_picture_book_profiles
BEGIN
    SELECT RAISE(ABORT, 'picture book profile is immutable');
END;
