PRAGMA defer_foreign_keys = ON;

-- Abort before changing schema whenever the unified model cannot be represented
-- by the historical single-subject / four-file contract.
CREATE TEMP TABLE chat_reference_down_guard (
    ok INTEGER NOT NULL CHECK (ok=1)
);
INSERT INTO chat_reference_down_guard(ok)
SELECT 0 WHERE EXISTS (
    SELECT 1
    FROM chat_context_references
    WHERE resource_type='file'
    GROUP BY COALESCE(chat_item_id,-follow_up_id)
    HAVING COUNT(*)>4
);
INSERT INTO chat_reference_down_guard(ok)
SELECT 0 WHERE EXISTS (
    SELECT 1
    FROM chat_context_references
    WHERE resource_type='file' AND file_id IS NULL
);
INSERT INTO chat_reference_down_guard(ok)
SELECT 0 WHERE EXISTS (
    SELECT 1
    FROM chat_context_references
    WHERE resource_type<>'file'
    GROUP BY COALESCE(chat_item_id,-follow_up_id)
    HAVING COUNT(*)>1
);
INSERT INTO chat_reference_down_guard(ok)
SELECT 0 WHERE EXISTS (
    SELECT 1 FROM (
        SELECT items.thread_id,refs.resource_type,refs.resource_uuid
        FROM chat_context_references AS refs
        JOIN chat_items AS items ON items.id=refs.chat_item_id
        WHERE refs.resource_type<>'file'
        UNION ALL
        SELECT follow_ups.thread_id,refs.resource_type,refs.resource_uuid
        FROM chat_context_references AS refs
        JOIN chat_follow_ups AS follow_ups ON follow_ups.id=refs.follow_up_id
        WHERE refs.resource_type<>'file'
    ) AS domain_refs
    GROUP BY domain_refs.thread_id
    HAVING COUNT(DISTINCT domain_refs.resource_type || ':' || domain_refs.resource_uuid)>1
);
WITH domain_threads AS (
    SELECT DISTINCT items.thread_id,refs.resource_type,refs.resource_uuid
    FROM chat_context_references AS refs
    JOIN chat_items AS items ON items.id=refs.chat_item_id
    WHERE refs.resource_type<>'file'
    UNION
    SELECT DISTINCT follow_ups.thread_id,refs.resource_type,refs.resource_uuid
    FROM chat_context_references AS refs
    JOIN chat_follow_ups AS follow_ups ON follow_ups.id=refs.follow_up_id
    WHERE refs.resource_type<>'file'
), input_owners AS (
    SELECT items.thread_id,'item' AS owner_type,items.id AS owner_id
    FROM chat_items AS items
    WHERE items.item_type='user_message'
    UNION ALL
    SELECT follow_ups.thread_id,'follow_up' AS owner_type,follow_ups.id AS owner_id
    FROM chat_follow_ups AS follow_ups
    WHERE follow_ups.status='queued' AND follow_ups.deleted_at IS NULL
)
INSERT INTO chat_reference_down_guard(ok)
SELECT 0 WHERE EXISTS (
    SELECT 1
    FROM input_owners AS owners
    JOIN domain_threads AS domain ON domain.thread_id=owners.thread_id
    WHERE NOT EXISTS (
        SELECT 1
        FROM chat_context_references AS refs
        WHERE refs.resource_type=domain.resource_type
          AND refs.resource_uuid=domain.resource_uuid
          AND (
              (owners.owner_type='item' AND refs.chat_item_id=owners.owner_id) OR
              (owners.owner_type='follow_up' AND refs.follow_up_id=owners.owner_id)
          )
    )
);
DROP TABLE chat_reference_down_guard;

ALTER TABLE chat_threads ADD COLUMN scope TEXT NOT NULL DEFAULT 'project'
    CHECK (scope IN ('project','premise'));
ALTER TABLE chat_threads ADD COLUMN scene TEXT NOT NULL DEFAULT ''
    CHECK (scene IN ('','premise_asset_generation','asset_reference'));
ALTER TABLE chat_threads ADD COLUMN subject_uuid TEXT NOT NULL DEFAULT ''
    CHECK (subject_uuid='' OR length(subject_uuid)=36);

UPDATE chat_threads
SET scope='premise', scene='asset_reference', subject_uuid=(
    SELECT candidates.resource_uuid
    FROM (
        SELECT refs.resource_uuid,items.sequence AS owner_sequence,refs.position,refs.id
        FROM chat_context_references AS refs
        JOIN chat_items AS items ON items.id=refs.chat_item_id
        WHERE items.thread_id=chat_threads.id AND refs.resource_type='premise_asset'
        UNION ALL
        SELECT refs.resource_uuid,follow_ups.position AS owner_sequence,refs.position,refs.id
        FROM chat_context_references AS refs
        JOIN chat_follow_ups AS follow_ups ON follow_ups.id=refs.follow_up_id
        WHERE follow_ups.thread_id=chat_threads.id AND refs.resource_type='premise_asset'
    ) AS candidates
    ORDER BY candidates.owner_sequence,candidates.position,candidates.id LIMIT 1
)
WHERE EXISTS (
    SELECT 1
    FROM chat_context_references AS refs
    LEFT JOIN chat_items AS items ON items.id=refs.chat_item_id
    LEFT JOIN chat_follow_ups AS follow_ups ON follow_ups.id=refs.follow_up_id
    WHERE COALESCE(items.thread_id,follow_ups.thread_id)=chat_threads.id AND refs.resource_type='premise_asset'
);

