# Step 4.5 · MCP Apps 宿主行为验收记录

日期：2026-10-30（Asia/Shanghai）。范围：macOS Codex App。交付仅为本文和 `spike/` 内的一次性探针、证据。

**当前结论：五项真实宿主行为均未验证，不能把非可视传输预检当作 iframe 验收通过。Step 5 当前采用完整文本结果 +「在 Lumi 中打开」；候选编号由用户在对话回复选择。暂不依赖 iframe 图片或 UI 工具回调。**

本次已生成可安装的两套探针，并请用户手动安装第一轮。记录时尚未获得真实 App 的 `show_app` 调用、截图或按钮回调证据。任务明确要求用户自行安装并在验收后卸载，因此代理没有安装测试插件、没有改动用户 `~/.codex`，也没有用普通浏览器加载 HTML 代替宿主。以下「未验证」表示缺少这条真实宿主链路，**不表示已证实不支持**。

## 环境、证据与边界

| 项目 | 本次值 |
| --- | --- |
| App bundle | `/Applications/ChatGPT.app`，bundle ID `com.openai.codex` |
| App 版本 / build | `26.1027.30824` / `22943` |
| 内置 CLI | `/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex` |
| 内置 `codex --version` | `codex-cli 0.166.0-alpha.5.1` |
| 版本来源 | 读取已安装 bundle 的 Info.plist，并实际运行上面的内置二进制；不是 PATH CLI，也未核验运行中 renderer 的二进制指纹 |
| 真实 App 观察 | `cua.getState`、`cua.listTabs` 可见 `Codex MCP Apps` 通道，当前 chat 的 side-panel tabs 为 `[]`；当前工具集没有此探针的入口 |
| 观察限制 | 该 inventory 只枚举侧栏 tab；空列表不排除内联 App，也不证明 renderer 失败。没有发起真实 App 的探针调用，故没有对应截图 |
| 人工夹具 | `/private/tmp/lumi-mcp-apps-6mxe4b0s`，曾启动仅监听 `127.0.0.1:18765` 的合成图片 server，零请求；准备结束后已停止，人工验收前需重新运行 manifest 的 `serve_command` |
| 非可视预检夹具 | `/private/tmp/lumi-mcp-apps-rexuk97g`，使用独立临时 CODEX_HOME；未使用用户认证、未发起模型 turn |

证据：[`环境与当前入口记录`](spike/evidence/mcp-apps-acceptance-environment.json)、[`传输预检报告`](spike/evidence/mcp-apps-transport.json)、原始 MCP JSONL（[`allow`](spike/evidence/mcp-apps-transport-allow.jsonl)、[`no-resource`](spike/evidence/mcp-apps-transport-no-resource.jsonl)）。JSONL 仅属于临时 app-server，**不是 App 内按钮调用日志**；大图仅记录字节数与 SHA256，没有重复保存 base64。

