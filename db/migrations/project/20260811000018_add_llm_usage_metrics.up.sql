ALTER TABLE llm_logs
ADD COLUMN cached_input_tokens INTEGER
CHECK (cached_input_tokens IS NULL OR cached_input_tokens >= 0);

ALTER TABLE llm_logs
ADD COLUMN input_characters INTEGER
CHECK (input_characters IS NULL OR input_characters >= 0);

ALTER TABLE llm_logs
ADD COLUMN output_characters INTEGER
CHECK (output_characters IS NULL OR output_characters >= 0);

CREATE INDEX llm_logs_project_provider_index
ON llm_logs(project_id, provider_uuid, created_at DESC, id DESC);

CREATE INDEX llm_logs_project_scenario_index
ON llm_logs(project_id, scenario, created_at DESC, id DESC);
