# AI Provider 与项目任务运行时

Lumi 的 AI 能力是可选的本机基础设施。没有 Provider、系统密钥库暂不可用或网络断开时，项目与 Story 手动工作台仍可正常使用。

## 存储与安全边界

- 全局 `lumi.sqlite` 的 `site_settings` 保存 Provider UUIDv7、Cloudflare Account ID、默认文本/图片模型、启用状态和加密后的 secret envelope。
- 首次配置使用独立前端地址 `/setup/`，先展示可选服务商列表，再通过所选服务商的 Dialog 完成连接。Cloudflare AI Gateway 的首次 Dialog 只要求 32 位 Account ID 和 Cloudflare API Token；默认文本/图片模型由系统设置，不要求用户首次选择。后续管理页允许调整 `author/model` 格式的模型 ID。Base URL 不允许用户输入，由后端固定派生为 `https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1`。
- Cloudflare API Token 由操作系统 Keychain 根密钥派生的 AES-256-GCM 密钥加密，密文保存在 `site_settings`；测试使用进程内 `MemoryMasterKeyStore`。API 不提供读取密钥的端点。
- 项目 `project.sqlite` 固化调用时的公开 Provider UUID、Base URL、模型、生成参数、Prompt 和章节 revision，但从不复制 API Key 或 Authorization header。
- REST、WebSocket 与 React 状态只暴露产品 UUID。`river_job_id` 与 River 表只存在于后端内部关联中。
- LLM 日志只保存受限长度的文本摘要、合法 JSON payload、token usage、Unicode 字符数、耗时、finish reason 和安全错误码，不保存请求 header、secret 或二进制内容。缓存输入 token 只在 Provider 返回 usage details 时记录；旧日志和图片调用保持 `NULL`，输出速度读取时推导。

Cloudflare 文本和 Agent 请求使用 AI Gateway REST API 的 `/ai/v1/chat/completions`；图片生成和参考图编辑统一使用 `/ai/v1/responses` 的 `image_generation` tool。第三方模型走账户自动创建的 `default` Gateway；`@cf/` Workers AI 模型会显式发送 `cf-aig-gateway-id: default`。该 Provider 不兼容任意 OpenAI-compatible Base URL。阿里云百炼保留为独立 Provider。

## River 版本与兼容性 gate

当前固定并验证的 River 版本是 `v0.41.0`，仅使用官方 `github.com/riverqueue/river`、`riverdriver/riversqlite` 和 `rivermigrate`。SQLite driver 仍由 River 官方标记为 early testing，因此任何 River 升级都必须先运行：

```bash
go test ./internal/jobqueue -run TestRiverSQLiteCompatibilityGate -count=1
```

该测试使用 Lumi 当前纯 Go SQLite driver、真实临时文件、WAL、5 秒 busy timeout 和 `MaxOpenConns(1)`，验证官方 migration、同一 `database/sql` 事务的提交/回滚、领取、完成、快速重试、scheduled job、active uniqueness、取消、graceful stop，以及关闭后重开。不得用内存 SQLite 替代此 gate。

River client 和类型只出现在 `internal/jobqueue`。domain、HTTP 和 React 使用 `task_runs` / `task_events` 产品投影，升级不得把 River job row 变成公开 contract。

## 生命周期与恢复

1. ProjectStore 完成项目 migration 和一致性备份。
2. AI runtime 检查单连接边界；River schema 需要升级时先额外创建 `project-before-river-*.sqlite` 备份，再运行官方 `rivermigrate` 和 validation。
3. 每个已打开 ProjectStore 启动自己的 River client；Runtime 注册表以 `project_uuid` 路由任务，打开 B 不停止 A。`story_generation`、`asset_maintenance`、`production` 与 `agent` queue 各自保持每项目 `MaxWorkers: 1`。
4. 创建 Story 生成时，产品 task、初始 append-only events、Agent run 与 River job 使用同一个 `*sql.Tx` 提交。domain 唯一索引与 River unique job 同时限制同章并发。
5. Worker 只在短事务中读写状态；Provider 网络调用期间没有数据库事务或长期写锁。
6. 切换标签页 URL 不操作 Runtime。仅目标项目显式关闭、独立空闲回收或应用退出时，River 才停止领取并最多 soft-stop 5 秒，再取消剩余 worker context；Runtime 未成功停止时对应 Store 和项目锁保持打开。安全幂等任务保持可恢复状态，旧 URL 重开项目后由 River 重试。

