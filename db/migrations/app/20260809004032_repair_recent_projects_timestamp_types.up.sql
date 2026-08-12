CREATE TABLE recent_projects_migrated (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    root_path TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    last_opened_at DATETIME NOT NULL
);

INSERT INTO recent_projects_migrated (
    id,
    uuid,
    name,
    root_path,
    created_at,
    updated_at,
    last_opened_at
)
SELECT
    id,
    uuid,
    name,
    root_path,
    created_at,
    updated_at,
    last_opened_at
FROM recent_projects;

DROP TABLE recent_projects;

ALTER TABLE recent_projects_migrated RENAME TO recent_projects;

CREATE INDEX recent_projects_last_opened_at_index
    ON recent_projects(last_opened_at DESC);
