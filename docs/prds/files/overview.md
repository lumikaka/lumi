# 文件 — 项目资产、完整性与受控回收

## 模块职责

文件模块负责项目内二进制资产的受控导入、逻辑 File、去重 Object、内容读取、缩略图、完整性扫描、reconcile、GC 和维护任务。它提供可被 Story、Chat、Premise 和漫画复用的本地资产基础，不定义这些业务域对图片或文档的语义。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | 上传暂存、对象校验与去重、File 生命周期、媒体内容服务、缩略图、完整性扫描、隔离、GC 计划和维护任务。 |
| 不负责 | Premise/漫画资产的业务元数据、Chapter 正文含义、聊天工具语义和新漫画导出产物的生命周期。 |

## 核心概念

### File 与 File Object

`files` 是可被业务引用的逻辑资产；`file_objects` 是按项目 SHA-256 去重的不可变物理对象。逻辑 File 可软删除而对象保留，直到所有结构化和历史引用均解除并通过受控 GC。

### 维护即业务安全边界

reconcile、扫描、修复、GC 与缩略图重建均生成可查询任务或计划，不允许任意路径删除。磁盘路径从不作为公开 API 字段返回。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `项目文件资产管理` | [`features/项目文件资产管理.md`](features/项目文件资产管理.md) | 上传、读取、更新、逻辑删除、恢复和缩略图。 |
| `资产完整性扫描与修复` | [`features/资产完整性扫描与修复.md`](features/资产完整性扫描与修复.md) | 扫描缺失、损坏、隔离和 reconcile 状态。 |
| `资产GC与维护任务` | [`features/资产GC与维护任务.md`](features/资产GC与维护任务.md) | 审计式 GC dry-run、应用和维护任务恢复。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| 项目 | 所有 File/Object 属于单一项目；首页创建参考图以稳定 Upload/File UUIDv7 导入，并由 `project_creation_reference_files` 保护跨库恢复窗口。项目关闭时 Runtime 按项目停止。 |
| 章节 | 导入源使用 `story_source_items.file_id` 关联 File。 |
| Premise 资产 / 漫画 Section | 设定图、variant 和 Section 图片引用受控 File。 |
| Chat Thread | `file` Reference 及其他资源 Reference 的 `image_file_id` 冻结关联 File；不允许跨项目引用或暴露磁盘路径。 |
| 导出 | 新漫画导出不写入 Asset Store；旧 `output_file_id` 仅为到期兼容回收。 |
