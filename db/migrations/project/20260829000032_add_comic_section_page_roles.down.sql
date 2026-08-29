-- The historical schema cannot represent page roles. Refuse a lossy downgrade
-- when either current/history Sections or durable chapter snapshots contain a
-- cover role. Soft-deleted Sections are history too and must remain protected.
CREATE TEMP TABLE comic_section_page_role_down_guard (
    ok INTEGER NOT NULL CHECK (ok = 1)
);
INSERT INTO comic_section_page_role_down_guard(ok)
SELECT 0 WHERE EXISTS (
    SELECT 1 FROM comic_sections WHERE page_role <> 'body'
);
INSERT INTO comic_section_page_role_down_guard(ok)
SELECT 0 WHERE EXISTS (
    SELECT 1
    FROM comic_chapter_snapshots AS snapshots,
         json_each(snapshots.snapshot_json, '$.sections') AS sections
    WHERE COALESCE(json_extract(sections.value, '$.page_role'), 'body') <> 'body'
);
-- Export snapshot v6 freezes page-role ordering. Even a body-only v6 snapshot
-- is not readable by the pre-page-role renderer, so all durable v6 exports
-- block downgrade rather than only snapshots that happen to contain a cover.
INSERT INTO comic_section_page_role_down_guard(ok)
SELECT 0 WHERE EXISTS (
    SELECT 1
    FROM comic_exports
    WHERE CAST(COALESCE(json_extract(snapshot_json, '$.version'), 0) AS INTEGER) >= 6
);
-- A queued/retry export keeps the v6 ExportSnapshot inside the generic
-- production task's parameters object.
INSERT INTO comic_section_page_role_down_guard(ok)
SELECT 0 WHERE EXISTS (
    SELECT 1
    FROM production_task_runs
    WHERE kind = 'comic_export'
      AND CAST(COALESCE(json_extract(input_snapshot, '$.parameters.version'), 0) AS INTEGER) >= 6
);
-- Comic image v5 freezes page_role and is rejected by the previous worker,
-- including the body-only case.
INSERT INTO comic_section_page_role_down_guard(ok)
SELECT 0 WHERE EXISTS (
    SELECT 1
    FROM production_task_runs
    WHERE kind = 'comic_image_generation'
      AND CAST(COALESCE(json_extract(input_snapshot, '$.version'), 0) AS INTEGER) >= 5
);
-- YOLO v5 creates and generates both the cover and first body page. The old
-- runner only understands v1-v4, so vertical/body-only workflows also guard.
INSERT INTO comic_section_page_role_down_guard(ok)
SELECT 0 WHERE EXISTS (
    SELECT 1
    FROM workflows
    WHERE kind = 'yolo_project_initialization'
      AND CAST(COALESCE(json_extract(input_snapshot, '$.version'), 0) AS INTEGER) >= 5
);
DROP TABLE comic_section_page_role_down_guard;

DROP INDEX IF EXISTS comic_sections_active_special_role_unique;
ALTER TABLE comic_sections DROP COLUMN page_role;
