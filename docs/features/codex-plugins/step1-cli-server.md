# Step 1 · `lumi` CLI 骨架与本地服务托管

## 目标

新增面向终端用户的 `lumi` CLI，负责在本地启动、发现和停止 Lumi server，并能在一个进程内同时服务多个项目。它是 Codex 插件的运行基础。

## 背景

- 方案：`docs/features/codex-plugins/plan.md`；调研结论：`docs/features/codex-plugins/spike.md`（如与本文冲突，以调研结论为准并在交付说明里指出）。
- 现有入口：`cmd/lumi_web`（业务服务，`Makefile` 的 `build` 目标构建）、`cmd/lumi_ctl`（migration 工具）。
- 项目生命周期：`docs/project-storage.md`。每个项目由 `.lumi/project.lock` 的操作系统级独占锁保护；同一进程可同时打开多个项目，按 UUID 注册，空闲回收。
- 实例发现：`internal/mcpbridge/discovery.go` 已有 `{data_dir}/mcp-{environment}.json` 的发现文件格式，可参考或复用。该文件只含旧 `/mcp` endpoint 与 nonce，不含任何 `/api` 访问凭证。
- 桌面鉴权：生产环境的桌面版由 Tauri 启动器（`rel/app/src-tauri/src/lib.rs`）每次启动生成随机 token，仅通过 `LUMI_DESKTOP_ACCESS_TOKEN` 环境变量传给后端，并以 `APP_ENV=production` 运行；后端只保存其 sha256 并立即清除该环境变量（`internal/config/config.go`）。`internal/server/desktop_auth.go` 的中间件要求 `/api/*`、`/media/*`（`/api/v1/health`、`/api/v1/desktop-sessions` 除外）携带由该 token 换来的 `lumi_desktop_session` cookie。token 不落盘，CLI 无法获取；未设置该环境变量时实例完全不鉴权。
- 现有来源校验：`trustedWriteOrigin`（`internal/server/server.go`）要求非 GET/HEAD/OPTIONS 请求的 `Origin` 属于 `FRONTEND_URL` 的回环别名，**缺少 `Origin` 时同样返回 403**，因此 CLI / stdio MCP 这类非浏览器客户端目前无法发起写请求。GET 与 `/media` 不校验 `Origin`，服务也不校验 `Host`。

## 范围

1. 新增 `cmd/lumi`，子命令：
   - `lumi serve`：确保当前数据目录有一个运行中的 Lumi 实例。已有实例（包括桌面版）时直接返回该实例信息，不重复启动；否则以 headless 方式后台启动。
   - `lumi status`：输出实例地址、PID、环境、已打开项目（`--json` 输出机器可读格式）。
   - `lumi stop`：停止由 CLI 启动的实例；对桌面版启动的实例拒绝并说明。
