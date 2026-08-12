ALTER TABLE comic_image_generations
    ADD COLUMN premise_file_id INTEGER REFERENCES files(id) ON DELETE SET NULL;

ALTER TABLE comic_image_generations
    ADD COLUMN premise_metadata TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(premise_metadata));

CREATE INDEX comic_generations_premise_file_index
    ON comic_image_generations(premise_file_id);
