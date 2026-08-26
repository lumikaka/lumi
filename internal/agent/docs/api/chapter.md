# Chapter API

文中“获得确认”均指：先提交参数完整的 `request_api`；若返回 `agent_tool_confirmation_required`，该次不会执行写操作，此时再按 Overview 的全局协议把 confirmation 只传给 `request_user_input`；确认后由运行时自动执行原请求。

使用 `request_api` 调用。将路径中的 UUID 占位符替换为当前项目或目标 Chapter 的公开 UUIDv7；`project_uuid` 不得放入 `query` 或 `request_body`。

## 必要规则

- 每次调用都传 `response_filter`，只读取当前步骤需要的字段。
- 修改、移入回收站、恢复或永久删除前，先读取 Chapter 的最新 `revision`，再作为 `expected_revision` 提交；冲突后重新读取，不要盲目重试。
- 删除类操作先用 `request_user_input` 获得确认。
- Chapter 编号使用 `vol01.ch01` 格式；正文格式只支持 `txt` 或 `md`。

常用过滤器：

- Chapter：`.data | {uuid,chapter_code,title,revision,trashed_at,current_story}`
- Chapter 列表：`.data.items[] | {uuid,chapter_code,title,revision,trashed_at}`
- 正文历史：`.data.items[] | {uuid,version_no,source_type,content,content_format,created_at}`

## 读取与编辑

- `GET /api/v1/projects/{project_uuid}/chapters`
  - 可选 `query`：`{"state":"active"}` 或 `{"state":"trashed"}`；默认 `active`。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}`
  - active 与回收站 Chapter 均可读取；写操作前用它获取最新 `revision`。
- `POST /api/v1/projects/{project_uuid}/chapters`
  - `request_body`：`chapter_code`、`title` 必填；`content`、`content_format` 可选。
  - 示例：`{"chapter_code":"vol01.ch01","title":"第一章","content":"...","content_format":"md"}`。
  - 提供 `content` 但省略 `content_format` 时默认使用 `txt`；不提供 `content` 时同时省略 `content_format`。
  - 该接口只创建 Chapter，不启动 AI 生成，也不接收 `prompt_key`。
- `PATCH /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}`
  - `request_body`：`{"title":"新标题","expected_revision":3}`；`expected_revision` 是 integer。
- `PUT /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/current-story`
  - `request_body`：`{"content":"完整正文","content_format":"md","expected_revision":3}`；不是增量修改。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/stories`
  - 返回不可变正文版本历史。

创建和更新操作返回 Chapter；列表返回 `{items}`，正文历史返回 `{items}`。所有响应都使用统一 API JSON 信封。

## 生成章节正文

先创建或读取目标 Chapter，再调用：

`POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/generations`

`request_body`：

- `prompt_key`：必填。普通章节用 `story_chapter`；创建下一章或基于上一章续写用 `next_story_chapter`。
- `prompt`：必填，描述本次生成要求。
- `model`：可选模型覆盖。

示例：`{"prompt_key":"story_chapter","prompt":"生成本章正文"}`；仅在确实要覆盖模型时追加 `model`，不要发送占位字符串。

使用 `.data | {uuid,kind,resource_uuid,status,error_code,error_message}` 读取创建的 Task。任务跟踪见 `/api/v1/agent-docs/api/task.md`。

## 回收站

- 移入回收站：`DELETE /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}`
  - `request_body`：`{"expected_revision":3}`。
- 恢复：`POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/restorations`
  - `request_body`：`{"expected_revision":3}`。
- 永久删除：`DELETE /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/permanent`
  - Chapter 必须已在回收站；传 `query: {"expected_revision":3}`，不传 `request_body`。
- 清空回收站：`DELETE /api/v1/projects/{project_uuid}/chapters/trash`
  - 不传 `query` 或 `request_body`；使用 `.data | {deleted_count,blocked_items}` 读取结果。

文件导入要求 multipart，当前 `request_api` 不支持；需要时引导用户使用界面导入。
