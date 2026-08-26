# Comic Section API

使用 `request_api` 读取单个漫画段落：

`GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}`

将占位符替换为公开 UUIDv7，不传 `query` 或 `request_body`。使用：

`.data | {uuid,chapter_uuid,section_no,title,description_md,current_storyboard,revision}`

返回的 `revision` 用于后续修改、删除、导入或选择版本。段落集合操作见 `/api/v1/agent-docs/api/comic.md`，分镜版本操作见 `/api/v1/agent-docs/api/storyboard.md`。
