# 工作流 — 数据模型

## 实体关系

```text
projects ──< workflows ──< workflow_steps
                   │              └──> task / production task / resource UUID
                   └──< workflow_events
                   └──> chat_threads（可空）
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

## 表：workflow_steps 与 workflow_events

- `workflow_steps` 以 `step_key` 和 `position` 在同一 Workflow 中唯一，保存状态、任务 UUID、资源 UUID、输入输出 JSON 和错误。
- `workflow_events` 以 `(workflow_id, sequence)` 唯一追加事件，可选关联一个 Step。

## 数据生命周期

1. 创建 Workflow 时写入冻结快照及初始 Steps。
2. Worker 按 position 推进 Step，并持久化状态、输出和 Event。
3. 取消、失败或中断保持可诊断快照；重试使用相同冻结输入创建或恢复步骤。
