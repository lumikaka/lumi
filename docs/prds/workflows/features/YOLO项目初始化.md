# 工作流 — YOLO项目初始化

## overview

该 Feature 使用 `yolo_project_initialization` Workflow 根据用户输入生成并初始化项目 Story 与后续创作资源。它把项目初始化拆成可观测步骤，允许中断、恢复和诊断，而不是把整次生成压缩成不可恢复的单一请求。新建普通绘本 YOLO 默认创建或复用一个 `front_cover`，并为封面和第一张 `body` 生成当前图片；它不自动创建 `back_cover`。条漫 `vertical_strip` 不创建封面，只生成第一个 `body` 图片。

首页直接入口保持原行为；对话式项目创建在 Project Setup 明确确认并定稿后，由同一 bootstrap Run 调用严格受审的同一创建 route，不引入新的 Workflow kind 或执行器。

对话式入口创建成功后使用 `dedicated_thread` 展示，并立即结束 bootstrap Turn。该 Turn 不等待、轮询或 await Workflow 终态，也不逐步模拟 YOLO；失败时报告安全错误并停止。这样 Workflow Worker 是唯一生产者，聊天模型不能循环创建 `vol01.ch02` 到 `vol01.ch06`。

## data_model

YOLO Workflow 使用 `workflows.input_snapshot` 固化用户输入、Prompt 与模型，输入快照 v5 还冻结绘本形式与 `chapter/cover_storyboard` Prompt。`workflow_steps` 保存各阶段的业务 Task 或资源 UUID；封面脚本草案、页面 UUID 和每个 Section 的图片任务 UUID 都作为步骤 checkpoint 持久化。`thread_id` 关联独立 Workflow Thread，Workflow 生命周期不依赖发起它的 bootstrap conversation。

对话式入口的幂等键由服务端根据可信 `project_creation_bootstraps.creation_session_uuid` 生成：`project-creation-yolo:<creation_session_uuid>`。项目内 `(project_id, kind, idempotency_key)` 唯一约束保证模型重复 Tool Call、Tool Result 丢失、恢复重放、应用重启或不同 `story_prompt` 重放都只返回同一个 Workflow 与 dedicated Thread。模型不能覆盖 title、Provider、幂等键或产出数量。

既有步骤固定为 `project_initialization → story → story_profile → premise → comic_sections → first_section_image`。fresh project 只创建或复用 `vol01.ch01`，生成 1～6 个 `body` Comic Sections。输入快照 v5 的 `comic_sections` 步骤会为非 `vertical_strip` 项目生成封面脚本并创建或复用唯一 `front_cover`；若既有封面已有 current storyboard 或 current image，则直接复用。新生成的封面脚本标题必须与 Chapter 标题逐字相同。

`first_section_image` 作为稳定 step key 继续保留：输入快照 v5 中它会为封面和第一张正文页批量创建缺失的图片任务，已有 current image 的目标不重复生成；`vertical_strip` 只处理第一个 `body`。它保留 `section_uuid|image_variant_uuid` 对首个正文页的兼容投影，正文图片任务实际存在时也保留指向该任务的 `task_uuid`；同时返回封面/正文页、装订顺序，以及本次实际创建任务的公开 UUID。v1–v4 输入快照继续按旧语义恢复，只生成旧 `first_section_uuid` 指向页面的图片。Premise Setting Image 继续属于设定步骤，不计入封面或正文页图片约束。

## api

| 接口 | 方法 | 说明 |
|---|---|---|
| `/api/v1/projects/:project_uuid/workflows` | POST | 创建 `yolo_project_initialization` Workflow；对话式 Agent overlay 只接受 `story_prompt` 和可选文本模型，并由服务端补齐项目名、当前 Run 与创建会话幂等键。 |
| `/api/v1/projects/:project_uuid/workflows/:workflow_uuid` | GET | 读取初始化进度、步骤和公开错误。 |
| `/api/v1/projects/:project_uuid/workflows/:workflow_uuid/cancellations` | POST | 取消初始化。 |
| `/api/v1/projects/:project_uuid/workflows/:workflow_uuid/retries` | POST | 使用冻结输入重试。 |

## ui

| 页面 / 入口 | 说明 |
|---|---|
| 新建项目 YOLO 流程 | 以 dedicated Thread 和步骤状态展示初始化，普通绘本完成时显示封面与第一张正文页图片，失败或中断时提供受控重试。 |
| bootstrap 对话 | 创建成功的 Tool Result 返回 `@project/workflows/{workflow_uuid}`；点击后选择对应 dedicated Thread，状态继续由 WebSocket 提示失效并经 REST 重读。 |

## others

Workflow 只编排；总纲、Chapter、Premise 或漫画结果各自按所属 domain 的 revision、版本和幂等约束提交。

项目必须为 `setup_status=ready`，且对话式入口还必须存在当前 Run 已消费的 `confirm_setup_and_start_yolo` 明确确认与成功 finalization 事实。缺少前者返回 `project_setup_incomplete`，缺少后者返回 `bootstrap_yolo_not_authorized`。Workflow 创建后首次 Turn 的其他生产写操作和 `image_gen` 仍由 bootstrap 门禁拒绝；后续普通 Turn 不受此专用限制。
