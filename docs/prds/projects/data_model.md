# 项目 — 数据模型

## 实体关系

```text
应用库
recent_projects ──(uuid / root_path)──> 本机发现索引
      └──< project_creation_sessions

项目库
projects ──< actors
   ├──0..1 project_picture_book_profiles
   ├──1 project_setup_drafts（仅对话式草稿项目）
   ├──0..1 project_creation_bootstraps ──> chat_threads / chat_turns
   ├──< project_story_profiles
   └──< project_prompt_versions
```

应用库与项目库是两个 SQLite 范围。所有项目库关联使用内部 `INTEGER PRIMARY KEY AUTOINCREMENT` 的 64-bit `id`；API、URL、前端与 WebSocket 只使用 UUIDv7 `uuid`。

## 表：recent_projects

应用库的最近项目发现索引，不保存项目业务数据。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅内部使用
- `uuid` — TEXT NOT NULL UNIQUE，项目公开 UUIDv7
- `name` / `root_path` — 最后已知展示名与本机目录；路径可能失效，不能代替项目身份
- `last_opened_at` / `updated_at` — 最近打开和索引更新时间

## 表：projects 与 actors

项目库只存在一个 `projects` 记录；创建项目时同时创建默认 `local_user` actor。

- `projects.uuid` — TEXT NOT NULL UNIQUE，项目公开 UUIDv7
- `name` / `description` / `generation_language` — 项目资料与生成语言
- `setup_status` — `draft|ready`；旧项目与手动创建项目默认 `ready`
- `revision` — 非负乐观锁版本
- `actors.uuid` — TEXT NOT NULL UNIQUE，公开操作者 UUIDv7
- `actors.project_id` — INTEGER FK → `projects.id`

## 表：project_picture_book_profiles

每个项目最多一行的正式绘本规格。`draft` 项目为零行，`ready` 项目必须恰有一行；正式记录由触发器禁止更新和删除。

- `project_id` — INTEGER NOT NULL UNIQUE FK → `projects.id`
- `page_width` / `page_height` / `page_count` — 页面尺寸和页数约束
- `revision` — 非负乐观锁版本

## 表：project_creation_sessions

应用库中的可恢复首页创建 Saga，不把跨库步骤伪装成一个 SQLite 事务。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅供内部主键和恢复排序
- `uuid` — TEXT NOT NULL UNIQUE，公开创建会话 UUIDv7
- `idempotency_key` — TEXT NOT NULL UNIQUE，客户端稳定重试键
- `input_text` — TEXT NOT NULL，逐字保存的首页原始输入，1–256 KiB
- `status` — `pending|creating_project|creating_conversation|active|failed|cancelled`
- `planned_project_uuid` / `planned_root_path` — 首次规划后冻结的项目 UUIDv7 与安全占位目录
- `recent_project_id` — INTEGER FK → `recent_projects.id`，项目创建完成检查点
- `thread_uuid` / `turn_uuid` — 对话 bootstrap 完成后的公开 UUIDv7
- `error_code` / `error_message` / `attempt_count` — 可展示失败与恢复计数
- `completed_at` / `failed_at` / 生命周期时间 — Saga 状态时间

**索引：**

- `(status, updated_at, id)` — 启动恢复顺序

**主要相关 Feature：**

- [`对话式项目创建与设置定稿`](features/对话式项目创建与设置定稿.md)

## 表：project_setup_drafts

项目库中的初始化候选事实源；每个对话式草稿项目唯一一行。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅供内部主键和 JOIN
- `uuid` — TEXT NOT NULL UNIQUE，公开 Setup UUIDv7
- `project_id` — INTEGER NOT NULL UNIQUE FK → `projects.id`
- `status` — `draft|pending_confirmation|finalized|failed`
- `revision` — 从 1 开始的乐观锁版本
- `original_input` — TEXT NOT NULL，逐字保存的首页原始需求
- `project_name` / `generation_language` / `overall_style` — 可空候选资料
- `format` / `aspect_ratio_mode` / `aspect_width` / `aspect_height` — 可空候选绘本形式与规范比例
- `large_image_minimal_text` / `interaction_mode` / `comic_layout` — 由绘本形式约束的可空专属候选
- `field_sources_json` / `missing_fields_json` — 合法 JSON object/array，记录 `system_default|agent_proposed|user_confirmed` 和缺失项
- `error_code` / `error_message` / `finalized_revision` — 失败和幂等定稿信息
- `finalized_at` / `failed_at` / 生命周期时间 — 候选生命周期