UPDATE chat_threads
SET scope='project', scene='asset_reference', subject_uuid=(
    SELECT candidates.resource_uuid
    FROM (
        SELECT refs.resource_uuid,items.sequence AS owner_sequence,refs.position,refs.id
        FROM chat_context_references AS refs
        JOIN chat_items AS items ON items.id=refs.chat_item_id
        WHERE items.thread_id=chat_threads.id AND refs.resource_type='comic_section'
        UNION ALL
        SELECT refs.resource_uuid,follow_ups.position AS owner_sequence,refs.position,refs.id
        FROM chat_context_references AS refs
        JOIN chat_follow_ups AS follow_ups ON follow_ups.id=refs.follow_up_id
        WHERE follow_ups.thread_id=chat_threads.id AND refs.resource_type='comic_section'
    ) AS candidates
    ORDER BY candidates.owner_sequence,candidates.position,candidates.id LIMIT 1
)
WHERE EXISTS (
    SELECT 1
    FROM chat_context_references AS refs
    LEFT JOIN chat_items AS items ON items.id=refs.chat_item_id
    LEFT JOIN chat_follow_ups AS follow_ups ON follow_ups.id=refs.follow_up_id
    WHERE COALESCE(items.thread_id,follow_ups.thread_id)=chat_threads.id AND refs.resource_type='comic_section'
);

CREATE INDEX chat_threads_project_scope_updated_index
    ON chat_threads(project_id,scope,archived_at,updated_at DESC,id DESC);

CREATE TABLE chat_item_file_references (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    chat_item_id INTEGER NOT NULL,
    file_id INTEGER NOT NULL,
    upload_stashed_id INTEGER,
    position INTEGER NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT chat_item_file_references_item_fk FOREIGN KEY (chat_item_id) REFERENCES chat_items(id) ON DELETE CASCADE,
    CONSTRAINT chat_item_file_references_file_fk FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE RESTRICT,
    CONSTRAINT chat_item_file_references_upload_fk FOREIGN KEY (upload_stashed_id) REFERENCES upload_stashed(id) ON DELETE SET NULL,
    CONSTRAINT chat_item_file_references_position_check CHECK (position BETWEEN 1 AND 4),
    UNIQUE(chat_item_id,position)
);
CREATE INDEX chat_item_file_references_item_position_index ON chat_item_file_references(chat_item_id,position,id);
CREATE INDEX chat_item_file_references_file_index ON chat_item_file_references(file_id);

CREATE TABLE chat_follow_up_file_references (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    follow_up_id INTEGER NOT NULL,
    file_id INTEGER NOT NULL,
    upload_stashed_id INTEGER,
    position INTEGER NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT chat_follow_up_file_references_follow_up_fk FOREIGN KEY (follow_up_id) REFERENCES chat_follow_ups(id) ON DELETE CASCADE,
    CONSTRAINT chat_follow_up_file_references_file_fk FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE RESTRICT,
    CONSTRAINT chat_follow_up_file_references_upload_fk FOREIGN KEY (upload_stashed_id) REFERENCES upload_stashed(id) ON DELETE SET NULL,
    CONSTRAINT chat_follow_up_file_references_position_check CHECK (position BETWEEN 1 AND 4),
    UNIQUE(follow_up_id,position)
);
CREATE INDEX chat_follow_up_file_references_follow_up_position_index ON chat_follow_up_file_references(follow_up_id,position,id);
CREATE INDEX chat_follow_up_file_references_file_index ON chat_follow_up_file_references(file_id);

INSERT INTO chat_item_file_references(uuid,chat_item_id,file_id,upload_stashed_id,position,created_at)
SELECT
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-7' || substr(lower(hex(randomblob(2))),2) || '-' ||
    substr('89ab',abs(random()) % 4 + 1,1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))),
    refs.chat_item_id,
    refs.file_id,
    (SELECT id FROM upload_stashed WHERE finalized_file_id=refs.file_id ORDER BY id DESC LIMIT 1),
    row_number() OVER (PARTITION BY refs.chat_item_id ORDER BY refs.position,refs.id),
    refs.created_at
FROM chat_context_references AS refs
WHERE refs.chat_item_id IS NOT NULL AND refs.resource_type='file' AND refs.file_id IS NOT NULL;

INSERT INTO chat_follow_up_file_references(uuid,follow_up_id,file_id,upload_stashed_id,position,created_at)
SELECT
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-7' || substr(lower(hex(randomblob(2))),2) || '-' ||
    substr('89ab',abs(random()) % 4 + 1,1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))),
    refs.follow_up_id,
    refs.file_id,
    (SELECT id FROM upload_stashed WHERE finalized_file_id=refs.file_id ORDER BY id DESC LIMIT 1),
    row_number() OVER (PARTITION BY refs.follow_up_id ORDER BY refs.position,refs.id),
    refs.created_at
FROM chat_context_references AS refs
WHERE refs.follow_up_id IS NOT NULL AND refs.resource_type='file' AND refs.file_id IS NOT NULL;

DROP TABLE chat_context_references;
