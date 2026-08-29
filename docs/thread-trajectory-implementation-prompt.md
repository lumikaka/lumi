# 实现 Lumi Thread–Turn–Item Trajectory Inspector

你正在 Lumi 项目中工作。请基于当前仓库的真实数据模型，实现一个独立、只读、可通过 URL 打开的 Thread 执行轨迹查看器。

这不是 Chat Transcript 的另一种皮肤，也不是对 ChatArea 的重构。它是从持久化事实数据构建的独立 execution projection。

## 0. 使用 Goal 推进

本任务允许并要求使用环境提供的 Goal 功能跟踪跨阶段实施：

1. 开始时先读取当前 Goal；不存在活动 Goal 时创建一个目标明确的 Goal，不要设置虚构的 token budget。
2. 使用普通工作计划拆分阶段，并保持最多一个阶段处于 `in_progress`。
3. 每一阶段完成后更新计划、运行该阶段相关测试并记录尚未覆盖的真实缺口。
4. 只有所有验收项都已完成、测试已通过且没有必需工作遗留时，才把 Goal 标记为 `complete`。
5. 不要因为任务较大、上下文压缩或预算接近结束而提前完成 Goal。

推荐 Goal objective：

```text
为 Lumi 实现独立 URL 的只读 Thread–Turn–Item Trajectory Inspector，提供稳定投影、模型请求检查、工具生命周期、历史浏览、搜索和时间概览，并从 ChatArea 的 Thread 项在新标签页打开。
```

## 1. 修改前必须理解的现状

先阅读并定位，不要凭通用 agent harness 经验假设 Lumi 已有能力：

- `internal/agent/types.go`：Thread、Turn、Run、Item、Event DTO 与 record。
- `internal/agent/service.go`：Turn/Item/Event 写入和 cursor 历史读取。
- `internal/agent/runtime.go`：一次 Turn 内的多次模型请求和工具循环。
- `internal/agent/tools.go`：Tool intent、execution、result、用户输入工具。
- `internal/agent/context.go`：prompt snapshot、上下文组装和 `agent_context_summaries`。
- `internal/llmlog/`：LLM 请求与响应日志。
- `internal/httpapi/agent.go`、`internal/httpapi/llm_logs.go`：现有 REST API。
- `web/src/components/ChatArea.jsx`：Thread 列表、Thread 详情和历史 prepend。
- `web/src/realtime/`：WebSocket 变更提示触发 TanStack Query 失效的现有机制。
- `web/src/pages/StoryWorkspacePage.jsx`：项目内路由注册。

必须以以下事实为实现前提：

- Lumi 已有持久化 Thread、Turn、Run、Item、Event、Tool Execution、Context Summary 和 LLM Log。
- `chat_items.uuid`、`chat_events.uuid`、`agent_tool_executions.uuid`、`agent_context_summaries.uuid`、`llm_logs.uuid` 都是公开 UUIDv7。
- `chat_items.sequence` 与 `chat_events.sequence` 分别是 Thread 内稳定序列，但它们目前不是同一个序列空间。
- 当前 Chat Agent 使用非流式 `Complete`；没有 assistant token chunk、首 token 时间或持久化 reasoning。
- `llm_logs` 已保存真实 LLM request/response snapshot、provider/model、attempt、usage、总耗时和诊断。
- 当前没有 subagent/nested tool 的父子关系数据。
- 当前没有通用 view/plugin 注册机制；项目页面使用 React Router 显式注册。
- ChatArea 桌面宽度约 360px，不适合承载完整 Ledger + Inspector + Timeline。

## 2. 职责边界

### 2.1 Trajectory 是只读投影

正确关系：

```text
Thread / Turn / Item / Event / Tool Execution / Context Summary
                               +
                   LLM request/response logs
                               ↓
                    Trajectory projection
                               ↓
              Timeline / Ledger / Inspector
```

禁止：

```text
Trajectory → 修改 ChatArea selection
Trajectory → 改写 Thread/Turn/Item 历史
Trajectory → 为缺失数据伪造 backend event
Trajectory → 根据 ChatArea 渲染后的 DOM 反推数据
Trajectory → 用数组 index 作为长期 identity
```

### 2.2 `llm_logs` 的职责保持不变

`llm_logs` 只负责记录 LLM 接口的一次请求及其返回或失败：

