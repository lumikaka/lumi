# 文件 — 数据模型

## 实体关系

```text
projects ──< file_objects ──< files
                              ├──< upload_stashed（完成后关联 File/Object）
                              ├──< asset_maintenance_runs ──< asset_maintenance_events
                              ├──< integrity_scans ──< integrity_findings
                              └──< asset_gc_plans ──< asset_gc_entries

业务表通过内部 file_id 引用 files；对外统一投影 File UUIDv7。
```

Chat 的 `chat_context_references.file_id` 指向普通 File Reference，`image_file_id` 指向输入接受时冻结的图片。两者均使用内部 bigint FK；Reference 对外只返回资源 UUIDv7 和 `image_available`。

## 表：file_objects

按项目内容去重的底层对象。

- `uuid` — TEXT NOT NULL UNIQUE，公开 Object UUIDv7，仅用于受控诊断投影
- `project_id` — INTEGER NOT NULL FK → `projects.id`
- `sha256` / `key_path` — 项目内唯一内容 hash 和受控相对存储键
- `mime_type` / `canonical_ext` / `byte_size` / 尺寸或时长 — 物理内容元数据
- `state` — `pending`、`ready`、`missing`、`corrupt` 或 `quarantined`

`(project_id, sha256)` 与 `(project_id, key_path)` 唯一；ready 对象的内容字段不可更新。

## 表：files 与 upload_stashed

`files` 是逻辑资产，保存 `uuid`、项目/Object 内部外键、kind、purpose、展示名、来源类型、可选来源 File、JSON 元数据、actor 和 `deleted_at`。`upload_stashed` 保存接收中的上传、保留的 File UUID、对象元数据、`receiving|ready|consuming|consumed|failed|expired` 状态与到期时间。

## 维护表

- `asset_maintenance_runs` / `asset_maintenance_events` — reconcile、扫描、缩略图、上传清理和 GC 应用的可恢复任务及 append-only 事件。
- `integrity_scans` / `integrity_findings` — light/full 扫描、进度、摘要、问题类型、严重度和处理状态。
- `asset_gc_plans` / `asset_gc_entries` — dry-run 快照、预计字节数和待处理对象审计清单。

## 数据生命周期

1. 上传先在 `upload_stashed` 接收和验证，完成后原子写入或复用 Object，再创建逻辑 File。
2. Chat 图片先完成上传和 finalize，再以稳定 File UUID 创建 Reference；Chat 服务不消费临时 Upload UUID。
3. 业务资源只关联 File；逻辑删除解除普通可见性但不立即删除 Object。冻结 Reference 继续保护 `image_file_id`，目标资源删除不重写其快照。
4. 扫描和 reconcile 记录对象状态；GC 必须先创建可审计计划并复检包括 Chat Reference 在内的全部结构化引用，再应用未过期且未失效的计划。
