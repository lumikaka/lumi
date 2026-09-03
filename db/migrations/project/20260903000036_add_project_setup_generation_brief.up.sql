ALTER TABLE project_setup_drafts
    ADD COLUMN generation_brief TEXT
    CHECK (generation_brief IS NULL OR length(trim(generation_brief)) BETWEEN 1 AND 4000);

UPDATE project_setup_drafts
SET generation_brief = substr(trim(original_input), 1, 4000),
    field_sources_json = json_set(field_sources_json, '$.generation_brief', 'system_default')
WHERE generation_brief IS NULL;
