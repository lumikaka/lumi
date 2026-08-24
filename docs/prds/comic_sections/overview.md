# 漫画段落 — 分镜、图片与可恢复快照

## 模块职责

漫画段落模块负责 Chapter 的漫画状态、有序 Section、分镜与图片 Variant、Section Premise 参考、生成冲突和快照恢复。它以 Chapter 为父资源，但拥有当前漫画产物的可编辑状态和历史。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | Chapter comic state、Section、排序、分镜、图片、Variant 选择、Section Premise、快照与恢复。 |
| 不负责 | Chapter 正文、Premise 资产本身、File/Object 存储、模型选择和最终 ZIP/PDF 导出保留。 |

## 核心概念

### 当前漫画状态

每个 Chapter 恰有一个 `chapter_comic_states`。active Section 的 current storyboard 和 current image 指向不可变 Variant；编辑、生成和恢复创建新历史或切换指针，不覆盖既有内容。

### 快照恢复

快照是 Chapter 漫画状态的不可变记录，含有序 Section、分镜、当前图片和 Premise 引用。恢复前阻止活动生成，详情以媒体可用状态而非内部路径或 hash 暴露历史。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `漫画分镜与图片生产` | [`features/漫画分镜与图片生产.md`](features/漫画分镜与图片生产.md) | 编辑 Section、生产分镜/图片并选择当前 Variant。 |
| `漫画快照与恢复` | [`features/漫画快照与恢复.md`](features/漫画快照与恢复.md) | 预览不可变快照并安全恢复 Chapter 漫画状态。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| 章节 | Chapter 是 comic state 的父资源；Chapter 删除会按外键清理自身漫画记录。 |
| 设定资产 | Section Premise 仅选择 active、current variant 和 ready File 的设定资产。 |
| 文件 | 图片 Variant 与 Premise 参考通过受控 File 投影读取。 |
| 工作流 / AI 运行时 | 分镜和图片生成可使用冻结任务或 Workflow；业务结果由本模块提交。 |
| 导出 | Export 读取当前 ready 图片并冻结快照，不拥有 Section 编辑状态。 |
