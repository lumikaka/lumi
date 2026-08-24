# 项目 — 数据模型

## 实体关系

```text
应用库
recent_projects ──(uuid / root_path)──> 本机发现索引

项目库
projects ──< actors
   ├──1 project_picture_book_profiles
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
- `revision` — 非负乐观锁版本
- `actors.uuid` — TEXT NOT NULL UNIQUE，公开操作者 UUIDv7
- `actors.project_id` — INTEGER FK → `projects.id`

## 表：project_picture_book_profiles

每个项目最多一行的绘本规格。

- `project_id` — INTEGER NOT NULL UNIQUE FK → `projects.id`
- `page_width` / `page_height` / `page_count` — 页面尺寸和页数约束
- `revision` — 非负乐观锁版本

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

1. 创建项目后写入 `projects`、默认 actor、初始目录文件与项目库。
2. 项目总纲、绘本规格与 Prompt 覆盖在项目库更新；总纲和 Prompt 通过版本历史保留可恢复来源。
3. 打开项目时验证 UUID、加锁、迁移、执行受控 reconciliation 并启动项目 Runtime；关闭只影响目标项目。
