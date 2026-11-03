# 安装与插件自动化验收

日期：2026-10-31（Asia/Shanghai）。历史记录覆盖源码构建、本地 Release 夹具、真实 App 内置 CLI 和 app-server，不代表真实用户安装、模型自主创作、原生生图或远程发布 CI 已验收。

当时环境为 macOS 14.5 arm64、App 26.1027.30824 / build 22943、内置 codex-cli 0.166.0-alpha.5.1、Go 1.25.0。全部项目、配置和安装目录位于隔离临时目录，测试结束停止自己的 CLI 并卸载自己的插件。

## 复跑

在仓库根目录准备本地 Release 资产后运行：

```bash
make codex-release CODEX_VERSION=0.1.0 CODEX_OUT=/private/tmp/lumi-step6-release
PYTHONDONTWRITEBYTECODE=1 python3 scripts/test-install-codex.py --assets /private/tmp/lumi-step6-release
```

每次运行使用新的临时目录，完整命令、RPC、安装报告和导出物留在该目录，不提交到仓库。安装方法和当前人工验收项见 [安装文档](../install.md)。

## 当时通过的检查

- 重复安装结果一致，含空格路径可用；生成绝对 stdio 入口且不固定 cwd。
- SHA256 错误、marketplace 冲突和配置写入失败保留原安装及配置；升级刷新缓存且重复执行幂等。
- App loader 识别插件和 skill，发现并调用 MCP 工具。
- 空目录完成初始化、setup、页面正文、合成 PNG 导入和完整 PDF 导出；PDF 文件头及零缺图事实已核对。
- 离线卸载及重复卸载保留项目与其他 Codex 配置。
- Skill 校验、Hugo 构建与站点检查，以及安装/打包脚本和 workflow 语法检查通过。

没有运行 Cargo/Rust，没有账号凭据、API key 或模型调用。初次 loopback bind 在命令沙箱内失败，随后经当时宿主审批在外层沙箱运行同一隔离测试；不把它算作真实用户安装批准次数。

## 未验证范围

真实 HTTPS Release 下载、PATH fallback、Intel App、Developer ID/Apple 公证、Gatekeeper、真实 App 一句话安装及批准界面、自主创作与原生图导入未在本轮实机验证。测试 PDF、安装状态和原始报告已归档，不作为源码保留。