- request snapshot
- response snapshot
- provider/model/options
- attempt/status
- usage
- duration
- provider diagnostics

不要把 `llm_logs` 改造成通用 Trajectory Item 表或 Thread Event 表。

不要向 `llm_logs` 写入普通 user Item、Tool 生命周期、compaction 或 UI 状态。

不要把完整 LLM request/response 再复制到新的 trajectory 表。Trajectory 的 Model Request inspector 应通过 `llm_logs.uuid` 读取现有 LLM Log detail。

允许其他事实记录通过内部 bigint FK 或公开 UUID 投影关联 `llm_logs`，但关联不能改变 `llm_logs` 的上述职责。

## 3. 统一术语

Trajectory 公共层级只使用：

```text
Thread → Turn → Item
```

- Thread：完整项目 Agent 对话线程。
- Turn：一次用户驱动的 Agent 回合。
- Item：Turn 内可观察的执行单元。
- Model Request：Turn 内的边界/metadata，不增加第四层级。

现有 `chat_runs` 可以继续作为内部执行和恢复实体，不要为了 Trajectory 重构或删除它；Trajectory UI 不把 Run 渲染成公共层级。

旧 `step_count/max_steps` 仅作物理兼容，运行时使用 `model_request_count` 生成 ordinal；Trajectory UI 不使用 “Step” 指代模型请求，应显示 `Request #N`。

## 4. 独立 URL 与 ChatArea 入口

### 4.1 URL

新增独立页面：

```text
/projects/:projectUuid/threads/:threadUuid/trajectory
```

要求：

- 页面组件命名可按项目惯例，例如 `ThreadTrajectoryPage`。
- 该路由仍经过现有 Project activation/provider gate。
- 页面使用项目工作区的完整内容宽度；打开该路由时隐藏 ChatArea，避免在 Trajectory 页面再次出现聊天侧栏。
- 刷新或直接粘贴 URL 后仍能从 REST 恢复全部当前状态。
- 选择的 Inspector Item 写入 URL：

```text
?item_uuid=<public_uuidv7>
```

- 直接打开带 `item_uuid` 的 URL 时，加载包含该 Item 的历史页、定位 Ledger row 并打开 Inspector。
- URL selection 只属于 Trajectory，不影响 ChatArea 的 `chat_thread_uuid` 或其他 workspace 查询参数。

### 4.2 ChatArea 按钮

在 ChatArea 的 Thread 列表中，为每个 Thread 增加独立的“查看轨迹”按钮/链接：

```jsx
target="_blank"
rel="noopener noreferrer"
```

要求：

- 必须生成真实 `href`，不要只依赖 `window.open`。
- 点击 Thread 主体仍执行现有的打开 Chat 交互；轨迹按钮必须阻止该交互冒泡。
- 链接只使用公开 `projectUuid` 和 `thread.uuid`。
- 按钮具有可本地化的可访问名称和 tooltip。
- 不要把完整 Trajectory 组件嵌入 360px ChatArea。

## 5. 数据来源与专用 REST 投影

不要让前端通过项目级 LLM Logs 列表扫描并猜测某个 Thread 的模型请求。

新增 Thread 下的只读子资源，路径按项目 REST 风格实现，例如：

```text
GET /api/v1/projects/:project_uuid/chat_threads/:thread_uuid/trajectory?before=&after=&limit=
```

响应遵守统一信封：

```json
{
  "success": true,
  "data": {
    "thread": {},
    "turns": [],
    "items": [],
    "model_requests": [],
    "cursor_pagination": {},
    "history_complete": false,
    "overview": {}
  }
}
```

具体 DTO 可按实现调整，但必须满足：

- 外部只暴露 UUIDv7，不暴露内部 `id`。
- 字段使用 `snake_case`。
- Item/Tool/Model Request/Compaction 的关联由服务端批量 JOIN 或批量查询完成，禁止逐 Item N+1 查询。
- 初次请求默认读取 tail；支持 cursor prepend older history。
- `history_complete` 明确告诉 UI 搜索和统计是否覆盖完整历史。
- 列表只返回 Ledger/Timeline 所需的安全摘要和结构化 metadata。
- 大型 raw payload、完整 system prompt、tool catalog、LLM response 通过现有 LLM Log detail 按需读取。
- 应用相同或更严格的 diagnostic sanitization，不能泄漏内部 bigint ID、磁盘路径、API key、Authorization、Cookie 或 secret。

