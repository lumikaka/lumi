ALTER TABLE project_setup_drafts
    ADD COLUMN generation_scope TEXT NOT NULL DEFAULT 'first_image'
    CHECK (generation_scope IN ('project_only', 'premise', 'first_image'));

UPDATE project_setup_drafts
SET field_sources_json = json_set(field_sources_json, '$.generation_scope', 'system_default')
WHERE json_extract(field_sources_json, '$.generation_scope') IS NULL;
