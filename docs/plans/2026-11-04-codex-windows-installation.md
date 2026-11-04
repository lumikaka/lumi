---
plan_state: implemented_locally
git_commit_message: "feat(codex): support native Windows plugin installation"
---

# Windows Codex App 一句话安装 Lumi 方案

起草日期：2026-11-04。状态：源码已在当前分支实施并进行本地验证；Windows 原生 CI 与 App 实机验收待执行，未推送或发布。详见 [实施验收记录](../features/codex-plugins/acceptance/windows-cli.md)。

## current_status

目前 macOS 已有一句提示词安装入口：Agent 阅读安装指南，下载安装脚本、预编译 CLI 与插件包，校验后注册本地 marketplace，用户新开会话使用插件。

可复用的基础：

- [`cmd/lumi`](../../cmd/lumi/main.go) 已提供 `serve/status/stop/init/open/mcp` 等命令，stdio MCP 按会话工作目录绑定项目，自动连接或启动本地服务。
- Windows 已有后台进程分离、文件锁、应用数据目录、项目目录和字体路径适配。
- [`desktop-windows.yml`](../../.github/workflows/desktop-windows.yml) 已配置 Windows Go 全量测试，并用 `CGO_ENABLED=0` 构建带前端的 `cmd/lumi_web`；这不等于插件 CLI 的 Windows 安装验收已经完成。
- [`Makefile`](../../Makefile) 的 `codex-release` 只输出 macOS 两种架构；[`install-codex.sh`](../../scripts/install-codex.sh) 限定 macOS，并依赖 `plutil` 与 Mac App 路径。
- [`desktop-release.yml`](../../.github/workflows/desktop-release.yml) 在全部 Desktop、Codex 构建成功后发布完整 Release。

Windows Desktop 的总耗时包含 Go 全量测试、前端构建、Rust 启动器两组测试、Tauri/NSIS 打包及安装卸载验收。插件 CLI 只需要 Go、Web 资源和对应验收，预期构建链路更短；冷缓存、热缓存的实际耗时仍需记录，不预设分钟数。

## overview

### 目标与首版范围

让 Windows 用户在 Codex App 中粘贴一句提示词，即可完成 Lumi 插件安装、更新和状态验证。用户只下载预编译文件，无需安装 Go、Node、Rust、Git Bash 或 Lumi Desktop。

首版支持 Windows 原生 x64 Agent、本地 Windows 项目目录、正式插件 `lumi@lumi-local`。最低系统版本沿用经核实的宿主要求，并记录实测系统及 App 版本。WSL2、Windows ARM64 原生包、TUI/IDE、开发版安装入口、npm/winget 分发另行安排。

沿用现有 Go CLI、SQLite 项目格式、Web 前端和 Codex 插件源码，不增加 Windows 专属业务实现。安装适配放在平台脚本，前端默认外观、路由、交互和功能保持不变；若实现中修改共享代码，必须检查受影响的桌面 Web 页面。

不增加业务 HTTP API、数据库迁移或定时状态轮询。运行时继续遵守现有 REST、WebSocket 和公开 UUID 契约。

### 关键决策

| 项目 | 本期方案 |
| --- | --- |
| 运行程序 | 从 `./cmd/lumi` 构建 Windows x64 exe，嵌入完整 Web/MCP Apps 资源 |
| 安装入口 | 新增 `scripts/install-codex.ps1`，兼容 Windows PowerShell 5.1，并覆盖 PowerShell 7 |
| 插件来源 | 复用 `codex-plugins/lumi`；生成 Windows ZIP 与现有 macOS tar.gz |
| 安装范围 | 当前用户目录，无需管理员权限，不依赖 PATH 改动 |
| 升级方式 | 不可变版本目录、稳定 marketplace 路径、注册失败回滚、旧程序延迟清理 |
| 构建 | 独立 Windows CLI job，与 Desktop 并行；无 Cargo、Rust、Tauri、NSIS 步骤 |
| 发布 | 首版沿用现有 Tag 与完整 Release；独立插件发布通道不在本期引入 |

## commands

### Windows CLI 构建

在 Windows CI 中安装仓库锁定的构建依赖，然后执行：

```powershell
pnpm --dir web run build
if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed.' }

$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build -trimpath -tags embed_frontend -ldflags='-s -w' `
  -o build/codex-windows/lumi-windows-amd64.exe ./cmd/lumi
