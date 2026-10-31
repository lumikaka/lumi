# Step 5 · MCP Apps 审阅

2026-10-31，Asia/Shanghai，macOS。**0.5.4 的自动化与真实 Codex App 验收通过。** 整本、单页、候选与设定均已在真实侧栏操作并截图；WS 变更后的自动更新、自动/手动重连、历史图片切换、明暗主题、单图失败重试和对话编号提交已有对应证据。

本轮宿主为 App `26.1027.30824` / build `22943`，内置 CLI `0.166.0-alpha.5.1`。当前验收包与证据位于 [step5-v054-20261031](acceptance/evidence/step5-v054-20261031/completion-audit.json)。此前普通浏览器与旧版本宿主结果单独保留，未用来替代新版验收。

## 实现与边界

- `web/mcp-app/` 使用 React 19、TanStack Query 5、Sass、Vite 7。`pnpm --dir web build:app` 独立生成 `internal/textmcp/appdist/review.html`，通过 Go embed 内嵌；完整 web 构建同时更新审阅包。资源 URI 含构建 SHA256，无新增运行时依赖或 CDN。
- 四个模型审阅工具返回完整文本、公开 UUIDv7、revision、稳定候选编号与 Lumi 入口。初始结果没有图片 base64。App 通过只读、App-only 的 `read_app_image` 获取 `structuredContent.data.data_uri`；text 仅包含元数据摘要。旧 `read_image` 的模型参考图链路保留。[完整契约](tools.md#review_book--review_page--review_candidates--review_settings)。
- 网格仅对可见项请求 384 预览，单页请求 1024；最多 3 路图片请求，按项目/Asset/尺寸缓存，LRU 最多 32 项、6 MiB。取消排队请求并丢弃过期响应，不预取整本原图。
- 缺图、加载、资源错误、宿主拒绝、超时及解码失败分别显示；单图失败不自动重试，正文、编号和打开入口保留。受限结果适配只接受生产 structuredContent 或观察到的一层自有 JSON text 包装，解包后校验信封、目标、视图和 revision。
- 版本切换与候选选中均为本地状态，明确显示尚未保存。UI 只调用审阅、取图和订阅工具；业务选择由对话回复编号，再重读并以 `expected_revision` 调用 `pages/settings action:select_image`。没有 UI 业务提交，也没有改变宿主批准设置。
- 完整编辑和原图由 `ui/open-link` 打开 CLI URL；工具文本另保留 `lumi open`。原图不依赖 iframe 的网络访问。

## WS 提示桥接

0.5.4 使用 [`watch_app_changes`](tools.md#watch_app_changes) 的 `open/next/close`。stdio 建立同一 `/api/v1/ws` 项目订阅，保持 `topic/event/payload/ref/join_ref` 信封；`next` 阻塞等待真实 WS 事件，普通 MCP 结果返回提示后接续等待。游标保留两次等待之间的变更，心跳不会掩盖业务提示。

只有变更、首次 join、重新 join、窗口聚焦及恢复可见会使 TanStack Query 失效，经 MCP→CLI REST 重读 SQLite 事实。WS ping 仅完成事件等待，不刷新业务。前端没有按定时器发起状态读取。实际记录中有 230 秒、5 次心跳且零审阅读取的连续区间，见 [生命周期记录](acceptance/evidence/step5-v054-20261031/real-host-lifecycle-and-quiet-window.json)。

订阅绑定 MCP 会话和公开项目 UUID，订阅自身使用 UUIDv7；原始 WS payload 被丢弃。CLI 票据与地址不传给 iframe，三个 CSP 域列表均为空，HTML 为 `connect-src 'none'; img-src data:`。票据限单个项目、一小时有效，CLI 重启失效；普通桌面 Origin 策略保持原状。取消、显式 close、会话结束和两分钟未续接等待都会释放连接；每进程最多 16 个订阅，每个订阅最多一个等待者。

## 自动化

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| 前端完整测试 | 435 项通过，含 13 项 App 测试 | [日志](acceptance/evidence/step5-v054-20261031/web-tests.txt) |
| Go 完整测试 | `go test ./...` 通过 | [日志](acceptance/evidence/step5-v054-20261031/go-tests.txt) |
| 新订阅生命周期竞态检查 | 通过 | [日志](acceptance/evidence/step5-v054-20261031/watch-race-test.txt) |
| 完整 web、独立 App 包及内嵌 CLI 构建 | 通过；完整 web 延续既有大 chunk 提示 | [构建日志](acceptance/evidence/step5-v054-20261031/build.txt)、[源码/二进制核对](acceptance/evidence/step5-v054-20261031/pre-host-audit.json) |

Go 测试覆盖资源关联、MIME、实际 HTML/CSP、App-only/read-only 注解、UUID 与项目隔离、错误信封、真实生产图片处理和 120/256 KiB 限制。`cmd/lumi/mcp_test.go` 在不读取 UI 资源的完整 Step 3 创作/设定/三页/导出链路中加入审阅文本和故意失败的 App 图片读取，流程继续完成；纯文本候选编号提交和旧 revision 冲突另有测试。

前端覆盖生产信封、一层文本包装、目标校验、新结果替换旧缓存、懒加载、并发和容量限制、取消与过期响应、单图失败及拒绝/超时分类。真实 QueryClient/QueryObserver 验证 join/rejoin/WS hint/focus/visibility 失效和重读；阻塞结果覆盖项目/订阅/游标校验、心跳不刷新、关闭释放和宿主异常时停止重试。

## 真实 Codex App 验收

四类视图均使用冻结的 0.5.4 资源；最后侧栏自动化已可访问。下列截图与 DOM 来自真实 MCP Apps tab。

| 项目 | 结果与证据 |
| --- | --- |
| 整本 | 三页、红靴/无靴两图、第三页缺图，完整正文及入口；[截图](acceptance/evidence/step5-v054-20261031/screenshots/host-book-final.png)、[DOM](acceptance/evidence/step5-v054-20261031/host-restored-dom.txt) |
| 单页与历史 | 当前版本 2 红靴，选择下拉框版本 1 后变为无靴；DOM 选中 UUID 与历史版本一致，图片正常解码为 1024×682，正文和画面描述完整；[当前版本](acceptance/evidence/step5-v054-20261031/screenshots/host-page-current-v2.png)、[版本 1](acceptance/evidence/step5-v054-20261031/screenshots/host-page-version1.png)、[像素/选择事实](acceptance/evidence/step5-v054-20261031/host-page-version1-facts.json) |
| 双候选与本地选择 | 稳定编号 2/1；2 仍标为当前使用，1 选中后明确尚未保存；[深色截图](acceptance/evidence/step5-v054-20261031/screenshots/host-candidates-dark-selected1.png)、[DOM/CSSOM](acceptance/evidence/step5-v054-20261031/host-candidates-dark-facts.json) |
| 设定 | 角色参考图、完整 summary、场景缺图及入口正确；[截图](acceptance/evidence/step5-v054-20261031/screenshots/host-settings-dark.png)、[DOM](acceptance/evidence/step5-v054-20261031/host-settings-dom.txt) |
| 明暗主题与组合状态 | 宿主浅色和手动深色均正确；实际 CSSOM 确认 `[aria-pressed="true"]:hover` 位于选中基础规则之后，深色背景/文本及选中色正确；同上主题截图和 CSSOM 事实 |
| 单图失败与重试 | 临时夹具物理图片短暂不可用，真实 App 图片回调失败后自动恢复文件；用户确认错误时说明和入口保留、点击重试后恢复。请求 117 失败、133 成功；[故障与恢复记录](acceptance/evidence/step5-v054-20261031/fixture-image-fault.json)、[图片回调](acceptance/evidence/step5-v054-20261031/real-host-image-callbacks.json)、[用户确认](acceptance/evidence/step5-v054-20261031/manual-ui-verification.json) |
| WS 自动更新 | 提交临时页正文后，WS changed → App 审阅重读 → DOM 显示新正文与 r8，期间没有点击刷新；恢复原文后又自动显示 r9；[截图](acceptance/evidence/step5-v054-20261031/screenshots/host-ws-updated.png)、[更新 DOM](acceptance/evidence/step5-v054-20261031/host-ws-updated-dom.txt)、[恢复 DOM](acceptance/evidence/step5-v054-20261031/host-restored-dom.txt)、[传输记录](acceptance/evidence/step5-v054-20261031/real-host-transport-and-change.json) |
| 自动及手动重连 | 仅停止临时 CLI 后，App 自动重新 join，审阅入口由 63196 更新至 57113；用户确认状态恢复。界面“重新连接”也实际经历连接中→已同步并校准；[重连记录](acceptance/evidence/step5-v054-20261031/real-host-rejoin.json)、[手动重连 DOM](acceptance/evidence/step5-v054-20261031/host-manual-reconnect-dom.txt) |
| 焦点校准 | 用户实际打开完整 Lumi 并切回审阅后确认已同步；焦点/可见性到 Query 失效的映射由真实 QueryClient 自动化验证；[用户确认](acceptance/evidence/step5-v054-20261031/manual-ui-verification.json)、[测试](../../../web/mcp-app/app.test.js) |
| 文本兜底与对话提交 | 完整 Step 3 自动化通过；真实已有临时项目仅凭候选文本选择 1、验证旧 revision 冲突、重读恢复 2 并导出三页与设定；[真实对话记录](acceptance/evidence/step5-v053-20261031/real-conversation-selection.json) |
| 在 Lumi 中打开 | 实际点击 App 链接，用户确认完整页面打开且切回仍同步；CLI URL 另经完整 UI 验证原图 1536×1024 解码；[原图截图](acceptance/evidence/step5-v054-20261031/screenshots/lumi-full-original.png)、[像素事实](acceptance/evidence/step5-v054-20261031/lumi-full-image-dom.json) |
| 批准与写入边界 | UI 业务写入未启用，其批准/拒绝/重复提交门槛不适用；对话写入仍重读和校验 revision。UI 本地选择后业务当前图保持版本 2；[用户确认和状态核对](acceptance/evidence/step5-v054-20261031/manual-ui-verification.json) |

单图资源失败和恢复在真实宿主测试；宿主拒绝、工具超时、解码失败的独立分类由自动化覆盖，未通过修改宿主批准设置来制造这些状态。选中/hover 组合样式核对宿主实际 CSSOM 的定义和顺序。

## 真实产品图片边界

素材为 Step 4 宿主实际生成并导入 Lumi 的 `fox-original.png` / `fox-boots.png`。没有使用合成 padding。相同当前 Asset 的独立 CLI 与真实 Host 回调通过 data URI SHA256 对齐。

| 请求最长边 | 实际像素 | JPEG 字节 | 整个 MCP 结果字节 | 验证范围 |
| ---: | ---: | ---: | ---: | --- |
| 256 | 256×170 | 6,521 | 9,063 | 独立 CLI 产品契约 |
| 384 | 384×256 | 11,905 | 16,243 | CLI + 真实 App 回调、解码、截图 |
| 1024 | 1024×682 | 59,595 | 79,829 | CLI + 真实 App 回调、解码、截图 |

1024 当前图的整个 JSON-RPC 消息也仅 79,865 字节，比 MCP 结果自身更大的这一上界仍低于 256 KiB。历史版本的 JPEG 严格上界与整个消息字节数另见 [图片回调记录](acceptance/evidence/step5-v054-20261031/real-host-image-callbacks.json)。这些数值是实测产物大小，不是精确容量上限；高熵图片自动化另验证超过预算时继续缩小。

## 夹具、恢复与历史证据

临时插件 `lumi-review-prbzu0ey` / 来源 `lumi-review-prbzu0ey-local` 已获用户明确注册和安装许可，后续版本从同一来源更新，项目始终仅绑定 `/private/tmp/lumi-step5-prbzu0ey/project`。未修改批准设置或其他插件。0.5.4 冻结根为 `/private/tmp/lumi-step5-v0.5.4-ih1679fv`，二进制 SHA256 `3847cd531984b728a2cf81a361b64506195169d88874ed1ebfcdcd94875713ed`，资源为 `ui://lumi/review/47db6bd0ede0afed29cf47c5a6a588543455866254c8369a937dc56e3464b2bb.html`。见 [manifest](acceptance/evidence/step5-v054-20261031/fixture-manifest.json)、[安装状态](acceptance/evidence/step5-v054-20261031/installation-state.json)、[最终审计](acceptance/evidence/step5-v054-20261031/completion-audit.json)。

临时正文和物理文件均已恢复，页面最终 revision 为 9、当前图片仍为版本 2，设定 revision 仍为 1。修订号因验收写入正常递增；[恢复记录](acceptance/evidence/step5-v054-20261031/pending-restoration.json)。普通浏览器夹具已停止，旧二进制与 manifest 保留用于追溯。

- 早期普通浏览器模拟 bridge：[浏览器记录](acceptance/evidence/step5-20261030/browser-fixture-manifest.json)。它不代表真实 Host。
- 0.5.2 在真实 Host 中正确显示整本图片，但 iframe 直连 WS 失败：[旧宿主截图](acceptance/evidence/step5-host-20261031/screenshots/host-book.png)。
- 0.5.3 的服务器 progress 已发出，真实界面仍中断：[用户截图](acceptance/evidence/step5-v053-20261031/screenshots/user-candidates-light.png)。因此最终使用 0.5.4 的普通结果事件桥接，未把服务器通知发出当成前端已同步。

重新准备夹具使用 `prepare_step5.py --binary BINARY`；同一项目升级使用 `prepare_upgrade.py --binary BINARY --base-manifest MANIFEST --version VERSION`。准备脚本不安装插件。透明 stdio 记录器归档请求与结果，图片/HTML/票据仅保留长度和摘要。当前真实 Host 原始记录见 [脱敏 JSONL](acceptance/evidence/step5-v054-20261031/real-host-events.jsonl)。

本步范围内没有仍待确认的完成门槛。真实宿主行为限定于以上实测版本；UI 业务提交保持未启用。官方参考：[MCP Apps UI](https://developers.openai.com/plugins/build/chatgpt-ui)、[CSP](https://developers.openai.com/plugins/build/chatgpt-ui#content-security-policy-csp)。
