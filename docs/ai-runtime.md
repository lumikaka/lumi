# AI Provider 与项目任务运行时

Lumi 的 AI 能力是可选的本机基础设施。没有 Provider、系统密钥库暂不可用或网络断开时，项目与 Story 手动工作台仍可正常使用。

## 存储与安全边界

- 全局 `lumi.sqlite` 的 `site_settings` 保存 Provider UUIDv7、Cloudflare Account ID、默认文本/图片模型、启用状态和加密后的 secret envelope。
- 首次配置使用独立前端地址 `/setup/`，先展示可选服务商列表，再通过所选服务商的 Dialog 完成连接。Cloudflare AI Gateway 的首次 Dialog 只要求 32 位 Account ID 和 Cloudflare API Token；默认文本/图片模型均为 `openai/gpt-6-sol`，首次配置与后续管理页也可分别选择 `openai/gpt-5.6-terra` 或 `openai/gpt-5.6-sol`。Base URL 不允许用户输入，由后端固定派生为 `https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1`。
- Cloudflare API Token 由操作系统 Keychain 根密钥派生的 AES-256-GCM 密钥加密，密文保存在 `site_settings`；测试使用进程内 `MemoryMasterKeyStore`。API 不提供读取密钥的端点。
- 项目 `project.sqlite` 固化调用时的公开 Provider UUID、Base URL、模型、生成参数、Prompt 和章节 revision，但从不复制 API Key 或 Authorization header。
- REST、WebSocket 与 React 状态只暴露产品 UUID。`river_job_id`、`execution_uuid` 与 River 表只存在于后端内部关联中，不增加公开 DTO 字段。
- LLM 日志只保存受限长度的文本摘要、合法 JSON payload、token usage、Unicode 字符数、耗时、finish reason 和安全错误码，不保存请求 header、secret 或二进制内容。缓存输入 token 只在 Provider 返回 usage details 时记录；旧日志和图片调用保持 `NULL`，输出速度读取时推导。

Cloudflare 连接验证、文本、Agent 和图片请求统一使用 AI Gateway REST API 的 `/ai/v1/responses`；图片生成和参考图编辑继续使用 `image_generation` tool。第三方模型走账户默认 Gateway；支持 Responses 的 `@cf/` Workers AI 模型会显式发送 `cf-aig-gateway-id: default`。该 Provider 不兼容任意 OpenAI-compatible Base URL。阿里云百炼保留独立的 Chat Completions 文本与流式验证流程。

