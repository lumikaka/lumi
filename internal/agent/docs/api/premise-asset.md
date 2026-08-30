# Premise Asset API

## `GET /api/v1/projects/{project_uuid}/premise-assets`

列出当前项目的 active 或回收站 Premise Asset，并可按标签筛选。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `state` | query | 枚举字符串 | 否 | `active` 或 `trashed`，默认 `active`。 |
| `tag` | query | 字符串，最长 64 字符 | 否 | 按规范化标签精确筛选。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | 数组 | 与状态和标签条件匹配的 Premise Asset 列表。 |
| `data.items[].uuid` | UUIDv7 字符串 | Asset 公开 UUID。 |
| `data.items[].asset_type` | 字符串 | `character`、`scene`、`prop` 或 `reference`。 |
| `data.items[].title` | 字符串 | 标题。 |
| `data.items[].summary` | 字符串 | 简介。 |
| `data.items[].tags` | 字符串数组 | 规范化标签。 |
| `data.items[].current_variant` | 对象或 `null` | 当前图片 variant；包含 `uuid`、`version_no`、`source_type`、可选 `source_setting_image_uuid`、`crop`、`asset` 和 `created_at`。 |
| `data.items[].current_variant.asset` | 对象 | 当前 variant 非空时返回；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.items[].revision` | 整数 | 当前乐观并发版本。 |
| `data.items[].deleted_at` | RFC 3339 字符串，可省略 | 软删除时间；active 资源省略。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/<project_uuid>/premise-assets",
  "response_filter": ".data.items[] | {uuid,asset_type,title,summary,tags,revision}"
}
```

### 接口约束

- 不传 `request_body`；省略 `query` 时固定列 active 资源。
- `state=trashed` 只列回收站，不会同时返回 active 资源。

## `GET /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}`

读取一个 active Premise Asset 及其当前图片 variant。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `premise_asset_uuid` | path | UUIDv7 字符串 | 是 | 要读取的 Premise Asset UUID。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Asset UUID。 |
| `data.asset_type` | 字符串 | `character`、`scene`、`prop` 或 `reference`。 |
| `data.title` | 字符串 | 标题。 |
| `data.summary` | 字符串 | 简介。 |
| `data.tags` | 字符串数组 | 标签。 |
| `data.current_variant` | 对象或 `null` | 当前图片 variant。 |
| `data.current_variant.uuid` | UUIDv7 字符串 | Variant UUID。 |
| `data.current_variant.version_no` | 整数 | 图片版本号。 |
| `data.current_variant.source_type` | 字符串 | 图片来源类型。 |
| `data.current_variant.source_setting_image_uuid` | UUIDv7 字符串，可省略 | 来源 Setting Image UUID。 |
| `data.current_variant.crop` | 任意 JSON 值或 `null` | Provider/导入流程保留的裁剪参数；未提供时为空。 |
| `data.current_variant.asset` | 对象 | 公开图片 Asset；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.current_variant.created_at` | RFC 3339 字符串 | Variant 创建时间。 |
| `data.revision` | 整数 | 当前 revision。 |
| `data.deleted_at` | RFC 3339 字符串，可省略 | active 资源省略。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/<project_uuid>/premise-assets/<premise_asset_uuid>",
  "response_filter": ".data | {uuid,asset_type,title,summary,tags,current_variant,revision}"
}
```

### 接口约束

- 不传 `query` 或 `request_body`；回收站资源不能通过本接口读取。
- 写入前使用返回的最新 `revision`。

## `POST /api/v1/projects/{project_uuid}/premise-assets`

