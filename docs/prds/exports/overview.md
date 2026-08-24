# 导出 — 漫画短期交付与到期清理

## 模块职责

导出模块负责把项目或 Chapter 的当前 ready 漫画图片冻结为可下载的 ZIP/PDF 短期产物。它管理导出就绪度、完整或部分导出确认、格式、任务、下载、复用、保留期和到期清理；当前实现只覆盖漫画导出。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | `comic_exports`、导出快照、ZIP/PDF 格式、下载名、短期保留、到期拒绝与清理。 |
| 不负责 | Section 编辑、图片生成、Premise 资产、通用 File 上传和未来未实现的其他导出类型。 |

## 核心概念

### 导出冻结

创建导出时在同一写锁窗口复核就绪度并冻结有序图片、范围和格式。任务重试和 canonical 复用以 frozen snapshot/hash 为准，不因后续 Section 编辑改变产物。

### 短期派生产物

新 ZIP/PDF 仅写入项目根受控 `exports/`，不进入 Asset Store。默认保留 7 天；`expires_at` 是下载和列表可见性的精确边界。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `漫画导出与短期交付` | [`features/漫画导出与短期交付.md`](features/漫画导出与短期交付.md) | 选择格式、创建导出、下载并在到期后受控清理。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| 漫画 Section | 读取 active Section 的 current ready 图片，并冻结其快照。 |
| AI 运行时 | `comic_export` 生产任务提供状态、取消和重试。 |
| 文件 | 新导出不创建 File/Object；旧 `output_file_id` 仅在到期兼容回收时使用。 |
| 项目 / Chapter | Export scope 为 project 或 chapter，公开 API 只接受 UUIDv7。 |
