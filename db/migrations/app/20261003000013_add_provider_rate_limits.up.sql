ALTER TABLE global_model_settings
ADD COLUMN provider_rate_limits TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(provider_rate_limits));
