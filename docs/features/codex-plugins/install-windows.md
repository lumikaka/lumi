# Windows 原生 x64 安装预览

Windows CLI、PowerShell 安装器和自动化检查已加入源码。当前未完成 Windows Codex App 实机验收，尚不能宣称原生生图、MCP Apps 或项目保护已验证。只有包含 Windows Codex 资产的 Release 才能使用下面的下载入口；本地验证见后文，不从缺少资产的 Release 猜测下载地址。

适用 Windows 原生 x64 Codex App。WSL2、Windows ARM64、TUI/IDE 不在本期范围。安装无需 Go、Node、Rust、Git Bash、Lumi Desktop 或 API key；使用系统 Windows PowerShell 5.1 或 PowerShell 7。

## 一句提示词

> 请阅读 https://raw.githubusercontent.com/lumikaka/lumi/refs/heads/master/docs/features/codex-plugins/install.md，识别当前 Codex App 的 Agent 执行环境，按对应平台文档安装或更新 Lumi。Windows 使用原生 x64 预览流程；完成后报告实际 Codex 路径和版本、Lumi 版本、安装结果及仍待实机验证的能力，并说明如何新开会话开始创作。

若 Agent 在 WSL 中运行，停止这条安装路径，不把 Windows exe 注册进 Linux 配置。下载、写入用户安装目录、注册插件可能需要宿主批准；安装器不会设置信任或修改全局执行策略。

## 从 Release 安装

在 PowerShell 中固定版本并下载脚本及其校验文件：

```powershell
$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
$lumiInstallTemp = Join-Path ([IO.Path]::GetTempPath()) ('lumi-install-' + [guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($lumiInstallTemp)
$lumiBase = 'https://github.com/lumikaka/lumi/releases'
Invoke-WebRequest -UseBasicParsing "$lumiBase/latest/download/lumi-codex-version.txt" -OutFile "$lumiInstallTemp/version.txt"
$lumiVersion = [IO.File]::ReadAllText("$lumiInstallTemp/version.txt").Trim()
if ($lumiVersion -notmatch '^\d+\.\d+\.\d+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$') { throw 'Invalid release version' }
foreach ($lumiFile in @('install-codex.ps1', 'install-codex.ps1.sha256')) {
    Invoke-WebRequest -UseBasicParsing "$lumiBase/download/v$lumiVersion/$lumiFile" -OutFile (Join-Path $lumiInstallTemp $lumiFile)
}
$lumiExpected = [IO.File]::ReadAllText("$lumiInstallTemp/install-codex.ps1.sha256").Trim()
if ($lumiExpected -notmatch '^([a-fA-F0-9]{64})\s+\*?install-codex\.ps1$') { throw 'Invalid installer checksum' }
$lumiInstallerHash = $Matches[1]
if ((Get-FileHash -LiteralPath "$lumiInstallTemp/install-codex.ps1" -Algorithm SHA256).Hash -ne $lumiInstallerHash) { throw 'Installer checksum mismatch' }
Get-Content -LiteralPath "$lumiInstallTemp/install-codex.ps1"
```

阅读脚本后，再次核对 SHA256，并执行同版本安装器：

```powershell
if ((Get-FileHash -LiteralPath "$lumiInstallTemp/install-codex.ps1" -Algorithm SHA256).Hash -ne $lumiInstallerHash) { throw 'Installer checksum mismatch' }
& "$lumiInstallTemp/install-codex.ps1" -Version $lumiVersion
if ($LASTEXITCODE -ne 0) { throw 'Lumi installation failed; inspect the preceding error.' }
```

任一步下载或校验失败就停止，不继续执行遗留脚本。Windows CLI 首版未签名；SHA256 校验完整性，不替代可信发布来源或 Authenticode。执行策略或系统策略阻断时，按实际提示处理；不改脚本跳过检查，不关闭系统保护。

宿主发现顺序为显式 `-CodexPath`、当前进程祖先中的 Codex exe、唯一 PATH `codex.exe`。使用原生 exe，不接受 npm 的 `.cmd` / `.ps1` 包装器。安装器检查命令能力并记录真实版本，不猜 App/Store 安装目录。无法发现时传入已核实的 App CLI 路径：

```powershell
& "$lumiInstallTemp/install-codex.ps1" -Version $lumiVersion -CodexPath 'C:\实际目录\codex.exe'
```

