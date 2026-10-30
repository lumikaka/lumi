# 一次性 Codex 宿主探针

结论见 [`../spike.md`](../spike.md)。依赖：已安装 Codex CLI、Python 3.10+、git。没有 pip/npm/Cargo 依赖，不使用用户认证。

所有命令从 Lumi 仓库根目录运行。脚本在 `/tmp` 下创建新目录并打印路径；macOS 上解析为 `/private/tmp`。不会自动删除证据目录，也不会写用户真实 `~/.codex/config.toml`。

## 运行

```sh
codex --version
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/run_probe.py

# 可选：让真实 MCP 读取本地图片；可使用已提交的测试图
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/run_probe.py \
  --image "$PWD/docs/features/codex-plugins/spike/evidence/fox-original.png"

# 可选：比较 App bundle 中的 CLI
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/run_probe.py \
  --codex /Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex

# 项目信任、权限配置、hook 发现；打印独立 sandbox_command
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/config_hooks_probe.py
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/config_hooks_probe.py \
  --codex /Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex

# 仅执行生成器提供的已审阅 hook；单次信任 bypass，不持久化信任
# 本地离线 provider 不会产生成功模型回复，5 秒后终止整个测试进程组
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/config_hooks_probe.py \
  --run-reviewed-hooks
```

然后执行配置探针打印的 `sandbox_command`。它只向新建夹具目录的假数据库、假素材文件尝试写入、原子替换、删除和改名。若从另一层 Codex 沙箱运行，可能需要批准在外层之外启动被测 sandbox；不要用真实 Lumi 项目替换夹具目录。

生成器内部创建 marketplace 的全部 JSON，可作为最小插件。`run_probe.py` 使用真实 JSON-RPC stdio app-server，拒绝任何意外批准请求，不发起模型 turn。资源 HTML 的传输成功不等于实际渲染成功。

`call_probe.py ROOT reference_image '{}'` 通过同一临时插件返回 MCP 图片内容。它的输出是 JSON；让宿主转呈其中 image 块才会进入模型视觉上下文。工具结果本身没有自动给另一条 CLI thread 添加图片。

## 文件

| 文件 | 用途 |
| --- | --- |
| `run_probe.py` | 生成/安装最小插件；两个不同 cwd 的 thread；重复 inspect；图片读取；UI 工具关联和资源读取 |
| `probe_server.py` | 无依赖 stdio MCP；提供 inspect/read_image/reference_image/show_app，以及 get_image/write_marker/fail_tool 验收工具 |
| `config_hooks_probe.py` | 生成 trusted/untrusted 项目和权限/hook 夹具，可选执行已审阅测试 hook |
| `hook_probe.py` | 记录测试 hook stdin 与 transcript 是否存在 |
| `write_probe.py` | 只应在生成器创建的临时夹具 cwd 执行 |
| `app.html` | MCP Apps 探针；loopback、本地六档 PNG data URI、读写/错误回调、工具取图；实际 GUI 结论见验收文档 |
| `prepare_app_acceptance.py` | 仅生成两套独立版本、独立资源 URI 的临时 marketplace，不安装插件 |
| `serve_acceptance_images.py` | 仅在 127.0.0.1 提供合成 reference.png；禁用缓存并记录每次请求 |
| `check_app_transport.py` | 隔离 CODEX_HOME 的非可视传输预检；不代表 App 渲染或批准验收 |
| `evidence/` | 本次实际结果；路径/PID/UUID/时间只属于此次运行 |

## 本次证据

