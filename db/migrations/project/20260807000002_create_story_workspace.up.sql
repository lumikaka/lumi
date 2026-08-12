ALTER TABLE projects ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;

CREATE TABLE chapters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    volume_no INTEGER NOT NULL,
    chapter_no INTEGER NOT NULL,
    chapter_code TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    current_story_id INTEGER,
    revision INTEGER NOT NULL DEFAULT 0,
    deleted_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT chapters_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT chapters_current_story_fk FOREIGN KEY (current_story_id) REFERENCES chapter_stories(id) ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT chapters_numbers_check CHECK (volume_no > 0 AND chapter_no > 0 AND chapter_no < 100000),
    CONSTRAINT chapters_sort_order_check CHECK (sort_order > 0),
    CONSTRAINT chapters_revision_check CHECK (revision >= 0),
    CONSTRAINT chapters_code_check CHECK (chapter_code = lower(chapter_code) AND length(chapter_code) >= 10)
);

CREATE UNIQUE INDEX chapters_active_code_unique
    ON chapters(project_id, chapter_code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX chapters_active_number_unique
    ON chapters(project_id, volume_no, chapter_no) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX chapters_active_sort_unique
    ON chapters(project_id, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX chapters_state_order_index ON chapters(project_id, deleted_at, sort_order);

CREATE TABLE story_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    actor_id INTEGER NOT NULL,
    source_type TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    item_count INTEGER NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT story_sources_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT story_sources_actor_fk FOREIGN KEY (actor_id) REFERENCES actors(id),
    CONSTRAINT story_sources_type_check CHECK (source_type IN ('manual_entry', 'file_import', 'manual_edit')),
    CONSTRAINT story_sources_request_hash_check CHECK (length(request_hash) = 64),
    CONSTRAINT story_sources_item_count_check CHECK (item_count > 0),
    UNIQUE(project_id, source_type, request_hash)
);

CREATE TABLE story_source_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    source_id INTEGER NOT NULL,
    chapter_id INTEGER,
    ordinal INTEGER NOT NULL,
    original_filename TEXT,
    content_format TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    byte_size INTEGER NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT story_source_items_source_fk FOREIGN KEY (source_id) REFERENCES story_sources(id) ON DELETE CASCADE,
    CONSTRAINT story_source_items_chapter_fk FOREIGN KEY (chapter_id) REFERENCES chapters(id) ON DELETE SET NULL,
    CONSTRAINT story_source_items_ordinal_check CHECK (ordinal > 0),
    CONSTRAINT story_source_items_format_check CHECK (content_format IN ('txt', 'md')),
    CONSTRAINT story_source_items_hash_check CHECK (length(content_hash) = 64),
    CONSTRAINT story_source_items_size_check CHECK (byte_size > 0),
    UNIQUE(source_id, ordinal)
);

CREATE TABLE chapter_stories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    chapter_id INTEGER NOT NULL,
    actor_id INTEGER NOT NULL,
    story_source_id INTEGER NOT NULL,
    story_source_item_id INTEGER NOT NULL,
    version_no INTEGER NOT NULL,
    source_type TEXT NOT NULL,
    content TEXT NOT NULL,
    content_format TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    char_count INTEGER NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT chapter_stories_chapter_fk FOREIGN KEY (chapter_id) REFERENCES chapters(id) ON DELETE CASCADE,
    CONSTRAINT chapter_stories_actor_fk FOREIGN KEY (actor_id) REFERENCES actors(id),
    CONSTRAINT chapter_stories_source_fk FOREIGN KEY (story_source_id) REFERENCES story_sources(id),
    CONSTRAINT chapter_stories_source_item_fk FOREIGN KEY (story_source_item_id) REFERENCES story_source_items(id),
    CONSTRAINT chapter_stories_version_check CHECK (version_no > 0),
    CONSTRAINT chapter_stories_source_type_check CHECK (source_type IN ('manual_entry', 'file_import', 'manual_edit')),
    CONSTRAINT chapter_stories_content_check CHECK (length(trim(content)) > 0),
    CONSTRAINT chapter_stories_format_check CHECK (content_format IN ('txt', 'md')),
    CONSTRAINT chapter_stories_hash_check CHECK (length(content_hash) = 64),
    CONSTRAINT chapter_stories_char_count_check CHECK (char_count > 0),
    UNIQUE(chapter_id, version_no)
);

