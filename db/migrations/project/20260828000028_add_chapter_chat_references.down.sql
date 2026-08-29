PRAGMA defer_foreign_keys = ON;

CREATE TEMP TABLE chapter_chat_reference_down_guard (
    ok INTEGER NOT NULL CHECK (ok=1)
);
INSERT INTO chapter_chat_reference_down_guard(ok)
SELECT 0 WHERE EXISTS (
    SELECT 1 FROM chat_context_references WHERE resource_type='chapter'
);
DROP TABLE chapter_chat_reference_down_guard;

CREATE TABLE chat_context_references_previous (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_item_id INTEGER,
    follow_up_id INTEGER,
    position INTEGER NOT NULL,
    resource_type TEXT NOT NULL,
    resource_uuid TEXT NOT NULL,
    snapshot_json TEXT NOT NULL,
    file_id INTEGER,
    premise_asset_id INTEGER,
    comic_section_id INTEGER,
    image_file_id INTEGER,
    created_at DATETIME NOT NULL,
    CONSTRAINT chat_context_references_item_fk FOREIGN KEY (chat_item_id) REFERENCES chat_items(id) ON DELETE CASCADE,
    CONSTRAINT chat_context_references_follow_up_fk FOREIGN KEY (follow_up_id) REFERENCES chat_follow_ups(id) ON DELETE CASCADE,
    CONSTRAINT chat_context_references_file_fk FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE SET NULL,
    CONSTRAINT chat_context_references_premise_asset_fk FOREIGN KEY (premise_asset_id) REFERENCES premise_assets(id) ON DELETE SET NULL,
    CONSTRAINT chat_context_references_comic_section_fk FOREIGN KEY (comic_section_id) REFERENCES comic_sections(id) ON DELETE SET NULL,
    CONSTRAINT chat_context_references_image_file_fk FOREIGN KEY (image_file_id) REFERENCES files(id) ON DELETE RESTRICT,
    CONSTRAINT chat_context_references_owner_check CHECK ((chat_item_id IS NOT NULL) <> (follow_up_id IS NOT NULL)),
    CONSTRAINT chat_context_references_position_check CHECK (position BETWEEN 1 AND 16),
    CONSTRAINT chat_context_references_type_check CHECK (resource_type IN ('file', 'premise_asset', 'comic_section')),
    CONSTRAINT chat_context_references_uuid_check CHECK (length(resource_uuid) = 36),
    CONSTRAINT chat_context_references_snapshot_check CHECK (json_valid(snapshot_json) AND length(CAST(snapshot_json AS BLOB)) <= 8192),
    CONSTRAINT chat_context_references_target_check CHECK (
        (resource_type='file' AND premise_asset_id IS NULL AND comic_section_id IS NULL) OR
        (resource_type='premise_asset' AND file_id IS NULL AND comic_section_id IS NULL) OR
        (resource_type='comic_section' AND file_id IS NULL AND premise_asset_id IS NULL)
    )
);

INSERT INTO chat_context_references_previous (
    id, chat_item_id, follow_up_id, position, resource_type, resource_uuid,
    snapshot_json, file_id, premise_asset_id, comic_section_id, image_file_id, created_at
)
SELECT
    id, chat_item_id, follow_up_id, position, resource_type, resource_uuid,
    snapshot_json, file_id, premise_asset_id, comic_section_id, image_file_id, created_at
FROM chat_context_references;

DROP TABLE chat_context_references;
ALTER TABLE chat_context_references_previous RENAME TO chat_context_references;

CREATE UNIQUE INDEX chat_context_references_item_position_unique
    ON chat_context_references(chat_item_id, position) WHERE chat_item_id IS NOT NULL;
CREATE UNIQUE INDEX chat_context_references_follow_up_position_unique
    ON chat_context_references(follow_up_id, position) WHERE follow_up_id IS NOT NULL;
CREATE UNIQUE INDEX chat_context_references_item_resource_unique
    ON chat_context_references(chat_item_id, resource_type, resource_uuid) WHERE chat_item_id IS NOT NULL;
CREATE UNIQUE INDEX chat_context_references_follow_up_resource_unique
    ON chat_context_references(follow_up_id, resource_type, resource_uuid) WHERE follow_up_id IS NOT NULL;
CREATE INDEX chat_context_references_file_index ON chat_context_references(file_id);
CREATE INDEX chat_context_references_premise_asset_index ON chat_context_references(premise_asset_id);
CREATE INDEX chat_context_references_comic_section_index ON chat_context_references(comic_section_id);
CREATE INDEX chat_context_references_image_file_index ON chat_context_references(image_file_id);
