ALTER TABLE workflow_steps DROP COLUMN current_wait_id;

DROP TABLE workflow_step_wait_dependencies;
DROP TABLE workflow_step_waits;
