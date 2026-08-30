# Project Asset API

本 Contract 维护已存在的项目 Asset。各响应表把 Overview 的 `public_asset_v1` 展开到 `data` 或 `data.items[]`；新上传使用 multipart UI 流程，不通过 `request_api`。

## `GET /api/v1/projects/{project_uuid}/assets`

按公开属性筛选并列出项目 Asset。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `purpose` | query | 字符串，最长 120 字符 | 否 | 按已注册用途精确筛选。 |
| `kind` | query | 字符串，最长 120 字符 | 否 | 按 Asset kind 精确筛选。 |
| `deleted` | query | 布尔值 | 否 | `true` 只列回收站；默认 `false`，只列 active。 |
| `limit` | query | 整数，1～100 | 否 | 最大返回数量，默认 `100`。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | 数组 | Asset 列表。 |
| `data.items[].uuid` | UUIDv7 字符串 | Asset 公开 UUID。 |
| `data.items[].kind` | 字符串 | 资源类型，如 `image`。 |
| `data.items[].purpose` | 字符串 | Asset Store 用途。 |
| `data.items[].original_filename` | 字符串，可省略 | 原始文件名；无原始上传文件名时省略。 |
| `data.items[].display_name` | 字符串，可省略 | 展示名称；未设置或已清空时省略。 |
| `data.items[].source_type` | 字符串 | 来源类型。 |
| `data.items[].source_asset_uuid` | UUIDv7 字符串，可省略 | 派生来源 Asset UUID。 |
| `data.items[].mime_type` | 字符串 | MIME 类型。 |
| `data.items[].byte_size` | 整数 | 文件字节数。 |
| `data.items[].width` | 整数，可省略 | 图片或视频宽度，像素。 |
| `data.items[].height` | 整数，可省略 | 图片或视频高度，像素。 |
| `data.items[].duration_ms` | 整数，可省略 | 音视频时长，毫秒。 |
| `data.items[].status` | 字符串 | 文件对象状态。 |
| `data.items[].deleted_at` | RFC 3339 字符串，可省略 | 软删除时间。 |
| `data.items[].created_at` | RFC 3339 字符串 | 创建时间。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/<project_uuid>/assets",
  "query": {
    "kind": "image",
    "limit": 20
  },
  "response_filter": ".data.items[] | {uuid,kind,purpose,original_filename,display_name,mime_type,byte_size,width,height,duration_ms,status,deleted_at,created_at}"
}
```

### 接口约束

- `deleted: true` 与 active 列表互斥，不表示同时包含两种状态。
- `purpose` 必须是服务端已注册用途；未知用途会被拒绝。

## `GET /api/v1/projects/{project_uuid}/assets/{asset_uuid}`

读取一个 Project Asset 的公开元数据。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `asset_uuid` | path | UUIDv7 字符串 | 是 | 要读取的 Asset UUID。 |
| `include_trashed` | query | 布尔值 | 否 | 是否允许读取回收站 Asset，默认 `false`。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Asset UUID。 |
| `data.kind` | 字符串 | 资源类型。 |
| `data.purpose` | 字符串 | Asset Store 用途。 |
| `data.original_filename` | 字符串，可省略 | 原始文件名；无原始上传文件名时省略。 |
| `data.display_name` | 字符串，可省略 | 展示名称；未设置或已清空时省略。 |
| `data.source_type` | 字符串 | 来源类型。 |
| `data.source_asset_uuid` | UUIDv7 字符串，可省略 | 派生来源 Asset UUID。 |
| `data.mime_type` | 字符串 | MIME 类型。 |
| `data.byte_size` | 整数 | 文件字节数。 |
| `data.width` | 整数，可省略 | 媒体宽度，像素。 |
| `data.height` | 整数，可省略 | 媒体高度，像素。 |
| `data.duration_ms` | 整数，可省略 | 音视频时长，毫秒。 |
| `data.status` | 字符串 | 文件对象状态。 |
| `data.deleted_at` | RFC 3339 字符串，可省略 | 软删除时间。 |
| `data.created_at` | RFC 3339 字符串 | 创建时间。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/<project_uuid>/assets/<asset_uuid>",
  "response_filter": ".data | {uuid,kind,purpose,original_filename,display_name,source_type,source_asset_uuid,mime_type,byte_size,width,height,duration_ms,status,deleted_at,created_at}"
}
```

### 接口约束

- 读取已删除 Asset 时必须显式传 `query: {"include_trashed": true}`。
- 不传 `request_body`。

## `PATCH /api/v1/projects/{project_uuid}/assets/{asset_uuid}`