异步 Generation 的调用来源由进程内可信 `DomainInvocationContext` 指定，不能从 User-Agent、请求头或公开 JSON 推断。`direct_ui` 使用 `dedicated_thread` 且不等待 Chat；`chat_tool` 使用 `inline`、携带当前 Thread/Turn/Run/Tool Execution 的公开 UUIDv7 并等待完成；`workflow_step` 使用 `none`，避免嵌套业务步骤再创建展示 Thread。bootstrap YOLO route 显式注入 `chat_tool + inline + await_completion=true`，公开 YOLO 与章节 Generation HTTP 端点仍显式使用 `direct_ui`，请求体保持兼容。

Chat Tool 创建任务、Workflow/Step 与 `workflow_awaits` 使用同一 SQLite 事务；多步骤 bootstrap YOLO 还把全部 Steps 与首个 Workflow Job 纳入该事务。等待期间 `chat_turns` / `chat_runs` 因现有 CHECK 约束保持 `in_progress`，await 的 `waiting` 状态提供语义等价的可恢复边界，Turn REST DTO 投影为 `waiting_for_workflow`；Agent worker 返回并释放 queue slot，不轮询领域任务。Workflow 完成、失败、中断或取消时，领域终态、await `ready`、父 Turn/Run `queued` 和唯一 `JobChatResume` 在同一 SQLite/River 事务提交。Resume 只读取持久化终态，幂等保存脱敏 Tool Result，再继续原模型 Run；YOLO 终态结果使用按 position 排序的步骤摘要，不任意选取单个 Step。

`ReconcileOnOpen` 不会把活动 `waiting` await 当普通 `in_progress` Run 重跑。它会取消父 Run 已终止的 await，修复已终态 Workflow 仍处于 `waiting` 的投影，为 `ready|resuming` 依赖补投唯一 Resume，并重新计算全部 Thread 聚合状态。父 Run 停止会取消其独占 Workflow；晚到终态不能复活已取消 Run。单独取消或失败的 Workflow 则以结构化 Tool Result 唤醒仍有效的父 Run，不伪装成 Provider failure。

项目空闲回收按 UUID 独立判断。Presence、在途 HTTP 请求或 queued/running 工作任一存在时都不累计 5 分钟 grace；工作刚完成时重新开始完整 grace。`waiting_for_input` 延续可持久恢复语义，不阻止回收。一个项目的 Runtime 启动、停止或 Provider 调用失败不得改变其他项目的 Store、Runtime 与任务。

`task_runs` 是 UI 刷新后的事实源，状态覆盖 `queued`、`running`、`waiting_for_input`、`completed`、`failed`、`cancelled` 和 `interrupted`。`task_events`、`agent_events` 为 append-only sequence。WebSocket 事件丢失不会影响恢复；客户端重连后重新读取 task 列表或用 event cursor 追赶。

Asset Store 的全量 reconcile、完整性扫描、缩略图批量重建、暂存清理和 GC apply 注册为同一 River client 的 `asset_maintenance` workers。`asset_maintenance_runs` / `asset_maintenance_events` 是其公开 task 投影；River args 只携带 project/task/scan 或 GC plan UUID、kind 和版本，不携带内部 ID、绝对路径或字节。数据库 partial unique index与 River unique args 同时保证每项目每类最多一个 active job。

漫画导出清理由每项目 River client 额外注册 `lumi_comic_export_cleanup_v1` worker。周期任务 `RunOnStart` 并每小时运行，按 project UUID 参数和 active River state 保证不重入；每轮最多处理 1000 条，补偿应用关闭期间错过的到期清理。清理完成只广播 `comic:exports_changed` 刷新提示，前端通过 TanStack Query 失效后重新读取 REST/SQLite，不增加 HTTP 轮询。

普通维护失败在 River 尚有 attempt 时保持 `queued` 产品投影，从而继续占用同类 active 唯一槽；应用停止造成的 context cancellation 也按可恢复中断处理，而不是伪装成用户取消。破坏性的 `asset_gc_apply` 固定只尝试一次，并把 dry-run plan UUID 与 grace period 固化进版本化 input snapshot，每次执行仍重新校验 snapshot hash。

## Project Chat 同步生图

