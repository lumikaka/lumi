-- River owns its schema. These columns belong only to Lumi's execution owners.
-- Existing bindings are reconciled against River before workers start; do not
-- infer execution termination from a business status in this migration.
ALTER TABLE task_runs ADD COLUMN execution_uuid TEXT;
ALTER TABLE production_task_runs ADD COLUMN execution_uuid TEXT;
ALTER TABLE asset_maintenance_runs ADD COLUMN execution_uuid TEXT;
ALTER TABLE chat_turns ADD COLUMN execution_uuid TEXT;
ALTER TABLE workflow_steps ADD COLUMN execution_uuid TEXT;
ALTER TABLE workflow_awaits ADD COLUMN execution_uuid TEXT;
