# AI 运行时 — 数据模型

## 实体关系

```text
projects ──1 project_model_settings
   │
   ├──< task_runs ──< task_events
   ├──< production_task_runs ──< production_task_events
   ├──< agent_threads ──< agent_runs ──< agent_events
   ├──< chat_threads ──< chat_runs
   ├──< workflows
   └──< llm_logs ──> task / production task / chat run / workflow step

Provider UUID ──(跨数据库公开标识)──> 配置和每个冻结执行资源
```

项目数据库关联仍使用内部 64-bit `id`；Provider 位于应用级存储，因此项目表保存公开 UUIDv7，而不建立跨数据库内部外键。

## 表：project_model_settings

每个项目最多一行；不存在行等同 revision 0、全部继承。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅内部使用
- `project_id` — INTEGER NOT NULL UNIQUE FK → `projects.id`，项目删除时级联
- `project_text_provider_uuid` / `project_text_model` — 项目文本覆盖，两个字段必须同时为空或同时有效
- `project_image_provider_uuid` / `project_image_model` — 项目图片覆盖
- `chat_area_provider_uuid` / `chat_area_model` — Chat 场景覆盖
- `story_text_provider_uuid` / `story_text_model` — Story 场景覆盖
- `section_premise_selection_provider_uuid` / `section_premise_selection_model` — Section Premise 选择场景覆盖
- `revision` — INTEGER NOT NULL DEFAULT 0，非负乐观锁
- `created_at` / `updated_at` — DATETIME NOT NULL

Provider UUID 字段非空时长度必须为 36；模型名 trim 后长度为 1–512。应用层另外验证 UUIDv7、Provider ready 状态和 text/image 能力类型。

## 表：可恢复执行记录

以下执行根表保存冻结输入、状态、尝试、取消请求和公开 UUIDv7；相关事件表只追加 sequence：

- `task_runs` / `task_events` — Story 和 Chapter 任务及其事件
- `production_task_runs` / `production_task_events` — Premise、漫画图片和导出任务及其事件
- `agent_threads` / `agent_runs` / `agent_events` — 早期 Story Agent 执行链，仍用于已有任务恢复
- `chat_threads` — 会话级冻结选择
- `chat_runs` — 每次会话执行的冻结选择
- `workflows` — YOLO 与漫画图片多步 Workflow

`task_runs`、`production_task_runs`、`chat_threads`、`chat_runs` 与 `workflows` 的 `provider_uuid`、`model` 保存实际执行选择，`model_source` 保存解析来源。Chat Thread 不保存业务 Scene 或 subject；Chat Run 的首个 User Item metadata 冻结 Prompt 内容、`tool_mode` 与活动 `project_api_v4` 协议，Reference 另存于对应 User Item。冻结的 `project_api_v3`、`project_api_v2` 和 `legacy_typed_tools` 只用于恢复，不改写 schema；迁移前记录使用 `legacy_frozen`，不推断历史来源。任务业务结果分别由章节、项目、漫画或导出 domain 解释。

## 表：llm_logs

统一保存 Story、Chat、Production 与 Workflow 的文本/图片调用。每行通过一个内部 bigint 外键关联对应执行上下文，对外只投影关联资源的 UUIDv7。

- `uuid` — TEXT NOT NULL UNIQUE，公开 UUIDv7
- `project_id` — INTEGER NOT NULL FK → `projects.id`
- `task_run_id` / `production_task_run_id` / `chat_thread_id` / `chat_run_id` / `workflow_id` / `workflow_step_id` — INTEGER FK，仅一个调用上下文组合有效
- `source_type` / `scenario` / `request_type` / `attempt` — 调用来源、业务场景、`text|image` 与尝试次数
- `provider_uuid` / `provider_type` / `model` — 调用时冻结的公开 Provider 标识与模型
- `status` — `pending`、`completed`、`failed` 或 `cancelled`
- `input_summary` / `output_summary` — 有界安全摘要；用于关键词筛选
- `input_tokens` / `output_tokens` / `duration_ms` — 既有非负 usage 与耗时字段
- `cached_input_tokens` / `input_characters` / `output_characters` — INTEGER，可空且非负；旧日志或 Provider 不支持时为空
- `request_payload` / `response` — 可空合法 JSON，只在详情读取；列表关键词不扫描这两个字段
- `finish_reason` / `error_code` / `error_message` / Provider 诊断字段 — 结束状态与安全诊断
- `created_at` / `completed_at` — 时间字段

`(project_id, created_at DESC, id DESC)` 支持稳定历史读取；Provider 与 scenario 联合索引用于常用筛选。输出 token/s 与字符/s 由非空 usage 和正 `duration_ms` 读取时推导，不持久化。

## 枚举：model_source

- `explicit_task` — 请求明确指定 Provider 或模型
- `scenario_override` — 命中 `chat_area`、`story_text` 或 `section_premise_selection`
- `project_text_override` — 命中项目文本覆盖
- `project_image_override` — 命中项目图片覆盖
- `global_provider_default` — 使用 active 且 ready Provider 的对应默认模型
- `legacy_frozen` — 迁移前已冻结记录

## 数据生命周期

1. 第一次保存任意覆盖时创建 `project_model_settings`，revision 从 0 变为 1；之后按 `expected_revision` 原子递增。
2. 清除覆盖把该 Provider/model 对恢复为空，不删除整行。
3. 创建 Task、Production task、Chat thread/run 或 Workflow 时解析一次并写入三元组与输入快照。
4. 运行转换状态时追加事件；取消和重试只作用于冻结记录，重试不重新解析当前模型或 Prompt。
5. Workflow 后续 Chat/步骤传播相同选择，不重新读取项目设置。
6. 新调用开始时插入 pending `llm_logs`；结束时原子写入状态、usage、Unicode 字符数和诊断。旧日志的新指标保持 `NULL`，不猜测回填。
