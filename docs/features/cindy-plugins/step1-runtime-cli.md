# Step 1 · 插件运行时与 Lumi CLI

## 目标

在 Cindy 内建立可靠的 Lumi 工具调用通路，复用现有 CLI/server，让多个任务能够访问各自项目，且插件进程退出不会误停其他任务的服务。

## 前置

阅读 [总体方案](plan.md)、[开发依据](references.md) 和 Step 0 产出的 `spike.md`。Node/CLI 与上下文门槛须通过。实现前按开发 Skill 对齐仍未确定的设计；不重新询问已有明确选择。

2026-11-01 补充：[本地开发与双版本隔离](development.md) 属于本步待补齐范围。已有 Step 1/2 包与历史测试不作为独立开发通道已实现的证明。

## 范围

1. 在作者目录 `cindy-plugins/lumi/` 建立 v3 插件骨架。优先使用 Forge `node-json-rpc` 模板，`main.js` 保持浏览器代码，`node/worker.cjs` 为制作阶段打包好的 Node 入口。
2. Go CLI 不能直接填进 `node.entry`。Node 使用 `spawn` 的参数数组和绝对可执行路径运行随包 CLI，禁止 shell 拼接、依赖 GUI PATH 或把 `process.execPath` 当普通 Node 二进制。
3. 首选让 worker 按项目管理 `lumi mcp` 子进程，每个子进程在创建时固定 `cwd`，worker 自己完成该子进程的 MCP 初始化。Cindy 与 worker 使用 JSON-RPC；不要把这条子进程协议与 Host 自动初始化的 `node.protocol=mcp-stdio` 混用。
4. 插件进程可共享，项目上下文不可共享：按规范化根目录、environment、data-dir 与 CLI 版本/代次管理连接；禁止全局 `process.chdir()`。每个请求绑定发起任务、项目和请求编号，响应按原请求返回。
5. main.js 只使用 Host 注入的 `session_context`；校验本地/只读状态后才能派发。工具不公开任意路径、命令、可执行文件覆盖、环境变量或 SQL 参数。
6. 每个在途 Node 工具请求透传真实 `msg.callId`。按文档决定 `cancelWithCall`，验证取消仅结束该请求/其专属进程，不杀其他任务共用连接；不能为单次请求绑定并杀掉全插件共享子进程。
7. 区分三个生命周期：Cindy worker、各项目 MCP 连接、共享 CLI server。连接需要引用计数与有界空闲回收；插件停用/升级/退出时回收代理和未提交暂存，不默认执行全局 `lumi stop`。server 沿用已有单实例、空闲项目回收及项目锁。
8. 业务错误保留 Lumi `success/data/error`；Host `ok:false`、进程崩溃、超时、协议错误明确映射为失败，不包装为业务成功。写入超时视为结果未知，后续重读，不自动重放写操作。
9. 日志写 stderr，stdout 只传协议。遵守 Node 单请求 params、单行响应与总请求数限制；超限返回可行动错误，不截断 JSON。
10. 先提供最小诊断/只读入口验证运行通路，项目与完整工具在 Step 2/3 扩展。诊断只公开必要版本/状态，不返回私有路径、系统环境或凭证。

## 正式与开发入口（新增待实现）

- 从同一份 `cindy-plugins/lumi/` 源码生成正式 `lumi` 与开发 `lumi-dev`，同步区分 manifest id、command、显示名称和 Manual 入口，避免维护两份业务代码。
- 增加 `make cindy-dev`，生成完整的 `build/cindy-dev/lumi-dev/` 作者目录与随包 `bin/lumi_dev`，记录 channel、源码和 CLI 指纹；不隐式安装或发布。此命令目前不存在，实施后再将 README 改为可执行指引。
- 开发 CLI 默认 development，worker 对运行及诊断命令显式传 `--environment development --data-dir <开发目录>`，默认目录为 `~/.lumi_dev`。正式入口保留 `production + ~/.lumi`；自定义开发目录来自受控构建配置，不开放为 Agent 参数。
- 增加明确的开发停止入口 `make cindy-dev-stop`，使用同一开发 data-dir；仅在用户已协调该实例使用者后执行，不作为构建/更新/卸载的自动步骤。开发目录可能与 Codex 开发版共享，不能因为名称含 dev 就自动 stop。
- 诊断提供必要的 channel/构建标识，便于区分已安装新包与仍存活的旧 server；不泄露路径、endpoint、PID 或 nonce。重新绑定时复核实际实例代次，不因磁盘二进制变化就宣称后台已更新。
- 完整开发循环按 [development.md](development.md)：修改 → 生成 → Forge pack → 协调旧开发任务/后台 → Forge install → 实际调用验证。只编译仓库中的 CLI 或只打包不会更新安装副本，JS/Manual 变化也必须重新打包安装。

## 与现有实现的关系

复用 `cmd/lumi` 的发现文件、实例身份验证、启动竞争处理，保留正式默认 production 环境，并复用现有开发 CLI 的 environment 选择机制。现有 server 缺少所需只读能力时补最小兼容扩展；不改桌面 token 鉴权，不借用通用 HTTP OAuth MCP，也不重写 `internal/textmcp` 的业务逻辑。

## 交付

- 插件骨架、worker 连接管理与针对生命周期/并发的测试。
- 共享源码的正式/开发产物生成、显式开发停止入口，以及身份/environment/data-dir/CLI 指纹一致性测试；更新 development.md 和作者 README 的真实命令状态。
- `docs/features/cindy-plugins/runtime.md`：实际消息链、参数与大小限制、服务发现、生命周期、错误与恢复。
- `docs/features/cindy-plugins/acceptance/step1-runtime.md`：临时项目、版本与真实调用证据；未安装实测时单独标记。

## 禁止

不直接打开项目 SQLite，不让调用参数决定任意命令，不把 Node 称为受项目沙箱限制的进程，不运行首次使用依赖安装，不改用户 Cindy/Codex 配置，不运行 Cargo/Rust。

## 完成标准

- 两个项目并发读取得到各自结果，同项目两个任务复用服务且不互相取消。
- 路径含空格/中文可用；拒绝缺失、非本地、伪造上下文及无权限写入。
- 覆盖 CLI 缺失/架构错误、过期发现信息、worker/子进程崩溃、请求取消、畸形协议、结果超限。
- 退出/停用不留下应回收的代理，也不结束无关 CLI/桌面进程；服务恢复后能重新建立连接。
- 正式/开发产物使用同一源码，默认环境和应用目录互不串用；开发与 Codex 开发客户端共享服务时，释放连接不误停另一客户端。真实双实例安装与同版本更新由 Step 6 验收，不以构建测试替代。
- 运行相关 Node 测试；只在修改 Go 共用部分时运行对应 Go 测试。Forge 包校验与宿主真实调用分别记录。
