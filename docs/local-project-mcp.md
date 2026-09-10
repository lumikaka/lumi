# Lumi 本地项目 MCP

Lumi 作为本机 MCP Server，允许外部 AI 客户端读取、编辑用户授权的一个项目。它不提供内置 Agent 连接其他 MCP Server 的能力，提供本机 HTTP OAuth 与 stdio 两种连接方式，不包含云端账号、成员体系或公网服务。

## Codex 插件方式

仓库提供与 VACS 相同组件结构的 [Lumi 插件](../codex-plugins/README.md)：桌面正式版 `lumi-prod-plugin`、开发版 `lumi-dev-plugin`。插件配置 HTTP URL，由 Codex 发现完整 `/mcp` OAuth resource 并动态登记实际 loopback 回调，并提供「先读实时文档，再操作授权项目」的技能。插件经目标市场安装后，连接时由 Lumi 授权页选择项目与权限，无须手动填写项目 UUID 或 Token。动态注册不跳过用户授权。

## HTTP OAuth 连接（推荐）

1. 启动 Lumi，在左侧栏底部「设置 → MCP 设置」进入 `/settings/mcp`，复制页面显示的 MCP 地址。入口不要求先进入项目或配置服务商。
2. 在支持 HTTP OAuth 的本机 MCP 客户端添加该地址，传输选择 Streamable HTTP，发起连接。
3. 客户端通过元数据发现本机授权服务并打开 Lumi 授权页。核实这是自己刚发起的连接，选择一个项目及「只读」或「编辑与生成」，点击「授权并连接」。Lumi 会打开所选项目；编辑前应完成项目设置。
4. 客户端自动取得绑定该项目的凭据，无须复制 Token。需要另一个项目时创建另一项连接并重新授权。已授予权限可在 MCP 设置中选择对应项目后查看和撤销。
5. 保持 Lumi 运行且项目打开。客户端断开不会关闭项目或取消已受理的生成任务。项目仍按现有租约、任务和空闲回收规则管理。

MCP 与 Lumi 业务服务共用一个端口。开发地址为 `http://127.0.0.1:5801/mcp`；桌面正式版优先使用 `http://127.0.0.1:32323/mcp`。32323 被占用时，桌面启动器回退到可用端口，MCP 设置页显示本次实际地址。以下为采用 `mcpServers` / `url` 格式的客户端配置示意；具体字段以客户端为准，也可直接在客户端界面粘贴地址：

```json
{
  "mcpServers": {
    "lumi": { "url": "http://127.0.0.1:32323/mcp" }
  }
}
```

凭据采用授权码 + PKCE S256 流程。授权请求和一次性授权码总有效期 5 分钟；访问令牌有效期 1 小时。客户端请求 `offline_access` 并经用户确认时，获得可轮换的刷新令牌，整个刷新链有效期 7 天；超过期限需重新授权。撤销后访问及续期立即拒绝新请求。刷新令牌重放会撤销整个授权。重新连接不改变危险操作的单独确认要求。

仅支持本机原生公共客户端，通过动态注册登记 HTTP loopback 回调（`127.0.0.1`、`localhost` 或 `::1`，必须有端口）。不支持云端连接器、远程 HTTPS 回调、客户端元数据 URL 获取或完整账号 OAuth 服务。客户端名称是客户端自报信息。浏览器需使用已打开 Lumi 的本地会话；若提示桌面未认证，从 Lumi 菜单栏重新打开应用后返回授权页。不同浏览器之间不会共享该会话。

不再使用独立的 `LUMI_MCP_ADDRESS` 配置。MCP 地址直接来自业务监听器的实际 `APP_ADDRESS`（本机 IPv4 loopback）。桌面启动器选择端口并传入业务地址；开发时可通过现有 `APP_ADDRESS` / `FRONTEND_URL` 自定义。端口回退或自定义地址后，HTTP 插件须同步更新 `url` 和技能依赖 URL，再重新 OAuth 授权；也可以释放 32323 后重启 Lumi 恢复默认地址。不要把凭据发送给占用原端口的其他进程。stdio 每次重新发现实例，因此回退时无需修改连接配置。

从旧独立端口升级后，HTTP 客户端需更新连接地址并重新授权；原手动 stdio 授权仍可使用。重新授权不会取消已受理任务，旧授权的调用历史与结果仍可在 Lumi 的 MCP 设置中查看；新授权不能用 get_call 读取另一授权的调用。MCP Bearer 凭据与桌面 Cookie 按路径独立校验，共用端口不授予普通 REST、授权管理或媒体 URL 的访问权限。

