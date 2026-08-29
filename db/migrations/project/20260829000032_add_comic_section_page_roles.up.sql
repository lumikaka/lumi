ALTER TABLE comic_sections
    ADD COLUMN page_role TEXT NOT NULL DEFAULT 'body'
    CHECK (page_role IN ('front_cover', 'body', 'back_cover'));

CREATE UNIQUE INDEX comic_sections_active_special_role_unique
    ON comic_sections(chapter_comic_state_id, page_role)
    WHERE deleted_at IS NULL
      AND page_role IN ('front_cover', 'back_cover');
