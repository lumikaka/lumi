# 故事生产 — 数据模型

## 实体关系

```text
projects
├── premise_sources ──< premise_setting_images ──> files
├── premise_assets ──< premise_asset_variants ──> files ──> file_objects
└── chapters ──1 chapter_comic_states
                ├──< comic_sections
                │    ├──< comic_storyboard_variants
                │    └──< comic_image_variants ──> files
                └──< comic_chapter_snapshots

projects ──< comic_exports ──> chapters (chapter scope, nullable)
                         └──> files (ready ZIP, nullable)
projects ──< production_task_runs
```

所有表的 `id` 均为 `INTEGER PRIMARY KEY AUTOINCREMENT`，是 SQLite 64-bit rowid，只供内部主键、外键和 JOIN。所有带 `uuid` 的领域资源使用 UUIDv7，并且只有 `uuid` 能进入 URL、JSON、前端和 WebSocket payload。

## 表：premise_sources 与 premise_setting_images

`premise_sources` 保存不可变的批次输入、画风快照、Provider/模型、参数 JSON 和创建时间；按 `(project_id, created_at DESC, id DESC)` 稳定分页。`premise_setting_images.source_id` 可空地关联来源批次，并通过 `file_id` 引用 Asset Store File。前端读取批次页后，用公开 source UUIDv7 批量筛选当前可见页的 setting images，不读取整个项目历史。

## 表：chapters

保存项目 Chapter 的顺序、当前 Story 指针和生命周期状态。

- `uuid` — TEXT NOT NULL UNIQUE，公开 UUIDv7
- `project_id` — INTEGER NOT NULL FK → `projects.id`
- `volume_no` / `chapter_no` / `chapter_code` / `sort_order` — 项目内编号与排序
- `current_story_id` — INTEGER FK → `chapter_stories.id`，永久删除前置空
- `revision` — INTEGER NOT NULL，非负乐观锁
- `deleted_at` — DATETIME，可空；非空表示已进入 Chapter 回收站
- `created_at` / `updated_at` — DATETIME NOT NULL

active Chapter 的 code、卷章编号和 sort order 分别唯一；`(project_id, deleted_at, sort_order)` 支持 active/trashed 读取。

## 表：premise_assets

保存项目设定资产及其 current variant 指针。

- `uuid` — TEXT NOT NULL UNIQUE，公开 UUIDv7
- `project_id` / `actor_id` — INTEGER NOT NULL FK
- `current_variant_id` — INTEGER FK → `premise_asset_variants.id`，可空
- `asset_type` — `character`、`scene`、`prop` 或 `reference`
- `title` / `summary` — 展示标题与摘要
- `position_json` / `crop_json` — 合法 JSON
- `revision` — INTEGER NOT NULL，乐观锁版本，非负
- `deleted_at` — DATETIME，可空；非空表示回收站状态
- `created_at` / `updated_at` — DATETIME NOT NULL

active 资产按 `(project_id, lower(title))` 唯一；`(project_id, deleted_at, updated_at DESC, id DESC)` 支持 active/trashed 列表。

## 表：premise_asset_variants

保存不可变的设定资产图片版本。

- `uuid` — TEXT NOT NULL UNIQUE，公开 UUIDv7
- `premise_asset_id` — INTEGER NOT NULL FK → `premise_assets.id`，领域资产删除时级联
- `file_id` — INTEGER NOT NULL FK → `files.id`，删除受限
- `source_setting_image_id` — INTEGER FK → `premise_setting_images.id`，可空
- `version_no` — INTEGER NOT NULL，资产内递增且大于 0
- `source_type` — `manual`、`breakdown` 或 `replacement`
- `crop_json` — 合法 JSON
- `created_at` — DATETIME NOT NULL

`(premise_asset_id, version_no)` 唯一；触发器禁止更新历史 variant。

## 表：chapter_comic_states

每个 Chapter 恰有一个漫画状态。

- `uuid` — TEXT NOT NULL UNIQUE，公开 UUIDv7
- `chapter_id` — INTEGER NOT NULL UNIQUE FK → `chapters.id`
- `status` — `empty`、`draft`、`storyboarded`、`rendering` 或 `ready`
- `revision` — INTEGER NOT NULL，非负
- `created_at` / `updated_at` — DATETIME NOT NULL