## stdio 连接

不支持 HTTP OAuth 的客户端仍可在 MCP 设置中选择项目，填写名称和权限，创建手动授权，然后复制 JSON 配置。明文只展示一次；丢失后撤销旧授权并新建。手动授权无自动过期，需主动撤销。

UI 复制的配置包含当前后端可执行文件绝对路径、运行环境和应用数据目录，不包含动态端口。macOS 安装版示例：

```json
{
  "mcpServers": {
    "lumi": {
      "command": "/Applications/Lumi.app/Contents/Resources/backend/lumi_web",
      "args": ["--mcp", "--environment", "production"],
      "env": { "LUMI_MCP_TOKEN": "<创建授权时显示的凭据>" }
    }
  }
}
```

实际安装路径以 Lumi 复制的配置为准。Windows 使用相应的 `lumi_web.exe`；如果移动应用安装目录，需要更新 `command`。对 stdio 连接，应用常规升级或后端重启导致的端口改变不需要修改配置。

开发版可先执行 `go build -o build/lumi_web ./cmd/lumi_web`，把 `command` 指向该文件，使用 `--environment development`。自定义 `LUMI_DATA_DIR` 时增加 `--data-dir /absolute/app/data`。默认目录使用 Lumi 已有平台目录解析，并区分开发与生产；stdio 模式不会加载项目 `.env`、打开数据库或启动业务运行时。

## 工具

| 工具 | 用途 |
|---|---|
| `read_agent_doc` | 从内置共享 Markdown 读取契约；起点为 `/api/v1/agent-docs/overview.md`。返回授权 `project_uuid`、权限以及明确的 `external_routes` 清单。 |
| `request_api` | 调用清单内的项目 API。要求 `method`、`url`、`response_filter`；可带 `query` 和 `request_body`；写入必须带顶层 `idempotency_key`。 |
| `get_call` | 通过 `call_uuid` 获取该授权的持久化调用状态和结果，适用于断开重连、确认和结果恢复。 |
| `read_media` | 使用本项目 `file_uuid` 返回 MCP 图片内容及业务信封；支持不超过 4 MiB 的 PNG、JPEG、WebP、GIF。 |

只支持显式清单中的项目资料、章节/绘本、正文、设定、页面/画面段落、脚本和图片版本、快照、任务与项目媒体操作。共享文档也服务内置 Agent，其中其他工具、工作流和接口不因此获得外部权限；以 `external_routes` 为准。新增内部路由默认不暴露。

第一版不开放聊天管理、项目路径打开、授权管理、全局服务商设置、上传/导出/GC，以及无逐项版本保护的批量清空回收站接口。可以在 Lumi UI 中使用这些原有功能。`read_api_doc` 不设别名。

读取项目资料：

```json
{
  "method": "GET",
  "url": "/api/v1/projects/<授权的 project_uuid>",
  "response_filter": ".data | {uuid,name,description,revision}"
}
```

编辑项目资料：

```json
{
  "method": "PATCH",
  "url": "/api/v1/projects/<授权的 project_uuid>",
  "request_body": {"name": "新名称", "description": "项目简介", "expected_revision": 1},
  "response_filter": ".data | {uuid,name,revision}",
  "idempotency_key": "rename-project-001"
}
```

`response_filter` 作用于业务信封 `.data`；对象接口必须选择契约声明的字段，不能用宽泛的 `.data` 读取整个对象。空值接口可用 `.data`。结果经过共享 projector 和内部字段清理，普通业务结果限制为约 64 KiB，超过限制时要求缩小投影或分页。媒体另有 4 MiB 限制。

工具返回的 MCP `content` 包含文本信封，`structuredContent` 为同一信封。`request_api` 的 `data.call` 是调用状态，`data.result` 是原始业务成功或错误信封；写入已经完成而结果超限时，也必须先读取状态，不能换幂等键盲目重试。业务错误同时设置 MCP `isError`。

## 生成与恢复

当前支持章节正文、来源设定图、设定拆解、页面图片生成，全部使用现有持久化任务运行时。服务商与默认模型由 Lumi 的项目/全局模型配置解析；契约允许时可提供 `model` 覆盖，不能传入服务商凭据或伪造 `provider_uuid`。

```json
{
  "method": "POST",
  "url": "/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/generations",
  "request_body": {"prompt_key":"story_chapter", "prompt":"补充这一章的情节"},
  "response_filter": ".data | {uuid,kind,status}",
  "idempotency_key": "chapter-generation-001"
}
```