| 文件 | 来源 |
| --- | --- |
| `stdio-image.json`、`stdio.jsonl` | PATH CLI `0.163.0` 的 app-server/RPC/MCP 记录 |
| `app-bundled-stdio-image.json`、`app-bundled-stdio.jsonl` | App bundle CLI `0.166.0-alpha.5.1` 同项记录 |
| `config-hooks.json`、`hooks.jsonl`、`hook-transcript-metadata.json` | CLI `0.163.0`，信任发现和真实离线轮次的两个 hook 输入；不含完整 rollout |
| `sandbox-writes.json` | CLI `0.163.0`，macOS Seatbelt 下的真实文件操作结果 |
| `app-bundled-config-hooks.json`、`app-bundled-sandbox-writes.json` | App 内置 CLI `0.166.0-alpha.5.1` 的 trusted/untrusted 配置读取、profile 选择与真实沙箱文件操作 |
| `app-manual-write-confirmation.json` | 用户在当前 App 的临时假项目中人工确认的原话与证据边界；无独立 App 工具日志 |
| `image-generation.json` | 本次 App 三次内置生图的原始参数、返回字段名及落盘提示；未重复保存 base64 |
| `fox-original.png`、`fox-boots.png` | 真实 App 生成/参考编辑，1536×1024 PNG |
| `mcp-reference-result.json`、`mcp-reference.png` | 真实 stdio MCP image 结果及其原始 PNG，128×128 |
| `mcp-reference-ring.png` | MCP 图片转呈进入对话后的参考编辑，1254×1254 |
| `subagents.json` | 两次真实 Subagent 调用与返回报告的整理记录；不是 CLI rollout |
| `ImageGenerationItem.ts`、`HookEventName.ts` | `codex app-server generate-ts --out /tmp/...` 导出的 `0.163.0` 类型 |
| `app-inventory.json` | 当前 App 的 MCP Apps 通道存在且 tabs 为空；不含其他应用/浏览器资料，不代表渲染通过 |
| `mcp-apps-acceptance-environment.json` | Step 4.5 的 bundle/内置 CLI 版本、人工夹具标识、当前入口与证据边界 |
| `mcp-apps-transport.json`、`mcp-apps-transport-allow.jsonl`、`mcp-apps-transport-no-resource.jsonl` | Step 4.5 非可视预检：两套 CSP、工具注解、临时 marker、预期错误和 36 组图片传输；不代表宿主 iframe 验收 |

App iframe、loopback、data URI 上限和 UI 回调没有成功观测记录，不能从这些文件推导为通过。所有 PNG 均为本次测试新建内容。

## Step 4.5 · 真实 Codex App 验收

结论记录在 [`../mcp-apps-acceptance.md`](../mcp-apps-acceptance.md)。以下工作只使用合成图片和临时 marker，不读写真实 Lumi 数据。没有 pip/npm/Cargo 依赖。

### 生成与预检

```sh
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/prepare_app_acceptance.py
```

保存脚本打印的 `root` 和 `acceptance-manifest.json`。里面有 App bundle / 内置 CLI 版本、源文件摘要、两套 marketplace 的路径、插件名称/版本/资源 URI，以及逐个注册/移除 marketplace 的命令。每次复测重新生成；不要编辑已经安装的夹具来更换 CSP。

