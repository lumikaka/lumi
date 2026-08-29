# Comic Snapshot API

文中“获得确认”均指：先提交参数完整的 `request_api`；若返回 `agent_tool_confirmation_required`，该次不会执行写操作，此时再按 Overview 的全局协议把 confirmation 只传给 `request_user_input`；确认后由运行时自动执行原请求。

使用 `request_api` 调用。将占位符替换为公开 UUIDv7；每次调用都传 `response_filter`。

## 读取

- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-snapshots`
  - 使用 `.data.items[] | {uuid,version_no,reason,source,section_count,created_at}`。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-snapshots/{snapshot_uuid}`
  - 使用 `.data | {uuid,version_no,reason,source,section_count,schema_version,created_at}`。

需要比较快照内容时，可在详情过滤器中追加 `chapter` 和 `sections`。
`sections` 中的每项都包含 `page_role`，用于区分封面、正文页和封底。

## 恢复

`POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-snapshots/{snapshot_uuid}/restorations`

- 恢复会用快照整体替换章节当前漫画状态，调用前必须用 `request_user_input` 获得确认。
- 普通绘本的目标快照必须包含至少一个 active `body`；服务端拒绝恢复空快照或只有封面/封底的快照。`vertical_strip` 仍可恢复为空序列，但其中的页面只能是 `body`。
- `request_body` 必须传空对象：`{}`。
- 使用 `.data.items[] | {uuid,chapter_uuid,section_no,page_role,title,revision}` 读取恢复后的 Section 列表。
