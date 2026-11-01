# Step 1 · 运行时与 CLI 验收

日期：2026-11-01。范围：独立作者源码、运行时、自动化验证和 Forge 打包。用户明确不委托安装；未安装、未更新或卸载任何插件，未发布，未开始 Step 2。

## 版本与复现

- 执行电脑：当前 macOS arm64 本机；本机 Cindy `0.1.121`，实际构建 sourceCommit `0a7d5a419f5cd3e0fbebc9642dde8388376a68aa`。
- 官方 tag `v0.1.121` 指向 `727afb1ad6902176e2ec55254154b900c489a998`；实际构建 ref 查询 404。当前 Skill 与应用内 Skill SHA256 均为 `cbdaae022b89900b546d3664a647b8174e444fbd057d04b4a9cbdd4fdaa7f7cc`。精确源码与最低正式版本未验证。
- 本步实际 CLI 从 Lumi commit `0837609129513b6438dbf416490bb60675961d26` 编译，共用产品代码无未提交改动，Go `1.27.3`。本步不复用 Step 0 的旧二进制。
- 自动化 Node 为本机 `v25.2.1`；Step 0 实测 Host Node `v24.19.0` 是历史证据，本产品没有在 Host Node 中运行。测试替身不等于真实 Host。
- 当前二进制、测试结果和包指纹以 [自动化回执](step1-evidence/automated.json)、[打包回执](step1-evidence/forge-pack.json)、[包清单](step1-evidence/package.json) 为准。
- 本轮自动化共 34 项，通过 34 项、失败 0 项；包括宿主以 require 加载入口的回归与只读目录包含 data-dir 的副作用检查。[宿主核对记录](step1-evidence/host.json) 单独记录证据等级。

```sh
node cindy-plugins/scripts/build.mjs
node cindy-plugins/scripts/verify.mjs
# Cindy 公共 Forge 工具：ghost_forge_pack({dir: <repo>/cindy-plugins/lumi})
python3 cindy-plugins/scripts/verify-package.py
```

集成测试需要本机 loopback bind/connect。默认 Agent 沙箱中的首次集成运行不能启动服务；经正常执行审批运行同一测试后通过，没有变更 Cindy 批准配置。测试自行创建 `lumi-cindy-step1-*` 临时根与 owner 标记，使用独立 HOME、production data-dir、含空格/中文的两个项目。只有 fixture 初始化会调用原 CLI `init`，产品插件没有初始化工具。

## 自动化与实际 Go CLI

