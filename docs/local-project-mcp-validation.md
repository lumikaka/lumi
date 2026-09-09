# 本地项目 MCP 验证记录

验证日期：2026-09-09。实现按只读、编辑/生成、危险确认与连接体验三个阶段交付。VACS 仅用于只读参考；测试使用临时应用库和项目库，不使用用户项目或真实模型生成服务。

## 自动验证

| 验证范围 | 证据 |
|---|---|
| 当前 MCP 2026-07-28 发现/版本元数据、旧版 2025-11-25 初始化、通知、工具发现、文档与错误 | [服务端集成测试](../internal/server/mcp_test.go)，真实官方 Go SDK 1.7.0 client，经 stdio pipes 和 HTTP 到真正 REST handlers |
| 插件实际 `.mcp.json` URL-only 配置、DCR 带后缀回调、重复 resource、SDK 授权读写/续期 | [插件配置直接参与的 SDK 集成](../internal/server/mcp_oauth_test.go)及 [回调边界](../internal/mcpserver/oauth_test.go)；正式/开发两种插件、两种权限均覆盖 |
| HTTP OAuth 发现、DCR、Lumi 授权 REST、PKCE、SDK 自动刷新、只读/编辑及撤销 | [官方 Go SDK OAuth 集成](../internal/server/mcp_oauth_test.go)，真实 HTTP 与工具调用 |
| OAuth 回调/客户端/resource/PKCE 篡改、授权码重放、刷新重放撤销、过期、拒绝、关闭项目、Host/Origin/nonce | [OAuth 状态与边界回归](../internal/mcpserver/oauth_test.go)；桌面 Cookie/Origin/未知字段由服务端 OAuth 集成覆盖 |
| 缺失、无效、撤销凭据；只读；项目隔离；内部元数据和路径攻击 | 同上；[外部路由及裁剪测试](../internal/agent/external_project_api_test.go) |
| 普通编辑、版本冲突、同键重复和参数变化；内部 ID 清理、输出大小 | 同上 |
| 危险操作确认、重复决定、拒绝、过期、篡改、版本复核及结果读取 | [服务端测试](../internal/server/mcp_test.go)，[东八区有效期回归](../internal/mcpserver/store_test.go) |
| 生成归属、配置来源、断开不取消、关闭后恢复、提交后结果丢失的幂等恢复、配置变化后找回原任务 | [真实 River/SQLite 与模拟模型集成](../internal/jobqueue/mcp_integration_test.go)；验证没有虚构 chat_threads，且只创建一个任务 |
| 开发/生产隔离、发现文件替换和旧实例清理、非法发现目标、协议 stdout | [桥接测试](../internal/mcpbridge/discovery_test.go) |
| 实际二进制 stdio 入口、业务端口动态回退发现与同端口 OAuth、授权码跨进程崩溃兑换、关闭项目错误、持久化结果恢复 | [二进制进程测试](../cmd/lumi_web/mcp_process_test.go)，官方 SDK `CommandTransport` 启动嵌入前端的 `lumi_web --mcp` |
| MCP 写入 → WebSocket 业务事件和 mcp:changed → REST 读取真实状态 | [真实 WebSocket 服务端测试](../internal/server/mcp_test.go)及 [TanStack Query 失效映射](../web/src/realtime/projectRealtimeQueries.test.js) |
| 授权项目图片读取、实际 MCP 图片字节、跨项目/无效媒体拒绝 | [服务端媒体测试](../internal/server/mcp_test.go) |

执行命令：

```sh
go test ./...
pnpm --dir web test
pnpm --dir web run build
go build -tags embed_frontend -o /tmp/lumi-mcp-backend ./cmd/lumi_web
LUMI_MCP_TEST_BINARY=/tmp/lumi-mcp-backend go test ./cmd/lumi_web -run TestMCPPackagedProcessReconnect -count=1 -v
git diff --check
```

