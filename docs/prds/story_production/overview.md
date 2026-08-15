# 故事生产 — 从设定资产到可恢复漫画交付

## 模块职责

故事生产模块负责把项目 Story、Premise 设定资产和 Chapter 组织为可编辑、可生成、可恢复并可导出的漫画产物。具体能力见 Feature 列表。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | Premise 生成批次与资产生命周期、Chapter 回收站、章节漫画 Section/分镜/图片、生成上限、快照预览与恢复、章节或项目的原图 ZIP / A4 PDF 导出及其 7 天保留清理。 |
| 不负责 | Provider 凭据和模型目录、通用 Asset Store 发布与 GC、Story 正文版本管理、项目外云端协作。 |

## 核心概念

### 当前生产状态

Chapter 的当前漫画由 active Section、每个 Section 的 current storyboard 与 current image 组成。current 指针只指向不可变 variant；编辑和恢复通过创建新 variant 或切换状态完成。

### 生产快照

生产快照是 Chapter 漫画状态的不可变记录。列表只给出版本、原因、来源和 Section 数量，详情才解析分镜、当前图片和 Premise 参考图，并显式报告媒体可用性。

### 导出就绪度

导出就绪度以 active Chapter、active Section 和 ready current image 为准。全图、部分缺图、零可用图分别进入格式选择、缺图二次确认和禁止导出流程。

### 导出保留边界

漫画 ZIP/PDF 是可重新生成的短期派生产物，只保存到项目根 `exports/` 并固定保留 7 天。到期以 `expires_at` 为精确事实边界：REST 立即隐藏并拒绝下载，项目 River Runtime 在启动时及每小时清理文件、Export 与对应终态任务。

### 逻辑删除与永久删除

Premise 资产与 Chapter 先进入各自回收站；永久删除只移除已入回收站且未被活动任务、Workflow 或会话引用的领域记录。Chapter 删除级联自身漫画状态，但不会直接删除共享 Asset Store 文件；Premise File 是否可软删除仍按所有历史、快照和任务引用重新判断。

### 长历史边界

Premise 生成批次和漫画导出按稳定服务端顺序分页。Premise 页面只批量读取已加载 source 页对应的 setting images；项目级与 Chapter 级导出各自维护筛选和分页，不先拉取全项目历史再在浏览器拆分。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `漫画可恢复生产与交付` | [`features/漫画可恢复生产与交付.md`](features/漫画可恢复生产与交付.md) | 控制 Section 生成规模、提示设定缺失、预览/恢复历史状态并安全导出。 |
| `设定资产生命周期` | [`features/设定资产生命周期.md`](features/设定资产生命周期.md) | 提供回收站、永久删除、批量清空和跨历史引用保护。 |
| `章节生命周期` | [`features/章节生命周期.md`](features/章节生命周期.md) | 管理 Chapter 的 active、回收站、恢复、单项永久删除和安全批量清空。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| AI 运行时 | 漫画和 Premise 生成在创建任务时解析并冻结 Provider、模型与来源。 |
| Story | Chapter 是漫画状态、快照和章节级导出的归属资源。 |
| Asset Store | 设定图和 Section 图片通过 `files` / `file_objects` 管理。新漫画 ZIP/PDF 不进入 Asset Store；旧版 `output_file_id` 只在到期兼容回收时使用受控 export-only GC。 |
| 实时通信 | `/api/v1/ws` 仅发布公开 UUIDv7 和刷新提示，REST/SQLite 是事实源。 |
