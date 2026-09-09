# Lumi MCP API 总览

本插件连接一个 OAuth 授权的 Lumi 项目。先调用 `read_agent_doc`，参数为 `{"path":"/api/v1/agent-docs/overview.md"}`，取得 `data.project_uuid`、`data.permission`、`data.external_routes` 和 `data.external_instructions`。

本文件是随插件分发的术语、路由与示例快照，用于选择下一份文档。实际调用前读取对应的实时 API Contract；运行中的 `external_routes` 决定当前允许的 method/path。共享文档也服务于内置 Chat Agent，其中的工具、确认方式与未开放路由不能直接用于外部 MCP。

## 术语表 / 资源名速查

| 术语 | Lumi 中的含义 |
|---|---|
| `project` | OAuth 授权的作品项目。URL 中显式使用 `read_agent_doc` 返回的 `project_uuid`。 |
| `story_profile` | 故事总纲和设定文档，包括完整 `story_md` 及结构化投影状态。 |
| `chapter` | 故事章节，`chapter_code` 是如 `vol01.ch01` 的业务编号，API 定位使用 `chapter_uuid`。 |
| `current_story` / `stories` | 当前章节正文与不可变正文历史；保存完整正文会追加版本并切换当前版本。 |
| `comic` | 章节对应的漫画/绘本制作状态，组织封面、正文页和封底。 |
| `comic_section` | 一个漫画/绘本页面单元；`page_role` 区分 `front_cover`、`body`、`back_cover`。 |
| `storyboard` | 页面分镜 Markdown，保存为 `storyboard-variants`，通过 selections 选择当前版本。 |
| `image_variant` | 页面图片候选版本；选择已有版本不等同于重新生成图片。 |
| `premise` | 项目的画风与设定制作状态。 |
| `premise_source` | 设定来源及生成输入；可据此生成整体设定图。 |
| `setting_image` | 设定图候选；可选择当前图，也可拆解为多个设定资产。 |
| `premise_asset` | 人物、场景、道具或参考图等设定项，具有元数据、标签和图片版本。 |
| `asset` / `file_uuid` | 项目文件对象。Asset API 返回公开元数据；将允许读取的文件 UUID 传给 `read_media` 查看图片。 |
| `comic_snapshot` | 一个章节漫画文档的历史快照，可读取并申请恢复。 |
| `task` | 故事任务，通过 `/tasks` 查询；正文生成被接受后仍需等待任务完成。 |
| `production_task` | 设定图、设定拆解、漫画图片等生产任务，通过 `/production-tasks` 查询。 |
| `call` | 一次 MCP 操作的持久化记录，使用 `get_call` 与 `call_uuid` 恢复结果；与生成 Task 是不同资源。 |
| `revision` | 资源的乐观并发版本；写操作按实时 Contract 提交刚读取的 `expected_revision`。 |
| `trash` | 资源回收站状态。章节与设定项列表通过 `query.state=trashed` 读取，恢复使用 `/restorations`。 |

所有外部标识均使用 UUIDv7，字段名为 `uuid` 或语义化的 `*_uuid`；不传内部 `id`、`project_id` 或本地路径。VACS 的 `/api/mcp/current-project/...` 和 `read_api_doc` 不是 Lumi 接口。

## 工具结果与 response_filter

- `read_agent_doc`：只接收 `path`；文档正文在 `data.content`，授权与外部路由信息位于同一 `data` 对象。
- `request_api`：必填 `method`、`url`、`response_filter`，可选 `query` 和 `request_body`；写操作另需顶层 `idempotency_key`。
- `get_call`：接收 `call_uuid`，读取本授权下已持久化的操作状态及结果。
- `read_media`：接收 `file_uuid`，返回支持的图片内容，最多 4 MiB；不要从桌面 Cookie URL 或本地文件绕过授权读取。

业务信封使用 `{success,data,error?}`。`response_filter` 针对业务响应的 `.data` 执行，不针对 MCP 外层的 `.data.result`；筛选后的值包装在工具结果 `data.result.data`，`data.result.success` 表示业务结果，`data.call` 保存操作状态。`get_call` 用同样的结构恢复结果。不要把 `call.status=succeeded` 理解成异步生成完成。

优先只读取 UUID、名称、状态和 revision，需要正文或图片时再取详情。示例投影：

- `.data | {uuid,name,description,revision}`：项目摘要。
- `.data.items[] | {uuid,chapter_code,title,revision}`：章节列表。
- `.data | {uuid,kind,resource_uuid,status,error_code,error_message}`：任务状态。
- `.data | {items:{uuid,sequence,event_type,created_at},cursor_pagination:{per_page,next_cursor,prev_cursor,has_more}}`：保留任务事件和分页信息。

## 当前项目 API 索引