`premise_asset_generation` 与 `asset_reference` 聊天 scene 使用受控 `image_gen` 工具。该工具沿用当前 Agent run 固化的 Provider UUID，并在执行时解析当前 Provider 的默认图片模型；当前 `project_api_v4` 中，`generate`/`restyle` 默认从 Premise 事实状态读取并注入 `default_style`，`edit` 默认关闭它以保留来源画风，显式 `use_default_style` 可覆盖。`operation` 区分 `generate|edit|restyle`；未显式指定尺寸时，`edit|restyle` 按首张参考图选择最接近的横、竖或方形尺寸，`generate` 默认 `1536x1024`。默认质量为 `medium`，单次调用最长 10 分钟并继承 turn 取消信号。图片调用日志关联 chat thread/run，只保存脱敏请求结构与响应摘要，不保存 API key、Authorization header 或图片 Base64。

图片附件以 `project_chatbot_reference` purpose 上传并写入 Asset Store。数据库用 bigint 外键关联 chat item/follow-up、file 与暂存 upload，REST/WS 只返回 UUIDv7 和受控 `content_url`。普通 turn 不跨轮继承附件；Follow-up 附件创建后固定；没有新附件的 Steering 继承当前 turn 最近一组附件。文本模型只收到附件会自动传给 `image_gen` 的提示，实际图片字节不会进入文本上下文。

附件校验使用稳定领域错误码区分数量超限、不支持的 scene、UUID 无效、不存在、项目不匹配、上传未完成和 MIME 不支持。项目级 `storyboard_reference` 虽与设定引用复用内部 scene discriminator，服务端仍以 scope + scene 联合判断，不能误用图片附件。删除 Follow-up 会在同一事务中解除文件引用并压紧队列位置。

聊天链路在同一 Agent turn 内同步完成写入，不会创建 `production_task_runs`。`premise_asset_generation` 和 `asset_reference` 均使用全局 `request_api`；图片 Scene 额外暴露 `image_gen`，通过项目 Route GET 当前事实后选择 PATCH 当前项、POST 派生新项，或经全局危险确认后 DELETE 软删除。`request_api` 以 Echo 当前注册的 `/api/v1/projects/{project_uuid}/...` Route 为可用范围：命中已审查覆盖层时直接分发领域服务，超出覆盖层字段或仅存在于真实 REST API 的 Route 则调用进程内应用路由，不访问 localhost 或外部网络。所有调用仍校验当前项目 UUID、公开参数和统一响应信封；未配置风险策略的写 Route 默认要求用户确认。

引用场景生成文件先持久化到 Asset Store，并绑定当前 project、chat thread、操作、来源 Reference UUID/类型、实际 File UUID 和 `tool_execution_uuid`。用新生成 File 创建或更新 Premise Asset 后，更新后的冻结 Reference 与对应 Tool Result 在同一 Chat 事务提交，后续同 Thread 选择该资源 UUID 会解析到新图。`edit`/`restyle` 的首张 Reference 若明确是另一 Premise Asset，写回目标会以稳定来源不匹配错误拒绝；缺少新元数据的兼容文件不受影响。PATCH 图片、POST 和 DELETE 均可在领域提交后安全重放。revision 冲突保留生成文件，Agent 必须重新 GET 最新 revision 后重试。设定项进入回收站后，新 turn、读取、生图与后续写操作都会被拒绝；升级前已持久化的 typed-tool execution 仍可恢复完成。

## Project Chat 用户输入协议

新 Run 冻结 `project_api_v4`。模型调用 `request_user_input` 只用于确实缺少的信息或关键选择：一次 1–3 题，每题使用唯一 snake_case 逻辑 id、最多 12 字符 header、2–3 个互斥选项、非空说明，且只有第一项 label 以精确的 ` (Recommended)` 结尾。模型不得生成 Other；ChatArea 为每题提供自由输入。该工具必须是模型当次响应中的唯一 Tool Call。活动 schema 仍保留顶层 Lumi `confirmation`，用于运行时生成的危险确认和升级前已持久化请求的恢复；兼容读取只对“唯一问题、顶层无 confirmation、唯一 question 内含 object confirmation”的无歧义旧形状提升到顶层并记录 `confirmation.placement`，双份确认、多问题确认或其他歧义形状仍失败关闭。

`chat_user_input_requests.schema_version` 区分 `codex_questions_v1` 和 `legacy_choice_v1`，`request_json` 是请求唯一事实源。服务端为选项生成公开 UUIDv7；浏览器按 question id 提交一个选项 UUID 或 Other，后端校验所属关系和完整性，再向原 Tool call 写入 `{answers:{question_id:{answers:[label_or_text]}}}`，将同一 Run 排队恢复。请求创建、回答、Tool Result 和 Resume 均以 SQLite 状态为事实；`chat:user_input_*` WebSocket 消息只触发 Query 失效和 REST 重读，不增加轮询。

