# 设定资产 — 数据模型

## 实体关系

```text
projects ──1 premise_profiles
   ├──< premise_sources ──< premise_setting_images ──> files
   ├──< project_creation_reference_files ──0..1> premise_assets
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

带创建参考的 setting generation 使用 production task 输入快照 v3，冻结 `reference_files` 与 `reference_composer_version=creation_reference_board/v1`。`premise_generation_steps.output_json` 在调用 Provider 前保存 `reference_board_file_uuid`，完成时以 JSON merge 追加 `setting_image_uuid`，保证重试仍能复用同一参考板。

## 表：premise_assets

- `uuid` — TEXT NOT NULL UNIQUE，公开资产 UUIDv7
- `project_id` / `actor_id` — INTEGER NOT NULL FK
- `current_variant_id` — INTEGER FK → `premise_asset_variants.id`，可空
- `asset_type` — `character`、`scene`、`prop` 或 `reference`
- `title` / `summary` / `position_json` / `crop_json` — 领域元数据和合法 JSON
- `revision` / `deleted_at` — 乐观锁与回收站状态

active 资产在 `(project_id, lower(title))` 上唯一；列表按项目、删除状态和更新时间读取。

included 首页参考按角色映射：`character|scene|prop` 保持同名 `asset_type`，`style|auto` 映射为 `reference`。Variant 使用既有 `source_type=manual` 并复用创建参考 File；tags 至少包含 `project-creation-reference` 与 `reference-role-<role>`。`project_creation_reference_files.premise_asset_id` 使用 partial unique index保证一对一绑定，归档后再次导入会恢复同一 Asset。

## 表：premise_asset_variants、tags 与 events

- Variant 保存 `uuid`、资产/File 内部关联、可选 setting image 来源、资产内 `version_no`、`manual|breakdown|replacement` 来源和 crop JSON。
- `(premise_asset_id, version_no)` 唯一，触发器禁止更新历史 Variant。
- tag 以资产内部关联保存；event 按资产内 sequence 追加。

## 数据生命周期

1. 保存 Premise 输入或生成候选图时追加 source/image 历史。
2. YOLO 对 included 首页参考先建立或恢复来源 Asset，再合成参考板生成新的 setting image；excluded 项只保留创建计划，不进入资产或图片请求。
3. 从候选图、手动上传或 Agent 结果创建资产和 Variant，切换 current variant 不覆盖历史。Breakdown 同名命中来源 Asset 时只追加 `breakdown_matched_project_reference` 事件，保留来源 current variant。
4. 资产删除先写 `deleted_at`，永久删除再检查活动工作和所有 File 保留引用；创建 binding 仍保留来源 File。
