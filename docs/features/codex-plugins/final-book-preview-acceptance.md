# 最终绘本预览：历史验收摘要

状态更新：独立阅读器及其路由已移除，以下阅读器结果仅代表当时验收。当前入口与验证见 [正常业务页面验收](acceptance/normal-review-pages-20261103.md)。

日期：2026-11-01（macOS）。本记录保留当时结果和限制；当前四层工作区及后续真实宿主验收见 [2026-11-03 摘要](acceptance/shared-workspace-20261103.md)。当前独立预览已移除，整本使用 `review_book()`，指定章使用 `review_chapter`，单页使用 `review_page`；下文保留历史阅读器验收证据。

## 当时完成的工作

- Web 和 MCP 实际复用 `BookPreview.jsx`：缩略图、单页大图、前后页、按 UUID 保留位置、懒加载、缺图、超时和单图重试。
- 数据、图片及链接由适配器传入；Web 原有网格和章节大图的两个指定区域，1280×900 视口下原版/修改版像素对照差异为零。该结论只覆盖被比较区域。
- 普通浏览器夹具读全两章 25 页，缩略图、大图、前后页、360px 窄屏、当时 App 主题、缺图和失败重试通过。它不证明真实 Codex 的权限或生命周期通过。
- `pnpm --dir web test` 当时 442 项通过；Web/MCP 构建及 `go test ./internal/textmcp ./internal/server ./cmd/lumi` 通过。Go 和隔离 CLI 的无 UI 导出检查通过，没有运行 Cargo/Rust。

## 当时未完成的宿主验证

隔离插件 0.6.1 已准备，但真实聊天仍加载旧 0.6.0 资源，App 自动化侧栏列表为空。因此当次不能声称 0.6.1 的真实宿主 UI 已通过；没有最终原生截图。历史 0.5.4 和普通浏览器结果未替代这个缺口。

2026-11-03 已由共享工作区另行验证实际宿主数据读取、保存、版本采用恢复及整本“封面 · 1 / 25”。后续结果不追溯改写 2026-11-01 的未验证状态。

## 当前复跑入口

旧 MCP 专用 BookReview 已移除，当前共享页面及普通业务交互见 [嵌入式工作区](embedded-workspace.md)，工具和旧指定章节兼容行为见 [工具参考](tools.md#review_book--review_chapters--review_chapter--review_page--review_candidates--review_settings)。

可复用的浏览器夹具见 [开发脚本](../../../scripts/codex-acceptance/README.md)。完整像素对照、旧资源传输日志、临时插件身份、测试导出和重复截图已归档到仓库外；仓库仅保留结论、复现方法和关键示例，见 [保留规则](acceptance/README.md)。