前端 391 项检查通过，生产构建通过；构建保留现有的大于 500 kB chunk 提示。实际后端二进制和官方 SDK 进程恢复测试通过。Go 全套检查通过，最终 OAuth/配置/服务端/命令改动的相关检查再次通过。原有 UTC 有效期回归保留。

## 界面验证

设置入口现已调整为全局左侧栏「设置 → MCP 设置」（`/settings/mcp`）。在开发实例验证了菜单入口、独立设置页、项目选择器与空状态，并通过前端检查和构建。选择项目后沿用项目 WebSocket 校准机制；项目概览不再重复放置管理卡片。

新增 OAuth 浏览器验证使用独立临时应用库和项目，启用真实桌面会话认证。通过 MCP 地址创建授权请求，在页面选择项目与只读权限并批准，真实 loopback 回调收到 state/code/iss 并自动兑换访问及刷新凭据；回到设置页可查看并撤销该 OAuth 授权。临时客户端与后端验证后关闭，不修改用户项目。授权页检查后补充时分显示，明确标注手动 stdio 授权入口。

以下为原有编辑/确认界面验证：使用隔离项目的实际嵌入前端和真实管理/业务后端。夹具仅将「服务商已配置」入口状态设为 ready，以免配置或使用真实凭据；它不模拟 MCP 授权、调用、确认、业务写入或 WebSocket。

观察授权卡片与读/编辑选项；通过实际 MCP HTTP 请求生成待确认操作，验证 WebSocket 自动刷新列表；在浏览器点击确认，将测试绘本移入可恢复的回收站，验证完成状态、原始参数、指纹、版本和业务结果可展开读取，活跃绘本数量自动更新。

此次检查发现并修复本地东八区时间与 SQLite UTC 时间字符串比较造成提前过期的问题。所有数据库过期比较使用 UTC；回归测试独立固定 UTC+8，不依赖 CI 的本地时区。

## CI 与验证边界

[macOS 桌面 workflow](../.github/workflows/desktop-macos.yml) 与 [Windows 桌面 workflow](../.github/workflows/desktop-windows.yml) 复用已有依赖安装和构建产物，运行前端 `pnpm test` 与 MCP 二进制进程集成。macOS 使用已构建的 `.app` 内后端；Windows 使用即将打入安装包的后端。两者均设置 `LUMI_MCP_TEST_BINARY` 并检查文件存在，防止测试静默跳过。macOS 额外执行同一源码的 OAuth SDK/状态回归，不重复构建桌面包；Windows 保留原有 `go test ./...`，两平台保留原有 Rust/native 测试与完整打包，不再另设 MCP workflow 重复构建。

macOS 检查随现有手动／发布流程执行，不在每个 PR 上运行。Windows 保留已有 PR、手动和发布流程调用入口。没有新增普通分支 push 触发。

本地没有运行 Cargo、Rust 编译/检查/测试，也没有运行完整桌面打包、签名、发布或部署。合并后的 CI 步骤尚未在本次本地会话执行，不能把这些原生/跨平台检查视为已通过。生成测试使用模拟服务商，不宣称已验证真实服务商联网效果。


插件 manifest 与 SKILL 分别通过 plugin-creator 的 validate_plugin.py 和 skill-creator 的 quick_validate.py。插件源码位于 `codex-plugins/`，未修改 VACS 插件或用户的市场配置，也未声称已在 Codex App 完成插件安装验证。


MCP 已合并到业务监听器。官方 SDK 的动态注册/预注册、读写/续期测试现直接访问同一个 Echo 服务；新增共端口鉴权隔离回归：Cookie 不能认证 MCP，MCP Bearer 不能认证普通 REST 或管理接口，OAuth 元数据无需桌面会话。实际二进制测试核对设置返回的 MCP 地址等于业务地址加 `/mcp`，并覆盖端口不变的 OAuth 码恢复与业务端口变化后的 stdio 发现。桌面 32323 首选/被占用回退沿用已实现的启动器逻辑，本次不修改 Rust、不在本地执行 Rust 验证。
