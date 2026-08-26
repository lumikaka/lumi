# 对话线程 — 数据模型

## 实体关系

```text
projects ──< chat_threads
                 ├──< chat_turns ──< chat_runs ──< chat_items
                 │                                ├──< chat_context_references
                 │                                ├──< agent_tool_executions
                 │                                └──< chat_user_input_requests
                 ├──< chat_events
                 ├──< chat_follow_ups ──< chat_context_references
                 └──< agent_context_summaries

chat_context_references ──> files / premise_assets / comic_sections
                        └──> files（冻结 image_file_id）
```

所有内部关联使用 bigint `id`；Thread、Turn、Run、Item、Event、Follow-up、用户输入和 File 引用对外只使用 UUIDv7。

## 表：chat_threads

- `uuid` — TEXT NOT NULL UNIQUE，公开 Thread UUIDv7
- `project_id` — INTEGER NOT NULL FK → `projects.id`
- `title` / `status` — 展示标题和 `idle|busy|waiting_for_input|completed|failed|cancelled|interrupted` 状态
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

## 表：chat_context_references

用户输入或排队追问的统一冻结 Reference。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅供内部主键和 JOIN
- `chat_item_id` / `follow_up_id` — 可空 INTEGER FK，必须且只能设置一个 owner；owner 删除时级联
- `position` — INTEGER NOT NULL，owner 内 `1–16`，保持用户输入顺序
- `resource_type` — TEXT NOT NULL，`file|premise_asset|comic_section`
- `resource_uuid` — TEXT NOT NULL，冻结的公开资源 UUIDv7
- `snapshot_json` — TEXT NOT NULL，不超过 8 KiB 的合法紧凑 JSON；包含 `truncated_fields`
- `file_id` / `premise_asset_id` / `comic_section_id` — 与类型匹配的可空内部 FK；目标永久删除后可置空
- `image_file_id` — 可空 INTEGER FK → `files.id`，Reference 接受时冻结的图片 File；删除受限
- `created_at` — DATETIME NOT NULL

**索引：**

- `(chat_item_id, position)` / `(follow_up_id, position)` — owner 内唯一顺序
- `(chat_item_id, resource_type, resource_uuid)` / `(follow_up_id, resource_type, resource_uuid)` — owner 内资源去重
- 四个目标 FK 字段均有反向查询索引，用于生命周期检查和 GC

## 数据生命周期

1. 首次发送时创建通用 Thread 和首个 Turn；Reference 在用户输入落库前校验并冻结到 User Item。
2. 执行创建 Run、Item、工具和用户输入记录，事件只追加；当前 Turn 的 Steering 可追加自己的 Reference。
3. Follow-up 保存自己的 Reference，可原子替换、提升为 Turn、立即引导或逻辑删除。
4. 目标资源删除后保留 `resource_uuid` 与快照；冻结图片继续保护对应 File/Object，直到 Reference owner 删除。
5. 客户端以 REST 重读列表、items 和 events；实时消息只触发目标 Thread 查询失效。