Cloudflare 的默认文本模型与默认图片模型均为 `openai/gpt-6-sol`，用户可以分别切换为 `openai/gpt-5.6-terra` 或 `openai/gpt-5.6-sol`，通过 [Responses 接口](https://developers.cloudflare.com/ai-gateway/usage/rest-api/) 调用。配置页提供两个模型下拉框，公开设置接口仅接受这三个内置模型；模型选择也用于全局和项目的模型列表。修改默认模型后需要重新验证。升级时保留受支持的内置模型选择，清除其他旧模型覆盖与 Cloudflare 的旧验证状态，保留双方凭据、百炼验证状态和当前服务商选择，重新验证后可继续使用。

Responses 请求使用 `input`、`max_output_tokens` 和 `store: false`。文本生成消费 `response.output_text.delta` 并要求有效终态；Agent 使用 `function_call` / `function_call_output`，以 `call_id` 关联工具结果。返回的 reasoning、函数调用和消息 phase 保存在现有 LLM 响应快照中，构建上下文时按请求 UUID 恢复，以支持无服务端会话状态的多轮调用和进程重启。连接验证发送非流式请求与 512 token 上限；因 `max_output_tokens` 结束的有效响应可以证明模型访问成功，业务生成仍拒绝截断结果。

验证失败的 `error.details` 保留经过脱敏的上游 HTTP 状态、错误码、消息及 request ID（包括 Cloudflare `errors[]` 和 `Cf-Ray`），以区分 Lumi 的 502 与真实上游错误。

## OpenAI 中转站生图协议

文本组固定使用 Responses API。生图组通过 `ai_providers.openai_relay.image_api_mode` 选择 `responses` 或 `images`，默认及缺省值均为 `responses`。旧凭据、默认模型、验证指纹保持兼容；切换模式或修改模型后重新验证。模式与默认图片模型必须一起满足后端校验，不能保存或重置成不兼容的组合。

Responses 模式保留原 `image_generation` 工具调用及模型选项。Images 模式提供 `gpt-image-2`、`gpt-image-2.5-flare`、`gpt-image-2.5-sunburst`，具体可用性取决于中转站。模型与请求格式参考 [GPT-Image2-Skill](https://github.com/wuyoscar/GPT-Image2-Skill/blob/main/src/gpt_image_cli/cli.py)：无参考图请求 `POST {base_url}/images/generations`（JSON）；有参考图请求 `POST {base_url}/images/edits`（multipart，按顺序上传 `image[]`）。显式发送模型、`n=1` 和 PNG 输出格式，不传 `response_format`。解析 `data[].b64_json`，兼容图片 URL 的受限下载。请求失败不自动切换协议。

图片预检、草稿定稿、自动生成工作流及漫画图片任务使用同一个尺寸解析器，显式传入当前图片调用模式。中转站 Responses（含缺省模式）和 Images 均按 `1024×1024`、`1536×1024`、`1024×1536` 三档精确比例接入，分别对应 1:1、3:2、2:3；Images 仍按图片模型目录校验，不因模型名称较新就开放自定义尺寸。4:3、3:4 等比例在预检、定稿和 YOLO 创建前返回 `image_aspect_ratio_unsupported`，不裁剪或近似替换。新项目对话按实际图片供应商提供比例建议；中转站且用户未指定精确比例时建议 custom 3:2、custom 2:3 或 square，用户明确指定的比例不得擅自改动。竖条继续沿用现有分段尺寸。百炼和 Cloudflare 的尺寸规则保持不变，Cloudflare 图片工具继续支持自定义尺寸。错误详情区分模式／模型不匹配与三档尺寸限制，并附带公开模型、模式和比例信息。历史任务重试仍沿用已冻结的输入快照。

Provider 响应提供 `image_api_mode`、`image_models_by_mode` 和 `text_models`，设置页及全局、项目模型选项使用后端目录。切换模式后，不兼容的项目或全局图片覆盖保留并显示失效，不静默改用其他模型。切换时需要重新选择兼容模型。

保存并验证依次检查文本组和生图组；生图组复用实际图片适配器生成一张 `1024x1024/low` 测试图，使用图片限流和超时，会产生调用费用。生成检查不能证明编辑能力可用。检查结果绑定配置指纹，检查期间配置变化时拒绝写入通过状态。百炼的 DashScope 生图和 Cloudflare 的 Responses 生图均保持原实现。

Agent 图片工具、模型测试和生产任务均传递模式。生产任务快照冻结 `image_api_mode`，执行和重试沿用快照；历史中转站快照缺少字段时走 Responses。图片日志记录 `image_api_mode`、相对 `api_path`、模型及用量，不记录主机地址、密钥或参考图字节。

## 应用级模型联调测试

「设置 → 工具 → 测试模型联通」创建独立测试记录，分别选择文本和图片模型后，一键顺序执行连接探测、短文本生成和单张测试图片生成。已配置但尚未验证的 Provider 也可参加测试；测试不会改变 Provider 验证状态或全局默认模型。第一步复用连接探测协议，第二步要求有效非空文本，第三步直接调用图片适配器，不经过 Agent。中转站 Images 模式下第三步使用内置 64×64 PNG 参考图验证 `/images/edits`，其余模式继续无参考图生成。任一步失败立即停止，后续步骤标记为未执行；再次测试创建新记录。

测试与步骤保存在应用数据库 `model_tests`、`model_test_steps`，使用内部整数外键和公开 UUIDv7，独立于项目级 LLM 日志。图片保存在应用数据目录 `model-tests/`，通过受桌面认证保护的 `/media/model-tests/:test_uuid/steps/:step_uuid/content` 读取。诊断仅保留脱敏错误码、消息、HTTP 状态和请求标识，不保存凭据或图片 Base64。

`POST /api/v1/model-tests` 接收 `text_model` 和 `image_model` 的 `{provider_uuid,model}`，冻结选择、创建排队记录并返回 202；`GET /api/v1/model-tests` 使用 `page/per_page` 分页；`GET /api/v1/model-tests/:test_uuid` 读取步骤事实；`PUT /api/v1/model-tests/:test_uuid/execution` 使用相同参数启动历史草稿并返回 202。重复提交相同选择只读回既有执行，不同选择返回 409。

排队中和运行中的测试可在详情页点击「终止测试」。`DELETE /api/v1/model-tests/:test_uuid/execution` 将该测试标记为 `interrupted`，取消正在执行的请求或限流等待，保留已完成步骤并将未开始步骤标记为 `skipped`，返回 200 和当前测试。重复终止或终止已结束测试只返回已有结果；终止不会影响其他测试，迟到的模型结果不能覆盖终止状态。

应用生命周期管理的单工作器串行消费已入库测试，不依赖页面请求存活。连接、文本、图片步骤的总时限分别为 60 秒、120 秒和 10 分钟，包含全局 Provider 限流等待。Cloudflare 测试图使用 `1024x1024/low`；百炼使用 `1536x1536`，关闭扩写和思考。退出或重启后遗留任务标记为已中断并保留已完成步骤，不自动重试。状态落库后向现有 WebSocket `system` topic 发布 `model_test:changed`，payload 仅含 `test_uuid`；前端失效 Query 并 REST 重读，同时在首次 join、重新 join 和窗口聚焦时校准，无 HTTP 轮询。

## River 版本与兼容性 gate

当前固定并验证的 River 版本是 `v0.41.0`，仅使用官方 `github.com/riverqueue/river`、`riverdriver/riversqlite` 和 `rivermigrate`。SQLite driver 仍由 River 官方标记为 early testing，因此任何 River 升级都必须先运行：

```bash
go test ./internal/jobqueue -run TestRiverSQLiteCompatibilityGate -count=1
```

该测试使用 Lumi 当前纯 Go SQLite driver、真实临时文件、WAL、5 秒 busy timeout 和 `MaxOpenConns(1)`，验证官方 migration、同一 `database/sql` 事务的提交/回滚、领取、完成、快速重试、scheduled job、active uniqueness、取消、graceful stop，以及关闭后重开。不得用内存 SQLite 替代此 gate。

River client 和类型只出现在 `internal/jobqueue`。domain、HTTP 和 React 使用 `task_runs` / `task_events` 产品投影，升级不得把 River job row 变成公开 contract。

## 生命周期与恢复

1. 取得项目锁，ProjectStore 完成 Lumi migration 和一致性备份。
2. AI runtime 检查单连接边界；River schema 需要升级时先额外创建 `project-before-river-*.sqlite` 备份，再运行官方 `rivermigrate` 和 validation，构造尚未启动的 client。
3. 在 worker 与周期任务启动前核对执行关联，再恢复业务投影和 Agent 持久安全边界。Agent 恢复显式接收项目队列的事务入队函数，不替换全局 Service 依赖。
4. 恢复提交后订阅事件、启动 River worker 和周期任务，再将运行时对外开放。Runtime 注册表以 `project_uuid` 路由任务，打开 B 不停止 A。每项目 `story_generation`、`asset_maintenance`、`agent` 的 `MaxWorkers` 为 1，`production` 为 5。
5. 创建任务时，产品记录、初始 append-only events、Agent run、River job 和执行绑定使用同一个 `*sql.Tx` 提交。domain 唯一索引与 River unique job 同时限制同资源并发。Worker 只在短事务中读写状态；Provider 网络调用期间没有数据库事务或长期写锁。
6. 切换标签页 URL 不操作 Runtime。仅目标项目显式关闭、独立空闲回收或应用退出时，River 才停止领取并最多 soft-stop 5 秒，再取消剩余 worker context；Runtime 未成功停止时对应 Store 和项目锁保持打开。安全幂等任务保持可恢复状态，重开项目后按执行归属恢复。

### 当前执行关联

业务任务永久保存，River job 是可以按正常 retention 清理的临时执行。SQLite 可能重新分配已清理的 job 数字编号，因此 `river_job_id` 不能作为历史执行身份。Lumi 的 `task_runs`、`production_task_runs`、`asset_maintenance_runs`、`chat_turns`、`workflow_steps`、`workflow_awaits` 增加可空 `execution_uuid`；保持原有唯一索引，River 自有源码、SQL、表、索引、迁移与 retention 不变。

每次新入队生成 UUIDv7，业务记录和版本 2 args 保存同一个执行 UUID；River 对该 job 的自动重试沿用它。worker kind 注册名称保持不变，执行 UUID 不参与活动任务的唯一键。版本 1 job 只有在项目、任务类型、业务 UUID、资源归属和当前数字绑定全部匹配时才可继续执行。River 返回 duplicate 时采用返回行的真实执行 UUID；归属不符则报状态冲突。

绑定新 job 编号前，在同一事务中查找占用该编号的 Lumi 记录，通过 River args 校验归属并释放失效绑定。因此即使队列最大 ID 为 8、历史任务仍占用 9，首次新建也能提交，不通过回滚重插来推进编号。

释放必须以 River 实际进入 `completed|cancelled|discarded` 为边界。业务先变为完成、失败或取消时，运行中的 job 仍保留绑定；终态事件先补齐必要业务投影，再按预期任务、job ID 和执行 UUID 同时清空两个关联字段。排队、运行、scheduled、retryable、pending job 保留绑定。结束事件用于及时释放；启动检查、取消和显式重试也能修复缺失事件留下的关联。

`internal/taskexecution` 将执行归属随 context 传入业务层，并在同一个 SQL/GORM 写事务内校验。旧执行的结果、进度与事件无法写入新执行，进程内取消句柄也按任务 UUID、job ID、执行 UUID 区分。即使同一任务再次获得同一数字编号，旧 worker 清理和迟到通知也不能影响新执行。

### 旧项目恢复与显式重试

启动修复先释放全部失效绑定，再查找可唯一确认的活动执行并重新关联。没有可采用 job 的活动任务从冻结输入和持久化进度重建一次；重复打开不会重复入队或重置已完成步骤。历史 `failed|cancelled|completed` 保留状态。无法安全重放、输入损坏或归属不唯一的任务转为明确的 `interrupted`，避免阻塞其他新任务。已开始的破坏性 GC 不自动重放。真正的数据库错误会中止恢复并返回，提交成功后日志记录修复类别、数量和任务 UUID。

显式重试保留任务 UUID、输入快照、冻结模型、已有结果和历史日志，创建新的执行 UUID 与 River job；不使用 `JobRetryTx` 复活历史编号。旧执行尚未结束时最多等待两秒，超时返回状态冲突；并发重复请求最多创建一个执行。新 job 的 attempt 从头计数，沿用原任务的最大尝试次数与可重试限制。YOLO 从未完成步骤恢复，已经提交的步骤输出保留。

聊天等待输入或 Workflow 依赖时，业务可以仍为活动状态，而对应 job 已成功结束并释放绑定。合法唤醒创建新执行，仍以 `wakeup_uuid` 去重；Turn 和 Await 的新绑定在同一事务提交。旧执行的终态通知只释放匹配自己的引用。

关联回归使用真实 River/SQLite：`execution_bindings_test.go` 覆盖编号复用、旧版 args、duplicate、错误归属取消、显式重试、回滚、丢失事件、重复恢复与聊天等待；`internal/taskexecution` 测试 SQL、GORM 和 `RETURNING` 写回的执行隔离。队列与 Agent 的并发用例同时运行 Go race 检查。

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

取消在项目事务内核对当前执行和 River args 的归属，再中断该执行的 Provider context、写入 cancel request 并调用 River cancellation。编号已被其他任务复用时，只释放本任务失效关联。Story 提交事务会再次检查执行归属与持久取消状态。若 append-only 结果已经提交，则任务继续完成投影，保持内容和任务状态一致。

## 错误分类与审计

统一 LLM client 将失败稳定分类为网络、超时、鉴权、限流、Provider 响应、默认模型不可用、内容无效和用户取消；worker 另行投影本地持久化错误。普通 API 只返回安全 message/details，底层技术错误进入本地服务日志。


### 长任务超时与步骤恢复

自动创建 Workflow 的同步文本生成（含漫画正文分镜和封面脚本），以及独立漫画分镜任务，单次 Provider 请求默认等待 15 分钟。通过 `LUMI_WORKFLOW_REQUEST_TIMEOUT=15m` 配置，支持 `1s` 至 `1h` 的 Go duration；修改后重启服务生效。普通聊天 HTTP 请求仍为 5 分钟。Workflow worker 为同一步中的正文、封面两次请求及收尾预留预算，River 的卡住任务回收期限始终大于 worker 预算。

自动创建 Workflow 的步骤遇到可重试的 Provider 超时或网络错误时，通过 River 退避自动重试一次（同一持久执行最多两次尝试）。等待重试期间 Workflow/步骤保持 queued，原对话的 await 不会提前结束；每次 Provider 请求仍独立记录 LLM 日志。鉴权、无效内容、用户取消不会触发此自动重试。耗尽后步骤标记失败，现有重试入口只恢复第一个未完成步骤，保留之前已完成的故事、设定及页面结果。显式手动重试开启新的有限尝试预算。独立分镜任务沿用现有最多三次队列尝试策略。

超时无法证明上游没有生成或计费，有限重试只减少重复请求，不能保证 Provider 端只计费一次。
