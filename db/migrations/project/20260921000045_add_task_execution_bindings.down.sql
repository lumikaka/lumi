ALTER TABLE workflow_awaits DROP COLUMN execution_uuid;
ALTER TABLE workflow_steps DROP COLUMN execution_uuid;
ALTER TABLE chat_turns DROP COLUMN execution_uuid;
ALTER TABLE asset_maintenance_runs DROP COLUMN execution_uuid;
ALTER TABLE production_task_runs DROP COLUMN execution_uuid;
ALTER TABLE task_runs DROP COLUMN execution_uuid;
