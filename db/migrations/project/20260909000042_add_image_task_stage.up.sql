ALTER TABLE production_task_runs ADD COLUMN stage TEXT NOT NULL DEFAULT '';
ALTER TABLE production_task_runs ADD COLUMN stage_started_at DATETIME;
