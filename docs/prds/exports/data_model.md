# 导出 — 数据模型

## 实体关系

```text
projects ──< comic_exports ──> chapters（chapter scope 时）
                    └──> production_task_runs（comic_export）
                    └──> files（仅旧 output_file_id，可空）
```

新导出产物在项目受控 `exports/` 目录中，不进入 `files` / `file_objects`。内部关联使用 bigint `id`，Export 和关联业务资源对外使用 UUIDv7。

## 表：comic_exports

- `uuid` — TEXT NOT NULL UNIQUE，公开 Export UUIDv7
- `project_id` / `chapter_id` / `production_task_run_id` — 内部项目、可选 Chapter 与任务关联
- `scope` — `project` 或 `chapter`
- `format` — `zip` 或 `pdf`
- `snapshot_json` / `snapshot_hash` — 冻结输入和内容身份，仅服务端解析；v6 冻结导出条目和缺图页的 `page_role`
- `status` — 导出生命周期状态
- `filename` / `relative_path` — 安全下载名与项目内受控相对路径
- `retention_days` / `expires_at` — 固定保留策略与精确到期边界
- `byte_size` / `content_sha256` — 发布后校验元数据
- `output_file_id` — 旧版兼容字段；新导出保持空

相同项目、范围、格式和 snapshot hash 的 ready canonical 产物唯一；复用未到期产物不会延长其到期时间。v6 `entries[]` 冻结可用图片的 Section UUID、`section_no`、`page_role`、`body_page_no`、Chapter 边界和图片资产信息；`missing_sections[]` 冻结缺图 Section 的 UUID、`section_no`、`page_role`、`body_page_no`、标题和 Chapter UUID，同时保留兼容字段 `missing_section_uuids`。`body_page_no` 在每个 Chapter 的 active `body` 中从 1 递增，特殊页为 0 并在 JSON 中省略。历史 PDF v5 没有页面角色，兼容渲染时按 `body` 处理。

## 数据生命周期

1. 预检当前 ready 图片；至少一张 ready `body` 才可导出，任一已存在封面、正文页或封底缺图都使 `complete=false`。用户选择 ZIP/PDF 和是否允许缺图后创建 Export 与任务。
2. Worker 使用冻结快照按装订顺序写入 Export UUID 专属 `.part`；v6 ZIP 使用 `<chapter>/front-cover.ext`、`<chapter>/pages/page-%03d.ext` 和 `<chapter>/back-cover.ext`，其中正文文件名使用冻结的 `body_page_no`。PDF 让封面与封底独占 A4 页、正文页按冻结布局排版。同步后原子 rename 并发布 hash、大小和下载信息。
3. 到期立即从 REST 列表隐藏并拒绝下载；项目 Runtime 启动时和随后每小时清理终态记录与受控文件。