| 场景 | 结果 | 实际证据与边界 |
| --- | --- | --- |
| MCP 初始化与适配 | 通过 | 真实 `lumi mcp` initialize/initialized/tools/list/ping/overview；不是 Step 0 serve/status 通路的推断 |
| 双项目同时并发 | 通过 | Promise.all 实际发起两个 cwd 代理；项目名与 UUID 分离；同一 server nonce；worker 全局 cwd 不变 |
| 同项目双任务 | 通过 | 共用一个 MCP 子进程；释放一个任务后另一个仍用同 PID 读取 |
| 单请求取消 | 通过 | 真实 Go MCP 的 watch next 等待收到 notifications/cancelled 后删除订阅，close 返回 app_watch_expired；并发 overview 继续成功 |
| Host 哨兵取消桥 | 自动化通过 | 模拟 spawnEntry 子进程退出只取消自身调用，其他调用成功；缺绑定拒绝。真实 Cindy 取消未测 |
| 释放竞争与初始化竞争 | 通过 | 撤销早于异步等待；晚到回包不复活绑定；共享 initialize 中取消一方不影响另一方；目录读取中释放不产生晚到代理 |
| 只读与非本地 | 自动化通过 | 只读 runtime_status 前后项目文件 size/mtime 一致；overview 拒绝；非本地、缺失、嵌套伪造路径参数均拒绝。真实 Host 只读/远端上下文未测 |
| MCP 子进程崩溃 | 通过 | 杀掉测试拥有的代理 ChildProcess，下一调用建新 PID，server nonce 不变 |
| worker 崩溃与恢复 | 通过 | 独立测试 worker SIGKILL，真实 Go 代理通过 EOF 退出；server 保留；新 runtime 无旧任务绑定，凭新上下文重连 |
| server 崩溃与过期发现 | 通过 | 仅杀掉本次隔离实例；旧发现不阻止安全重启，新 nonce；Cindy 与普通 MCP 客户端重新读到同一项目 |
| Codex 共用服务兼容 | 自动化通过 | 使用现有普通 stdio MCP 调用方式，无 Cindy 上下文；Cindy 全释放后对方仍可 overview，server nonce 不变。不是实际 Codex App 交互验收 |
| 无法证明独占的实例 | 通过 | 产品无 server stop 路径；已有或首次观测的实例都按 shared_retained 保留；测试清理使用自己的临时 data-dir 和确切 nonce |
| 空闲与重绑定 | 通过 | 短化测试阈值后代理退出、任务绑定到期；新工具调用能重新连接；symlink 指向变化先拒绝再显式复核 |
| CLI 缺失/架构错误 | 通过 | 缺失路径与 build 平台不匹配均失败；随包 CLI SHA256/执行位由构建与包检查校验 |
| 协议与错误 | 通过 | 畸形 JSON、错误 requestId、反向请求、MCP error、无效信封、业务失败、不兼容版本与崩溃；未包装成 success:true |
| 大小/数量限制 | 通过 | 超大 params、超长响应、第 5 条在途请求；取消后的晚响应不串单、不截断 JSON |
| 写入结果未知 | 协议层通过 | 合成写请求超时仅发送一次并返回 result_unknown；没有暴露业务写工具，未宣称真实写入取消原子性 |
| 清理 | 通过 | 测试关闭拥有的代理，核对隔离 server nonce 后通过自身实例 DELETE 关闭，再核查 status stopped 并删除临时根 |

协议/生命期竞争用确定性 fixture；项目数据与实例恢复用真实本地 Go CLI。`tests/fixtures/worker.cjs` 明确模拟 Host spawnEntry，位于作者包之外，不进入 `.cindy`。自动化轮询只用于测试等待退出/就绪，不是产品业务同步；产品没有业务 HTTP 定时轮询。

## Forge 与真实宿主

| 项目 | 结果 |
| --- | --- |
| 当前 Host 只读版本和构建核对 | 通过，沿用本次会话核对值；不能代替产品运行 |
| Forge node-json-rpc scaffold | 通过，独立 `cindy-plugins/lumi/` 作者目录 |
| Forge schema/manual/node 校验与打包 | 通过；仅生成 `lumi-0.1.0.cindy` 开发包 |
| 包内容、CLI 指纹/执行位、文件数/体积 | 通过；详见包校验回执 |
| 产品安装/真实工具调用 | 未验证：本步明确不安装 |
| 两个真实 Cindy 任务同时并发、同项目双任务 | 未验证；本步提供自动化证据 |
| 真实 Host 取消、四个 childSpawn 并发与生命周期时序 | 未验证；依据随包契约设计，需后续授权安装后验收 |
| 真实只读/非本地任务、更新/停用/卸载 | 未验证；不得用 Step 0 探针或 Node mock 替代产品证据 |
| 其他架构、签名/公证、最低正式版本 | 未验证；当前只构建 macOS arm64 开发包 |

后台通知不能阻止按需 Node 回收、逻辑页 resident 不等于 Node 常驻、卸载不保证 detached CLI 退出，是 Step 0 已实测的边界。本步没有依赖这些错误假设，也没有复制探针的媒体、hooks、taskView 或创建任务权限。

## 共用代码与后续范围

共用文件改动：无。`cmd/`、`internal/`、`web/`、现有 `codex-plugins/` 均未修改。没有运行 Cargo/Rust；Go 仅用于制作随包 CLI，未修改 Go 因而未运行无关 Go 回归。实际 MCP 集成覆盖现有 CLI 初始化、读接口、发现/锁、取消与共享实例兼容；未把自动化客户端称为真实 Codex 宿主。

开发与打包交付完成后，本阶段可交付审阅；真实宿主验收保持待办，不据此开始 Step 2。正式发布前还须确认最低正式兼容版本，不能把 scaffold 自动写入的 `minCindyVersion` 当成已证明的最低版本。
