# 在 Codex App 中安装和使用 Lumi

Lumi 的新 `lumi` 插件通过本地 stdio CLI 管理绘本，写作和生图使用 Codex 自身额度，**无需 API key**。项目保存在本地；需要生成的文字和参考图片仍会交给 Codex，不能把“本地保存”理解为离线推理。

正式入口支持 **macOS 上的 Codex App**，提供 Apple Silicon（arm64）和 Intel（amd64）CLI。新增 [Windows 原生 x64 安装预览](install-windows.md)，实机验收尚未完成；不支持 WSL2、Windows ARM64、Codex TUI 和 IDE 扩展。`codex app-server` 是自动化验收手段，不是另一种产品入口。Intel 构建不代表已经完成 Intel App 人工验收。

这套安装入口从首个包含 `lumi-codex-version.txt`、CLI 和插件包的 Release 起可用。若当前 Latest Release 尚无这些资产，下载会明确失败；开发者可按[开发与验收说明](installation-development.md)使用本地构建验证，不能把源码实现完成说成已公开发布。

## 一句提示词

在已登录的 Codex App 中粘贴：

> 请按 https://raw.githubusercontent.com/lumikaka/lumi/refs/heads/master/docs/features/codex-plugins/install.md 的普通用户安装流程，识别当前 Agent 执行环境，为我安装或更新 Lumi 插件（macOS 正式入口；Windows 原生 x64 预览）；完成安装与启用状态验证后，报告实际使用的 codex 路径和版本、Lumi 版本、安装结果，以及新开会话开始创作的方法。

一句提示词会让 Codex 执行安装流程，不等于零批准，也不保证当前会话热加载。可能需要用户处理的步骤：

| 步骤 | 用户可见行为 |
| --- | --- |
| 下载 Release 二进制和插件包 | 宿主可能请求网络/命令批准 |
| 写入用户安装目录和 `~/.codex`，注册 marketplace、安装并启用插件 | 宿主可能请求文件写入批准；次数和界面取决于权限模式、规则与组织策略 |
| 安装完成 | **在 Codex App 中新开会话**，以所选绘本文件夹作为工作目录 |
| 第一次创建项目 | 保护配置要生效，需要用户信任该项目，再新开会话核对权限；安装器不替用户设置信任 |

插件没有 hooks，不需要 hook 信任或 OAuth。若组织策略禁止插件，应由用户按实际策略处理，不通过覆盖配置绕过。

## 平台分流

先确认 Agent 的实际执行环境。macOS 使用下文 Bash 流程；Windows 原生 x64 使用 [PowerShell 安装预览](install-windows.md)，不要执行 Bash 脚本。Windows App 内的 WSL Agent 不属于 Windows 原生环境。未包含 Windows 资产的 Release 无法安装预览版；本地验证使用完整构建资产与 `-FromDir`。

## Codex 安装执行流程（macOS）

普通安装按以下步骤完成；源码构建、发布和全书创作验收属于[开发流程](installation-development.md)。

1. 下载下面的 Release 安装脚本及同版本 SHA256，校验后阅读脚本，再执行。安装器负责选择 Codex、检查架构、目录归属和安装锁，并在下载资产前定向检查 `lumi-local`。
2. 安装器将 Release 固定到同一版本，下载并校验 CLI 和插件包。已校验的网络下载按版本、架构和 SHA256 缓存，重试会重新校验并复用；无需反复重新下载整个安装包。
3. 安装器只注册 `lumi-local`、安装 `lumi@lumi-local`，随后定向验证版本、`installed=true`、`enabled=true`、缓存中的绝对 CLI 路径及创作技能。以退出码 0 和最终成功 JSON 作为本次安装完成条件。
4. 遇到错误，按日志中的阶段处理后用同版本、同安装目录和缓存目录重试。无关市场失效由安装器隔离；Lumi 自身市场损坏、同名来源冲突或策略拒绝需要处理具体原因。保留原错误，不把查询失败解释为“尚未安装”，不临时修改脚本或其他插件配置来跳过检查。
5. 向用户报告成功 JSON 中的 Codex 路径、版本、`CODEX_HOME`、Lumi 版本、安装/启用状态和路径，然后给出新开目录会话的创作方法。安装任务到此完成；新会话发现、项目信任和原生生图是后续使用步骤。

安装器把阶段、累计耗时、下载字节数、下载耗时和缓存命中写入 stderr；成功安装的 stdout 为 `{"success":true,"data":{...}}`，其中包含 `codex_path`、`codex_version`、`codex_home`、`lumi_version`、`installed`、`enabled`、`cli_path` 和 `installed_path`。

## 安装命令（macOS）

先解析 Release 版本，再从同一 Tag 下载脚本和校验文件。以下下载块任一步失败都会停止：

