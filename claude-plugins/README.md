# Lumi Claude Code 插件

`claude-plugins/` 提供在 Claude Code 中创作绘本的两个插件：

| 插件 | 用途 | MCP 服务 |
| --- | --- | --- |
| `lumi` | 正式版模板：连接已安装的 `lumi` CLI | `lumi mcp --host claude` |
| `lumi-dev` | 开发版：连接从当前源码构建的 `lumi_dev`，使用独立开发数据目录 | `lumi_dev mcp --host claude --environment development --data-dir <dir>` |

两者都通过 stdio `lumi mcp` 绑定 Claude Code 会话的当前目录项目，与 Codex、Cindy 插件共用同一套 Go 工具实现和同一份创作 skill（源文件在 `plugin-skills/`，由 `node scripts/build-skills.mjs` 按宿主拼装；`--check` 检查已提交结果是否与源一致）。

## 与 Codex 插件的差异

- **写作**由 Claude 完成。**页面插图**由 Lumi 服务端调用你在 Lumi 中配置的图片服务商生成（`generate_page_image`，仅 `--host claude` 提供），按服务商计费；Codex 插件仍只用宿主 imagegen，没有服务商回退。
- Claude Code 不支持 MCP Apps：`--host claude` 的工具清单不含 `read_app_*`、`write_app_resource`、`watch_app_changes` 与 `review_*`，改用只读工具 `web_url` 返回真实 Lumi Web 页面的链接（作品、章节、页面、设定、导出、服务商配置）。链接端口来自当前运行的 CLI 后台，重启后可能变化，每次交付前重新获取。
- 批量设定总览图没有服务端工具。设定图来源为：用户提供图片后导入（`import_image` / `setting_boards`），或在 Lumi Web/桌面端生成；只有用户明确接受时才以纯文字设定继续。skill 不会假装批量设定图可用。
- `lumi init`（任何宿主）都会创建仅含一行 `@AGENTS.md` 的 `CLAUDE.md`；已有 `CLAUDE.md` 时不覆盖，只提示手工加入该行。
- 插件不声明 MCP Apps 或交互界面能力。

## 项目写保护

Lumi 项目文件只能通过 Lumi MCP 工具和 `lumi` CLI 读写。插件带有 `PreToolUse` Bash hook（`hooks/guard-lumi-project.mjs`，需要 PATH 上有 `node`）：

- 先从会话 `cwd`（以及 `CLAUDE_PROJECT_DIR`）向上查找 `project.sqlite` 或 `.lumi/`；**不是 Lumi 项目时直接放行**，不输出任何决定。
- 在 Lumi 项目内，命令文本直接引用 `project.sqlite*`、`.lumi/` 或 `assets/`（相对项目根，或指向本项目的绝对路径）时返回 `permissionDecision: "deny"` 并说明应改用的工具。
- 这是尽力而为的文本检查，不能识别变量拼接、`../` 等间接路径；hook 运行失败时 Claude Code 视为不阻止。

Read/Edit 等内置文件工具不经过 Bash hook。建议在绘本项目的 `.claude/settings.json` 中加入 deny 规则（`/` 前缀相对该设置文件所在项目根）：

```json
{
  "permissions": {
    "deny": [
      "Read(/project.sqlite*)",
      "Edit(/project.sqlite*)",
      "Read(/.lumi/**)",
      "Edit(/.lumi/**)",
      "Read(/assets/**)",
      "Edit(/assets/**)"
    ]
  }
}
```

`Edit` 规则覆盖所有内置写文件工具，`Read` deny 会尽力作用于 Grep/Glob。页面参考合图写在系统临时目录，看图使用 `prepare_page_image` 返回的路径或 `read_image`，不受这些规则影响。

## 开发安装

```sh
make claude-dev            # 构建 web + lumi_dev（-tags embed_frontend），生成并通过 claude CLI 安装
make claude-dev-build      # 仅构建并生成 build/claude-dev/marketplace
make claude-dev-refresh    # 重新生成并通过 claude CLI 刷新（marketplace update）
make claude-dev-stop       # 停止共享的开发后台
make claude-dev-uninstall  # 卸载插件与 marketplace，保留 CLI、应用数据和项目
```

变量与 Codex 开发版一致：`CLAUDE_DEV_OUT`（默认 `build/claude-dev`）、`CLAUDE_DEV_DATA_DIR`（默认 `~/.lumi_dev`，必须为绝对路径）。`CLAUDE_BIN` 可指定 `claude` CLI 路径。

- `scripts/dev-claude.mjs` 把 `claude-plugins/lumi` 与 `claude-plugins/lumi-dev` 合并到 `build/claude-dev/marketplace/plugins/lumi-dev`，写入 `.claude-plugin/marketplace.json`（marketplace `lumi-dev-local`），替换二进制与数据目录占位符，并把 skill 的 `references/cli.md` 改为开发 CLI 入口。
- 首次 `--install` 依次执行 `claude plugin marketplace add <dir> --scope user` 与 `claude plugin install lumi-dev@lumi-dev-local --scope user`；之后执行 `claude plugin marketplace update lumi-dev-local`。本地目录 marketplace 的插件由 Claude Code 原地加载，刷新后新开会话或运行 `/reload-plugins` 生效；脚本不修改插件缓存。
- 已有同名 marketplace 指向其他目录、或输出目录不是脚本管理的 marketplace 时拒绝覆盖。
- 开发后台与 Codex、Cindy 开发版共用 `~/.lumi_dev`。创作协议升级（当前 v6，`generate_page_image` 需要）后，先结束各宿主的开发任务，再 `make claude-dev-stop` 停止旧后台；旧后台对新工具返回 `cli_upgrade_required`，其他工具不受影响。
- 构建必须带 `-tags embed_frontend`，否则 `web_url` 链接打不开页面。

正式版 `.mcp.json` 中的 `__LUMI_ABSOLUTE_BINARY__` 是安装时替换的占位符；Claude Code 正式版的打包与发布流程尚未提供。

## 测试

```sh
node --test scripts/test-dev-claude.mjs scripts/test-build-skills.mjs
node scripts/build-skills.mjs --check
go test ./cmd/lumi ./internal/textmcp ./internal/jobqueue
```
