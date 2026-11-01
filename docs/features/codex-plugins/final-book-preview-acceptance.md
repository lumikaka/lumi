# 最终绘本预览：实现与验收

2026-11-01，macOS。实现与自动化、普通浏览器交互验证完成。**本轮最终构建的真实 Codex App UI 验收尚未完成，不能以浏览器夹具或历史 0.5.4 宿主结果替代。** 本文的真实宿主状态以末节为准。

## 共享实现及 Web 视觉约束

`web/src/components/book-preview/BookPreview.jsx` 是 Web 和 MCP Apps 实际导入的同一个组件，负责缩略图、单页阅读、前后翻页、按页面 UUID 保留位置，以及懒加载、缺图、超时、解码错误和单图重试。数据、取图与链接均由回调传入；组件不依赖 REST、MCP、路由或项目上下文。

`book-preview-layout.sass` 从原 `simple-project.sass` 和 `workspaces.sass` 提取网格与章节阅读器规则。`book-preview.scss` 的窄屏和明暗主题调整限定在 `.book-preview--app` 下。大图使用 `object-fit: contain`，保留完整图片比例；App 缩略图也完整显示图片。

Web 外层适配器 `webAdapter.js` 映射原页面、图片 URL 和翻译。`SimpleProjectPages.jsx` 的 `SimpleBookView` 保留标题、工具栏、全屏、原网格链接和 Agent 入口，仍由 React Router 回调进入原页面；`ChapterComicPreviewPage.jsx` 保留单页/竖条模式、原图链接、比例提示和原页码布局。`SimpleProjectWorkspace.jsx` 的路由分派无需改动。**本次不重新设计 Web 原界面。**

相同隔离数据和 1280×900 视口下，用 `git archive HEAD web` 的原版与当前源码分别启动 Vite 对照；没有重置工作区。网格区域 `(258,302)-(916,459)` 和章节大图区域 `(262,185)-(1242,900)` 的像素差异均为零。这只证明两个实测预览区域，未将其扩大为所有页面或断点的像素保证。

- [原网格](acceptance/evidence/final-book-preview-20261101/web-original-grid.jpg) / [保留后的网格](acceptance/evidence/final-book-preview-20261101/web-preserved-grid.jpg)
- [原章节大图](acceptance/evidence/final-book-preview-20261101/web-original-paged.jpg) / [保留后的章节大图](acceptance/evidence/final-book-preview-20261101/web-preserved-paged.jpg)
- [像素对照结果](acceptance/evidence/final-book-preview-20261101/web-visual-comparison.json)

## App 数据与同步

`web/mcp-app/BookReview.jsx` 负责适配 `review_book`、`read_app_image`、TanStack Query 和现有 `AppRealtime` / `bindReviewSync`。只有全书视图切换到新组件；单页、设定与候选审阅保留原有功能。新视图没有项目导航、聊天、编辑、生成、采用或审批按钮。