使用新生成 File 或 ready upload 创建 Premise Asset。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `file_uuid` | body | UUIDv7 字符串 | 条件必填 | 当前 Thread 中 `image_gen` 刚生成、用途匹配且尚未消费的 File；与 `upload_uuid` 必须且只能提供一个。 |
| `upload_uuid` | body | UUIDv7 字符串 | 条件必填 | 当前项目用途匹配、ready 且尚未消费的 upload；与 `file_uuid` 必须且只能提供一个。 |
| `asset_type` | body | 枚举字符串 | 是 | `character`、`scene`、`prop` 或 `reference`。 |
| `title` | body | 非空字符串，最长 160 字符 | 是 | Asset 标题。 |
| `summary` | body | 字符串，最长 12000 字符 | 否 | Asset 简介，省略时为空字符串。 |
| `tags` | body | 字符串数组 | 否 | 最多 64 个标签，每项最长 64 字符；会去空、转小写并去重。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | 新 Asset UUID。 |
| `data.asset_type` | 字符串 | Asset 类型。 |
| `data.title` | 字符串 | 标题。 |
| `data.summary` | 字符串 | 简介。 |
| `data.tags` | 字符串数组 | 规范化标签。 |
| `data.current_variant` | 对象 | 首个图片 variant；包含 `uuid`、`version_no`、`source_type`、`crop`、公开 `asset` 和 `created_at`。 |
| `data.current_variant.asset` | 对象 | 首个 variant 的公开 Asset；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.revision` | 整数 | 初始 revision。 |
| `data.deleted_at` | RFC 3339 字符串，可省略 | 新建资源省略。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/<project_uuid>/premise-assets",
  "request_body": {
    "file_uuid": "<file_uuid>",
    "asset_type": "character",
    "title": "主角",
    "summary": "戴红围巾的小狐狸",
    "tags": ["主角"]
  },
  "response_filter": ".data | {uuid,asset_type,title,summary,tags,revision}"
}
```

### 接口约束

- 已有项目图片、现有 variant 的 Asset UUID 或其他 Thread 的 File 不可作为 `file_uuid`。
- 成功消费同一图片来源的重放是幂等恢复，不得创建第二个逻辑 Asset。

## `PATCH /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}`

更新 Premise Asset 元数据，并可同时替换为当前 Thread 新生成的图片。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `premise_asset_uuid` | path | UUIDv7 字符串 | 是 | 要更新的 Asset UUID。 |
| `expected_revision` | body | 非负整数 | 是 | 刚读取到的 Asset revision。 |
| `file_uuid` | body | UUIDv7 字符串 | 否 | 当前 Thread 中 `image_gen` 刚生成、用途匹配且尚未消费的 File；ready upload 应使用 variant 创建接口。 |
| `asset_type` | body | 枚举字符串 | 否 | `character`、`scene`、`prop` 或 `reference`。 |
| `title` | body | 非空字符串，最长 160 字符 | 否 | 新标题。 |
| `summary` | body | 字符串，最长 12000 字符 | 否 | 新简介。 |
| `tags` | body | 字符串数组 | 否 | 最多 64 项、每项最长 64 字符；空数组清空标签。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Asset UUID。 |
| `data.asset_type` | 字符串 | 更新后的类型。 |
| `data.title` | 字符串 | 更新后的标题。 |
| `data.summary` | 字符串 | 更新后的简介。 |
| `data.tags` | 字符串数组 | 更新后的标签。 |
| `data.current_variant` | 对象或 `null` | 当前 variant；替换图片后为新建并选中的 variant，包含 `uuid`、`version_no`、`source_type`、`crop`、`asset` 和 `created_at`。 |
| `data.current_variant.asset` | 对象 | 当前 variant 非空时返回；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.revision` | 整数 | 更新后的 revision。 |
| `data.deleted_at` | RFC 3339 字符串，可省略 | active 资源省略。 |

### request_api 示例

```json
{
  "method": "PATCH",
  "url": "/api/v1/projects/<project_uuid>/premise-assets/<premise_asset_uuid>",
  "request_body": {
    "summary": "戴红围巾、提着黄铜灯的小狐狸",
    "expected_revision": 3
  },
  "response_filter": ".data | {uuid,asset_type,title,summary,tags,revision}"
}
```

### 接口约束

- 使用乐观并发；冲突后重新 GET，再基于最新 revision 修改。
- 只提交 `expected_revision` 且状态未变化时可作为幂等读取返回当前对象。

## `DELETE /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}`

将 active Premise Asset 移入回收站。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `premise_asset_uuid` | path | UUIDv7 字符串 | 是 | 要移入回收站的 Asset UUID。 |
| `expected_revision` | body | 非负整数 | 是 | Asset 最新 revision。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Asset UUID。 |
| `data.asset_type` | 字符串 | Asset 类型。 |
| `data.title` | 字符串 | 标题。 |
| `data.summary` | 字符串 | 简介。 |
| `data.tags` | 字符串数组 | 标签。 |
| `data.current_variant` | 对象或 `null` | 当前图片 variant。 |
| `data.current_variant.asset` | 对象 | 当前 variant 非空时返回；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.revision` | 整数 | 删除后递增的 revision。 |
| `data.deleted_at` | RFC 3339 字符串 | 移入回收站时间。 |

### request_api 示例