**索引：**

- `(project_id, status)` — 按项目读取候选状态

**主要相关 Feature：**

- [`对话式项目创建与设置定稿`](features/对话式项目创建与设置定稿.md)

## 表：project_creation_bootstraps

项目库中的创建会话与普通 Chat 首轮之间的恰好一次边界。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅供内部主键和 JOIN
- `uuid` — TEXT NOT NULL UNIQUE，UUIDv7
- `project_id` — INTEGER NOT NULL FK → `projects.id`
- `creation_session_uuid` — TEXT NOT NULL UNIQUE，关联应用库公开创建会话，不跨库保存内部 ID
- `thread_id` / `turn_id` — INTEGER NOT NULL UNIQUE FK → `chat_threads.id` / `chat_turns.id`
- `created_at` — DATETIME NOT NULL

`turn_id` 仅标记首页创建的第一个 Turn。运行时据此恢复进程内 bootstrap 权限边界，`creation_session_uuid` 同时派生受控 YOLO 的服务端幂等键；后续 Turn 不沿用该标记。

**主要相关 Feature：**

- [`对话式项目创建与设置定稿`](features/对话式项目创建与设置定稿.md)

## 表：project_story_profiles

项目级 Story 总纲的追加式版本历史；每个项目恰有一个 current 记录。

- `uuid` — TEXT NOT NULL UNIQUE，公开 UUIDv7
- `project_id` / `actor_id` — INTEGER FK → `projects.id` / `actors.id`
- `version_no` / `revision` — 项目内递增版本与修订号
- `is_current` — INTEGER，项目内仅一条 current 记录
- `story_md` / `content_hash` — 结构化总纲事实与内容 hash
- `source_type` — `project_created`、`manual_edit` 或 `external_import`
- `projection_state` — `pending`、`synced` 或 `conflict`
- `exported_revision` / `exported_hash` / `observed_file_hash` — `STORY.md` 投影状态

## 表：project_prompt_versions

项目覆盖内置 Prompt 的追加式历史。

- `uuid` — TEXT NOT NULL UNIQUE，公开 UUIDv7
- `project_id` / `actor_id` / `restored_from_version_id` — 内部项目、操作者和恢复来源关联
- `prompt_group` / `prompt_key` — Catalog 定位键
- `version_no` / `prompt` / `prompt_hash` — 项目内版本与内容快照
- `source_type` — `manual_edit`、`version_restore`、`project_language_changed` 或 `default_restore`

`(project_id, prompt_group, prompt_key, version_no)` 唯一，更新触发器禁止修改历史版本。

## 数据生命周期

1. 手动创建项目直接写入 `ready` 项目和唯一正式绘本规格；迁移前旧项目通过默认值保持 `ready`。
2. 对话式创建先持久化应用库 Session，再按检查点建立 `draft` 项目、Setup Draft 和普通 Chat bootstrap；启动 reconciliation 可从失败或中间态幂等续跑。
3. 用户明确选择“定稿并启动 YOLO”后，在项目库单事务写入正式绘本规格、项目资料、默认画风和 finalized Setup，再切换 `setup_status=ready`；同 revision 重放幂等成功。
4. 同一 bootstrap Turn 使用 `creation_session_uuid` 幂等创建一个 existing YOLO Workflow 后立即完成；其他生产写入继续失败关闭，后续 Turn 恢复普通 ready 能力。
5. 项目总纲与 Prompt 覆盖仅在 `ready` 后创建；总纲和 Prompt 通过版本历史保留可恢复来源。
6. 打开项目时验证 UUID、加锁、迁移、执行受控 reconciliation 并启动项目 Runtime；关闭只影响目标项目。
