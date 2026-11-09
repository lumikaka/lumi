# 正式版 CLI 入口

<!-- host:codex -->
使用安装器输出的 CLI 绝对路径，或当前插件 `.mcp.json` 的 `mcpServers.lumi.command`，不依赖 PATH。macOS 默认位置为 `~/.local/share/lumi-codex/current/bin/lumi`，先展开 `~`。Windows 原生 x64 预览使用 `%LOCALAPPDATA%\LumiCodex\versions\<版本>-<内容摘要>\lumi.exe`，必须取实际安装结果，不能猜版本目录。
<!-- host:claude -->
使用当前插件 `.mcp.json` 中 `mcpServers.lumi.command` 指向的 CLI 绝对路径，不依赖 PATH，也不使用其他宿主插件的安装目录。路径不可读时请用户按插件 README 重新安装，不猜版本目录。
<!-- /host -->

<!-- host:codex -->
子命令如 `init --json`、`project info --json`、`open --json`。macOS 使用 `"/绝对路径/lumi" init --json`；Windows PowerShell 使用 `& 'C:\实际版本目录\lumi.exe' init --json`。环境默认为 `production`，未指定 `LUMI_DATA_DIR` 或 `--data-dir` 时，macOS 应用数据位于 `~/.lumi`，Windows 位于系统 LocalAppData 下的 `dev.lumi.Lumi`。在当前绘本文件夹执行，不通过更改 MCP 的 cwd 切换项目。
<!-- host:claude -->
子命令如 `init --json`、`project info --json`、`open --json`，写作 `"/绝对路径/lumi" init --json`。环境默认为 `production`，未指定 `LUMI_DATA_DIR` 或 `--data-dir` 时，macOS 应用数据位于 `~/.lumi`。在当前绘本文件夹执行，不通过更改 MCP 的 cwd 切换项目。
<!-- /host -->

开发插件会在生成时替换本文件，提供开发 CLI 的绝对路径和固定环境、数据目录参数。若当前使用 `lumi-dev` 却看到正式版入口，先刷新开发插件并新开会话，不用正式 CLI 代替。
