# 正常业务页面验收

日期：2026-11-03（macOS）。独立作品/章节预览已移除；此前阅读器记录仅供历史参考。

## 当前行为

| 工具 | 正常业务页面 |
| --- | --- |
| `review_book()` | `/projects/:project_uuid/` 作品总览 |
| `review_chapters()` | `/projects/:project_uuid/chapters` 章节列表 |
| `review_chapter(chapter_uuid)` | `/projects/:project_uuid/chapters/:chapter_uuid` 章节 |
| `review_page(chapter_uuid,page_uuid)` | 章节下 `/sections/:page_uuid` 单页 |

创作、仅生成第一页、导出完成均按交付范围打开一次对应页面。新 schema 不再公布 `mode`、reader 或作品工具的章节参数。旧参数仍经严格校验，返回规范的正常视图、资源身份、请求和 URL。历史 book/reader 结果保持章节范围；缺少章节身份时失败，不扩大为整本。

Web 简洁版与专家版共用旧 URL 规范化：作品 `/preview` 回作品总览，章节 `/preview` 和旧 `/simple/books/:chapter_uuid/book` 回同一章节；搜索参数和 hash 保留。没有预览路由组件。MCP App 没有阅读预览导航，历史启动映射直接进入正常页面。

已删除 SimpleBookView、SimpleWorkPreview、ChapterComicPreviewPage、独立 BookPreview 阅读器和不再使用的 reader 数据加载器、样式、翻译。SectionRail 的按需图片加载提取为 SectionImage，正常页面的大图、候选稿与版本操作继续保留。

## 自动验证

- `pnpm --dir web test`：454 项通过，覆盖四层正常入口、历史 book/reader（无版本及 v2/v3）、范围校验、旧 URL、原生图片/runtime 隔离，以及既有编辑、图片和实时同步回归。
- `go test ./internal/textmcp ./cmd/lumi`：通过；包含真实 MCP 协议的旧参数调用、规范返回、章节/单页身份、跨项目/删除保护、游标分页、采用图片顺序，以及无需 UI 即可完成 PDF/Markdown/ZIP 导出。
- `node --test scripts/test-package-codex.mjs scripts/test-dev-codex.mjs`：4 项通过。
- `node --test cindy-plugins/tests/catalog.test.cjs`：3 项通过；使用仓库 `go run ./cindy-plugins/schema` 生成的共享 schema 快照。
- skill-creator `quick_validate.py`：通过。
- `make codex-dev-build`：通过，生成 Web dist、内嵌 `internal/textmcp/appdist/review.html` 与开发 CLI；`node scripts/dev-codex.mjs` 重新生成开发 marketplace。未手工修改插件缓存，未运行 Cargo/Rust。

## 浏览器实测

使用 `scripts/codex-acceptance/hierarchy_fixture.py`、真实开发 CLI stdio → REST → 隔离 SQLite 项目：22 个章节、25 页，首章 24 页；合成素材为仓库图标。普通浏览器模拟 MCP 宿主桥接，不代表新一次 Codex 原生宿主验收。

- 作品、章节集合、指定章节、指定单页均显示正常页面，外部 Lumi 链接保持相同资源范围。
- 旧整本 preview 参数、旧指定章 preview 参数和历史章节 book 结果分别落在作品总览或该章节；章节故事、全部 24 页缩略图及 section 栏正常。
- 单页当前图片与大图弹窗可用；候选稿弹窗可打开，采用 v2 后显示当前第 2 稿，再切回 v1 成功；标题编辑保存成功，并在真实 Web 页读到同一更新。测试标题与采用版本随后恢复。
- 简洁版旧章节 URL 重定向到正常章根，原有逐页编辑、图片、页栏、聊天布局保留；单页大图可用。
- 专家版旧章节 URL 重定向到正常章根，24 页脚本/图片工作台、正文/提示词标签、聊天和图片大图可用，原独立预览按钮消失。
- 专家版旧作品 URL 回正常作品总览，无 `/preview` 导航链接。

本次不修改已有用户项目，不重启共享开发后台，不安装/刷新 Codex 缓存；开发生成物来自共享正式版 skill 模板。
