# AI 运行时 — 分层模型解析、可恢复执行与调用审计

## 模块职责

AI 运行时模块负责把全局 Provider 默认值、项目级覆盖、场景级覆盖和请求显式选择解析成实际 Provider/模型，在创建任务、Chat 或 Workflow 时冻结解析结果；它还提供可恢复任务状态、append-only 事件和项目内 AI 调用的安全审计记录。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | 模型选项投影、项目/场景覆盖、继承与失效回退、冻结来源、任务状态/事件恢复、取消重试、调用摘要、用量与诊断读取。 |
| 不负责 | Provider 密钥持久化和连通性实现、Prompt 内容，以及 Story、Premise、漫画、导出和 Workflow 的业务输入输出语义。 |

## 核心概念

### 模型场景

项目具有 `project_text`、`project_image` 两个基础场景，以及 `chat_area`、`story_text`、`section_premise_selection` 三个文本子场景。子场景只覆盖自身，否则继承项目文本模型。

### 有效选择

配置值与有效值分开返回。覆盖指向未就绪或已移除的 Provider/模型时保留配置但标记 `invalid`，执行时回退到继承值；只有已就绪且能力类型匹配的选项能保存或执行。

### 冻结来源

每次创建执行资源时保存最终 `provider_uuid`、`model` 和 `model_source`。重试和多步 Workflow 沿用已冻结值，后续修改项目设置不改变历史或正在执行的任务。

### 调用可观测性

Story、Chat、Production 与 Workflow 的文本/图片调用统一投影到项目级日志。日志只保存安全摘要、JSON payload、公开关联 UUID、Provider 诊断和可用 usage；列表筛选不扫描巨大原始 payload，图片调用和 Provider 未返回的指标保持不可用状态。

### 可恢复执行

`task_runs` 与 `production_task_runs` 记录可取消、可重试的执行快照，事件表仅追加。客户端用 WebSocket 变更提示失效 TanStack Query，并通过 REST 在首次 join、重连和窗口重新聚焦时校准 SQLite 事实状态。

### Project Chat Prompt 协议

新 Chat Run 使用 `project_api_v3`。System Prompt 只含静态 Agent 规则、API Overview 和当前 `project_uuid`；生成语言等可变事实由 Agent 按需通过 Project API 读取。当前 Turn 的 Reference 快照作为明确标记的不可信 User Message 数据注入，历史 Turn 不重新注入 Reference。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `项目模型解析与任务冻结` | [`features/项目模型解析与任务冻结.md`](features/项目模型解析与任务冻结.md) | 用统一优先级解析文本/图片模型，并让任务、Chat 和 Workflow 可审计、可重放。 |
| `可恢复AI任务执行` | [`features/可恢复AI任务执行.md`](features/可恢复AI任务执行.md) | 以冻结输入、状态机和 append-only 事件支撑跨域任务的恢复、取消与重试。 |
| `AI调用可观测性` | [`features/AI调用可观测性.md`](features/AI调用可观测性.md) | 统一查询项目 AI 调用、组合筛选安全摘要，并展示可用的 token、字符和吞吐指标。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| AI Provider | 提供 ready/active 状态和默认模型；项目库只保存 Provider UUIDv7 与模型名。 |
| 章节、设定资产、漫画 Section、导出 | 业务域创建任务并定义输入输出语义，运行时只管理冻结和执行状态。 |
| Chat thread / Workflow | Chat thread/run 与 Workflow 创建时冻结模型和 Prompt 协议；已有 v2/legacy Run 只按持久快照恢复。 |
