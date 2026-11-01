CREATE TABLE host_setting_boards (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_id INTEGER NOT NULL UNIQUE REFERENCES premise_sources(id),
    setting_image_id INTEGER NOT NULL UNIQUE REFERENCES premise_setting_images(id),
    idempotency_key TEXT NOT NULL,
    input_hash TEXT NOT NULL,
    revision INTEGER NOT NULL DEFAULT 0 CHECK(revision >= 0),
    created_at DATETIME NOT NULL,
    UNIQUE(project_id, idempotency_key)
);

CREATE TABLE host_setting_breakdowns (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    board_id INTEGER NOT NULL REFERENCES host_setting_boards(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    input_hash TEXT NOT NULL,
    input_json TEXT NOT NULL CHECK(json_valid(input_json)),
    output_json TEXT NOT NULL CHECK(json_valid(output_json)),
    created_at DATETIME NOT NULL,
    UNIQUE(board_id, idempotency_key)
);
