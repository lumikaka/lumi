ALTER TABLE llm_logs ADD COLUMN price_snapshot TEXT CHECK(price_snapshot IS NULL OR json_valid(price_snapshot));
ALTER TABLE llm_logs ADD COLUMN billing_usage TEXT CHECK(billing_usage IS NULL OR json_valid(billing_usage));
ALTER TABLE llm_logs ADD COLUMN cost_status TEXT NOT NULL DEFAULT 'unpriced' CHECK(cost_status IN ('pending','calculated','unpriced'));
ALTER TABLE llm_logs ADD COLUMN cost_reason TEXT NOT NULL DEFAULT 'legacy_usage';
ALTER TABLE llm_logs ADD COLUMN cost_origin TEXT NOT NULL DEFAULT '';
ALTER TABLE llm_logs ADD COLUMN cost_currency TEXT NOT NULL DEFAULT '';
ALTER TABLE llm_logs ADD COLUMN cost_nanos INTEGER CHECK(cost_nanos IS NULL OR cost_nanos>=0);
ALTER TABLE llm_logs ADD COLUMN cost_breakdown TEXT CHECK(cost_breakdown IS NULL OR json_valid(cost_breakdown));
UPDATE llm_logs SET cost_status='pending',cost_reason='' WHERE status='pending';
CREATE INDEX llm_logs_cost_index ON llm_logs(project_id,cost_status,created_at,id);
CREATE TABLE llm_cost_backfills (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 uuid TEXT NOT NULL UNIQUE CHECK(length(uuid)=36),
 project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 created_at DATETIME NOT NULL
);
CREATE TABLE llm_cost_backfill_items (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 backfill_id INTEGER NOT NULL REFERENCES llm_cost_backfills(id) ON DELETE CASCADE,
 llm_log_id INTEGER NOT NULL REFERENCES llm_logs(id) ON DELETE CASCADE,
 estimate_json TEXT NOT NULL CHECK(json_valid(estimate_json)),
 applied INTEGER NOT NULL DEFAULT 0 CHECK(applied IN (0,1)),
 UNIQUE(backfill_id,llm_log_id)
);
