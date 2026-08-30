# Premise API

生成设定图和拆解任务见 `/api/v1/agent-docs/api/generation.md`。

## `GET /api/v1/projects/{project_uuid}/premise`

读取项目当前 Premise、当前来源和已选设定图。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Premise 公开 UUID。 |
| `data.default_style` | 字符串 | 当前项目整体画风。 |
| `data.current_source` | 对象或 `null` | 当前 Premise 来源；尚无来源时为 `null`。 |
| `data.current_source.uuid` | UUIDv7 字符串 | Source 公开 UUID。 |
| `data.current_source.source_type` | 字符串 | 来源类型：`manual` 或 `generated`。 |
| `data.current_source.source_text` | 字符串 | 完整来源文本。 |
| `data.current_source.style_snapshot` | 字符串 | 创建来源时保存的画风快照。 |
| `data.current_source.provider_uuid` | UUIDv7 字符串，可省略 | 生成来源使用的 Provider。 |
| `data.current_source.model` | 字符串，可省略 | 生成来源使用的模型。 |
| `data.current_source.parameters` | JSON 对象 | 来源参数快照。 |
| `data.current_source.ignored_at` | RFC 3339 字符串，可省略 | 来源被忽略的时间。 |
| `data.current_source.revision` | 整数 | Source 当前 revision。 |
| `data.current_source.created_at` | RFC 3339 字符串 | Source 创建时间。 |
| `data.current_setting_image` | 对象或 `null` | 当前选中的设定图；尚未选择时为 `null`。 |
| `data.current_setting_image.uuid` | UUIDv7 字符串 | Setting Image 公开 UUID。 |
| `data.current_setting_image.source_uuid` | UUIDv7 字符串，可省略 | 关联 Source UUID。 |
| `data.current_setting_image.origin` | 字符串 | 图片来源，如 `manual` 或 `generated`。 |
| `data.current_setting_image.prompt` | 字符串 | 图片说明或生成提示。 |
| `data.current_setting_image.asset` | 对象 | 公开图片 Asset；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.current_setting_image.created_at` | RFC 3339 字符串 | Setting Image 创建时间。 |
| `data.revision` | 整数 | Premise 当前乐观并发版本。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/<project_uuid>/premise",
  "response_filter": ".data | {uuid,default_style,current_source,current_setting_image,revision}"
}
```

### 接口约束

- 不传 `query` 或 `request_body`。
- 更新前应从本接口读取最新 `revision`。

## `PATCH /api/v1/projects/{project_uuid}/premise`

替换项目整体画风并推进 Premise revision。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `default_style` | body | 字符串，最长 8192 字符 | 是 | 新的完整画风描述；空白值会恢复当前语言的系统默认画风。 |
| `expected_revision` | body | 整数，0～2147483647 | 是 | 刚读取到的 Premise revision。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Premise 公开 UUID。 |
| `data.default_style` | 字符串 | 更新后的整体画风。 |
| `data.current_source` | 对象或 `null` | 当前来源；包含 `uuid`、`source_type`、`source_text`、`style_snapshot`、`ignored_at`、`revision` 和 `created_at`。 |
| `data.current_setting_image` | 对象或 `null` | 当前设定图；包含 `uuid`、`source_uuid`、`origin`、`prompt`、`asset` 和 `created_at`。 |
| `data.current_setting_image.asset` | 对象 | 当前设定图非空时返回；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.revision` | 整数 | 更新后的 revision。 |

### request_api 示例

```json
{
  "method": "PATCH",
  "url": "/api/v1/projects/<project_uuid>/premise",
  "request_body": {
    "default_style": "温暖的水彩绘本，柔和纸张肌理",
    "expected_revision": 3
  },
  "response_filter": ".data | {uuid,default_style,revision}"
}
```

### 接口约束

- 使用乐观并发；revision 冲突后重新 GET，再基于事实状态修改。
- `request_body` 不接受未列出的字段。

## `GET /api/v1/projects/{project_uuid}/premise-sources`

按页读取项目的 Premise Source 历史。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `page` | query | 整数，1～1000000 | 否 | 页码，默认 `1`。 |
| `per_page` | query | 整数，1～100 | 否 | 每页数量，默认 `20`。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | 数组 | Source 列表。 |
| `data.items[].uuid` | UUIDv7 字符串 | Source 公开 UUID。 |
| `data.items[].source_type` | 字符串 | `manual` 或 `generated`。 |
| `data.items[].source_text` | 字符串 | 完整来源文本。 |
| `data.items[].style_snapshot` | 字符串 | 画风快照。 |
| `data.items[].ignored_at` | RFC 3339 字符串，可省略 | 被忽略的时间；未忽略时省略。 |
| `data.items[].revision` | 整数 | Source 当前 revision。 |
| `data.items[].created_at` | RFC 3339 字符串 | 创建时间。 |
| `data.pagination` | 对象 | 页码分页信息。 |
| `data.pagination.per_page` | 整数 | 实际每页数量。 |
| `data.pagination.current_page` | 整数 | 当前页码。 |
| `data.pagination.last_page` | 整数 | 最后一页页码。 |
| `data.pagination.total` | 整数 | Source 总数。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/<project_uuid>/premise-sources",
  "query": {
    "page": 1,
    "per_page": 20
  },
  "response_filter": ".data.items[] | {uuid,source_type,ignored_at,revision,created_at}"
}
```

