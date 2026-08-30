# Chapter API

Chapter 负责章节元数据、当前正文、不可变正文历史和回收站生命周期。章节正文生成的唯一详细定义见 [Generation API](./generation.md)。

## `GET /api/v1/projects/{project_uuid}/chapters`

按状态列出当前项目的 Chapter。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `state` | query | string | 否 | `active` 或 `trashed`；默认 `active`。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | array<object> | Chapter 列表；无结果时为空数组。 |
| `data.items[].uuid` | string(UUIDv7) | Chapter 公开 UUIDv7。 |
| `data.items[].chapter_code` | string | 章节业务编号，如 `vol01.ch01`。 |
| `data.items[].title` | string | 章节标题。 |
| `data.items[].revision` | integer | 当前乐观并发版本。 |
| `data.items[].trashed_at` | string(date-time) \| null | 回收时间；active Chapter 为 `null`。 |
| `data.items[].current_story` | object \| null | 当前正文；非空时公开 `uuid`、`version_no`、`source_type`、`source_uuid`、`source_item_uuid`、`content`、`content_format`、`char_count`、`created_at`。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters",
  "query": {"state": "active"},
  "response_filter": ".data.items[] | {uuid,chapter_code,title,revision,trashed_at}"
}
```

### 接口约束

- `state=active` 与 `state=trashed` 是互斥的完整集合选择，不支持同时返回两类状态。

## `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}`

读取一个 active 或已回收的 Chapter，并取得后续写操作所需的最新 revision。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 目标 Chapter 公开 UUIDv7。 |

不接收 `query` 或 `request_body`。

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Chapter 公开 UUIDv7。 |
| `data.chapter_code` | string | 章节业务编号。 |
| `data.title` | string | 章节标题。 |
| `data.revision` | integer | 当前乐观并发版本。 |
| `data.trashed_at` | string(date-time) \| null | 回收时间；active Chapter 为 `null`。 |
| `data.current_story` | object \| null | 当前正文；非空时公开 `uuid`、`version_no`、`source_type`、`source_uuid`、`source_item_uuid`、`content`、`content_format`、`char_count`、`created_at`。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters/01970000-0000-7000-8000-000000000002",
  "response_filter": ".data | {uuid,chapter_code,title,revision,trashed_at,current_story}"
}
```

### 接口约束

无额外状态或跨字段约束。

## `POST /api/v1/projects/{project_uuid}/chapters`

