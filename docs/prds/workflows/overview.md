# 工作流 — 可恢复多步业务编排

## 模块职责

工作流模块负责跨步骤业务任务的冻结输入、进度、步骤状态、事件、取消与重试。Workflow 可以可选关联 Chat Thread，但不是 Chat 的子资源：它还用于 Story、漫画分镜、漫画图片和 YOLO 项目初始化。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | Workflow、Step、Event、输入快照、幂等键、状态机、诊断、取消和重试。 |
| 不负责 | 每个步骤的 Story、Premise、漫画或导出业务结果；这些由调用业务域解释和提交。 |

## 核心概念

### 冻结编排

Workflow 创建时冻结 Provider、模型和 `input_snapshot`。后续步骤使用同一冻结选择；重试读取已冻结输入而非当前项目设置或当前业务资源。

### 独立恢复

Step 和 Event 分别按 position / sequence 持久化。客户端读取 Workflow、runs、events 和关联 LLM logs 恢复诊断，WebSocket 只触发失效。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `可恢复多步工作流` | [`features/可恢复多步工作流.md`](features/可恢复多步工作流.md) | 管理通用 Workflow 的步骤、事件和恢复。 |
| `YOLO项目初始化` | [`features/YOLO项目初始化.md`](features/YOLO项目初始化.md) | 以多步 Workflow 初始化项目 Story 与创作资源。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| Chat Thread | Workflow 可选关联 Thread；对话只是一个入口，不拥有 Workflow 状态。 |
| AI 运行时 | 解析并冻结模型，统一记录 LLM 调用。 |
| 章节、Premise、漫画 Section | Workflow step 调用其领域服务并生成业务结果。 |
