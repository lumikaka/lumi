# 设定资产 — 数据模型

## 实体关系

```text
projects ──1 premise_profiles
   ├──< premise_sources ──< premise_setting_images ──> files
   └──< premise_assets ──< premise_asset_variants ──> files
                          ├──< premise_asset_tags
                          └──< premise_asset_events

premise_assets 的 current variant 可被 comic_sections 作为 Section Premise 参考。
```

内部关联使用 bigint `id`；公开 Premise、source、setting image、asset 和 variant 资源使用 UUIDv7。

## 表：premise_profiles、premise_sources 与 premise_setting_images

- `premise_profiles` — 每项目一行，保存 `default_style`、当前 source/image 指针和 revision。
- `premise_sources` — 不可变批次输入，保存 source text、画风快照、Provider/模型、参数 JSON、`ignored_at` 和 revision；按 `(project_id, created_at DESC, id DESC)` 稳定分页。
- `premise_setting_images` — 候选设定图，`source_id` 可空关联批次，`file_id` 关联受控 File，保存 origin 和 Prompt。
- `premise_generation_steps` — 生成批次的可恢复阶段记录，业务任务状态由 `ai_runtime` 管理。

## 表：premise_assets

- `uuid` — TEXT NOT NULL UNIQUE，公开资产 UUIDv7
- `project_id` / `actor_id` — INTEGER NOT NULL FK
- `current_variant_id` — INTEGER FK → `premise_asset_variants.id`，可空
- `asset_type` — `character`、`scene`、`prop` 或 `reference`
- `title` / `summary` / `position_json` / `crop_json` — 领域元数据和合法 JSON
- `revision` / `deleted_at` — 乐观锁与回收站状态

active 资产在 `(project_id, lower(title))` 上唯一；列表按项目、删除状态和更新时间读取。

## 表：premise_asset_variants、tags 与 events

- Variant 保存 `uuid`、资产/File 内部关联、可选 setting image 来源、资产内 `version_no`、`manual|breakdown|replacement` 来源和 crop JSON。
- `(premise_asset_id, version_no)` 唯一，触发器禁止更新历史 Variant。
- tag 以资产内部关联保存；event 按资产内 sequence 追加。

## 数据生命周期

1. 保存 Premise 输入或生成候选图时追加 source/image 历史。
2. 从候选图、手动上传或 Agent 结果创建资产和 Variant，切换 current variant 不覆盖历史。
3. 资产删除先写 `deleted_at`，永久删除再检查活动工作和所有 File 保留引用。