危险 `request_api` 返回 `agent_tool_confirmation_required` 时，v4 运行时在持久化 Tool Result 的同一 SQLite 事务内，根据原 execution 重新计算 route、project、target、revision 和请求 fingerprint，创建固定的单问题、双选项 `request_user_input` intent；随后主循环直接消费该 intent 并进入 `waiting_for_input`，不会再次请求模型。第一项固定为安全推荐项，第二项才是 `confirm_option`；只有选择该服务端 UUID 才会创建一次具有确定性幂等键的原请求 replay，安全项、Other 和取消均不执行。冻结的 `project_api_v3`、`project_api_v2` 继续按旧单题单选/多选与模型确认语义恢复，`legacy_typed_tools` 继续走隔离恢复，均不会套用 v4 自动确认。

## Chat 与 Workflow 诊断读取

ChatArea 不靠 WebSocket 消息作为事实源：WS 只触发与 `thread_uuid` / `workflow_uuid` 对应的查询失效，业务状态不使用定时 HTTP 轮询；首次 join、重新 join、窗口重新聚焦和重新可见后从 `project.sqlite` 重新读取。运行诊断仅在展开时读取：`workflow:*` 刷新 workflow、runs 和 events，LLM 日志在 pending 与终态提交后发布 `llm_log:changed` 并单独刷新 logs；payload 仅含公开 UUIDv7、状态和必要定位字段。会话列表使用页码分页，消息与 workflow runs/events 使用 cursor，关联 LLM logs 使用页码分页，避免长历史一次性读取。点击 workflow step/run 时使用公开 UUIDv7 `workflow_step_uuid` 筛选关联调用；服务端同时校验该步骤属于当前 project 和 workflow。

Workflow DTO 的 `presentation_mode` 为 `dedicated_thread|inline|none`。inline DTO 额外提供 `origin_turn_uuid`、`origin_run_uuid`、`origin_tool_call_uuid`、`origin_item_uuid` 与 `await_status`；不提供内部 bigint ID 或 River job ID。ChatArea 只让 dedicated Workflow 覆盖独立 Workflow Thread 的标题和顶部卡片；inline Workflow 按 origin Turn 分组并以创建时间、UUID 稳定排序到 Tool 活动附近，因此同一个普通 Thread/Turn 可同时展示多个 Workflow。

Workflow 的公开 DTO、workflow/chat event payload 与 item metadata 在返回前递归净化：移除内部 `id`/`*_id`、路径、Authorization、cookie、credential、password、API key 和 token 字段，并校验所有 `*_uuid` 为 UUIDv7。ChatArea 将 chat events 作为可展开的 cursor 事件流显示，不再只请求后丢弃。普通错误 UI 只显示本地化错误码说明；模型调用诊断只展示安全摘要、模型、状态、tokens、耗时和关联公开 UUID，不渲染底层技术错误或供应商原始报文。

项目 LLM 日志列表支持 Provider UUID/type、模型、scenario、状态、文本/图片类型和关键词组合筛选；关键词只匹配安全摘要、模型、scenario、error code 与 Provider request id，筛选发生在数据库分页前。`filter_groups` 以当前 project/scope 为边界返回可用选项。文本日志额外展示缓存输入 token、输入/输出 Unicode 字符数，以及由正耗时推导的输出 token/s 和字符/s；不可用值统一显示“—”。

## Story 生成的幂等与取消

创建 task 时固化当前 chapter story UUID、revision、完整输入正文、Prompt、Provider、模型和参数。worker 不读取后来变化的 current story 作为替代输入。

成功结果只能通过 Story domain service 追加新的正文版本。`story_generation_results.task_run_id` 唯一，River 重跑不会创建第二个结果；若用户在生成期间手动修改章节，revision check 会拒绝陈旧结果，原正文不被覆盖。

取消按以下顺序执行：先中断应用级 Provider context，再在项目事务中写入 cancel request 并调用 River cancellation。Story 提交事务会再次检查持久取消状态。若 append-only 结果已经提交，则任务继续完成投影，不会产生“内容已修改但任务显示 cancelled”的矛盾状态。

## 错误分类与审计

统一 LLM client 将失败稳定分类为网络、超时、鉴权、限流、Provider 响应、默认模型不可用、内容无效和用户取消；worker 另行投影本地持久化错误。普通 API 只返回安全 message/details，底层技术错误进入本地服务日志。
