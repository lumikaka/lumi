# Step 4.5 · MCP Apps 宿主行为验收记录

日期：2026-10-30（Asia/Shanghai）。范围：macOS Codex App。关键日志与截图已归档到 [`spike/evidence/mcp-apps-host-20261030/`](spike/evidence/mcp-apps-host-20261030/archive-manifest.json)；原始夹具为 `/private/tmp/lumi-mcp-apps-6mxe4b0s`。文中带 Z 的事件时间沿用日志 UTC 时间。

**真实渲染、桥接、只读回调、写入临时 marker、工具错误回显均通过。iframe 本地六档 data URI，以及工具取图两种模式 × 六档，共 12 种回调组合全部加载成功，已测至 8 MiB padding。allow 轮 loopback 加载失败；no-resource 对照、写操作批准/拒绝流程仍未验证。**

Step 5 推荐改为 **App 内只读工具取图，通过 structuredContent 返回受控缩略图的 data URI；保留完整文本和「在 Lumi 中打开」**。image content 也已通过同样大小测试。Lumi 的生产信封与取图字段见 [Step 5 计划](step5-mcp-apps.md#图片传输与工具契约)。当前不依赖 loopback 或模型通知直接传图，不把 marker 成功外推为写操作无需批准。

## 环境与范围

| 项目 | 值 |
| --- | --- |
| App / build | 26.1027.30824 / 22943 |
| Bundle ID | com.openai.codex |
| 内置 CLI | codex-cli 0.166.0-alpha.5.1 |
| 内置 CLI 路径 | /Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex |
| 会话 | 01a18d23-7b86-7b32-b75e-d6fb190d727f |
| 插件 / 版本 | lumi-apps-allow-6mxe4b0s / 0.4.1+20261030133557.6mxe4b0s |
| UI URI | ui://lumi-spike/20261030133557.6mxe4b0s/allow/app.html |
| MCP PID / cwd | 6035 / /Users/qingyang/mine-work/lumi |
| 当时权限 | workspace-write / auto_review |
| HTTP | 127.0.0.1:18765；仅合成 reference.png |

版本来自已安装 bundle 及实际执行的内置 CLI；真实 UI 初始化另报告 hostInfo 为 chatgpt / 26.1027.30824。以下五项共用此环境。没有普通浏览器替代验证，没有修改产品代码、已安装探针或用户 ~/.codex。

初始侧栏 inventory 为空，后由用户截图和真实 tab DOM 证实 UI 已渲染；自动化中途曾超时，之后恢复。这些早期观测限制已被后续成功证据取代，不能继续写成所有宿主能力未验证。

## 1. show_app 后的真实渲染

**通过。** 用户截图和真实 MCP Apps tab DOM 均显示标题、版本、CSP allow 及“桥接：初始化成功”。UI 在 14:10:21.103Z 记录 script_started，instance 为 `1793369421103-eea7eb0e0e7e78`；14:10:21.219Z initialize 成功，protocolVersion 2026-01-26。

截图描述：用户先提供 Codex 侧栏截图，随后代理从真实 MCP Apps tab 保存的 [screenshots/host-local-and-callback-8m.png](spike/evidence/mcp-apps-host-20261030/screenshots/host-local-and-callback-8m.png) 显示标题、版本、桥接状态、loopback 失败和两张正确的合成图片。

## 2. Loopback 图片与 CSP 对照

**allow 轮失败；no-resource 对照未验证。** 14:10:21.127Z 的 loopback 事件为 failed / 0×0 / 13 ms。首轮 HTTP server 在首次 show_app 前已启动，到 14:13:30 前一直运行，没有收到 GET。该失败发生于 server 运行期间，不能仅因截图较晚提供而归因于 server 已停止。

探针源码 SHA256 与 manifest 一致，声明 `resourceDomains: ["http://127.0.0.1:18765"]`、`connectDomains: []`。真实 initialize 响应中 `hostCapabilities.sandbox.csp.resourceDomains` 却为 `[]`，其余 domains 同为空。已观察到声明与宿主报告的差异，尚未定位适配、策略或其他层面的原因，不能断言某一层 CSP 拒绝。

no-resource 插件未由用户切换并新开会话，不能给出完整 CSP 因果对照。服务器曾重启尝试复测，但未成功触发新的 loopback；现已停止，状态和重启命令见 [host-server-status.json](spike/evidence/mcp-apps-host-20261030/host-server-status.json)。没有健康检查或普通浏览器 GET 混入日志。

截图描述：用户及代理截图都显示 loopback 破图、failed / 0×0。没有 no-resource 截图。

## 3. Data URI 解码与工具结果 payload

| padding 字节 | PNG 字节 | URI 字符 | 模型工具至编排层完整性 | iframe 本地 | 回调 data_uri | 回调 image |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 0 | 411 | 570 | 通过 | 通过 | 通过 | 通过 |
| 65,536 | 65,947 | 87,954 | 通过 | 通过 | 通过 | 通过 |
| 262,144 | 262,555 | 350,098 | 通过 | 通过 | 通过 | 通过 |
| 1,048,576 | 1,048,987 | 1,398,674 | 通过 | 通过 | 通过 | 通过 |
| 4,194,304 | 4,194,715 | 5,592,978 | 通过 | 通过 | 通过 | 通过 |
| 8,388,608 | 8,389,019 | 11,185,382 | 通过 | 通过 | 通过 | 通过 |

- 六次模型发起的 show_app 结果完整到达工具编排层，已独立校验 base64、PNG 签名、chunk CRC、IEND、字节数和 IHDR。
- iframe 内本地六档均有 loaded / 128×128。8 MiB 已再次执行并保存正确图形截图。
- 两种工具回调模式的六档均有新的 tools/call、对应 request_token、服务端结果和 UI loaded / 128×128。共 12 个唯一组合，含重复验证在内有 13 次成功加载事件。
- 以上路径的已测下界为 **8 MiB padding、8,389,019 PNG 字节、11,185,382 URI 字符**；没有失败档，不能给出精确上限或把此大小定为生产默认值。六档均为同一张 128×128 合成 PNG，padding 是有效 `tEXt` chunk 中的填充字节，不是图像像素；不能据此推断真实大像素图片的性能。

模型宿主通知是另一条路径：只检查到对应 8 MiB 的 host_tool_result，其 `structured_keys: []`，唯一 text 内容起始为序列化的 `{content, structuredContent}`。当前探针仅读取顶层 structuredContent / image content，因此未取到该图片。不能从日志保留的前 700 字符判断完整通知是否截断；其余五档没有逐一核验。该结果不影响已独立验证的 UI tools/call 回调路径。

截图描述：[screenshots/host-local-and-callback-8m.png](spike/evidence/mcp-apps-host-20261030/screenshots/host-local-and-callback-8m.png) 同时显示本地 8 MiB 和工具回调 8 MiB 图片，均为左红右蓝、左上黄块。后者状态显示 loaded、padding_bytes=8388608。

## 4. 只读、写操作与错误回显

| 场景 | UI 与服务端对应证据 | 结论 |
| --- | --- | --- |
| inspect | 14:31:54.583Z 开始；MCP id 20；UI 回显 PID/cwd/build | 通过 |
| fail_tool | token 尾号 -4；MCP id 21；PROBE_EXPECTED_ERROR、isError=true、UI tool_error | 通过 |
| write_marker 首次 | token 尾号 -6；MCP id 22；UI 回显和 marker 落盘一致 | 通过 |
| write_marker 再次 | token 尾号 -10；UI 回显和第二条 marker 落盘一致 | 通过 |
| 批准弹窗与拒绝 | 没有明确的弹窗观察或拒绝用例 | 未验证 |

两个 marker 均位于此临时目录的 [markers.jsonl](spike/evidence/mcp-apps-host-20261030/markers.jsonl)，与 UI token、MCP 请求/结果及时间一致，没有修改 Lumi 数据。当前权限为 workspace-write / auto_review，但用户日志不记录批准弹窗；不能从快速成功或 marker 存在推断没有批准要求。没有代替用户点击批准。

这里的 fail_tool 是有意返回的工具错误，已与曾发生的自动化通道超时区分；没有证据把后者当作宿主策略拒绝或 bridge 超时。

## 5. App 内工具取图

**两种模式、六档全部通过。** 用户先完成 padding 0 的两种模式，自动化恢复后补齐 64 KiB、256 KiB、1 MiB、4 MiB、8 MiB。服务端 PID 6035 的请求、返回摘要及 UI token 全部可对应。

- data_uri：structuredContent 含 data_uri，iframe 显示正确图片。
- image：content 含 image，UI 取出 base64 后显示；最大档 image 块为 11,185,360 base64 字符。

取图期间没有 HTTP GET；两种模式均可作为无需 loopback 图片请求的方案，但仍依赖宿主 bridge、工具结果大小和 data URI 图片策略。

## Step 5 决策与回调范围

探针验证了 **App 内只读 get_image → structuredContent.data_uri → iframe img**。Lumi 的 [Step 5 计划](step5-mcp-apps.md) 将新增仅供 App 使用的 `read_app_image`，沿用统一成功信封，从 **`structuredContent.data.data_uri`** 读取预览，并复用现有 120 KiB 预览 / 256 KiB 结果限制；这里的字段调整属于生产实现计划，不是已经执行过的探针契约。保留完整文本及「在 Lumi 中打开」，标准 image content 可作为已测可行的备选。当前不用 loopback 作为图片依赖，也不依赖已观察到文本包装的模型通知直接传图路径。

只读回调、工具错误回显及临时 marker 写回调在本环境中已通过。真实业务写操作仍需沿用宿主批准和业务校验，批准/拒绝语义尚未完成验收。本步仅记录建议，没有实现产品代码。

## 证据与验证

以下文件从本轮临时目录按原字节归档，路径和会话标识只描述此次运行；归档范围与 SHA256 见 [archive-manifest.json](spike/evidence/mcp-apps-host-20261030/archive-manifest.json)。原始目录还保留用户截图、完整探针目录和验证脚本，仓库只归档下列关键证据，避免依赖临时目录才能复核结论。

- [acceptance-results.json](spike/evidence/mcp-apps-host-20261030/acceptance-results.json)、[host-environment.json](spike/evidence/mcp-apps-host-20261030/host-environment.json)、[acceptance-manifest.json](spike/evidence/mcp-apps-host-20261030/acceptance-manifest.json)：结论快照、实际版本与夹具标识。
- [host-ui-followup-events.json](spike/evidence/mcp-apps-host-20261030/host-ui-followup-events.json)：从真实 tab 读取的完整 UI 日志，含初始化、loopback 失败、模型通知形状、用户操作及代理补测。
- [host-callback-matrix.json](spike/evidence/mcp-apps-host-20261030/host-callback-matrix.json)：12 个唯一组合的 MCP id、request_token、SHA256、UI loaded 时间与大小。
- [callback-verification.json](spike/evidence/mcp-apps-host-20261030/callback-verification.json)、[markers.jsonl](spike/evidence/mcp-apps-host-20261030/markers.jsonl)：只读、两次 marker 写入、错误回显，以及批准/拒绝仍未验证的边界。
- [host-local-data-uri-observed.json](spike/evidence/mcp-apps-host-20261030/host-local-data-uri-observed.json)、[model-payload-results.json](spike/evidence/mcp-apps-host-20261030/model-payload-results.json)：六档本地解码与模型工具到编排层的完整性结果。
- [host-mcp-events.jsonl](spike/evidence/mcp-apps-host-20261030/host-mcp-events.jsonl)：本轮相关请求和结果的规范化摘录，去除无关会话元数据；不同于早期隔离 app-server 的预检日志。
- [host-http-events.json](spike/evidence/mcp-apps-host-20261030/host-http-events.json)、[host-server-status.json](spike/evidence/mcp-apps-host-20261030/host-server-status.json)：HTTP 请求快照及 server 停止状态。
- [screenshots/host-local-and-callback-8m.png](spike/evidence/mcp-apps-host-20261030/screenshots/host-local-and-callback-8m.png)：真实宿主中本地与工具回调 8 MiB 图片正确显示的截图。

已交叉核对 UI → MCP 请求 → MCP 结果 → UI loaded 的 token、模式、字节和尺寸，并核对两个 marker 与写请求一致。早期隔离 app-server 的 [36 组传输预检](spike/evidence/mcp-apps-transport.json)、[allow 日志](spike/evidence/mcp-apps-transport-allow.jsonl)、[no-resource 日志](spike/evidence/mcp-apps-transport-no-resource.jsonl) 保留为辅助证据；它们不代表宿主 iframe 验收。

剩余未验证项：用户按原流程切换 no-resource 插件并新开会话的 CSP 对照；写操作批准及拒绝用例；其余五档模型宿主通知。补验及用户手动安装、切换、卸载步骤见 [spike/README.md](spike/README.md#step-45--真实-codex-app-验收)。这些未验证项不阻塞已通过的只读工具取图方案，不能据此宣称 Step 4.5 所有子项都已通过。

协议参考：[OpenAI Docs · CSP](https://developers.openai.com/plugins/build/chatgpt-ui#content-security-policy-csp)。规范说明与上述宿主实测分开。