创建 Chapter，并可同时写入第一个不可变正文版本。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `chapter_code` | body | string | 是 | 章节业务编号，格式 `vol{卷号}.ch{章号}`；卷号和章号至少两位且为正数，章号小于 100000；最多 64 个字符。 |
| `title` | body | string | 是 | 章节标题；最多 255 个字符。 |
| `content` | body | string | 否 | 完整初始正文；须为非空白 UTF-8 文本，最多 3000000 个字符。 |
| `content_format` | body | string | 否 | `txt` 或 `md`；提供 `content` 而省略本字段时默认 `txt`。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 新 Chapter 公开 UUIDv7。 |
| `data.chapter_code` | string | 规范化为小写的章节业务编号。 |
| `data.title` | string | 章节标题。 |
| `data.revision` | integer | 初始 revision；有初始正文时已包含正文写入产生的版本。 |
| `data.trashed_at` | null | 新 Chapter 为 active。 |
| `data.current_story` | object \| null | 初始正文版本；非空时公开 `uuid`、`version_no`、`source_type`、`source_uuid`、`source_item_uuid`、`content`、`content_format`、`char_count`、`created_at`。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters",
  "request_body": {
    "chapter_code": "vol01.ch01",
    "title": "第一章",
    "content": "月光邮局在午夜开门。",
    "content_format": "md"
  },
  "response_filter": ".data | {uuid,chapter_code,title,revision}"
}
```

### 接口约束

- active Chapter 的 `chapter_code` 与排序位置必须在项目内唯一。
- 提供 `content` 时可省略 `content_format` 以使用 `txt`；不提供 `content` 时应同时省略 `content_format`。
- 此接口只创建 Chapter，不创建异步生成任务。

## `PATCH /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}`

基于最新 revision 完整替换 active Chapter 的标题。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 目标 active Chapter 公开 UUIDv7。 |
| `title` | body | string | 是 | 新标题；最多 255 个字符。 |
| `expected_revision` | body | integer | 是 | 刚读取到的 Chapter revision；范围 0–2,147,483,647。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Chapter 公开 UUIDv7。 |
| `data.chapter_code` | string | 章节业务编号。 |
| `data.title` | string | 保存后的标题。 |
| `data.revision` | integer | 更新后的 revision。 |
| `data.trashed_at` | null | 该接口仅更新 active Chapter。 |
| `data.current_story` | object \| null | 未被本接口修改的当前正文；非空时公开 `uuid`、`version_no`、`source_type`、`source_uuid`、`source_item_uuid`、`content`、`content_format`、`char_count`、`created_at`。 |

### request_api 示例

```json
{
  "method": "PATCH",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters/01970000-0000-7000-8000-000000000002",
  "request_body": {"title": "午夜来信", "expected_revision": 3},
  "response_filter": ".data | {uuid,chapter_code,title,revision}"
}
```

### 接口约束

- Chapter 必须处于 active 状态。
- `expected_revision` 必须匹配当前 revision；冲突后重新读取。

## `PUT /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/current-story`

为 active Chapter 追加一个不可变正文版本，并把它设为当前正文。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 目标 active Chapter 公开 UUIDv7。 |
| `content` | body | string | 是 | 完整替换正文，不是增量片段；须为非空白 UTF-8 文本。 |
| `content_format` | body | string | 是 | 正文格式枚举：`txt`、`md`。 |
| `expected_revision` | body | integer | 是 | 刚读取到的 Chapter revision。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Chapter 公开 UUIDv7。 |
| `data.chapter_code` | string | 章节业务编号。 |
| `data.title` | string | 章节标题。 |
| `data.revision` | integer | 正文变更后的 revision；提交相同内容与格式时可能保持不变。 |
| `data.trashed_at` | null | 该接口仅写入 active Chapter。 |
| `data.current_story` | object | 当前正文版本，公开 `uuid`、`version_no`、`source_type`、`source_uuid`、`source_item_uuid`、`content`、`content_format`、`char_count`、`created_at`。 |

### request_api 示例

```json
{
  "method": "PUT",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters/01970000-0000-7000-8000-000000000002/current-story",
  "request_body": {
    "content": "月光邮局在午夜开门。",
    "content_format": "md",
    "expected_revision": 3
  },
  "response_filter": ".data | {uuid,revision,current_story}"
}
```

### 接口约束

- Chapter 必须处于 active 状态。
- `content` 是完整正文；不得按补丁或续写片段提交。
- `expected_revision` 必须匹配当前 revision；内容和格式均与当前版本相同时接口幂等返回当前 Chapter。

## `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/stories`

按版本号从新到旧读取一个 Chapter 的不可变正文历史，最多返回 200 个版本。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | active 或已回收 Chapter 的公开 UUIDv7。 |

不接收 `query` 或 `request_body`。

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | array<object> | 正文版本列表；无正文时为空数组。 |
| `data.items[].uuid` | string(UUIDv7) | 正文版本公开 UUIDv7。 |
| `data.items[].version_no` | integer | 在当前 Chapter 内递增的版本号。 |
| `data.items[].source_type` | string | 版本来源类型。 |
| `data.items[].source_uuid` | string(UUIDv7) | 公开来源 UUIDv7。 |
| `data.items[].source_item_uuid` | string(UUIDv7) | 公开来源项 UUIDv7。 |
| `data.items[].content` | string | 该版本的完整正文。 |
| `data.items[].content_format` | string | `txt` 或 `md`。 |
| `data.items[].char_count` | integer | Unicode 字符数。 |
| `data.items[].created_at` | string(date-time) | 版本创建时间。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters/01970000-0000-7000-8000-000000000002/stories",
  "response_filter": ".data.items[] | {uuid,version_no,source_type,content_format,char_count,created_at}"
}
```

### 接口约束

- 历史版本只读，不支持原地修改。

## `DELETE /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}`

