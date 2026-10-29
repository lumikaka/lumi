CREATE TABLE project_initialization_handoffs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    workflow_id INTEGER NOT NULL UNIQUE REFERENCES workflows(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL
);
