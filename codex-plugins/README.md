# Lumi MCP plugins

这两个可分发的 Codex 插件参考 VACS 插件的实际结构：`.codex-plugin/plugin.json`、`.mcp.json`、`skills/<name>/SKILL.md`、`agents/openai.yaml`、`assets/icon.png` 和 API 总览参考文档。它们连接正在运行的 Lumi，不启动后端，不携带用户凭据，也不访问项目数据库。

| 插件 | 场景 | MCP URL / OAuth resource |
|---|---|---|
| `lumi-prod-plugin` | 桌面正式版 | `http://127.0.0.1:32323/mcp` |
| `lumi-dev-plugin` | 本地开发版 | `http://127.0.0.1:5801/mcp` |

`.mcp.json` 只配置 `type: http` 与 `url`。Codex 从 MCP 的 401 响应发现资源元数据，再读取授权服务器元数据，通过 `/oauth/register` 动态登记实际的 loopback 回调，包括 `/callback/<server-specific-id>` 后缀。插件不固定 client ID 或 callback，也不重复配置 `oauth_resource`。客户端登记不授予项目权限，仍需授权码、PKCE S256 和 Lumi 用户选择项目与权限。

MCP resource 使用完整 `/mcp` URL，由服务器元数据提供。授权、Token 与注册端点分别是同一地址下的 `/oauth/authorize`、`/oauth/token`、`/oauth/register`。服务端按登记值匹配回调，仅允许 loopback 端口变化，不开放任意回调前缀；授权返回 `iss` 供客户端校验。同一请求可重复携带完全相同的当前 MCP resource，混合资源及其他重复 OAuth 参数会被拒绝。

MCP 与业务服务共用端口。桌面优先 32323，被占用时回退；回退或自定义地址时，从 Lumi 的 MCP 设置复制实际地址，修改插件 `.mcp.json` 的 `url` 及技能依赖 URL，再重新授权。

## 安装与使用

本仓库在 `codex-plugins/` 交付插件源码，不修改已有的 VACS 插件、个人市场或 Codex 已安装配置。

分发时将本仓库 `codex-plugins/` 下所需插件目录完整复制到目标市场的 `plugins/`，在该市场的 `marketplace.json` 中增加对应条目（保留原有插件）：

```json
{
  "name": "lumi-prod-plugin",
  "source": {"source": "local", "path": "./plugins/lumi-prod-plugin"},
  "policy": {"installation": "AVAILABLE", "authentication": "ON_INSTALL"},
  "category": "Productivity"
}
```

开发插件把名称和路径改为 `lumi-dev-plugin`。在目标市场已配置、能通过 `codex plugin list` 找到插件后，使用 `codex plugin add <plugin-name>@<marketplace-name>` 安装。例如，若将插件加入已有 `hxgdzyuyi` 市场并确认当前市场快照包含它，则安装 `lumi-prod-plugin@hxgdzyuyi`。Git 市场需要先让其配置快照包含新插件；仅复制本地文件不会自动发布或安装插件。

从旧版升级时，先重启 Lumi 后端并更新已安装插件，再重新发起 MCP 登录；旧登录链接仍携带固定 client ID，不能用于验证新版。安装后新建一个 Codex 对话：

1. 启动对应环境的 Lumi。
2. 发起插件的 MCP 连接；若需要认证，按客户端提示登录。
3. 在打开的 Lumi 授权页选择一个项目和只读／编辑权限，批准连接。
4. 使用 `$lumi-prod-plugin` 或 `$lumi-dev-plugin`，例如「查看当前授权项目，汇总章节和待处理任务」。
5. 技能先通过 `read_agent_doc` 获取授权项目 UUID、外部允许路由和实时契约，再调用 `request_api`。

项目必须保持打开。需要切换授权项目时重新授权或创建独立连接，不能修改工具参数访问其他项目。危险操作在 Lumi 的「设置 → MCP 设置」中确认；任务和确认结果可断开后恢复。完整说明见 [MCP 使用文档](../docs/local-project-mcp.md)。

## Codex CLI 的 iss 回调错误

`Authorization server response missing required issuer` 不一定代表 Lumi 漏发参数。Codex CLI 0.144.1 的回调解析只保留 code/state，丢弃 iss，随后触发 issuer 校验失败；Lumi 的批准和拒绝回调都保留 iss。先用 `command -v codex` 和 `codex --version` 检查实际执行的版本。桌面 App 与 PATH 中的独立 CLI 可能不是同一版本。

macOS 上若存在 App 自带 CLI，可检查并使用较新的版本发起登录：

```sh
/Applications/ChatGPT.app/Contents/Resources/codex --version
/Applications/ChatGPT.app/Contents/Resources/codex mcp login lumi-dev-plugin
```

正式插件将服务器名替换为 `lumi-prod-plugin`。也可以更新独立 CLI 后重试；保留服务端 issuer 支持和回调中的 iss。登录命令会自动打开授权页，不需要重复打开同一 URL。

源码依据：[Codex 0.144.1 OAuth 回调解析](https://github.com/openai/codex/blob/rust-v0.144.1/codex-rs/rmcp-client/src/perform_oauth_login.rs)。

## 与 VACS 的使用差异

- Lumi 用 `read_agent_doc`，不新增没有实际兼容需要的 `read_api_doc` 别名。
- 授权项目 UUID 来自文档工具返回的 `data.project_uuid`，随后使用原有 `/api/v1/projects/<uuid>/...` 路由；不另建 `/api/mcp/current-project` 业务接口。
- 文档返回 `external_routes` 和 `external_instructions`。插件随包提供术语表、外部路由索引和典型调用示例；该索引是源码快照，实际调用以实时 `external_routes` 和领域 Contract 为准。
- 写入保留 `expected_revision`、顶层 `idempotency_key`；确认使用已持久化请求，不能传入 `confirmed=true`。
- `request_api` 的业务信封位于 `data.result`，调用状态位于 `data.call`；媒体通过 `read_media`，不依赖桌面 Cookie URL。

## 维护插件资源

两个插件的 `assets/icon.png` 复用 `rel/app/src-tauri/icons/icon.png`，manifest 的 `composerIcon` 与 `logo` 指向该图标。图标更新时同步两份资源。

两个技能的 `references/overview.md` 内容保持一致；路由索引按 `internal/agent/external_project_api.go` 的显式允许列表整理，method/path 与 `agent_api_registry.go`、`agent_api_phase3_routes.go` 对照，领域文档来自 `internal/agent/docs/api/`。开放路由变化时同步两份索引，调用示例中的写操作须包含顶层幂等键。

## 验证

服务端集成测试直接读取这两份 `.mcp.json`，检查 URL 与运行环境默认值相符且无固定 OAuth 配置，通过官方 Go MCP SDK 完成动态注册、带后缀回调、重复 resource、文档读取、自动刷新、只读/编辑和撤销。测试用隔离监听端口替代固定端口，避免连接用户实际项目。动态注册的原有测试继续保留。

```sh
go test ./internal/server ./internal/mcpserver -run 'TestMCPOAuth|TestOAuth' -count=1
```

插件 manifest 和 SKILL 已通过对应校验器。未自动安装到用户 Codex 市场，不能将 SDK 配置集成验证等同于已经完成用户 Codex App 的插件安装测试。

回调与配置依据：[Codex MCP OAuth](https://learn.chatgpt.com/docs/extend/mcp?surface=cli#oauth-client-registration-and-callbacks)、[Codex 配置参考](https://learn.chatgpt.com/docs/config-file/config-reference)。