更新 active Project Asset 的展示名称或用途允许的公开 metadata。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `asset_uuid` | path | UUIDv7 字符串 | 是 | 要更新的 active Asset UUID。 |
| `display_name` | body | 字符串，最长 255 字符 | 否 | 新展示名称；空字符串可清空。 |
| `metadata` | body | JSON 对象 | 否 | 新公开 metadata；服务端按该 Asset 的 `purpose` 过滤允许字段。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Asset UUID。 |
| `data.kind` | 字符串 | 资源类型。 |
| `data.purpose` | 字符串 | Asset Store 用途。 |
| `data.original_filename` | 字符串，可省略 | 原始文件名；无原始上传文件名时省略。 |
| `data.display_name` | 字符串，可省略 | 更新后的展示名称；未设置或清空后省略。 |
| `data.source_type` | 字符串 | 来源类型。 |
| `data.source_asset_uuid` | UUIDv7 字符串，可省略 | 派生来源 Asset UUID。 |
| `data.mime_type` | 字符串 | MIME 类型。 |
| `data.byte_size` | 整数 | 文件字节数。 |
| `data.width` | 整数，可省略 | 媒体宽度，像素。 |
| `data.height` | 整数，可省略 | 媒体高度，像素。 |
| `data.duration_ms` | 整数，可省略 | 音视频时长。 |
| `data.status` | 字符串 | 文件对象状态。 |
| `data.deleted_at` | RFC 3339 字符串，可省略 | active 资源省略。 |
| `data.created_at` | RFC 3339 字符串 | 创建时间。 |

### request_api 示例

```json
{
  "method": "PATCH",
  "url": "/api/v1/projects/<project_uuid>/assets/<asset_uuid>",
  "request_body": {
    "display_name": "封面图"
  },
  "response_filter": ".data | {uuid,kind,purpose,display_name,mime_type,byte_size,width,height,status}"
}
```

### 接口约束

- `request_body` 必须存在；`display_name` 与 `metadata` 均可选，空对象是无副作用读取。
- 本接口没有 revision；不接受 `expected_revision`，也不能修改文件内容、kind 或 purpose。

## `DELETE /api/v1/projects/{project_uuid}/assets/{asset_uuid}`

将 Project Asset 软删除到回收站。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `asset_uuid` | path | UUIDv7 字符串 | 是 | 要移入回收站的 Asset UUID。 |
| `request_body` | body | 空 JSON 对象 | 是 | 必须为 `{}`。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Asset UUID。 |
| `data.kind` | 字符串 | 资源类型。 |
| `data.purpose` | 字符串 | Asset Store 用途。 |
| `data.original_filename` | 字符串，可省略 | 原始文件名；无原始上传文件名时省略。 |
| `data.display_name` | 字符串，可省略 | 展示名称；未设置或已清空时省略。 |
| `data.source_type` | 字符串 | 来源类型。 |
| `data.source_asset_uuid` | UUIDv7 字符串，可省略 | 派生来源 Asset UUID。 |
| `data.mime_type` | 字符串 | MIME 类型。 |
| `data.byte_size` | 整数 | 文件字节数。 |
| `data.width` | 整数，可省略 | 媒体宽度，像素。 |
| `data.height` | 整数，可省略 | 媒体高度，像素。 |
| `data.duration_ms` | 整数，可省略 | 音视频时长。 |
| `data.status` | 字符串 | 文件对象状态。 |
| `data.deleted_at` | RFC 3339 字符串 | 软删除时间。 |
| `data.created_at` | RFC 3339 字符串 | 创建时间。 |

### request_api 示例

```json
{
  "method": "DELETE",
  "url": "/api/v1/projects/<project_uuid>/assets/<asset_uuid>",
  "request_body": {},
  "response_filter": ".data | {uuid,kind,purpose,display_name,status,deleted_at}"
}
```

### 接口约束

- 危险操作；完整请求首次提交后，按 Overview 的确认协议处理。
- 本接口没有 revision；重复软删除同一对象是幂等的。

## `POST /api/v1/projects/{project_uuid}/assets/{asset_uuid}/restorations`

从回收站恢复 Project Asset。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `asset_uuid` | path | UUIDv7 字符串 | 是 | 要恢复的 Asset UUID。 |
| `request_body` | body | 空 JSON 对象 | 是 | 必须为 `{}`。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Asset UUID。 |
| `data.kind` | 字符串 | 资源类型。 |
| `data.purpose` | 字符串 | Asset Store 用途。 |
| `data.original_filename` | 字符串，可省略 | 原始文件名；无原始上传文件名时省略。 |
| `data.display_name` | 字符串，可省略 | 展示名称；未设置或已清空时省略。 |
| `data.source_type` | 字符串 | 来源类型。 |
| `data.source_asset_uuid` | UUIDv7 字符串，可省略 | 派生来源 Asset UUID。 |
| `data.mime_type` | 字符串 | MIME 类型。 |
| `data.byte_size` | 整数 | 文件字节数。 |
| `data.width` | 整数，可省略 | 媒体宽度，像素。 |
| `data.height` | 整数，可省略 | 媒体高度，像素。 |
| `data.duration_ms` | 整数，可省略 | 音视频时长。 |
| `data.status` | 字符串 | 文件对象状态。 |
| `data.deleted_at` | RFC 3339 字符串，可省略 | 恢复成功后省略。 |
| `data.created_at` | RFC 3339 字符串 | 创建时间。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/<project_uuid>/assets/<asset_uuid>/restorations",
  "request_body": {},
  "response_filter": ".data | {uuid,kind,purpose,display_name,status,deleted_at}"
}
```

### 接口约束

- 本接口没有 revision；重复恢复同一对象是幂等的。
