# 工作流 — 可恢复多步业务编排

## 模块职责

工作流模块负责跨步骤业务任务的冻结输入、进度、步骤状态、事件、取消与重试。Workflow 可以可选关联 Chat Thread，但不是 Chat 的子资源：它还用于 Story、漫画分镜、漫画图片和 YOLO 项目初始化。普通 Chat Tool 发起的异步 Workflow 复用当前会话并以持久 await 暂停父 Run；公开 UI 与受控 bootstrap YOLO 使用独立 Workflow Thread，后者创建成功后立即结束发起 Turn。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | Workflow、Step、Event、输入快照、幂等键、状态机、诊断、取消和重试；`direct_ui|chat_tool|workflow_step` 调用上下文、`dedicated_thread|inline|none` 展示方式，以及 Workflow 终态到父 Chat Run 的可靠唤醒。 |
| 不负责 | 每个步骤的 Story、Premise、漫画或导出业务结果；这些由调用业务域解释和提交。 |

## 核心概念

### 冻结编排

Workflow 创建时冻结 Provider、模型和 `input_snapshot`。后续步骤使用同一冻结选择；重试读取已冻结输入而非当前项目设置或当前业务资源。

### 独立恢复

Step 和 Event 分别按 position / sequence 持久化。客户端读取 Workflow、runs、events 和关联 LLM logs 恢复诊断，WebSocket 只触发失效。

### Chat 依赖恢复

普通 `chat_tool` 调用不创建影子 Thread。Workflow 与当前 Thread、Turn、Run、Tool Execution 的内部关联写入 `workflow_awaits`；父 Run 释放 worker，Workflow 终态再以唯一 `JobChatResume` 恢复。应用重启可从 Workflow、await 和 Run 的持久状态修复未完成投递。对话式 bootstrap YOLO 是专用例外：它创建 dedicated Thread、不写 await，原 Turn 在拿到 Workflow 引用后完成。

### 项目就绪门禁

Workflow 创建和 Worker 执行都重新读取项目事实。`setup_status=draft` 时返回 `project_setup_incomplete`，即使任务在状态变化前已排队也不能执行；定稿为 `ready` 后沿用现有冻结与恢复语义。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `可恢复多步工作流` | [`features/可恢复多步工作流.md`](features/可恢复多步工作流.md) | 管理通用 Workflow 的步骤、事件和恢复。 |
| `YOLO项目初始化` | [`features/YOLO项目初始化.md`](features/YOLO项目初始化.md) | 以多步 Workflow 初始化项目 Story、设定、正文页，并为普通绘本生成封面与第一张正文页图片。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| Chat Thread | `direct_ui` 与 bootstrap YOLO 使用独立 `workflow` Thread；普通 `chat_tool` 复用 `conversation` Thread 并内联展示；父对话不拥有 Workflow 状态。 |
| 项目 | `draft` 项目禁止创建或执行业务 Workflow；Project Setup 原子定稿为 `ready` 后才开放。 |
| AI 运行时 | 解析并冻结模型，统一记录 LLM 调用。 |
| 章节、Premise、漫画 Section | Workflow step 调用其领域服务并生成业务结果。 |
