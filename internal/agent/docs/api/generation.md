# Generation API

使用 `request_api` 调用，将路径和示例中的占位符替换为公开 UUIDv7。除漫画图片批量接口外，以下接口都直接返回一个异步 Task；使用 `.data | {uuid,kind,resource_uuid,status,error_code,error_message}`，再按 `/api/v1/agent-docs/api/task.md` 跟踪。`model` 都是可选覆盖值；不覆盖时省略，不要发送“可选”等占位字符串。文中“获得确认”均指：先提交参数完整的 `request_api` 获取 `agent_tool_confirmation_required`（此时不会创建 Task），再按 Overview 的全局协议把 confirmation 只传给 `request_user_input`；确认后由运行时自动执行原请求。

## Story 与 Chapter

- `POST /api/v1/projects/{project_uuid}/story-profile/generations`
  - `request_body`：`prompt` 必填；`chapter_count` 可选，范围 1–20；`model` 可选。
  - 示例：`{"prompt":"生成故事总纲","chapter_count":8}`。
- `POST /api/v1/projects/{project_uuid}/story-profile/reconstructions`
  - 从现有章节重建总纲，会覆盖当前 Story Profile，调用前必须获得确认。
  - 不覆盖模型时传 `request_body: {}`；否则传 `{"model":"<model>"}`。
- `POST /api/v1/projects/{project_uuid}/chapter-batches`
  - 批量规划并创建章节，调用前必须获得确认。
  - `request_body`：`prompt` 必填；`chapter_count` 可选，范围 1–20；`model` 可选。
  - 示例：`{"prompt":"规划第一卷章节","chapter_count":8}`。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/generations`
  - `request_body`：`prompt_key`、`prompt` 必填；`model` 可选。
  - 普通章节用 `story_chapter`；创建下一章或续写用 `next_story_chapter`。
  - 示例：`{"prompt_key":"story_chapter","prompt":"生成本章正文"}`。

## Premise

- `POST /api/v1/projects/{project_uuid}/premise-sources/{source_uuid}/setting-generations`
  - `request_body`：`prompt` 必填，`model` 可选。
- `POST /api/v1/projects/{project_uuid}/premise-setting-images/{setting_image_uuid}/breakdowns`
  - `request_body`：`prompt` 必填，`model` 可选。

这两个接口不要传 `premise_asset_uuids`；该字段只用于 Comic 图片生成。

## Comic

- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-storyboard-generations`
  - 依据章节正文创建完整分镜规划，调用前必须获得确认。
  - `request_body`：`prompt` 必填；`max_section_count` 可选，范围 1–48；`model` 可选。
  - 示例：`{"prompt":"按主要情节点拆分分镜","max_section_count":8}`。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-generations`
  - 只用于生成一个 Section。
  - `request_body`：`prompt` 必填；`model`、`premise_asset_uuids` 可选。
  - `premise_asset_uuids` 只传与画面直接相关的 Premise Asset UUIDv7。
  - 示例：`{"prompt":"生成该段落漫画图","premise_asset_uuids":["<premise_asset_uuid>"]}`。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-image-generation-batches`
  - 生成两个及以上 Section 时必须使用，并且只调用一次；不要逐页读取 Section、循环调用单图接口或改用 `image_gen`。
  - `request_body` 只传 `section_uuids`，包含 1–48 个有效、不重复的 Section UUIDv7，并保持用户要求的页序。幂等键由 Tool Execution 自动提供。
  - 服务端统一预检归属、active 状态、当前 Storyboard 和活动图片任务；任一项不满足时整批不创建任务。
  - 服务端自动使用每个 Section 的当前 Storyboard、项目画风、设定引用和项目图片模型；不要传 Prompt、引用或生成参数。
  - 示例：`{"section_uuids":["<section_uuid_1>","<section_uuid_2>"]}`。
  - 使用 `.data | {chapter_uuid,requested_count,accepted_count,tasks:{uuid,kind,resource_uuid,status,error_code,error_message}}`。

Task 状态为 `queued` 只表示任务已创建，不能报告图片生成完成。
