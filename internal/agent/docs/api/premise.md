# Premise API

使用 `request_api` 调用。将占位符替换为公开 UUIDv7；更新前先读取最新 `revision`，冲突后重新读取。

## Premise 与来源

- `GET /api/v1/projects/{project_uuid}/premise`
  - 使用 `.data | {uuid,default_style,current_source,current_setting_image,revision}`。
- `PATCH /api/v1/projects/{project_uuid}/premise`
  - `request_body`：`{"default_style":"完整画风描述","expected_revision":3}`。
- `GET /api/v1/projects/{project_uuid}/premise-sources`
  - 可选 `query`：`page`、`per_page`。
  - 使用 `.data.items[] | {uuid,source_type,source_text,style_snapshot,ignored_at,revision,created_at}`。
- `POST /api/v1/projects/{project_uuid}/premise-sources`
  - `source_text`、`style_snapshot`、`source_type` 必填；`source_type` 为 `manual` 或 `generated`。
  - `model`、`parameters` 仅用于记录实际生成来源时可选填写。
  - 示例：`{"source_text":"完整设定描述","style_snapshot":"当前画风","source_type":"manual"}`。
- `PATCH /api/v1/projects/{project_uuid}/premise-sources/{source_uuid}`
  - `request_body`：`{"ignored":true,"expected_revision":3}`。

## Setting Image

- `GET /api/v1/projects/{project_uuid}/premise-setting-images`
  - 可选 `query`：`{"source_uuids":["<source_uuid>"]}`，最多 100 个。
  - 使用 `.data.items[] | {uuid,source_uuid,origin,prompt,asset,created_at}`。
- `POST /api/v1/projects/{project_uuid}/premise-setting-images`
  - 导入当前项目尚未消费的 ready upload。
  - `request_body`：`upload_uuid` 必填；`source_uuid`、`prompt` 可选。
  - 示例：`{"upload_uuid":"<upload_uuid>","source_uuid":"<source_uuid>","prompt":"设定图说明"}`。
- `POST /api/v1/projects/{project_uuid}/premise-setting-images/{setting_image_uuid}/selections`
  - `request_body` 必须传空对象：`{}`。

生成设定图和拆解任务见 `/api/v1/agent-docs/api/generation.md`。
