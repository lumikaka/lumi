# Story Profile API

使用 `request_api` 调用，将 `project_uuid` 替换为当前项目的公开 UUIDv7。写入前先读取最新 `revision`，冲突后重新读取；`story_md` 始终是完整文档，不是增量片段。

## 读取与编辑

- `GET /api/v1/projects/{project_uuid}/story-profile`
  - 使用 `.data | {uuid,revision,story_md,projection_state}`。
- `PUT /api/v1/projects/{project_uuid}/story-profile`
  - `request_body`：`{"story_md":"完整 STORY.md","expected_revision":3}`。
- `GET /api/v1/projects/{project_uuid}/story-profile/versions`
  - 使用 `.data.items[] | {uuid,revision,story_md,projection_state}`。

## 文件同步

以下操作可能覆盖 SQLite 事实状态或项目文件，调用前必须用 `request_user_input` 获得确认。

- `POST /api/v1/projects/{project_uuid}/story-profile/imports`
  - 将项目中的 `STORY.md` 导入 SQLite，覆盖当前 Story Profile。
  - `request_body`：`{"expected_revision":3}`。
- `POST /api/v1/projects/{project_uuid}/story-profile/projection`
  - 用 SQLite 中的当前 Story Profile 重新生成项目 `STORY.md`。
  - `request_body`：`{"expected_revision":3}`。

生成或从章节重建 Story Profile 见 `/api/v1/agent-docs/api/generation.md`。
