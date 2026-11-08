# Lumi Cindy 插件作者源码

2026-11-04 正在按 Codex 当前实现对齐，见 [基线与执行计划](../docs/features/cindy-plugins/codex-alignment.md) 和 [本次验收](../docs/features/cindy-plugins/acceptance/codex-alignment.md)。源码已补齐设定板授权导入/原子拆分、setting_reviews 与同源契约检查，以及可编辑工作区独立入口、构建和隔离测试；原生页面授权及合图到 Core 的传递仍待完成。`node cindy-plugins/scripts/sync-contracts.mjs --check` 比较实际 Go 契约，`node cindy-plugins/scripts/verify.mjs codex-alignment` 保存本次测试记录，历史记录不代表本次新增能力通过。

`lumi/` 维护一份浏览器逻辑、零第三方依赖 Node worker、Manual 与构建时编译的 Go CLI。当前实现项目绑定、文本创作、授权图片导入和受控参考图，版本 0.4.0。2026-11-02 开发版已安装并同版本原位更新，早期构建真实诊断连续 6 次成功，归档构建更新后亦已核对新指纹；修复宿主子进程代理清理导致的并发名额泄漏，最新完整自动化 79 项通过。任务审阅页、真实媒体验收及其余安装矩阵仍待完成，没有发布。安装记录与待验项见 [Step 6 状态](../docs/features/cindy-plugins/acceptance/step6-install.md)。

## 构建与验证

仅支持本次验证的 macOS arm64，使用 Go 1.27.3；运行时使用 Cindy 随包 Node，不要求用户安装 Node/Go/Lumi 桌面版。仓库根执行：

```sh
make cindy-build
make cindy-dev
node cindy-plugins/scripts/verify.mjs step4
go test ./internal/production ./internal/files ./internal/textmcp ./internal/project ./internal/promptcatalog ./cmd/lumi ./cindy-plugins/schema
```

正式内容生成于 `cindy-plugins/lumi/`，身份/command 为 lumi，随包 `bin/lumi`、production 与默认 `~/.lumi`。开发内容生成于 `build/cindy-dev/lumi-dev/`，身份/command 为 lumi-dev，随包 `bin/lumi_dev`、development 与默认 `~/.lumi_dev`。源码、Manual 与业务语义共用，身份和受控 runtime-config.json 在构建时生成。build-info.json 包含 CLI SHA256、源 commit/dirty 和全部随包内容的 build_id；同版本下也可区分内容。

运行 `make cindy-dev-install` 会先构建开发版，再生成标准 ZIP 格式的 `.cindy` 包，通过 macOS `open -a Cindy` 交给 Cindy 导入；需要时在 Cindy 内确认并选择已有 `lumi-dev` 实例。`CINDY_APP` 可指定应用名称或 `.app` 路径。构建、打包或提交失败会输出中文错误并返回非零；退出码 0 和“已提交安装”仅表示已交给 Cindy，不表示安装完成。按本次流程不等待回执、不核验安装后的指纹，不自动停止共享后台。`make cindy-dev` 仍只构建，无需 Cindy 源码或额外安装 CLI。依赖与失败处理见[开发说明](../docs/features/cindy-plugins/development.md#从终端构建并提交开发版安装)。

自定义开发 data-dir 使用构建参数，例如 `make cindy-dev CINDY_DEV_DATA_DIR=/private/tmp/lumi-cindy-acceptance-data`。业务工具和页面不能更改它。只有协调同一 development/data-dir 的全部使用者后，才手动 `make cindy-dev-stop`；脚本读取刚生成的同一配置，不会停止正式环境。Codex 开发版默认也用 `~/.lumi_dev`，可能共享服务。

自动化使用临时 HOME、作品和 data-dir；需允许本机 loopback bind/connect。测试关闭前核对实例 nonce，只清理自己的目录和服务。没有 Cargo/Rust、运行时安装依赖或公开发布。

## Forge 包与安装

先用 Host `ghost_forge_pack({dir:<作者目录绝对路径>})` 打包，再核对确切产物：

```sh
python3 cindy-plugins/scripts/verify-package.py step4
python3 cindy-plugins/scripts/verify-package.py step4 development
```

第二个命令要求先对 `build/cindy-dev/lumi-dev/` 执行 Forge pack。脚本核对每个包内文件与作者源码、身份、environment、CLI 执行位和 SHA256、build_id 及大小上限；输出保存到该步骤的 evidence，并按 SHA256 固定审阅 ZIP 到 build/cindy-reviewed/，避免 install 重打覆盖原件。Go 二进制、build-info、生成开发目录与 `.cindy` 都是本地生成物。

构建和 pack 不安装。执行包含安装的授权任务时，再用 `ghost_forge_install` 从同一未改动作者目录安装，核对真实 instanceId/namespace/source 和安装时重打的包。开发版同版本刷新、正式/开发同时安装与只读诊断已有真实回执；独立作品主线及 Go 后台切换仍待验，不能靠 build_id 或打包结果推断。不得从 Host 已安装目录制作、改写安装副本、自动停止共享服务或上传市场。

完整流程见 [开发说明](../docs/features/cindy-plugins/development.md)、[媒体契约](../docs/features/cindy-plugins/media.md)、[工具契约](../docs/features/cindy-plugins/tools.md) 和 [Step 4 验收](../docs/features/cindy-plugins/acceptance/step4-image-import.md)。历史 [Step 1](../docs/features/cindy-plugins/acceptance/step1-runtime.md)、[Step 2](../docs/features/cindy-plugins/acceptance/step2-project-binding.md)、[Step 3](../docs/features/cindy-plugins/acceptance/step3-authoring-tools.md) 记录不追记为新能力证明。

manifest 的 minCindyVersion=0.1.121 沿用 Forge scaffold 的本机版本。宿主实际 sourceCommit 与官方 tag 不同，匹配公开 ref 返回 404；这不是正式最低兼容版本证据。当前按随包 Skill 与实测推进本地验收，正式发布前须校准兼容声明。图标复用 Lumi 现有品牌资源。

Step 7 在源码 CLI 中新增显式的 `lumi cindy-session import --file PATH --source-session ID`，将公开文本包装文件保存到当前选择的项目；默认不采集。独立迁移、幂等存储、REST/WS 与桌面“Cindy · 只读”已通过合成项目端到端和原 Codex 回归。开发版已再次原位更新，宿主诊断核对新 build_id=d3452b2a…；实际浏览器的 320/390/1200 px 展示、实时重读和 54 条文本分页已验证。随包 CLI 不内嵌前端，页面验收使用独立 embed_frontend 构建。真实聊天仍只在私有临时样本中。格式、范围与待验项见 [手动归档](../docs/features/cindy-plugins/session-archive.md)。