受理后返回任务 UUID，调用 `succeeded` 表示受理成功，任务本身可能仍为 `queued` 或 `running`。通过清单中的 `GET .../tasks/{task_uuid}` 或 `GET .../production-tasks/{task_uuid}` 读取任务事实状态。客户端需要状态时主动查询；Lumi UI 使用 WebSocket 失效提示重新读取 REST，不做定时 HTTP 轮询。

调用以应用库授权和调用 UUID 归属，任务使用 `mcp:<call_uuid>` 幂等键。工具活动会生成只读 `thread_type=mcp` 展示线程，不创建虚构的聊天消息、`chat_turns` 或 `chat_runs`。原有任务自己的 `agent_threads` / `agent_runs` 审计仍由任务运行时正常创建，这些不是 MCP 聊天上下文。

重试同一写入必须复用原 `idempotency_key` 和完整参数（包括投影），不同参数返回 `mcp_idempotency_conflict`。后端退出后，已提交任务按原运行时规则恢复；如果任务已提交但 MCP 结果还未保存，同键重试通过持久化任务幂等键找回原任务。普通同步写入若在进程崩溃时结果不明，会标为 `interrupted`，不会自动重放；先核查业务资源当前状态再决定新操作。

## 危险操作

删除、恢复快照等要求确认的操作会返回 `pending_confirmation` 和调用 UUID，而不会长期占用一个待回复的 MCP 请求。Lumi 保存完整原始参数、请求指纹、原授权和 15 分钟有效期。

用户在「设置 → MCP 设置」中选择对应项目，查看客户端名称、操作内容和指纹，选择拒绝或确认。确认端点只接受决定和指纹，不接受替换操作；执行重新使用共享参数校验和业务版本检查。撤销授权、过期、内容被篡改或版本冲突均不会绕过约束。确认后的结果可用 `get_call` 读取，拒绝/过期返回对应终态；再次决定不会重复执行。

客户端的 `confirmed=true`、自行重放、聊天 UUID、内部 Route/Execution 元数据都不授予权限。授权只能在 Lumi 中创建和撤销，MCP 工具无法调用管理接口。

## 常见错误

| 情况 | 处理 |
|---|---|
| 后端不可用或发现信息过期 | 启动正确环境的 Lumi；桥接每条消息重新发现实例，不重放结果不明的写入。 |
| `project_not_open` | 在 Lumi 打开已授权项目；客户端不能提供磁盘路径打开项目。 |
| 凭据缺失、无效、已撤销 | OAuth 客户端重新连接授权；stdio 客户端新建授权并更新环境变量。桌面 Token/Cookie 不能代替 MCP 凭据。 |
| `mcp_read_only` | 此授权不能编辑；如确需编辑，在 Lumi 创建编辑授权。 |
| 版本冲突 | 重读资源，核实新状态，再使用新操作及幂等键提交。 |
| `pending_confirmation` | 在 Lumi 审核原始请求，之后调用 `get_call`。 |
| `interrupted` | 普通写入的结果不明，核查资源，不自动重放。 |
| 媒体不可读取 | 确认文件属于本项目、状态可用且满足类型/大小限制。不要请求 Cookie 保护的图片 URL。 |

多个项目可分别创建授权，并在客户端配置中使用不同的服务器名称；每个凭据始终只能访问它绑定的项目。

更多细节见 [架构说明](architecture/local-project-mcp.md)、[验证记录](local-project-mcp-validation.md) 与 [项目级 MCP PRD](prds/projects/features/本地项目级MCP接入.md)。

## 在 ChatArea 查看 MCP 操作历史

MCP 活动直接显示在原有 thread 列表中，无须切换页签。按项目和授权来源聚合，连续 30 分钟无工具调用后，下次调用创建新 thread；后台任务不延长会话。

打开后可查看按调用受理顺序保留的概括历史。连续成功读取、同资源同类别修改会合并，切换操作或资源后另起一段；失败独立保留，晚返回也不会打乱顺序或被成功覆盖。概括由固定规则生成，不调用 AI，也不显示详细参数和完整响应。

生成请求只显示“已提交”，此处不追踪任务进度或完成结果。MCP thread 没有聊天输入框和运行状态；待确认条目可前往原 MCP 设置页处理。原始请求与结果仍使用既有 MCP 管理和 `get_call` 能力查看。
