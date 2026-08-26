# AI 运行时 — 可恢复AI任务执行

## overview

该 Feature 为 Story、Premise、漫画和导出提供共享的可恢复执行契约。任务在创建时冻结业务输入、Prompt、Provider、模型与参数；worker 不读取后来变化的业务资源作为替代输入。状态变化写入 SQLite，事件只追加，WebSocket 仅通知客户端重新读取任务事实状态。

业务结果归各业务 domain：例如 Chapter 生成追加正文版本，Premise/漫画生成写入其 variant，导出发布短期产物。运行时只负责调度状态、取消、重试、幂等与恢复边界。

## data_model

`task_runs` 和 `production_task_runs` 均保存公开 `uuid`、项目内部关联、kind、resource UUID、输入快照、冻结模型、状态、进度、尝试次数、取消时间、错误和生命周期时间。`task_events` 与 `production_task_events` 以任务内递增 sequence 保存 append-only 事件。旧 Story Agent 链使用 `agent_threads`、`agent_runs` 和 `agent_events` 保留相同恢复原则。

## api

| 接口 | 方法 | 说明 |
|---|---|---|
| `/api/v1/projects/:project_uuid/tasks` | GET | 分页读取 Story/Chapter 任务。 |
| `/api/v1/projects/:project_uuid/tasks/:task_uuid` | GET | 读取单个任务。 |
| `/api/v1/projects/:project_uuid/tasks/:task_uuid/events` | GET | cursor 读取任务事件。 |
| `/api/v1/projects/:project_uuid/tasks/:task_uuid/cancellations` | POST | 请求取消任务。 |
| `/api/v1/projects/:project_uuid/tasks/:task_uuid/retries` | POST | 重试冻结任务。 |
| `/api/v1/projects/:project_uuid/production-tasks` | GET | 分页读取生产任务。 |
| `/api/v1/projects/:project_uuid/production-tasks/:task_uuid` | GET | 读取单个生产任务。 |
| `/api/v1/projects/:project_uuid/production-tasks/:task_uuid/events` | GET | cursor 读取生产任务事件。 |
| `/api/v1/projects/:project_uuid/production-tasks/:task_uuid/cancellations` | POST | 请求取消生产任务。 |
| `/api/v1/projects/:project_uuid/production-tasks/:task_uuid/retries` | POST | 重试冻结生产任务。 |

## others

状态以 REST/SQLite 为事实源，不使用定时 HTTP 轮询。首次 join、重新 join 和窗口重新聚焦必须重读目标任务；实时 payload 只可携带公开 UUIDv7、状态和刷新定位信息。Chat Retry/Resume 复用原 User Item 的 Prompt snapshot 与 Reference snapshot；只有新 Run 使用 `project_api_v4`，已有 v3、v2 或 legacy typed Run 不升级协议，按各自冻结的 Tool schema 和用户输入回答语义继续执行。