```json
{
  "method": "DELETE",
  "url": "/api/v1/projects/<project_uuid>/premise-assets/<premise_asset_uuid>",
  "request_body": {
    "expected_revision": 3
  },
  "response_filter": ".data | {uuid,asset_type,title,revision,deleted_at}"
}
```

### 接口约束

- 危险操作；完整请求首次提交后，按 Overview 的确认协议处理。
- revision 冲突或资源不是 active 时不得盲目重试。

## `POST /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/restorations`

从回收站恢复 Premise Asset。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `premise_asset_uuid` | path | UUIDv7 字符串 | 是 | 要恢复的 Asset UUID。 |
| `expected_revision` | body | 整数，0～2147483647 | 是 | 回收站对象的最新 revision。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Asset UUID。 |
| `data.asset_type` | 字符串 | Asset 类型。 |
| `data.title` | 字符串 | 标题。 |
| `data.summary` | 字符串 | 简介。 |
| `data.tags` | 字符串数组 | 标签。 |
| `data.current_variant` | 对象或 `null` | 当前图片 variant。 |
| `data.current_variant.asset` | 对象 | 当前 variant 非空时返回；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.revision` | 整数 | 恢复后递增的 revision。 |
| `data.deleted_at` | RFC 3339 字符串，可省略 | 恢复成功后省略。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/<project_uuid>/premise-assets/<premise_asset_uuid>/restorations",
  "request_body": {
    "expected_revision": 4
  },
  "response_filter": ".data | {uuid,asset_type,title,revision,deleted_at}"
}
```

### 接口约束

- 目标必须仍在回收站；使用最新 revision 做乐观并发。

## `GET /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/variants`

按版本顺序列出 Premise Asset 的所有图片 variant。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `premise_asset_uuid` | path | UUIDv7 字符串 | 是 | 所属 Premise Asset UUID。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | 数组 | Variant 列表。 |
| `data.items[].uuid` | UUIDv7 字符串 | Variant UUID。 |
| `data.items[].version_no` | 整数 | 单个 Asset 内递增的版本号。 |
| `data.items[].source_type` | 字符串 | 图片来源类型。 |
| `data.items[].source_setting_image_uuid` | UUIDv7 字符串，可省略 | 来源 Setting Image UUID。 |
| `data.items[].crop` | 任意 JSON 值或 `null` | Provider/导入流程保留的裁剪参数；未提供时为空。 |
| `data.items[].asset` | 对象 | 公开图片 Asset；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.items[].created_at` | RFC 3339 字符串 | 创建时间。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/<project_uuid>/premise-assets/<premise_asset_uuid>/variants",
  "response_filter": ".data.items[] | {uuid,version_no,source_type,source_setting_image_uuid,created_at}"
}
```

### 接口约束

- 不传 `query` 或 `request_body`。

## `POST /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/variants`

从 ready upload 创建并选择新的图片 variant。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `premise_asset_uuid` | path | UUIDv7 字符串 | 是 | 所属 Premise Asset UUID。 |
| `upload_uuid` | body | UUIDv7 字符串 | 是 | 当前项目用途匹配、ready 且尚未消费的 upload UUID。 |
| `expected_revision` | body | 整数，0～2147483647 | 是 | Asset 最新 revision。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Asset UUID。 |
| `data.asset_type` | 字符串 | Asset 类型。 |
| `data.title` | 字符串 | 标题。 |
| `data.summary` | 字符串 | 简介。 |
| `data.tags` | 字符串数组 | 标签。 |
| `data.current_variant` | 对象 | 新建且已选中的 variant；包含 `uuid`、`version_no`、`source_type`、`crop`、`asset` 和 `created_at`。 |
| `data.current_variant.asset` | 对象 | 新 variant 的公开 Asset；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.revision` | 整数 | 创建后递增的 revision。 |
| `data.deleted_at` | RFC 3339 字符串，可省略 | active 资源省略。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/<project_uuid>/premise-assets/<premise_asset_uuid>/variants",
  "request_body": {
    "upload_uuid": "<upload_uuid>",
    "expected_revision": 3
  },
  "response_filter": ".data | {uuid,title,current_variant,revision}"
}
```

### 接口约束

- 创建与选择在同一写入中完成；upload 只能消费一次。
- revision 冲突后重新读取 Asset。

## `POST /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/variants/{variant_uuid}/selections`

