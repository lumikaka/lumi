# Task API

Story Task 使用 `/tasks`；Premise、Comic 与 Export 等生产任务使用 `/production-tasks`。状态变化由 WebSocket 提示后再通过这些 REST 接口校准，不做定时 HTTP 轮询。

## `GET /api/v1/projects/{project_uuid}/tasks/{task_uuid}`

读取一个 Story Task 的当前公开状态。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `task_uuid` | path | string(UUIDv7) | 是 | Story Task 公开 UUIDv7。 |

不接收 `query` 或 `request_body`。

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Task 公开 UUIDv7。 |
| `data.kind` | string | Story 任务类型。 |
| `data.resource_uuid` | string(UUIDv7) | 任务目标资源公开 UUIDv7。 |
| `data.status` | string | `queued`、`running`、`waiting_for_input`、`completed`、`failed`、`cancelled` 或 `interrupted`。 |
| `data.error_code` | string，可省略 | 失败时的公开错误码。 |
| `data.error_message` | string，可省略 | 失败时的公开错误信息。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/tasks/01970000-0000-7000-8000-000000000008",
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- Task 是异步状态快照；`queued` 或 `running` 不表示业务结果已完成。

## `GET /api/v1/projects/{project_uuid}/production-tasks/{task_uuid}`

读取一个 Production Task 的当前公开状态。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `task_uuid` | path | string(UUIDv7) | 是 | Production Task 公开 UUIDv7。 |

不接收 `query` 或 `request_body`。

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Task 公开 UUIDv7。 |
| `data.kind` | string | Production 任务类型。 |
| `data.resource_uuid` | string(UUIDv7) | 任务目标资源公开 UUIDv7。 |
| `data.status` | string | `queued`、`running`、`waiting_for_input`、`completed`、`failed`、`cancelled` 或 `interrupted`。 |
| `data.error_code` | string，可省略 | 失败时的公开错误码。 |
| `data.error_message` | string，可省略 | 失败时的公开错误信息。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/production-tasks/01970000-0000-7000-8000-000000000009",
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- Task 是异步状态快照；图片或导出只能在 `completed` 后报告完成。

## `GET /api/v1/projects/{project_uuid}/tasks`

列出当前项目的 Story Task，可按公开状态过滤。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `status` | query | string | 否 | `queued`、`running`、`waiting_for_input`、`completed`、`failed`、`cancelled` 或 `interrupted`。 |
| `limit` | query | integer | 否 | 返回数量，范围 1–100；默认 50。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | array<object> | Story Task 列表；无结果时为空数组。 |
| `data.items[].uuid` | string(UUIDv7) | Task 公开 UUIDv7。 |
| `data.items[].kind` | string | Story 任务类型。 |
| `data.items[].resource_uuid` | string(UUIDv7) | 目标资源公开 UUIDv7。 |
| `data.items[].status` | string | 当前公开状态。 |
| `data.items[].error_code` | string，可省略 | 公开错误码。 |
| `data.items[].error_message` | string，可省略 | 公开错误信息。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/tasks",
  "query": {"status": "running", "limit": 20},
  "response_filter": ".data.items[] | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- 列表是异步状态快照，不提供分页 cursor；按需读取，不用于定时轮询。

## `GET /api/v1/projects/{project_uuid}/production-tasks`

列出当前项目的 Production Task，可按公开状态过滤。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `status` | query | string | 否 | `queued`、`running`、`waiting_for_input`、`completed`、`failed`、`cancelled` 或 `interrupted`。 |
| `limit` | query | integer | 否 | 返回数量，范围 1–100；默认 50。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | array<object> | Production Task 列表；无结果时为空数组。 |
| `data.items[].uuid` | string(UUIDv7) | Task 公开 UUIDv7。 |
| `data.items[].kind` | string | Production 任务类型。 |
| `data.items[].resource_uuid` | string(UUIDv7) | 目标资源公开 UUIDv7。 |
| `data.items[].status` | string | 当前公开状态。 |
| `data.items[].error_code` | string，可省略 | 公开错误码。 |
| `data.items[].error_message` | string，可省略 | 公开错误信息。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/production-tasks",
  "query": {"status": "failed", "limit": 20},
  "response_filter": ".data.items[] | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- 列表是异步状态快照，不提供分页 cursor；按需读取，不用于定时轮询。

