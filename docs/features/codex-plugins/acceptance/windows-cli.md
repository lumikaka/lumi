# Windows CLI 实施与本地验收记录

日期：2026-11-04。分支：`feats/ai`。本次仅在当前分支修改源码并进行本地验证，没有提交、推送、触发 GitHub Actions 或发布 Release；没有运行 Rust/Cargo。

## 已实现

- `make codex-windows-build` 和原生 Windows `scripts/build-codex-windows.ps1`：构建带 Web/MCP Apps 的 x64 CLI，生成 SHA256；Windows 脚本记录分段耗时。
- PowerShell 5.1 兼容语法的安装器：原生宿主路径选择、固定版本、下载校验/缓存、离线资产、中文路径、插件注册和缓存验证、不可变版本目录、事务回滚/恢复、保留用户数据的卸载。
- 宿主选择优先显式 exe，其次运行中 Codex 祖先进程，再尝试唯一 PATH `codex.exe`。发现不到时要求显式路径；不猜 Store/App 目录，不通过 npm shell 包装器执行。
- 通用 ZIP/tar.gz 插件打包任务、独立 Windows CLI workflow、现有完整 Release 的资产汇总及校验。工作流只写入本地，尚未执行。
- 中英文安装入口和 Windows 预览指南；平台状态明确保留“待实机验收”。

## 本地验证结果

环境：macOS arm64，Go 1.27.3，Node 25.2.1；官方 PowerShell 7.6.9 便携运行时用于本地脚本测试。CI 继续使用仓库固定的 Node 26 和 pnpm 12.16.1。

| 检查 | 结果与边界 |
| --- | --- |
| `make codex-windows-build CODEX_WINDOWS_OUT=build/codex-windows-local` | 通过；实际前端构建与 Go 交叉编译，产物为 Windows x86-64 PE32+ exe。没有在 macOS 上执行 Windows exe |
| `make codex-package CODEX_VERSION=0.0.0-preview CODEX_OUT=build/codex-windows-local` | 通过；生成可供 Windows `-FromDir` 验证的完整本地资产 |
| `node --test scripts/test-package-codex.mjs scripts/test-dev-codex.mjs` | 4 项通过；独立 ZIP reader 验证 CRC、点文件及 tar.gz 内容对应，检查 SHA256；开发插件生成回归通过 |
| `python3 scripts/test-install-codex-unit.py` | 现有 macOS 安装器 10 项回归通过 |
| `pwsh -NoProfile -File scripts/test-install-codex-windows.ps1 -Assets build/codex-windows-local` | macOS PowerShell 7 下 15 组场景通过；宿主与网络为隔离替身，文件操作为真实操作。Windows 专属占用句柄用例不在 macOS 执行 |
| `go test -timeout=15m ./cmd/lumi ./internal/textmcp` | 通过，临时数据与 loopback 服务；没有使用用户项目 |
| `python3 scripts/test-codex-cli.py --binary <本机原生编译的 lumi>` | 通过；验证真实本机 CLI 的后台单例、嵌入 Web、stdio MCP、中文项目路径、页面准备签名、合成图片导入、PDF/ZIP 导出及停止重启 |
| Hugo 构建与 `site/scripts/check-build.sh` | 通过；32 个路由/资源和两种语言的搜索索引有效 |
| 工作流 YAML、job 依赖、Markdown 本地链接、`git diff --check` | 本地静态检查；不等同于 GitHub runner 实际执行 |

安装器场景包括原生命令 UTF-8/错误流/退出码、首次/重复安装、缓存命中与损坏、离线损坏资产、目标市场冲突/损坏、注册失败回滚、回滚失败保留事务和下次恢复、策略拒绝、版本并存、锁和目录归属、ZIP 路径穿越/Windows 保留名称/大小写重复、版本混用、非归属事务保护、卸载残留与幂等清理、首次安装失败的注册撤销。

本地完整资产位于忽略目录 `build/codex-windows-local/`，版本标识为 `0.0.0-preview`。它们不是公开 Release，也不表示 Windows 运行验收已完成。

## 仍待 Windows 原生验证

1. 在 Windows runner 上运行 PowerShell 5.1 和 7 安装器测试，包括真实文件占用句柄；执行真正 Windows exe 的 CLI/MCP/导出冒烟与 Go 定向测试。
2. 在真实 Windows Codex App 核实祖先进程发现/显式 exe 路径、插件注册、实际缓存和新会话工具发现；记录 App、CLI、系统版本。
3. 验证项目写保护、宿主参考图读取、生图工具返回路径和立即导入、MCP Apps 页面交互、PDF/ZIP 交付。
4. 真实会话仍存活时升级，工作结束后停止旧服务并重连；检查控制台窗口、旧版本延迟清理、卸载后的项目保留。
5. 从 Windows CI 记录冷/热缓存的分阶段耗时。当前未测得 Windows 构建时间，不承诺具体分钟数。

这些项不能由 macOS 交叉编译、PowerShell 替身测试或合成图片替代。完成前保留 Windows 预览标识。
