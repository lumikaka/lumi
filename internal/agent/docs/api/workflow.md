# Workflow / YOLO API

本 Contract 中的创建入口仅用于首页创建会话的首个 bootstrap Turn。普通 ready 对话不能借用该入口；普通用户入口仍由现有 UI 发起。

- `POST /api/v1/projects/{project_uuid}/workflows`
  - 调用前必须已读取初始化 Guide 和本 Contract。
  - 仅在同一 bootstrap Run 已明确消费 `confirm_setup_and_start_yolo`，且绑定的 Project Setup finalization 已成功、项目事实为 `ready` 时可用。
  - `request_body.story_prompt` 必填，最多 4000 字符；它可以整理 `original_input`、后续回答和已经展示的 Agent 建议，但不得覆盖原始输入。
  - `request_body.model` 可选。不得提交 `title`、`provider_uuid`、`idempotency_key`、Chapter 数、Section 数或其他字段；这些值由服务端固定或解析。
  - 推荐响应过滤器为 `.data | {uuid,thread_uuid,presentation_mode,kind,title,status,current_step_key,steps}`。
  - 成功响应是 `yolo_project_initialization` 的紧凑公开摘要，并提供 `@project/workflows/{workflow_uuid}` 形式的 `ui_ref`。Workflow 使用 dedicated Thread。

服务端按可信 `creation_session_uuid` 绑定幂等键。相同 bootstrap Turn 的重复 Tool Call、恢复或应用重启只返回同一个 Workflow，不创建第二个 Workflow 或 Thread。

YOLO 固定完成项目初始化、故事、Story Profile、Premise、正文页规划和初始页面图片，只创建或复用 `vol01.ch01`。它生成 1～6 个 `body` Comic Sections：普通绘本还会创建一个 `front_cover`，并默认为封面和第一个正文页生成成品图；`vertical_strip` 不创建封面，只为第一个画面段落生成成品图。Premise 的 Setting Image 仍是既有步骤的一部分。

创建成功即代表“已排队”，不是全部创作已经完成。回复中使用 Tool Result 的 `ui_ref` 后立即结束当前 Turn；不得轮询状态、等待终态或手工模拟任何步骤。失败时报告公开错误并停止，后续恢复使用既有 Workflow retry，不要再次生产一个新 Workflow。
