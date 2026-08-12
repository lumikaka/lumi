CREATE TABLE site_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key TEXT NOT NULL UNIQUE,
    value TEXT NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT site_settings_key_check CHECK (length(trim(key)) BETWEEN 1 AND 255),
    CONSTRAINT site_settings_value_check CHECK (json_valid(value))
);

CREATE UNIQUE INDEX site_settings_key_index ON site_settings(key);

DROP TABLE ai_providers;
