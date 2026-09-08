ALTER TABLE project_model_settings ADD COLUMN project_image_enable_thinking INTEGER CHECK (project_image_enable_thinking IN (0, 1));
