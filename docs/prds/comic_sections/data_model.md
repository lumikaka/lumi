# 页面 / 画面段落 — 数据模型

## 实体关系

```text
chapters ──1 chapter_comic_states ──< comic_sections
                                      ├──< comic_storyboard_variants
                                      ├──< comic_image_generations
                                      ├──< comic_image_variants ──> files
                                      └──< comic_section_events

chapter_comic_states ──< comic_chapter_snapshots
```

所有资源以内部 bigint `id` 关联；Chapter、state、Section、Variant、生成和 snapshot 对外使用 UUIDv7。

数据库与 API 保留 Chapter、Section、Storyboard 和 Variant 技术名。面向用户时，非 `vertical_strip` 显示“绘本 / 页面 / 页面脚本”，`vertical_strip` 显示“章节 / 画面段落 / 分镜脚本”；`comic_image_variants` 在两种形式下都显示为“页面图片版本”。

## 表：chapter_comic_states 与 comic_sections

- `chapter_comic_states` — 每 Chapter 一条，保存公开 UUID、`empty|draft|storyboarded|rendering|ready` 状态、revision 和时间。
- `comic_sections` — 关联 comic state 与 actor，保存 `section_no`、`page_role`、标题、`description_md`、current storyboard/image 内部指针、revision、`deleted_at` 和时间。

`page_role` 为 `front_cover|body|back_cover`，非空且默认 `body`。active Section 的 `(chapter_comic_state_id, section_no)` 唯一；active `front_cover` 和 `back_cover` 分别通过 `(chapter_comic_state_id, page_role)` 部分唯一索引限制为最多一个。按 `section_no` 读取时，服务端把封面归一到首位、封底归一到末位，正文页位于两者之间。`has_premise_assets` 与 `premise_asset_count` 是读取时投影，不持久化。

条漫 `vertical_strip` 在应用层只允许 `body`，并允许删除最后一个画面段落回到 empty。普通绘本的全空 comic state 必须先创建 `body`，新增特殊页前必须已有正文页；删除唯一 active `body`、把它转换为特殊角色，或恢复 empty / 只含特殊页的快照都会在应用层被拒绝。`chapter_comic_states.status=storyboarded` 必须至少有一个 active `body`，且所有 active Section 都已选择 current storyboard；`ready` 同样要求至少一个 active `body`，且所有 active Section 都已选择 current image。

## 枚举：page_role

- `front_cover` — 普通绘本的可选封面，最多一个并固定为首页。
- `body` — 正文页；条漫画面段落也固定使用此角色。
- `back_cover` — 普通绘本的可选封底，最多一个并固定为末页。

## Variant、生成与事件表

- `comic_storyboard_variants` — 追加分镜版本，保存内容、`manual|generated|restore` 来源和版本号。
- `comic_image_generations` — 图片生成请求的状态、冻结输入、Provider/模型和公开 UUID；新任务的输入快照 v5 冻结 `page_role` 与已按角色组合完成的图片 Prompt。
- `comic_image_variants` — 追加图片版本，关联 File、可选 generation 和 Section Premise 快照。
- `comic_section_events` — Section 内 append-only 事件。

两类 Variant 都以 `(comic_section_id, version_no)` 唯一；current 指针只能引用本 Section 的历史版本。

图片生成 v5 在提交结果时再次比较当前 Section 与冻结的 `page_role`；生成期间若从封面、正文页或封底转换为另一角色，旧结果拒绝落库并要求按当前角色重新生成。v1–v4 历史任务没有角色字段，继续按旧兼容路径提交。图片 Prompt 分别使用 `chapter/cover_before_image`、`chapter/before_image` 与 `chapter/back_cover_before_image`，避免特殊页继承正文句数、互动提问、无字或漫画分格约束。

## 表：comic_chapter_snapshots

每次关键状态变更可创建快照。表中保存公开 `uuid`、`chapter_comic_state_id`、`actor_id`、`version_no`、`reason`、`snapshot_json`、`snapshot_hash` 和 `created_at`；API `source` 由 `reason` 归类投影，schema version 保存在 `snapshot_json.version` 而不是独立列。快照 schema v3 在每个 Section 中冻结 `page_role`；v1/v2 没有该字段，读取与恢复时按 `body` 兼容。JSON/hash 仅用于服务端恢复和校验，API 只返回安全详情投影。

## 数据生命周期

1. 创建或打开 Chapter 漫画时得到唯一 comic state；Section 按角色插入装订序列，正文重排、角色转换和逻辑删除都会再次归一顺序。
2. 脚本/图片编辑和生成追加 Variant，用户显式选择 current 版本；AI 整章脚本生成只替换 `body`，保留现有封面和封底。
3. 快照记录当前有序 Section 及其页面角色；恢复生成新的可追溯状态，不直接篡改历史 Variant。

回退该 migration 时，旧 schema 与旧 worker 无法表达或解释页面角色。active / 软删除 Section 仍有非 `body`、Chapter 快照含非 `body`，或项目仍持久化任一 Export v6、`comic_export` v6 任务、漫画图片 v5 任务、YOLO v5 Workflow 时，down migration 都拒绝降级。这些 Export、任务或 Workflow 即使只含正文页也会阻止回退，因为旧运行时不认识对应快照版本。