if ($LASTEXITCODE -ne 0) { throw 'Windows CLI build failed.' }
```

构建步骤应先创建输出目录。Windows 任务直接使用 PowerShell，不要求 runner 安装 GNU Make。先验证现有依赖在该目标上成功构建和启动，再对外声明支持。

### 安装器参数

计划提供以下参数，语义与 macOS 安装器对应：

| 参数 | 作用 |
| --- | --- |
| `-Version X.Y.Z` | 指定发布版本；省略时只解析一次最新版本，后续下载锁定同一 Tag |
| `-InstallDir ABS_PATH` | 默认 `%LOCALAPPDATA%\LumiCodex` |
| `-CacheDir ABS_PATH` | 默认 `%LOCALAPPDATA%\LumiCodexCache` |
| `-CodexPath ABS_PATH` | 显式选择宿主兼容的 Codex 可执行文件 |
| `-FromDir ABS_PATH` | 使用本地发布资产，供离线安装与验收，仍校验全部文件 |
| `-Uninstall` | 注销插件并清理本安装器管理的文件，保留项目和应用数据 |

进度和诊断写 stderr，成功结果在 stdout 输出统一 JSON 信封，字段使用 `snake_case`。记录 `plugin_id`、`lumi_version`、`codex_path`、`codex_version`、`codex_home`、`cli_path`、`installed_path`、`installed`、`enabled`，并单独说明是否需要新会话、旧服务是否仍在运行。失败返回非零退出码与明确错误阶段。

脚本须显式检查原生命令的退出码；不能只依赖 `$ErrorActionPreference`。JSON 用序列化器生成，覆盖反斜杠、空格和中文路径，使用明确的 UTF-8 编码。

## installation

### 宿主探测

安装前确认 Agent 运行在 Windows 原生环境，并识别系统架构。不能只根据 App 所在系统决定下载目标；处于 WSL2 时报告当前不支持该安装路径，不自动混用 Windows exe 与 Linux 配置。

优先使用显式 `-CodexPath`，其次使用经过实机核实的 App 内置 CLI 发现方式，最后尝试 PATH。不得写死未经验证的 Windows App 安装路径；候选有歧义时要求明确指定。

核对实际版本及 `plugin marketplace add`、`plugin add`、`plugin list`、卸载命令能力。解析当前有效 `CODEX_HOME`，默认 Windows 用户目录下的 `.codex`，记录后沿用；不得重置用户已有配置。

与 macOS 保持相同的市场冲突语义：只处理 Lumi 的本地来源，验证安装目录归属；无关市场损坏不阻断 Lumi，目标市场损坏或同名异来源冲突必须在修改注册前失败。

### 安装文件布局

```text
%LOCALAPPDATA%\LumiCodex\
  install-state.json
  versions\
    <version>-<payload_digest>\
      lumi.exe
      install-codex.ps1
  marketplace\
    ...生成的本地 marketplace 与插件源码
