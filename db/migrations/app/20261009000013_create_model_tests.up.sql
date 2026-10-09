CREATE TABLE model_tests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','queued','running','succeeded','failed','interrupted')),
    text_provider_uuid TEXT NOT NULL DEFAULT '',
    text_model TEXT NOT NULL DEFAULT '',
    text_provider_type TEXT NOT NULL DEFAULT '',
    text_provider_name TEXT NOT NULL DEFAULT '',
    image_provider_uuid TEXT NOT NULL DEFAULT '',
    image_model TEXT NOT NULL DEFAULT '',
    image_provider_type TEXT NOT NULL DEFAULT '',
    image_provider_name TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    started_at DATETIME,
    completed_at DATETIME
);
CREATE INDEX model_tests_history ON model_tests(created_at DESC, id DESC);
CREATE INDEX model_tests_queue ON model_tests(status, updated_at, id);

CREATE TABLE model_test_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    model_test_id INTEGER NOT NULL REFERENCES model_tests(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position BETWEEN 1 AND 3),
    kind TEXT NOT NULL CHECK (kind IN ('connection','text','image')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','succeeded','failed','skipped','interrupted')),
    started_at DATETIME,
    completed_at DATETIME,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    output_text TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    http_status INTEGER NOT NULL DEFAULT 0,
    provider_error_code TEXT NOT NULL DEFAULT '',
    provider_request_id TEXT NOT NULL DEFAULT '',
    image_mime_type TEXT NOT NULL DEFAULT '',
    image_width INTEGER NOT NULL DEFAULT 0,
    image_height INTEGER NOT NULL DEFAULT 0,
    UNIQUE(model_test_id, position),
    UNIQUE(model_test_id, kind)
);
