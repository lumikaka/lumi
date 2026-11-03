# 共享 MCP 工作区验收

日期：2026-11-03（Asia/Shanghai）。四层共享工作区的实现与验收完成；使用隔离的 22 章测试作品，首章 24 页、次章 1 页，其余为空。未修改真实绘本，未触发付费生成或运行界面导出。

实现及能力清单见 [嵌入式工作区](../embedded-workspace.md)，工具契约见 [工具参考](../tools.md#review_book--review_chapters--review_chapter--review_page--review_candidates--review_settings)。

## 被测版本

- 隔离验收插件：0.1.4；这是验收包版本，不是 Lumi 发行版本。
- MCP HTML SHA256：`535e4583c35fe849b3363cf04dab5fa53a00aebac8b2f48f3edb8b4b6af2be52`。
- 内嵌前端 CLI SHA256：`b66b843fc555fdf2746929b5b5db3419e447bf13c3777ec1f2536c553b3e23e2`。
- 最后只清理两份 Sass 的末尾空行；重建后 Web 资产名与 MCP HTML 哈希不变，与被测二进制一致。

## 功能与实际结果

| 检查 | 结果与验证范围 |
| --- | --- |
| 源码共享 | Web 和 MCP 导入同一 `SimpleProjectPages.jsx` 主体、表单、弹窗、SectionRail 与 BookPreview。运行时差异集中于 workspace API、导航、图片及能力适配。 |
| 四层和内部导航 | 总览 → 章节列表 → 章节故事/网格 → 单页 → 前后页 → 返回章/作品通过；章和页均保留 section 栏。浏览器适配器有完整操作与截图，真实宿主有四层 DOM 和用户手动确认。 |
| 普通操作 | 编辑保存、排序、软删除和恢复在隔离界面通过。真实 App 标题保存 r7→r8，Web 恢复 r9 后 App 经 WS 提示重读；两端读取同一 SQLite 事实。 |
| 候选与版本 | 预览不写入。修复候选列表遮挡大图关闭按钮的问题，改为互斥弹窗状态。真实 App 显式采用 v1 再恢复 v2，事实为 v2/r9 → v1/r10 → v2/r11，Web 同步。 |
| 整本与单章阅读 | 浏览器适配器验证整本 25 页、单章 24 页，封面/正文/封底顺序、缺图、缩略图和翻页。真实宿主读取全部 22 章、25 个唯一页，用户准确确认“封面 · 1 / 25”。 |
| 设定 | 共享设定入口、详情与版本可用；真实宿主目标详情及版本请求成功，用户完成手动操作。 |
| 按需读取与上下文 | 摘要按 20 项游标分页，当前页正文/分镜/历史按需读取。真实整本加载期间无新增模型响应；UI 结果最大 8931 字节，模型侧最大 210 字节。两次版本采用的模型结果各 210 字节，完整结果仅在 `_meta`。显式模型读取和 DOM 检查另计。 |
| 分页与异常 | 覆盖 after=20、多章/长章、空作品/空章、缺图、读取失败重试、目标删除和快速切换。首章两批 snapshot 一致，没有丢失或重复页面。 |
| 边界 | Go 实际 MCP→REST→SQLite 验证项目/路径隔离、UUID 归属、只读连接拒绝写入、revision 冲突、分页范围、版本身份清单和附件上传/导入。 |
| 实时与 Web 链接 | WS 提示触发 Query 失效后 REST 重读；join/rejoin/focus 校准。实际服务重启后链接跟随端口变化，不固定端口或资源 UUID，不使用定时业务 HTTP 轮询。 |
| 兼容 | 旧 `review_book({chapter_uuid})` 仍仅进入该章阅读；新调用用 `mode:"preview"` 明确阅读范围。历史结果不静默扩大为全书。 |
| 两端布局 | Web 与 MCP 适配器均实际操作并检查 640px 窄屏截图；真实宿主保存截图宽 457px，section 栏横向显示。 |

## 自动化与构建

以下命令在当次被测源码上通过；本地未运行 Cargo/Rust。

```bash
pnpm --dir web test
pnpm --dir web build
go test ./internal/textmcp ./internal/story ./internal/production ./internal/httpapi ./internal/sessionimport ./cmd/lumi
node --test scripts/test-dev-codex.mjs scripts/test-package-codex.mjs
git diff HEAD --check
```

前端 459 项、6 个相关 Go 包、4 个插件打包测试通过，Web/MCP 及 `embed_frontend` CLI 构建通过。Vite 保留既有的大 chunk 提示。主要回归位于 `internal/textmcp/*_test.go`、`cmd/lumi/embedded_workspace_test.go`、`web/mcp-app/*.test.js` 及 `web/src/pages/simpleProjectState.test.js`。

## 截图与证据范围

| 截图 | 范围 |
| --- | --- |
| [章节结构](screenshots/chapter-structure.jpg) | MCP 浏览器适配器中的故事、网格与 section 栏 |
| [MCP 窄屏](screenshots/mcp-narrow.jpg) | 640px 共享主体 |
| [Web 窄屏](screenshots/web-narrow.jpg) | 640px Web 页面 |
| [候选大图](screenshots/candidate-preview.jpg) | 0.1.4 修复后的浏览器适配器，预览和候选列表互斥 |
| [真实宿主保存](screenshots/codex-page-saved.png) | 用户提供的 0.1.3 Codex App 可见区域，保存状态与 section 栏 |
| [整本阅读](screenshots/whole-book-reader.jpg) | 0.1.4 浏览器适配器的 1/25；真实宿主 1/25 另由用户明确确认 |

原始传输、DOM、完整测试输出、多轮失败记录和重复截图已归档到仓库外。摘要保留当时结论及其范围，不将浏览器适配器截图当作新版原生截图。

## 限制

- 原生 MCP App 的 DOM/截图自动化反复超时或返回空列表。采用真实请求/结果、已有宿主截图和用户实际操作反馈补充核对，没有绕过受限原生窗口或内部接口。
- 单次文件附件上限 6 MiB，后端真实上传/导入已验证；原生文件选择及下载未完成宿主验证，保留同资源 Web 继续入口。
- AI 生成、聊天引用和项目系统配置在 Web 继续。仅查看导出准备并取消，没有执行生图或导出。
- 共享主体沿用 Web 浅色样式，未声称完成全应用深色适配。
