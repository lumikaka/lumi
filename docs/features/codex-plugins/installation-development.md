# Codex 插件开发、发布与安装验收

普通用户和安装代理从 [安装 Lumi](install.md) 开始。本文用于开发与发布验收，普通安装完成条件见安装文档。

## 本地开发

在 Lumi 仓库根目录执行 `make codex-dev`，从当前源码构建并安装 `lumi-dev@lumi-dev-local`。开发 CLI 为 `build/codex-dev/lumi_dev`，默认环境 `development`、应用数据 `~/.lumi_dev`，与正式插件并存，不需要发布 patch。结束开发任务后用 `make codex-dev-stop` 停止旧后台，再新开 Codex App 会话加载新构建。仅改 skill 或插件配置时执行 `make codex-dev-refresh`。

详细配置、卸载与旧插件迁移见 [Codex 插件开发说明](../../../codex-plugins/README.md)。`install-codex.sh --from-dir` 用于验证本地 Release 资产，仍管理正式插件身份；日常开发使用 `make codex-dev`。

## 发布与 CI

`make build` 继续构建 `lumi`，并增加插件包产出。`make codex-build` 构建带 web UI/MCP Apps 的本机 CLI；`make codex-release CODEX_VERSION=X.Y.Z` 构建两个 macOS 架构和以下资产，**没有 Cargo/Rust 命令**：

```text
lumi-darwin-arm64                 lumi-darwin-arm64.sha256
lumi-darwin-amd64                 lumi-darwin-amd64.sha256
lumi-codex-plugin.tar.gz          lumi-codex-plugin.tar.gz.sha256
install-codex.sh                  install-codex.sh.sha256
lumi-codex-plugin.zip             lumi-codex-plugin.zip.sha256
install-codex.ps1                 install-codex.ps1.sha256
lumi-codex-version.txt
```

沿用 `make tag-patch/minor/major → git push origin vX.Y.Z`，不由安装脚本创建或发布 Tag。[发布协调 workflow](../../../.github/workflows/desktop-release.yml) 增加 [Codex macOS assets job](../../../.github/workflows/codex-macos.yml)，通用插件资产由 `codex-package.yml` 单独生成，macOS job 只上传二进制；`codex-windows.yml` 消费同一份通用包并上传 Windows CLI。所有桌面和 Codex job 都成功后才发布完整 Release。Tag 版本写入包内 manifest 和版本文件，仓库模板不注入用户路径。

CI 顺序：构建 Go/web → Go 定向测试 → 两架构 codesign 与 verify → 可选 notarization → **重新计算签名后 SHA256** → 上传 artifact → 协调 Release 发布。没有证书时仅 ad-hoc 签名，不宣称 Apple 公证。

Developer ID 沿用 `APPLE_SIGNING_IDENTITY` repository variable，以及 `APPLE_CERTIFICATE_BASE64`、`APPLE_CERTIFICATE_PASSWORD` secrets。公证另设置 `APPLE_NOTARY_APPLE_ID`、`APPLE_NOTARY_PASSWORD`（app-specific password）、`APPLE_NOTARY_TEAM_ID` secrets；必须同时配置 Developer ID。CI 把已签名 CLI 打成临时 ZIP，`xcrun notarytool submit --wait` 后检查 `Accepted`，失败阻止发布，流程依据 [Apple 公证文档](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow)。独立 CLI 不执行 `.app` 的 stapling；签名、远程公证和 Gatekeeper 实机行为仍需真实发布验收。此改动没有本地执行 Cargo/Rust，也没有触发 CI 或发布 Release。

## 自动化验收与证据

先运行安装器的快速回归：

```bash
bash -n scripts/install-codex.sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/test-install-codex-unit.py
```

快速回归使用真实 macOS Bash、plutil、tar 和隔离的 Codex/curl 测试替身，覆盖无关市场失效、目标市场损坏、同名空市场冲突、下载前失败、缓存命中与损坏重下、注册失败重试、校验失败回滚及禁用状态。无网络、登录或模型调用；不能代替真实 App CLI 验证。

已有本机 Release 资产和 Codex App 时，可只验证真实 CLI 的安装生命周期：

```bash
PYTHONDONTWRITEBYTECODE=1 python3 scripts/test-install-codex.py --assets /绝对路径/发布资产 --install-only
```

该模式使用隔离配置，包含失效的无关市场，不启动创作服务、模型或 App UI。完整绘本自动化验收继续使用下面的默认模式。

在 macOS、有 Codex App、Go、pnpm/Node 和 Python 3.9+ 的开发环境运行：

```bash
make codex-release CODEX_VERSION=0.1.0 CODEX_OUT=/private/tmp/lumi-codex-assets
make codex-smoke CODEX_OUT=/private/tmp/lumi-codex-assets
# 可指定 PYTHON=/path/to/native/python3；建议与当前 Mac 原生架构一致。
python3 /Users/你的用户名/.codex/skills/.system/skill-creator/scripts/quick_validate.py \
  codex-plugins/lumi/skills/create-picture-book
```

最后一条是本机有该 Codex skill 校验器时的示例，不要求普通安装用户具备它。测试可能需要允许本机 loopback 监听；这与真实用户的一句话安装批准次数无关。