可以在 `internal/agent` 增加独立 trajectory query/projection service，但不要让它修改 runtime 状态。

## 6. Model Request 的真实关联和排序

当前 `llm_logs.uuid` 就是稳定的 Model Request identity。不要再创造另一个重复 request UUID。

一次 Turn 内多次请求使用：

```text
request_uuid = llm_logs.uuid
request_ordinal = llm_logs.attempt
```

但不能只靠 timestamp 猜测 Model Request 与 Assistant/Tool Item 的对应关系。

为新数据补充真实关联，推荐使用现有 `chat_events` 记录轻量事件：

```text
model_request_started
model_request_completed
```

事件 payload 只包含公开 UUIDv7、ordinal、status 等安全字段，例如：

```json
{
  "project_uuid": "...",
  "thread_uuid": "...",
  "turn_uuid": "...",
  "run_uuid": "...",
  "request_uuid": "...",
  "request_ordinal": 2,
  "status": "completed"
}
```

要求：

- 事件使用 `chat_events.sequence` 提供 Thread 内稳定的执行顺序。
- Tool intent、Tool result、Assistant final Item 必须能显式关联产生它们的 `request_uuid`；具体使用内部 FK 还是安全 metadata 按现有规范选择。
- 内部 FK 使用 bigint，Web/API 投影使用 UUIDv7。
- retry 产生新的 `llm_logs.uuid`，不得复用旧请求 identity。
- 对历史旧数据，只有在关系可确定时才关联；无法确定时标记 `legacy_unlinked` 或 `ordering_accuracy = approximate`，不要伪造精确关系。

这些 Thread events 只记录执行关联与顺序；完整请求和响应仍只保存在 `llm_logs`。

## 7. Trajectory projection 模型

前端创建独立、稳定的 projection，不直接把任意后端对象塞进 JSX：

```ts
type TrajectoryItemKind =
  | "system"
  | "user"
  | "context"
  | "assistant"
  | "tool"
  | "compaction"
  | "error";

interface TrajectoryItem {
  id: string;
  sourceUuid: string;
  threadUuid: string;
  turnUuid: string | null;
  seq: number | null;
  kind: TrajectoryItemKind;

  requestUuid?: string;
  requestOrdinal?: number;
  callUuid?: string;
  parentItemUuid?: string;

  status: "pending" | "running" | "completed" | "error" | "interrupted";
  startedAt?: number;
  completedAt?: number;
  durationMs?: number;

  preview: string;
  input?: unknown;
  output?: unknown;
  source?: unknown;
}
```

字段按 JavaScript 项目实际 conventions 调整，不要求引入 TypeScript。

Projection 至少支持：

```text
replace(full or page snapshot)
apply(upserts)
prepend(older page)
```

identity 优先级：

1. 原生公开 UUIDv7
2. Tool lifecycle 使用公开 `tool_call_uuid`
3. Model Request/System 使用 `llm_logs.uuid`
4. Compaction 使用 `agent_context_summaries.uuid`
5. 只有没有原生 identity 的折叠 summary row 才使用稳定派生 key

禁止使用数组 index。

如果不同 source kind 共享同一个 UUID，React key 使用稳定的 `<source_kind>:<uuid>`，但不得因此制造随分页变化的 identity。

## 8. 按 Lumi 真实能力投影 Item

### User

- `chat_items.item_type = user_message`。
- 正常用户消息开启其已有 Turn。
- `metadata.steering = true` 时保留在当前 Turn，并明确标记 Steering。
- Follow-up 只有实际 promoted/steered 成 Item 后才进入执行轨迹；queued follow-up 不是已执行 Item。

### Assistant

- 已持久化 `assistant_message` 直接投影。
- 产生 Tool Call 的中间 Assistant 响应可以从对应 LLM Log response 投影为稳定 Assistant Item，identity 使用 request UUID。
- 当前 runtime 没有持久化 reasoning；Inspector 显示 `Not recorded`，不要把普通 content 当 reasoning。
- 当前 runtime 没有 token streaming；不要为每个伪 chunk 建 Item。

### Tool

