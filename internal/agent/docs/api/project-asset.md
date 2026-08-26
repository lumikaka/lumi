# Project Asset API

文中“获得确认”均指：先提交参数完整的 `request_api`；若返回 `agent_tool_confirmation_required`，该次不会执行写操作，此时再按 Overview 的全局协议把 confirmation 只传给 `request_user_input`；确认后由运行时自动执行原请求。

使用 `request_api` 维护已存在的项目资产，将占位符替换为公开 UUIDv7。上传是 multipart 流程，`request_api` 不支持；需要新上传时引导用户使用界面。

## 读取

- `GET /api/v1/projects/{project_uuid}/assets`
  - 可选 `query`：`purpose`、`kind`、`deleted`、`limit`（1–100）。`deleted:true` 只读取回收站资产。
  - 使用 `.data.items[] | {uuid,kind,purpose,original_filename,display_name,mime_type,byte_size,width,height,duration_ms,status,deleted_at,created_at}`。
- `GET /api/v1/projects/{project_uuid}/assets/{asset_uuid}`
  - 读取回收站资产时传 `query: {"include_trashed":true}`。
  - 使用 `.data | {uuid,kind,purpose,original_filename,display_name,source_type,source_asset_uuid,mime_type,byte_size,width,height,duration_ms,status,deleted_at,created_at}`。

## 修改与生命周期

- `PATCH /api/v1/projects/{project_uuid}/assets/{asset_uuid}`
  - 按需传 `display_name`、`metadata`，至少传一个。
  - 示例：`{"display_name":"封面图"}`。
- `DELETE /api/v1/projects/{project_uuid}/assets/{asset_uuid}`
  - 移入回收站；调用前必须用 `request_user_input` 获得确认。
  - `request_body` 必须传空对象：`{}`。
- `POST /api/v1/projects/{project_uuid}/assets/{asset_uuid}/restorations`
  - `request_body` 必须传空对象：`{}`。

这些接口没有 `expected_revision`。
