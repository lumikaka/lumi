DROP INDEX llm_logs_project_scenario_index;
DROP INDEX llm_logs_project_provider_index;

ALTER TABLE llm_logs DROP COLUMN output_characters;
ALTER TABLE llm_logs DROP COLUMN input_characters;
ALTER TABLE llm_logs DROP COLUMN cached_input_tokens;
