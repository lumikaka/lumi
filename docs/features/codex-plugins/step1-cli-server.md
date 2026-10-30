# Step 1 · `lumi` CLI 骨架与本地服务托管

## 目标

新增面向终端用户的 `lumi` CLI，负责在本地启动、发现和停止 Lumi server，并能在一个进程内同时服务多个项目。它是 Codex 插件的运行基础。

CLI 实例与桌面版实例**相互独立**：各自启动、各自发现，互不复用；二者唯一的交集是项目锁。

## 背景

- 方案：`docs/features/codex-plugins/plan.md`；调研结论：`docs/features/codex-plugins/spike.md`（如与本文冲突，以调研结论为准并在交付说明里指出）。
- 现有入口：`cmd/lumi_web`（业务服务，`Makefile` 的 `build` 目标构建）、`cmd/lumi_ctl`（migration 工具）。
- 项目生命周期：`docs/project-storage.md`。每个项目由 `.lumi/project.lock` 的操作系统级独占锁保护；同一进程可同时打开多个项目，按 UUID 注册，空闲回收。
- 实例发现：`internal/mcpbridge/discovery.go` 已有 `{data_dir}/mcp-{environment}.json` 的发现文件格式，可参考写法，但 CLI 使用独立的发现文件。
- 桌面鉴权：生产环境的桌面版由 Tauri 启动器（`rel/app/src-tauri/src/lib.rs`）每次启动生成随机 token，仅通过 `LUMI_DESKTOP_ACCESS_TOKEN` 环境变量传给后端；`internal/server/desktop_auth.go` 要求 `/api/*`、`/media/*` 携带由该 token 换来的 cookie。CLI 不复用桌面实例，因此不需要获取该 token；未设置该环境变量时实例不做 token 鉴权。
- 现有来源校验：`trustedWriteOrigin`（`internal/server/server.go`）要求写请求的 `Origin` 属于 `FRONTEND_URL` 的回环别名，**缺少 `Origin` 时同样返回 403**，因此 CLI / stdio MCP 这类非浏览器客户端目前无法发起写请求。GET 与 `/media` 不校验 `Origin`，服务也不校验 `Host`。
- 应用库 `lumi.sqlite` 的 DSN 已启用 WAL、`busy_timeout(5000)`、`_txlock=immediate`（`internal/config/config.go`），支持多进程并发访问。

## 范围

1. 新增 `cmd/lumi`，子命令：
   - `lumi serve`：确保有一个运行中的 CLI 实例。已有 CLI 实例时直接返回其信息，不重复启动；否则以 headless 方式后台启动。不探测、不复用桌面版实例。
   - `lumi status`：输出 CLI 实例地址、PID、环境、已打开项目（`--json` 输出机器可读格式）。
   - `lumi stop`：停止 CLI 实例。
2. 单实例：同一 `LUMI_DATA_DIR` + environment 只允许一个 **CLI** 实例，启动竞争需安全处理（文件锁或等价机制），遗留的过期发现信息可被识别和替换。CLI 的单实例锁不得影响桌面版后端启动。
3. 发现文件：CLI 使用独立的 `{data_dir}/cli-{environment}.json`，记录 endpoint、PID、environment、nonce；权限 `0600`，原子写入，退出时按 nonce 清理。不含任何凭证。
4. 与桌面版的关系：
   - 共用数据目录与应用库 `lumi.sqlite`（最近项目索引等）。启动时的 migration 必须容忍另一个进程正在运行或已迁移完成（不得因此失败或重复执行破坏性步骤）。
   - 同一项目同一时刻只能被一个实例打开，由项目锁保证。打开被占用的项目时返回明确错误（错误码如 `project_locked`），说明占用方 PID（来自 lock 文件中的展示信息）及释放方式（在桌面版中关闭该项目，或等待 / 执行 `lumi stop`）。
   - CLI 实例的项目空闲回收时间应较短，并在交付说明中给出取值，使用户切回桌面版时无需长时间等待锁释放。