把 active Chapter 移入回收站，保留正文与恢复能力。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 目标 active Chapter 公开 UUIDv7。 |
| `expected_revision` | body | integer | 是 | 刚读取到的 Chapter revision；范围 0–2,147,483,647。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 已回收 Chapter 公开 UUIDv7。 |
| `data.chapter_code` | string | 章节业务编号。 |
| `data.title` | string | 章节标题。 |
| `data.revision` | integer | 回收后的 revision。 |
| `data.trashed_at` | string(date-time) | 本次回收时间。 |
| `data.current_story` | object \| null | 保留的当前正文；非空时公开 `uuid`、`version_no`、`source_type`、`source_uuid`、`source_item_uuid`、`content`、`content_format`、`char_count`、`created_at`。 |

### request_api 示例

```json
{
  "method": "DELETE",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters/01970000-0000-7000-8000-000000000002",
  "request_body": {"expected_revision": 3},
  "response_filter": ".data | {uuid,chapter_code,title,revision,trashed_at}"
}
```

### 接口约束

- Chapter 必须处于 active 状态，且 `expected_revision` 必须匹配当前 revision。
- 这是危险操作，执行前需要确认；重复回收已在回收站的 Chapter 会返回状态冲突。

## `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/restorations`

基于最新 revision 将一个已回收 Chapter 恢复为 active。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 回收站 Chapter 公开 UUIDv7。 |
| `expected_revision` | body | integer | 是 | 回收站对象当前 revision；范围 0–2,147,483,647。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 恢复后的 Chapter 公开 UUIDv7。 |
| `data.chapter_code` | string | 章节业务编号。 |
| `data.title` | string | 章节标题。 |
| `data.revision` | integer | 恢复后的 revision。 |
| `data.trashed_at` | null | 恢复成功后为空。 |
| `data.current_story` | object \| null | 恢复的当前正文；非空时公开 `uuid`、`version_no`、`source_type`、`source_uuid`、`source_item_uuid`、`content`、`content_format`、`char_count`、`created_at`。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters/01970000-0000-7000-8000-000000000002/restorations",
  "request_body": {"expected_revision": 4},
  "response_filter": ".data | {uuid,chapter_code,title,revision,trashed_at}"
}
```

### 接口约束

- Chapter 必须在回收站，且 `expected_revision` 必须匹配当前 revision。
- active 集合中不得已有相同章节编号或排序位置；存在冲突时恢复失败。

## `DELETE /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/permanent`

永久删除一个已回收 Chapter 及其正文历史。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 回收站 Chapter 公开 UUIDv7。 |
| `expected_revision` | query | integer | 是 | 回收站对象当前 revision；范围 0–2,147,483,647。必须放在 `query`，不放在 body。 |

不接收 `request_body`。

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data` | null | 删除成功后固定为 `null`。 |

### request_api 示例

```json
{
  "method": "DELETE",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters/01970000-0000-7000-8000-000000000002/permanent",
  "query": {"expected_revision": 4},
  "response_filter": ".data"
}
```

### 接口约束

- Chapter 必须已在回收站，且 `query.expected_revision` 必须匹配当前 revision。
- 这是不可恢复的危险操作，执行前需要确认。
- 仍被 active 或 queued/running 任务引用时删除被阻塞，不会产生部分删除。

## `DELETE /api/v1/projects/{project_uuid}/chapters/trash`

尽可能永久删除回收站中的 Chapter，并逐项报告被活动引用阻塞的对象。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |

不接收 `query` 或 `request_body`。

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.deleted_count` | integer | 本次永久删除的 Chapter 数量。 |
| `data.blocked_items` | array<object> | 因活动引用而保留的 Chapter；没有阻塞项时为空数组。 |
| `data.blocked_items[].uuid` | string(UUIDv7) | 被阻塞 Chapter 公开 UUIDv7。 |
| `data.blocked_items[].chapter_code` | string | 被阻塞 Chapter 的业务编号。 |
| `data.blocked_items[].error_code` | string | 公开阻塞错误码。 |

### request_api 示例

```json
{
  "method": "DELETE",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters/trash",
  "response_filter": ".data | {deleted_count,blocked_items:{uuid,chapter_code,error_code}}"
}
```

### 接口约束

- 这是不可恢复的危险操作，执行前需要确认。
- 操作允许部分成功：可删除项被永久删除，受活动引用阻塞的项保留并进入 `blocked_items`。
- 回收站为空时幂等返回 `deleted_count=0` 与空 `blocked_items`。
