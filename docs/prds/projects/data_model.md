# 项目 / 绘本项目 — 数据模型

## 实体关系

```text
应用库
recent_projects ──(uuid / root_path)──> 本机发现索引
      └──< project_creation_sessions
                    └──< project_creation_session_references

项目库
projects ──< actors
   ├──0..1 project_picture_book_profiles
   ├──1 project_setup_drafts（仅对话式草稿项目）
   ├──0..1 project_creation_bootstraps ──> chat_threads / chat_turns
   ├──< project_creation_reference_files ──> files
   ├──< project_story_profiles
   └──< project_prompt_versions
```

应用库与项目库是两个 SQLite 范围。所有项目库关联使用内部 `INTEGER PRIMARY KEY AUTOINCREMENT` 的 64-bit `id`；API、URL、前端与 WebSocket 只使用 UUIDv7 `uuid`。

`projects` 的产品称呼为“项目”或“绘本项目”。`project_picture_book_profiles.format` 同时决定下级资源的用户称呼：普通绘本形式使用“绘本 / 页面 / 页面脚本”，`vertical_strip` 使用“章节 / 画面段落 / 分镜脚本”。

## 表：recent_projects

应用库的最近项目发现索引，不保存项目业务数据。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅内部使用
- `uuid` — TEXT NOT NULL UNIQUE，项目公开 UUIDv7
- `name` / `root_path` — 最后已知展示名与本机目录；路径可能失效，不能代替项目身份
- `last_opened_at` / `updated_at` — 最近打开和索引更新时间

最近项目 API 的 `cover_image_url` 不对应 `recent_projects` 持久字段。服务端只读项目库，在第一本存在 ready `front_cover|body` 候选的 Chapter 内优先 `front_cover`、否则取首个 `body`；`back_cover` 不参与候选。URL 携带由所选图片内容 SHA-256 派生的 `v` 查询参数，current 图片变化时浏览器地址随之变化，避免沿用旧缩略图缓存。

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
- `status` — `pending|creating_project|awaiting_references|creating_conversation|active|failed|cancelled`
- `planned_project_uuid` / `planned_root_path` — 首次规划后冻结的项目 UUIDv7 与安全占位目录
- `recent_project_id` — INTEGER FK → `recent_projects.id`，项目创建完成检查点
- `thread_uuid` / `turn_uuid` — 对话 bootstrap 完成后的公开 UUIDv7
- `error_code` / `error_message` / `attempt_count` — 可展示失败与恢复计数
- `completed_at` / `failed_at` / 生命周期时间 — Saga 状态时间

**索引：**

- `(status, updated_at, id)` — 启动恢复顺序

**主要相关 Feature：**

- [`对话式项目创建与设置定稿`](features/对话式项目创建与设置定稿.md)

## 表：project_creation_session_references

应用库中属于创建 Session 的有序参考图清单和上传恢复检查点；清单与 Session 在同一个应用库事务中创建。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅供内部主键、外键和 JOIN
- `uuid` — TEXT NOT NULL UNIQUE，公开 Reference UUIDv7
- `project_creation_session_id` — INTEGER NOT NULL FK → `project_creation_sessions.id`，删除 Session 时级联删除
- `position` — INTEGER NOT NULL，1–16；`(project_creation_session_id, position)` 唯一
- `upload_uuid` / `file_uuid` — TEXT NOT NULL UNIQUE，预分配且稳定的项目 Upload/File UUIDv7
- `original_filename` — TEXT NOT NULL，首次提交清单中的文件名
- `declared_mime_type` — `image/png|image/jpeg|image/webp`
- `declared_byte_size` — INTEGER NOT NULL，1–33,554,432 字节
- `reference_role` — `auto|character|scene|prop|style`，默认 `auto`
- `title` — TEXT NOT NULL，最多 160 个 Unicode 字符；空旧值在读取时由文件名去扩展名派生
- `instruction` — TEXT NOT NULL，最多 2000 个 Unicode 字符
- `include_in_yolo` — INTEGER NOT NULL，`0|1`，默认 `1`
- `plan_source` — `system_default|agent_proposed|user_confirmed`；创建 Session 只写默认或用户确认来源
- `status` — `pending|uploading|ready|failed`
- `error_code` / 生命周期时间 — 单图上传错误和恢复排序事实

**索引：**

- `(project_creation_session_id, status, position)` — 恢复未完成图片并保持用户顺序

只有 `ready` Reference 的公开投影包含 `file_uuid`；响应同时返回服务端规范化后的引用计划与 `plan_source`。Upload UUID 和两个数据库的内部 ID 均不对外返回。相同创建幂等键必须同时匹配完整有序计划，不能在重试时静默改变图片用途。

**主要相关 Feature：**

- [`对话式项目创建与设置定稿`](features/对话式项目创建与设置定稿.md)

