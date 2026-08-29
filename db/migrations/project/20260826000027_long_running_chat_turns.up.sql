ALTER TABLE chat_runs ADD COLUMN model_request_count INTEGER NOT NULL DEFAULT 0 CHECK (model_request_count >= 0);
ALTER TABLE chat_runs ADD COLUMN max_model_requests INTEGER NOT NULL DEFAULT 256 CHECK (max_model_requests >= 2);
ALTER TABLE chat_runs ADD COLUMN active_duration_ms INTEGER NOT NULL DEFAULT 0 CHECK (active_duration_ms >= 0);
ALTER TABLE chat_runs ADD COLUMN max_active_duration_ms INTEGER NOT NULL DEFAULT 7200000 CHECK (max_active_duration_ms > 0);
ALTER TABLE chat_runs ADD COLUMN token_units INTEGER NOT NULL DEFAULT 0 CHECK (token_units >= 0);
ALTER TABLE chat_runs ADD COLUMN max_token_units INTEGER NOT NULL DEFAULT 1000000 CHECK (max_token_units > 0);
ALTER TABLE chat_runs ADD COLUMN no_progress_streak INTEGER NOT NULL DEFAULT 0 CHECK (no_progress_streak >= 0);
ALTER TABLE chat_runs ADD COLUMN max_no_progress_rounds INTEGER NOT NULL DEFAULT 2 CHECK (max_no_progress_rounds > 0);
ALTER TABLE chat_runs ADD COLUMN last_cycle_fingerprint TEXT NOT NULL DEFAULT '' CHECK (last_cycle_fingerprint = '' OR length(last_cycle_fingerprint) = 64);
ALTER TABLE chat_runs ADD COLUMN limit_reason TEXT NOT NULL DEFAULT '' CHECK (limit_reason IN ('', 'no_progress', 'model_request_limit', 'token_limit', 'active_duration_limit'));
ALTER TABLE chat_runs ADD COLUMN finalization_attempted_at DATETIME;

UPDATE chat_runs SET model_request_count = step_count;
