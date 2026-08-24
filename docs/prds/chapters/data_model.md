# 章节 — 数据模型

## 实体关系

```text
projects ──< chapters ──< chapter_stories
                 │             ↑
                 │             └── story_sources ──< story_source_items ──> files
                 └──1 chapter_comic_states（由 comic_sections 管理）
```

所有表以内部 bigint `id` 关联；`chapters`、正文和来源对外都使用 UUIDv7。

## 表：chapters

- `uuid` — TEXT NOT NULL UNIQUE，公开 Chapter UUIDv7
- `project_id` — INTEGER NOT NULL FK → `projects.id`
- `volume_no` / `chapter_no` / `chapter_code` / `sort_order` — 项目内编号和顺序
- `title` — 展示标题
- `current_story_id` — INTEGER FK → `chapter_stories.id`，永久删除前置空
- `revision` — 非负乐观锁版本
- `deleted_at` — 非空时位于回收站

active Chapter 的 code、卷章编号和 sort order 分别唯一；列表索引为 `(project_id, deleted_at, sort_order)`。

## 表：story_sources 与 story_source_items

导入或人工写入的来源批次及其有序内容项。

- `story_sources.project_id` / `actor_id` — 内部项目和操作者关联
- `source_type` — `manual_entry`、`file_import` 或 `manual_edit`
- `request_hash` — 项目内来源请求幂等键
- `story_source_items.source_id` / `chapter_id` — 来源和可选 Chapter 关联
- `file_id` — INTEGER FK → `files.id`，导入原文件的受控资产引用

## 表：chapter_stories

追加式 Chapter 正文版本。

- `uuid` — TEXT NOT NULL UNIQUE，公开正文版本 UUIDv7
- `chapter_id` / `actor_id` / `story_source_id` / `story_source_item_id` — 内部关联
- `version_no` — Chapter 内递增，`(chapter_id, version_no)` 唯一
- `content` / `content_format` / `content_hash` / `char_count` — 正文快照
- `source_type` — `manual_entry`、`file_import` 或 `manual_edit`

更新触发器禁止修改历史正文。

## 数据生命周期

1. 创建或导入时写入 Chapter、来源和第一条正文版本，并设置 current 指针。
2. 手动编辑或生成结果始终追加 `chapter_stories`，再以 Chapter revision 保护当前指针。
3. 删除先写 `deleted_at`；恢复验证 revision；永久删除检查活动引用后按外键清理 Chapter 自有内容。
