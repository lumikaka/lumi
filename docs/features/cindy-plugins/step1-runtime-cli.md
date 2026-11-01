# Step 1 · 插件运行时与 Lumi CLI

## 目标

在 Cindy 内建立可靠的 Lumi 工具调用通路，复用现有 CLI/server，让多个任务能够访问各自项目，且插件进程退出不会误停其他任务的服务。

## 前置

阅读 [总体方案](plan.md)、[开发依据](references.md) 和 Step 0 产出的 `spike.md`。Node/CLI 与上下文门槛须通过。实现前按开发 Skill 对齐仍未确定的设计；不重新询问已有明确选择。

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

## 与现有实现的关系

复用 `cmd/lumi` 的发现文件、实例身份验证、启动竞争处理及默认 production 环境。现有 server 缺少所需只读能力时补最小兼容扩展；不改桌面 token 鉴权，不借用旧 OAuth MCP，也不重写 `internal/textmcp` 的业务逻辑。

## 交付

- 插件骨架、worker 连接管理与针对生命周期/并发的测试。
- `docs/features/cindy-plugins/runtime.md`：实际消息链、参数与大小限制、服务发现、生命周期、错误与恢复。
- `docs/features/cindy-plugins/acceptance/step1-runtime.md`：临时项目、版本与真实调用证据；未安装实测时单独标记。

## 禁止

不直接打开项目 SQLite，不让调用参数决定任意命令，不把 Node 称为受项目沙箱限制的进程，不运行首次使用依赖安装，不改用户 Cindy/Codex 配置，不运行 Cargo/Rust。

## 完成标准

- 两个项目并发读取得到各自结果，同项目两个任务复用服务且不互相取消。
- 路径含空格/中文可用；拒绝缺失、非本地、伪造上下文及无权限写入。
- 覆盖 CLI 缺失/架构错误、过期发现信息、worker/子进程崩溃、请求取消、畸形协议、结果超限。
- 退出/停用不留下应回收的代理，也不结束无关 CLI/桌面进程；服务恢复后能重新建立连接。
- 运行相关 Node 测试；只在修改 Go 共用部分时运行对应 Go 测试。Forge 包校验与宿主真实调用分别记录。