成功必须同时满足退出码 0、最终 JSON `success=true`、`data.installed=true`、`data.enabled=true`，以及实际插件缓存验证通过。stdout 只输出 JSON，进度与错误在 stderr。结果含 `codex_path`、`codex_version`、`codex_home`、`lumi_version`、`cli_path`、`installed_path`、`new_chat_required`、`server_running` 和 `pending_cleanup`。`server_running=null` 表示未能核实后台状态，不代表已停止。

## 目录、升级和卸载

默认安装到 `%LOCALAPPDATA%\LumiCodex`，下载缓存为 `%LOCALAPPDATA%\LumiCodexCache`，也可指定绝对路径 `-InstallDir`、`-CacheDir`。安装器遵循当前有效 `CODEX_HOME`，未设置时使用宿主默认的 `%USERPROFILE%\.codex`。

安装目录包含归属标记 `.lumi-codex-install`、互斥锁文件 `.install.lock`、`install-state.json`、稳定的 `marketplace`，以及 `versions/<版本>-<内容摘要>/lumi.exe`。MCP 使用该 exe 绝对路径和 `mcp` 参数，不覆盖会话工作目录，也不修改 PATH。

升级重新执行同一安装流程。新版本不覆盖正在使用的 exe，保留当前与前一版本；更旧且未占用的版本才会清理。安装成功后必须新开 App 会话。旧后台服务不会被安装器停止；出现 `cli_upgrade_required` 时，协调同一数据目录的使用者，在工作结束后用安装结果中的 CLI 路径执行 `stop`，再重连。不要强杀服务或删除实例文件。

卸载前结束活动工作，读取当前安装记录：

```powershell
$lumiInstallRoot = Join-Path $env:LOCALAPPDATA 'LumiCodex'
$lumiState = Get-Content -Raw -LiteralPath (Join-Path $lumiInstallRoot 'install-state.json') | ConvertFrom-Json
& $lumiState.cli_path stop
if ($LASTEXITCODE -ne 0) { throw 'Failed to stop shared Lumi server' }
$lumiUninstaller = Join-Path (Split-Path $lumiState.cli_path -Parent) 'install-codex.ps1'
& $lumiUninstaller -Uninstall -InstallDir $lumiInstallRoot
```

自定义安装或宿主路径时沿用原参数。卸载只处理本安装器的插件注册和文件，保留绘本、production 应用数据 `%LOCALAPPDATA%\dev.lumi.Lumi` 和下载缓存。若旧会话仍占用 exe，`pending_cleanup` 列出残留；退出对应会话后用同一脚本再次 `-Uninstall`，或使用重新下载并校验的脚本完成清理。标记和锁文件保留以支持幂等重试。

注册或缓存验证失败会恢复旧 marketplace 与插件注册。若恢复也失败，保留 `.transaction`，修复报告的宿主错误后用相同目录、`CODEX_HOME` 和宿主路径重试；下一次会先恢复。不要手工删除事务、用户 `.codex` 或绘本数据。锁文件本身不代表正在安装，互斥由文件句柄控制，进程退出会释放。

## 当前源码的本地验证

这部分用于发布前验证，不要求普通用户编译。macOS/Linux 可执行：

```bash
make codex-package CODEX_VERSION=0.0.0-preview CODEX_OUT=build/codex-windows-local
make codex-windows-build CODEX_WINDOWS_OUT=build/codex-windows-local
```

将完整资产目录复制到 Windows；原生 Windows 也可用 `scripts/build-codex-windows.ps1` 构建 CLI，再合入同版本通用插件资产。必须保留 exe、ZIP、安装脚本、各自 `.sha256` 和 `lumi-codex-version.txt`。

```powershell
& 'C:\发布资产\install-codex.ps1' -FromDir 'C:\发布资产' -CodexPath 'C:\实际目录\codex.exe'
```

`-FromDir` 不访问网络，始终校验传入文件，不用网络缓存替代。正在执行的脚本也必须与资产目录的脚本 SHA256 一致。测试完成后按上节卸载；真实用户项目不用于自动化测试。

## 验收状态

本地检查与未完成项见 [Windows 实施验收记录](acceptance/windows-cli.md)。Windows App 新会话发现、真实宿主生图/路径导入、MCP Apps 审阅、写保护及真实升级卸载仍需实机确认；CI 的合成 PNG 与宿主替身不能替代这些结果。