```bash
lumi_install_tmp="$(mktemp -d)"
(
  set -e
  curl --fail --location --proto '=https' --proto-redir '=https' \
    https://github.com/lumikaka/lumi/releases/latest/download/lumi-codex-version.txt \
    -o "$lumi_install_tmp/lumi-codex-version.txt"
  lumi_release_version="$(cat "$lumi_install_tmp/lumi-codex-version.txt")"
  [[ "$lumi_release_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$ ]]
  for lumi_file in install-codex.sh install-codex.sh.sha256; do
    curl --fail --location --proto '=https' --proto-redir '=https' \
      "https://github.com/lumikaka/lumi/releases/download/v$lumi_release_version/$lumi_file" \
      -o "$lumi_install_tmp/$lumi_file"
  done
  cd "$lumi_install_tmp"
  shasum -a 256 -c install-codex.sh.sha256
)
```

检查下载的 `install-codex.sh` 内容后执行下面的块；它会再次校验，再安装刚才固定的版本：

```bash
(
  set -e
  cd "$lumi_install_tmp"
  shasum -a 256 -c install-codex.sh.sha256
  bash ./install-codex.sh --version "$(cat lumi-codex-version.txt)"
)
```

下载/校验任何一步失败就停止，不继续执行旧文件。SHA256 用于发现损坏或资产混用，不代替可信 Release 来源或 Apple 签名。安装器只依赖 macOS 自带 Bash、curl、shasum、tar、plutil，无需 Python、Node、Homebrew、sudo 或 API key。

直接运行安装器且未指定版本时，它先读取 stable 版本文件，再固定到同一个 `vX.Y.Z` Tag。CLI 和插件包全部校验通过后才切换安装。自定义安装示例：

```bash
# 指定已发布版本；升级、卸载时沿用同一个目录和 CODEX_HOME。
bash "$lumi_install_tmp/install-codex.sh" --version "$(cat "$lumi_install_tmp/lumi-codex-version.txt")" \
  --install-dir "$HOME/.local/share/lumi-codex" \
  --cache-dir "$HOME/Library/Caches/lumi-codex"
```

缓存默认位于 `~/Library/Caches/lumi-codex/<版本>/<架构>/<SHA256>/`。失败会清理本次暂存文件并回滚安装，已校验的下载缓存保留；缓存损坏会重新下载。每次重试仍从固定 Tag 读取校验文件。`--from-dir` 始终检查传入的本地资产，不用网络缓存替代它。

默认安装布局：

```text
~/.local/share/lumi-codex/
  .lumi-codex-install             # 绑定安装时的 CODEX_HOME
  current/
    bin/lumi
    version.txt
    install-codex.sh              # 可离线卸载
    marketplace/
      .agents/plugins/marketplace.json
      plugins/lumi/
        .codex-plugin/plugin.json
        .mcp.json
        skills/create-picture-book/SKILL.md
        assets/icon.png
```

安装器优先使用 `/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex`，不存在时再使用 PATH 中的 `codex`，并输出实际路径与 `--version`。PATH CLI 与 App 内置版本可能不同，但默认共用 `~/.codex`。fallback 只解决安装命令入口，不扩展 TUI/IDE 支持范围。

由安装器生成的 `.mcp.json` 使用 `current/bin/lumi` 的**规范化绝对路径**和 `args:["mcp"]`，没有 `cwd`。仓库模板仅保留安装器占位符，不能把源插件直接复制进缓存使用。Codex 安装到 `CODEX_HOME/plugins/cache/` 后，绝对二进制路径仍有效，stdio 服务以新会话 cwd 绑定项目。

配置仅通过这两个 Codex 命令写入：

```bash
"$codex_bin" plugin marketplace add "$lumi_install_dir/current/marketplace" --json
"$codex_bin" plugin add lumi@lumi-local --json
```

这里的变量取安装日志实际值。只管理 `lumi-local` marketplace 和 `lumi@lumi-local`，不编辑用户配置中的其他项目、模型、权限或插件；同名 marketplace 指向别处时拒绝覆盖。旧 HTTP OAuth 插件已移除；已安装的旧副本需从 Codex 插件管理卸载。

## 开始创作与续做

新建一个空文件夹，在 Codex App 中以它为项目目录新开会话，例如说：

> 用 Lumi 给 5 岁孩子做一本 8 页横版水彩绘本：小狐狸把迷路的星星送回天空。细节请你决定，直接完成整本并导出 PDF 和包含全部文字的 ZIP。

模型会确认当前项目、初始化并完成 setup，然后依次写故事、设定、分页和画面描述，使用 App 原生生图并立即导入，最后审阅和导出。未授权自行决定时，skill 默认在 setup 定稿前、以及故事/设定/代表页完成后停顿审阅；用户的明确要求优先。

