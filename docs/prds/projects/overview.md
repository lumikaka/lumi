# 项目 / 绘本项目 — 本地项目与项目级创作配置

## 模块职责

项目模块负责本地项目的创建、打开、关闭、最近项目索引、带可选参考图的对话式草稿初始化和项目级创作配置。项目是项目库中其他业务资源的归属边界；Story 总纲、绘本规格和 Prompt 覆盖都依附于项目，但各自保留独立的版本或配置生命周期。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | 项目目录与 SQLite 生命周期、首页创建 Saga、草稿设置与定稿、项目资料、绘本规格、Story 总纲及其 `STORY.md` 投影、项目 Prompt 覆盖与版本恢复。 |
| 不负责 | Chapter 正文与生命周期、Provider 凭据、模型解析、聊天、文件资产、Premise、漫画生产和导出。 |

## 核心概念

### 项目与绘本项目

`project` 的正式中文为“项目”，需要强调作品语境时可称“绘本项目”。它是本地目录、SQLite、项目级设定和执行记录的归属边界，不替代内部 `chapter` 资源；普通绘本形式下 `chapter` 面向用户称“绘本”，条漫下称“章节”。

### 双数据库边界

应用库保存本机的 `recent_projects` 发现索引；每个项目目录保存自己的 `project.sqlite` 与业务数据。两个库不共享内部 `id`，项目目录身份以 UUIDv7 为准而不是本机路径。

最近项目卡片的 `cover_image_url` 是读取项目库得到的临时投影，不写入应用库：按 Chapter 顺序选择第一本存在 ready 候选的绘本，并在该绘本内优先封面、其次第一张正文图，永不使用封底。

### 项目级创作配置

`project_story_profiles` 是项目总纲的结构化事实源，`STORY.md` 只是可读投影。`project_prompt_versions` 为内置 Prompt Catalog 保存项目级追加式覆盖，不把当前值直接写回内置定义。

### 草稿设置门禁

首页的一行需求和有序可选参考图先创建 `setup_status=draft` 的真实项目和普通 Agent 对话。参考图在首轮对话前以稳定 Upload/File UUIDv7 落入项目 Asset Store，并原子挂到首个 User Item；刷新或进程重启后可按创建 Session 的清单续传。草稿只允许读取事实、继续聊天和维护 Project Setup；Story、图片、Workflow、生产与导出统一以稳定错误码拒绝。Agent 只在必要时补问影响首章的少量问题，并以稳定确认项同时说明“定稿并启动 YOLO”。用户明确选择后，定稿事务写入唯一正式绘本规格并把项目切换为 `ready`；同一 bootstrap Turn 只能幂等启动 existing YOLO，其他生产写入继续失败关闭，后续 Turn 恢复普通 ready 能力。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `本地项目生命周期` | [`features/本地项目生命周期.md`](features/本地项目生命周期.md) | 创建、打开、关闭、重定位和最近项目管理。 |
| `对话式项目创建与设置定稿` | [`features/对话式项目创建与设置定稿.md`](features/对话式项目创建与设置定稿.md) | 从首页原始需求与可选参考图可靠创建草稿项目、首个 Agent Turn，并在确认后原子定稿。 |
| `项目资料与绘本规格` | [`features/项目资料与绘本规格.md`](features/项目资料与绘本规格.md) | 管理项目基本资料、生成语言和绘本尺寸约束。 |
| `故事总纲版本与STORY投影` | [`features/故事总纲版本与STORY投影.md`](features/故事总纲版本与STORY投影.md) | 维护总纲版本、外部导入和安全文件投影。 |
| `项目Prompt定制与版本恢复` | [`features/项目Prompt定制与版本恢复.md`](features/项目Prompt定制与版本恢复.md) | 覆盖内置 Prompt 并保留可恢复历史。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| 绘本 / 章节 | `chapters.project_id` 归属项目；Chapter 正文生成可读取项目总纲和 Prompt，最近项目卡片按 Chapter 边界选择封面或正文候选。 |
| 对话线程 | 创建 Saga 在草稿项目中恰好一次建立普通 `conversation` Thread、首个 Turn/Run 和原始用户 Item，并把已就绪参考图按清单顺序冻结为 Item Reference；Agent 通过受控 Setup API 推进唯一的 Setup Draft。 |
| 工作流 | `draft` 项目不能创建或执行 Workflow；对话式首次 Turn 定稿后只可启动服务端绑定幂等键的 existing YOLO，后续 ready Turn 才开放普通业务编排。 |
| AI 运行时 | `project_model_settings` 以项目为边界保存模型覆盖。 |
| 文件 | 首页参考图先通过稳定 Upload/File UUIDv7 提交到项目 Asset Store，再由创建绑定和首个 Chat Item Reference 共同保护；绘本或章节导入源文件也通过项目 File 资产保存。 |
| 所有项目域 | URL、JSON、前端状态和实时 payload 都使用项目 UUIDv7。 |
