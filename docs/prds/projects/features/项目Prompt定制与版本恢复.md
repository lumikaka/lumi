# 项目 — 项目Prompt定制与版本恢复

## overview

该 Feature 为内置 Prompt Catalog 保存项目级覆盖，而不修改代码内默认 Prompt。覆盖按 group 和 key 版本化，支持创建、批量更新、恢复历史版本和恢复默认；Story、Premise、漫画与 Agent 调用在任务创建时冻结最终 Prompt，不在重试时重新读取当前覆盖。

Agent 活动 Catalog 只包含通用 `base` 与 `conversation_summary`。新 `project_api_v4` System Prompt 由静态规则、运行时生成的 API Overview 和唯一动态项目事实 `project_uuid` 组成，不注入生成语言、业务 Scene、subject 或 Reference 内容；当前 Turn Reference 快照和同一 Thread 最近可用的历史图片 Reference 紧凑清单只作为明确标记的不可信 User Message 数据注入。v4 Base Prompt 同时冻结 Codex 风格的多问题、推荐项、客户端 Other、跨 Turn 冻结 Reference 选择和危险确认规则；旧默认值仍在 `PreviousDefaultValues` 中按版本迁移，自定义 Prompt 不被覆盖。

## data_model

`project_prompt_versions` 以 `(project_id, prompt_group, prompt_key, version_no)` 唯一保存不可变版本。每条记录含公开 UUIDv7、内容 hash、来源类型及可选 `restored_from_version_id` 内部关联；当前值由 Catalog 默认值与最新项目版本投影得出。

Chapter Prompt group 包含 `cover_storyboard`、`cover_before_image` 与 `back_cover_before_image`：普通绘本 YOLO 冻结前者生成封面脚本，Section 图片任务依冻结的 `page_role` 在 `cover_before_image|before_image|back_cover_before_image` 中选择角色规则。`vertical_strip` 不使用特殊页 Prompt。

Scene-era Agent Prompt key 已退出活动 Catalog。既有 `project_prompt_versions` 行不删除、不改写，只作为历史记录保留；已有 v2 或 legacy typed Run 从 User Item 中冻结的 Prompt snapshot 恢复，新的 Turn 不得选择旧协议或 Scene Prompt。

## api

| 接口 | 方法 | 说明 |
|---|---|---|
| `/api/v1/projects/:project_uuid/prompts` | GET | 返回默认值、有效值、占位符和当前版本。 |
| `/api/v1/projects/:project_uuid/prompt-groups/:prompt_group` | PATCH | 原子更新一个 Prompt group 的多个 key。 |
| `/api/v1/projects/:project_uuid/prompt-versions` | GET / POST | 查询历史或创建单个覆盖版本。 |
| `/api/v1/projects/:project_uuid/prompt-versions/:version_uuid/restorations` | POST | 基于历史版本创建新的恢复版本。 |

## ui

| 页面 / 入口 | 说明 |
|---|---|
| `/projects/:project_uuid/overview/prompts` | 按 group 编辑有效 Prompt、查看版本、恢复历史或默认值。 |

## others

默认 Prompt、group、key 和占位符定义位于 `internal/promptcatalog`。保存必须校验当前活动 group/key 和模板占位符；退出 Catalog 的 Scene 版本只读保留。Conversation Summary 作为不可信派生上下文以 User 角色注入，本地运行错误仅作为诊断上下文，不提升到 System 优先级。