- 把同一 `tool_call_uuid` 的 call、execution、result 合并成一个 Tool Item 生命周期。
- 参数来自安全的 Tool Call Item；结果来自 Tool Result/Execution。
- schema 来自产生该调用的 LLM Log request tool catalog。
- `result.success = false` 或 execution/error 事实存在时，状态必须显示 error，不能仅因为 Tool Result Item 已写入而显示 completed。
- Turn 已 terminal 且 Tool 仍为 intent/executing/in_progress 时，只在 Trajectory projection 中派生 `interrupted`，并显示派生原因；不要回写或重写历史 Item。
- `request_user_input` 保持一个 Tool 生命周期，用户回答作为它的 result；不要额外虚构公共层级。

### Nested Tool/Subagent

当前 Lumi 没有持久化父子工具或 subagent 数据。

- 不要根据工具名、调用时间或 JSON 嵌套猜测 Subtool。
- Projection 可以保留可空 `parent_item_uuid` 以便未来扩展。
- 没有真实父子数据时，UI 不显示虚构的 SUBTOOL。
- 若本任务确实新增父子执行能力，必须使用内部 bigint FK、外部 UUIDv7，并加入 cycle/depth 防护；不要为了 UI 演示添加假数据。

### Context 与 Error

- `context_summary` 或未来插件/memory/goal/system injection 只有在真实持久记录存在时投影为 Context。
- `error` Item 明确投影为 Error，不要丢进普通 Assistant content。
- Turn 终态错误同时显示在 Turn header 和相关 Error Item。

### Compaction

当前事实来源是 `agent_context_summaries`：

- 使用 summary UUID 作为 identity。
- 展示 `through_item_sequence`、`source_bytes`、summary、created_at。
- 当前没有 running/completed 生命周期和 duration，因此显示 `Not recorded`，不要根据 UI 当前时间计算。
- 当前记录不能可靠证明属于哪个 Turn 时，`turn_uuid = null`，放入 Thread-level/Between turns 区域并注明归属未记录。
- 如新增 `compaction_created` Thread Event，只记录 summary UUID、request UUID、through sequence 等关联；不要写入 `llm_logs`。

### System Prompt / Tool Catalog

- 真实输入来自 LLM Log request snapshot，不从 ChatArea 或当前 prompt catalog 反推历史请求。
- 对连续 Model Requests 计算 system prompt 和 tool catalog 的稳定 digest。
- 只有 digest 真实变化时产生/显示 SYSTEM change Item。
- Inspector 提供 Diff、System Prompt、Tools。
- 初始值没有 previous 时不显示虚构 diff。
- `chat_items` 公共 metadata 已有意移除内部 prompt snapshot；不要绕过这个安全边界直接暴露 DB metadata。

## 9. Usage 与 Timing 的现实边界

Model Request Inspector 展示当前真实可用字段：

```text
provider
model
status
request ordinal / attempt
finish reason
input tokens
cached input tokens
output tokens
total duration
created/completed time
provider diagnostics
```

要求：

- `llm_logs` 记录的总耗时可以展示。
- 当前 Chat Agent 没有 first-token timing，因此 TTFT、generation duration 显示 `Not recorded`。
- 当前没有 reasoning tokens、cache write tokens，显示 `Not recorded`。
- 数据库中的历史 `0` 无法区分“真实 0”和“Provider 未记录”时，UI 显示 `Not recorded` 或明确的 legacy note，不能把它计入精确统计。
- Pending Request 没有 completed time 时显示 Pending，不根据 `Date.now()` 伪造 duration。
- 本任务不顺带把非流式 Chat Agent 重构为流式客户端；如果仓库之后已有真实 first-token 数据，Trajectory 再渐进展示。

## 10. Ledger 与 Inspector 页面

页面主结构：

```text
Toolbar
Overview Timeline
Turn / Request Ledger        Resizable Inspector
```

Ledger 保持高密度三列语义：

```text
#       EVENT          CONTENT
```

至少视觉区分：SYSTEM、USER、CONTEXT、ASSISTANT、TOOL、COMPACTION、ERROR；颜色不能是唯一语义。

要求：

- Turn 边界明显强于普通 Item 边界，可 sticky。
- Request boundary 显示 `Request #N`，不是 Step。
- preview 单行 ellipsis，完整内容放 Inspector。
- Tool preview 为 `toolName · compact arguments`，可附 compact result。
- 选择 Item 更新 `item_uuid` URL，并打开右侧 Inspector。
- Inspector 可 resize；宽度、tab、collapse、search、timeline range 都是 Trajectory-local state。
- Inspector 页签按 kind 提供 Summary、Raw/Source、Payload/Result/Schema/Timing、System Diff/Prompt/Tools、Compaction Summary 等。
- Raw JSON 使用安全的 machine-value 展示，不执行 HTML。

