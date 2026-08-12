DROP INDEX IF EXISTS comic_generations_premise_file_index;

ALTER TABLE comic_image_generations DROP COLUMN premise_metadata;
ALTER TABLE comic_image_generations DROP COLUMN premise_file_id;
