# Storyboard API

使用 `request_api` 调用。将占位符替换为公开 UUIDv7；写入或选择前先读取 Section 的最新 `revision`，冲突后重新读取。

## 版本列表

`GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/storyboard-variants`

使用 `.data.items[] | {uuid,version_no,content_md,source_type,created_at}`。需要当前 Section revision 时，先读取 `/api/v1/agent-docs/api/comic-section.md` 中的单项接口。

## 创建新版本

`POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/storyboard-variants`

- `content_md` 是完整 Storyboard Markdown，不是增量修改。
- `request_body`：`{"content_md":"完整分镜 Markdown","expected_revision":3}`。
- 返回更新后的 Section；使用 `.data | {uuid,chapter_uuid,section_no,page_role,title,current_storyboard,revision}`。

## 选择历史版本

`POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/storyboard-variants/{variant_uuid}/selections`

- `request_body`：`{"expected_revision":3}`。
- 返回更新后的 Section，过滤器同上。
