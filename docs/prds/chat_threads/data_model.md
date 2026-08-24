# 对话线程 — 数据模型

## 实体关系

```text
projects ──< chat_threads
                 ├──< chat_turns ──< chat_runs ──< chat_items
                 │                                ├──< agent_tool_executions
                 │                                └──< chat_user_input_requests
                 ├──< chat_events
                 ├──< chat_follow_ups
                 └──< agent_context_summaries

chat_items / chat_follow_ups ──< *_file_references ──> files
```

所有内部关联使用 bigint `id`；Thread、Turn、Run、Item、Event、Follow-up、用户输入和 File 引用对外只使用 UUIDv7。

## 表：chat_threads

- `uuid` — TEXT NOT NULL UNIQUE，公开 Thread UUIDv7
- `project_id` — INTEGER NOT NULL FK → `projects.id`
- `title` / `status` — 展示标题和 `idle|busy|waiting_for_input|completed|failed|cancelled|interrupted` 状态
- `scope` / `scene` / `subject_uuid` — 对话业务上下文和公开绑定资源
- `provider_uuid` / `model` / `model_source` — 创建时冻结的模型选择
- `next_turn_sequence` / `next_item_sequence` / `next_event_sequence` — Thread 内递增序列
- `archived_at` / `created_at` / `updated_at` — 生命周期时间

## 关联表

- `chat_turns` — 排队输入、来源、状态、取消和执行时间；每个 Thread 内 queue sequence 有序。
- `chat_runs` — Turn 的一次 Agent 执行，保存冻结模型、上下文大小、步骤数和终态。
- `chat_items` — 用户、Assistant、工具、错误或输入请求等可读序列项。
- `chat_events` — Thread 内 append-only 诊断事件。
- `chat_follow_ups` — 可排序的待发送追问及其 promoted Turn。
- `chat_user_input_requests` — Tool call 触发的单选或多选请求及回答状态。
- `agent_tool_executions` — 工具意图、受控参数、幂等键与结果状态。
- `agent_context_summaries` — 到指定 Item sequence 为止的持久上下文摘要。
- `chat_item_file_references` / `chat_follow_up_file_references` — Item 或排队追问与 File 的内部关联。

## 数据生命周期

1. 首次发送时创建 Thread 和首个 Turn；后续输入按 Thread sequence 排队。
2. 执行创建 Run、Item、工具和用户输入记录，事件只追加。
3. Follow-up 可等待、提升为 Turn、立即引导或逻辑删除；运行结束后恢复下一条排队项。
4. 客户端以 REST 重读列表、items 和 events；实时消息只触发目标 Thread 查询失效。