## `GET /api/v1/projects/{project_uuid}/tasks/{task_uuid}/events`

用 sequence cursor 读取一个 Story Task 的公开事件。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `task_uuid` | path | string(UUIDv7) | 是 | Story Task 公开 UUIDv7。 |
| `before` | query | string | 否 | 最多 32 个字符的非负十进制 sequence cursor，读取更早事件。 |
| `after` | query | string | 否 | 最多 32 个字符的非负十进制 sequence cursor，读取更新事件。 |
| `limit` | query | integer | 否 | 每页事件数，范围 1–100；默认 50。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | array<object> | 事件列表；无结果时为空数组。 |
| `data.items[].uuid` | string(UUIDv7) | 事件公开 UUIDv7。 |
| `data.items[].sequence` | integer | Task 内单调递增的事件序号。 |
| `data.items[].event_type` | string | 公开事件类型。 |
| `data.items[].created_at` | string(date-time) | 事件创建时间。 |
| `data.cursor_pagination.per_page` | integer | 本页请求的 `limit`。 |
| `data.cursor_pagination.next_cursor` | string，可省略 | 下一页 cursor。 |
| `data.cursor_pagination.prev_cursor` | string，可省略 | 上一页 cursor。 |
| `data.cursor_pagination.has_more` | boolean | 当前方向是否还有更多事件。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/tasks/01970000-0000-7000-8000-000000000008/events",
  "query": {"after": "42", "limit": 20},
  "response_filter": ".data | {items:{uuid,sequence,event_type,created_at},cursor_pagination:{per_page,next_cursor,prev_cursor,has_more}}"
}
```

### 接口约束

- `before` 与 `after` 不能同时提交；cursor 必须对应非负 sequence。
- 事件用于异步恢复与审计，业务事实仍应通过对应 REST 资源重读。

## `GET /api/v1/projects/{project_uuid}/production-tasks/{task_uuid}/events`

用 sequence cursor 读取一个 Production Task 的公开事件。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `task_uuid` | path | string(UUIDv7) | 是 | Production Task 公开 UUIDv7。 |
| `before` | query | string | 否 | 最多 32 个字符的非负十进制 sequence cursor，读取更早事件。 |
| `after` | query | string | 否 | 最多 32 个字符的非负十进制 sequence cursor，读取更新事件。 |
| `limit` | query | integer | 否 | 每页事件数，范围 1–100；默认 50。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | array<object> | 事件列表；无结果时为空数组。 |
| `data.items[].uuid` | string(UUIDv7) | 事件公开 UUIDv7。 |
| `data.items[].sequence` | integer | Task 内单调递增的事件序号。 |
| `data.items[].event_type` | string | 公开事件类型。 |
| `data.items[].created_at` | string(date-time) | 事件创建时间。 |
| `data.cursor_pagination.per_page` | integer | 本页请求的 `limit`。 |
| `data.cursor_pagination.next_cursor` | string，可省略 | 下一页 cursor。 |
| `data.cursor_pagination.prev_cursor` | string，可省略 | 上一页 cursor。 |
| `data.cursor_pagination.has_more` | boolean | 当前方向是否还有更多事件。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/production-tasks/01970000-0000-7000-8000-000000000009/events",
  "query": {"before": "100", "limit": 20},
  "response_filter": ".data | {items:{uuid,sequence,event_type,created_at},cursor_pagination:{per_page,next_cursor,prev_cursor,has_more}}"
}
```

### 接口约束

- `before` 与 `after` 不能同时提交；cursor 必须对应非负 sequence。
- 事件用于异步恢复与审计，业务事实仍应通过对应 REST 资源重读。

## `POST /api/v1/projects/{project_uuid}/tasks/{task_uuid}/cancellations`

