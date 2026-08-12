CREATE TABLE recent_projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    root_path TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    last_opened_at DATETIME NOT NULL
);

CREATE INDEX recent_projects_last_opened_at_index
    ON recent_projects(last_opened_at DESC);
