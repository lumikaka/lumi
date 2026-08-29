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
- `comic_sections` — 关联 comic state 与 actor，保存 section_no、标题、`description_md`、current storyboard/image 内部指针、revision、`deleted_at` 和时间。

active Section 的 `(chapter_comic_state_id, section_no)` 唯一，按 section_no 读取。`has_premise_assets` 与 `premise_asset_count` 是读取时投影，不持久化。

## Variant、生成与事件表

- `comic_storyboard_variants` — 追加分镜版本，保存内容、`manual|generated|restore` 来源和版本号。
- `comic_image_generations` — 图片生成请求的状态、冻结输入、Provider/模型和公开 UUID。
- `comic_image_variants` — 追加图片版本，关联 File、可选 generation 和 Section Premise 快照。
- `comic_section_events` — Section 内 append-only 事件。

两类 Variant 都以 `(comic_section_id, version_no)` 唯一；current 指针只能引用本 Section 的历史版本。

## 表：comic_chapter_snapshots

每次关键状态变更可创建快照，保存公开 UUID、Chapter comic state 内部关联、version_no、reason、source、schema version、快照 JSON/hash 和创建时间。JSON/hash 仅用于服务端恢复和校验，API 只返回安全详情投影。

## 数据生命周期

1. 创建或打开 Chapter 漫画时得到唯一 comic state；Section 追加、排序和逻辑删除更新当前状态。
2. 分镜/图片编辑和生成追加 Variant，用户显式选择 current 版本。
3. 快照记录当前有序 Section 状态；恢复生成新的可追溯状态，不直接篡改历史 Variant。
