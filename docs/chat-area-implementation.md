# Lumi ChatArea 实现记录

日期：2026-08-09

最终对齐更新：2026-08-11

## 范围与基线

- Lumi 验收页面：`http://localhost:5801`
- 实现源码：Lumi `ChatArea.jsx`、`ProjectWorkspaceLayout.jsx`、`chatAreaPresentation.js`、`chat.sass` 与 `shell.sass`
- 宽屏基线：ChatArea 展开宽度 360px，折叠宽度 52px；顶部工作区栏高 54px；ChatArea 顶部栏高约 65px。
- 窄屏基线：760×900 时使用覆盖整个视口的模态层，锁定页面正文滚动。

## 对齐检查清单

- [x] 展开、折叠和窄屏覆盖层的尺寸、边框、背景、层级和滚动方式。
- [x] 线程列表的标题、数量、新建入口、状态圆点、选中态、更多菜单与 UUID 复制。
- [x] 线程详情顶部栏、返回、折叠/关闭、运行状态和失败状态。
- [x] 点击“新建会话”直接进入草稿对话，首次发送时持久化会话与首轮消息，并以首条消息前 60 个字符生成标题。
- [x] 按 turn 分组的消息流、用户浅色气泡、Assistant 无气泡文本、工具调用与错误状态。
- [x] 等待用户输入、已回答输入、单选/多选、提交和取消状态。
- [x] 输入区高度、提示、发送/停止按钮、Enter/Shift+Enter 与转向快捷键。
- [x] `premise_asset_generation` 与 `asset_reference` 场景支持选择或粘贴最多 4 张 PNG/JPEG/WebP；显示本地缩略图和上传状态，支持移除与失败重试，未就绪时禁止发送。
- [x] 图片引用用 `chat_item_file_references` / `chat_follow_up_file_references` 持久化；内部只关联 bigint ID，API/WS 只返回 UUIDv7、公开元数据和受控 `content_url`。删除排队项同步解除引用。
- [x] 初始 turn、Follow-up 与 Steering 都支持固定图片引用；普通新 turn 不继承，未提供新图片的 Steering 只继承当前活动 turn 最近一组，不能跨 turn/thread。
- [x] `asset_reference` 可修改当前设定项、以当前项为参考创建独立新设定项；用户明确要求时可将当前项移入回收站。派生创建不会修改来源项。
- [x] 新引用会话只向模型暴露 `request_current_project_api`、`image_gen`、`request_user_input`。通用工具按 GET/POST/PATCH/DELETE 分别展示读取、创建、更新和移入回收站状态；旧 typed tool execution 仍可恢复。
- [x] 聊天生图在同一 Agent turn 中同步执行 `image_gen → request_current_project_api POST/PATCH → 最终回复`，并展示生成图片、设定项写入和整理回复状态。
- [x] Assistant 消息安全渲染 Markdown：段落、标题、有序/无序列表、引用、链接、行内代码和 fenced code；不使用原始 HTML 注入，链接协议使用白名单。
- [x] Follow-up 队列支持固定附件预览、原位编辑、删除、鼠标拖拽、Alt+方向键重排，以及活动 run 中的单条“立即引导”。若并发窗口关闭，消息保持 queued 并给出明确提示。
- [x] 会话列表使用页码分页和滚动继续加载；消息初始只读最近 50 条并通过 cursor 加载更早历史，插入旧消息后保持阅读位置；URL 可直接打开未出现在第一页的会话。
- [x] Premise“线程”Tab 复用 scope 分页并滚动加载全部记录，按 UUID 去重、使用服务端总数；首次失败和后续页失败分别可重试，打开详情不清空已加载缓存。
- [x] run 进入执行但尚无 Assistant/tool/error/input item 时显示可访问的 pending 状态，10 秒后增加长耗时提示；真实 item 到达即移除，`prefers-reduced-motion` 下关闭动画。
- [x] 工作流卡展示可点击 steps，并按 runs、events、关联 LLM logs 分组提供可展开诊断；点击 step/run 会打开脱敏输入输出详情并将模型调用筛选到对应步骤，支持返回全部调用。runs/events 使用 cursor，调用记录使用页码分页。payload/metadata 递归移除内部 ID、路径与 secret/token 字段。
- [x] 会话运行事件不再只在后台请求：详情流底部提供可展开的运行事件，展示公开 event/thread/run UUID、序号、时间与脱敏 payload，并通过 `after` cursor 分段加载。
- [x] Realtime 只失效 payload 所关联的 thread/workflow 查询；重连时重新读取持久化事实源，输入草稿和阅读位置保留在组件状态中。
- [x] 窄屏 Escape、背景点击关闭、顶部按钮重开、焦点标注、`aria-modal` 与正文滚动锁定。
- [x] 对选中态/按下态按钮显式定义组合 hover，避免同等优先级样式覆盖反馈。
- [x] 移除 `workspaces.sass` 中旧 ChatArea 视觉定义，ChatArea 样式只由 `chat.sass` 维护。