## 11. Search、Collapse、History 与 Virtualization

### Search

至少索引已经加载的：

- Turn UUID/number
- Item kind/status/preview
- user input
- assistant output
- tool arguments/result/schema/call UUID
- source/error
- system prompt/tool catalog
- compaction summary

规则：case-insensitive、空格分词、AND semantics。

如果 `history_complete = false`，必须在 UI 明示“仅搜索已加载历史”；不要声称覆盖完整 Thread，也不要为了搜索静默拉取全部大型 payload。

LLM Log detail 按需加载后，只增量更新对应 request 的 search document；不要重建全索引。

### Collapse

- Turn collapse 与 Assistant-following Tool group collapse 独立。
- 折叠 summary 使用稳定派生 key。
- SYSTEM/Request boundary 可以隐藏在当前视觉折叠中，但状态必须可恢复，不能从 projection 删除。

### History

- 初次打开 tail。
- 接近顶部时加载 earlier page。
- prepend 后保持 scroll anchor，不发生明显 jump。
- 用户位于 tail 时，新结构 Item 才自动 follow。
- 用户向上滚动后暂停 tail following。

### Virtualization

当前 Web 项目没有已安装的 virtualizer。先确认依赖策略；如需新增，选择轻量且支持动态高度、stable key 和 prepend anchor 的方案。

要求只 mount visible window + overscan；逻辑 ARIA row index 不因虚拟化改变。

## 12. Overview Timeline

Timeline 最后实现，至少三条 lane：

```text
lane 0: system / user / context / error
lane 1: assistant / model request / compaction
lane 2: tool
```

支持：sequence、duration、time、actual。

现实约束：

- 只有真实 duration 才画 span。
- 没有 duration 的 running/pending Item 只画 start marker。
- 当前 Assistant 没有 TTFT/generation 数据，不画伪造分段。
- Tool duration 优先使用 execution started/completed；缺失时显示 marker/Not recorded。
- Timeline overview 如覆盖完整 Thread，应由专用 REST DTO 返回紧凑 metadata，不能要求 Ledger 先加载全部 raw content。

交互：hover、click 定位 Item、wheel zoom、zoom 后 pan、range focus、clear focus。

Timeline range、Search、Collapse 三种状态互相独立，最终过滤顺序必须有单元测试。

## 13. Realtime

继续使用项目统一的：

```text
/api/v1/ws
topic/event/payload/ref/join_ref
```

- realtime payload 只包含公开 UUIDv7 和必要状态，不包含内部 ID 或 raw payload。
- `chat:*` 以及新增的 Model Request/Compaction Thread 变更提示使 `chat-trajectory` Query 失效。
- `llm_log:changed` 仍负责 LLM Log 列表/detail 的失效；不要让它承载完整 trajectory payload。
- TanStack Query 失效后通过 REST 重读 SQLite 事实状态。
- 首次 join、重新 join、窗口重新聚焦继续使用现有项目级校准。
- 不增加定时 HTTP 轮询。

## 14. 推荐职责边界

按项目 JavaScript/Go conventions 调整文件名，职责建议如下：

```text
internal/agent/
  trajectory.go             # read-only query/projection DTO

internal/httpapi/
  agent.go                  # trajectory REST handler/route

web/src/pages/trajectory/
  trajectoryProjector.js
  trajectoryIdentity.js
  trajectorySearch.js
  trajectoryTimeline.js
  trajectoryVirtualRows.js
  ThreadTrajectoryPage.jsx
  TrajectoryToolbar.jsx
  TrajectoryLedger.jsx
  TrajectoryInspector.jsx
  TrajectoryTimeline.jsx
```

目录只是建议。不要为匹配目录而破坏当前项目 conventions，也不要重构 ChatArea。

## 15. 实施阶段

### Phase 1：事实关联和 API

- 为新 Model Request 写入真实 Thread request lifecycle event。
- 建立 request 与产生的 Assistant/Tool 的显式关联。
- 批量读取 Thread/Turn/Item/Event/Tool Execution/Context Summary/LLM Log summaries。
- 完成 trajectory REST DTO、cursor、sanitization、legacy incomplete 标记和后端测试。

