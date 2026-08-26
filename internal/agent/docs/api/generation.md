# Generation API

使用 `request_api` 调用，将路径和示例中的占位符替换为公开 UUIDv7。以下接口都返回异步 Task。使用 `.data | {uuid,kind,resource_uuid,status,error_code,error_message}`，再按 `/api/v1/agent-docs/api/task.md` 跟踪。`model` 都是可选覆盖值；不覆盖时省略，不要发送“可选”等占位字符串。

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
  - `request_body`：`prompt` 必填；`model`、`premise_asset_uuids` 可选。
  - `premise_asset_uuids` 只传与画面直接相关的 Premise Asset UUIDv7。
  - 示例：`{"prompt":"生成该段落漫画图","premise_asset_uuids":["<premise_asset_uuid>"]}`。
