# Step 2 验收：当前任务项目绑定与初始化

日期：2026-11-01。执行平台：macOS arm64，Go 1.27.3，Node v25.2.1。当前作者包 `lumi 0.2.0`；只开发、测试和 Forge 打包，未安装、未发布，未运行 Cargo/Rust。实现说明见 [project-binding.md](../project-binding.md)。

## 可复现结果

```text
node cindy-plugins/scripts/build.mjs
  CLI build: success, go1.27.3 darwin/arm64
node cindy-plugins/scripts/verify.mjs
  tests 47; pass 47; fail 0; skipped 0
go test ./internal/textmcp ./internal/project ./cmd/lumi ./internal/httpapi
  ok lumi/internal/textmcp 6.083s
  ok lumi/internal/project 19.792s
  ok lumi/cmd/lumi 8.392s
  ok lumi/internal/httpapi 20.631s
ghost_forge_pack
  ok: true; lumi-0.2.0.cindy
python3 cindy-plugins/scripts/verify-package.py
  PASS: 14 archive entries, manifest, source bytes, CLI SHA256/mode, size limits; no installation
git diff --check
  exit 0
```

本次复查补充了同路径、相同 UUID 的副本替换场景：先以失败测试证实旧 Store 被复用，再增加目录、数据库和持锁文件的真实身份校验。三种替换的服务端测试与真实 CLI 新任务重绑测试均通过；同时重新完整运行上述四个 Go 包，未使用缓存结果。

记录：[Node 全量结果与 CLI 构建信息](step2-evidence/automated.json)、[Go 测试输出](step2-evidence/go-tests.txt)、[Forge 回执](step2-evidence/forge-pack.json)、[包字节校验](step2-evidence/package.json)。

包 SHA256：`cf61f74ea0d0e3f092df3e82dbc7fbfc4c91b5033ede246c7d7338558bd532e8`。CLI SHA256：`d548db0a9cbef5fb9d8332de80a5247ac1a5df58b1f930d773d294e8d4a39191`。CLI 基于仓库 commit `1b6b988ddfa36cbdd5d5bcb06d956575207e880f` 加本次未提交变更，build-info 明确 `source_dirty=true`。生成物沿用 Git ignore；源码及验收记录保留在仓库。

## 完成标准对应证据

| 场景 | 验证与结果 |
| --- | --- |
| 空目录、普通非冲突文件 | 真实 Runtime → MCP → CLI 初始化两个项目；普通笔记、.git、AGENTS.md、.codex/config.toml、用户 hook 文件原样保留 |
| 无目录、非本地、只读拒绝 | 浏览器适配测试在 Node 前拒绝；Runtime 再次检查当前上下文；真实 OS chmod 无写权限目录拒绝，未生成项目文件 |
| 已有项目、重复初始化 | 完成 setup、写入章节和角色设定后重复 init；名称、UUID、revision、最终规格、章节与素材记录不变 |
| 冲突文件、悬空链接 | 真实 CLI 对全部八个受管路径冲突拒绝；保留原文件；悬空 README.md 链接同样拒绝 |
| 中途失败清理 | `TestTextDraftInitializationRollbackAndDesktopRoundTrip/rollback` 在已建立 draft 的 open hook 注入失败，验证文件快照恢复、AGENTS 原文和无遗留 Manager 绑定；原 InitAt migration/hook/recent-index 与并发替换文件清理测试继续通过 |
| 符号链接与目录变化 | 真实项目别名正常打开；别名切换到另一项目或当前工作目录改变时先 `binding_changed`，新的可信调用才可重绑 |
| 同项目双任务、不同项目并发 | 两项目同时初始化 UUID 分离；同根两个首次 init 返回同 UUID 与 setup；释放一个任务保留另一个任务及共享 server |
| 复制项目相同 UUID | 在临时目录复制已关闭作品；原目录打开时副本返回 `project_identity_mismatch`，不建立副本项目身份绑定 |
| 同路径副本替换 | 服务端在每次业务调用的 lease 内校验真实目录、数据库和持锁文件；同路径同 UUID 副本也拒绝。Go 覆盖三类文件替换，真实 CLI 覆盖新任务打开/更新拒绝且恢复原路径后正常读取 |
| 身份不符写入拒绝 | 服务端在 init/setup/overview 前拒绝不匹配 UUID；空目录携带旧 UUID 不创建文件；真实 MCP 元数据传递也已验证 |
| 重启与缓存 | worker/runtime 重建后绑定为空；只读上下文仍拒绝，当前可信可写上下文重新读取相同项目；Step 1 SIGKILL、失效实例与代理恢复回归通过 |
| 桌面重开与双向切换 | Go 测试证明 Cindy draft 可经桌面 Manager 重开且 format_version 与原始创意保持；桌面新建作品可被 Cindy 业务初始化幂等接纳并打开，保留 ready 规格 |
| 桌面锁 | Manager 持锁测试及 `TestMCPDesktopLockAndReleaseGuidance` 通过；桌面释放前 `project_locked`，释放后可打开；适配层保留 code 并返回桌面释放指引 |
| Codex 指引兼容 | `cmd/lumi` 全套包含原 `lumi init`、guidance 配置保留及 MCP 测试；`guidance.go`、`project.go` 与 `codex-plugins/` diff 为空；Cindy 新建项目不含 Codex/其他引擎配置 |
| 公开字段与统一信封 | 成功返回 UUIDv7、名称、revision、setup 与限制；内部路径、数字 id、endpoint 不出适配层；失败保留统一信封与业务 code |
| 打包与清理 | Forge 校验成功；包内文件逐字、CLI SHA256 和执行位复核；测试仅停止自己 nonce 对应的隔离实例并删除临时项目，未操作默认生产实例 |

具体 Node 用例在 `cindy-plugins/tests/project-binding.test.cjs`、`main.test.cjs`，生命周期与传输沿用完整 Step 1 套件；Go 新增测试在 `internal/project/init_text_test.go` 与 `internal/textmcp/binding_test.go`。

## 证据边界

本次读到执行宿主版本 Cindy 0.1.121，应用包记录 sourceCommit `0a7d5a419f5cd3e0fbebc9642dde8388376a68aa`。精确公开源码未取得；不以另一个 tag 或 main 代替。依据当前随包 Skill 和 Step 0 既有宿主实测完成开发；`minCindyVersion` 仍是开发包字段，不是最低正式兼容版本证明。

上述 Node 测试的 Host 上下文由测试注入，浏览器 main.js 用 VM 执行；CLI、SQLite 业务、锁和桌面 Manager 为真实代码。未安装产品插件，所以不声称真实 Cindy 工具调用的只读/远端上下文、宿主取消、任务关闭、升级和卸载时序已在本产品复测。真实宿主安装验收继续沿用 Step 1 待验项，不影响本步源码、CLI 互操作与开发包的可复现交付。
