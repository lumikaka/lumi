# Lumi Agent 能力与 Project API 索引

本索引列出 Agent Guide Registry 中的能力流程和领域 API Contract 文档入口。具体 method、path、字段与响应只保存在对应领域文档中，避免在每次模型请求的 system prompt 中重复完整 Route 表。所有有效 Scene 都获得相同的四个工具；Scene 只提供运行时身份、Subject 上下文、安全边界、图片参考策略和推荐 Guide，不授予或收回工具或文档权限。

## 调用规则

- 业务调用使用 `request_api`；Guide 和 API Contract 读取使用 `read_agent_doc`。
- 先识别所需能力；流程不确定时读取对应 Guide，调用 `request_api` 前读取目标领域 API Contract，以其中的 method、path、字段和响应为准。
- `request_api` 匹配服务端实际注册在当前项目范围下的 method + path；调用在进程内完成，不访问 localhost 或外部网络。
- URL 必须是 `/api/v1/projects/{current_project_uuid}/...` 形式的规范相对路径。
- 外部资源只使用 UUIDv7，JSON 使用 snake_case，不得传递或接收数据库内部 `id`。
- API Contract 要求乐观并发时，写操作先读取最新资源和 revision，再提交 `expected_revision`。
- 经过审查的 POST 和异步生成任务使用 Tool Execution UUID 保证幂等；通用路由遵循对应 REST API 自身的幂等契约。
- 没有显式风险覆盖层的写路由默认按危险操作处理，必须经过请求指纹绑定的用户确认。
- 每次调用必须提供 `response_filter`；它只能从 `.data` 开始，可使用有限字段路径、数组和对象投影。
- `response_filter` 应只选择当前步骤需要的最少字段；列表默认排除正文、图片详情等大字段，只有确实需要完整紧凑响应时才使用 `.data`。
- `request_user_input` 是 Runtime 控制工具，必须作为一次模型响应中唯一的 Tool Call。

## 能力索引

{{capability_index}}

Guide 负责标准步骤、禁止捷径、前置条件与失败恢复；API Contract 负责 method、path、字段和响应结构。二者职责不同，需要时分别读取。

## API Contract 索引

{{api_doc_index}}

## 文档层次

- 本文档：能力与领域文档索引，回答“应该读取哪份文档”。
- `/api/v1/agent-docs/guides/*.md`：可复用能力流程，回答“如何安全完成任务”。
- 顶层领域 Markdown：按当前服务端项目 API 渲染的具体 API Contract，回答“method、path、字段与响应是什么”。

`read_agent_doc` 只能读取注册表中的 Overview、Guide 和 API Contract；任意文件路径、Scene 文档、Query、Fragment 与路径穿越都不可读。
