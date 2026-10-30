# Step 5 · MCP Apps 审阅界面

## 目标

为关键创作节点提供 Codex App 内嵌的 MCP Apps 界面，通过 **App 内只读工具取图**展示项目缩略图，让用户在对话中审阅整本、单页、候选与设定。每个面向模型的审阅工具仍返回完整文本、候选编号和「在 Lumi 中打开」入口，图片或 UI 加载失败时仍能推进流程。

## 背景

- 方案：`docs/features/codex-plugins/plan.md`（「大量使用 Codex 新支持的 MCP Apps」「MCP Apps 有自己的 UI」）。
- 调研与真实宿主证据：[Step 4.5 验收记录](mcp-apps-acceptance.md)。App `26.1027.30824` / build `22943`、内置 CLI `0.166.0-alpha.5.1` 下，渲染、桥接、只读及临时 marker 写回调、工具错误回显通过；本地 data URI 六档和两种工具取图模式 × 六档全部加载成功。
- Step 3 / Step 4 已提供文本与图片工具，当前契约见 [tools.md](tools.md#read_image)。`read_image` 接收项目 Asset UUID，生成最长边 256–1024、编码不超过 120 KiB 的 JPEG 预览；整个 MCP 结果不超过 256 KiB。
- 前端技术栈见 `AGENTS.md`（React 19 JavaScript、Vite 7、Sass、pnpm）；可复用 `web/` 中现有的展示组件与样式变量，但 MCP Apps 的构建产物需独立打包并内嵌进 Go 二进制。

## 前置条件

- **工具取图方案的宿主前提已验证，可以据此实现 Step 5。** 8 MiB padding 是合成 128×128 PNG 在该版本的已测下界，不是生产默认值、精确上限或真实大像素图片的性能承诺。
- allow 轮 loopback 加载失败；no-resource 对照尚未执行。只检查到的 8 MiB 模型宿主通知将结果包装进 text，当前探针没有显示该图片。首版图片不依赖这两条路径，相关补验不阻塞只读工具取图。
- marker 写入及回显通过，但批准弹窗、拒绝行为和真实业务写入尚未验收。不能把写回调成功当作无需批准；UI 直接提交业务选择需满足下文的独立门槛。
- 只支持 Codex App，不考虑 TUI / IDE 的渲染兼容。

## 图片传输与工具契约

1. 确定主路径：关联审阅工具提供视图类型、公开目标 UUID、revision 和完整文本 → iframe 初始化桥接 → App 按需调用只读 `read_app_image` → 读取返回的 data URI → 设置 `<img>`。初始模型工具结果只携带轻量上下文，不内联整批图片。
2. 新增 `read_app_image`，与现有 `read_image` 共用图片读取、等比缩小和大小控制逻辑；保留现有工具供模型审阅及宿主参考图生成使用。不要因探针的 8 MiB 测试而提高现有生产限制。

   | 项目 | 约定 |
   | --- | --- |
   | 输入 | 必填 `asset_uuid`；可选 `max_edge`，256–1024，默认 1024；网格主动请求较小预览 |
   | 项目边界 | 沿用 cwd 绑定的项目和 CLI 实例转发；校验 Asset 属于该项目，不接受任意文件路径，不直接打开 SQLite |
   | 可见性 / 注解 | `_meta.ui.visibility: ["app"]`；`readOnlyHint: true`、`destructiveHint: false`、`openWorldHint: false` |
   | 成功 structuredContent | 统一成功信封 `success: true`；`data` 直接包含 `asset_uuid`、`mime_type`、`width`、`height`、`data_uri` |
   | text 内容 | 返回同一成功信封下的图片元数据摘要，省略 `data_uri`，不重复打印 base64 |
   | 失败 | 既有失败信封：`success: false`、`data: null`，`error` 包含 `code`、`message`、`details`；MCP `isError: true`；沿用资源缺失、格式和大小错误码 |
   | 大小 | JPEG 二进制 ≤120 KiB；含 base64 和全部内容块的序列化 MCP 结果 ≤256 KiB，必要时继续缩小或返回明确错误 |

3. Lumi 的生产结果保留统一信封，因此前端读 **`structuredContent.data.data_uri`**；探针的平铺 `structuredContent.data_uri` 只作为传输证据，不照搬为生产契约。新工具单次仅返回一种图片表示，不同时附带同图的 image content。标准 image content 已测可行，可作为后续替换方案。
4. 关联审阅工具声明 `_meta.ui.resourceUri`，资源使用 `text/html;profile=mcp-app`。脚本、样式随 UI 打包；图片不使用 loopback URL，不为取图添加本地 `resourceDomains` 或 `connectDomains`。其他用途所需 CSP 按实际连接单独声明和验收。
5. 为轻量初始上下文实现受限的结果适配：正常读取 structuredContent，也覆盖本次观察到的自有工具结果被 JSON 文本包装的形状，解包后校验视图类型、UUID、revision 和信封；不得猜测目标或递归解析任意文本。解析失败保留完整文本和打开 Lumi 的入口。图片本体始终通过 App 回调重新获取，不依赖模型通知中完整的 base64。

## 审阅界面与交互

1. 至少实现以下视图：
   - **整本概览**：页面缩略图网格、每页文本摘要、缺图标记。
   - **单页预览**：画面图片、正文、画面描述、图片版本切换。
   - **多选一**：展示若干候选（图片或文本）、稳定编号和选中态；业务提交遵守下文的回调范围。
   - **设定卡片**：角色 / 场景的设定与参考图。
2. 网格按可见项懒加载，限制同时在途的取图数量；缓存按项目、Asset UUID 和请求尺寸区分，图片版本切换时按最新 Asset 更新。丢弃过期请求结果，避免快速切页时显示另一页的图；缓存有容量上限，不预取整本原图。
3. 区分缺图、加载中、资源错误、宿主拒绝、超时和图片解码失败。失败只影响对应预览，可由用户重试；不自动循环重试、不把超时标为宿主拒绝。正文、候选编号和 Lumi 入口始终保留。
4. 「在 Lumi 中打开」沿用 CLI 实例的打开能力（`lumi open` 或等价方式），不把 iframe 能加载本地图片作为前提；原图及复杂编辑在完整 UI 中查看。
5. 交互状态样式遵守 `AGENTS.md` 中关于 `selected` / `active` / `aria-pressed` 与 hover 组合状态的规则；适配浅色和深色主题。

## 回调范围与状态同步

- 首版可依赖已验证的只读 `tools/call` 取图与读取能力，所有资源标识使用公开 UUIDv7。`read_app_image` 是实现计划中的新工具，完成后须更新 `tools.md` 并以真实产品契约复测。
- 「多选一」默认由用户在对话回复候选编号，模型调用已有写工具推进。UI 选中态只表示本地选择，不显示“已保存”。若本步启用 UI 直接提交，必须补验真实业务写入、批准、拒绝、重复点击、中断后重读及 `expected_revision` 冲突；未通过时保持对话提交路径。
- 业务状态同步遵守 `AGENTS.md`：复用 `/api/v1/ws` 的 `topic/event/payload/ref/join_ref` 变更提示，触发 TanStack Query 失效并通过 REST 重读 SQLite 事实状态；经 MCP 调用时由 CLI 实例转发 REST 读取。首次 join、重新 join、窗口聚焦时校准。若为 iframe 新增 WebSocket 提示的桥接转发，该能力须在本步单独实现并验收，不能由工具取图成功推断已经可用，也不得以定时 HTTP / MCP 查询替代。

## 禁止

- 不修改旧 MCP 逻辑（`internal/mcpserver`、`codex-plugins/`）。
- 不在 MCP Apps 中做复杂编辑（版面拖拽、富文本编辑等），这些留给完整 web UI。
- 不使用定时轮询刷新数据，不自动修改宿主批准设置。
- 不将原图或整本图片 base64 塞入模型文本、初始审阅结果或列表响应。
- 不运行任何 Cargo / Rust 命令。

## 完成标准

- `pnpm --dir web test`（或新前端包的测试）和 `go test ./...` 通过。
- Go 测试验证资源关联、HTML MIME type、实际 CSP 和 app-only / read-only 注解；`read_app_image` 的项目隔离、UUID 校验、缺失资源、错误信封、预览尺寸/字节上限及整个 256 KiB 结果上限。现有 `read_image` 的模型参考图链路继续通过。
- 前端测试覆盖成功/错误信封、轻量模型结果的文本包装、单图失败兜底、懒加载、缓存和过期响应处理；不得把探针平铺字段或 8 MiB 常量当生产契约。
- 文本兜底测试：不读取 UI 资源或故意让图片回调失败时，仅凭工具文本结果即可完成 Step 3 端到端流程。UI 直接写入若启用，另测批准/拒绝/冲突/结果未知时重读；未启用时验证候选编号的对话提交链路。
- 验证 WebSocket 变更提示 → TanStack Query 失效 → REST 重读，以及首次 join、重新 join、窗口聚焦校准；不存在定时 HTTP / MCP 轮询。
- 在 `docs/features/codex-plugins/mcp-apps.md` 写出真实 Codex App 验收清单：四类视图、正确图形、历史版本切换、明暗主题、失败兜底、回调与批准边界，并记录 App / CLI 版本、截图、请求与结果。至少验证真实 Lumi 预览的 120 KiB / 256 KiB 边界，不能用合成 padding 探针替代产品验收。**交付时明确自动化通过、真实宿主通过及仍待确认的项目。**