5. 默认数据目录、开发/生产环境区分沿用 `internal/platformpath` 与 `internal/config` 的现有规则。注意 `internal/config` 的默认环境是 `development`：面向终端用户的 `lumi` 默认必须使用 `production`（与桌面版共用数据目录），开发环境需显式指定（如 `--environment development`）。
6. 实例生命周期与项目锁不得绑定任何 MCP 进程：调研实测同一 Codex 会话可能有多条 MCP 连接（不同 PID），不同会话各有独立进程。
7. 鉴权（「本地 CLI 不需要额外鉴权」：CLI 实例与 Vite / hyperframes preview 这类本地服务一样不设 token，防护重点是浏览器侧的跨站请求与 DNS rebinding；同一 OS 用户的本地进程本就能直接读写项目文件，不作为防御对象）：
   - CLI 实例不设置 `LUMI_DESKTOP_ACCESS_TOKEN`，不做 token 鉴权。
   - Host 白名单（所有实例）：新增中间件，只接受 `Host` 为 `127.0.0.1`、`localhost`、`[::1]` 加本实例端口的请求，其余返回 403，以阻断 DNS rebinding 读取 GET 接口与 `/media`。
   - 非浏览器写请求：调整 `trustedWriteOrigin`，缺少 `Origin` 头的写请求放行（浏览器的跨源写请求必然携带 `Origin`）；带 `Origin` 时仍须属于允许列表。WebSocket 的 `CheckOrigin` 规则保持不变。
   - CLI 实例自带的 web UI 直接通过实例地址访问，`lumi status` 输出该地址。

## 禁止

- 不修改旧 MCP 逻辑：`internal/mcpserver`、`codex-plugins/`、`docs/local-project-mcp.md` 保持原样。
- 不改变 `project.sqlite`、应用库 schema 的语义；如需新增 migration，必须向后兼容桌面版。
- 不引入定时 HTTP 轮询来同步业务状态（见 `AGENTS.md`）；等待实例就绪可用有超时的启动探测。
- 不运行任何 Cargo / Rust 命令，不修改 Tauri 启动器。
- 不改变桌面实例的鉴权行为：不得关闭或绕过桌面 cookie 鉴权，不得把桌面会话 token 写入磁盘，不得把 `/api`、`/media` 加入免鉴权白名单。

## 已知限制

- 桌面版与 CLI 不能同时打开同一项目。step3 中「桌面版 web UI 实时看到 MCP 写入」不在首版范围内；需要时再另行设计实例复用或锁移交。

## 完成标准

- `go build ./cmd/lumi` 成功，`Makefile` 的 `build` 目标同时产出 `build/lumi`。
- `go test ./cmd/lumi/... ./internal/...` 通过，新增测试至少覆盖：
  - 连续两次 `serve` 返回同一实例；
  - 并发 `serve` 只启动一个实例；
  - 发现信息过期（进程已退出）时能重新启动；
  - 「桌面版」实例（设置 `LUMI_DESKTOP_ACCESS_TOKEN`、`APP_ENV=production`）运行时，`serve` 仍启动独立的 CLI 实例，两者共用数据目录均正常工作；
  - 项目已被另一实例打开时，CLI 打开该项目返回 `project_locked` 及占用方信息；对方释放后可正常打开；
  - CLI 实例无需凭证即可读写 `/api`（写请求不带 `Origin`）；
  - 所有实例对非回环 `Host`（如 `evil.example:<port>`）返回 403；带不受信任 `Origin` 的写请求仍返回 403；
  - 桌面鉴权开启时行为不变：不带 cookie 的 `/api` 请求返回 401；
  - 两个进程同时对同一应用库启动时 migration 不失败。
- 新增 `docs/features/codex-plugins/cli.md`，说明命令、退出码、`--json` 输出格式、发现文件格式、鉴权约定，以及与桌面版的项目锁冲突与处理方式。
