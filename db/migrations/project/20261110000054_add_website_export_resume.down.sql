DROP INDEX website_plugin_pending;
ALTER TABLE website_generations DROP COLUMN plugin_result_observed;
ALTER TABLE website_generations DROP COLUMN plugin_request_scope;
