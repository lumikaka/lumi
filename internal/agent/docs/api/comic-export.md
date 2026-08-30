# Comic Export API

## `GET /api/v1/projects/{project_uuid}/comic-exports/readiness`

检查项目或单章当前是否具备可导出的图片快照。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `scope` | query | string | 是 | `project` 或 `chapter`。 |
| `chapter_uuid` | query | string(UUIDv7) | 条件必填 | `scope=chapter` 时必填，且必须是当前项目的 active Chapter；project 范围省略。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.scope` | string | 实际检查范围：`project` 或 `chapter`。 |
| `data.chapter_uuid` | string(UUIDv7)，可缺省 | chapter 范围的 Chapter UUID；project 范围缺省。 |
| `data.active_chapter_count` | integer | 范围内 active Chapter 数。 |
| `data.active_section_count` | integer | 范围内 active Section 数。 |
| `data.image_section_count` | integer | 当前图片可用于导出的 Section 数。 |
| `data.missing_section_count` | integer | 缺少可用当前图片的 Section 数。 |
| `data.can_export` | boolean | 是否至少存在可导出的正文图片。 |
| `data.complete` | boolean | `can_export=true` 且没有缺图 Section 时为 true。 |
| `data.missing_sections` | array<object> | 缺图项；每项含 `uuid:string(UUIDv7)`、`chapter_uuid:string(UUIDv7)`、`section_no:integer`、`page_role:string`、`body_page_no:integer`、`title:string`。特殊页的 `body_page_no` 会缺省。 |

### request_api 示例

```json
{"method":"GET","url":"/api/v1/projects/<project_uuid>/comic-exports/readiness","query":{"scope":"chapter","chapter_uuid":"<chapter_uuid>"},"response_filter":".data | {can_export,complete,missing_section_count,missing_sections:{uuid,page_role,body_page_no,title}}"}
```

### 接口约束

`scope=chapter` 与 `chapter_uuid` 必须同时使用；`scope=project` 时省略 `chapter_uuid`。`section_no` 是绝对装订顺序，`body_page_no` 仅为正文页提供从 1 开始的页码。

## `POST /api/v1/projects/{project_uuid}/comic-exports`

冻结当前内容并异步创建 ZIP 或 PDF 导出任务。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `scope` | body | string | 是 | `project` 或 `chapter`。 |
| `chapter_uuid` | body | string(UUIDv7) | 条件必填 | `scope=chapter` 时必填；project 范围省略。 |
| `format` | body | string | 是 | `zip` 或 `pdf`。 |
| `allow_missing_images` | body | boolean | 否 | 是否允许跳过缺图 Section；默认 `false`。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Production Task UUID。 |
| `data.kind` | string | 任务类型，值为 `comic_export`。 |
| `data.resource_uuid` | string(UUIDv7) | project 范围为 Project UUID，chapter 范围为 Chapter UUID。 |
| `data.status` | string | 任务状态，例如 `queued`、`running`、`completed`、`failed`。 |
| `data.error_code` | string，可缺省 | 公开错误码；无错误时缺省。 |
| `data.error_message` | string，可缺省 | 公开错误信息；无错误时缺省。 |

### request_api 示例

```json
{"method":"POST","url":"/api/v1/projects/<project_uuid>/comic-exports","request_body":{"scope":"chapter","chapter_uuid":"<chapter_uuid>","format":"pdf","allow_missing_images":false},"response_filter":".data | {uuid,kind,resource_uuid,status,error_code,error_message}"}
```

### 接口约束

这是异步写操作，后续通过 Task API 读取进度。创建前应读取 readiness；`can_export=false` 时不能创建，`complete=false` 时只有用户明确接受缺图才可传 `allow_missing_images=true`。`scope=chapter` 必须携带 `chapter_uuid`，project 范围必须省略。内容在检查后发生变化时，服务端拒绝以过期快照建任务；同一持久化调用恢复时按幂等键避免重复任务。

## `GET /api/v1/projects/{project_uuid}/comic-exports`

按创建时间倒序分页查询尚未过期的 Comic Export 记录。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `page` | query | integer，1–1000000 | 否 | 页码；默认 `1`。 |
| `per_page` | query | integer，1–100 | 否 | 每页数量；默认 `20`。 |
| `scope` | query | string | 否 | `project` 或 `chapter`。 |
| `chapter_uuid` | query | string(UUIDv7) | 否 | 按 Chapter 筛选；不能与 `scope=project` 同用。 |
| `task_uuid` | query | string(UUIDv7) | 否 | 按 Production Task 筛选。 |
| `snapshot_hash` | query | string，64 位小写十六进制 | 否 | 按冻结快照 SHA-256 筛选。 |
| `format` | query | string | 否 | `zip` 或 `pdf`。 |
| `status` | query | string，最长 64 字符 | 否 | `queued`、`running`、`ready`、`failed` 或 `cancelled`。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | array<object> | 当前页 Export；无结果时为空数组。 |
| `data.items[].uuid` | string(UUIDv7) | Export UUID。 |
| `data.items[].task_uuid` | string(UUIDv7) | 关联 Production Task UUID。 |
| `data.items[].scope` | string | `project` 或 `chapter`。 |
| `data.items[].chapter_uuid` | string(UUIDv7)，可缺省 | chapter 范围的 Chapter UUID。 |
| `data.items[].format` | string | `zip` 或 `pdf`。 |
| `data.items[].filename` | string | 下载文件名。 |
| `data.items[].status` | string | `queued`、`running`、`ready`、`failed` 或 `cancelled`。 |
| `data.items[].snapshot_hash` | string | 冻结快照 SHA-256。 |
| `data.items[].expires_at` | string(date-time) 或 null | 完成后的过期时间；未进入终态时为 null。 |
| `data.items[].retention_days` | integer | 完成后保留天数。 |
| `data.items[].byte_size` | integer | 已生成文件字节数；未完成时通常为 0。 |
| `data.items[].content_sha256` | string | 文件内容 SHA-256；未完成时为空字符串。 |
| `data.items[].error_code` | string，可缺省 | 失败错误码。 |
| `data.items[].created_at` | string(date-time) | 记录创建时间。 |
| `data.items[].completed_at` | string(date-time)，可缺省 | 进入终态的时间。 |
| `data.pagination` | object | 页码分页信息。 |
| `data.pagination.per_page` | integer | 当前每页数量。 |
| `data.pagination.current_page` | integer | 当前页码。 |
| `data.pagination.last_page` | integer | 最后一页页码。 |
| `data.pagination.total` | integer | 匹配总数。 |

### request_api 示例

```json
{"method":"GET","url":"/api/v1/projects/<project_uuid>/comic-exports","query":{"page":1,"per_page":20,"status":"ready"},"response_filter":".data.items[] | {uuid,task_uuid,scope,chapter_uuid,format,filename,status,expires_at}"}
```

### 接口约束

`scope=project` 不能与 `chapter_uuid` 同用。过期记录不会出现在列表中。
