# 本地项目 MCP 架构

## 边界和传输

`lumi_web --mcp` 是同一分发二进制的早期命令分支，只进入 `internal/mcpbridge`。它读取环境隔离的发现文件，将 stdio JSON-RPC 转发给正在运行的后端，不读取项目 SQLite，也不启动 Agent、River 或第二个服务器。

应用只启动一个业务 loopback 监听器（开发默认 `127.0.0.1:5801`，桌面优先 `127.0.0.1:32323`，被占用时沿用桌面回退逻辑），MCP/OAuth 挂载同一服务，并原子写入应用目录的 `mcp-development.json` 或 `mcp-production.json`。文件只有 endpoint、环境和本次随机 nonce，权限来自 0600 临时文件。桥接禁止代理、重定向和非 127.0.0.1 的发现目标，每次请求重读；桥接提交的 nonce 必须匹配，避免旧端口被另一实例复用。退出仅清理仍属于自身 nonce 的发现文件。客户端配置只依赖稳定的二进制位置、环境和数据目录。

该 listener 承载业务 REST/WS/媒体、本机 Streamable HTTP OAuth 和 stdio 转发。`/mcp` 需要独立 Bearer 凭据；`X-Lumi-Instance` 由桥接提供，直接 HTTP 客户端无需提供。精确校验 Host，拒绝浏览器 Origin；只有 OAuth authorize 的浏览器 GET 导航例外。MCP/OAuth 的精确路径在 Echo pre-middleware 中分流，使用独立 handler，避开桌面 Cookie、普通 REST Origin/CORS、业务信封与请求日志中间件。普通 REST（包括 `/api/v1/mcp`、授权和确认管理）仍走原有桌面认证与 Origin 中间件；Bearer 凭据不能访问这些管理接口，桌面 Cookie 也不能替代 MCP 凭据。

## 本机 OAuth

`internal/mcpserver/oauth_http.go` 提供受保护资源元数据（根和 `/mcp` 后缀）、授权服务器元数据、原生客户端动态注册、授权导航和 token 端点。401 带 `WWW-Authenticate` 资源元数据地址。注册只允许公共客户端（`none`）及 loopback HTTP 回调，按 RFC 8252 允许回调端口变化，其余 URI 精确匹配；不抓取远程客户端元数据。

`codex-plugins/` 的 `.mcp.json` 仅声明 HTTP URL，Codex 根据资源/授权服务器元数据执行 DCR，登记实际 `/callback/<server-specific-id>` 回调。回调继续精确匹配登记路径，只放宽 loopback 端口。`internal/mcpserver/plugin_clients.go` 保留两个历史固定客户端的幂等登记，用于已有连接兼容；新插件不再引用这些 client ID，也不使用固定 callback。客户端名称不代表已认证的进程身份。

授权与 Token 请求允许重复的 `resource`，但每个值必须等于当前 MCP endpoint；混合资源、空资源及其他重复参数均拒绝，防止参数解析产生歧义。

客户端请求包含 resource、PKCE S256 challenge、回调和 state。服务保存不可变授权请求，303 跳转当前 Lumi 业务前端 `/settings/mcp/authorize?request_uuid=...`。Lumi 页面通过受桌面会话/可信 Origin 保护的普通 REST 查看请求、选择项目与 read/edit、批准或拒绝。客户端不能通过 MCP/OAuth 公共端点批准请求。项目打开只来自用户明确的 UI 操作，仍通过既有 Project Manager UUID 接口。

批准后只向当前 UI 返回携带一次性 code、state 和 iss 的已登记回调 URL，拒绝返回 access_denied。应用库仅保存 code/token 摘要。授权请求和 code 总有效期 5 分钟，兑换时校验客户端、回调、resource、PKCE 和未使用状态，并在同一个应用库事务内消耗 code、创建原有 Grant。项目绑定与权限来自持久化授权决定，忽略不了也替换不了。OAuth 标准响应/元数据字段也是协议级例外，管理 REST 继续使用统一信封和 snake_case。

