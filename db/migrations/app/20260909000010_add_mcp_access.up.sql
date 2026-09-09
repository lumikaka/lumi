CREATE TABLE mcp_grants (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 uuid TEXT NOT NULL UNIQUE,
 recent_project_id INTEGER NOT NULL REFERENCES recent_projects(id) ON DELETE CASCADE,
 name TEXT NOT NULL,
 permission TEXT NOT NULL CHECK(permission IN ('read','edit')),
 token_hash TEXT NOT NULL UNIQUE,
 token_prefix TEXT NOT NULL,
 created_at DATETIME NOT NULL,
 revoked_at DATETIME,
 last_used_at DATETIME
);
CREATE TABLE mcp_calls (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 uuid TEXT NOT NULL UNIQUE,
 grant_id INTEGER NOT NULL REFERENCES mcp_grants(id) ON DELETE CASCADE,
 idempotency_key TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 arguments TEXT NOT NULL,
 action TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending_confirmation','executing','succeeded','failed','rejected','expired','interrupted')),
 async BOOLEAN NOT NULL DEFAULT 0,
 result TEXT NOT NULL DEFAULT '',
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 expires_at DATETIME NOT NULL,
 UNIQUE(grant_id,idempotency_key)
);
CREATE INDEX mcp_calls_grant_status ON mcp_calls(grant_id,status);
