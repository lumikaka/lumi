CREATE TABLE project_model_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL UNIQUE,
    project_text_provider_uuid TEXT NOT NULL DEFAULT '',
    project_text_model TEXT NOT NULL DEFAULT '',
    project_image_provider_uuid TEXT NOT NULL DEFAULT '',
    project_image_model TEXT NOT NULL DEFAULT '',
    chat_area_provider_uuid TEXT NOT NULL DEFAULT '',
    chat_area_model TEXT NOT NULL DEFAULT '',
    story_text_provider_uuid TEXT NOT NULL DEFAULT '',
    story_text_model TEXT NOT NULL DEFAULT '',
    section_premise_selection_provider_uuid TEXT NOT NULL DEFAULT '',
    section_premise_selection_model TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT project_model_settings_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT project_model_settings_revision_check CHECK (revision >= 0),
    CONSTRAINT project_model_settings_project_text_check CHECK ((project_text_provider_uuid = '' AND project_text_model = '') OR (length(project_text_provider_uuid) = 36 AND length(trim(project_text_model)) BETWEEN 1 AND 512)),
    CONSTRAINT project_model_settings_project_image_check CHECK ((project_image_provider_uuid = '' AND project_image_model = '') OR (length(project_image_provider_uuid) = 36 AND length(trim(project_image_model)) BETWEEN 1 AND 512)),
    CONSTRAINT project_model_settings_chat_check CHECK ((chat_area_provider_uuid = '' AND chat_area_model = '') OR (length(chat_area_provider_uuid) = 36 AND length(trim(chat_area_model)) BETWEEN 1 AND 512)),
    CONSTRAINT project_model_settings_story_check CHECK ((story_text_provider_uuid = '' AND story_text_model = '') OR (length(story_text_provider_uuid) = 36 AND length(trim(story_text_model)) BETWEEN 1 AND 512)),
    CONSTRAINT project_model_settings_selection_check CHECK ((section_premise_selection_provider_uuid = '' AND section_premise_selection_model = '') OR (length(section_premise_selection_provider_uuid) = 36 AND length(trim(section_premise_selection_model)) BETWEEN 1 AND 512))
);

ALTER TABLE task_runs ADD COLUMN model_source TEXT NOT NULL DEFAULT 'legacy_frozen';
ALTER TABLE production_task_runs ADD COLUMN model_source TEXT NOT NULL DEFAULT 'legacy_frozen';
ALTER TABLE chat_threads ADD COLUMN model_source TEXT NOT NULL DEFAULT 'legacy_frozen';
ALTER TABLE chat_runs ADD COLUMN model_source TEXT NOT NULL DEFAULT 'legacy_frozen';
ALTER TABLE workflows ADD COLUMN model_source TEXT NOT NULL DEFAULT 'legacy_frozen';
