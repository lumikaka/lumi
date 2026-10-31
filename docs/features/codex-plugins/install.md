# 在 Codex App 中安装和使用 Lumi

Lumi 的新 `lumi` 插件通过本地 stdio CLI 管理绘本，写作和生图使用 Codex 自身额度，**无需 API key**。项目保存在本地；需要生成的文字和参考图片仍会交给 Codex，不能把“本地保存”理解为离线推理。

首版仅支持 **macOS 上的 Codex App**，提供 Apple Silicon（arm64）和 Intel（amd64）CLI；不支持 Codex TUI 和 IDE 扩展。`codex app-server` 是自动化验收手段，不是另一种产品入口。Intel 构建不代表已经完成 Intel App 人工验收。

这套安装入口从首个包含 `lumi-codex-version.txt`、CLI 和插件包的 Release 起可用。若当前 Latest Release 尚无这些资产，下载会明确失败；开发者可按后文使用本地构建验证，不能把源码实现完成说成已公开发布。

## 一句提示词

在已登录的 Codex App 中粘贴：

> 请阅读 https://raw.githubusercontent.com/lumikaka/lumi/main/docs/features/codex-plugins/install.md，按文档为我安装 macOS Codex App 的 Lumi 绘本插件；安装后告诉我实际使用的 codex 路径和版本、安装结果及如何新开会话开始创作。

一句提示词会让 Codex 执行安装流程，不等于零批准，也不保证当前会话热加载。可能需要用户处理的步骤：

| 步骤 | 用户可见行为 |
| --- | --- |
| 下载 Release 二进制和插件包 | 宿主可能请求网络/命令批准 |
| 写入用户安装目录和 `~/.codex`，注册 marketplace、安装并启用插件 | 宿主可能请求文件写入批准；次数和界面取决于权限模式、规则与组织策略 |
| 安装完成 | **在 Codex App 中新开会话**，以所选绘本文件夹作为工作目录 |
| 第一次创建项目 | 保护配置要生效，需要用户信任该项目，再新开会话核对权限；安装器不替用户设置信任 |

插件没有 hooks，不需要 hook 信任或 OAuth。若组织策略禁止插件，应由用户按实际策略处理，不通过覆盖配置绕过。

## 安装命令

先下载可审阅的脚本和同版本校验文件，再校验执行；不要把未检查的网络输出直接 pipe 给 shell：

```bash
lumi_install_tmp="$(mktemp -d)"
curl --fail --location --proto '=https' --proto-redir '=https' \
  https://github.com/lumikaka/lumi/releases/latest/download/install-codex.sh \
  -o "$lumi_install_tmp/install-codex.sh"
curl --fail --location --proto '=https' --proto-redir '=https' \
  https://github.com/lumikaka/lumi/releases/latest/download/install-codex.sh.sha256 \
  -o "$lumi_install_tmp/install-codex.sh.sha256"
(cd "$lumi_install_tmp" && shasum -a 256 -c install-codex.sh.sha256)
# 查看脚本；上一条校验成功后才执行：
bash "$lumi_install_tmp/install-codex.sh"
```

下载/校验任何一步失败就停止，不继续执行旧文件。SHA256 用于发现损坏或资产混用，不代替可信 Release 来源或 Apple 签名。安装器只依赖 macOS 自带 Bash、curl、shasum、tar、plutil，无需 Python、Node、Homebrew、sudo 或 API key。

安装器先读取 stable 版本文件，再固定到同一个 `vX.Y.Z` Tag 下载该架构 CLI、插件包及各自 `.sha256`。两者全部校验通过后才切换安装。自定义安装示例：

```bash
# 指定已发布版本；升级、卸载时沿用同一个目录和 CODEX_HOME。
bash "$lumi_install_tmp/install-codex.sh" --version 0.2.0 \
  --install-dir "$HOME/.local/share/lumi-codex"
```

默认布局：

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

这里的变量取安装日志实际值。只管理 `lumi-local` marketplace 和 `lumi@lumi-local`，不编辑用户配置中的其他项目、模型、权限或插件；同名 marketplace 指向别处时拒绝覆盖。旧 HTTP OAuth 插件 `lumi-prod-plugin` / `lumi-dev-plugin` 保持独立。

## 开始创作与续做

新建一个空文件夹，在 Codex App 中以它为项目目录新开会话，例如说：

> 用 Lumi 给 5 岁孩子做一本 8 页横版水彩绘本：小狐狸把迷路的星星送回天空。细节请你决定，直接完成整本并导出 PDF 和包含全部文字的 ZIP。

模型会确认当前项目、初始化并完成 setup，然后依次写故事、设定、分页和画面描述，使用 App 原生生图并立即导入，最后审阅和导出。未授权自行决定时，skill 默认在 setup 定稿前、以及故事/设定/代表页完成后停顿审阅；用户的明确要求优先。

生图和写作受当前 Codex 账号能力与额度限制。额度不足或对话中断时，在同一目录新开会话说“继续这个 Lumi 绘本”；模型从保存状态读取，只补缺失部分。PDF 使用图片版面；可编辑文字随 Markdown/ZIP 交付。不要自己修改或读取 `project.sqlite`、`.lumi/`、`assets/` 来续做。

