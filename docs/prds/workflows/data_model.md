# 工作流 — 数据模型

## 实体关系

```text
projects ──< workflows ──< workflow_steps
                   │              └──> task / production task / resource UUID
                   └──< workflow_events
                   └──> chat_threads（可空）
                   └──< workflow_awaits >── chat_turns / chat_runs / agent_tool_executions
```

Workflow、Step 与 Event 的内部关联使用 bigint `id`；外部 API 和实时 payload 只使用 UUIDv7。

## 表：workflows

- `uuid` — TEXT NOT NULL UNIQUE，公开 Workflow UUIDv7
- `project_id` / `thread_id` — 项目内部关联和可选 Chat Thread
- `kind` — `yolo_project_initialization`、`comic_section_image_generation`、`comic_storyboard_generation`、`story_chapter_generation` 或 `story_chapter_batch_plan`
- `title` / `status` — 展示标题和 `queued|running|completed|failed|cancelled|interrupted`
- `input_version` / `input_snapshot` / `idempotency_key` — 冻结输入和项目内幂等身份
- `provider_uuid` / `model` / `model_source` — 冻结模型选择
- `current_step_key`、错误、取消和生命周期时间 — 执行投影

`(project_id, kind, idempotency_key)` 唯一。

`thread_id` 的含义由调用上下文决定：公开 UI 与对话式 bootstrap YOLO 关联独立 `workflow` Thread；普通 Chat Tool 生成关联当前 `conversation` Thread；内部 Workflow Step 投影可以为空。bootstrap YOLO 不创建 `workflow_awaits`，其原 conversation Turn 在拿到 Workflow 引用后完成。

## 表：workflow_steps 与 workflow_events

- `workflow_steps` 以 `step_key` 和 `position` 在同一 Workflow 中唯一，保存状态、任务 UUID、资源 UUID、输入输出 JSON 和错误。
- `workflow_events` 以 `(workflow_id, sequence)` 唯一追加事件，可选关联一个 Step。

## 表：workflow_awaits

Chat Tool 异步调用的持久恢复边界。

- `uuid` — TEXT NOT NULL UNIQUE，UUIDv7 格式的持久身份；不对外暴露 bigint ID
- `workflow_id` / `chat_thread_id` / `chat_turn_id` / `chat_run_id` / `tool_execution_id` — bigint 内部外键，固定父 Run 与独占 Tool Execution
- `status` — `waiting|ready|resuming|resumed|cancelled`
- `river_job_id` — 可空内部 River 关联，不进入 REST 或 WebSocket
- `ready_at` / `resumed_at` / `cancelled_at` / 生命周期时间 — 恢复投影

`workflow_id` 与 `tool_execution_id` 分别唯一，防止一个 Workflow 或 Tool Execution 重复创建等待关系。外部 Workflow DTO 只投影 `presentation_mode`、`origin_turn_uuid`、`origin_run_uuid`、`origin_tool_call_uuid`、`origin_item_uuid` 和 `await_status` 等公开 UUIDv7 字段。

## 数据生命周期

1. 创建 Workflow 时写入冻结快照及初始 Steps。
2. Worker 按 position 推进 Step，并持久化状态、输出和 Event。
3. Chat Tool Workflow 创建时在同一业务事务写入 Workflow、Step、await 与任务；父 Run 保持可恢复等待且不占 worker。
4. Workflow 终态事务把有效 await 标为 `ready`、父 Turn/Run 重新排为 `queued`，并唯一插入 `JobChatResume`；恢复先幂等写入结构化 Tool Result，再把 await 标为 `resumed`。
5. 父 Run 取消会把 await 标为 `cancelled` 并请求取消其独占 Workflow；单独取消 Workflow 则以 cancelled Tool Result 唤醒仍有效的父 Run。失败、中断和取消都保留可诊断快照。
