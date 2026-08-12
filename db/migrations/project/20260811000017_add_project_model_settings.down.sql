ALTER TABLE workflows DROP COLUMN model_source;
ALTER TABLE chat_runs DROP COLUMN model_source;
ALTER TABLE chat_threads DROP COLUMN model_source;
ALTER TABLE production_task_runs DROP COLUMN model_source;
ALTER TABLE task_runs DROP COLUMN model_source;

DROP TABLE project_model_settings;
