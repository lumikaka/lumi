DROP TABLE mcp_oauth_refresh_tokens;
ALTER TABLE mcp_grants DROP COLUMN expires_at;
ALTER TABLE mcp_grants DROP COLUMN oauth_resource;
ALTER TABLE mcp_grants DROP COLUMN oauth_client_id;
DROP TABLE mcp_oauth_authorizations;
DROP TABLE mcp_oauth_clients;
