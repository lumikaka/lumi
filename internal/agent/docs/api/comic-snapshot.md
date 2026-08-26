# Comic Snapshot API

使用 `request_api` 调用。将占位符替换为公开 UUIDv7；每次调用都传 `response_filter`。

## 读取

- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-snapshots`
  - 使用 `.data.items[] | {uuid,version_no,reason,source,section_count,created_at}`。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-snapshots/{snapshot_uuid}`
  - 使用 `.data | {uuid,version_no,reason,source,section_count,schema_version,created_at}`。

需要比较快照内容时，可在详情过滤器中追加 `chapter` 和 `sections`。

## 恢复

`POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-snapshots/{snapshot_uuid}/restorations`

- 恢复会用快照整体替换章节当前漫画状态，调用前必须用 `request_user_input` 获得确认。
- `request_body` 必须传空对象：`{}`。
- 使用 `.data.items[] | {uuid,chapter_uuid,section_no,title,revision}` 读取恢复后的 Section 列表。
