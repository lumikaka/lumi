# 章节 — 章节AI生成与批量规划

## overview

本 Feature 的技术聚合根为 Chapter。产品称呼由项目形式决定：普通绘本形式称“绘本”，条漫 `vertical_strip` 称“章节”；生成任务和批量规划的 UI 文案必须使用对应称呼。

该 Feature 使用冻结的项目总纲、Chapter 正文、Prompt 和模型创建可恢复的 Story 任务。支持单 Chapter 生成、批量 Chapter 规划和由规划结果创建 Chapter；任务完成只通过 Story 服务追加正文版本，绝不覆盖用户在任务期间产生的新正文。

## data_model

执行记录由 `ai_runtime` 的 `task_runs` / `task_events` 管理；本域拥有 `story_generation_results` 与 `story_prompt_results` 的业务结果含义。结果通过内部 task、Chapter 与正文版本外键关联，对外只投影 Task、Chapter 和 Story UUIDv7。

## api

| 接口 | 方法 | 说明 |
|---|---|---|
| `/api/v1/projects/:project_uuid/chapters/:chapter_uuid/generations` | POST | 创建单 Chapter Story 生成任务。 |
| `/api/v1/projects/:project_uuid/chapter-batches` | POST | 创建批量 Chapter 规划任务。 |
| `/api/v1/projects/:project_uuid/tasks` | GET | 查询项目 Story 任务。 |
| `/api/v1/projects/:project_uuid/tasks/:task_uuid` | GET | 读取单任务状态和公开快照。 |
| `/api/v1/projects/:project_uuid/tasks/:task_uuid/events` | GET | cursor 读取 append-only 任务事件。 |
| `/api/v1/projects/:project_uuid/tasks/:task_uuid/cancellations` | POST | 请求取消。 |
| `/api/v1/projects/:project_uuid/tasks/:task_uuid/retries` | POST | 基于冻结输入重试。 |

## jobs

| Job / Worker | 触发条件 | 策略 |
|---|---|---|
| `story_chapter_generation` | 用户请求生成 Chapter | 冻结 current Story、Prompt、模型和参数；提交前复核 Chapter revision。 |
| `story_chapter_batch_plan` | 用户请求批量规划 | 验证生成的编号和数量，结果写入幂等 Prompt result。 |

## others

任务、事件和 WebSocket 只用于恢复和查询失效。首次 join、重新 join 与窗口重新聚焦后必须由 REST 重读事实状态。
