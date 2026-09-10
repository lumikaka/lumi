# 项目 — 本地项目级 MCP 接入

## overview

本机外部 AI 客户端通过 MCP 安全读取、编辑一个明确授权的已打开 Lumi 项目。Lumi 是 MCP Server；授权绑定项目和只读/编辑权限，支持本机 HTTP OAuth，不引入多用户、云端账号或公网入口，不包含内置 Agent 的 MCP Client 功能。

stdio 桥接与后端复用现有业务运行时；后端退出或项目关闭时给出明确指引。允许读取项目资料、章节/绘本、设定、页面/画面段落、脚本、任务和授权媒体，允许清单内编辑、生成及经用户确认的危险操作。共享契约中的其他内部路由不会自动暴露。

## data_model

应用库包含 `mcp_grants`、`mcp_calls` 和本机 OAuth 客户端/授权请求/刷新凭据表，结构见 [domain 数据模型](../data_model.md)。内部均使用自增 64-bit ID 和外键；URL、JSON、实时消息只使用 UUIDv7。

凭据为 256-bit 随机值，数据库只保存 SHA-256 摘要与展示前缀。每请求重新查验撤销状态；凭据不保存到项目文件夹。

调用记录授权、项目关联、UUID、原始 JSON、指纹、幂等键、状态、有效期及结果。写入同键同参数复用调用，不同参数失败。危险请求保存后进入 `pending_confirmation`，确认执行原始请求并复核版本，拒绝或过期进入终态；客户端无法自授确认。

普通写入在应用库记录与项目库提交之间发生进程崩溃时标为 `interrupted`，禁止自动重放；生成凭现有任务幂等键恢复受理结果。

## api

| 方法与资源 | 说明 |
|---|---|
| `GET /api/v1/projects/:project_uuid/mcp-grants` | `{items}`，列出项目本机授权，不返回摘要或明文。 |
| `POST /api/v1/projects/:project_uuid/mcp-grants` | `name`、`permission: read|edit`；返回 grant、一次性 token 和可复制配置。 |
| `DELETE /api/v1/projects/:project_uuid/mcp-grants/:grant_uuid` | 撤销；成功 `data: null`。 |
| `GET /api/v1/projects/:project_uuid/mcp-calls` | `{items}`，最多 100 条，待确认优先，再按新旧排序；含原始请求与结果。 |
| `POST /api/v1/projects/:project_uuid/mcp-calls/:call_uuid/decisions` | `decision: approve|reject`、`fingerprint`；不接受替换参数。 |

管理 REST 复用桌面认证、可信 Origin、项目请求租约及统一 JSON 信封；与 MCP 共用端口，但不接受 MCP Bearer 凭据代替桌面会话。

本机 `/mcp` 复用业务 loopback listener，使用独立 Bearer 授权，支持直接 HTTP OAuth 及携带实例 nonce 的 stdio 桥接。开发默认 5801；桌面优先 32323，被占用时回退，MCP 设置页返回实际地址。HTTP 地址变化需更新配置并重新授权；stdio 每请求重新发现。协议 JSON-RPC/MCP 字段为普通 REST 信封和命名的协议级例外。工具内业务继续使用 `success/data/error` 和 `snake_case`，JSON-RPC `id` 不属于数据库内部 ID。

工具为 `request_api`、`read_agent_doc`、`get_call`、`read_media`。项目边界来自授权；拒绝跨项目 URL、任意 URL、编码/穿越路径和伪造内部上下文。读取经共享 projector/response_filter 限制大小并清理内部 ID。媒体使用文件 UUID 和受控文件服务，最多 4 MiB 图片，不假定客户端持有桌面 Cookie。

| 本机 OAuth / 管理资源 | 说明 |
|---|---|
| `GET /.well-known/oauth-protected-resource[/mcp]` | MCP 资源与授权服务器发现。 |
| `GET /.well-known/oauth-authorization-server` | PKCE、注册、token 端点元数据。 |
| `POST /oauth/register` | 本机原生公共客户端登记，回调限 HTTP loopback。 |
| `GET /oauth/authorize` | 持久化授权请求，跳转 Lumi 授权页，不直接授予权限。 |
| `POST /oauth/token` | 授权码兑换和轮换刷新；绑定客户端、resource 和授权项目。 |
| `GET /api/v1/mcp` | 返回运行实例的可复制 HTTP 地址。 |
| `GET /api/v1/mcp-authorization-requests/:uuid` | 受桌面认证保护的待授权详情。 |
| `POST /api/v1/mcp-authorization-requests/:uuid/decisions` | `decision`、`project_uuid`、`permission`；返回已登记回调地址。 |

