# Lumi Codex 绘本插件

两个入口都通过本地 stdio CLI 工作，无需 API key，面向 Codex App。macOS 保留正式版和开发版；正式版另有 [Windows 原生 x64 安装预览](../docs/features/codex-plugins/install-windows.md)，实机验收尚未完成，开发版脚本仍面向 macOS：

| 入口 | 插件身份 | CLI | 默认应用数据 |
|---|---|---|---|
| 正式版 [`lumi/`](lumi/) | `lumi@lumi-local` | Release 安装的 `lumi` | `~/.lumi` |
| 开发版 [`lumi-dev/`](lumi-dev/) | `lumi-dev@lumi-dev-local` | 当前源码构建的 `lumi_dev` | `~/.lumi_dev` |

插件以会话 cwd 绑定绘本项目，使用 Codex 的写作和生图能力，无需 OAuth 或启动桌面 Lumi。

正式版使用[一句话安装指南](../docs/features/codex-plugins/install.md)。开发版在仓库根目录执行：

```sh
make codex-dev
```

此命令构建本机 Go CLI 和前端，生成 `build/codex-dev/marketplace`，然后使用 Codex App 内置 CLI 注册独立开发市场并安装或刷新插件。没有 Release 下载、Tag、版本发布或 Rust 构建。安装后在独立测试绘本文件夹中打开新的 Codex App 会话，使用 `lumi-dev:create-picture-book`。

`lumi-dev/` 只保存 manifest 和 MCP 配置差异。构建脚本从 `lumi/` 复制共享 skill、调用参考和图标，再生成开发 CLI 入口文档；不要维护第二份创作 skill，也不要直接安装含占位符的源码模板。生成的 MCP 配置使用绝对 CLI 路径，固定 `--environment development` 和 `--data-dir`，不指定 cwd。

## 日常开发

```sh
# 修改 Go / 前端后：重新构建并刷新插件。
make codex-dev
# 结束正在进行的开发任务后，停止旧后台；下一次连接自动启动新构建。
make codex-dev-stop

# 仅修改 skill / manifest / MCP 配置时：刷新即可，无需重新编译。
make codex-dev-refresh

# 只构建开发 CLI，不修改 Codex 安装。
make codex-dev-build
```

重建和刷新后新开 App 会话；旧会话不会热加载。只有 CLI 二进制变化时，也必须停止旧开发后台。Codex 使用插件缓存，修改仓库模板后需重新执行 refresh；无需增加 patch 版本。若 App 仍显示旧插件，重启 App 后再开会话。参见 [OpenAI 本地插件说明](https://developers.openai.com/plugins/build/plugins#how-local-marketplaces-work)。

所有目标支持 `CODEX_DEV_OUT`（默认 `build/codex-dev`）和 `CODEX_DEV_DATA_DIR`（默认 `~/.lumi_dev`，自定义必须绝对路径）；后续刷新、停止和卸载使用同一组参数。`CODEX_HOME` 未设置时为 `~/.codex`，应与实际使用的 Codex App 一致。安装器会输出实际 Codex 路径、版本、CODEX_HOME、开发 CLI 和数据目录。

开发 CLI 构建默认环境为 `development`，不读取 `APP_ENV` 来选择环境；普通 Release CLI 默认仍为 `production`。直接调用 CLI 时 `LUMI_DATA_DIR` / `--data-dir` 仍可覆盖目录，生成的开发插件始终显式指定开发目录。应用数据隔离不等于项目文件隔离，请使用独立测试绘本目录。

卸载开发插件（保留二进制、应用数据和项目）：

```sh
make codex-dev-stop
node scripts/dev-codex.mjs --uninstall
```

自定义输出目录时给脚本传 `--out /绝对路径`；沿用安装时的 `CODEX_HOME`。正式插件的注册与文件不受开发安装、刷新或卸载影响。

## 验证

```sh
node --test scripts/test-package-codex.mjs scripts/test-dev-codex.mjs
go test ./cmd/lumi
go test ./internal/server -run TestMCPOAuthOfficialSDK -count=1
```

开发生成测试覆盖共享资源、含空格路径、同版本刷新及目录保护。HTTP OAuth SDK 测试覆盖通用 MCP 服务的动态注册、带后缀回调和重复 resource。真实 Codex App 的新会话和原生生图仍需在安装后验证。

页面宿主生图现使用 `prepare_page_image → 当前代理首次看该摘要与路径的合图 → imagegen referenced_image_paths=[同一路径] → import_image（同一提示词和 preparation）`。`codex-dev-build` 同时生成开发市场源，`codex-dev-refresh` 才通过官方 CLI 刷新安装；不手工改插件缓存。不兼容的旧后台会返回 `cli_upgrade_required`，需协调现有连接后停止旧开发后台并新开会话。临时合图和准备签名不跨后台重启复用。
