ALTER TABLE project_model_settings ADD COLUMN project_image_prompt_extend INTEGER CHECK (project_image_prompt_extend IN (0, 1));
