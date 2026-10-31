# Step 6 安装与插件自动化验收

日期：2026-10-31（Asia/Shanghai）。本记录覆盖源码构建、本地 Release 夹具、真实 App 内置 CLI 和 app-server；**没有执行真实用户 App 安装、自主模型创作、原生生图或发布 CI**。

## 环境

| 项目 | 实测 |
| --- | --- |
| macOS | 14.5，arm64 |
| App | `/Applications/ChatGPT.app`，`26.1027.30824`，build `22943` |
| 使用的 codex | `/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex` |
| codex 版本 | `codex-cli 0.166.0-alpha.5.1` |
| Go | `go1.25.0 darwin/arm64` |
| 临时 CODEX_HOME | `/private/tmp/lumi-codex-acceptance-3k0phpow/codex-home` |
| 临时安装目录 | `/private/tmp/lumi-codex-acceptance-3k0phpow/install with spaces` |
| 新 thread 的空目录 | `/private/tmp/lumi-codex-acceptance-3k0phpow/empty-project` |

## 执行命令

在仓库根目录执行（复跑会生成新目录，不要依赖上述历史路径）：

```bash
make codex-release CODEX_VERSION=0.1.0 CODEX_OUT=/private/tmp/lumi-step6-release
PYTHONDONTWRITEBYTECODE=1 python3 scripts/test-install-codex.py \
  --assets /private/tmp/lumi-step6-release
python3 /Users/qingyang/.codex/skills/.system/skill-creator/scripts/quick_validate.py \
  codex-plugins/lumi/skills/create-picture-book
hugo --source site --gc --minify --panicOnWarning
./site/scripts/check-build.sh site/public
```

构建生成真实 arm64 / x86_64 Mach-O，含 web UI 和 MCP Apps；Vite 构建通过，保留已有的大 chunk 提示。最终 skill 调整后重新打包，再运行了本记录的完整安装验收。二进制未在本地执行 Developer ID 签名或公证。没有运行 Cargo/Rust。

测试第一次在命令沙箱中启动 Lumi 时，loopback bind 被拒绝；随后通过宿主审批在外层沙箱外运行同一隔离测试，Lumi 仅监听本机 loopback。此审批不是“真实用户一句话安装批准次数”的样本。

## 结果

```text
PASS: two installs produce identical payload and Codex config (path contains spaces)
PASS: generated stdio command is absolute and has no cwd
PASS: SHA256 failure preserves the existing installation and config
PASS: marketplace collision preserves the original installation
PASS: Codex config-write failure rolls back the switched payload
PASS: version upgrade refreshes the plugin cache; repeating upgrade is idempotent
PASS: same-version add refreshes MCP config in cache
PASS: App loader accepts manifest and skill, and discovers Lumi MCP tools
PASS: CLI init adds protection guidance to the MCP project without resetting its setup draft
PASS: empty-directory thread: initialize → setup → page text → import PNG → complete PDF
PASS: offline uninstall and repeated uninstall preserve projects and unrelated Codex config
Skill is valid!
Verified 30 generated routes/assets and both language search indexes.
```

升级夹具把插件版本从 `0.1.0` 改成 `999.0.1`，保留同一个测试 CLI，以验证实际插件缓存刷新；没有发布该测试版本。配置故障通过临时将测试 CODEX_HOME 设为只读触发，随后恢复权限；安装器恢复旧 payload，原配置完全相同。无关的模型设置、另一个插件条目和用户注释在正常安装、升级、失败和卸载后均保留。

App `skills/list` 识别到启用的 `lumi:create-picture-book`，`pluginId=lumi@lumi-local`，无 skill 错误；实际 MCP 工具发现和调用成功。本机 CLI 没有独立 `plugin validate` 子命令，因此没有声称运行了不存在的校验器。另由 skill-creator 的独立行为演练检查“用户禁止 subagent 并要求直接完成”和“中断、版本变化、图片额度不足”两种场景，据此澄清了父级集合 revision 与缺图文字导出规则；这不替代模型端到端验收。

空目录流程先证实 `project_not_initialized`，再调用 `project_init`；补一次 CLI `init` 后项目 UUID 与 setup revision 不变、仍为 draft，保护配置已生成但有效状态仍为 unknown。随后 setup 定稿、创建章节和一页正文、重读核对、查询图片比例、导入 `96×64` 合成 PNG，再用 `allow_missing_images=false` 导出 PDF。

- PDF：1,750 字节，`missing_image_count=0`，文件头 `%PDF-`。
- SHA256：`60b22ce2439315ebd031ced53ab884c610a7aff2d716c4a3e6cb46983256243a`。
- 可查看[测试 PDF](step6-book.pdf)；纯色合成图仅用于验证导入和确定性导出。
- [命令输出与 RPC 记录](step6-install.json)保存完整命令、退出码、stdout/stderr、请求和响应；仅从 `skills/list` 中剔除无关的用户/系统 skills，并附被测脚本和 skill 的 SHA256。

`scripts/install-codex.sh` 通过 Bash 语法检查，打包脚本通过 Node 语法检查。两个发布 workflow 通过 YAML 解析与内嵌 Bash 语法检查；未安装 actionlint，未把语法检查说成远程 CI 已执行。

## 保留边界

本次未复制账号凭据，没有请求 API key、没有模型调用。安装资产来自本地 `--from-dir` 夹具；真实 HTTPS Release 下载、PATH CLI fallback、Intel App、Developer ID/Apple 公证和 Gatekeeper 行为未完成实机验收。测试仅在生成的临时 MCP 配置中加入 `LUMI_DATA_DIR`，确保不会访问用户实际 Lumi 应用库。

最终测试已正常停止自己的 CLI、卸载自己的插件；临时项目和导出保留供检查。真实 App 的一句话安装、每次批准的界面/次数、新会话自主创作、至少一张 App 原生图导入及整本交付，全部在 [install.md 人工验收表](../install.md#codex-app-人工验收由用户执行)标为待人工确认。