请求取消一个 Story Task，并返回取消后的公开状态。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `task_uuid` | path | string(UUIDv7) | 是 | Story Task 公开 UUIDv7。 |
| `request_body` | body | 空 JSON 对象 | 是 | 必须为 `{}`，不接受额外字段。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Task 公开 UUIDv7。 |
| `data.kind` | string | Story 任务类型。 |
| `data.resource_uuid` | string(UUIDv7) | 目标资源公开 UUIDv7。 |
| `data.status` | string | 取消后的状态；已终态任务可能保持原状态。 |
| `data.error_code` | string，可省略 | 公开错误码。 |
| `data.error_message` | string，可省略 | 公开错误信息。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/tasks/01970000-0000-7000-8000-000000000008/cancellations",
  "request_body": {},
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- 取消是危险操作，执行前需要确认。
- 已提交不可变业务结果时取消不得回滚该结果；已终态任务按当前状态幂等返回。

## `POST /api/v1/projects/{project_uuid}/production-tasks/{task_uuid}/cancellations`

请求取消一个 Production Task，并返回取消后的公开状态。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `task_uuid` | path | string(UUIDv7) | 是 | Production Task 公开 UUIDv7。 |
| `request_body` | body | 空 JSON 对象 | 是 | 必须为 `{}`，不接受额外字段。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Task 公开 UUIDv7。 |
| `data.kind` | string | Production 任务类型。 |
| `data.resource_uuid` | string(UUIDv7) | 目标资源公开 UUIDv7。 |
| `data.status` | string | 取消后的状态；已终态任务可能保持原状态。 |
| `data.error_code` | string，可省略 | 公开错误码。 |
| `data.error_message` | string，可省略 | 公开错误信息。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/production-tasks/01970000-0000-7000-8000-000000000009/cancellations",
  "request_body": {},
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- 取消是危险操作，执行前需要确认。
- 已完成或已取消任务按当前状态幂等返回；取消不会删除已持久化的业务资源。

## `POST /api/v1/projects/{project_uuid}/tasks/{task_uuid}/retries`

显式重试一个可重试的 Story Task。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `task_uuid` | path | string(UUIDv7) | 是 | Story Task 公开 UUIDv7。 |
| `request_body` | body | 空 JSON 对象 | 是 | 必须为 `{}`，不接受额外字段。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 原 Task 公开 UUIDv7，不创建新 Task。 |
| `data.kind` | string | Story 任务类型。 |
| `data.resource_uuid` | string(UUIDv7) | 目标资源公开 UUIDv7。 |
| `data.status` | string | 接受重试后通常为 `queued`。 |
| `data.error_code` | string，可省略 | 重试排队后通常省略。 |
| `data.error_message` | string，可省略 | 重试排队后通常省略。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/tasks/01970000-0000-7000-8000-000000000008/retries",
  "request_body": {},
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- 只有声明为可重试且状态为 `failed`、`interrupted` 或 `cancelled` 的 Story Task 可重试。
- 异步任务重试复用原 Task UUID 与已固化输入，排队前必须结束上一轮执行。

## `POST /api/v1/projects/{project_uuid}/production-tasks/{task_uuid}/retries`

显式重试一个失败、中断或取消的 Production Task。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `task_uuid` | path | string(UUIDv7) | 是 | Production Task 公开 UUIDv7。 |
| `request_body` | body | 空 JSON 对象 | 是 | 必须为 `{}`，不接受额外字段。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 原 Task 公开 UUIDv7，不创建新 Task。 |
| `data.kind` | string | Production 任务类型。 |
| `data.resource_uuid` | string(UUIDv7) | 目标资源公开 UUIDv7。 |
| `data.status` | string | 接受重试后通常为 `queued`。 |
| `data.error_code` | string，可省略 | 重试排队后通常省略。 |
| `data.error_message` | string，可省略 | 重试排队后通常省略。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/production-tasks/01970000-0000-7000-8000-000000000009/retries",
  "request_body": {},
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- 只有 `failed`、`interrupted` 或 `cancelled` 的 Production Task 可重试。
- 异步任务重试复用原 Task UUID 与已固化输入；过期且不再保留输入的 Export Task 不能重试。
