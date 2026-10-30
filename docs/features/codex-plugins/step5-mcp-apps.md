# Step 5 · MCP Apps 审阅界面

## 目标

为关键创作节点提供 Codex App 内嵌的 MCP Apps 界面，让用户不离开对话就能审阅和选择。MCP Apps 是 Codex App 中的主要审阅 UI；每个工具仍保留完整的文本结果，供模型推进流程，并在 UI 加载失败时兜底。

## 背景

- 方案：`docs/features/codex-plugins/plan.md`（「大量使用 Codex 新支持的 MCP Apps」「MCP Apps 有自己的 UI」）。
- 调研结论：`docs/features/codex-plugins/spike.md` 第 4 项，以及 Step 4.5 的验收结论。
- Step 3 / Step 4 已提供文本与图片工具。
- 前端技术栈见 `AGENTS.md`（React 19 JavaScript、Vite 7、Sass、pnpm）；可复用 `web/` 中现有的展示组件与样式变量，但 MCP Apps 的构建产物需独立打包并内嵌进 Go 二进制。

## 前置条件

- **Step 4.5 已完成**，并在 `mcp-apps-acceptance.md` 中给出图片传输方案与工具回调结论。调研只验证了 UI 资源与 CSP 元数据能经 MCP 传输；iframe 内 loopback 图片、data URI 上限、App 内回调工具均未验证，不得在此之前冻结图片加载方案。
- 只支持 Codex App，不考虑 TUI / IDE 的渲染兼容。

## 范围

1. 至少实现以下视图：
   - **整本概览**：页面缩略图网格、每页文本摘要、缺图标记。
   - **单页预览**：画面图片、正文、画面描述、图片版本切换。
   - **多选一**：展示若干候选（图片或文本），用户选择后回传结果。
   - **设定卡片**：角色 / 场景的设定与参考图。
2. 图片加载方式按 Step 4.5 的验收结论实现（本地地址加 `_meta.ui.csp.resourceDomains`、受控尺寸的内联数据，或 App 内调用工具取图）。
3. App 内回调工具仅在 Step 4.5 证实可用时使用；写操作需要的批准交互以验收结果为准。不可用时「多选一」退化为展示候选编号，由用户在对话中回复选择。
4. 每个关联工具的文本结果都包含完成流程所需的全部信息，不依赖 UI 才能继续。
5. 视图中提供「在 Lumi 中打开」的入口，可以跳转到完整 web UI（`lumi open` 或等价方式）。
6. 交互状态样式遵守 `AGENTS.md` 中关于 `selected` / `active` / `aria-pressed` 与 hover 组合状态的规则；适配浅色和深色主题。

## 禁止

- 不修改旧 MCP 逻辑（`internal/mcpserver`、`codex-plugins/`）。
- 不在 MCP Apps 中做复杂编辑（版面拖拽、富文本编辑等），这些留给完整 web UI。
- 不使用定时轮询刷新数据。
- 不运行任何 Cargo / Rust 命令。

## 完成标准

- `pnpm --dir web test`（或新前端包的测试）和 `go test ./...` 通过。
- Go 测试验证：MCP Apps 资源能被列出和读取，声明的 CSP / 元数据符合调研结论，工具结果正确关联 UI 资源。
- 文本兜底测试：不读取 UI 资源时，仅凭工具文本结果即可完成 Step 3 端到端测试中的流程。
- 在 `docs/features/codex-plugins/mcp-apps.md` 中写出人工验收清单（在 Codex App 中逐项检查的步骤和预期截图描述）。**本步的视觉效果需要人工在 Codex App 中验收，goal 完成时须明确说明哪些项仅通过自动化验证、哪些待人工确认。**
