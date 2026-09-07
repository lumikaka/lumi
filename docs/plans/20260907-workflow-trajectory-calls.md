---
plan_state: finished
git_commit_message: "feat: expose workflow requests in thread trajectory"
---

# 在现有 Trajectory 中展示 Workflow 请求与真实 Tool 调用

## current_status

2026-09-07 已完成实现和验收。现有 Trajectory 已接入独立 Workflow 的请求归属、调用分页与定位、Request 入口、Inspector、Timeline、统计和日志变更刷新，并同步两份相关 PRD。Request 保留原有圆点交互，每次实际 Workflow 执行增加独立行展示名称、状态和耗时；标题保留 Workflow 标记，来源链放在圆点提示中，Workflow 行与请求分别打开各自的详情。

验收记录：

- 指定设定线程显示 1 次图片 Request、模型 `qwen-image-3.0-pro`、耗时 `47.061 s`、实际 Tool 数 0；安全 Payload/Response 可读，请求深链接刷新后保持选中。
- 漫画失败线程显示 4 次请求，3 次图片尝试的耗时、attempt 与错误码均与 SQLite 记录一致；第三次尝试的 `image_network_error` 可在 Inspector 查看。
- `go test ./internal/agent ./internal/httpapi`、前端 371 项测试及 `pnpm --dir web build` 全部通过。测试覆盖三类 Workflow 日志来源、共享任务去重、同项目其他线程与跨项目隔离、默认 80/最大 200 分页、稳定 cursor/anchor、真实 Tool 及分页外 Request 上下文、未知用量和实时查询失效。查询数量不随请求数量增长。
- Request 圆点交互修正后，前端 373 项测试及构建通过；浏览器验证成功请求与三次失败重试均显示圆点，点击能定位原请求，提示与 Inspector 保留 Workflow 来源。该次修正未改动后端。
- 增加独立 Workflow 行后，前端 375 项测试、构建及 `go test ./internal/agent ./internal/httpapi` 通过。两个样例均显示一条 Workflow 行，分别保留 1 个和 4 个 Request 圆点；工作流自身状态、耗时、输入快照、步骤结果和深链接刷新通过浏览器验证，分页不重复行且不增加 Tool/LLM 统计。
- 连续 Request 圆点修正为左右排列后，前端 378 项测试及构建通过；浏览器验证 4 个请求同排显示且分别选中，回归覆盖 Tool 归属、跨 Turn/内容分隔和历史加载后的圆点定位。
- 未新增数据库迁移，也未为验收重新执行真实生成任务。前端构建保留现有大 chunk 提示。

以下为本次实施采用的需求与方案，现状问题描述保留为验收前的基线。

用户希望在现有 `/projects/:projectUuid/threads/:threadUuid/trajectory` 页面查看实际发生的模型请求与 Tool 调用，包括调用顺序、参数、返回结果、耗时、状态及错误。