完整安装、按钮动作、日志对应和卸载步骤见 [`spike/README.md · Step 4.5`](spike/README.md#step-45--真实-codex-app-验收)。临时路径只属于本轮；复测应重新运行生成器，使用新打印的 root。

## 1. 调用 show_app 后的真实渲染

- App `26.1027.30824` / build `22943`；内置 CLI `0.166.0-alpha.5.1`。
- **结论：未验证。** 当前 chat 未暴露一次性探针工具，尚无用户安装后的真实 `show_app` 调用。
- **截图描述：无该项宿主截图，尚未出现可观察的探针 UI。** 未将资源读取响应、HTML 源码或普通浏览器截图当作画面。
- 已验证的辅助项：隔离 app-server 能发现两套 `show_app` 的资源关联，并读到 `text/html;profile=mcp-app` HTML 和 CSP。这只能证明传输。
- 复测通过条件：真实 App 截图显示「Lumi MCP Apps 验收探针」、本轮版本与 CSP variant，UI 日志有 `script_started`；另记录初始化成功或实际错误，区分“HTML 已显示”和“桥接已初始化”。

## 2. Loopback 图片与 CSP 对照

App `26.1027.30824` / build `22943`；内置 CLI `0.166.0-alpha.5.1`。

| 轮次 | 插件版本 / 资源 URI | CSP | 真实图片 / HTTP 请求 | 截图描述与结论 |
| --- | --- | --- | --- | --- |
| allow | `0.4.1+20261030133557.6mxe4b0s`；`ui://lumi-spike/20261030133557.6mxe4b0s/allow/app.html` | `resourceDomains: ["http://127.0.0.1:18765"]`；`connectDomains: []` | 尚未发起 App 图像加载；没有对应 GET 证据 | 无探针图片截图；**未验证** |
| no-resource | `0.4.2+20261030133557.6mxe4b0s`；`ui://lumi-spike/20261030133557.6mxe4b0s/no-resource/app.html` | **省略** `resourceDomains`；`connectDomains: []` | 尚未安装对照轮或新开验收会话 | 无对照轮截图；**未验证** |

本次非可视预检验证了两个 CSP 的差异，但无法决定宿主最终策略。官方 [Add UI / CSP](https://developers.openai.com/plugins/build/chatgpt-ui#content-security-policy-csp) 将图片 origin 归入 `resourceDomains`；这不构成允许 HTTP loopback 的实测依据。

探针使用不同插件名、版本、资源 URI、图片 query（build/variant/instance），HTTP 使用 `Cache-Control: no-store`。人工验收必须先卸载上一轮，再安装下一轮并新开会话；核对标题版本、图片图形及同 instance 的 GET 200。server 未运行、资源未渲染、桥接未初始化、未发起请求均须单独记录，不能只凭“没有 GET”归因于 CSP。

## 3. Data URI 解码与工具结果 payload

App `26.1027.30824` / build `22943`；内置 CLI `0.166.0-alpha.5.1`。

**真实 iframe 的六档解码、UI callback payload 和模型工具结果 payload 均未验证。截图描述：没有六档宿主图像或错误截图。没有可报告的 App 图片成功下界，也没有可报告的 App payload 上限。**

下表的“通过”仅来自内置 CLI 隔离 app-server 的预检。每套 CSP 都检查 `get_image(data_uri)`、`get_image(image)`、`show_app(padding_bytes)` 三种结果，共 `2 × 6 × 3 = 36` 次，收到完整字节后逐一验证 PNG chunk CRC、像素数据长度和 SHA256。

| PNG padding | PNG 字节数 | 最终 data URI 字符数 | app-server 三种结果传输（两套 CSP） | iframe 本地生成 / callback / 模型通知 |
| --- | ---: | ---: | --- | --- |
| 0 | 411 | 570 | 通过 | 全部未验证 |
| 64 KiB（65,536） | 65,947 | 87,954 | 通过 | 全部未验证 |
| 256 KiB（262,144） | 262,555 | 350,098 | 通过 | 全部未验证 |
| 1 MiB（1,048,576） | 1,048,987 | 1,398,674 | 通过 | 全部未验证 |
| 4 MiB（4,194,304） | 4,194,715 | 5,592,978 | 通过 | 全部未验证 |
| 8 MiB（8,388,608） | 8,389,019 | 11,185,382 | 通过 | 全部未验证 |

padding 是有效 PNG `tEXt` chunk 的填充字节，不是 URI 长度。最大一档 `get_image(data_uri)` 的工具结果序列化 JSON 为 `11,185,958` 字节（按报告的 Python JSON 编码计量，不含 JSON-RPC 外层/换行）。此值只能作为本版本 **app-server 直接调用传输路径**的已测下界；该预检不经过 App renderer、模型上下文裁剪或 iframe bridge，不能当作它们的限制。

复测应分三条链路记录：

1. iframe 点击「测试本地全部六档」：图片在 iframe 内生成，不测试 MCP payload。
2. 每档分别点击「工具返回 data URI」/「工具返回 image content」：测试 UI `tools/call` 返回完整结果并显示。
3. 每档由模型调用 `show_app({"padding_bytes": N})`：测试模型工具结果通过 `ui/notifications/tool-result` 到达 iframe 并显示。

成功只表示该版本该路径的已测下界。若某档失败，应记录截断/JSON-RPC 错误/宿主拒绝/超时/图片解码失败及发生层；未定位层级前不宣称得到了精确大小上限。

## 4. 只读工具、写工具与错误回显

App `26.1027.30824` / build `22943`；内置 CLI `0.166.0-alpha.5.1`。

| 场景 | 探针能力 / 非可视预检 | 真实回调与批准结论 | 截图描述 |
| --- | --- | --- | --- |
| 只读 | `inspect` 显式 `readOnlyHint: true`，visibility 包含 app；直接调用能返回 build/PID/cwd | **未验证** UI 点击是否产生新的 tools/call、是否有批准弹窗、结果是否回显 | 无按钮点击及批准截图 |
| 写 | `write_marker` 显式 `readOnlyHint: false`、`destructiveHint: false`、`idempotentHint: false`，仅 app 可见；直接调用确实追加临时 marker | **未验证** App 的写调用、批准、拒绝及回显；直接 RPC 不是批准测试 | 无批准/拒绝/写入反馈截图 |
| 工具错误 | `fail_tool` 返回 `isError: true` 和 `PROBE_EXPECTED_ERROR`，直接传输通过 | **未验证** UI 错误回显；也未验证宿主策略拒绝或桥接错误 | 无错误反馈截图 |

写工具不接受任意路径，仅可向生成器创建的 `/tmp` 或 `/private/tmp/lumi-mcp-apps-*` 目录内 `markers.jsonl` 追加合成记录；默认旧探针未启用此临时目录时会拒绝写入。它不读取或修改 Lumi 数据库。

人工复测以点击时间和 request_token 对应新的 MCP `tools/call`、server 结果、marker 与 UI 回显。记录当时权限模式、是否弹窗、用户批准/拒绝及实际错误；未弹窗不能外推为所有模式都无需批准。探针 30 秒未响应只记录超时，不当作宿主拒绝。不能从只读按钮成功推断写回调可用。

## 5. App 内调用工具取图

App `26.1027.30824` / build `22943`；内置 CLI `0.166.0-alpha.5.1`。

- **结论：未验证真实 App 中的候选方案。截图描述：无工具返回图片在宿主 iframe 中显示的截图。**
- 探针 `get_image` 可返回 `structuredContent.data_uri` 或标准 `content.type=image`；UI 分别提取后赋给 `<img>`。两种结果在隔离 app-server 的六档完整传输通过，不能证明 App 将它们完整交给 UI。
- 此路径不依赖本地 HTTP 请求；若真实宿主中回调和显示均通过，可减少对 loopback origin 的依赖。但它仍依赖宿主 bridge、工具结果体积及最终 data URI 的 `img-src` 策略，不能称为绕过所有 CSP。
- 复测须同时有新的 get_image `tools/call`、结果摘要/字节数、iframe 内正确图形与 loaded/128×128。对 `data_uri` 和 `image` 两种模式分别记录；对话里出现一张 MCP 图片不等于 iframe 取图成功。

## Step 5 决策与回调可用范围

**当前执行方案：Step 5 只做完整文本结果 +「在 Lumi 中打开」。** 返回可审阅的文字、候选编号和正常的 Lumi 入口，由用户在对话回复编号，模型调用既有工具推进流程；完整图片与编辑在 Lumi 中打开。此结论不要求本步实现产品代码。

当前无任何 UI 回调获得真实宿主验收通过：**不将只读、写操作或工具取图回调列为首版依赖。** UI 控件不承诺直接提交选择，写操作不省略既有宿主批准流程。

图片方案尚无足够真实宿主证据冻结。后续补验后，若只读回调取图及受控大小的缩略图在 iframe 中显示通过，可优先评估工具取图；若仅 loopback 在 CSP 对照中通过，可评估精确 origin 的 loopback 方案。两者都必须以宿主实测为依据，并保留文字及 Lumi 入口。大图原件不应仅凭本次 app-server 的 8 MiB padding 成功就直接内联。

规范参考：[Add UI](https://developers.openai.com/plugins/build/chatgpt-ui)（资源关联、JSON-RPC bridge、UI 工具调用和 CSP）、[Build plugins](https://developers.openai.com/plugins/build/plugins#add-a-marketplace-from-the-cli)（本地 marketplace 注册和 App 测试流程）。规范说明与本次实测结论分开；用户手动安装、换版本新会话验收及卸载仍需实际完成并补充证据。
