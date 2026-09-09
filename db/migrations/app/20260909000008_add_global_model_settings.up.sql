CREATE TABLE global_model_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    singleton INTEGER NOT NULL UNIQUE DEFAULT 1 CHECK (singleton = 1),
    settings TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(settings)),
    revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

INSERT INTO global_model_settings (created_at, updated_at)
VALUES (CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