可选非可视预检（建议另生成一套 root，以保持 GUI 的 MCP 日志和 marker 干净）：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/check_app_transport.py /private/tmp/lumi-mcp-apps-REPLACE
```

它只向**临时 CODEX_HOME**安装测试插件，启动 App 内置 CLI 的 app-server，不使用用户认证、不发模型 turn、不批准请求；检查两套 CSP、工具注解、临时 marker、预期错误及 36 个图片结果的字节/CRC/SHA256。`transport-report.json` 的成功只表示 app-server 传输和 PNG 完整性，不证明 iframe 解码、UI 回调或批准流程。

### 用户手动安装第一轮

1. 执行 manifest 的 `serve_command`（保持终端打开），只监听 `127.0.0.1`；遇到端口占用就用 `--port` 重新生成整套夹具，勿让 HTML 和 CSP 的端口不一致。不要用普通浏览器打开 HTML。
2. 用户手动执行 `allow` 的 `manual_register_command`，在 Codex App 插件目录安装该轮 `plugin`。若目录未刷新，重启 App 再安装。**代理不运行这些注册、安装、卸载或移除命令，也不编辑用户 `~/.codex`。**
3. 新开同项目会话，明确要求调用这一个探针的 `show_app({})`。确认界面显示 manifest 中的版本和 `CSP allow`，保存真实宿主截图及描述。若入口不存在或不渲染，记录停在哪一步；不以 resource/read 替代。
4. 只用真实 `Codex MCP Apps` tab 或人工观察执行下列按钮。工具不可见时可从原始 MCP 日志定位，不拿 app-server 的直接调用当作按钮验收。

### 逐项动作与取证

| 项目 | 动作 | 通过证据 |
| --- | --- | --- |
| 渲染 | 模型调用 `show_app({})` | 真实 App 截图含标题/版本、桥接状态；有 `script_started` 和初始化结果 |
| Loopback | 等待参考图 | 图为左红右蓝、左上黄块；`loopback: loaded` 对应 `128×128`；`http.jsonl` 有同版本、variant、instance 的 GET 200 |
| iframe data URI | 点击「测试本地全部六档」 | 每档的 `local_data_uri` loaded/failed/timeout，padding、URI 字符数和宽高；截图看图形正确 |
| 只读回调 | 点击「只读 inspect」 | 点击时刻之后 `mcp.jsonl` 有新的 `tools/call inspect`，UI 回显 build/PID/cwd；记录是否出现批准弹窗 |
| 写回调 | 点击「写入 marker」，按实际弹窗批准或拒绝 | 对照 request_token、新的 `tools/call`、`markers.jsonl` 是否新增以及 UI 回显；拒绝时不应有新增 marker。不要代替用户点击批准 |
| 错误反馈 | 点击「预期工具错误」 | 新的 tools/call、`PROBE_EXPECTED_ERROR` / `isError: true` 回显；与宿主拒绝、JSON-RPC 错误、30 秒未响应分别记录 |
| 工具取图 / payload | 对六档分别点击「工具返回 data URI」和「工具返回 image content」 | 每次新的 get_image 请求及结果摘要；UI 得到完整结果后显示正确的 128×128 图；记录 bytes/base64_chars/uri_chars |
| 模型工具结果 payload | 六次分别让模型调用 `show_app({"padding_bytes": N})` | host_tool_result 事件、完整字段、图片 loaded 和正确图形；不要把 callback 路径代替这条路径 |

padding `N` 为 `0, 65536, 262144, 1048576, 4194304, 8388608`，单位是有效 PNG `tEXt` chunk 中的填充字节，并非 URI 长度或图片像素。两种工具取图均不发 HTTP 请求；仍依赖宿主允许 data URI 图片和将完整工具结果交给 iframe，不能先声称“绕过所有 CSP”。

每次记录 App 版本、**内置** CLI 版本、插件版本和资源 URI、会话、时间、截图路径与描述、UI 事件和对应 MCP/HTTP 请求。成功值只作为该版本该路径的**已测下界**。超时、截断、宿主拒绝或 PNG 解码失败分别描述；没有定位拒绝层时不宣称得到精确 payload 上限。写回调未弹批准框时记录当时权限模式，不推断所有模式均无需批准。

### 对照轮与卸载

用户先在 App 卸载 `allow`，再注册/安装 `no-resource` 版本并**新开会话**。两轮均为 `connectDomains: []`，控制轮**省略** `resourceDomains`，其他探针逻辑一致。插件名、版本、资源 URI 和图片 query 不同，HTTP 同时使用 `Cache-Control: no-store`。再次确认新版本标题、loopback 图/状态和 GET 日志。若只看到白屏、超时或没有 HTTP 请求，先检查资源/初始化及 server 状态，不直接归因为 CSP。

验收后由用户在 App 卸载两轮探针，并分别执行 manifest 中的 `manual_remove_marketplace_command` 移除两个测试来源。终端 Ctrl-C 停止图片 server；确认测试插件不再列出，不要求删除其他插件或清理用户配置。保留夹具证据直到需要的截图和日志归档。