2. 单实例：同一 `LUMI_DATA_DIR` + environment 只允许一个实例，启动竞争需安全处理（文件锁或等价机制），遗留的过期发现信息可被识别和替换。
3. 与桌面版共用数据目录、最近项目索引和发现机制，保证二者不会同时持有同一项目锁。
4. 默认数据目录、开发/生产环境区分沿用 `internal/platformpath` 与 `internal/config` 的现有规则。注意 `internal/config` 的默认环境是 `development`：面向终端用户的 `lumi` 默认必须使用 `production`（与桌面版一致），开发环境需显式指定（如 `--environment development`），否则会落到另一个数据目录、发现不了桌面版实例。
5. 实例生命周期与项目锁不得绑定任何 MCP 进程：调研实测同一 Codex 会话可能有多条 MCP 连接（不同 PID），不同会话各有独立进程。
6. 鉴权分层（「本地 CLI 不需要额外鉴权」：CLI 实例与 Vite / hyperframes preview 这类本地服务一样不设 token，防护重点是浏览器侧的跨站与 DNS rebinding；同一 OS 用户的本地进程本就能直接读写项目文件，不作为防御对象）：
   - **CLI 启动的实例**：不设置 `LUMI_DESKTOP_ACCESS_TOKEN`，不做 token 鉴权。
   - **Host 白名单（所有实例）**：新增中间件，只接受 `Host` 为 `127.0.0.1`、`localhost`、`[::1]` 加本实例端口的请求，其余返回 403，以阻断 DNS rebinding 读取 GET 接口与 `/media`。
   - **非浏览器写请求**：调整 `trustedWriteOrigin`，缺少 `Origin` 头的写请求放行（浏览器的跨源写请求必然携带 `Origin`）；带 `Origin` 时仍须属于允许列表。WebSocket 的 `CheckOrigin` 规则保持不变。
   - **桌面实例**：保留现有 cookie 鉴权不变。仅当 `DesktopAuth` 开启时，后端额外生成一个随机本地 token（与桌面会话 token 独立），写入发现文件；鉴权中间件额外接受 `Authorization: Bearer <本地 token>`，持有有效 bearer 的请求视同已认证。该 token 只用于让 CLI 复用桌面实例，由后端自行生成写入，无需修改 Tauri 启动器。
   - **发现文件**：记录 endpoint、PID、启动方（`desktop` / `cli`）、environment、nonce，以及桌面实例的本地 token；权限 `0600`，原子写入，退出时按 nonce 清理。CLI 访问实例时若发现文件含 token 则附带 bearer。
   - `lumi status --json` 不得输出 token 本身。

## 禁止

- 不修改旧 MCP 逻辑：`internal/mcpserver`、`codex-plugins/`、`docs/local-project-mcp.md` 保持原样。
- 不改变 `project.sqlite`、应用库 schema 的语义；如需新增 migration，必须向后兼容桌面版。
- 不引入定时 HTTP 轮询来同步业务状态（见 `AGENTS.md`）；等待实例就绪可用有超时的启动探测。
- 不运行任何 Cargo / Rust 命令。
- 不放宽现有桌面鉴权：不得为 CLI 关闭桌面实例的鉴权、把桌面会话 token 写入磁盘，或把 `/api`、`/media` 加入免鉴权白名单。

## 已知限制

- Tauri 启动器当前不读取发现文件，每次启动都会自行拉起后端。CLI 实例运行时再打开桌面版，会与「同一数据目录 + environment 只允许一个实例」冲突。本步骤须保证后启动者安全失败（不得同时持有同一项目锁、不得损坏数据），并给出明确错误；桌面版改为复用已有实例另行立项（涉及 Rust 改动，只能在 CI 中构建验证）。

## 完成标准

- `go build ./cmd/lumi` 成功，`Makefile` 的 `build` 目标同时产出 `build/lumi`。
- `go test ./cmd/lumi/... ./internal/...` 通过，新增测试至少覆盖：
  - 连续两次 `serve` 返回同一实例；
  - 并发 `serve` 只启动一个实例；
  - 已有「桌面版」实例（设置 `LUMI_DESKTOP_ACCESS_TOKEN`、`APP_ENV=production`）时 `serve` 复用它、`stop` 拒绝，且使用发现文件中的本地 token 调用 `/api` 成功；
  - 桌面鉴权开启时，不带凭证或凭证错误的 `/api` 请求返回 401；
  - CLI 启动的实例无需凭证即可读写 `/api`（写请求不带 `Origin`）；
  - 所有实例对非回环 `Host`（如 `evil.example:<port>`）返回 403；带不受信任 `Origin` 的写请求仍返回 403；
  - 发现文件权限为 `0600`，CLI 实例的发现文件不含 token，`lumi status --json` 输出不含 token；
  - 发现信息过期（进程已退出）时能重新启动。
- 新增 `docs/features/codex-plugins/cli.md`，说明命令、退出码、`--json` 输出格式，以及发现文件格式与鉴权分层（CLI 实例无 token、桌面实例 bearer）的约定。