访问令牌有效期 1 小时；`offline_access` 控制是否发放刷新令牌，刷新链绝对有效期 7 天。每次刷新保留 Grant UUID、项目、权限和调用历史，轮换访问令牌和刷新令牌；旧访问令牌立即失效。已用刷新令牌再次出现时提交整项授权撤销。授权码请求/兑换必须显式 resource；刷新时可以省略，服务从唯一绑定的授权验证当前资源，显式不匹配始终拒绝。这兼容官方 Go SDK 默认 TokenSource 的刷新请求。

注册最多 1024 个客户端，未过期授权请求最多 2048 个；过期请求在创建新请求时清理，过期刷新摘要在兑换时清理。JSON/form 大小有界，不打印明文。HTTP 授权状态持久化后能跨后端重启恢复；桌面前端地址由启动配置提供，新授权导航指向当前前端。MCP resource/issuer 使用实际业务监听地址；回退或自定义业务端口会改变它们，HTTP 客户端需更新地址并重新授权。stdio 通过发现文件继续透明转发。旧 `LUMI_MCP_ADDRESS` 配置已移除。


协议核心为官方 Go SDK 1.7.0 `mcp.Server`，可独立于 stdio 桥接测试。支持当前 `2026-07-28` 的 `server/discover` 与每请求协议元数据，也兼容旧版本 `initialize`、版本协商、`notifications/initialized`、工具发现/调用、ping 与 JSON-RPC 错误。桥接把标准协议元数据映射到 HTTP header，业务参数不会被当作内部调用上下文。工具清单固定，不宣告变更订阅或 MCP task 扩展；Lumi 的任务和确认通过业务调用资源恢复。

MCP JSON-RPC 和 MCP 协议字段（例如 `protocolVersion`、`inputSchema`、`structuredContent`、`isError`）是普通 REST 信封与 `snake_case` 的协议级例外。JSON-RPC `id` 只是请求关联值，不是数据库主键。工具内部业务信封和管理 REST 保持 Lumi 标准。

## 授权、请求与共享执行

应用库 `mcp_grants` 以 SHA-256 存储 256-bit 随机凭据摘要，关联 `recent_projects.id`。`mcp_calls` 关联 grant 内部 ID。所有主键使用 SQLite 自增 64-bit `id`，对外 UUIDv7。授权不随项目文件夹复制，删除最近项目记录会级联删除该本机授权和调用记录。

每个 HTTP 请求重新验证摘要、有效期、OAuth resource 与撤销状态，工具受理和 UI 确认再核验原授权。`read` 只允许显式只读路由，`edit` 允许普通编辑和生成；危险操作另外确认。授权管理复用桌面会话与可信 Origin 中间件，MCP 不暴露任何管理路由。

`internal/agent/external_project_api.go` 是显式外部 Route 清单。共享 parser 不再要求 Chat Thread/Turn/Run；沿用路径、UUID、字段、跨字段、响应投影校验。外部路径必须属于授权项目，拒绝完整 URL、路径穿越、编码路径、query 注入与内部调用字段。未列入清单的新路由默认不可访问。

普通操作经现有进程内 Echo dispatcher 进入 REST handlers 和业务服务，保留事务、版本检查和事件。少数共享 Agent 契约把软删除 revision 放在 body，桥接到现有 REST query 时做明确适配；Comic Section DELETE 的 REST null 也按共享 Agent projector 适配，不复制业务实现。生成通过现有 `StartDomainTask`，传递 `external_mcp`、独立调用 UUID 和 `PresentationNone`。

文件读取经 `Project Manager.WithStore` 和 `files.OpenContent`，只按项目文件 UUID 解析。MCP `ImageContent` 携带受限媒体数据，不发放免鉴权 URL。

## 持久化、确认和崩溃语义

每个项目 API 调用保存原始参数、授权关联、幂等键、SHA-256 指纹、状态、结果/错误和时间。写入幂等键在 grant 内唯一；同键不同参数失败。操作受理与撤销/确认在应用运行时串行裁决；危险请求从 pending 通过数据库条件更新进入执行，重复决定不会再次执行。

普通编辑断线后继续在最多两分钟的独立上下文与 Project Manager 请求租约中执行。已提交后台任务的生命周期归现有持久化运行时。项目关闭等待请求租约，空闲回收仍检查任务活动。MCP 不持有永久 presence，不通过磁盘路径打开项目。

