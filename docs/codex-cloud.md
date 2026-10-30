# Lumi 的 Codex Cloud 环境配置

本配置用于在 Codex Cloud 中开发和验证 Lumi 的 Go 服务与 React 前端。`site/` 是独立的 Hugo 项目；桌面端的 Rust/Tauri 构建由 CI 负责。本文件是创建环境时可直接使用的配置清单，环境是否发布以 Codex Cloud 中的状态为准。

## 创建环境

按照 [Codex Cloud 环境文档](https://learn.chatgpt.com/docs/environments/cloud-environments#create-and-publish-an-environment)，在 **Work in → Cloud → Select environment → Create environment** 中选择：

| 配置 | 值 |
| --- | --- |
| GitHub 仓库 | `lumikaka/lumi` |
| 初始分支 | `master`（仓库默认分支） |
| Who can use | `Only me` |
| Allow Codex to access internet | 开启 |
| Allow domains | `Package managers` |
| Environment variables / Network secrets | 基础安装、测试和启动均不需要 |
| VPN / OIDC | 基础开发环境不需要 |

`Package managers` 预设包含本项目所需的 npm、Go 模块代理、GitHub 下载地址和 Ubuntu/Debian 软件源。若实际安装日志显示另一个下载主机被阻止，只添加日志中的具体主机并重试；无需使用不受限网络。

在环境的准备对话中发送：

> 为 `lumikaka/lumi` 准备 Linux 开发环境。安装并使用 Go 1.25、Node.js 26 和 pnpm 12.16.1。只安装 Go 服务与 `web/` 前端的依赖：运行 `go mod download`、`pnpm --dir web install --frozen-lockfile`。不要运行 Cargo、Rust 编译、桌面打包或 `make deps`（后者会额外安装 `rel/app/` 的桌面工具）。运行 `go test ./...`、`pnpm --dir web test`、`pnpm --dir web run build` 验证安装。然后分别启动 `make dev-web` 和 `make dev-api`，确认 `http://127.0.0.1:5801/api/v1/health` 返回成功 JSON，并记录可重复使用的 Install script 与 Start skill。

## Install script

准备对话安装好上述工具后，将测试通过的安装命令记录为 Install script。项目依赖部分应为：

```bash
set -eu
go mod download
pnpm --dir web install --frozen-lockfile
```

Go、Node.js 和 pnpm 的安装命令及 `PATH` 设置应由云端准备对话根据实际 VM 验证后加入脚本；不能直接假设 VM 已安装指定版本。`web/package.json` 固定 `pnpm@12.16.1`。无需复制 `.env.example`：服务在 development 模式下已有本地默认值。

## Start skill

在 Start skill 中记录以下说明：

> 从仓库根目录，在两个持续运行的终端分别执行 `make dev-web` 和 `make dev-api`。前者在 `127.0.0.1:5802` 启动 Vite，后者通过 `go tool air` 在 `127.0.0.1:5801` 启动 Go 服务。等待 `curl --fail --silent --show-error http://127.0.0.1:5801/api/v1/health` 返回 `{"success":true,"data":{"status":"ok","database":"connected"}}`。访问应用时使用 `http://127.0.0.1:5801/`；Go 会代理前端请求。开发和测试不需要第三方 AI Provider 密钥。

若只运行 Go 单元测试或前端静态检查，无需启动服务。云任务目前不支持电脑或浏览器操作，因此交互式 UI 验证应在本地完成。

## 发布前检查

1. 在准备对话中确认 `go version` 为 1.25.x、`node --version` 为 26.x、`pnpm --version` 为 12.16.1。
2. 确认 `go test ./...`、`pnpm --dir web test`、`pnpm --dir web run build` 均通过；再检查健康端点。
3. 检查生成的 Install script、Start skill、网络访问范围及准备出的文件，保存并发布环境。
4. 新建一个云任务，再运行上述检查。已存在的任务保留自己的工作状态，不会自动切换到重新发布的环境。

若任务需要修改 `site/`，再为该任务安装 Hugo Extended 0.164.0，并按 [站点说明](../site/README.md) 构建；它不属于基础环境。`rel/app/` 的 Rust/Tauri 检查继续交给现有 CI workflow。
