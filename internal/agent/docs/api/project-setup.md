# Project Setup API

仅当当前项目的 `setup_status` 为 `draft` 时，使用本 API 从首页原始输入整理 Setup Draft（初始化草稿/待确认设置）。Setup Draft 不是正式项目事实；必须经过用户明确确认并完成 finalization，项目才会变为 `ready`。此流程不得创建、选择或切换 Candidate。

- `GET /api/v1/projects/{project_uuid}/project-setup`
  - 不传 `query` 或 `request_body`。
  - 使用 `.data | {uuid,project_uuid,setup_status,status,revision,draft_values,field_sources,missing_information,final_picture_book,updated_at}`。
  - `draft_values` 包含 `project_name`、`generation_language`、`overall_style` 与 `picture_book`。
  - `field_sources` 的值为 `system_default`、`agent_proposed` 或 `user_confirmed`；不得把默认值描述成用户选择。
- `PATCH /api/v1/projects/{project_uuid}/project-setup`
  - `request_body.expected_revision` 必须来自刚读取的设置。
  - 至少提交一个草稿字段：`project_name`、`generation_language`、`overall_style`、`picture_book`。
  - `picture_book.format` 支持 `classic_picture_book`、`wordless_picture_book`、`interactive_picture_book`、`comic_story`、`vertical_strip`。
  - 不确定的信息应向用户询问，不要臆造具体人物、剧情或风格偏好。
- `POST /api/v1/projects/{project_uuid}/project-setup-finalizations`
  - `request_body` 只包含 `expected_revision`。
  - 这是危险操作：先向用户清楚展示全部待确认设置、默认来源和缺失信息，再按确认协议请求一次明确确认。
  - 定稿会一次性创建唯一且不可修改的正式绘本规格，并将项目切换为 `ready`。
  - 版本冲突后重新 GET，不要盲目重试。

只使用公开 `uuid`，不要发送内部 `id`。
