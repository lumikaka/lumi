# 当前任务的项目绑定与初始化

Step 2，2026-11-01。作者源码为 `cindy-plugins/lumi/`，版本 `0.2.0`。在 Step 1 运行时上新增 `project_init`、`project_setup`，加强 `project_overview` 的绑定复核。沿用标准 Lumi 项目格式、Manager、锁、迁移和 revision；不改变 `format_version`。测试与构建结果见 [验收记录](acceptance/step2-project-binding.md)。

## 目录选择与工具

任务必须先在 Cindy 选择**本机项目根目录**。空目录或只有普通非冲突文件的目录可原地初始化；已有作品应选择直接包含 `project.sqlite` 的根。无目录时返回 `workdir_required` 并提示在 Cindy 选择，不创建默认作品目录。不存在的目录不自动创建，不向上搜索项目，不使用插件安装目录、PWD 环境变量或模型自报路径作为替代。远端、URI、相对路径、未知本地状态均拒绝，不映射成本机路径。

| 工具 | 输入 | 行为 |
| --- | --- | --- |
| `project_init` | 用户原始 `idea`，可选 `name`、`generation_language` | 复用 `project_init → Manager.InitTextAt`，原地创建标准 draft，再显式打开并返回现状 |
| `project_overview` | 无 | 打开当前目录的有效项目，返回身份、名称、项目 revision、setup 与数量 |
| `project_setup` | `action=get/update/finalize` 与已有业务字段 | 先读取并绑定真实项目；update/finalize 使用 setup `expected_revision` |
| `runtime_status` | 无 | 检查运行环境；只读模式只执行无业务副作用的 MCP 初始化与 ping |
| `runtime_release` | 无 | 撤销当前真实任务绑定、取消其在途请求、释放其代理引用 |

main.js 仅处理 Host `tool-call`，验证真实 `callId`、`args.session_context` 的任务、本地标记与当次只读状态，拒绝额外绑定参数。浏览器不具备 realpath 能力，因此在进入 Node 前完成上下文与绝对路径形状校验，Node 在启动业务 CLI 前完成 realpath、目录设备/inode 及当前 OS 读写访问检查。所有子进程固定 cwd，不调用全局 chdir。

## 初始化、清理与指引

Cindy 使用已有 MCP 业务初始化，不调用会补 Codex 指引的 `lumi init`。`cmd/lumi/project.go` 和 `guidance.go` 的默认行为及既有 Codex 指引原文均未修改。Cindy 不生成、补充或改写 AGENTS.md、.codex/config.toml、hooks、用户引擎目录；已有文件按原文保留。没有新增宿主模式开关，因为业务路径本身已具备副作用分离。

创作规约集中在插件 [项目 Manual](../../../cindy-plugins/lumi/manual/project/MANUAL.md)。Manual 是行为指导，不能证明系统写保护，不假定所有执行引擎读取同一文件。

`InitTextAt` 沿用 `.lumi` 独占创建、标准 SQLite 初始化、setup draft 和 Manager 打开过程。已有有效项目幂等返回；再次提供名称、语言或原始创意不会覆盖已有事实状态。非冲突文件及 `.git` 保留。无效项目中出现 README.md、STORY.md、project.sqlite、其 WAL/SHM/journal、.lumi 或 assets 时按原有规则拒绝，包括悬空符号链接。

失败回收沿用 `initializationLayout` 的本次创建记录和文件身份比较，不递归删除目录，不清理用户已有文件，也保留初始化期间由其他写入者新增或替换的文件。中途取消、崩溃或无法核对结果时返回 `result_unknown`；业务可能已提交，先读取事实状态，不自动重试写入。同一 worker 的同根初始化排队，两个任务首次并发初始化可以返回同一项目；不同根仍并发。

## 绑定与权限

仅可信工具调用可建立绑定。内存中分别记录：

- 真实任务 `session_id`、规范根与目录设备/inode、本次权限快照；
- 项目公开 UUIDv7、数据库文件设备/inode；
- 固定 cwd 的 CLI MCP 连接、构建 SHA256、environment 与规范 data-dir；
- 经在线查询核对的 CLI 实例 endpoint/PID/nonce。

这些内部字段不返回 Agent 或页面。工具结果保留 Lumi `success/data/error` 信封；成功概况采用公开字段允许列表，含 UUID、名称、setup 状态与 revision、必要限制。业务失败保留错误 code，使用本地适配层的恢复提示，避免将 OS 路径、endpoint、锁元数据或 nonce 带出。项目 revision 与 setup revision 分开返回。