`book.js` 从首游标开始读取完整范围，保留章节和页面顺序，不依赖调用者传来的首批或中间批数据；校验范围、总数、重复项及游标前进。超出结果预算时递减 limit，任一批失败不会发布半本内容；已有完整快照则保留并显示错误。`review_book(chapter_uuid?)` 的向后兼容字段为 `scope,total,chapter_title,page_number`，`target_uuid` 保持项目 UUID。工具契约见 [tools.md](tools.md#review_book--review_page--review_candidates--review_settings)。

只展示当前采用的 `asset_uuid`。网格可见项请求 384、阅读器请求 1024；沿用最多 3 路并发、32 项 / 6 MiB LRU、排队取消及过期响应保护。手动重试只清除对应项目/Asset/尺寸的缓存。服务端 JPEG ≤120 KiB、完整 MCP 结果 ≤256 KiB，未放开图片限制。大图不是原图，原图通过工具返回的真实 `open_url` 在 Lumi 查看；无固定端口、项目 UUID 或内嵌 Web iframe。

状态链保持 WebSocket `/api/v1/ws` 提示 → Query 失效 → MCP/CLI REST 重读 SQLite。首次 join、重新 join、窗口聚焦与恢复可见仍校准；`watch_app_changes next` 等待 WS 提示，心跳不读取业务。没有定时业务轮询。刷新与图片替换尽量保留当前页面 UUID；删除当前页时按原阅读位置回退。全部公开字段使用 UUIDv7、snake_case 和统一 JSON 信封。

`create-picture-book` Skill 明确按用户要求完成创作、导入保存和导出，再展示最终预览；不等待 UI、不加确认节点，保留用户明确要求的中途审阅。UI 关闭或不可用时仍交付文本结果、准确导出路径和 Web 入口，`import_image` 语义不变。

## 自动化与普通浏览器结果

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `pnpm --dir web test` | 442 项通过，0 失败 | [完整日志](acceptance/evidence/final-book-preview-20261101/web-tests.txt) |
| `pnpm --dir web build` | 成功，包含内嵌 MCP App；保留主 Web 大 chunk 提示 | [日志](acceptance/evidence/final-book-preview-20261101/build.txt) |
| `go test ./internal/textmcp ./internal/server ./cmd/lumi` | 全部通过 | [日志](acceptance/evidence/final-book-preview-20261101/go-tests.txt) |
| Skill 校验、`git diff --check` | 通过 | 本轮检查；未运行 Cargo/Rust |
| 页面与契约 | Go 真实服务及 JS 测试覆盖 >20 页、多章节、封面/正文/封底、非最新但当前采用图片、缺图、章节范围、分页错误和中止 | `internal/textmcp/book_preview_test.go`、`web/mcp-app/book.test.js` |
| 无 UI 导出 | Go 测试先创建、导入、导出 Markdown/ZIP/PDF，之后才调用审阅；不读取 UI 资源 | 同上 Go 测试；[独立 CLI 导出记录](acceptance/evidence/final-book-preview-20261101/headless-exports.json) |
| 浏览器最终构建 | 两章 25 页读全，缩略图进入大图、上一页/下一页、360px 窄屏、明暗主题、缺图、单图失败/重试及刷新保留位置通过 | 下方截图、[浏览器事实](acceptance/evidence/final-book-preview-20261101/browser-facts.json) |

普通浏览器夹具只使用真实 Lumi CLI 读取和导出隔离项目，宿主消息由夹具模拟；界面显示实时连接中断是该夹具不提供宿主事件等待所致。它**不能验证真实 Codex 的 WS 传输、批准行为或生命周期**。WS join/rejoin/hint/focus/visibility → Query 失效已有 `app.test.js` 覆盖，不能因此声称本轮真实宿主通过。

- [全书分组](acceptance/evidence/final-book-preview-20261101/browser-final-grid.jpg)
- [360px 阅读](acceptance/evidence/final-book-preview-20261101/browser-final-reader-narrow.jpg) / [深色](acceptance/evidence/final-book-preview-20261101/browser-final-reader-dark-narrow.jpg)
- [缺图](acceptance/evidence/final-book-preview-20261101/browser-final-missing.jpg)
- [单图失败](acceptance/evidence/final-book-preview-20261101/browser-final-single-image-error.jpg) / [重试恢复](acceptance/evidence/final-book-preview-20261101/browser-final-image-retried.jpg)

## 本轮真实 Codex 宿主状态

宿主 App `26.1027.51404` / build `23880`，内置 CLI `0.166.0-alpha.5.1`。隔离插件 `lumi-review-9rvr7oxj@lumi-review-9rvr7oxj-local` 升级至 `0.6.1`，绑定专用临时项目，不访问用户绘本数据；没有修改批准设置或其他插件。最终二进制与全部相关源文件摘要见 [manifest](acceptance/evidence/final-book-preview-20261101/fixture-manifest.json)。

最终资源为 `ui://lumi/review/263d05f6bbc61ccbb25c1cdc2f43ab8db575059462da62b1b227ac78037de454.html`。已实际从本聊天调用 `project_overview` 和 `review_book(limit:2)`，但当前会话仍返回旧 0.6.0 资源 `ee8d5cea…`，[透明日志](acceptance/evidence/final-book-preview-20261101/host-old-bundle-events.jsonl)确认未读取 0.6.1；MCP Apps 自动化的侧栏列表为空。因此这次调用只证明真实工具文本可返回，**不能证明最终 UI 已在宿主加载或可用**。

已请求刷新聊天的 MCP 连接。待宿主真正加载最终摘要并展开侧栏后，还需核对：分组和分页、缩略图及翻页、窄屏主题、单图失败重试、独立 CLI 修改当前图后的 WS 自动同步、重连/聚焦保留阅读位置、“在 Lumi 中打开”，以及宿主阅读期间只有只读工具调用。目前以上最终构建宿主项目均标为**未验证**，没有最终宿主截图。

隔离夹具包含有意缺图页，CLI 导出记录使用 `allow_missing_images:true`，不代表已生成完整新绘本。没有重新调用付费/宿主生图；创作与图片导入流程由真实服务测试及已有测试素材验证。最终交付仍应遵循 Skill 的完整绘本 `allow_missing_images:false` 规则。

本轮最终浏览器夹具与 Web 视觉对照服务已停止，浏览器视口覆盖已恢复。隔离插件 0.6.1 暂保留供刷新连接后续验；仅绑定 `/private/tmp/lumi-step5-9rvr7oxj/project`。验收结束后可用 `codex plugin remove lumi-review-9rvr7oxj@lumi-review-9rvr7oxj-local` 及 `codex plugin marketplace remove lumi-review-9rvr7oxj-local` 清理本轮夹具，不影响其他插件。[交付核对](acceptance/evidence/final-book-preview-20261101/completion-audit.json)记录了已验证与未验证边界。
