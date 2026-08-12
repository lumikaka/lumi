ALTER TABLE llm_logs
ADD COLUMN request_payload TEXT
CHECK (request_payload IS NULL OR json_valid(request_payload));

ALTER TABLE llm_logs
ADD COLUMN response TEXT
CHECK (response IS NULL OR json_valid(response));