## 单机版保留差异

- Lumi 继续使用现有 REST、`/api/v1/ws` 信封与公开 UUIDv7，不暴露内部 ID、服务端权限或多租户字段。
- Lumi 的新线程 UI 直接进入草稿对话；受现有资源化 REST API 约束，首次提交会依次创建会话和首轮消息。
- Lumi 的“立即引导”是资源化 `POST .../follow_ups/:uuid/steerings`：服务端事务决定提升为 steering 或保留为 follow-up，不依赖前端竞态判断。
- Lumi 保留本地工作流进度、取消、重试和待处理输入；诊断信息来自 `project.sqlite` 的安全公开投影，不提供后台管理、计费、协作者与权限 UI。
- Lumi 使用轻量结构化 Markdown 解析器，不引入服务端 Markdown/HTML 或语法高亮依赖；原始 HTML 始终作为文本。
- 会话列表采用统一 API contract 的页码分页，消息、run 和 event 采用 opaque cursor；这是单机 SQLite 读取边界下的实现差异。
- `request_current_project_api` 是 Agent 内部的 REST-shaped 工具：只解析固定相对路径并直接调用当前项目领域服务，不做 HTTP 回环，也不扩展外部 REST API；项目 UUID、`subject_uuid`、方法、字段与软删除边界都在持久化工具意图和执行时校验。

## 关键 API 与迁移

- Migration：`20260811000016_add_project_chat_image_references`，包含 up/down、外键、顺序约束、唯一约束与索引。
- 会话历史：`GET /api/v1/projects/:project_uuid/chat_threads?page=&per_page=&scope=`；Premise Tab 固定 `scope=premise`。
- 消息历史：`GET /api/v1/projects/:project_uuid/chat_threads/:thread_uuid/items?before=&after=&limit=`。
- 会话运行事件：`GET /api/v1/projects/:project_uuid/chat_threads/:thread_uuid/events?after=&limit=`。
- 单条排队引导：`POST /api/v1/projects/:project_uuid/chat_threads/:thread_uuid/follow_ups/:follow_up_uuid/steerings`。
- 工作流诊断：`GET .../workflows/:workflow_uuid/runs`、`events`、`llm-logs`；后者可传公开 UUIDv7 `workflow_step_uuid` 精确筛选步骤调用。
- 所有响应继续使用 `{ "success": true, "data": ... }`；外部标识仅为 UUIDv7，不返回内部 `id`、River job ID、绝对路径、Authorization 或 API token。

## 验证命令

```sh
go test ./...
pnpm --dir web test
pnpm --dir web build
```

验证结果：Go 全量测试、Node 测试与 Vite 生产构建均通过；迁移 up/down、图片边界、队列提升/回退、分页、Markdown URL 安全、payload 净化和 realtime 定向刷新都有自动化覆盖。

浏览器验收覆盖 1440×900 的列表、详情、360px 展开侧栏与 52px 折叠栏；760×900 下覆盖层实测为 758×898，正文滚动被锁定，Escape 可关闭并从顶部按钮重新打开。工作流诊断可展开，浏览器无 error/warn 控制台日志。
