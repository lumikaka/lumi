# 章节 — 章节正文、生命周期与生成

## 模块职责

章节模块负责项目内 Chapter 的排序、正文版本、源文件导入、回收站和 Story 生成。Chapter 是故事正文的聚合根，也是漫画状态与章节级导出的归属资源；漫画 Section 本身由 `comic_sections` 管理。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | Chapter 编号与排序、正文追加版本、导入、回收站、永久删除、章节 Story 生成和批量规划。 |
| 不负责 | 项目总纲与 Prompt 配置、漫画 Section/图片/快照、文件物理存储、通用任务调度和导出产物。 |

## 核心概念

### 当前正文

`chapters.current_story_id` 只指向不可变 `chapter_stories` 历史中的一个版本。编辑、导入和 AI 生成均追加正文版本，再以 revision 条件切换当前指针。

### 生命周期边界

普通删除只把 Chapter 移入回收站。永久删除会先检查活动 Story、Production、Workflow 与 Chat 引用；安全删除只移除 Chapter 自有记录，不主动回收共享 File。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `章节创作与正文版本` | [`features/章节创作与正文版本.md`](features/章节创作与正文版本.md) | 编辑、导入、排序和追加式正文历史。 |
| `章节生命周期` | [`features/章节生命周期.md`](features/章节生命周期.md) | 回收站、恢复和受活动引用保护的永久删除。 |
| `章节AI生成与批量规划` | [`features/章节AI生成与批量规划.md`](features/章节AI生成与批量规划.md) | 生成章节、批量规划并安全提交结果。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| 项目 | Chapter、来源和正文版本均以 `projects.id` 为归属。 |
| 文件 | 导入源内容可通过 `story_source_items.file_id` 关联受控 File。 |
| 漫画 Section | Chapter 是 `chapter_comic_states`、快照和章节范围导出的父资源。 |
| AI 运行时 | Story 任务冻结模型和 Prompt，并通过 REST 任务资源恢复状态。 |
