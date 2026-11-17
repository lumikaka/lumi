# 正式版 CLI 入口

使用接入配置中启动 `lumi mcp --host generic` 的 CLI 绝对路径与相同 environment/data-dir，不依赖 PATH 或其他插件安装目录。参见通用接入包 README。

在当前绘本文件夹执行 `"/绝对路径/lumi" init --host generic --json` 补齐共享指引，或用 `"/绝对路径/lumi" project info --host generic --json` 读取项目身份。打开项目网页用 `"/绝对路径/lumi" open --json`。环境默认为 `production`，未指定 `LUMI_DATA_DIR` 或 `--data-dir` 时，macOS 应用数据位于 `~/.lumi`。

开发环境显式使用 `--environment development --data-dir /绝对路径/开发数据目录`，CLI 和 MCP 必须一致。不停止其他宿主共享的后台实例；需要升级实例时先协调使用者。
