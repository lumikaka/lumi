CREATE TABLE ai_providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    provider_type TEXT NOT NULL,
    display_name TEXT NOT NULL,
    base_url TEXT NOT NULL,
    default_model TEXT NOT NULL,
    secret_ref TEXT NOT NULL UNIQUE,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT ai_providers_type_check CHECK (provider_type IN ('openai_compatible')),
    CONSTRAINT ai_providers_name_check CHECK (length(trim(display_name)) BETWEEN 1 AND 120),
    CONSTRAINT ai_providers_base_url_check CHECK (length(trim(base_url)) BETWEEN 1 AND 2048),
    CONSTRAINT ai_providers_model_check CHECK (length(trim(default_model)) BETWEEN 1 AND 255),
    CONSTRAINT ai_providers_secret_ref_check CHECK (length(trim(secret_ref)) BETWEEN 1 AND 255),
    CONSTRAINT ai_providers_enabled_check CHECK (enabled IN (0, 1))
);

CREATE INDEX ai_providers_enabled_name_index
    ON ai_providers(enabled, display_name, id);
