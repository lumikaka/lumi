# Lumi MCP 工作区验收夹具

这些开发脚本仅使用隔离测试项目，不是应用运行时依赖。验收摘要位于 [docs/features/codex-plugins/acceptance](../../docs/features/codex-plugins/acceptance/README.md)。

## 浏览器集成夹具

```bash
pnpm --dir web build
go build -tags embed_frontend -o /private/tmp/lumi-acceptance-cli ./cmd/lumi
PYTHONDONTWRITEBYTECODE=1 python3 scripts/codex-acceptance/hierarchy_fixture.py --binary /private/tmp/lumi-acceptance-cli
```

`hierarchy_fixture.py` 创建新的系统临时目录，内含独立数据目录、22 章测试作品、25 页、空作品、候选及设定；只复用仓库图标，不触发生图或导出。它打印浏览器地址和 manifest，浏览器通过真实 stdio→CLI REST→SQLite 读写测试数据。停止脚本时关闭自己启动的 MCP 会话；不管理其他项目或服务。

可使用 `--reuse` 继续同一夹具。该参数只接受系统临时目录中由此流程创建的 `lumi-review-hierarchy-*` 目录；不要传入真实作品。运行时生成的 HTML、SQLite、manifest 和请求记录留在该临时目录。

夹具模拟 MCP 宿主桥接，可验证界面、导航、REST 事实与 `_meta` 传输；它不能代替真实 Codex App 的面板、权限、文件选择及生命周期验收。

## 真实宿主 stdio 记录器

`log_stdio.py` 接受一个隔离夹具根目录，其中 `manifest.json` 提供 binary、project 和 data_dir。它透明转发 JSON-RPC，把请求及结果写到该目录的 `host-mcp.jsonl`，图片、HTML 和连接票据保留长度/摘要。仅用于自己的测试连接，不自动安装或切换插件。

完整业务文本可能出现在记录中，所以使用测试内容并将输出留在仓库外。正式保留结果时，只更新验收摘要和少量必要截图。