CREATE INDEX chapter_stories_chapter_version_index ON chapter_stories(chapter_id, version_no DESC);

CREATE TRIGGER chapter_stories_append_only
BEFORE UPDATE ON chapter_stories
BEGIN
    SELECT RAISE(ABORT, 'chapter_stories are append-only');
END;

CREATE TABLE project_story_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    actor_id INTEGER NOT NULL,
    version_no INTEGER NOT NULL,
    revision INTEGER NOT NULL,
    is_current INTEGER NOT NULL DEFAULT 1,
    story_md TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    source_type TEXT NOT NULL,
    projection_state TEXT NOT NULL,
    exported_revision INTEGER NOT NULL DEFAULT 0,
    exported_hash TEXT NOT NULL DEFAULT '',
    observed_file_hash TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    CONSTRAINT project_story_profiles_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT project_story_profiles_actor_fk FOREIGN KEY (actor_id) REFERENCES actors(id),
    CONSTRAINT project_story_profiles_version_check CHECK (version_no > 0 AND revision > 0),
    CONSTRAINT project_story_profiles_current_check CHECK (is_current IN (0, 1)),
    CONSTRAINT project_story_profiles_story_check CHECK (length(trim(story_md)) > 0),
    CONSTRAINT project_story_profiles_hash_check CHECK (length(content_hash) = 64),
    CONSTRAINT project_story_profiles_source_check CHECK (source_type IN ('project_created', 'manual_edit', 'external_import')),
    CONSTRAINT project_story_profiles_projection_check CHECK (projection_state IN ('pending', 'synced', 'conflict')),
    CONSTRAINT project_story_profiles_export_revision_check CHECK (exported_revision >= 0),
    UNIQUE(project_id, version_no),
    UNIQUE(project_id, revision)
);

CREATE UNIQUE INDEX project_story_profiles_current_unique
    ON project_story_profiles(project_id) WHERE is_current = 1;
CREATE INDEX project_story_profiles_history_index
    ON project_story_profiles(project_id, version_no DESC);

CREATE TRIGGER project_story_profiles_content_append_only
BEFORE UPDATE OF story_md, content_hash, source_type, version_no, revision, actor_id, project_id
ON project_story_profiles
BEGIN
    SELECT RAISE(ABORT, 'project_story_profiles content is append-only');
END;

CREATE TABLE project_prompt_versions (
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
    CONSTRAINT project_prompt_versions_restore_fk FOREIGN KEY (restored_from_version_id) REFERENCES project_prompt_versions(id),
    CONSTRAINT project_prompt_versions_group_check CHECK (prompt_group IN ('story', 'chapter')),
    CONSTRAINT project_prompt_versions_key_check CHECK (length(trim(prompt_key)) BETWEEN 1 AND 120),
    CONSTRAINT project_prompt_versions_version_check CHECK (version_no > 0),
    CONSTRAINT project_prompt_versions_prompt_check CHECK (length(trim(prompt)) > 0),
    CONSTRAINT project_prompt_versions_hash_check CHECK (length(prompt_hash) = 64),
    CONSTRAINT project_prompt_versions_source_check CHECK (source_type IN ('manual_edit', 'version_restore')),
    UNIQUE(project_id, prompt_group, prompt_key, version_no)
);

CREATE INDEX project_prompt_versions_history_index
    ON project_prompt_versions(project_id, prompt_group, prompt_key, version_no DESC);

CREATE TRIGGER project_prompt_versions_append_only
BEFORE UPDATE ON project_prompt_versions
BEGIN
    SELECT RAISE(ABORT, 'project_prompt_versions are append-only');
END;