### Phase 2：Projection tests

- raw DTO → stable Trajectory Item。
- Turn grouping、Steering、Tool merge、error/interrupted、request boundary、system change、compaction unassigned。
- identity 在 upsert/prepend 后不变。

### Phase 3：独立页面与入口

- 注册独立 URL，并在该路由隐藏 ChatArea。
- Thread 列表 `_blank` 轨迹链接。
- Ledger、selection URL、Inspector、resize。

### Phase 4：Search 与 Collapse

- 增量 search document。
- incomplete-history 提示。
- Turn/Tool group collapse。

### Phase 5：History 与 Virtualization

- tail initial load、older prepend、anchor preservation、tail follow suspend。
- long Thread virtualization 与 ARIA。

### Phase 6：Timeline

- sequence/duration/time/actual。
- zoom/pan/range focus/Item navigation。
- 缺失 timing 不伪造 span。

每阶段运行：

- `go test` 的相关 package
- Web `node --test`/相关测试
- `pnpm build` 或项目已有的相关构建检查

遵守仓库约束：不要运行 Cargo 或 Rust 编译、检查和测试。

## 16. 最低测试覆盖

后端/API：

- Thread scope/project ownership
- 外部响应无内部 ID/path/secret
- cursor tail/prepend
- request started/completed event 与 `llm_logs.uuid` 关联
- retry 使用新 request UUID 和正确 ordinal
- bulk query 不出现逐 Item N+1
- legacy log payload/timing 缺失
- context summary 投影但不虚构 Turn
- interrupted Tool 派生状态

Projection：

- user opens existing Turn
- steering stays in current Turn
- completed assistant
- request-only/tool-call-only assistant
- tool call/result merge and stable call identity
- tool result `success=false` → error
- terminal Turn + unfinished Tool → interrupted
- system prompt/tool catalog digest change
- compaction summary with `turnUuid=null`
- prepend/upsert identity stability
- unknown usage/timing stays unknown

UI：

- ChatArea Thread trajectory link has correct href, `_blank`, `noopener noreferrer`
- trajectory direct URL refresh
- `item_uuid` deep link selects and scrolls to Item
- Thread boundary/collapse
- Inspector tabs and resize
- search AND semantics and incomplete-history notice
- prepend scroll anchor and tail-follow suspension
- virtualization stable row key/ARIA index
- timeline modes, unknown duration and range focus
- selected/active/`aria-pressed` buttons retain explicit combined hover rules after base state styles

## 17. 最终验收

在不伪造缺失数据的前提下，用户从 ChatArea Thread 列表的新标签页打开 Trajectory 后，可以直接回答：

1. Thread 有多少已知 Turn，当前历史是否完整？
2. 每个 Turn 发生了哪些 Item？
3. 进行了哪些真实 Model Request，哪些是 retry？
4. 每个请求使用的 provider/model/options 是什么？
5. 有记录时消耗多少 input/cache-read/output token？
6. 每个请求真实总耗时是多少？
7. TTFT/reasoning 等未记录信息是否明确显示 Not recorded？
8. 调用了哪些 Tool，输入、输出和 schema 是什么？
9. Tool 是否失败、仍在运行或因 Turn 结束而中断？
10. system prompt/tool catalog 是否发生真实变化？
11. 是否存在 context summary/compaction，能否区分已知与未记录归属？
12. 哪些有真实 duration 的操作最耗时？
13. 当前仍有哪些 Request/Tool/Turn 处于未终态？
14. 搜索是否明确区分“已加载历史”和“完整 Thread”？
15. URL 是否可以分享并直接恢复选中的 Inspector Item？

以下不属于本任务的虚假验收项：

- 在当前非流式 runtime 中声称拥有 TTFT 或 token chunk。
- 在没有 reasoning 数据时展示模型 reasoning。
- 在没有父子执行关系时展示 Subtool/Subagent。
- 把历史未知 usage 当作精确的 0。
- 把 `llm_logs` 扩张成通用轨迹数据库。

如果某项底层事实不存在，正确实现是显示 `Not recorded` / `Not available` / `Legacy unlinked`，并保留后续扩展接口，而不是猜测或伪造。