```

`payload_digest` 由该版本的 CLI 与插件包校验值确定，防止重复安装时覆盖被占用的 exe。安装状态记录格式版本、所属 `CODEX_HOME`、当前/前一份程序与插件信息、marketplace 路径和未完成事务；不保存凭据。

安装目录与现有 Windows Desktop 安装目录分开。正式 CLI 沿用现有 production 应用数据目录及项目锁语义；不另建 Windows 插件专属项目数据库。MCP `command` 指向版本目录内 exe 的绝对路径，`args` 保持 `["mcp"]`，不设置 `cwd`。

### 下载与安装事务

1. 完成宿主、路径、目录归属和市场冲突检查，获取安装锁。
2. 从同一个不可变 Tag 下载脚本、CLI、插件包及对应 SHA256；安装指南先校验脚本，再运行它。缓存每次命中也重新校验，损坏时重下；离线输入必须校验原文件。
3. 在临时目录解包并检查 manifest 版本、技能引用和 MCP 占位符。ZIP 必须保留 `.codex-plugin`、`.mcp.json` 等点文件；拒绝越界条目与链接，不在下载完成前改动现有安装。
4. 将验证过的程序放入不可变版本目录，在临时目录生成指向该程序的 marketplace，并运行 CLI `--help` 验证目标程序可执行。
5. 备份本安装器管理的 marketplace 与状态，切换到稳定 marketplace 路径，再调用宿主注册及安装命令；不替换整份用户 Codex 配置。
6. 从宿主实际安装缓存核对版本、启用状态、技能文件、MCP 绝对路径与参数，验证通过后提交状态。
7. 注册或验证失败时恢复旧 marketplace 和注册；首次安装只撤销本次新增内容。恢复失败需保留事务证据并输出恢复步骤，不能宣称已回滚成功。

并发安装应明确失败；中断残留需通过状态识别并恢复，不能以删除整个 `.codex` 作为修复方式。同版本重复安装复用验证过的程序，并刷新、核对插件缓存。

### 升级与卸载

新版本写入独立目录，不覆盖或立即删除运行中的 exe。插件缓存切换成功仅代表安装完成，不代表当前会话或后台服务已经升级。

保留现有 `cli_upgrade_required` 协议检查；遇到旧服务不兼容时，先协调同一 environment/data-dir 的使用者，完成活动工作后执行对应的 `lumi stop`，再新开会话启动新服务。安装器不强杀共享服务，不自动重放业务写入。

旧程序在相关进程退出后清理，至少保留回滚所需版本。卸载先核实来源并注销插件，项目、应用数据和缓存默认保留；仍被占用的文件记录为待清理，输出实际残留及处理方法，不能报告全部删除成功。

首版按未签名 Windows CLI 交付并如实说明状态；SHA256 负责下载完整性校验，不等同于代码签名。遇到宿主或系统策略阻断时报告具体错误，不由安装器修改全局执行策略或关闭系统保护。签名支持可后续独立补充。

## ui

官网与安装文档保留统一的一句提示词，由指南识别执行环境后分流：

> 请阅读 Lumi 安装指南，识别当前 Codex App 的运行环境，为我安装或更新 Lumi 绘本插件；完成后报告实际使用的 Codex 路径和版本、Lumi 版本、安装结果，以及新开会话开始创作的方法。

正式文案使用现有安装指南 URL。安装结果继续说明需要新开项目会话、首次项目保护需由用户信任并验证，不承诺零批准或热加载。只有 Windows 宿主验收通过后，才将插件 manifest、Skill 和中英文站点文案中的 macOS 限制扩展为已验证的平台。

## release

### 资产与 CI

新增以下资产，其余 macOS 资产名称和安装路径保持兼容：

```text
lumi-windows-amd64.exe           lumi-windows-amd64.exe.sha256
lumi-codex-plugin.zip            lumi-codex-plugin.zip.sha256
install-codex.ps1                install-codex.ps1.sha256
```

继续共用 `lumi-codex-version.txt`。扩展 `scripts/package-codex.mjs`，从同一插件源码生成两种归档，保证解包后的逻辑内容一致。通用包和安装脚本只由一个明确的打包任务产出；各平台任务消费同一份资产，避免 Release 汇总时同名文件互相覆盖。

新增 `.github/workflows/codex-windows.yml`，使用 Windows runner，支持 PR 验证、手动验证及发布协调调用。执行依赖安装、前端构建、Go CLI 构建、定向测试、安装器测试和运行冒烟，生成 SHA256 与耗时记录后上传 artifact。

扩展发布协调器的 job 依赖、下载步骤、必需资产清单、版本校验与 Release 说明。首版保留完整 Release 门槛和已发布 Tag 不可变规则；Windows CLI artifact 可以较早完成，但公开发布仍需等待 Desktop。后续若要独立发布插件，另行设计版本入口及完整性规则，不能提前发布缺少其他平台资产的现有 Latest Release。

### 构建耗时与公共检查

- 复用 Go 模块/构建缓存和 pnpm 缓存，缓存按系统、架构、工具链与依赖锁文件隔离。
- 单独记录依赖准备、前端构建、Go 编译、测试与打包耗时，以及缓存命中情况，区分排队时间、执行时间和最终发布等待时间。
- Windows CLI job 不重复 Desktop 的 Rust、NSIS 和桌面验收。现有 Windows Go 全量测试及前端检查继续作为合并/发布门槛；本期不为缩短时长删除公共测试。
- 安装器和 CLI 专项测试独立执行。若以后拆分公共检查，必须明确依赖同一 commit 的检查结果，不能仅假定另一条流水线曾通过。
- 至少比较一次冷缓存和一次热缓存记录，再决定是否需要共用前端构建 artifact 或改用交叉编译；首版优先直接在 Windows 构建并运行产物。

## acceptance

### 自动化

| 范围 | 必须覆盖 |
| --- | --- |
| 包契约 | ZIP/tar.gz 内容对应、点文件和技能引用完整、版本一致、恶意归档条目被拒绝 |
| 安装事务 | 首次/重复安装、指定版本、离线输入、缓存命中与损坏、SHA256 失败、市场冲突、禁用策略、注册失败回滚、并发与中断恢复 |
| Windows 路径 | 中文用户名、空格路径、JSON 转义、不同有效 `CODEX_HOME`、原生架构识别 |
| 升级卸载 | 旧 MCP/后台进程仍存活时安装新版本、旧程序保留、新会话切换、协议不兼容提示、保留项目、重复卸载、被占用残留报告 |
| CLI/MCP 运行 | 使用真正发布的 exe，验证 `serve/status/stop`、stdio 工具发现、项目绑定与锁释放、创建文本、导入测试图、PDF/ZIP 导出 |
| 回归 | macOS 安装与打包测试继续通过；共享代码受影响时检查对应桌面 Web 页面 |

安装器测试在 PowerShell 5.1 与 7 执行，使用临时安装目录、临时配置、独立 Lumi data-dir 和宿主测试替身。真实发布 exe 的运行冒烟不使用用户项目或生产数据。替身通过只能证明安装逻辑，不能证明真实宿主插件可用。

在有真实 Codex App CLI 的 Windows 环境补充隔离配置下的插件注册、缓存加载与 MCP 调用验收。标准 CI 若不具备该宿主，不伪装为通过，应附真实环境的版本与验收记录。

### Windows App 实机发布门槛

用原生 x64 Agent 在本地项目目录完成一句话安装、新会话加载、项目初始化与写保护验证、必要设定图与至少一页原生生图、参考图准备/读取、图片路径导入、MCP Apps 审阅、PDF/ZIP 导出，再验证升级和卸载后的项目保留。

记录 Windows/App/CLI 版本、实际程序路径、Release 版本、批准步骤、宿主图片路径、工具调用结果、审阅截图和导出文件。中文路径至少覆盖一次。确认后台启动不产生影响使用的额外控制台窗口。

上述能力通过前，Windows 支持保持开发/预览状态。模拟图片、注册成功或 `--help` 可执行均不能替代原生生图与交互审阅闭环。

## implementation

1. **验证现有 Windows CLI 与宿主。** 从当前源码构建 exe，验证后台进程、MCP、目录绑定；核实真实 App CLI 发现方式、注册命令、图片路径和 MCP Apps 能力，记录差异。若发现缺口，先补 Windows 专属适配。
2. **增加分发资产和独立 CI。** 扩展打包器与包契约测试，产出 Windows exe、ZIP、脚本及 SHA256；接入版本一致性检查，记录冷/热缓存耗时。
3. **实现 PowerShell 安装生命周期。** 完成探测、校验缓存、安装事务、版本目录、幂等更新、回滚与卸载，补充专项测试。
4. **接入完整 Release 并验收。** 更新发布资产清单，跑通真实 exe 和真实 App 安装创作闭环，保留证据。
5. **开放统一安装入口。** 更新安装指南、开发验收说明、插件 manifest/Skill、中英文官网安装页及相关 README，明确平台范围与未覆盖项。

预计涉及：`scripts/install-codex.ps1`、Windows 安装器测试、`scripts/package-codex.mjs` 及测试、`.github/workflows/codex-windows.yml`、通用打包任务与发布协调器、`docs/features/codex-plugins/` 安装文档、`codex-plugins/lumi/` 平台文案、`site/content/docs/codex-installation.*.md` 及其他 macOS 限定说明。运行时代码仅在验证发现实际兼容问题时修改。

## references

- [现有安装流程](../features/codex-plugins/install.md)、[开发与发布验收](../features/codex-plugins/installation-development.md)。
- [OpenAI Windows App 文档](https://learn.chatgpt.com/docs/windows/windows-app)：原生 PowerShell、插件/技能与 WSL2 环境边界；具体 Lumi 能力仍需实测。
- [Microsoft DeleteFile 文档](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-deletefile)：打开句柄和映射文件对清理操作的约束。
- [OpenFilm](https://github.com/openfilm/openfilm)、[HyperFrames CLI](https://hyperframes.app/docs/5-packages/cli)：参考 Agent 驱动安装和 CLI 使用体验；Lumi 沿用原生 Go 分发。
