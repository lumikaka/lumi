# Project API

使用 `request_api` 读取或修改项目公开元数据。

- `GET /api/v1/projects/{project_uuid}`
  - 不传 `query` 或 `request_body`。
  - 使用 `.data | {uuid,name,description,generation_language,revision,chapter_count,trash_count,updated_at}`。
- `PATCH /api/v1/projects/{project_uuid}`
  - 先读取最新 `revision`；冲突后重新读取，不要盲目重试。
  - `request_body` 必须包含完整的 `name`、`description` 和 `expected_revision`；`generation_language` 可选，只能为 `zh-Hans` 或 `en`。
  - 示例：`{"name":"项目名","description":"项目简介","generation_language":"zh-Hans","expected_revision":3}`。

只使用公开 `uuid`，不要发送内部 `id`。
