# AI 运行时 — 分层模型解析与可追溯执行

## 模块职责

AI 运行时模块负责把全局 Provider 默认值、项目级覆盖、场景级覆盖和请求显式选择解析成实际 Provider/模型，在创建任务、Chat 或 Workflow 时冻结解析结果，并为项目内 AI 调用提供可筛选的安全审计记录。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | 模型选项投影、项目/场景覆盖、继承与失效回退、乐观锁更新、任务来源审计、重试冻结，以及调用摘要、用量与诊断读取。 |
| 不负责 | Provider 密钥持久化和连通性实现、模型调用协议、Prompt 内容、各业务任务的输入输出语义。 |

## 核心概念

### 模型场景

项目具有 `project_text`、`project_image` 两个基础场景，以及 `chat_area`、`story_text`、`section_premise_selection` 三个文本子场景。子场景只覆盖自身，否则继承项目文本模型。

### 有效选择

配置值与有效值分开返回。覆盖指向未就绪或已移除的 Provider/模型时保留配置但标记 `invalid`，执行时回退到继承值；只有已就绪且能力类型匹配的选项能保存或执行。

### 冻结来源

每次创建执行资源时保存最终 `provider_uuid`、`model` 和 `model_source`。重试和多步 Workflow 沿用已冻结值，后续修改项目设置不改变历史或正在执行的任务。

### 调用可观测性

Story、Chat、Production 与 Workflow 的文本/图片调用统一投影到项目级日志。日志只保存安全摘要、JSON payload、公开关联 UUID、Provider 诊断和可用 usage；列表筛选不扫描巨大原始 payload，图片调用和 Provider 未返回的指标保持不可用状态。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `项目模型解析与任务冻结` | [`features/项目模型解析与任务冻结.md`](features/项目模型解析与任务冻结.md) | 用统一优先级解析文本/图片模型，并让任务、Chat 和 Workflow 可审计、可重放。 |
| `AI调用可观测性` | [`features/AI调用可观测性.md`](features/AI调用可观测性.md) | 统一查询项目 AI 调用、组合筛选安全摘要，并展示可用的 token、字符和吞吐指标。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| Provider | 提供 ready/active 状态、默认文本模型和默认图片模型；模型设置只保存 Provider UUIDv7 与模型名。 |
| 故事生产 | Story、Premise、漫画分镜/图片和导出前置选择使用对应场景解析。 |
| Chat / YOLO | Chat thread/run 与 Workflow 创建时冻结同一解析结果，并向后续步骤传播。 |
