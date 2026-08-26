# Task API

使用 `request_api` 调用，将占位符替换为公开 UUIDv7。Story 任务使用 `/tasks`；Premise、Comic 与导出等生产任务使用 `/production-tasks`。不要定时 HTTP 轮询；只在工作流需要时读取，应用状态同步由 WebSocket 变更提示后 REST 重读完成。

常用 Task 过滤器：`.data | {uuid,kind,resource_uuid,status,error_code,error_message}`。

## 读取任务

- `GET /api/v1/projects/{project_uuid}/tasks/{task_uuid}`
- `GET /api/v1/projects/{project_uuid}/production-tasks/{task_uuid}`

## 列表

- `GET /api/v1/projects/{project_uuid}/tasks`
- `GET /api/v1/projects/{project_uuid}/production-tasks`

可选 `query`：`status`、`limit`（1–100）。`status` 为 `queued`、`running`、`waiting_for_input`、`completed`、`failed`、`cancelled` 或 `interrupted`。使用 `.data.items[] | {uuid,kind,resource_uuid,status,error_code,error_message}`。

## 事件

- `GET /api/v1/projects/{project_uuid}/tasks/{task_uuid}/events`
- `GET /api/v1/projects/{project_uuid}/production-tasks/{task_uuid}/events`

可选 `query`：`before`、`after`、`limit`（1–100）；`before` 与 `after` 不能同时传。使用 `.data | {items,cursor_pagination}`；事件中的必要字段为 `uuid`、`sequence`、`event_type`、`created_at`。

## 取消与重试

- `POST /api/v1/projects/{project_uuid}/tasks/{task_uuid}/cancellations`
- `POST /api/v1/projects/{project_uuid}/production-tasks/{task_uuid}/cancellations`
  - 取消前必须用 `request_user_input` 获得确认。
- `POST /api/v1/projects/{project_uuid}/tasks/{task_uuid}/retries`
- `POST /api/v1/projects/{project_uuid}/production-tasks/{task_uuid}/retries`

以上四个接口的 `request_body` 都必须传空对象：`{}`；返回 Task，使用常用 Task 过滤器。
