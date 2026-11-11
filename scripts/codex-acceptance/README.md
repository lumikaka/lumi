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

## 页面参考合图夹具

```bash
make codex-dev-build
PYTHONDONTWRITEBYTECODE=1 python3 scripts/codex-acceptance/page_image_fixture.py --binary "$PWD/build/codex-dev/lumi_dev"
```

夹具通过真实 stdio MCP 创建四项中文设定和一页分镜，使用仓库图标作为明确的图片替身。它返回 `manifest`、真实 `reference_image_path` 和正常 Web 页面地址。使用宿主读图工具实际查看该路径，再执行：

```bash
PYTHONDONTWRITEBYTECODE=1 python3 scripts/codex-acceptance/page_image_fixture.py --complete /实际返回的/manifest.json --viewed-path /实际返回的/合图.png
```

完成阶段明确模拟宿主 imagegen，只记录 `referenced_image_paths=[同一路径]` 与原提示词，再经真实 MCP 导入相同 preparation。此夹具不调用图片模型、不消耗生图额度，不能证明真实宿主 imagegen 已接受图片。manifest 保留输入、摘要及导入回执。结束后用 manifest 中的 binary 和 data_dir 执行 `stop --data-dir`，只停止该隔离后台；不停止其他开发实例。

## 两章串行制作夹具

```sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/codex-acceptance/serial_production_fixture.py --binary "$PWD/build/codex-dev/lumi_dev"
PYTHONDONTWRITEBYTECODE=1 python3 scripts/codex-acceptance/serial_production_fixture.py --verify /实际返回的/manifest.json
PYTHONDONTWRITEBYTECODE=1 python3 scripts/codex-acceptance/serial_production_fixture.py --snapshot /实际返回的/manifest.json
```

创建第 10、11 章共四页，场次为第 10 章页 1–2、页 3，第 11 章页 1。初始状态依次为 passed、pending、missing_image、missing_image；前两页及设定使用明确的图标替身，审阅记录也是协议测试替身，不代表真实画面通过。`--verify` 通过新 MCP 连接重读，断言初始采用版本与状态不变，不调用生成或导入。真实宿主开始制作后改用 `--snapshot` 读取当前版本和审阅摘要，不再要求保持初始状态。

真实宿主测试须将返回的 project 目录添加到 Codex App；不能把 Lumi 源码目录代替测试作品。可使用 `log_stdio.py` 绑定 manifest 指定的隔离后台，保存实际工具轨迹。真实生图、设定返修、场景代理和章节 thread 要单独执行、记录，不能用此夹具或 Go 协议测试宣称已完成。Claude Code 的宿主验收使用其实际聊天能力与服务商路径，不套用 Codex 宿主生图。
