# 调研：Chat Thread 的 Codex App Server 执行引擎

> 状态：调研 / 未排期。本文记录一种可能方案，不代表已实现行为。Codex app-server 协议仍在演进，文中的方法名以落地时固定的 codex 版本及其导出的 schema 为准。

## 目标

允许用户在项目 Chat 中新建一种 thread，由本机 `codex app-server` 完成推理与工具调度，同时保持 Lumi 现有的数据、权限与实时同步边界不变。

## 核心思路

Codex 只负责推理与工具编排，所有项目读写都经过 Lumi 已有的[本地项目 MCP](local-project-mcp.md)。

本地 MCP 已经提供面向外部客户端的项目 API 边界：`read_agent_doc` / `request_api` / `get_call` / `read_media`、`external_routes` 白名单、幂等键、危险操作确认、projector 字段裁剪和 64 KiB 结果限制，[Codex 插件](../codex-plugins/README.md)就是这样接入的。新模式只是把「用户在 Codex 中手动连接 Lumi」变为「Lumi 托管 Codex 进程并挂到 chat thread」，不新增权限面，Codex 也不触碰 `project.sqlite` 或项目文件系统。

```
ChatArea ──REST/WS──▶ agent.Service ──River chat_turn job──▶ engine 分支
                                                  ├─ lumi  : 现有 ExecuteJob 循环
                                                  └─ codex : CodexEngine
                                                        │ JSON-RPC（stdio JSONL）
                                                        ▼
                                                 codex app-server 子进程
                                                        │ MCP（Bearer = 线程专属 grant）
                                                        ▼
                                                 Lumi /mcp → ExternalProjectAPI
```

## 数据模型

- 不新增 `thread_type`。`conversation` / `workflow` / `mcp` 表达线程语义，Codex 是执行引擎。thread 增加 `engine` 字段（`lumi` 默认 | `codex`），创建时固定，之后不可修改；Thread DTO 公开该字段。
- 新增内部表 `codex_thread_bindings`：`thread_id → codex_thread_id`，以及 codex 版本、MCP grant 内部 ID、配置快照。codex thread id 属于外部系统标识，与内部 `id` 一样不进入 REST、WS 或前端。
- turn、run、item 与 event 继续使用 `chat_turns` / `chat_runs` / `chat_items` 和现有事件表，thread 列表、cursor 分页、trajectory 和 WS 失效提示均可直接复用。

## 执行路径

继续由 River `chat_turn` job 驱动。`ExecuteJob`（`internal/agent/runtime.go`）在 `claimRun` 之后、`resolveProvider` 之前按 `tc.Thread.Engine` 分支，项目初始化锁、FIFO、取消、预算与 `failOrRetryRun` 等前置逻辑保持不变。

`CodexEngine.ExecuteTurn`：

1. 确保全局 app-server 子进程存活（每个 Lumi 进程一个，多个 thread 复用）。
2. 无 binding 时 `thread/start`，否则 `thread/resume`。
3. `turn/start`，输入为用户文本加上 references 转换后的文本（携带公开 UUID）。
4. 消费通知流并翻译为 `chat_items`，经 `appendEventTx` 与 `broadcastThread` 发出 WS 变更提示；payload 只含 UUID，前端照常通过 REST 重读。
5. `turn/completed` 时完成 run，并把 token usage 写入 `llm_logs`（provider 标记为 `codex`）。
6. 用户取消时调用 `turn/interrupt`。

事件映射草案：

| Codex | Lumi `item_type` |
|---|---|
| agentMessage（增量/完成） | `assistant_message` |
| reasoning summary | `reasoning`（折叠展示） |
| mcpToolCall started / completed | `tool_call` / `tool_result` |
| plan / todo | `plan`，或先映射为 message |
| 用户输入请求 | 复用 `codex_questions_v1`（`internal/agent/user_input_protocol.go` 已按 Codex 对齐） |
| 命令执行 / 文件修改审批请求 | 按策略拒绝，记录一条说明 item |

## 权限与沙箱