OAuth 授权请求/码有效期 5 分钟，访问令牌 1 小时，用户同意 offline_access 时刷新链 7 天。撤销阻止访问及续期；刷新重放撤销整项授权。每次刷新保留 Grant UUID 及其调用历史。无远程客户端元数据抓取、云端回调或成员体系。

## ui

全局左侧栏底部「设置 → MCP 设置」进入独立页面 `/settings/mcp`，不在项目概览展示授权卡片。页面首先显示可复制的本机 HTTP OAuth 地址和连接步骤；外部客户端发起连接后，在 `/settings/mcp/authorize` 查看自报客户端名称及回调目标，并选择一个项目和只读/编辑权限。批准后返回客户端，拒绝和过期可恢复重连。管理页面可选择最近或已打开项目，通过现有 Project Manager 打开所选项目后复用项目授权管理组件。入口不依赖当前项目或服务商配置。用户可创建只读/编辑授权、一次性复制客户端配置、查看并撤销授权；待确认请求展示客户端、动作、原始参数、指纹和状态，允许明确确认或拒绝。已完成操作可查看结果。

WebSocket `mcp:changed` 仅发送项目 UUID，触发相关 TanStack Query 失效；业务变更沿用原有业务事件。首次 join、重新 join 和聚焦均通过 REST 校准，没有定时 HTTP 轮询。

## commands

`codex-plugins/lumi-prod-plugin` 与 `codex-plugins/lumi-dev-plugin` 包含 manifest、HTTP MCP 配置、技能及流程参考，支持通过现有 Codex 市场分发。插件仅配置 HTTP URL，客户端发现 OAuth 元数据后动态注册实际回调；仍要求 PKCE、resource、精确登记回调及用户项目/权限选择。重复 resource 仅在每个值都等于当前 MCP endpoint 时接受。插件从 `read_agent_doc` 的授权 UUID 和 `external_routes` 开始操作，复用现有 Lumi 路由，不另建 VACS 风格业务接口或复制完整业务文档。

`lumi_web --mcp --environment production|development [--data-dir ABSOLUTE_PATH]`，凭据来自 `LUMI_MCP_TOKEN`。桥接只做发现和协议转发，stdout 仅输出 JSON-RPC，stderr 用于诊断。发现文件按环境隔离，每条消息重读，因此端口变化不修改配置；移动安装目录时仍须更新命令路径。

## jobs

章节正文、来源设定图、设定拆解和页面图片任务复用现有 `StartDomainTask`/River 持久化运行时与 Lumi 模型设置。外部来源使用独立调用 UUID、`external_mcp` 和 `PresentationNone`，只创建独立的只读 MCP 展示 Thread，不创建聊天 Turn/Run。任务自身现有审计保留。

调用受理后，即使 MCP 客户端断开，任务也继续由后台运行时管理；关闭项目后按原队列规则恢复。外部请求租约通过 Project Manager 获取，不允许客户端以任意路径打开项目。

## others

- [使用与配置](../../../local-project-mcp.md)
- [架构与规范来源](../../../architecture/local-project-mcp.md)
- 自动测试覆盖真实 MCP SDK、业务编辑、确认状态、幂等恢复、媒体边界、发现替换和 WebSocket/REST 同步。
- Rust/native 仅安排 CI；完整桌面包验证由现有 Desktop workflows 执行，不在本地运行 Cargo。

## ChatArea 操作历史

工具调用按「项目 + 授权来源」聚合为 ChatArea 原列表中的只读 MCP thread。连续 30 分钟无调用后下次新建；后台任务运行、结果返回、确认及界面查看都不续期。OAuth token 刷新保留授权归属。

项目库保存 session 与轻量受理事实，应用库 `mcp_calls` 继续负责原执行、幂等、确认和结果。概括按受理顺序，而非返回顺序；仅连续已成功的读取和同资源同类别修改合并。未知结果与等待确认阻断合并，失败在原位置独立保留。幂等重放作为读取原结果概括，不重复业务效果。

详情 REST 为 `GET /api/v1/projects/:project_uuid/chat_threads/:thread_uuid/mcp_activity`，支持 `before/after/limit`，返回标准列表及 cursor 分页信封。历史 revision 变化时旧 cursor 返回 `mcp_activity_changed`，前端重读全部已加载页。来源仅为名称快照，不代表已验证客户端身份。

只显示模板概括、等待确认和调用结果，无完整参数、响应、聊天输入框或 thread 运行状态；生成只记录已提交，不追踪后台任务进度。`mcp:changed` 同时刷新 ChatArea 列表及概括历史；后台任务事件不刷新该历史。