选择一个既有图片 variant 作为当前图片。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `premise_asset_uuid` | path | UUIDv7 字符串 | 是 | 所属 Premise Asset UUID。 |
| `variant_uuid` | path | UUIDv7 字符串 | 是 | 要选择的 Variant UUID。 |
| `expected_revision` | body | 整数，0～2147483647 | 是 | Asset 最新 revision。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Asset UUID。 |
| `data.asset_type` | 字符串 | Asset 类型。 |
| `data.title` | 字符串 | 标题。 |
| `data.summary` | 字符串 | 简介。 |
| `data.tags` | 字符串数组 | 标签。 |
| `data.current_variant` | 对象 | 已选中的 variant；包含 `uuid`、`version_no`、`source_type`、`source_setting_image_uuid`、`crop`、`asset` 和 `created_at`。 |
| `data.current_variant.asset` | 对象 | 已选 variant 的公开 Asset；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.revision` | 整数 | 选择后递增的 revision。 |
| `data.deleted_at` | RFC 3339 字符串，可省略 | active 资源省略。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/<project_uuid>/premise-assets/<premise_asset_uuid>/variants/<variant_uuid>/selections",
  "request_body": {
    "expected_revision": 4
  },
  "response_filter": ".data | {uuid,title,current_variant,revision}"
}
```

### 接口约束

- Variant 必须属于路径中的 Asset；使用 Asset revision 做乐观并发。

## `DELETE /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}/permanent`

永久删除一个已在回收站的 Premise Asset，并报告关联文件处理统计。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `premise_asset_uuid` | path | UUIDv7 字符串 | 是 | 要永久删除的 Asset UUID。 |
| `expected_revision` | query | 整数，0～2147483647 | 是 | 回收站对象的最新 revision；必须放在 `query`，不放在 body 或 URL。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.deleted_count` | 整数 | 本次永久删除的 Premise Asset 数，成功时通常为 `1`。 |
| `data.file_soft_deleted_count` | 整数 | 因无其他引用而被软删除的文件数。 |
| `data.retained_file_count` | 整数 | 仍被其他公开资源引用而保留的文件数。 |
| `data.blocked_items` | 对象数组 | 成功永久删除时固定为空；目标受阻时接口返回失败信封而不是成功统计。 |
| `data.blocked_items[].uuid` | UUIDv7 字符串 | 仅说明统一统计结构的元素类型；本接口成功时不存在该元素。 |
| `data.blocked_items[].reason` | 字符串 | 仅说明统一统计结构的公开阻塞原因；本接口成功时不存在该元素。 |

### request_api 示例

```json
{
  "method": "DELETE",
  "url": "/api/v1/projects/<project_uuid>/premise-assets/<premise_asset_uuid>/permanent",
  "query": {
    "expected_revision": 4
  },
  "response_filter": ".data | {deleted_count,file_soft_deleted_count,retained_file_count,blocked_items}"
}
```

### 接口约束

- 危险且不可恢复；完整请求首次提交后，按 Overview 的确认协议处理。
- 目标必须已在回收站，且 `query.expected_revision` 必须匹配；不传 `request_body`。
- 引用安全检查失败时不会强行删除文件，并返回 `success=false`、`data=null`、`error.code=premise_asset_delete_blocked` 的失败信封。
- 成功信封中的 `blocked_items` 固定为空数组；只有清空回收站接口会在成功信封中逐项报告可跳过的阻塞项。

## `DELETE /api/v1/projects/{project_uuid}/premise-assets/trash`

清空 Premise Asset 回收站，并逐项报告删除、文件保留和阻塞统计。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.deleted_count` | 整数 | 本次永久删除的 Premise Asset 数。 |
| `data.file_soft_deleted_count` | 整数 | 因无其他引用而被软删除的文件数。 |
| `data.retained_file_count` | 整数 | 仍被其他资源引用而保留的文件数。 |
| `data.blocked_items` | 数组 | 未删除的阻塞项。 |
| `data.blocked_items[].uuid` | UUIDv7 字符串 | 被阻塞 Premise Asset UUID。 |
| `data.blocked_items[].reason` | 字符串 | 安全的阻塞原因。 |

### request_api 示例

```json
{
  "method": "DELETE",
  "url": "/api/v1/projects/<project_uuid>/premise-assets/trash",
  "response_filter": ".data | {deleted_count,file_soft_deleted_count,retained_file_count,blocked_items}"
}
```

### 接口约束

- 危险且不可恢复；完整请求首次提交后，按 Overview 的确认协议处理。
- 不传 `query` 或 `request_body`；空回收站调用幂等返回零统计。
- 单个阻塞项不会回滚其他可安全删除项，必须检查 `blocked_items`。
