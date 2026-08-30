# Project API

项目元数据接口；字段只覆盖 Agent reviewed contract 暴露的公开投影。

## `GET /api/v1/projects/{project_uuid}`

读取项目名称、生成语言、版本与章节计数。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目的公开 UUIDv7。 |

不接收 `query` 或 `request_body`。

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 项目公开 UUIDv7。 |
| `data.name` | string | 项目名称。 |
| `data.description` | string | 项目简介。 |
| `data.generation_language` | string | 生成语言：`zh-Hans` 或 `en`。 |
| `data.revision` | integer | 项目元数据的当前乐观并发版本。 |
| `data.chapter_count` | integer | active Chapter 数量。 |
| `data.trash_count` | integer | 回收站中的 Chapter 数量。 |
| `data.updated_at` | string(date-time) | 项目元数据最后更新时间。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001",
  "response_filter": ".data | {uuid,name,generation_language,revision,chapter_count,trash_count,updated_at}"
}
```

### 接口约束

无额外状态或跨字段约束。

## `PATCH /api/v1/projects/{project_uuid}`

基于最新 revision 更新项目名称、简介，并可切换生成语言。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目的公开 UUIDv7。 |
| `name` | body | string | 是 | 完整项目名称；去除首尾空白后须为 1–120 个字符。 |
| `description` | body | string | 是 | 完整项目简介；最多 2,000 个字符，可为空字符串。 |
| `generation_language` | body | string | 否 | 生成语言枚举：`zh-Hans`、`en`；省略时保持原值。 |
| `expected_revision` | body | integer | 是 | 刚读取到的 `data.revision`；范围 0–2,147,483,647。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 项目公开 UUIDv7。 |
| `data.name` | string | 保存后的项目名称。 |
| `data.description` | string | 保存后的项目简介。 |
| `data.generation_language` | string | 保存后的生成语言。 |
| `data.revision` | integer | 更新后的乐观并发版本。 |
| `data.chapter_count` | integer | active Chapter 数量。 |
| `data.trash_count` | integer | 回收站中的 Chapter 数量。 |
| `data.updated_at` | string(date-time) | 本次更新时间。 |

### request_api 示例

```json
{
  "method": "PATCH",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001",
  "request_body": {
    "name": "月光邮局",
    "description": "一只狐狸替人们寄送未说出口的话。",
    "expected_revision": 3
  },
  "response_filter": ".data | {uuid,name,generation_language,revision,updated_at}"
}
```

### 接口约束

- `expected_revision` 必须与当前项目版本一致；冲突后重新读取，不得盲目重试。
- `name` 与 `description` 是完整替换值，即使只改其中之一也必须同时提交。
- 更改 `generation_language` 会同步迁移项目提示词语言，整个更新在同一事务内完成。
