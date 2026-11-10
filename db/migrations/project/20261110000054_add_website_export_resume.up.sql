-- Desktop and explicitly keyed generations leave this scope null. Anonymous
-- plugin retries must find their task even if the first reply never arrived.
ALTER TABLE website_generations ADD COLUMN plugin_request_scope TEXT;
ALTER TABLE website_generations ADD COLUMN plugin_result_observed INTEGER NOT NULL DEFAULT 0 CHECK(plugin_result_observed IN (0,1));
CREATE UNIQUE INDEX website_plugin_pending ON website_generations(project_id, plugin_request_scope) WHERE plugin_request_scope IS NOT NULL AND plugin_result_observed=0;
