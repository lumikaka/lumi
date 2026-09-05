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
                 ├──< workflow_awaits >── workflows
                 └──< agent_context_summaries

project_creation_bootstraps ──> chat_threads / chat_turns

chat_context_references ──> files / premise_assets / comic_sections
                        ├──> files（冻结 image_file_id）
                        └── chapter（仅 resource_uuid + snapshot_json）
```

所有内部关联使用 bigint `id`；Thread、Turn、Run、Item、Event、Follow-up、用户输入和 File 引用对外只使用 UUIDv7。

## 表：chat_threads

- `uuid` — TEXT NOT NULL UNIQUE，公开 Thread UUIDv7
- `project_id` — INTEGER NOT NULL FK → `projects.id`
- `thread_type` — `conversation|workflow`；普通对话使用聚合状态，独立 Workflow Thread 镜像其 Workflow 终态
- `title` / `status` — 展示标题和 `idle|busy|waiting_for_input|completed|failed|cancelled|interrupted` 状态
- `provider_uuid` / `model` / `model_source` — 创建时冻结的模型选择
- `next_turn_sequence` / `next_item_sequence` / `next_event_sequence` — Thread 内递增序列
- `archived_at` / `created_at` / `updated_at` — 生命周期时间

## 关联表

- `chat_turns` — 排队输入、来源、状态、取消和执行时间；每个 Thread 内 queue sequence 有序。等待异步 Workflow 时底层保持 `in_progress`，REST 投影为 `waiting_for_workflow`。
- `chat_runs` — Turn 的一次 Agent 执行，保存冻结模型、上下文大小、模型请求数、主动执行时长、累计 token、无进展状态、预算收尾状态和终态；同一 Turn 在等待用户或 Workflow 后恢复同一 Run。旧 `step_count/max_steps` 列仅为迁移兼容保留，不再参与运行时限制；Workflow 等待语义由唯一 `workflow_awaits` 关系补充。
- `chat_items` — 用户、Assistant、工具、错误或输入请求等可读序列项。
- `chat_events` — Thread 内 append-only 诊断事件。
- `chat_follow_ups` — 可排序的待发送追问及其 promoted Turn。
- `chat_user_input_requests` — Tool call 触发的可恢复用户输入。`schema_version` 为 `codex_questions_v1|legacy_choice_v1`；`request_json` 是请求唯一事实源，`response_json` 保存已校验回答。v4 请求包含 1–3 个带逻辑 `id` 的互斥单选问题和服务端生成的选项 UUIDv7；legacy 投影完整保留旧单选、多选与 Other。
- `agent_tool_executions` — 工具意图、受控参数、幂等键与结果状态。
- `agent_context_summaries` — 到指定 Item sequence 为止的持久上下文摘要。
- `workflow_awaits` — 将一个异步 Workflow 唯一绑定到当前 Thread、Turn、Run 和 Tool Execution，保存 `waiting|ready|resuming|resumed|cancelled` 与内部 Resume Job 关联。

## 表：chat_context_references

用户输入、排队追问或服务端确认成功的图片写回 Tool Result 所拥有的统一冻结 Reference。用户和模型都不能直接为 Tool Result 构造 Reference。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅供内部主键和 JOIN
- `chat_item_id` / `follow_up_id` — 可空 INTEGER FK，必须且只能设置一个 owner；owner 删除时级联
- `position` — INTEGER NOT NULL；用户输入/追问 owner 内为 `1–16` 并保持输入顺序，图片写回 Tool Result 固定为 `1`
- `resource_type` — TEXT NOT NULL，`file|premise_asset|chapter|comic_section`
- `resource_uuid` — TEXT NOT NULL，冻结的公开资源 UUIDv7
- `snapshot_json` — TEXT NOT NULL，不超过 8 KiB 的合法紧凑 JSON；包含 `truncated_fields`
- `file_id` / `premise_asset_id` / `comic_section_id` — 与对应类型匹配的可空内部 FK；Chapter Reference 不保存内部 FK，仅以 `resource_uuid` 和冻结快照表达
- `image_file_id` — 可空 INTEGER FK → `files.id`，Reference 接受时冻结的图片 File；删除受限
- `created_at` — DATETIME NOT NULL

`comic_section` 类型的 `snapshot_json` 冻结 `chapter_uuid`、`section_no`、`page_role`、`body_page_no`、标题、描述、revision、`current_storyboard_uuid` 与可用的 `current_image_file_uuid`。`section_no` 是含封面和封底的绝对装订顺序；`body_page_no` 只在 `page_role=body` 时从 1 起计算，特殊页为 0。

**索引：**

- `(chat_item_id, position)` / `(follow_up_id, position)` — owner 内唯一顺序
- `(chat_item_id, resource_type, resource_uuid)` / `(follow_up_id, resource_type, resource_uuid)` — owner 内资源去重
- 四个目标 FK 字段（含 `image_file_id`）均有反向查询索引，用于生命周期检查和 GC

## 表：project_creation_bootstraps

首页创建 Session 与普通 Chat 首轮之间的恰好一次关联；该表归项目创建 Feature 所有，但其内部外键落在 Chat 聚合内。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅供内部主键和 JOIN
- `uuid` — TEXT NOT NULL UNIQUE，UUIDv7
- `project_id` — INTEGER NOT NULL FK → `projects.id`
- `creation_session_uuid` — TEXT NOT NULL UNIQUE，跨应用库的公开相关性 UUID，不伪造外键
- `thread_id` / `turn_id` — INTEGER NOT NULL UNIQUE FK → `chat_threads.id` / `chat_turns.id`
- `created_at` — DATETIME NOT NULL

## 数据生命周期

1. 普通首次发送创建通用 Thread 和首个 Turn；首页项目创建则在同一项目库事务额外写入唯一 bootstrap、首个 User Item、Run 和队列 Job。bootstrap 的 `turn_id` 仅授权该 Turn 在定稿后幂等启动 existing YOLO，后续 Turn 不继承。Reference 在用户输入落库前校验并冻结到 User Item；成功用 `image_gen` File 创建或更新 Premise Asset 时，更新后快照与对应 Tool Result 在同一 Chat 事务写入。
2. 执行创建 Run、Item、工具和用户输入记录，事件只追加；当前 Turn 的 Steering 可追加自己的 Reference。
3. Follow-up 保存自己的 Reference，可原子替换、提升为 Turn、立即引导或逻辑删除。
4. 目标资源删除后保留 `resource_uuid` 与快照；冻结图片继续保护对应 File/Object，直到 Reference owner 删除。
5. Chat Tool 创建异步 Generation 时不创建新 Thread；父 Run 释放 worker，Workflow 终态以结构化 Tool Result 恢复同一 Run。父 Run 取消会取消其独占 await 和 Workflow，已取消父 Run 不会被晚到终态重新唤醒。
6. Thread 状态由集中函数重算：用户输入或决策优先为 `waiting_for_input`；存在 queued、in-progress、Workflow-waiting Turn 或活动 Workflow 时为 `busy`；普通对话无活动项为 `idle`；仅独立 `workflow` Thread 镜像 Workflow 终态。
7. 客户端以 REST 重读列表、items 和 events；实时消息只触发目标 Thread 查询失效。
8. v4 回答必须覆盖请求中每个 question id，每题恰好使用一个所属选项 UUID 或非空 Other；写入回答、同 Tool call 的 Codex 形状 Tool Result、Run/Turn 排队和唯一 Resume Job 在同一事务完成。
9. 项目仍为 `draft` 时，Agent 每次工具执行都重读 `setup_status`；Project Setup 定稿后，同一 Run 的后续工具即可按 `ready` 能力继续。
