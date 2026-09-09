CREATE TABLE mcp_oauth_clients (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 uuid TEXT NOT NULL UNIQUE,
 name TEXT NOT NULL,
 redirect_uris TEXT NOT NULL,
 created_at DATETIME NOT NULL
);
CREATE TABLE mcp_oauth_authorizations (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 uuid TEXT NOT NULL UNIQUE,
 client_id INTEGER NOT NULL REFERENCES mcp_oauth_clients(id) ON DELETE CASCADE,
 resource TEXT NOT NULL,
 redirect_uri TEXT NOT NULL,
 challenge TEXT NOT NULL,
 state TEXT NOT NULL,
 scope TEXT NOT NULL,
 recent_project_id INTEGER REFERENCES recent_projects(id) ON DELETE CASCADE,
 permission TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL CHECK(status IN ('pending','approved','denied','used')),
 code_hash TEXT UNIQUE,
 expires_at DATETIME NOT NULL,
 created_at DATETIME NOT NULL
);
CREATE INDEX mcp_oauth_authorizations_expiry ON mcp_oauth_authorizations(expires_at);
ALTER TABLE mcp_grants ADD COLUMN oauth_client_id INTEGER REFERENCES mcp_oauth_clients(id);
ALTER TABLE mcp_grants ADD COLUMN oauth_resource TEXT NOT NULL DEFAULT '';
ALTER TABLE mcp_grants ADD COLUMN expires_at DATETIME;
CREATE TABLE mcp_oauth_refresh_tokens (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 uuid TEXT NOT NULL UNIQUE,
 grant_id INTEGER NOT NULL REFERENCES mcp_grants(id) ON DELETE CASCADE,
 token_hash TEXT NOT NULL UNIQUE,
 expires_at DATETIME NOT NULL,
 used_at DATETIME
);
CREATE INDEX mcp_oauth_refresh_expiry ON mcp_oauth_refresh_tokens(expires_at);
