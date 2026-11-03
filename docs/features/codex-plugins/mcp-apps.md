# MCP Apps 共享工作区

四层入口直接复用 Web 简洁模式页面主体，包含普通业务编辑和版本操作。工具契约见 [工具参考](tools.md#review_book--review_chapters--review_chapter--review_page--review_candidates--review_settings)，功能清单见 [嵌入式工作区](embedded-workspace.md)，实际结果见 [验收摘要](acceptance/shared-workspace-20261103.md)。

## 视图与源码

| 工具/模式 | 页面 |
| --- | --- |
| `review_book()` | 作品总览 |
| `review_chapters()` | 按业务顺序排列的章节列表 |
| `review_chapter(chapter_uuid)` | 章节故事、完整页面网格和 section 栏 |
| `review_page(chapter_uuid, page_uuid)` | 单页编辑、正文、分镜、版本和本章切换 |
| `review_settings` / `review_candidates` | 设定和候选入口，区分本地预览与显式采用 |

Web 与 MCP 导入同一 `SimpleProjectPages.jsx`、SectionRail、图片、表单和弹窗；MCP 没有复制另一套精简 JSX，也没有嵌入整个 Web 应用。Web 保留全局外壳，MCP 由 `web/mcp-app/main.jsx` 装配内存路由和宿主适配，保留章内 section 导航。

`pnpm --dir web build:app` 生成单个 `internal/textmcp/appdist/review.html`，由 Go embed 内嵌。URI 为 `ui://lumi/review/<内容哈希>.html`，目标和视图由工具结果及内部导航状态决定。只有显式“在 Lumi 中打开”等 Web 入口交由宿主打开链接，地址来自当前服务实例。

完成创作或导出时，按整本、指定章节、单页范围分别打开作品总览、章节或单页；仅生成第一页也打开该单页。旧 preview/reader 参数和历史结果只兼容到同范围正常页面，不渲染独立阅读器。

## 数据、权限和图片

启动工具返回有界的资源身份、标题、摘要和状态。共享页面通过 workspace API 工厂按需访问 App-only `read_app_resource` / `write_app_resource`，转发到绑定项目现有 REST handler 和 SQLite；完整详情放入 `_meta["lumi/app_response"]`，模型侧 content/structuredContent 仅保留有界标记。模型明确需要正文时仍使用业务读取工具。

写入只由用户实际提交触发，复用既有权限、revision、确认框和冲突处理。服务器校验绑定项目、公开 UUIDv7、资源路径及 `_meta` 中的 expected project；不开放任意项目、文件路径或转发目标。

导航使用摘要集合和有界游标分页，当前页正文、分镜和历史按需读取。图片只传 Asset UUID，通过 `read_app_image` 的 UI 元数据取得 384/1024 预览；有界并发、缓存、取消和过期结果保护继续生效，原图经同资源 Web 入口查看。

App-only 可见性本身不证明数据未进入模型上下文；实际宿主的请求、结果、普通操作期间模型响应以及手动反馈已分别核对，测量范围见验收摘要。

## 实时同步

`watch_app_changes` 的 open/next/close 在 stdio 端连接同一 `/api/v1/ws`，next 等待真实 WS 变更提示或心跳。变更提示触发 TanStack Query 失效，页面再经 REST 读取 SQLite 事实；首次 join、重新 join、窗口聚焦和恢复可见时校准。心跳不刷新业务，没有定时 HTTP 轮询。

订阅绑定当前 MCP 会话和公开项目 UUID，取消或会话结束会释放连接。连接票据不交给页面，MCP App 的业务数据和图片继续走宿主桥接。

## 验收及历史

历史共享工作区的保存、版本恢复、真实宿主 25 页阅读、异常与边界覆盖见 [2026-11-03 摘要](acceptance/shared-workspace-20261103.md)。文件/下载、生成及原生自动化的具体限制同样保留在该摘要。

早期只读 0.5.4、最终阅读器及安装流程的历史结论见 [验收索引](acceptance/README.md)。旧版“全量工具结果”“UI 只能本地选择”“仅通过对话采用”的描述不适用于当前工作区。
