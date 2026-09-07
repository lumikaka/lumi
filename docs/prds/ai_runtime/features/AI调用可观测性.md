# AI 运行时 — AI调用可观测性

## overview

该 Feature 为单机项目提供统一、可恢复的 AI 调用审计视图。Story、Project Chat、Premise/漫画 Production 与 Workflow 调用都进入同一项目日志；用户可以按 Provider、模型、scenario、状态、请求类型和关键词组合筛选，并从公开 Task、Thread、Run、Workflow UUID 追踪调用上下文。

新 Chat 文本和图片调用的 scenario 统一为 `project_chat`，对外不返回 Scene。项目级日志包含全部 Chat；Premise scope 只展示 Premise Production 调用。历史 scenario 继续可筛选和阅读，但不会影响新日志归类。

列表只搜索受限长度的输入/输出摘要、模型、scenario、错误码和 Provider request id，不对原始请求/响应 JSON 做无界扫描。详情保留既有安全 payload 阅读器，但不保存或返回请求 header、API Key、Authorization、二进制图片或内部数据库 ID。

文本调用在 Provider 返回 usage 时记录输入、缓存输入和输出 token。输入/输出字符数统一统计 JSON 字符串值中的 Unicode code point；输出 token/s 和字符/s 根据正耗时读取时推导。Provider 未提供的缓存指标、迁移前日志、图片请求和无效耗时显示不可用，不伪造为 0。

## data_model

`llm_logs` 通过内部 bigint 外键关联 Task、Production task、Chat 或 Workflow 上下文；`uuid` 与关联资源 UUIDv7 是唯一外部身份。迁移新增可空 `cached_input_tokens`、`input_characters`、`output_characters`，并为项目 + Provider、项目 + scenario 建立历史索引。速度字段是读取投影，不写入 SQLite。

Thread Trajectory 是已有事实的只读投影。独立 `workflow` Thread 按同项目的 Workflow 关联原生 `workflow` 日志，并由 Steps 已保存的 `task_uuid` 解析 `production_task_runs` / `task_runs`，纳入 `production` / `story_generation` 请求；同线程真实 `project_chat` 请求仍保留。所有来源按日志 UUID 去重，不能依据时间、标题或资源 UUID 猜测归属，也不能从任务挂载位置推断请求发生的具体 Step。普通 `conversation` 的轨迹继续只读取其 Chat 请求。

`model_requests` 返回 `source_type`、原始 `attempt` 和 `request_ordinal`。Workflow 请求序号按全线程去重请求的 `created_at, uuid` 计算，独立于尝试次数及分页；稳定身份为 `model_request` 与日志 UUID，缺少真实 Turn/Run 时关联为空。实际 Tool 读取 `agent_tool_executions`，保留调用/结果与真实 Request 关联。Workflow Step 不生成 Tool Execution。图片和缺少记录的文本指标保持不可用，未完成请求不把默认耗时 0 当作已测耗时。

请求的 `workflow_origins` 返回当前项目与线程内、根据同一持久化关系确认的 Workflow `{uuid, kind, title}`，按 Workflow 去重；无关系时不返回。保留日志原始 `source_type`，因此 Production 请求同时表达“由哪个 Workflow 关联”和“实际由生产任务执行”。不根据线程类型给普通 Chat 请求补造 Workflow 归属。

独立 Workflow Thread 的 `workflows` 直接读取当前项目与线程的工作流记录，包含公开 UUID、名称、类型、状态、当前步骤、时间和安全错误。生命周期耗时仅由已记录的开始与结束时间计算，不由子请求状态或耗时推断。尚未发起请求时也能展示工作流。

## api

| 接口 | 方法 | 说明 |
|---|---|---|
| `/api/v1/projects/:project_uuid/llm-logs?page=&per_page=&scope=` | GET | 返回 `{items,pagination,filter_groups}`；可组合 `provider_uuid`、`provider_type`、`model`、`scenario`、`status`、`request_type`、`keyword`，筛选发生在分页前。 |
| `/api/v1/projects/:project_uuid/llm-logs/:log_uuid` | GET | 返回单条日志及安全请求/响应 JSON；关联上下文只含公开 UUIDv7。 |
| `/api/v1/projects/:project_uuid/chat_threads/:thread_uuid/trajectory?before=&after=&limit=&item_uuid=` | GET | 返回 Thread 调用事实、全线程统计与 Timeline。Workflow 调用无需 Chat Item 即可分页和定位；详情继续按请求 UUID 延迟读取。 |