### 接口约束

- `query` 只接受 `page` 与 `per_page`；两者均可省略。

## `POST /api/v1/projects/{project_uuid}/premise-sources`

创建一个 Premise Source，并将其设为当前来源。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `source_text` | body | 非空字符串，最长 262144 字符 | 是 | 完整 Premise 来源文本。 |
| `style_snapshot` | body | 字符串，最长 8192 字符 | 是 | 创建时使用的整体画风快照。 |
| `source_type` | body | 枚举字符串 | 是 | `manual` 或 `generated`。 |
| `model` | body | 字符串，最长 512 字符 | 否 | 记录实际生成来源时使用的模型；手工来源通常省略。 |
| `parameters` | body | JSON 对象 | 否 | 记录生成参数；允许任意公开 JSON 字段，省略时保存为空对象。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | 新 Source UUID。 |
| `data.source_type` | 字符串 | 来源类型。 |
| `data.source_text` | 字符串 | 规范化后的来源文本。 |
| `data.style_snapshot` | 字符串 | 画风快照。 |
| `data.ignored_at` | RFC 3339 字符串，可省略 | 新建 Source 默认未忽略，因此通常省略。 |
| `data.revision` | 整数 | 初始 revision。 |
| `data.created_at` | RFC 3339 字符串 | 创建时间。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/<project_uuid>/premise-sources",
  "request_body": {
    "source_text": "故事发生在漂浮于云海之上的灯塔群。",
    "style_snapshot": "温暖水彩与柔和纸张肌理",
    "source_type": "manual"
  },
  "response_filter": ".data | {uuid,source_type,ignored_at,revision,created_at}"
}
```

### 接口约束

- 创建会同步更新 Premise 的 `current_source` 并推进 Premise revision。
- `model` 和 `parameters` 只用于如实记录实际生成来源，不应臆造。

## `PATCH /api/v1/projects/{project_uuid}/premise-sources/{source_uuid}`

切换一个 Premise Source 的忽略状态。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `source_uuid` | path | UUIDv7 字符串 | 是 | 要修改的 Source UUID。 |
| `ignored` | body | 布尔值 | 是 | `true` 表示忽略，`false` 表示恢复使用。 |
| `expected_revision` | body | 整数，0～2147483647 | 是 | Source 最新 revision。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Source UUID。 |
| `data.source_type` | 字符串 | 来源类型。 |
| `data.source_text` | 字符串 | 来源文本。 |
| `data.style_snapshot` | 字符串 | 画风快照。 |
| `data.ignored_at` | RFC 3339 字符串，可省略 | 忽略时间；恢复后省略。 |
| `data.revision` | 整数 | 更新后的 revision。 |
| `data.created_at` | RFC 3339 字符串 | 创建时间。 |

### request_api 示例

```json
{
  "method": "PATCH",
  "url": "/api/v1/projects/<project_uuid>/premise-sources/<source_uuid>",
  "request_body": {
    "ignored": true,
    "expected_revision": 3
  },
  "response_filter": ".data | {uuid,source_type,ignored_at,revision,created_at}"
}
```

### 接口约束

- 使用 Source revision 做乐观并发；冲突后重新读取 Source 列表。

## `GET /api/v1/projects/{project_uuid}/premise-setting-images`

列出全部设定图，或按 Source UUID 集合筛选。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `source_uuids` | query | UUIDv7 字符串数组，最多 100 项 | 否 | 仅返回关联任一 Source 的设定图；重复 UUID 会去重。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | 数组 | Setting Image 列表。 |
| `data.items[].uuid` | UUIDv7 字符串 | Setting Image UUID。 |
| `data.items[].source_uuid` | UUIDv7 字符串，可省略 | 关联 Source UUID。 |
| `data.items[].origin` | 字符串 | 图片来源，如 `manual` 或 `generated`。 |
| `data.items[].prompt` | 字符串 | 图片说明或生成提示。 |
| `data.items[].asset` | 对象 | 公开图片 Asset；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.items[].created_at` | RFC 3339 字符串 | Setting Image 创建时间。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/<project_uuid>/premise-setting-images",
  "query": {
    "source_uuids": ["<source_uuid>"]
  },
  "response_filter": ".data.items[] | {uuid,source_uuid,origin,created_at}"
}
```

### 接口约束

- 省略 `source_uuids` 时返回项目全部 Setting Image。
- 数组只允许公开 Source UUID，不接受内部 ID。

## `POST /api/v1/projects/{project_uuid}/premise-setting-images`

把当前项目尚未消费的 ready upload 导入为 Setting Image，并将其设为当前设定图。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `upload_uuid` | body | UUIDv7 字符串 | 是 | 当前项目用途匹配、状态为 ready 且尚未消费的 upload UUID。 |
| `source_uuid` | body | UUIDv7 字符串 | 否 | 关联的 Premise Source UUID。 |
| `prompt` | body | 字符串，最长 262144 字符 | 否 | 图片说明；省略时为空字符串。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | 新 Setting Image UUID。 |
| `data.source_uuid` | UUIDv7 字符串，可省略 | 关联 Source UUID。 |
| `data.origin` | 字符串 | 导入图片为 `manual`。 |
| `data.prompt` | 字符串 | 规范化后的图片说明。 |
| `data.asset` | 对象 | 已持久化的公开 Asset；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.created_at` | RFC 3339 字符串 | 创建时间。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/<project_uuid>/premise-setting-images",
  "request_body": {
    "upload_uuid": "<upload_uuid>",
    "source_uuid": "<source_uuid>",
    "prompt": "云海灯塔群的远景设定图"
  },
  "response_filter": ".data | {uuid,source_uuid,origin,created_at}"
}
```

### 接口约束

- upload 只能成功消费一次；重放同一成功请求返回既有 Setting Image，不重复创建。
- `source_uuid` 若提供，必须属于当前项目。

## `POST /api/v1/projects/{project_uuid}/premise-setting-images/{setting_image_uuid}/selections`

将指定 Setting Image 选为 Premise 当前设定图。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `setting_image_uuid` | path | UUIDv7 字符串 | 是 | 要选择的 Setting Image UUID。 |
| `request_body` | body | 空 JSON 对象 | 是 | 必须为 `{}`。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Premise UUID。 |
| `data.default_style` | 字符串 | 当前整体画风。 |
| `data.current_source` | 对象或 `null` | 当前来源；非空时包含 `uuid`、`source_type`、`source_text`、`style_snapshot`、`ignored_at`、`revision` 和 `created_at`。 |
| `data.current_setting_image` | 对象 | 选中的 Setting Image；包含 `uuid`、`source_uuid`、`origin`、`prompt`、`asset` 和 `created_at`。 |
| `data.current_setting_image.asset` | 对象 | 选中图片的公开 Asset；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.revision` | 整数 | 选择后递增的 Premise revision。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/<project_uuid>/premise-setting-images/<setting_image_uuid>/selections",
  "request_body": {},
  "response_filter": ".data | {uuid,default_style,current_setting_image,revision}"
}
```

### 接口约束

- Setting Image 必须属于当前项目。
- 选择会推进 Premise revision；本接口不接收 `expected_revision`。