绑定按任务保存，连接按规范根及运行实例配置共享，不能仅按 UUID 建索引。两份复制项目即使 UUID 相同，也不会合并目录；Manager 对已从另一目录打开的同 UUID 返回 `project_identity_mismatch`。同一项目可有多个独立任务引用，释放一个任务不撤销其他任务。

每次调用都检查当前 Host 只读标记，不能由缓存恢复写权限。OS access 检查只验证当次可访问性，不代替 Host 权限。`project_init`、概况、setup get/update/finalize 均要求可写：打开可能迁移数据库、写锁/WAL 和运行状态。只读任务可诊断与释放，不能假称当前实现支持完全无副作用的作品打开。Node 仍是拥有本机账号权限的可信原生代码，此适配不是操作系统沙箱。

setup 调用先通过当前 cwd 读取项目，再以 MCP `_meta.lumi/expected_project_uuid` 传递附加身份约束。CLI 将它转为内部 REST `expected_project_uuid`；服务在初始化/打开前核对根与 UUID，并在 Manager lease 内再次核对 store 根和 UUID，避免 OpenSelected 与 WithStore 之间关闭/重开的切换。该字段只能收紧请求，不能选择目录或建立授权。现有 Codex 调用可省略，原工具参数和默认初始化路径不变；不支持该字段的旧共享 server 会拒绝请求而不是忽略约束执行写入。

服务端 Store 还在打开时记录真实目录与数据库的文件身份。每次业务调用在 Manager lease 内核对当前目录、数据库和实际持有的锁文件，拒绝将同一路径、相同 UUID 的新副本误认为仍打开的原件。此检查在新任务或 worker 重启后仍生效，不依赖 Node 的旧缓存；只读取文件元数据，不直接读取或改写 SQLite 内容。

## 目录变化与重启

任务目录的符号链接先解析为规范根；同一真实目录的别名可以共享连接。任务换目录、链接重定向、目录被替换或数据库文件身份变化，先拒绝并撤销当前任务旧绑定。调用前后再次校验目录与文件；结果跨 CLI 实例代次时拒绝确认，写操作报告结果未知。用户核对 Cindy 当前目录后，新的显式工具调用才重新建立绑定。

如果原件仍被 CLI 打开，却已从磁盘移走或被副本替换，仅释放任务连接不足以修复服务端文件身份不符；应先恢复原路径，或协调持锁实例释放后再打开目标副本。插件不会为重绑而替其他客户端关闭项目，也不会把相同 UUID 当成替换文件的许可。

绑定不写 KV 或磁盘。worker 重启、插件更新或空闲回收后，从下一次 Host 可信上下文重新取目录与权限，并在线发现 CLI 实例、读取项目 UUID。页面路由、缓存 UUID、模型自报 session/project 标识均不能恢复授权。已打开的共享 server 可以保留，新的 worker 不将它视为独占。

## 项目锁、关闭任务与桌面切换

只有 CLI server 的 Manager 持有项目锁；worker/MCP 代理不另造项目锁，不直接读写 SQLite。桌面占用时返回 `project_locked`，提示在桌面关闭该项目或退出占用实例，绝不删除锁文件或抢锁。

`runtime_release` 立即撤销当前任务并取消它的在途调用；结束调用/关闭任务的 Host 取消沿用 Step 1 哨兵。没有可信 task-close 主动绑定入口，空闲任务引用约 60 秒过期，代理约 30 秒回收；不会通过页面关闭事件新建或恢复绑定。释放仅影响当前任务，不清空项目、不停止其他任务服务。

Cindy → 桌面：释放任务连接，让同项目其他任务停止使用，等待 CLI 约 15 秒的项目空闲释放机制，再在桌面选择该根目录。释放任务回执不代表共享项目锁已经释放。桌面 → Cindy：在桌面关闭作品，Cindy 选择同一根后读取概况或调用幂等初始化。用户确需停止 CLI 时应先协调其他客户端，插件不自动 stop 共享服务。

## 验证边界

已以隔离 HOME、临时项目与真实随包 CLI 验证本机适配；Go 测试覆盖桌面 Manager 双向打开、锁和失败清理。没有运行 Cargo/Rust，没有安装或发布插件。真实 Cindy 产品工具调用的只读/非本地权限切换、取消与任务关闭的宿主时序仍属安装后验收，不能用测试中的上下文注入替代真实 Host 证明。

执行宿主为 Cindy 0.1.121，应用包记录 sourceCommit `0a7d5a419f5cd3e0fbebc9642dde8388376a68aa`。本次再次读取版本和构建来源；官方精确 ref 仍未能取得，不以不匹配的 tag/main 替代。实现依据为该宿主随包 sessionContext、Node 和调用权限契约及既有 Step 0 实测；最低正式兼容版本的证据仍遵循 [Step 1 限制](runtime.md#宿主依据)。
