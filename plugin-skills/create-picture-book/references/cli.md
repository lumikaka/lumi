# 正式版 CLI 入口

<!-- host:codex -->
使用安装器输出的 CLI 绝对路径，或当前插件 `.mcp.json` 的 `mcpServers.lumi.command`，不依赖 PATH。macOS 默认位置为 `~/.local/share/lumi-codex/current/bin/lumi`，先展开 `~`。Windows 原生 x64 预览使用 `%LOCALAPPDATA%\LumiCodex\versions\<版本>-<内容摘要>\lumi.exe`，必须取实际安装结果，不能猜版本目录。
<!-- host:generic -->
使用接入配置中启动 `lumi mcp --host generic` 的 CLI 绝对路径与相同 environment/data-dir，不依赖 PATH 或其他插件安装目录。参见通用接入包 README。
<!-- /host -->

<!-- host:codex -->
子命令如 `init --json`、`project info --json`、`open --json`。macOS 使用 `"/绝对路径/lumi" init --json`；Windows PowerShell 使用 `& 'C:\实际版本目录\lumi.exe' init --json`。环境默认为 `production`，未指定 `LUMI_DATA_DIR` 或 `--data-dir` 时，macOS 应用数据位于 `~/.lumi`，Windows 位于系统 LocalAppData 下的 `dev.lumi.Lumi`。在当前绘本文件夹执行，不通过更改 MCP 的 cwd 切换项目。
<!-- host:generic -->
在当前绘本文件夹执行 `"/绝对路径/lumi" init --host generic --json` 补齐共享指引，或用 `"/绝对路径/lumi" project info --host generic --json` 读取项目身份。打开项目网页用 `"/绝对路径/lumi" open --json`。环境默认为 `production`，未指定 `LUMI_DATA_DIR` 或 `--data-dir` 时，macOS 应用数据位于 `~/.lumi`。
<!-- /host -->

<!-- host:codex -->
开发插件会在生成时替换本文件，提供开发 CLI 的绝对路径和固定环境、数据目录参数。若当前使用 `lumi-dev` 却看到正式版入口，先刷新开发插件并新开会话，不用正式 CLI 代替。
<!-- host:generic -->
开发环境显式使用 `--environment development --data-dir /绝对路径/开发数据目录`，CLI 和 MCP 必须一致。不停止其他宿主共享的后台实例；需要升级实例时先协调使用者。
<!-- /host -->
