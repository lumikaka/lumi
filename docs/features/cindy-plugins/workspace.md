# Cindy 可编辑作品工作区

2026-11-04。独立入口、共享页面装配、内部适配接口及隔离浏览器测试已经实现；当前 Cindy 原生生产页面桥仍关闭，不宣称原生 taskView 或手机工作区已验收。

## 页面与现有行为

`web/cindy-app/Workspace.jsx` 直接装配 Web 的 SimpleHomePage、SimpleBooksPage、SimplePagesPage（通过现有 ChapterReview）、SimplePageView、SimpleSettingsPage、SimpleSettingDetailPage、SimpleStoryPage 和 TextCandidates。作品、章节列表、完整章节、单页、设定及候选保持原资源含义。表单保存、排序、软删除/恢复、脚本恢复版本及图片版本采用继续使用原有 API 与 Go 事务；预览读取不会提交采用。

Web 与 Codex 入口、默认运行时和共享 Sass 源码均未改动。Cindy 单独构建、作用域样式并在产物中映射宿主主题变量，包括压缩后的透明色值；不会向 Web 或 Codex 注入这些主题规则。已检查 1200 px 浅色页面、390 px 暗色、320 px 浅色及缩略图标签。窄屏验证是桌面布局测试，不是手机原生实现证明。

尚未接入的文件选择、模型生成、页面导出和下载明确拒绝；不提交生成请求、不显示已下载。当前任务可继续通过对话工具、授权附件和任务文件交付。聊天引用链接进入同资源并显示未接入说明，不会自动发消息。

## 数据、缓存和生命周期

`session.js` 使用相同 createWorkspaceAPI/createAppRequest 契约。每次页面/任务绑定各有独立 QueryClient、PreviewCache 和取消控制器，即使项目 UUID 相同也不复用本地表单或缓存。关闭/切换取消查询和媒体请求、丢弃晚到响应并关闭订阅；写入结果未知时先重读，不自动重发。

内部链路为：Lumi `/api/v1/ws` → stdio watch_app_changes（等待真实 WS 提示或 keepalive）→ AppRealtime → TanStack Query 失效 → read_app_resource 转发原 REST handler → SQLite。复用既有首次 join、rejoin、聚焦/可见校准；静默期不定时读取业务状态。受控图片按 Asset UUID、项目和尺寸读取，缺图/失败单项重试，界面专用数据不进入 Agent 工具返回。

## 授权接口与生产关闭条件

`node/workspace.cjs` 的 WorkspaceGateway 是内部适配接口，**不是 Cindy 公开 API**。测试的 authority.resolve(sender) 由合成宿主提供固定、可撤销的 grant 对象，含项目、真实样式的任务上下文和生命周期取消信号。每次请求及返回后均检查同一有效授权；任务/权限变更后拒绝旧结果。Runtime.workspace 继续校验目录、项目、当前只读状态及业务 revision，只允许工作区固定方法。

worker 和 main.js 没有注册页面 RPC、BroadcastChannel、URL session 或通用 HTTP 代理。普通 call_tool 无法调用内部工作区方法。正式入口 main.jsx 只显示授权尚未核验的说明；包内 workspace.html 尚未声明为 taskView，也没有 /wake。导出的 React 组件和 session 工厂不提供任何宿主身份或权限。

开放生产桥需要匹配执行宿主的正式契约及可核验实现：由 Host 观察发送者并证明插件实例、任务、页面和当前读写资格，关闭、切换、停用、卸载时撤销；真实双任务测试应拒绝 session 篡改和旧消息重放。当前 0.1.121 构建的精确公开源码仍不可取得，历史探针未提供本轮页面身份回执。生产接口不能由此处的测试 authority、随机 page key 或页面字段替代。

## 构建与复跑

`node web/cindy-app/build.mjs` 独立构建内联 HTML；cindy-build/cindy-dev 自动运行它并随包携带。生成的 workspace.html 不提交，Web 和 Codex 原构建脚本不依赖它。

`node scripts/cindy-acceptance/workspace.mjs` 创建独立临时 HOME、data-dir、两个项目、三页及固定图候选，打印浏览器 URL/manifest。它是模拟页面授权、真实 CLI/REST/SQLite 的测试宿主，不能替代 Cindy 原生验收；不调用模型。结束时只按该夹具 nonce 停止自己的 CLI，保留临时证据。

本轮结果与限制见 [验收记录](acceptance/codex-alignment.md)。
