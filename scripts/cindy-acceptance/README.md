# Cindy 工作区隔离验收

仓库根运行 `node scripts/cindy-acceptance/workspace.mjs`。先完成 `make cindy-build` 和 `pnpm --dir web install`；无需安装插件或调用模型。

脚本只在新的系统临时目录创建 HOME、data-dir 和 A/B 合成作品，构建测试入口，打印 browser_url、manifest 与本进程 PID。测试直接复用产品内部 WorkspaceGateway、Runtime、共享页面及真实 CLI/REST/SQLite；页面授权由夹具模拟，不是 Cindy Host 接口。

浏览器可检查作品、章节列表、章节、单页、设定和候选。A 的修改不影响 B；标题保存、稿件采用及外部 `/mutate/a` 产生的 WS 提示均写入测试项目。`/stats` 只用于该夹具的请求计数，区分真实 WS 等待与业务重读。产品包不含 fixture-main.jsx，也没有这些 HTTP 路由。

结束时对打印的本进程发送 SIGTERM，脚本仅按最初记录的 nonce 清理自己的 CLI 和页面连接。不要停止其他任务的默认 development/production 后台。临时目录保留用于核对；正式证据只保留简明结果和必要截图，不把合成授权写成真实宿主通过。

原生 Forge 更新后可运行 `python3 scripts/cindy-acceptance/verify-installed.py '<已确认的 Cindy owner>/cindy-brain/_ns/_root'`，只读比对已安装开发/探针与当前包及作者树的全部文件，核对 CLI 模式和 SHA256。正式实例只按本轮首次观察比较 build_id/CLI，不读取作品或改动安装。该脚本用于本次 macOS 对齐记录，不扫描或默认选择其它 owner；需先按正常 Host 回执确认当前实例。结果写入 codex-alignment-evidence/media-installed-files-20261104.json，不代表原生页面或用户文件接收通过。
