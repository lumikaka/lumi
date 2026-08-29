# Comic Export API

使用 `request_api` 调用。将占位符替换为公开 UUIDv7；每次调用都传只包含当前所需字段的 `response_filter`。

## 检查与创建

- `GET /api/v1/projects/{project_uuid}/comic-exports/readiness`
  - `query` 必须传 `scope`：`project` 或 `chapter`；`chapter` 范围还必须传 `chapter_uuid`。
  - 示例：`{"scope":"chapter","chapter_uuid":"<chapter_uuid>"}`。
  - 使用 `.data | {scope,chapter_uuid,active_chapter_count,active_section_count,image_section_count,missing_section_count,can_export,complete,missing_sections:{uuid,chapter_uuid,section_no,page_role,body_page_no,title}}`。
  - `section_no` 是绝对装订顺序；`body_page_no` 只为正文页提供从 1 开始的页码。根据 `page_role` 把缺图项准确称为封面、第 N 个正文页或封底。
- `POST /api/v1/projects/{project_uuid}/comic-exports`
  - `request_body`：`scope` 和 `format` 必填；`format` 为 `zip` 或 `pdf`。
  - `scope` 为 `chapter` 时必须传 `chapter_uuid`。
  - `allow_missing_images` 可选；只有用户明确接受缺图导出时才传 `true`。
  - 示例：`{"scope":"chapter","chapter_uuid":"<chapter_uuid>","format":"pdf","allow_missing_images":false}`。
  - 返回 Task；使用 `.data | {uuid,kind,resource_uuid,status,error_code,error_message}`，后续见 `/api/v1/agent-docs/api/task.md`。

先检查 readiness；`can_export` 为 `false` 时不要创建导出任务。

## 查询结果

- `GET /api/v1/projects/{project_uuid}/comic-exports`
  - 可选 `query`：`page`、`per_page`、`scope`、`chapter_uuid`、`task_uuid`、`snapshot_hash`、`format`、`status`。
  - 使用 `.data.items[] | {uuid,task_uuid,scope,chapter_uuid,format,filename,status,expires_at,byte_size,error_code,completed_at}`。