以下索引对应源码 `internal/agent/external_project_api.go` 的 59 条对外路由。花括号为路径参数，必须替换为授权项目内实际读取到的 UUIDv7。文档路径通过 `read_agent_doc` 的 `path` 参数读取。

### 项目与故事总纲

文档：`/api/v1/agent-docs/api/project.md`。
- `GET /api/v1/projects/{project_uuid}`：读取项目公开信息。
- `PATCH /api/v1/projects/{project_uuid}`：更新项目元数据。

文档：`/api/v1/agent-docs/api/story.md`。
- `GET /api/v1/projects/{project_uuid}/story-profile`：读取故事档案。
- `PUT /api/v1/projects/{project_uuid}/story-profile`：更新故事档案。

### 章节、正文与回收站

文档：`/api/v1/agent-docs/api/chapter.md`。
- `GET /api/v1/projects/{project_uuid}/chapters`：列出章节。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}`：读取章节。
- `PUT /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/current-story`：更新章节正文。
- `DELETE /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/permanent`：永久删除回收站章节。
- `POST /api/v1/projects/{project_uuid}/chapters`：创建章节。
- `PATCH /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}`：更新章节元数据。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/stories`：读取章节正文历史。
- `DELETE /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}`：将章节移入回收站。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/restorations`：从回收站恢复章节。

### 画风、设定来源与设定资产

文档：`/api/v1/agent-docs/api/premise.md`。
- `GET /api/v1/projects/{project_uuid}/premise`：读取当前 Premise。
- `PATCH /api/v1/projects/{project_uuid}/premise`：更新 Premise。
- `GET /api/v1/projects/{project_uuid}/premise-sources`：列出 Premise Source。
- `POST /api/v1/projects/{project_uuid}/premise-sources`：创建 Premise Source。
- `PATCH /api/v1/projects/{project_uuid}/premise-sources/{source_uuid}`：更新 Premise Source 状态。
- `GET /api/v1/projects/{project_uuid}/premise-setting-images`：列出 Premise Setting Image。
- `POST /api/v1/projects/{project_uuid}/premise-setting-images/{setting_image_uuid}/selections`：选择 Premise Setting Image。

文档：`/api/v1/agent-docs/api/premise-asset.md`。
- `GET /api/v1/projects/{project_uuid}/premise-assets`：列出设定项。
- `GET /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}`：读取设定项。
- `PATCH /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}`：更新设定项。
- `DELETE /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}`：将设定项移入回收站。
- `DELETE /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/permanent`：永久删除回收站设定项。
- `POST /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/restorations`：从回收站恢复设定项。
- `GET /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/variants`：列出设定项图片 variants。
- `POST /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/variants/{variant_uuid}/selections`：选择设定项图片 variant。

### 漫画页面、分镜、图片版本与快照

文档：`/api/v1/agent-docs/api/comic.md`。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic`：读取 Comic 状态。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections`：列出 Comic Section。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections`：创建 Comic Section。
- `PATCH /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}`：更新 Comic Section。
- `PUT /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-section-order`：重排 Comic Section。
- `DELETE /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}`：删除 Comic Section。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-variants`：列出 Section Image variants。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-variants/{variant_uuid}/selections`：选择 Section Image variant。

文档：`/api/v1/agent-docs/api/comic-section.md`。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}`：读取漫画 Section。

文档：`/api/v1/agent-docs/api/storyboard.md`。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/storyboard-variants`：更新 Storyboard。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/storyboard-variants`：列出 Storyboard variants。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/storyboard-variants/{variant_uuid}/selections`：选择 Storyboard variant。