## 表：project_setup_drafts

项目库中的 Setup Draft 事实源；每个对话式草稿项目唯一一行。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅供内部主键和 JOIN
- `uuid` — TEXT NOT NULL UNIQUE，公开 Setup UUIDv7
- `project_id` — INTEGER NOT NULL UNIQUE FK → `projects.id`
- `status` — `draft|pending_confirmation|finalized|failed`
- `revision` — 从 1 开始的乐观锁版本
- `original_input` — TEXT NOT NULL，逐字保存的首页原始需求
- `project_name` / `generation_language` / `overall_style` — 可空草稿资料
- `format` / `aspect_ratio_mode` / `aspect_width` / `aspect_height` — 可空草稿绘本形式与规范比例
- `large_image_minimal_text` / `interaction_mode` / `comic_layout` — 由绘本形式约束的可空草稿专属设置
- `field_sources_json` / `missing_fields_json` — 合法 JSON object/array，记录 `system_default|agent_proposed|user_confirmed` 和缺失项
- `error_code` / `error_message` / `finalized_revision` — 失败和幂等定稿信息
- `finalized_at` / `failed_at` / 生命周期时间 — Setup Draft 生命周期

**索引：**

- `(project_id, status)` — 按项目读取 Setup Draft 状态

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

## 表：project_creation_reference_files

项目库中创建 Session、参考图清单项和正式 File 之间的恢复事实。File finalize 与该绑定在同一个项目库事务中提交，应用库即使在随后写检查点前崩溃也可据此校准。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅供内部主键和 JOIN
- `uuid` — TEXT NOT NULL UNIQUE，项目库 binding 的公开 UUIDv7；与应用库来源 `reference_uuid` 分离
- `project_id` — INTEGER NOT NULL FK → `projects.id`
- `creation_session_uuid` — TEXT NOT NULL，关联应用库公开 Session UUIDv7，不保存跨库内部 ID
- `reference_uuid` — TEXT NOT NULL UNIQUE，关联应用库公开 Reference UUIDv7
- `position` — INTEGER NOT NULL，1–16；`(creation_session_uuid, position)` 唯一
- `file_id` — INTEGER NOT NULL UNIQUE FK → `files.id`，`ON DELETE RESTRICT`
- `reference_role` / `title` / `instruction` / `include_in_yolo` / `plan_source` — Project Setup 中的事实计划；约束与应用库清单一致
- `premise_asset_id` — INTEGER NULL FK → `premise_assets.id`，`ON DELETE SET NULL`；一个来源 Asset 最多绑定一个创建参考图
- `imported_at` — DATETIME NULL，最近一次幂等导入或恢复时间
- `created_at` / `updated_at` — DATETIME NOT NULL

**索引：**

- `(project_id, creation_session_uuid, position)` — 按项目和 Session 恢复有序绑定
- `premise_asset_id IS NOT NULL` — partial unique index，避免一个来源 Asset 被多个创建 binding 复用

重复跨库绑定只校验项目、Session、位置与 File 的不可变身份，不会用应用库旧计划覆盖用户已在 Project Setup 中修改的事实。

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
2. 对话式创建在应用库单事务持久化 Session 和有序参考图清单，再按检查点建立 `draft` 项目与 Setup Draft。
3. 每张参考图使用预分配 Upload/File UUIDv7 重试；项目库单事务完成 File finalize、独立 binding UUID 与初始计划绑定，应用库随后标记 `ready`。启动 reconciliation 以项目绑定为事实校准跨库崩溃窗口。
4. 全部参考图 ready 后，普通 Chat bootstrap 在项目库单事务创建首个 Thread/Turn/Run/User Item/Job，并把 File References 按清单顺序挂到首个 User Item；无参考图时直接进入该步骤。
5. 草稿期对任一引用计划的更新使用 Setup `expected_revision`；成功后增加 revision、恢复可确认状态并清除旧错误。定稿事务把最终计划统一标记为 `user_confirmed`。
6. 用户明确选择“定稿并启动 YOLO”后，在项目库单事务写入正式绘本规格、项目资料、默认画风和 finalized Setup，再切换 `setup_status=ready`；同 revision 重放幂等成功。
7. 同一 bootstrap Turn 使用 `creation_session_uuid` 幂等创建一个 inline existing YOLO Workflow 和唯一 await；YOLO v6 按位置冻结完整计划，被排除项留在审计快照但不进入 Premise 或图片输入。等待终态后恢复同一 Run 并完成；其他生产写入继续失败关闭，后续 Turn 恢复普通 ready 能力。
8. 项目总纲与 Prompt 覆盖仅在 `ready` 后创建；总纲和 Prompt 通过版本历史保留可恢复来源。
9. 打开项目时验证 UUID、加锁、迁移、执行受控 reconciliation 并启动项目 Runtime；关闭只影响目标项目。
