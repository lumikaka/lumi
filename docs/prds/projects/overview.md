# 项目 — 本地项目与项目级创作配置

## 模块职责

项目模块负责本地项目的创建、打开、关闭、最近项目索引和项目级创作配置。项目是项目库中其他业务资源的归属边界；Story 总纲、绘本规格和 Prompt 覆盖都依附于项目，但各自保留独立的版本或配置生命周期。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | 项目目录与 SQLite 生命周期、项目资料、绘本规格、Story 总纲及其 `STORY.md` 投影、项目 Prompt 覆盖与版本恢复。 |
| 不负责 | Chapter 正文与生命周期、Provider 凭据、模型解析、聊天、文件资产、Premise、漫画生产和导出。 |

## 核心概念

### 双数据库边界

应用库保存本机的 `recent_projects` 发现索引；每个项目目录保存自己的 `project.sqlite` 与业务数据。两个库不共享内部 `id`，项目目录身份以 UUIDv7 为准而不是本机路径。

### 项目级创作配置

`project_story_profiles` 是项目总纲的结构化事实源，`STORY.md` 只是可读投影。`project_prompt_versions` 为内置 Prompt Catalog 保存项目级追加式覆盖，不把当前值直接写回内置定义。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `本地项目生命周期` | [`features/本地项目生命周期.md`](features/本地项目生命周期.md) | 创建、打开、关闭、重定位和最近项目管理。 |
| `项目资料与绘本规格` | [`features/项目资料与绘本规格.md`](features/项目资料与绘本规格.md) | 管理项目基本资料、生成语言和绘本尺寸约束。 |
| `故事总纲版本与STORY投影` | [`features/故事总纲版本与STORY投影.md`](features/故事总纲版本与STORY投影.md) | 维护总纲版本、外部导入和安全文件投影。 |
| `项目Prompt定制与版本恢复` | [`features/项目Prompt定制与版本恢复.md`](features/项目Prompt定制与版本恢复.md) | 覆盖内置 Prompt 并保留可恢复历史。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| 章节 | `chapters.project_id` 归属项目；Chapter 正文生成可读取项目总纲和 Prompt。 |
| AI 运行时 | `project_model_settings` 以项目为边界保存模型覆盖。 |
| 文件 | 章节导入的源文件通过项目 File 资产保存。 |
| 所有项目域 | URL、JSON、前端状态和实时 payload 都使用项目 UUIDv7。 |
