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

## Codex 小说分镜与用量对照

修改技能前将 `codex-plugins/lumi/skills/create-picture-book` 复制到仓库外作为基线。分别用基线和同步后的技能创建两个独立夹具：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/codex-acceptance/storyboard_fixture.py --binary "$PWD/build/codex-dev/lumi_dev" --baseline-skill /实际基线/create-picture-book
PYTHONDONTWRITEBYTECODE=1 python3 scripts/codex-acceptance/storyboard_fixture.py --binary "$PWD/build/codex-dev/lumi_dev" --baseline-skill "$PWD/codex-plugins/lumi/skills/create-picture-book"
```

两者使用脚本内同一篇原创小说、相同 setup、两章原文且零页面；UUID 和临时路径不同。脚本只经真实 MCP 初始化，保存源文指纹，复制指定技能并替换该副本的 CLI 入口。不会安装插件、改全局配置或触发生图。将两个返回的 project 添加到 Codex App，实际信任由用户处理。项目本地 MCP 记录器绑定各自的隔离后台。

两次用同一 prompt、默认模型和推理设置，核对宿主实际模型没有变化；按章建独立聊天，每章 3 页、直接引语逐字保留。统一结束于两章分镜核对写回、划场和连续性交接保存，生图之前。保留实际根聊天与章节聊天、subagent 身份。另在已有分镜的项目发起只配图请求，观察无分镜委派；这条路径的用量不混入 A/B 数字。中断／重做等额外验收也单列。

完成后通过 MCP 保存事实快照，再在继续任何额外验收前提取真实宿主 rollout 用量：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/codex-acceptance/storyboard_fixture.py --snapshot /实际夹具/manifest.json
PYTHONDONTWRITEBYTECODE=1 python3 scripts/codex-acceptance/storyboard_audit.py --manifest /实际夹具/manifest.json --root-thread 实际总聊天UUID --sessions-dir /实际Codex/sessions/年/月/日 --output /实际夹具/usage-audit.json
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts/codex-acceptance -p test_storyboard_audit.py
```

统计器按准确项目目录及时间窗收集总聊天、独立章节聊天、全部 subagent，每条会话只取最后一次累计计数，以 `last_token_usage.input_tokens` 求总聊天单次输入峰值。未缓存输入为 input 减 cached；output 已含 reasoning，不重复相加。缺失真实用量或累计计数重置时报错，不填零或估算。根聊天工具回显检查覆盖原文段落及整章分镜，还须人工检查原文读取调用、章节串行顺序、`fork_turns`、草稿写入边界及页数／对白／说话人／连续性。统计器单元测试只证明计数与检测逻辑，不代表真实宿主验收。

`workflow_totals` 汇总总聊天、章节聊天及创作 subagent；`totals` 还包括同一项目和时间窗内有真实计数的宿主辅助自动审查会话，单独标记为 `host_auxiliary`。两种口径均保留，避免隐去审查开销。

原始小说、草稿、完整工具历史与用量证据保留仓库外；仓库只保存验收结果与准确限制。不能把总聊天上下文下降等同于全流程 token 节省。