主要验收入口：[设定生成线程的 Trajectory](http://127.0.0.1:5801/projects/019ffed5-99c4-7173-bb1b-a037e5869ecc/threads/01a0798c-d86f-7b5a-a2bf-bc37ec9519e9/trajectory)。

2026-09-07 已通过本地 SQLite 只读查询和页面访问确认：

| 验收样例 | 已持久化事实 | 当前问题 |
|---|---|---|
| `01a0798c-d86f-7b5a-a2bf-bc37ec9519e9`，`premise_asset_generation` | 1 次成功的图片请求，模型 `qwen-image-3.0-pro`，耗时 `47061 ms`；没有 Agent Tool 执行记录 | 页面显示“暂无 Timeline 事实”和“当前已加载历史中没有执行项” |
| `01a07161-9abf-708c-a08f-c175e9d90932`，`comic_section_image_generation` | 1 次成功的文本请求、3 次失败的图片请求；图片请求依次为两次 `image_timeout` 和一次 `image_network_error` | 请求存储在 Production 日志中，未接入 Trajectory |

两个样例均属于项目 `019ffed5-99c4-7173-bb1b-a037e5869ecc`，线程类型为 `workflow`，没有普通聊天的 Turn、Run 和 Item。第一个样例的请求 UUID 为 `01a0798c-d926-7979-866b-e81c6bccd990`。

当前实现存在以下接入缺口：

- `internal/agent/trajectory.go` 的模型查询要求 `source_type='project_chat'`，并 INNER JOIN `chat_runs`、`chat_turns`；Workflow/Production 请求无法进入结果。
- API 分页、请求选入当前页和 `item_uuid` 定位均依赖 `chat_items`；无聊天消息的线程需要独立的调用分页路径。
- `trajectoryCollapse.js` 将 Request 转成附着在内容行上的边界按钮；无可附着内容行的请求会消失。
- `llm_log:changed` 当前只使日志相关 Query 失效，没有刷新 `chat-trajectory`。

## overview

### 目标与范围

1. 在现有 URL、Ledger、Timeline 和 Inspector 中接入 dedicated Workflow Thread 的真实模型请求，支持历史数据直接读取。
2. Tool 继续以 `agent_tool_executions` 及其持久化调用/结果关联为事实，展示名称、参数、结果、状态和时间；Workflow 查询路径不能遗漏真实存在的同线程 Tool。
3. 请求没有 Assistant 或 Tool 内容行时，仍保留可选择的 Request 圆点。
4. 普通 conversation 的 Turn、请求边界、Tool 分组和历史浏览行为保持兼容，并纳入回归验证。

本期以已确认的 dedicated Workflow Thread 为入口。Inline Workflow 在 conversation 内展开其内部请求属于后续扩展，本次不扩大普通聊天的日志归属范围。

### 实现边界

- 使用现有事实表构建只读投影，本期不新增业务表或数据库迁移，不回写历史、不重新执行生成任务。
- 继续使用 `llm_logs.uuid` 标识一次模型请求，完整请求/响应通过现有 LLM Log detail 获取。
- Workflow Step、保存文件和业务函数执行不转换为 Agent Tool 调用。未持久化 Tool Execution 时不生成虚构的 Tool 行。
- 模型响应内的 `tool_calls` 可在请求详情中查看；只有调用意图而无结果时不能展示为成功执行。
- 本期不增加 Workflow 执行树、步骤看板、重试按钮或新的生成控制入口。

## data_model

### 请求归属

由服务端在当前 project/thread 边界内批量解析来源，前端不扫描项目级日志列表或按时间、标题、资源 UUID 猜测归属。

| 来源 | 关联方式 |
|---|---|
| 普通 Chat 请求 | 保留 `llm_logs.chat_thread_id`、`chat_run_id` 与 Turn 的现有关联 |
| 原生 Workflow 请求 | 当前线程的 `workflows` → `llm_logs.workflow_id`，步骤信息以真实 `workflow_step_id` 为准 |
| Production 请求 | 当前线程的 Workflow Steps 中已有 `task_uuid` → 同项目 `production_task_runs` → `llm_logs.production_task_run_id` |
| Story 请求 | 当前线程的 Workflow Steps 中已有 `task_uuid` → 同项目 `task_runs` → `llm_logs.task_run_id` |
| Agent Tool | 保留 `agent_tool_executions.thread_id` 与 Call/Result Item 的现有关联 |

要求：

- 内部关联使用既有 bigint 主外键；Steps 中历史 `task_uuid` 仅用于解析既有任务关系，不新增以公开 UUID 充当数据库外键的关系。
- 每一路关联均校验 project/thread 范围，按 `llm_logs.uuid` 去重，避免一个任务关联多个 Step 时重复返回和计数。
- `ListWorkflowLLMLogs` 的任务关联可供参考，但不能将“任务挂在哪一步”直接视为“该任务所有请求发生在哪一步”。漫画样例的素材选择和图片生成共用生产任务，无法证明的步骤归属留空。
- 本期无需向前端返回完整 Workflow/Steps 树；关联用于请求筛选及必要的来源说明。

### DTO 与身份

复用 `TrajectoryPage` 的 `thread`、`turns`、`items`、`tools`、`model_requests`、`compactions`、`overview` 和 `cursor_pagination`。

- `TrajectoryModelRequest` 增加 `source_type` 与 `attempt`。`turn_uuid`、`run_uuid` 允许在 Workflow 请求中缺省；`thread_uuid` 表示当前轨迹归属，不意味着日志已写入 `chat_thread_id`。
- `workflow_origins` 补充真实关联的 Workflow `{uuid, kind, title}`，由同一归属条件批量读取、按 Workflow 去重。原始 `source_type` 保持不变，圆点提示展示“Workflow：名称 → 生产任务”等来源链，详情展示 Workflow UUID；未关联的 Chat 请求不标为 Workflow 调用。
- `workflows` 返回当前项目与独立线程的工作流摘要，包含 UUID、名称、类型、实际状态、当前步骤、开始/结束时间和安全错误。生命周期耗时只由已记录时间计算；状态不由子请求推断。作为每页共享上下文返回，不占调用 cursor，前端按 `workflow:uuid` 去重。
- Workflow 请求的 `request_ordinal` 在当前线程全部去重请求中，按 `created_at, uuid` 排序计算；编号独立于当前页。`attempt` 保留原始任务尝试次数，两者不得混用。
- 普通 Chat 的 `request_ordinal` 继续采用当前运行时定义。稳定身份始终为 `(source_kind, source_uuid)`，不依赖编号或数组下标。
- 无对应生命周期事件的 Workflow 请求不填造 `chat_events.sequence`，沿用近似排序标记；真实 `duration_ms` 不因缺少事件序号而丢失。
- 图片请求与缺失 usage 的请求使用不可用值，不能把数据库默认 0 当成已记录的 token、缓存、TTFT 或吞吐率。
- Tool 保留原始执行状态和真实 Request 关联；API 结果信封为 `success:false` 时仍按失败展示。

## api

### 既有接口扩展

```text
GET /api/v1/projects/:project_uuid/chat_threads/:thread_uuid/trajectory
    ?before=&after=&limit=&item_uuid=

GET /api/v1/projects/:project_uuid/llm-logs/:log_uuid
```

响应继续使用 `{ "success": true, "data": ... }` 或统一失败信封，字段采用 `snake_case`，URL 与 JSON 只返回公开 UUIDv7。

在 `ListTrajectory` 确认线程与项目归属后，选择普通 Chat 或 dedicated Workflow 查询路径。Workflow 查询可放入 `internal/agent/trajectory_workflow.go`，共用现有净化、Model Request DTO、Tool DTO 和统计函数。

### Workflow 调用分页

1. 以去重后的模型请求与真实 Tool Execution 构成调用索引，不要求存在 `chat_items`。使用不可变的 `created_at, source_kind, source_uuid` 作为稳定排序键。
2. Workflow cursor 使用独立的版本化不透明编码，包含当前 thread UUID 和上述排序键；不能将其解码为聊天 Item sequence。普通 Chat 的旧 cursor 保持兼容，跨线程、错误类型和非法 cursor 返回校验错误。
3. 默认读取最新一页，沿用默认 `80`、最大 `200` 的 limit；支持向前加载、`after` 增量查询与单独的 `item_uuid` 定位，保留现有互斥校验。
4. `item_uuid` 能直接定位当前线程的 `llm_logs.uuid`，以及既有 Tool Execution/Tool Call、Workflow 的公开 UUID；校验实际归属，不能只验证 UUID 格式。选中 Workflow 时保留默认调用窗口。
5. Tool 的关联 Request 如位于当前分页边界外，可作为关联上下文一并返回。Cursor、limit 和 `has_more` 只按主要调用索引计算，不能被补充上下文改变；前端按稳定 key 去重。
6. `history_complete` 依据该线程是否还有更早的调用事实计算，不能因 `chat_items` 为空就视为完整。
7. 全线程 `overview` 与紧凑 Timeline 从同一归属集合计算；请求计数和耗时不随已加载页数变化。复用一次读事务保持响应内部一致，采用批量查询，避免逐日志 N+1。

列表只返回必要摘要、选项和诊断信息；完整 payload/response 仍按请求 UUID 延迟加载。沿用现有脱敏规则，避免内部 ID、磁盘路径、凭据和二进制图片进入投影。

## ui

### Ledger 与请求选择

- Chat 与 Dedicated Workflow 的 Request 均为圆点入口；悬浮或键盘聚焦展示如 `Request #1 · 图片 · qwen-image-3.0-pro · 已完成 · 47.061 s` 的提示。
- 复用 `rowType: 'request'` 与 `sourceKind: 'model_request'` 作为逻辑事实，展示层只渲染圆点，不显示整行 Request 标签或内容卡片。
- Workflow 的文本/图片输出摘要留在 Inspector，不额外伪造 conversation 的 Assistant/System Prompt 变化记录。
- 有真实关联可见内容行时，圆点附着在原有请求边界；没有可见关联行时，在请求位置保留紧凑圆点。按 Request 筛选、搜索或 Timeline 定位后也不能把唯一匹配的请求隐藏。
- 不将请求边界挂到无真实关联的另一次调用上；每次请求只保留一个圆点入口，后续出现真实关联内容时沿用同一请求身份。
- 同一 Turn 内、没有内容分隔的连续 Request 圆点左右排列；无内容请求和相邻的请求边界共享一排，Tool 的真实请求归属保持不变。不跨 Turn 或内容行合并，宽度不足时换行；分页前后按各圆点稳定身份恢复浏览位置。
- 真实 Tool 沿用现有行与 Inspector。没有 Tool 记录时计数为 0，关闭无可折叠调用的 Calls 操作，不创建 Workflow Step 对应的 Tool。
- 每次真实 Workflow 执行呈现一条独立 Workflow 行，展示名称、状态与已记录耗时；未发起请求时也可见。内部 Request 保持各自圆点，Workflow 行不作为子请求的边界锚点，不制造 Tool 或 Assistant。
- 选择写入现有 `item_uuid`/`item_kind` 查询参数；直接打开请求链接、刷新、加载历史、Query upsert 后均保持选择及滚动锚点。

### Inspector、Timeline 与统计

- Request 复用摘要、请求参数、返回结果、耗时和安全 JSON 标签；参数/结果直接读取现有项目 LLM Log detail，无需额外跳转。
- 摘要展示请求类型、模型、来源、请求编号、原始尝试次数及失败诊断。Workflow 没有 Turn/Run 时不要求提供占位的聊天层级。
- Tool 继续展示名称、参数、结果、执行耗时以及所属真实请求；只有 Tool 支持的 Schema 信息才展示对应入口。
- Workflow 行选择使用 `item_kind=workflow`，展示摘要、当前步骤、输入快照、步骤结果、时间与安全原始数据，详情复用既有 Workflow REST 接口；工作流显示名称可搜索，生命周期耗时不叠加到请求 Timeline 和 LLM/Tool 汇总。
- Timeline 的四种模式都纳入 Workflow 请求；点击和范围筛选能够定位并选中 Request 圆点。一次请求只统计一次耗时。
- 工作流统计显示请求数量、真实 Tool 数量和已记录耗时，不制造 Turn 数；图片请求未提供的文本指标显示不可用或按现有规则隐藏。
- 搜索覆盖已加载请求的模型、请求类型、来源、scenario、状态、错误码和摘要，打开详情后继续增量收录安全参数/结果；历史未完整加载时保留提示。
- 新增或调整状态按钮样式时显式保留 selected/active/`aria-pressed` 与 hover 的组合规则。

### 实时更新

复用项目级 `/api/v1/ws` 与 `topic/event/payload/ref/join_ref` 信封：

1. `llm_log:changed` 在保留日志列表和详情失效行为的同时，使 `['chat-trajectory', projectUuid]` 前缀失效。该事件已有 project/log UUID，无须为本期增加后台请求生命周期写入。
2. 前缀同时覆盖主查询和 `anchor` 查询；仅活动查询自动重读，未挂载缓存标记失效。后端查询仍按各自 thread 隔离事实。
3. 保留已有 `workflow:*`、`chat:*` 的精确线程失效。Tool 的意图、执行和终态继续由现有聊天变更提示同步。
4. 继续执行首次 join、重新 join 和窗口重新聚焦的 REST 校准。WebSocket 不传完整执行数据，不增加定时 HTTP 轮询。

## others

### 实施顺序

1. **后端接入与回归夹具**：补齐 Workflow 的三类日志来源、去重、独立 cursor、请求定位和真实 Tool 查询；针对无聊天 Item 的线程建立可复现测试。
2. **前端请求呈现**：完成统一 Request 圆点、无内容时的入口、Inspector、搜索、Timeline 和统计；验证现有 Chat/Tool 折叠行为。
3. **同步与端到端验证**：补齐 Query 失效，使用本地样例只读验证请求、详情和定位；回归普通聊天与多次重试，更新 PRD。

### 主要文件

| 职责 | 位置 |
|---|---|
| 查询、分页与公共 DTO | `internal/agent/trajectory.go`、新增 `internal/agent/trajectory_workflow.go`、`internal/agent/trajectory_test.go` |
| REST 契约与测试 | `internal/httpapi/agent.go` 及对应测试 |
| 请求投影与可见性 | `web/src/pages/trajectory/trajectoryProjector.js`、`trajectoryCollapse.js`、`TrajectoryLedger.jsx` |
| 选择、详情、筛选和时间轴 | `ThreadTrajectoryPage.jsx`、`TrajectoryInspector.jsx`、`trajectorySearch.js`、`trajectoryTimeline.js`、`trajectoryStats.js` 及对应组件/测试 |
| 实时、文案与样式 | `web/src/realtime/projectRealtimeQueries.js`、`web/src/i18n/messages/trajectory.js`、`web/src/styles/trajectory.sass` |

### 验收与测试

| 场景 | 验收结果 |
|---|---|
| 指定设定生成线程 | Ledger 和 Timeline 显示请求 `01a0798c-d926-7979-866b-e81c6bccd990`；状态 completed、耗时 `47061 ms`、模型正确；请求总数 1、Tool 总数 0 |
| 请求详情与直接链接 | 点击或直接访问 `?item_uuid=01a0798c-d926-7979-866b-e81c6bccd990&item_kind=model_request` 后打开对应 Inspector，能读取安全请求/响应，刷新仍定位成功 |
| 漫画失败线程 | 显示 4 次独立请求，文本 1 次、图片 3 次；图片的 attempt 为 1/2/3，错误码依次为 `image_timeout`、`image_timeout`、`image_network_error`；请求编号与身份不重复 |
| Workflow 来源与归属 | workflow、production、story_generation 来源均被覆盖；同任务多 Step 不重复计数，不混入同项目其他线程或其他项目日志；跨线程 anchor/cursor 被拒绝 |
| 超过一页、相同时间戳 | tail、before、after、anchor 分页无漏项/重复；补充关联 Request 不改变 cursor；prepend 与 upsert 后 key、选择、滚动位置稳定 |
| 无内容行与未完成请求 | pending、failed、无 response、图片输出及没有 Assistant/Tool 的请求均可见；仅筛选 Request 或仅命中某个请求时也可选择 |
| 真实 Tool 与普通聊天 | 正常调用、`success:false`、运行中和中断调用均保持真实状态；参数/结果可读；无记录的 Workflow 不新增 Tool 行；普通聊天请求边界与折叠回归通过 |
| Realtime 与校准 | 日志开始/完成后主查询与 anchor 经 WS 提示失效并 REST 重读；断线重连和窗口聚焦可恢复；无定时 HTTP 轮询 |
| 数据质量与脱敏 | 原始未知指标不显示为测得的 0；响应与 cursor 不含内部 ID、凭据、磁盘路径或二进制图片；列表不复制完整请求/响应 |

实施时运行受影响的 Go 与前端测试，并执行前端构建：

```text
go test ./internal/agent ./internal/httpapi
pnpm --dir web test
pnpm --dir web build
```

新增测试应验证真实数据归属、分页、可见性和查询失效行为；浏览器验收在上述现有 URL 完成，不为验收重跑真实图片生成。遵守仓库约束，不在本地运行 Cargo 或 Rust 编译、检查和测试。

## prds

实施完成后同步：

- `docs/prds/ai_runtime/features/AI调用可观测性.md`：补充 Thread Trajectory 对 Workflow 请求的归属规则、详情入口和未知指标语义。
- `docs/prds/workflows/features/可恢复多步工作流.md`：补充 dedicated Workflow Thread 的调用轨迹入口，以及 Workflow Step 与真实 Tool 的区分。
- 若现有数据模型说明包含 Trajectory DTO，再补充新增投影字段；本次无数据库表结构变更。
