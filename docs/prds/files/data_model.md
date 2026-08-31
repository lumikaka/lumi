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

项目创建的 `project_creation_reference_files.file_id` 在首页 File finalize 的同一事务中写入，保存创建 Session/Reference UUIDv7、有序位置和视觉计划。它既是应用库 Reference 状态的跨库恢复事实，也是 File/Object 的结构化保留来源；正式首个 User Item 创建后，同一 File 还会由 `chat_context_references` 冻结引用。included 项成为来源 Premise Asset 时，其 manual Variant 继续指向同一个 `files.id`。

## 表：file_objects

按项目内容去重的底层对象。

- `uuid` — TEXT NOT NULL UNIQUE，公开 Object UUIDv7，仅用于受控诊断投影
- `project_id` — INTEGER NOT NULL FK → `projects.id`
- `sha256` / `key_path` — 项目内唯一内容 hash 和受控相对存储键
- `mime_type` / `canonical_ext` / `byte_size` / 尺寸或时长 — 物理内容元数据
- `state` — `pending`、`ready`、`missing`、`corrupt` 或 `quarantined`

`(project_id, sha256)` 与 `(project_id, key_path)` 唯一；ready 对象的内容字段不可更新。

## 表：files 与 upload_stashed

`files` 是逻辑资产，保存 `uuid`、项目/Object 内部外键、kind、purpose、展示名、来源类型、可选来源 File、JSON 元数据、actor 和 `deleted_at`。`premise_reference_board` 只允许 PNG，namespace 为 `premise/reference-boards`，元数据保存任务 UUID、`creation_reference_board/v1`、有序来源 File UUID 与数量。`upload_stashed` 保存接收中的上传、保留的 File UUID、对象元数据、`receiving|ready|consuming|consumed|failed|expired` 状态与到期时间。

## 维护表

- `asset_maintenance_runs` / `asset_maintenance_events` — reconcile、扫描、缩略图、上传清理和 GC 应用的可恢复任务及 append-only 事件。
- `integrity_scans` / `integrity_findings` — light/full 扫描、进度、摘要、问题类型、严重度和处理状态。
- `asset_gc_plans` / `asset_gc_entries` — dry-run 快照、预计字节数和待处理对象审计清单。

## 数据生命周期

1. 上传先在 `upload_stashed` 接收和验证，完成后原子写入或复用 Object，再创建逻辑 File。可信业务流程可预分配稳定 Upload/File UUIDv7；失败、过期或进程重启后的重放复用同一逻辑身份。
2. 首页项目创建参考图在 File finalize 的同一事务写入 `project_creation_reference_files`；应用库检查点丢失时以该项目库事实恢复，不创建第二个 File。
3. Chat 图片先完成上传和 finalize，再以稳定 File UUID 创建 Reference；Chat 服务不消费临时 Upload UUID。首页首轮复用已 finalize File，并把 References 与 User Item 原子创建。
4. YOLO setting task 读取 included 原图并合成/复用一个参考板；board UUID 先写入 `premise_generation_steps.output_json`。任务完成以 JSON merge 保留 board 和 setting UUID，取消或重试不重复创建有效 board。
5. 业务资源只关联 File；逻辑删除解除普通可见性但不立即删除 Object。冻结 Reference、创建绑定、来源 Variant 和任务/Workflow JSON 快照继续保护 File/Object，目标资源删除不重写其快照。
6. 扫描和 reconcile 记录对象状态；GC 必须先创建可审计计划并复检包括 Chat Reference、项目创建绑定与所有快照 UUID 在内的全部结构化引用，再应用未过期且未失效的计划。
