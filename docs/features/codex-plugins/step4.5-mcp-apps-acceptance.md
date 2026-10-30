# Step 4.5 · MCP Apps 宿主行为人工验收

## 目标

在真实 Codex App 中验证 MCP Apps 的图片加载与工具回调，确定 Step 5 的图片传输方案。本步只产出结论文档和一次性探针，不写产品代码。

## 背景

- 调研结论：`docs/features/codex-plugins/spike.md` 第 4 项。已验证：UI 资源关联、列出 / 读取，以及带 `_meta.ui.csp` 的 HTML 能经 MCP 传输。未验证：iframe 是否真正渲染、loopback 图片能否加载、data URI 上限、App 内回调工具。
- 产品只支持 Codex App（见 `plan.md`），本步只在 Codex App 中验收，不测 TUI / IDE。
- 探针与生成器：`docs/features/codex-plugins/spike/`（`run_probe.py`、`probe_server.py`、`app.html`）。

## 需要验证的问题

按 `spike.md` 第 4 节「可复现的下一步人工验收」执行，每项记录 App 版本、内置 `codex --version`、截图描述与结论：

1. **渲染**：调用 `show_app` 后 UI 是否真正出现。
2. **loopback 图片**：声明 `resourceDomains: ["http://127.0.0.1:<port>"]` 时图片是否加载，HTTP server 是否收到请求；与去掉该声明的版本对比。每次更换插件版本并新开会话，避免缓存干扰。
3. **data URI**：padding 为 0、64 KiB、256 KiB、1 MiB、4 MiB、8 MiB 时能否显示；成功值只作该版本下界。另测大 data URI 经工具结果传输时的 payload 上限。
4. **工具回调**：点击按钮后 server 是否收到新的 `tools/call`、结果能否回显；只读工具与写工具分别测试，记录批准弹窗与错误反馈。
5. **工具取图（候选方案）**：App 内调用一个返回图片 base64 或 image content 的工具，再在 iframe 内显示；若可行，可绕开 CSP 与 loopback 问题。

## 交付

- `docs/features/codex-plugins/mcp-apps-acceptance.md`：逐条结论，并在文末给出 Step 5 推荐的图片传输方案和回调可用范围。
- 探针改动只放在 `docs/features/codex-plugins/spike/`。

## 禁止

- 不修改 `internal/`、`cmd/`、`web/`、`codex-plugins/`。
- 不用普通浏览器加载 HTML 来冒充宿主 iframe 验证。
- 需要在真实 App 中安装测试插件时，由用户手动操作并在验收后卸载；不擅自改动用户 `~/.codex`。

## 完成标准

- 5 个问题都有结论；无法验证的写明「未验证」及原因。
- 明确给出 Step 5 的图片加载方案，或说明 Step 5 只做文本 + 「在 Lumi 中打开」的降级方案。