skill 先用 `project_init` 建 setup 草稿，再在同一目录用绝对路径的 `lumi init --json` 为已有项目幂等补齐保护配置，不重置草稿。用户信任项目后才能加载该项目级配置；`lumi project info --json` 只能报告配置及可检测的冲突，不能证明实际沙箱策略已生效。旧 `sandbox_mode`、会话参数或更高优先级规则可能覆盖它，见 [Step 2 / 写保护调研](spike.md#5-沙箱写保护与项目配置的信任)。

## 升级与卸载

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

只移除新插件缓存、它的 marketplace 和安装器管理的 `current/` 内容；不删除绘本项目、Lumi 应用数据或旧插件。不退出仍打开的旧会话就不能保证工具立即消失。无运行任务后可用 `rmdir "$HOME/.local/share/lumi-codex"` 清理留下的空目录。若忘记 stop，已有 CLI 进程可能仍在运行；先重新安装相同目录，再用 `lumi stop` 正常退出，不删除实例文件强行解锁。

## 常见问题

| 现象 | 处理 |
| --- | --- |
| Release 404 / 下载失败 | 检查最新版本是否含 Codex 资产、网络是否获批；不跳过校验 |
| SHA256 不一致 | 停止安装，重新从同一 Tag 下载；旧安装不会因此被替换 |
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

## 发布与 CI

`make build` 继续构建 `lumi`，并增加插件包产出。`make codex-build` 构建带 web UI/MCP Apps 的本机 CLI；`make codex-release CODEX_VERSION=X.Y.Z` 构建两个 macOS 架构和以下资产，**没有 Cargo/Rust 命令**：

```text
lumi-darwin-arm64                 lumi-darwin-arm64.sha256
lumi-darwin-amd64                 lumi-darwin-amd64.sha256
lumi-codex-plugin.tar.gz          lumi-codex-plugin.tar.gz.sha256
install-codex.sh                  install-codex.sh.sha256
lumi-codex-version.txt
```

沿用 `make tag-patch/minor/major → git push origin vX.Y.Z`，不由安装脚本创建或发布 Tag。[发布协调 workflow](../../../.github/workflows/desktop-release.yml) 增加 [Codex macOS assets job](../../../.github/workflows/codex-macos.yml)，所有桌面和 Codex job 都成功后才发布完整 Release。Tag 版本写入包内 manifest 和版本文件，仓库模板不注入用户路径。

CI 顺序：构建 Go/web → Go 定向测试 → 两架构 codesign 与 verify → 可选 notarization → **重新计算签名后 SHA256** → 上传 artifact → 协调 Release 发布。没有证书时仅 ad-hoc 签名，不宣称 Apple 公证。

Developer ID 沿用 `APPLE_SIGNING_IDENTITY` repository variable，以及 `APPLE_CERTIFICATE_BASE64`、`APPLE_CERTIFICATE_PASSWORD` secrets。公证另设置 `APPLE_NOTARY_APPLE_ID`、`APPLE_NOTARY_PASSWORD`（app-specific password）、`APPLE_NOTARY_TEAM_ID` secrets；必须同时配置 Developer ID。CI 把已签名 CLI 打成临时 ZIP，`xcrun notarytool submit --wait` 后检查 `Accepted`，失败阻止发布，流程依据 [Apple 公证文档](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow)。独立 CLI 不执行 `.app` 的 stapling；签名、远程公证和 Gatekeeper 实机行为仍需真实发布验收。此改动没有本地执行 Cargo/Rust，也没有触发 CI 或发布 Release。

## 自动化验收与证据

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
| 一句话真实安装 | 在用户 Codex App 执行本页提示词；记录 App 版本、内置 CLI 路径/版本、Release 版本、每次批准的动作与界面截图，以及批准总次数 | 待人工确认；次数未知 |
| 新会话发现 | 安装后新开目录会话，确认 `lumi:create-picture-book` 与项目工具被模型识别 | 待人工确认 |
| 项目信任与保护 | 初始化后由用户信任项目；新开会话记录有效权限。用临时测试项目验证受管路径写保护，不让模型修改真实项目数据库 | 待人工确认 |
| 自主完整创作 | 用上方小狐狸提示词，让模型自行从想法完成 setup、故事、设定、分页、生图、导入和整本导出；记录各审阅节点，确认没有索取 API key | 待人工确认 |
| 原生生图导入 | 至少一张图由 App 原生生图产生；记录工具实际返回路径、随后 import_image 成功结果、页面/图片 UUID 和审阅截图 | 待人工确认 |
| 一致性与参考 | 记录 `read_image` 的设定参考、紧随其后的生图及实际画面效果 | 待人工确认 |
| 最终交付 | 在 App 打开全书审阅并检查 PDF/ZIP，记录页数、文字、图片完整性及文件路径 | 待人工确认 |
| 真实升级/卸载、Intel | 在相应机器复测新会话可用性及项目保留；记录 Gatekeeper、签名/公证状态 | 待人工确认 |

验收记录模板：日期/架构；App 版本；codex 路径与版本；Release；原始提示词；批准列表（序号、动作、截图、结果）；项目 UUID；至少一张原生图导入证据；PDF/ZIP 路径与页数；通过项与未通过项。批准数应填实际观察值，不能预填 0。
