# 对话线程 — 可恢复项目协作对话

## 模块职责

对话线程模块负责项目内 Agent 对话的持久化会话、轮次、运行、消息、追问队列、用户输入、Reference 和工具执行。普通 `conversation` Thread 本身不绑定业务场景或单一资源；每条用户输入用最多 16 个冻结 Reference 显式声明本次上下文，并可在进程中断、异步 Workflow 等待或实时事件丢失后从 SQLite 恢复事实状态。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | Thread、turn、run、item、event、follow-up、用户输入、统一 Reference、受控工具意图和图片生成交互。 |
| 不负责 | Provider 配置、模型解析、通用 Workflow 编排、业务资源的字段语义和文件物理存储。 |

## 核心概念

### Thread 与 Turn

Thread 只固定项目、标题、会话级模型选择和运行状态。每次用户输入形成按序排队的 Turn，Reference 归属于对应 User Item；执行产生 Run、可读 Item 和 append-only Event，Thread 的展示状态是这些持久记录的投影。

Thread 以 `thread_type=conversation|workflow` 区分普通对话与公开 UI 创建的独立 Workflow 页面。Chat Tool 发起的 Workflow 仍关联当前 `conversation` Thread，并通过 origin Turn 内联显示，不覆盖会话标题。

### Reference

Reference 是用户对当前输入所需项目资源的显式引用，首版支持 `file`、`premise_asset` 和 `comic_section`。服务在接受输入时校验项目边界并冻结紧凑快照；后续 Turn 不自动继承，历史资源发生删除或修改也不重写已有快照。

### 受控交互

Agent 的工具调用、用户选择题和图片引用均先写入持久记录再执行。对外投影递归清理内部 ID、路径、密钥和凭据，业务修改仍由对应领域服务校验。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `持久化项目对话` | [`features/持久化项目对话.md`](features/持久化项目对话.md) | 创建、阅读、恢复线程、消息、运行和事件历史。 |
| `追问队列与运行控制` | [`features/追问队列与运行控制.md`](features/追问队列与运行控制.md) | 排队追问、立即引导、取消和运行窗口控制。 |
| `对话工具与多模态交互` | [`features/对话工具与多模态交互.md`](features/对话工具与多模态交互.md) | 用户输入、工具执行和受控图片引用。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| AI 运行时 | 创建 Thread/Run 时冻结模型；调用明细统一进入 `llm_logs`。 |
| 工作流 | `direct_ui` Workflow 使用独立 `workflow` Thread；`chat_tool` Workflow 复用当前 `conversation` Thread，并以持久 await 暂停/恢复父 Run。 |
| 文件 | 普通上传图片作为 `file` Reference；实际上传、内容服务和冻结图片引用保护由 `files` 管理。 |
| Premise 资产 / 漫画 Section | 作为用户输入 Reference 提供紧凑上下文；修改仍通过受控项目 API 并由各领域服务校验。 |
