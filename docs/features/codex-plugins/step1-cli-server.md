# Step 1 · `lumi` CLI 骨架与本地服务托管

## 目标

新增面向终端用户的 `lumi` CLI，负责在本地启动、发现和停止 Lumi server，并能在一个进程内同时服务多个项目。它是 Codex 插件的运行基础。

## 背景

- 方案：`docs/features/codex-plugins/plan.md`；调研结论：`docs/features/codex-plugins/spike.md`（如与本文冲突，以调研结论为准并在交付说明里指出）。
- 现有入口：`cmd/lumi_web`（业务服务，`Makefile` 的 `build` 目标构建）、`cmd/lumi_ctl`（migration 工具）。
- 项目生命周期：`docs/project-storage.md`。每个项目由 `.lumi/project.lock` 的操作系统级独占锁保护；同一进程可同时打开多个项目，按 UUID 注册，空闲回收。
- 实例发现：`internal/mcpbridge/discovery.go` 已有 `{data_dir}/mcp-{environment}.json` 的发现文件格式，可参考或复用。

## 范围

1. 新增 `cmd/lumi`，子命令：
   - `lumi serve`：确保当前数据目录有一个运行中的 Lumi 实例。已有实例（包括桌面版）时直接返回该实例信息，不重复启动；否则以 headless 方式后台启动。
   - `lumi status`：输出实例地址、PID、环境、已打开项目（`--json` 输出机器可读格式）。
   - `lumi stop`：停止由 CLI 启动的实例；对桌面版启动的实例拒绝并说明。
2. 单实例：同一 `LUMI_DATA_DIR` + environment 只允许一个实例，启动竞争需安全处理（文件锁或等价机制），遗留的过期发现信息可被识别和替换。
3. 与桌面版共用数据目录、最近项目索引和发现机制，保证二者不会同时持有同一项目锁。
4. 默认数据目录、开发/生产环境区分沿用 `internal/platformpath` 与 `internal/config` 的现有规则。

## 禁止

- 不修改旧 MCP 逻辑：`internal/mcpserver`、`codex-plugins/`、`docs/local-project-mcp.md` 保持原样。
- 不改变 `project.sqlite`、应用库 schema 的语义；如需新增 migration，必须向后兼容桌面版。
- 不引入定时 HTTP 轮询来同步业务状态（见 `AGENTS.md`）；等待实例就绪可用有超时的启动探测。
- 不运行任何 Cargo / Rust 命令。

## 完成标准

- `go build ./cmd/lumi` 成功，`Makefile` 的 `build` 目标同时产出 `build/lumi`。
- `go test ./cmd/lumi/... ./internal/...` 通过，新增测试至少覆盖：
  - 连续两次 `serve` 返回同一实例；
  - 并发 `serve` 只启动一个实例；
  - 已有「桌面版」实例时 `serve` 复用它、`stop` 拒绝；
  - 发现信息过期（进程已退出）时能重新启动。
- 新增 `docs/features/codex-plugins/cli.md`，说明命令、退出码、`--json` 输出格式。
