# Premise Asset API

文中“获得确认”均指：先提交参数完整的 `request_api`；若返回 `agent_tool_confirmation_required`，该次不会执行写操作，此时再按 Overview 的全局协议把 confirmation 只传给 `request_user_input`；确认后由运行时自动执行原请求。

使用 `request_api` 调用。将占位符替换为公开 UUIDv7；修改生命周期或图片前先读取最新 `revision`，冲突后重新读取。

常用过滤器：

- 列表：`.data.items[] | {uuid,asset_type,title,summary,tags,revision,deleted_at}`
- 单项：`.data | {uuid,asset_type,title,summary,tags,current_variant,revision,deleted_at}`

## 读取与创建

- `GET /api/v1/projects/{project_uuid}/premise-assets`
  - 可选 `query`：`{"state":"active"}`、`{"state":"trashed"}`，以及 `tag`。
- `GET /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}`
- `POST /api/v1/projects/{project_uuid}/premise-assets`
  - `asset_type`、`title` 必填；`asset_type` 为 `character`、`scene`、`prop` 或 `reference`。
  - `file_uuid` 与 `upload_uuid` 必须且只能传一个；`summary`、`tags` 可选。
  - `file_uuid` 只能是当前 Thread 中 `image_gen` 刚生成、用途匹配且尚未消费的 File UUIDv7；已有项目图片或当前 variant 的 asset UUID 不可复用。
  - `upload_uuid` 只能是当前项目尚未消费的 ready upload UUIDv7。
  - 示例：`{"file_uuid":"<file_uuid>","asset_type":"character","title":"主角","summary":"人物设定","tags":["主角"]}`。

## 修改图片与元数据

- `PATCH /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}`
  - `expected_revision` 必填；按需传 `asset_type`、`title`、`summary`、`tags`。
  - 替换为当前 Thread 新生成的图片时可同时传 `file_uuid`；ready upload 应改用下面的 variant 接口。
- `GET /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/variants`
  - 使用 `.data.items[] | {uuid,version_no,source_type,source_setting_image_uuid,crop,asset,created_at}`。
- `POST /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/variants`
  - `request_body`：`{"upload_uuid":"<upload_uuid>","expected_revision":3}`。
- `POST /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/variants/{variant_uuid}/selections`
  - `request_body`：`{"expected_revision":3}`。

## 回收站

删除类操作调用前必须用 `request_user_input` 获得确认。

- 移入回收站：`DELETE /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}`
  - `request_body`：`{"expected_revision":3}`。
- 恢复：`POST /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/restorations`
  - `request_body`：`{"expected_revision":3}`。
- 永久删除：`DELETE /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/permanent`
  - 资产必须已在回收站；传 `query: {"expected_revision":3}`，不传 `request_body`。
- 清空回收站：`DELETE /api/v1/projects/{project_uuid}/premise-assets/trash`
  - 不传 `query` 或 `request_body`；使用 `.data | {deleted_count,file_soft_deleted_count,retained_file_count,blocked_items}`。
