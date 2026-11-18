# Cindy 可编辑作品工作区

2026-11-17。标准 MCP Apps 工具结果页入口已接入源码，声明 6 个模型可见开页工具及 5 个 app-only 页面工具；原生 Cindy 安装与三条真实链路仍待终态回执。历史 taskView 身份门槛不作为此入口的限制，手机原生支持不由桌面窄屏测试推断。

## 页面与现有行为

`web/cindy-app/Workspace.jsx` 直接装配 Web 的 SimpleHomePage、SimpleBooksPage、SimplePageView、SimpleSettingsPage、SimpleSettingDetailPage、SimpleStoryPage 和 TextCandidates。章节和单页同 Codex 使用 SimplePageView 的逐页阅读/编辑布局。表单保存、排序、软删除/恢复、脚本恢复版本及图片版本采用继续使用原有 API 与 Go 事务；预览读取不会提交采用。地址栏、前进后退、详情返回和操作菜单直接复用 `web/mcp-app/` 组件及 workspaceHistoryEntries。

Web 与 Codex 入口、默认运行时和共享 Sass 源码均未改动。Cindy 单独构建、作用域样式并在产物中映射宿主主题变量，包括压缩后的透明色值；不会向 Web 或 Codex 注入这些主题规则。已检查 1200 px 浅色页面、390 px 暗色、320 px 浅色及缩略图标签。窄屏验证是桌面布局测试，不是手机原生实现证明。

文件选择、模型生成、页面导出和下载通过当前任务的对话工具、授权附件及文件列表继续，页面没有虚假提交。点选页面、章节、设定、内容卡 @ 引用和文本候选复用 ModelContextAction，经官方 App 的 updateModelContext 更新当前对话；不发送消息、不保存、不采用。SDK 超时明确按未知结果处理，停止排队点选，避免旧更新与后续更新乱序。

## 数据、缓存和生命周期

`session.js` 使用相同 createWorkspaceAPI/createAppRequest 契约。每次页面/任务绑定各有独立 QueryClient、PreviewCache 和取消控制器，即使项目 UUID 相同也不复用本地表单或缓存。关闭/切换取消查询和媒体请求、丢弃晚到响应并关闭订阅；写入结果未知时先重读，不自动重发。

链路为：Lumi `/api/v1/ws` → stdio watch_app_changes（等待真实 WS 提示或 keepalive）→ AppRealtime → TanStack Query 失效 → read_app_resource 转发原 REST handler → SQLite。复用既有首次 join、rejoin、聚焦/可见校准；next/close 使用已核验绑定，不再为每次 WS 等待预读项目概览。受控图片按 Asset UUID、项目和尺寸读取，完整正文/图片只在根 _meta 中交付页面；文本候选的开页摘要不重复携带全部候选。

## 标准 MCP Apps 任务绑定

manifest 的 mcpApps.ui.resources 将 `ui://lumi/workspace/v1` 绑定包内 workspace.html，mcpApps.ui.tools 为 review_* 声明 model 可见性，为固定读写/图片/WS 工具声明 app 可见性。页面从 Host 提供的官方 `@modelcontextprotocol/ext-apps` 导入 App，在 connect 前注册 ontoolresult、onhostcontextchanged 和 onteardown。网络与资源限制由清单和 Host 的 MCP Apps CSP 执行，不使用 CDN、通用网络代理或页面 session 查询。

main.js 接收 Host 路由的 tool-call，逐次读取 Host 注入的 session_context 和实际 callId，绑定 Node 的 cancelWithCall。页面的 expected_project_uuid 仅为卡片显示项目的身份约束；Runtime 仍从当前任务目录核验项目、只读状态和 revision，不接受页面指定任务或目录。普通 call_tool 不开放页面方法，也不能包装 review_* 冒充具有 UI 绑定的顶层工具。

WorkspaceGateway 继续仅供隔离测试的合成 authority 使用，**不是 Cindy API**。该测试接口、随机 page key、BroadcastChannel 和 URL session 均不构成生产授权。真实宿主仍需核验当前标准页面工具是否收到完整可信任务上下文及关闭/切换/权限生命周期；代码完成和浏览器测试不代替这些回执。

## 构建与复跑

`node web/cindy-app/build.mjs` 独立构建内联 HTML；cindy-build/cindy-dev 自动运行它并随包携带。生成的 workspace.html 不提交，Web 和 Codex 原构建脚本不依赖它。

`node scripts/cindy-acceptance/workspace.mjs` 创建独立临时 HOME、data-dir、两个项目、三页及固定图候选，打印浏览器 URL/manifest。它是模拟页面授权、真实 CLI/REST/SQLite 的测试宿主，不能替代 Cindy 原生验收；不调用模型。结束时只按该夹具 nonce 停止自己的 CLI，保留临时证据。

本轮结果与限制见 [验收记录](acceptance/codex-alignment.md)。