文档：`/api/v1/agent-docs/api/comic-snapshot.md`。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-snapshots`：列出 Comic Snapshot。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-snapshots/{snapshot_uuid}`：读取 Comic Snapshot。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-snapshots/{snapshot_uuid}/restorations`：恢复 Comic Snapshot。

### 异步生成

文档：`/api/v1/agent-docs/api/generation.md`。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/generations`：创建章节生成任务。
- `POST /api/v1/projects/{project_uuid}/premise-sources/{source_uuid}/setting-generations`：创建 Premise 设定图任务。
- `POST /api/v1/projects/{project_uuid}/premise-setting-images/{setting_image_uuid}/breakdowns`：创建 Premise 拆解任务。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-generations`：创建漫画图片任务。

### 任务、事件、取消与重试

文档：`/api/v1/agent-docs/api/task.md`。
- `GET /api/v1/projects/{project_uuid}/tasks/{task_uuid}`：读取故事任务状态。
- `GET /api/v1/projects/{project_uuid}/production-tasks/{task_uuid}`：读取生产任务状态。
- `GET /api/v1/projects/{project_uuid}/tasks`：列出 Story Task。
- `GET /api/v1/projects/{project_uuid}/tasks/{task_uuid}/events`：读取 Story Task 事件。
- `POST /api/v1/projects/{project_uuid}/tasks/{task_uuid}/cancellations`：取消 Story Task。
- `POST /api/v1/projects/{project_uuid}/tasks/{task_uuid}/retries`：重试 Story Task。
- `GET /api/v1/projects/{project_uuid}/production-tasks`：列出 Production Task。
- `GET /api/v1/projects/{project_uuid}/production-tasks/{task_uuid}/events`：读取 Production Task 事件。
- `POST /api/v1/projects/{project_uuid}/production-tasks/{task_uuid}/cancellations`：取消 Production Task。
- `POST /api/v1/projects/{project_uuid}/production-tasks/{task_uuid}/retries`：重试 Production Task。

### 项目文件

文档：`/api/v1/agent-docs/api/project-asset.md`。
- `GET /api/v1/projects/{project_uuid}/assets`：列出 Project Asset。
- `GET /api/v1/projects/{project_uuid}/assets/{asset_uuid}`：读取 Project Asset。

## 典型调用示例

以下 JSON 是参数示意。示例 UUID 必须替换为本次授权与实际读取到的资源 UUID；revision 必须使用最新值。写操作为每个新意图生成新幂等键，只有参数完全相同的网络重试才复用原键，不能把示例键作为固定默认值。

### 读取项目与章节列表

先读取 `/api/v1/agent-docs/api/project.md` 与 `/api/v1/agent-docs/api/chapter.md`，分别调用 `request_api`：

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001",
  "response_filter": ".data | {uuid,name,description,revision}"
}
```

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters",
  "query": {"state": "active"},
  "response_filter": ".data.items[] | {uuid,chapter_code,title,revision}"
}
```

### 读取最新版本后修改章节标题

先 `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}`，投影 `.data | {uuid,title,revision}`。假设读到 revision 为 3，且用户要求修改标题，则调用 `request_api`：

```json
{
  "method": "PATCH",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters/01970000-0000-7000-8000-000000000002",
  "request_body": {"title": "午夜来信", "expected_revision": 3},
  "response_filter": ".data | {uuid,chapter_code,title,revision}",
  "idempotency_key": "01970000-0000-7000-8000-000000000010"
}
```

版本冲突后重新读取事实状态并重新决定修改内容，不盲目覆盖。修改正文使用实时 Contract 中的 `PUT .../current-story`，提交完整正文与 `content_format`，不是标题 PATCH 或正文增量。

### 提交生成并按需查看状态

先读取 `/api/v1/agent-docs/api/generation.md`，在用户请求生成章节正文时调用 `request_api`：

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters/01970000-0000-7000-8000-000000000002/generations",
  "request_body": {"prompt_key": "story_chapter", "prompt": "依据故事总纲生成本章完整正文"},
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}",
  "idempotency_key": "01970000-0000-7000-8000-000000000011"
}
```

保存 `data.call.uuid` 与成功业务结果 `data.result.data.uuid`（Task UUID）。再按需读取 `/api/v1/agent-docs/api/task.md` 并查询 Story Task：

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/tasks/01970000-0000-7000-8000-000000000008",
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

设定图、设定拆解和漫画图片生成返回的任务通过 `/production-tasks/{task_uuid}` 查询。状态为 `queued`、`running` 或 `waiting_for_input` 时只报告实际状态；在 `completed` 后重读对应业务资源。不启动定时轮询或阻塞等待循环。

### 恢复确认结果与查看图片

`pending_confirmation` 表示操作尚待用户在 Lumi 的 MCP 设置中处理。展示调用 UUID 和具体操作；用户处理后调用 `get_call`：

```json
{"call_uuid": "01970000-0000-7000-8000-000000000012"}
```

`rejected`、`expired` 是终态；不要传 `confirmed=true` 或通过重放代替确认。网络结果不确定时，优先恢复已有 Call，或以原幂等键和完全相同的参数重试；`interrupted` 后先检查业务事实。

从允许的 Asset API 或图片版本数据获得真实文件 UUID 后，调用 `read_media`：

```json
{"file_uuid": "01970000-0000-7000-8000-000000000009"}
```

## 当前未开放的能力

外部 MCP 未开放漫画导出、文件直传、LLM 日志、模型与 Provider 管理、项目初始化/自动 Workflow、章节批量规划、整章分镜生成或批量图片生成。即使共享文档提及这些路由，也不能调用。不要复制 VACS 路径、注入内部 Chat/Run 元数据，或通过原始 HTTP、文件系统、SQLite 绕过限制。

权限不足、项目未打开、授权过期或被撤销时，按错误提示让用户在 Lumi 打开已授权项目或重新走 MCP 登录。切换项目需要单独连接或重新授权，不能仅替换 URL 中的 UUID。