[`scripts/test-install-codex.py`](../../../scripts/test-install-codex.py) 创建临时 `CODEX_HOME`、含空格安装目录、空项目和独立 Lumi data-dir，不复用登录凭据、没有模型推理。测试两次安装结果相同、校验失败不覆盖旧版本、版本升级/同版本缓存刷新、无关配置保留、卸载与重复卸载。新 App `app-server` thread 依次调用 `project_init → project_setup → chapters → pages → image_target → import_image → export_book`，核对返回路径的 PDF 文件头、字节数和无缺图。

测试夹具只在临时生成的 `.mcp.json` 加入独立 `LUMI_DATA_DIR`，不加 cwd；重新 `plugin add` 后从实际插件缓存启动。测试图片是合成 PNG，**不能当作 App 原生生图端到端通过**。

每次运行打印新临时目录，其中 `report.json` 记录命令、stdout/stderr、codex 版本和 RPC 输入输出，`exported-book.pdf` 保存交付结果；测试结束停止自己的 CLI 并卸载自己的插件。检查证据后只清理本次临时目录。仓库验收记录见 [Step 6 自动化记录](acceptance/step6-install.md)。

本机没有发现独立 `codex plugin validate` 子命令：manifest 采用当前仍受支持的兼容目录格式，验证采用真实 App 插件加载、`skills/list` 无错误、MCP 工具发现及调用，另外运行 skill-creator 的 `quick_validate.py`。依据见 [OpenAI 插件打包文档](https://developers.openai.com/plugins/build/plugins)；本地命令行为以记录的具体版本为准。

## Codex App 人工验收（由用户执行）

**以下全部待人工确认**，不能用 app-server 冒烟或 Step 0/5 的旧样本代替。

| 项目 | 操作与要记录的证据 | 状态 |
| --- | --- | --- |
| 一句话真实安装 | 在用户 Codex App 执行[安装页提示词](install.md#一句提示词)；记录 App 版本、内置 CLI 路径/版本、Release 版本、每次批准的动作与界面截图，以及批准总次数 | 待人工确认；次数未知 |
| 新会话发现 | 安装后新开目录会话，确认 `lumi:create-picture-book` 与项目工具被模型识别 | 待人工确认 |
| 项目信任与保护 | 初始化后由用户信任项目；新开会话记录有效权限。用临时测试项目验证受管路径写保护，不让模型修改真实项目数据库 | 待人工确认 |
| 自主完整创作 | 用[安装页的小狐狸提示词](install.md#开始创作与续做)，让模型自行从想法完成 setup、故事、设定、分页、生图、导入和整本导出；记录各审阅节点，确认没有索取 API key | 待人工确认 |
| 原生生图导入 | 至少一张图由 App 原生生图产生；记录工具实际返回路径、随后 import_image 成功结果、页面/图片 UUID 和审阅截图 | 待人工确认 |
| 一致性与参考 | 记录 `read_image` 的设定参考、紧随其后的生图及实际画面效果 | 待人工确认 |
| 最终交付 | 在 App 打开全书审阅并检查 PDF/ZIP，记录页数、文字、图片完整性及文件路径 | 待人工确认 |
| 真实升级/卸载、Intel | 在相应机器复测新会话可用性及项目保留；记录 Gatekeeper、签名/公证状态 | 待人工确认 |

验收记录模板：日期/架构；App 版本；codex 路径与版本；Release；原始提示词；批准列表（序号、动作、截图、结果）；项目 UUID；至少一张原生图导入证据；PDF/ZIP 路径与页数；通过项与未通过项。批准数应填实际观察值，不能预填 0。

## Windows CLI 本地构建与测试

Windows 原生 x64 安装当前为预览，见 [Windows 安装指南](install-windows.md) 与 [实施验收记录](acceptance/windows-cli.md)。

macOS/Linux 使用 `make codex-windows-build` 交叉编译 `build/codex-windows/lumi-windows-amd64.exe`，再用 `make codex-package CODEX_VERSION=X.Y.Z CODEX_OUT=build/codex-windows` 生成完整离线资产。Windows 原生构建使用 `scripts/build-codex-windows.ps1`，它记录依赖、前端和 Go 编译的分段耗时。两条链路均无 Rust/Tauri。

自动化检查：

```powershell
# 在 Windows 分别用 powershell.exe 和 pwsh 执行；资产需包含 exe 及通用插件包。
./scripts/test-install-codex-windows.ps1 -Assets build/codex-windows
python scripts/test-codex-cli.py --binary build/codex-windows/lumi-windows-amd64.exe
```

安装器测试将宿主命令和下载替换为隔离夹具，覆盖真实文件操作、缓存、归属、事务回滚、重试和恶意 ZIP；Windows 额外使用真实文件占用句柄验证延迟清理。测试不会安装到用户 Codex。CLI 冒烟使用真正发布的程序，在临时项目验证后台单例、嵌入前端、stdio MCP、中文路径、页面准备签名、合成 PNG 导入、PDF/ZIP 导出与重启。标准 CI 不具备真实 App 的宿主能力时，不将这些测试称为 App 验收。

发布协调器继续等待 Desktop，因此 CLI 提前构建成功不代表提前公开发布。工作流支持 PR、手动预览和被发布协调器调用；手动构建不发布 Release。冷/热缓存耗时与 Windows 原生执行结果均须从实际 CI 记录，不从 macOS 交叉编译时间推算。
