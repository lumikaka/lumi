# 正式版 CLI 入口

使用安装器输出的 CLI 绝对路径。默认安装位置为 `~/.local/share/lumi-codex/current/bin/lumi`；先将 `~` 展开为当前用户目录。自定义安装取安装器输出或当前插件 `.mcp.json` 的 `mcpServers.lumi.command`，不依赖 PATH。

命令格式：`/绝对路径/lumi <子命令> [参数]`，例如 `init --json`、`project info --json`、`open --json`。环境默认为 `production`，未指定 `LUMI_DATA_DIR` 或 `--data-dir` 时应用数据位于 `~/.lumi`。在当前绘本文件夹执行，不通过更改 MCP 的 cwd 切换项目。

开发插件会在生成时替换本文件，提供开发 CLI 的绝对路径和固定环境、数据目录参数。若当前使用 `lumi-dev` 却看到正式版入口，先刷新开发插件并新开会话，不用正式 CLI 代替。