`has_premise_assets` 和 `premise_asset_count` 是读取时按项目 active、current variant、ready File 计算的投影，不是持久化列。

## 表：comic_sections

保存 Chapter 内有序且可软删除的漫画段落。

- `uuid` — TEXT NOT NULL UNIQUE，公开 UUIDv7
- `chapter_comic_state_id` / `actor_id` — INTEGER NOT NULL FK
- `section_no` — INTEGER NOT NULL，大于 0
- `title` / `description_md` — 标题和正文
- `current_storyboard_variant_id` — INTEGER FK → `comic_storyboard_variants.id`，可空
- `current_image_variant_id` — INTEGER FK → `comic_image_variants.id`，可空
- `revision` — INTEGER NOT NULL，非负
- `deleted_at` — DATETIME，可空
- `created_at` / `updated_at` — DATETIME NOT NULL

active Section 的 `(chapter_comic_state_id, section_no)` 唯一，列表按 `section_no` 排序。

## 表：comic_storyboard_variants 与 comic_image_variants

两类 variant 都是追加写历史，按 `(comic_section_id, version_no)` 唯一。

- Storyboard 保存 `uuid`、`comic_section_id`、`actor_id`、`version_no`、`content_md`、`source_type` 和 `created_at`；`source_type` 为 `manual`、`generated` 或 `restore`。
- Image 保存 `uuid`、`comic_section_id`、`file_id`、可空 `image_generation_id`、`actor_id`、`version_no`、`source_type`、合法 JSON `input_snapshot` 和 `created_at`；`source_type` 为 `manual`、`generated`、`replacement` 或 `restore`。

## 表：comic_chapter_snapshots

保存 Chapter 漫画的不可变恢复点。

- `uuid` — TEXT NOT NULL UNIQUE，公开 UUIDv7
- `chapter_comic_state_id` / `actor_id` — INTEGER NOT NULL FK
- `version_no` — INTEGER NOT NULL，状态内递增
- `reason` — 创建原因；对外再归类为 `generated`、`manual` 或 `restore`
- `snapshot_json` — TEXT NOT NULL，合法 JSON；当前 schema 为 v2，读取兼容 v1
- `snapshot_hash` — TEXT NOT NULL，64 位 hash
- `created_at` — DATETIME NOT NULL

`(chapter_comic_state_id, version_no)` 唯一，列表按版本倒序；触发器禁止更新。

## 表：comic_exports

保存项目或 Chapter ZIP 导出及其冻结输入。

- `uuid` / `task_uuid` — TEXT NOT NULL UNIQUE，公开 UUIDv7
- `project_id` — INTEGER NOT NULL FK → `projects.id`
- `chapter_id` — INTEGER FK → `chapters.id`，项目级导出为空
- `scope` — `chapter` 或 `project`
- `format` — 当前仅 `zip`
- `status` — `queued`、`running`、`ready`、`failed` 或 `cancelled`
- `snapshot_json` / `snapshot_hash` — 合法 JSON 与 64 位 hash
- `output_file_id` — INTEGER FK → `files.id`，完成前可空
- `relative_path` / `error_code` — 导出投影路径与稳定错误码
- `created_at` / `completed_at` — 时间字段

相同项目、scope、Chapter、format 和 snapshot hash 的 ready 导出唯一。

## 表：production_task_runs

保存 Premise、漫画图片与导出后台任务。与本 domain 相关的字段为公开 `uuid`、`kind`、`resource_uuid`、`input_snapshot`、状态、幂等键、冻结的 `provider_uuid` / `model` / `model_source`、进度、重试次数、错误和时间字段。active 任务按 `(project_id, kind, resource_uuid)` 唯一。

## 数据生命周期

1. Section 或 Premise 图片写入新的不可变 variant，再切换 current 指针。
2. Chapter 关键变更创建 `comic_chapter_snapshots`；恢复时按快照创建 restore variant/状态，再追加一个 `snapshot_restored` 快照。
3. 导出预检生成就绪度；创建任务时在同一写锁窗口重新检查并冻结 `snapshot_json`。
4. Premise 软删除只设置 `deleted_at`；永久删除领域资产和其 variant，但历史仍引用的 File 保留。
5. 无任何 active、history、trash、snapshot 或 pending export 引用的 File 先逻辑删除，物理 object 仅能在 grace period 后由可审计 GC 回收。