生图和写作受当前 Codex 账号能力与额度限制。额度不足或对话中断时，在同一目录新开会话说“继续这个 Lumi 绘本”；模型从保存状态读取，只补缺失部分。PDF 使用图片版面；可编辑文字随 Markdown/ZIP 交付。不要自己修改或读取 `project.sqlite`、`.lumi/`、`assets/` 来续做。

skill 先用 `project_init` 建 setup 草稿，再在同一目录用绝对路径的 `lumi init --json` 为已有项目幂等补齐保护配置，不重置草稿。用户信任项目后才能加载该项目级配置；`lumi project info --json` 只能报告配置及可检测的冲突，不能证明实际沙箱策略已生效。旧 `sandbox_mode`、会话参数或更高优先级规则可能覆盖它，见 [Step 2 / 写保护调研](spike.md#5-沙箱写保护与项目配置的信任)。

## 升级与卸载（macOS）

升级前结束正在创作的会话，并在可以关闭所有 CLI 项目时运行：

```bash
"$HOME/.local/share/lumi-codex/current/bin/lumi" stop
```

随后重新下载并运行最新安装脚本，沿用相同 `CODEX_HOME` 与 `--install-dir`。重复安装会刷新本地插件；当前 App 内置版本的 `plugin add` 同时用于首次安装和升级，无需臆造 `plugin update`。成功后新开 App 会话。安装失败时恢复旧 payload，并尝试用 Codex 命令恢复旧 Lumi 注册，错误仍会返回给调用者。

卸载（自定义安装时替换路径，且沿用相同 `CODEX_HOME`）：

```bash
"$HOME/.local/share/lumi-codex/current/bin/lumi" stop
bash "$HOME/.local/share/lumi-codex/current/install-codex.sh" --uninstall
```

只移除新插件缓存、它的 marketplace 和安装器管理的 `current/` 内容；不删除绘本项目、Lumi 应用数据、下载缓存或旧插件。不退出仍打开的旧会话就不能保证工具立即消失。无运行任务后可用 `rmdir "$HOME/.local/share/lumi-codex"` 清理留下的空目录。若忘记 stop，已有 CLI 进程可能仍在运行；先重新安装相同目录，再用 `lumi stop` 正常退出，不删除实例文件强行解锁。

## 常见问题

| 现象 | 处理 |
| --- | --- |
| Release 404 / 下载失败 | 检查最新版本是否含 Codex 资产、网络是否获批；不跳过校验 |
| SHA256 不一致 | 网络资产校验不符则停止；缓存校验不符会自动重下；本地 `--from-dir` 资产应修复后重试。旧安装保持不变 |
| 无关 marketplace 源目录失效 | 安装器定向检查 Lumi，正常安装/升级可继续；不必删除其他市场或修改安装脚本 |
| `Cannot inspect lumi-local` | Lumi 自身市场或配置加载失败；按紧邻错误修复其源目录，再重试。此错误发生在资产下载前 |
| 注册/最终验证失败 | 先处理日志中的具体原因，再用同版本参数重试；已校验资产自动复用，失败不会报告成功 |
| 无法核对空市场归属 | 空目录没有插件条目可用于核对来源；安装时仍由 Codex 拒绝来源冲突，卸载时须先恢复可读取的市场信息 |
| App 中没有工具 | 新开以项目目录为 cwd 的会话；查安装日志中的 codex 路径、版本、CODEX_HOME，确认插件启用。必要时重启 App |
| 找不到 CLI / exec format error | 检查架构、安装目录是否被移动。用当前架构重装，不能把缓存中的 command 改成裸 `lumi` |
| Gatekeeper 阻止 | 核对 Release 来源、SHA256 和 CI 签名/公证状态，再按系统“隐私与安全性”提示处理；脚本不会移除 quarantine 或关闭系统检查 |
| `project_not_initialized` | 确认新会话目录是目标绘本目录，授权创建后由插件 `project_init` |
| `project_locked` | 在桌面版关闭该项目，或按错误详情停止占用它的 CLI 实例 |
| `cli_upgrade_required` | 结束该实例上的工作，用相同 environment/data-dir 执行 `lumi stop`，然后重开会话 |
| `lumi-local already belongs to ...` | 沿用日志里的原安装目录；迁移安装位置前先卸载旧安装 |
| `.install-lock` / `.previous` 残留 | 先确认没有安装脚本运行。若有 `.previous`，将失败的 `current` 移到独立备份位置，再将 `.previous` 恢复为 `current`；移除空 `.install-lock` 后重新安装。不要删除整个 `~/.codex` |
| 没有图片额度 / 生图工具 | 保留已写入内容，先导出文字 ZIP；额度恢复或宿主能力可用后继续，不要求 API key |
| 画面比例略有偏差 | 导入会提示偏差；可保留原图、裁切或补边，不能以精确像素作为继续创作的前提 |

## 开发、发布与验收

本地开发插件、Release/CI 和自动化/人工验收步骤见[开发与安装验收说明](installation-development.md)。
