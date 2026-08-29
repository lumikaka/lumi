# Comic API

文中“获得确认”均指：先提交参数完整的 `request_api`；若返回 `agent_tool_confirmation_required`，该次不会执行写操作，此时再按 Overview 的全局协议把 confirmation 只传给 `request_user_input`；确认后由运行时自动执行原请求。

使用 `request_api` 调用。将占位符替换为公开 UUIDv7；修改前先读取目标 Section 的最新 `revision`，冲突后重新读取。

## 状态与段落

- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic`
  - 使用 `.data | {uuid,chapter_uuid,status,has_premise_assets,premise_asset_count,revision,updated_at}`。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections`
  - 使用 `.data.items[] | {uuid,chapter_uuid,section_no,title,description_md,revision}`。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections`
  - `request_body`：`title` 必填；`description_md`、`storyboard_md` 可选。
  - 示例：`{"title":"雨夜相遇","description_md":"场景与动作说明","storyboard_md":"完整分镜 Markdown"}`。
- `PATCH /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}`
  - `request_body`：`expected_revision` 必填；按需传 `title`、`description_md`。
  - 示例：`{"title":"新标题","expected_revision":3}`。
- `PUT /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-section-order`
  - 会整体重排，调用前必须获得确认。
  - `request_body`：`{"section_uuids":["<section_uuid_1>","<section_uuid_2>"]}`；数组必须包含排序后的完整 Section UUID 列表，数量为 1–200。
- `DELETE /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}`
  - 调用前必须获得确认。
  - `request_body`：`{"expected_revision":3}`。

单个 Section 的完整读取见 `/api/v1/agent-docs/api/comic-section.md`。

## 页面图片版本

- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/images`
  - 导入当前项目尚未消费的 ready upload。
  - `request_body`：`{"upload_uuid":"<upload_uuid>","expected_revision":3}`。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-variants`
  - 使用 `.data.items[] | {uuid,version_no,source_type,generation_uuid,asset,created_at}`。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-variants/{variant_uuid}/selections`
  - `request_body`：`{"expected_revision":3}`。

写入和选择操作返回更新后的 Section；使用 `.data | {uuid,chapter_uuid,section_no,title,current_storyboard,revision}`。
