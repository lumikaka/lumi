# Lumi Cindy 插件作者源码

`lumi/` 是独立作者目录，交付浏览器 `main.js`、零第三方依赖的 Node worker 与制作时编译的 Go CLI。当前只实现 [Step 1](../docs/features/cindy-plugins/step1-runtime-cli.md)，尚未安装或发布。

在仓库根运行：

```sh
node cindy-plugins/scripts/build.mjs
node cindy-plugins/scripts/verify.mjs
```

构建只支持当前已验证的 macOS arm64，使用本机 Go 1.27.3。它生成 `lumi/bin/lumi` 与 `lumi/build-info.json`；测试使用独立临时 HOME、项目与 data-dir，需要 loopback bind/connect 权限。测试结束核对并停止自己的隔离实例、移除临时目录。不会使用默认用户数据目录、安装插件或运行 Cargo/Rust。

随后用 Cindy 的 `ghost_forge_pack({dir: <仓库>/cindy-plugins/lumi})` 验证并打包，再运行：

```sh
python3 cindy-plugins/scripts/verify-package.py
```

不从已安装目录制作，不在运行时安装依赖。Go 二进制、build-info 与 `.cindy` 是 Git 忽略的生成物；包内包含 CLI SHA256、源 commit 和编译平台信息。现有 Lumi 图标复用自 `codex-plugins/lumi/assets/icon.png`。

manifest 的 `minCindyVersion: 0.1.121` 是本次 Forge scaffold 自动写入的本机版本字段；实际构建 sourceCommit 与官方 tag 不同，**这个字段不是最低正式兼容版本的证明**。本步仅交付本地开发包；正式发布前须取得符合当前 Skill 的正式版本证据并校准该字段。

运行链路和资源归属见 [runtime.md](../docs/features/cindy-plugins/runtime.md)，结果与未验证项见 [验收记录](../docs/features/cindy-plugins/acceptance/step1-runtime.md)。