`filter_groups` 以当前 project/scope 为边界返回 Provider、Provider type、model、scenario、status 和 request type。列表按 `created_at DESC` 与稳定 UUID 次序分页；筛选非法时返回统一 `validation_failed` 错误信封。

Workflow Trajectory 使用绑定当前 Thread 的版本化 cursor，按不可变的 `created_at, source_kind, source_uuid` 排序，默认 80、最大 200 条主要调用。`before`、`after`、`item_uuid` 互斥；跨线程或非法 cursor 拒绝读取。`item_uuid` 可定位当前线程请求、真实 Tool 或 Workflow。`workflows` 作为线程上下文随每页返回，分页外的 Tool 关联 Request 也可作为上下文补充，均不改变调用分页边界。全线程统计不随已加载页数变化；普通 Chat cursor 保持既有语义。

## ui

| 页面 / 入口 | 说明 |
|---|---|
| 项目或 Premise 的 LLM Logs 面板 | 提供有 label 的组合筛选、关键词提交和重置；筛选变化回到第一页，窄屏改为单列。 |
| 日志表格与详情 Dialog | 展示 token、缓存 token、输入/输出字符数、token/s、字符/s、耗时和公开诊断关联；图片与缺失指标显示“—”。 |
| `/projects/:projectUuid/threads/:threadUuid/trajectory` | Chat 与 Workflow 请求统一使用圆点入口，悬浮或键盘聚焦显示类型、模型、状态、耗时、重试次数和错误码；Inspector 展示摘要、安全参数、响应及时间。Timeline 四种模式、已加载内容搜索和范围筛选均覆盖请求。 |

请求选择写入 `item_uuid` 与 `item_kind=model_request`，刷新和历史加载后继续定位。Request 圆点附着于真实关联的可见内容行；没有内容行时，在原请求位置保留紧凑圆点，不显示整行 Request 标签或内容卡片。请求筛选和失败、pending 状态也保留可选圆点。Workflow 汇总显示请求数、真实 Tool 数及已记录耗时，无真实 Tool 时显示 0 并禁用 Calls 折叠。

同一 Turn 内、没有内容分隔的连续 Request 圆点按执行顺序左右排列；无内容请求与紧随其后的请求边界共享一排，各自保留原 UUID、状态和点击入口，不把它们都归属于后面的 Tool。不同 Turn 或被内容分隔的请求保持独立。宽度不足时圆点自然换行，实际行高参与虚拟滚动测量；加载更早请求后仍能按圆点身份恢复浏览位置。

独立 Workflow 页标题显示 Workflow 标记；请求圆点的提示显示来源链，例如“Workflow：设定项图片生成 → 生产任务”。Inspector 在摘要顶部列出所属 Workflow 的名称与公开 UUID，原生日志直接显示 Workflow 名称。工作流名称遵循项目绘本或条漫术语，来源 UUID、类型和标题纳入已加载轨迹搜索。

每个实际 Workflow 在 Ledger 中有一条独立行，展示名称、状态及已记录的生命周期耗时。按 UUID 去重，分页和状态更新保持同一行身份；Workflow 行与 Request 圆点分别选择，工作流选择写入 `item_kind=workflow`。点击工作流行查看摘要、当前步骤、输入快照、步骤结果、时间与安全原始数据，输入输出按现有 Workflow REST 详情接口延迟加载。Workflow 可按类型和显示名称搜索，不计入 Tool 数，生命周期耗时不叠加到请求 Timeline 或 LLM/Tool 总耗时。

## others

该能力不估算 credits、订阅费用或 Provider 实际账单。WebSocket 只用于触发查询刷新，REST 与项目 SQLite 仍是事实源。

项目级 `llm_log:changed` 同时使日志查询和 `['chat-trajectory', projectUuid]` 前缀失效，覆盖主查询与 anchor 查询。活动页面经 REST 重读，其余缓存标记失效；继续执行首次 join、重新 join 和窗口重新聚焦校准，不使用定时 HTTP 轮询。