危险操作在 UI 显示原文与指纹；只执行已保存原始操作，重新校验当前资源 revision。过期、拒绝、撤销及指纹不符均不会执行。状态可以在重新连接后通过 `get_call` 读取。时间保存与数据库比较统一使用 UTC；过期状态在查询/决定时校准，不引入 HTTP 轮询。

应用库与项目库是独立 SQLite，不能把二者提交伪称一个原子事务。普通写入若崩溃于业务提交和结果记录之间，启动后标记 `interrupted`，保守保留结果不明状态。生成受理可凭 `mcp:<call_uuid>` 找回已提交任务；如果原任务不存在才进入正常生成校验。客户端断开从不隐式取消已接受任务。

实时路径保持 `/api/v1/ws` 的 `topic/event/payload/ref/join_ref` 信封。现有业务 handler/任务事件使业务查询失效；`mcp:changed` 仅携带 `project_uuid`，使授权/确认查询以及 ChatArea 的 thread 列表、详情、MCP 概括历史查询失效。首次 join、重新 join 与窗口聚焦沿用全项目 REST 校准；没有定时 HTTP 同步。

## 规范和验证来源

- [MCP HTTP 授权规范](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization)
- [RFC 8707 resource 绑定](https://www.rfc-editor.org/rfc/rfc8707.html)
- [官方 Go SDK 1.7.0 发布说明](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0)
- [MCP 2026-07-28 传输约束](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports)
- [旧版生命周期兼容规范](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle)
- [工具结果规范](https://modelcontextprotocol.io/specification/2025-11-25/server/tools)

OAuth 真实 SDK 集成位于 `internal/server/mcp_oauth_test.go`，覆盖 DCR、授权页面管理接口、PKCE、自动刷新及工具读写；状态和攻击输入回归位于 `internal/mcpserver/oauth_test.go`。原有真实 SDK 集成位于 `internal/server/mcp_test.go`，通过 SDK client → stdio pipes → discovery → HTTP SDK server → 真正项目 REST handlers 验证。任务测试位于 `internal/jobqueue/mcp_integration_test.go`，使用真实应用/项目 SQLite、River 和模拟模型服务；不会产生线上生成费用。发现/帧校验与裁剪测试分别位于 `internal/mcpbridge` 与 `internal/agent`。

桌面原有打包已包含 `lumi_web`，因此无须增加 Rust 业务实现。前端检查与 MCP 二进制进程测试合入现有 `desktop-macos.yml`、`desktop-windows.yml`，复用已有构建产物；Go 全套检查沿用 Windows 流程，Rust/native 检查和完整打包沿用两平台原有步骤，不设独立 MCP workflow。macOS 验证随手动／发布流程执行，Windows 保留 PR 检查；本地不运行 Cargo/Rust。

实际嵌入式二进制使用 SDK `CommandTransport` 的测试位于 `cmd/lumi_web/mcp_process_test.go`，通过 `LUMI_MCP_TEST_BINARY` 指定构建产物。详细执行结果与 CI 边界见 [验证记录](../local-project-mcp-validation.md)。

## MCP 概括历史

项目库的 `mcp_threads` 与 `mcp_thread_activities` 使用现有 `chat_threads` 的 `mcp` 类型作为统一展示入口。session 按项目和授权来源划分，只计算调用受理间隔，30 分钟无调用后下次新建。OAuth 刷新和后台任务均不改变分组规则。

每次受理在项目事务中分配递增序号，结果写回原位置；应用库 `mcp_calls` 保留执行事实。概括只归并相邻已成功的读取或同资源同类别修改，未返回/待确认阻断合并，失败独立保留。只保存轻量动作和资源摘要，不复制参数、响应或媒体。重启和重新打开项目时校准尚未落下的调用结果，不重执行业务操作。

生成保持 `PresentationNone`；MCP 只读 Thread 不生成 Turn/Run/Item，也不依赖模型配置、聊天状态重算或后台任务进度。调用提交成功后只显示已提交，后台结束不回写历史。详情分页在归并后执行，cursor 使用公开 thread/段 UUID 和 revision，拒绝旧版本，前端重读已加载窗口。

SQLite 父表约束调整采用显式标记的迁移：同一连接在事务外暂停外键动作，事务内重建并保留自增序列，提交前执行 `foreign_key_check`，结束后恢复原外键设置；普通迁移行为不变。有 MCP 历史时拒绝丢失数据的降级。
