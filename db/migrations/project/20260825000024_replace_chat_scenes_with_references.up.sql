PRAGMA defer_foreign_keys = ON;

CREATE TABLE chat_context_references (
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

-- A legacy bound subject becomes the first Reference on every historical user
-- item. Missing resources remain useful as an immutable unavailable snapshot.
INSERT INTO chat_context_references (
    chat_item_id, position, resource_type, resource_uuid, snapshot_json,
    premise_asset_id, image_file_id, created_at
)
SELECT
    items.id,
    1,
    'premise_asset',
    threads.subject_uuid,
    CASE WHEN assets.id IS NULL THEN
        json_object('resource_type','premise_asset','resource_uuid',threads.subject_uuid,'status','unavailable','truncated_fields',json('[]'))
    ELSE
        json_object(
            'resource_type','premise_asset',
            'resource_uuid',assets.uuid,
            'status','available',
            'asset_type',assets.asset_type,
            'title',substr(assets.title,1,256),
            'summary',substr(assets.summary,1,1000),
            'tags',json(COALESCE((SELECT json_group_array(tag) FROM (SELECT substr(tag,1,32) AS tag FROM premise_asset_tags WHERE premise_asset_id=assets.id ORDER BY tag LIMIT 8)),'[]')),
            'revision',assets.revision,
            'current_variant_uuid',COALESCE(variants.uuid,''),
            'current_file_uuid',COALESCE(files.uuid,''),
            'truncated_fields',json(COALESCE((
                SELECT json_group_array(field)
                FROM (
                    SELECT 'summary' AS field WHERE length(assets.summary)>1000
                    UNION ALL SELECT 'title' WHERE length(assets.title)>256
                    UNION ALL SELECT 'tags' WHERE (SELECT COUNT(*) FROM premise_asset_tags WHERE premise_asset_id=assets.id)>8
                )
            ),'[]'))
        )
    END,
    assets.id,
    files.id,
    items.created_at
FROM chat_items AS items
JOIN chat_threads AS threads ON threads.id=items.thread_id
LEFT JOIN premise_assets AS assets
    ON assets.project_id=threads.project_id AND assets.uuid=threads.subject_uuid
LEFT JOIN premise_asset_variants AS variants ON variants.id=assets.current_variant_id
LEFT JOIN files ON files.id=variants.file_id
WHERE items.item_type='user_message'
  AND threads.scope='premise'
  AND threads.scene='asset_reference'
  AND threads.subject_uuid<>'';

INSERT INTO chat_context_references (
    chat_item_id, position, resource_type, resource_uuid, snapshot_json,
    comic_section_id, image_file_id, created_at
)
SELECT
    items.id,
    1,
    'comic_section',
    threads.subject_uuid,
    CASE WHEN sections.id IS NULL THEN
        json_object('resource_type','comic_section','resource_uuid',threads.subject_uuid,'status','unavailable','truncated_fields',json('[]'))
    ELSE
        json_object(
            'resource_type','comic_section',
            'resource_uuid',sections.uuid,
            'status','available',
            'chapter_uuid',chapters.uuid,
            'section_no',sections.section_no,
            'title',substr(sections.title,1,256),
            'description',substr(sections.description_md,1,1000),
            'revision',sections.revision,
            'current_storyboard_uuid',COALESCE(storyboards.uuid,''),
            'current_image_file_uuid',COALESCE(files.uuid,''),
            'truncated_fields',json(COALESCE((
                SELECT json_group_array(field)
                FROM (
                    SELECT 'description' AS field WHERE length(sections.description_md)>1000
                    UNION ALL SELECT 'title' WHERE length(sections.title)>256
                )
            ),'[]'))
        )
    END,
    sections.id,
    files.id,
    items.created_at
FROM chat_items AS items
JOIN chat_threads AS threads ON threads.id=items.thread_id
LEFT JOIN comic_sections AS sections
    ON sections.uuid=threads.subject_uuid
   AND EXISTS (
       SELECT 1
       FROM chapter_comic_states AS owning_states
       JOIN chapters AS owning_chapters ON owning_chapters.id=owning_states.chapter_id
       WHERE owning_states.id=sections.chapter_comic_state_id
         AND owning_chapters.project_id=threads.project_id
   )
LEFT JOIN chapter_comic_states AS states ON states.id=sections.chapter_comic_state_id
LEFT JOIN chapters ON chapters.id=states.chapter_id AND chapters.project_id=threads.project_id
LEFT JOIN comic_storyboard_variants AS storyboards ON storyboards.id=sections.current_storyboard_variant_id
LEFT JOIN comic_image_variants AS images ON images.id=sections.current_image_variant_id
LEFT JOIN files ON files.id=images.file_id
WHERE items.item_type='user_message'
  AND threads.scope='project'
  AND threads.scene='asset_reference'
  AND threads.subject_uuid<>'';

-- Existing uploaded image attachments become ordinary File References. A
-- migrated bound subject, when present, has already occupied position one.
WITH ranked AS (
    SELECT
        old.*,
        row_number() OVER (
            PARTITION BY old.chat_item_id, old.file_id
            ORDER BY old.position, old.id
        ) AS duplicate_rank
    FROM chat_item_file_references AS old
), deduplicated AS (
    SELECT
        ranked.*,
        row_number() OVER (
            PARTITION BY ranked.chat_item_id
            ORDER BY ranked.position, ranked.id
        ) AS compact_position
    FROM ranked
    WHERE ranked.duplicate_rank=1
)
INSERT INTO chat_context_references (
    chat_item_id, position, resource_type, resource_uuid, snapshot_json,
    file_id, image_file_id, created_at
)
SELECT
    old.chat_item_id,
    old.compact_position + CASE WHEN threads.subject_uuid<>'' AND threads.scene='asset_reference' THEN 1 ELSE 0 END,
    'file',
    files.uuid,
    json_object(
        'resource_type','file',
        'resource_uuid',files.uuid,
        'status','available',
        'name',COALESCE(files.display_name,files.original_filename,''),
        'original_filename',COALESCE(files.original_filename,''),
        'mime_type',objects.mime_type,
        'byte_size',objects.byte_size,
        'width',objects.width,
        'height',objects.height,
        'truncated_fields',json('[]')
    ),
    files.id,
    files.id,
    old.created_at
FROM deduplicated AS old
JOIN chat_items AS items ON items.id=old.chat_item_id
JOIN chat_threads AS threads ON threads.id=items.thread_id
JOIN files ON files.id=old.file_id
JOIN file_objects AS objects ON objects.id=files.file_object_id;

WITH ranked AS (
    SELECT
        old.*,
        row_number() OVER (
            PARTITION BY old.follow_up_id, old.file_id
            ORDER BY old.position, old.id
        ) AS duplicate_rank
    FROM chat_follow_up_file_references AS old
), deduplicated AS (
    SELECT
        ranked.*,
        row_number() OVER (
            PARTITION BY ranked.follow_up_id
            ORDER BY ranked.position, ranked.id
        ) AS compact_position
    FROM ranked
    WHERE ranked.duplicate_rank=1
)
INSERT INTO chat_context_references (
    follow_up_id, position, resource_type, resource_uuid, snapshot_json,
    file_id, image_file_id, created_at
)
SELECT
    old.follow_up_id,
    old.compact_position,
    'file',
    files.uuid,
    json_object(
        'resource_type','file',
        'resource_uuid',files.uuid,
        'status','available',
        'name',COALESCE(files.display_name,files.original_filename,''),
        'original_filename',COALESCE(files.original_filename,''),
        'mime_type',objects.mime_type,
        'byte_size',objects.byte_size,
        'width',objects.width,
        'height',objects.height,
        'truncated_fields',json('[]')
    ),
    files.id,
    files.id,
    old.created_at
FROM deduplicated AS old
JOIN files ON files.id=old.file_id
JOIN file_objects AS objects ON objects.id=files.file_object_id;

-- Freeze the old thread discriminator on the first user item of each existing
-- run. It is recovery-only data for persisted v2/legacy executions.
UPDATE chat_items
SET metadata_json=json_set(
    metadata_json,
    '$.legacy_thread_context',
    json_object(
        'scope',(SELECT scope FROM chat_threads WHERE id=chat_items.thread_id),
        'scene',(SELECT scene FROM chat_threads WHERE id=chat_items.thread_id),
        'subject_uuid',(SELECT subject_uuid FROM chat_threads WHERE id=chat_items.thread_id)
    )
)
WHERE item_type='user_message'
  AND run_id IS NOT NULL
  AND id=(
      SELECT first_item.id
      FROM chat_items AS first_item
      WHERE first_item.run_id=chat_items.run_id AND first_item.item_type='user_message'
      ORDER BY first_item.sequence,first_item.id
      LIMIT 1
  );

DROP TABLE chat_follow_up_file_references;
DROP TABLE chat_item_file_references;

DROP INDEX IF EXISTS chat_threads_project_scope_updated_index;
ALTER TABLE chat_threads DROP COLUMN subject_uuid;
ALTER TABLE chat_threads DROP COLUMN scene;
ALTER TABLE chat_threads DROP COLUMN scope;
