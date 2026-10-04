ALTER TABLE global_model_settings
ADD COLUMN rate_limits TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(rate_limits));