- **线程专属 grant**：创建 Codex thread 时，Lumi 内部调用 `mcpserver.CreateGrant`，绑定当前项目与「编辑与生成」权限，名称标注来源 thread。发起者就是 Lumi 用户本人，因此不经过 OAuth 授权页。token 通过 app-server config override 注入 `mcp_servers.lumi`：HTTP 加 bearer env，或复用 `lumi_web --mcp` stdio 桥。thread 归档或删除时撤销 grant。
- **禁用 Codex 本地副作用**：sandbox 设为 `read-only`，cwd 指向 Lumi 数据目录下的空 scratch 目录；approval policy 要求 shell 与 apply_patch 审批，Lumi 收到 `commandExecution` / `fileChange` 审批请求时一律拒绝。Codex 唯一的副作用通道是 Lumi MCP。
- **危险操作**：MCP 返回 `pending_confirmation` 时，当前需要到 MCP 设置页处理。Codex thread 应在聊天内展示确认（`internal/mcpserver/activity.go` 已能把调用关联到线程），确认后由下一个 turn 或续跑通过 `get_call` 读取结果。客户端的 `confirmed=true` 仍不授予权限。

## 上下文与提示词

- 历史与压缩由 Codex 自行管理，Lumi 的 `MaxContextBytes` 与上下文组装不作用于该引擎。
- 首个 turn 通过 developer instructions 注入精简的 Lumi agent prompt：先 `read_agent_doc` overview，再操作授权项目，写法与 `codex-plugins` 中的技能一致，并写入项目 UUID。
- 图片生成仍经 MCP generation 路由由 Lumi Provider 执行：文本推理消耗 Codex 账号额度，生图消耗 Lumi 配置的服务商。

## 进程、认证与可用性

- 不打包 codex 二进制，从 PATH 或用户配置路径探测。新增 `GET /api/v1/agent-engines`，返回 `{ items: [{ engine, available, version, auth_status }] }`；前端新建 thread 时据此展示「Lumi Agent / Codex」选择，不可用时置灰并说明原因。
- 认证默认沿用用户 `~/.codex` 登录态；提供 `POST /api/v1/agent-engines/codex/login` 封装 app-server 的账号登录流程，为日后隔离 `CODEX_HOME` 留出口。
- 设定最低支持版本，`initialize` 时校验，不满足时 thread 标记不可用，不勉强运行。

## 恢复语义

- Lumi 重启时，进行中的 Codex turn 标记为 `interrupted`，不自动重放，与 MCP 同步写入「结果不明即 interrupted」的规则一致。下一条用户消息通过 `thread/resume` 接续 Codex rollout。
- 已提交的写入均有 `mcp_calls` 记录与幂等键，可以核查；生成任务按原持久化任务运行时恢复。
- app-server 子进程崩溃时由 supervisor 重启，期间进行中的 run 同样标记 `interrupted`。

## 分期建议

1. **Spike**：Go JSON-RPC 客户端加 schema 契约测试（`codex app-server generate-json-schema`），从命令行驱动单个 turn，跑通 MCP 只读调用。
2. **P1 只读**：`engine` 字段、引擎状态接口、前端选择器；grant 仅「只读」；消息、推理和工具调用落库并实时展示。
3. **P2 编辑**：编辑权限 grant、聊天内危险操作确认、用户输入映射、取消与 interrupt。
4. **P3**：恢复、usage 与成本展示；把 Codex 引擎接入 `evals` 的 shared agent benchmark，与内置 Agent 对比。

## 风险

- **协议稳定性**：app-server 协议仍在变化。固定版本，加契约测试，适配层只放在 `CodexEngine` 一处。
- **成本归属**：用户容易混淆 Codex 账号额度与 Lumi 服务商费用，UI 需要分开标注。
- **能力不对等**：内置 Agent 的 YOLO 初始化、workflow await 等深度集成，Codex 只能通过 MCP 白名单间接使用。第一版 Codex thread 不参与项目初始化工作流。

## 未采用的替代方案

- **`codex exec --json`**：没有双向审批、中断与 resume，不适合多轮聊天。
- **让 Codex 直接操作项目文件或 SQLite**：绕过 Lumi 的业务校验、版本保护与审计，违背本地 MCP 的边界设计。
