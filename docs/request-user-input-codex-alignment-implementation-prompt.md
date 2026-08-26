# 实现与 Codex 对齐的 `request_user_input`

你正在 Lumi 项目中工作。请基于当前仓库的真实实现，把 Agent 的 `request_user_input` 从“单个单选/多选题”升级为与当前 Codex 模型侧交互契约一致的“单次 1–3 个短问题、每题互斥单选、客户端自动提供 Other”的可恢复交互。

这是一项完整实施任务，不是只修改 Tool JSON Schema，也不是只重做一张前端卡片。协议快照、SQLite 事实、危险操作确认、暂停/恢复、REST、WebSocket 失效、历史展示和旧 Run 恢复必须一起闭环。

## 0. 使用 Goal 推进

本任务要求使用环境提供的 Goal 功能跟踪跨阶段实施。这里的 Goal 是 Codex 执行环境的长期目标能力，不是要求在 Lumi 中新增 Goal 表、Goal API 或 Goal UI。

1. 开始时先调用 `get_goal`。
2. 如果不存在活动 Goal，调用 `create_goal` 创建目标；用户没有指定 token budget，因此不要设置 `token_budget`。
3. 如果已有 Goal 正好覆盖本任务，继续该 Goal；如果存在无关且未完成的 Goal，不要覆盖或伪造完成状态，应明确报告冲突。
4. 同时使用普通计划拆分实施阶段，并保持最多一个阶段为 `in_progress`。Goal 保存长期目标，计划保存当前执行步骤，两者不要混用。
5. 每阶段完成后更新计划，运行该阶段相关测试，并记录仍未覆盖的真实缺口。
6. 只有全部验收项完成、相关测试通过且没有必需工作遗留时，才调用 `update_goal(status="complete")`。
7. 不要因为任务较大、上下文压缩、运行时间较长或预算接近结束而提前完成 Goal；只有陷入真实且反复出现的同一阻塞条件时才可按环境规则标记 `blocked`。

推荐 Goal objective：

```text
为 Lumi 实现与 Codex 模型侧契约对齐的 request_user_input：支持单次 1–3 个互斥单选问题、推荐项和客户端 Other，保持 UUIDv7、危险确认绑定、SQLite 持久化、同 Run 恢复、历史兼容和 WebSocket 驱动的 REST 校准。
```

## 1. 修改前必须确认的基线

先检查工作区状态和下列文件。仓库可能已有用户未提交的改动；必须阅读 `git status` 和相关 `git diff`，保留并兼容这些改动，不得覆盖、回退或格式化无关文件。

- `internal/agent/protocol_registry.go`：当前活动 Tool protocol 与允许的四个工具。
- `internal/agent/runtime.go`：Tool definition 装配、冻结协议恢复、Tool loop 和等待输入边界。
- `internal/agent/tools.go`：活动 Tool schema、参数校验、Tool intent 和用户输入请求创建。
- `internal/agent/legacy_tool_definitions.go`：旧协议恢复专用 schema。
- `internal/agent/interactions.go`：用户输入列表、回答校验、Tool result 持久化、恢复 Job 和取消。
- `internal/agent/dangerous_route_confirmation.go`：危险 Route 的请求指纹与确认选项绑定。
- `internal/agent/types.go`：公开 DTO 和回答输入。
- `internal/agent/context.go`、`internal/agent/service.go`：Prompt snapshot 与新 Run 协议冻结。
- `internal/agent/prompts/base.zh-Hans.md`、`internal/agent/prompts/base.en.md`：模型使用工具的稳定规则。
- `internal/httpapi/agent.go`、`internal/server/server.go`：现有 REST 入口和统一响应信封。
- `db/migrations/project/`、`internal/dbmigrate/migrate_test.go`：项目数据库表与迁移测试。
- `web/src/components/ChatArea.jsx`：待回答卡片与历史卡片。
- `web/src/pages/chatAreaPresentation.js`：回答投影和兼容解析。
- `web/src/api/chat.js`：用户输入 REST 调用。
- `web/src/realtime/`、`web/src/pages/chatWorkspaceState.js`：WebSocket 变更提示和 Query 失效。
- `web/src/i18n/messages/chat.js`、`web/src/styles/chat.sass` 及对应测试。
- `docs/prds/chat_threads/features/对话工具与多模态交互.md`、`docs/prds/chat_threads/data_model.md`。
- `docs/prds/ai_runtime/` 中描述活动 Tool protocol 和冻结恢复的文档。

必须以这些现状为前提：

- 当前新 Run 使用 `project_api_v3`；已冻结的 v2/v3 Run 必须按原协议恢复，不能静默套用新 schema。
- 当前 `request_user_input` 一次只保存一个 `question`，支持 `single_choice|multiple_choice` 和 2–8 个选项。
- 当前请求会持久化到 `chat_user_input_requests`，把 Run/Turn 置为 `waiting_for_input`，回答后写入同一 Tool call 的 `tool_result` 并排队恢复同一个 Run。
- 当前 `request_user_input` 必须是一次模型响应中唯一的 Tool Call；这一约束继续保留。
- 当前危险 Route 依赖 route、project UUID、target UUID、expected revision、request fingerprint 和确认选项索引的完整绑定；不能为了外形对齐而削弱。
- 客户端状态不是事实源。SQLite/REST 是事实源，WebSocket 只发送变化提示并触发 TanStack Query 失效，不得增加定时 HTTP 轮询。

## 2. 对齐目标与明确边界

### 2.1 本任务对齐的 Codex 契约

模型可调用的 `request_user_input` 使用以下语义：

- 顶层参数是 `questions`。
- 每次 1–3 个问题；优先只问 1 个，只有确实相关时才组合多个问题。
- 每个问题包含短 `header`、稳定的 snake_case `id`、单句 `question` 和 2–3 个 `options`。
- 每题选项互斥，不再向新协议暴露 `input_type` 或多选语义。
- 每个选项包含短 `label` 和一句 `description`。
- 推荐项放在第一位，label 以精确的 ` (Recommended)` 结尾。
- 模型不得自己创建 Other 选项；客户端自动提供自由输入的 Other。
- 回答以 question id 为 key 映射回模型。

Codex App Server 的回答形状是：

```json
{
  "answers": {
    "question_id": {
      "answers": ["one selected label or free-form answer"]
    }
  }
}
```

新 Lumi Tool result 应采用这一模型可见形状。由于新问题均为互斥单选，每个 question 的 `answers` 必须恰好包含一个字符串。

### 2.2 有意保留的 Lumi 差异

以下差异是架构或安全边界，不属于待消除的产品偏差：

- 不在本任务中为 Lumi 发明 Codex 的 Plan/Default collaboration mode。Lumi 的危险业务操作会在执行阶段使用该工具，若简单限制为 Plan mode 会破坏现有确认链路。
- `confirmation` 继续作为 Lumi 的受控扩展，但必须适配 question id 和新单选结构。
- 浏览器提交选项时继续使用服务端生成的公开 UUIDv7，不信任客户端回传 label；服务端校验 UUID 后再生成 Codex 形状的模型可见回答。
- Codex 宿主传输层的 `threadId`、`turnId`、`itemId` 等由 Lumi 自己的持久实体承担，不应复制到模型 Tool 参数。
- 不在本任务中接入 Codex App Server 的实验性 secret input 或自动超时解析能力。除非执行环境在实施时向模型实际暴露了这些字段并且用户扩大范围，否则不要加入 `is_secret`、`auto_resolution_ms` 或后台超时自动选择。

`question.id` 是模型生成的逻辑映射 key，不是数据库 bigint 主键。数据库主键、外键和 JOIN 仍只使用内部 `id`；所有资源实体、请求和选项对外仍使用 UUIDv7，绝不泄漏内部 `id`。

## 3. 新活动 Tool schema

为新 Run 提供等价于以下结构的严格 JSON Schema；所有 object 都必须 `additionalProperties: false`：

```json
{
  "questions": [
    {
      "header": "画面风格",
      "id": "art_style",
      "question": "这次画面应采用哪种整体风格？",
      "options": [
        {
          "label": "温暖手绘 (Recommended)",
          "description": "延续绘本现有的柔和质感和亲切氛围。"
        },
        {
          "label": "电影写实",
          "description": "强化真实光影、景深和镜头感。"
        }
      ]
    }
  ]
}
```

运行时必须二次校验，不能只依赖 Provider 遵守 JSON Schema：

- `questions`：1–3 项。
- `header`：trim 后非空，最多 12 个 Unicode 字符。
- `id`：trim 后匹配 `^[a-z][a-z0-9_]{0,63}$`，同一请求内唯一。
- `question`：trim 后非空，最多 4000 个 Unicode 字符。
- `options`：每题 2–3 项。
- `label`：trim 后非空，设置合理硬上限；Tool description 明确要求面向用户的 1–5 个词。不要使用只适用于英文空格分词的脆弱校验来拒绝中文标签。
- `description`：必填、trim 后非空、单句、设置合理硬上限。
- 第一项 label 必须以精确的 ` (Recommended)` 结尾；其他项不得带该后缀。
- 模型参数中不得出现额外的 Other 选项；前端统一渲染自由输入入口。
- 任何未知字段、重复 JSON key、空字符串、重复 question id、错误数量或非法 confirmation 都返回现有可修复的 Tool validation error。

如果底层 Provider 的 JSON Schema 能表达 `minItems`、`maxItems`、`pattern`、`minLength` 和 `maxLength`，应同时声明；Go 运行时校验仍是最终安全边界。

## 4. 回答契约

### 4.1 浏览器到 REST

REST 回答必须使用选项 UUID，而不是可伪造的 label。推荐请求体：

```json
{
  "answers": {
    "art_style": {
      "selected_option_uuid": "019c...",
      "other_text": ""
    },
    "page_count": {
      "selected_option_uuid": "",
      "other_text": "12 页"
    }
  }
}
```

要求：

- `answers` 必须且只能覆盖请求中的全部 question id。
- 每题必须且只能提供一个合法 `selected_option_uuid` 或一个 trim 后非空的 `other_text`。
- `selected_option_uuid` 必须是该问题所属选项的公开 UUIDv7；不能选择另一个问题的选项。
- Other 最多 4000 个 Unicode 字符。
- 拒绝未知 question id、缺失回答、重复映射、同时选择选项和 Other、空 Other、未知 UUID 和额外字段。
- 不从 label 反查选项，不接受数组下标作为浏览器回答身份。

### 4.2 REST/SQLite 到模型 Tool result

服务端校验浏览器回答后，生成并持久化 Codex 形状的规范 Tool result：

```json
{
  "answers": {
    "art_style": {
      "answers": ["温暖手绘 (Recommended)"]
    },
    "page_count": {
      "answers": ["12 页"]
    }
  }
}
```

- 选择预设项时，由服务端从冻结请求中取 label。
- 选择 Other 时，使用用户提交并校验后的文本。
- 每个数组在新协议中恰好一个字符串。
- Tool result 继续属于原 `request_user_input` Tool call，不新增虚构的公共 Item 层级。
- 恢复模型请求必须收到这一持久化结果；重试、重启或重复投递不得重新解释浏览器临时状态。

公开 UserInput DTO 可以额外返回安全的展示投影，例如已选 option UUID/label，但模型可见 `tool_result` 必须保持上述形状。不要把内部 bigint、数据库行信息、文件路径或 confirmation 指纹之外的敏感 metadata 暴露给浏览器。

## 5. 危险操作确认适配

保留顶层可选 `confirmation` 扩展，并增加 question 绑定：

```json
{
  "questions": [
    {
      "header": "确认删除",
      "id": "confirm_delete",
      "question": "是否将该资源移入回收站？",
      "options": [
        {
          "label": "取消操作 (Recommended)",
          "description": "保留当前资源，不执行任何更改。"
        },
        {
          "label": "确认删除",
          "description": "使用当前 revision 将指定资源移入回收站。"
        }
      ]
    }
  ],
  "confirmation": {
    "route": "...",
    "project_uuid": "...",
    "target_uuid": "...",
    "expected_revision": 3,
    "request_fingerprint": "sha256:...",
    "question_id": "confirm_delete",
    "confirm_option": 1
  }
}
```

危险确认必须满足：

- 整个 Tool call 只有一个 question。
- question 为新协议的互斥单选题。
- `question_id` 精确匹配该 question 的 `id`。
- `confirm_option` 仍为冻结 options 中的零基索引且必须有效。
- 第一项推荐安全动作；实际确认项不能是第一项。
- route、project UUID、target UUID、expected revision 和 request fingerprint 继续逐项绑定并使用现有验证逻辑。
- 只有回答中选中的 option UUID 对应 `confirm_option` 时，后续危险 `request_api` 才能消费确认。
- Other 文本、取消项、缺失项、另一个 question 的选项或过期 fingerprint 都不能授权危险操作。
- 危险请求绝不自动超时确认。

更新中英文 Base Prompt，明确普通问题的 Codex 结构，以及危险操作应把安全取消项放在第一位并绑定实际确认项。

## 6. Tool protocol 版本与旧 Run 恢复

这是破坏性 Tool schema 变更，必须升级冻结协议，不得原地改变 `project_api_v3` 的含义。

建议：

```text
活动协议：project_api_v4
恢复协议：project_api_v3、project_api_v2、legacy_typed_tools
```

要求：

- 新 Thread、Turn、正常 Follow-up、Steering 和新 Run 只冻结 `project_api_v4`。
- v4 暴露新的 `questions` schema。
- v3 恢复原来的 `input_type/question/options` schema和 v3 `image_gen` 参数。
- v2 同时恢复原来的用户输入 schema 和 v2 `image_gen` 参数。
- legacy typed Run 继续使用它自己的冻结定义。
- `toolDefinitionsForProtocol` 必须按明确协议选择定义，不能通过“当前定义再替换一个字段”的方式意外让旧协议继承未来变更。
- `loadRunToolMode`、Tool context 恢复、Prompt snapshot 判断、测试夹具和文档都要接受并正确区分 v2/v3/v4。
- queued Follow-up、Retry、Restart、Steering 和 user-input Resume 继续使用来源 Run 冻结的 snapshot，不在恢复中升级协议。
- 不重写历史 `agent_tool_executions.arguments_json`、`chat_items` 或旧 Tool result。
- 已经处于 pending 的 v2/v3 用户输入必须仍能由旧 UI/兼容投影回答并恢复；不能因应用升级而卡死。

为旧协议建立清晰命名的冻结定义和测试。不要继续让名为 `legacyRecoveryToolDefinitions` 的一个大集合同时含混承担所有 v2/v3/v4 差异。

## 7. 持久化与迁移

新请求需要持久化整个问题集合，而不是只保存第一个问题。

推荐把用户输入记录演进为版本化的规范请求载荷，例如：

```text
schema_version
request_json
response_json
```

具体迁移形态可按现有 SQLite/GORM 约束决定，但必须满足：

- 新记录只有一个权威请求载荷，避免 `question/options_json/questions_json` 多份可变事实互相漂移。
- 请求载荷保存 question id、header、question、按原顺序排列的 options，以及服务端生成的 option UUIDv7。
- response 保存可恢复、可审计的规范回答。
- schema version 能区分旧单题协议和新的 Codex questions 协议。
- 旧行完整保留并可投影；原 `multiple_choice` 历史答案不得丢失或被伪装为新单选答案。
- `UNIQUE(run_id, tool_call_uuid)`、Thread/Run/Turn/Item 内部 FK、状态约束和索引继续成立。
- JSON 使用数据库约束和 Go 校验保证有效。
- Up migration 必须迁移真实旧数据，不依赖应用启动时猜测。
- Down migration 不得静默删除、截断多问题请求或伪造单题数据；如果无法无损降级，应采用项目现有规范提供明确且可测试的安全失败边界。
- 对外 DTO 只含公开 UUIDv7 和安全字段。

新增或重建项目迁移时更新迁移顺序测试、已有项目升级测试和新项目 schema 测试。不要修改已经发布迁移的内容来假装历史从未存在。

## 8. Runtime 生命周期

新协议仍复用当前可靠的暂停/恢复机制：

```text
模型调用 request_user_input
  → 持久化 Tool intent
  → 持久化 user_input_request
  → Run/Turn = waiting_for_input
  → WebSocket 发送变化提示
  → 用户通过 REST 提交完整 answers
  → 原 Tool execution = completed
  → 持久化原 Tool call 的 tool_result
  → Run/Turn 排队
  → Job 恢复同一个 Run
  → 模型读取 Codex 形状 answers
```

要求：

- `request_user_input` 继续必须是该模型响应中唯一的 Tool Call。
- 一次 Tool call 创建一条请求记录和一个可见 `user_input_request` Item；其中可以包含 1–3 个 questions，不为每题创建独立 Tool call 或独立 Run。
- Tool call idempotency、Provider call id、重复投递、进程崩溃恢复和事务边界继续有效。
- 回答只允许 `pending → resuming → resumed`；重复提交按现有幂等/冲突语义处理，不能生成第二个 Tool result 或第二个 Resume Job。
- 取消仍取消整个等待中的 Turn，并把请求标记为 `cancelled`。
- Reconcile 必须识别新旧 schema 的 pending/resuming 请求。
- 状态变化后通过已有 `chat:user_input_requested|answered|cancelled` 等事件提示客户端；payload 只含公开 UUIDv7、状态和定位信息。
- 不增加定时 HTTP 轮询。

## 9. REST 契约

保持现有资源化路径，除非真实代码证明必须增加子资源：

```text
GET  /api/v1/projects/:project_uuid/chat_threads/:thread_uuid/user_input_requests
POST /api/v1/projects/:project_uuid/chat_threads/:thread_uuid/user_input_requests/:request_uuid/responses
POST /api/v1/projects/:project_uuid/chat_threads/:thread_uuid/user_input_requests/:request_uuid/cancellations
```

响应继续遵守统一信封：

```json
{ "success": true, "data": { "items": [] } }
```

或单对象：

```json
{ "success": true, "data": { "uuid": "..." } }
```

失败继续使用：

```json
{
  "success": false,
  "data": null,
  "error": {
    "code": "...",
    "message": "...",
    "details": "..."
  }
}
```

所有新增 JSON 字段使用 `snake_case`。不要在 URL、DTO、事件或 Tool result 中暴露内部 bigint。

## 10. ChatArea 交互

把 pending 卡片改成单次请求内的 1–3 个问题：

- 按原顺序显示每个 question。
- `header` 是短分组标题，`question` 是完整问题。
- 每题 2–3 个 radio 选项，不能使用 checkbox。
- 第一项清楚显示推荐状态；可以把精确后缀转换为本地化 badge，但不得改变提交映射或历史事实。
- 每题自动显示一个自由输入 Other；Other 不是服务端 option，不生成伪 option UUID。
- 同一题选择预设 option 时清空 Other；输入 Other 时清除 radio。
- 不同问题彼此独立。
- 所有问题都获得有效回答后才允许提交。
- 一次提交整个 answers map；提交期间禁用所有控件，防止重复提交。
- 保留“取消本轮”。
- 错误后保留本地尚未提交的选择，允许修正并重试。

历史卡片必须：

- 显示全部问题及各自最终答案，而不是把多个答案拼成无法归属的一行。
- 能渲染 v4 选项回答和 Other 回答。
- 能继续渲染 v2/v3 的单选、多选和 Other 历史。
- cancelled/incomplete 请求显示清楚，不伪造回答。
- 刷新、首次 join、重新 join、WebSocket 重连和窗口重新聚焦后通过 REST 恢复事实状态。

可访问性要求：

- 每题使用语义化 `fieldset`/`legend` 或等价的可访问分组。
- radio name 在 request 内按 question id 隔离。
- label、描述、推荐状态、错误与提交状态可被辅助技术读取。
- 键盘可完成选择、Other 输入、提交和取消。
- 不能只用颜色表达推荐或已选状态。
- 为带 `selected`、`active`、`aria-pressed` 等状态的按钮/选项显式编写组合 hover 状态，并放在基础状态之后，避免同等优先级规则覆盖 hover 反馈。
- 保持 ChatArea 窄栏可用；长问题、长描述和 3 题组合不能横向溢出。

## 11. Prompt 与文档同步

更新中英文 Base Prompt，使模型明确：

- 只有关键选择、真实缺失信息或危险确认才调用 `request_user_input`。
- 能安全合理推断时继续执行，不要为了次要偏好频繁打断用户。
- 每次优先 1 题，最多 3 题。
- 每题 2–3 个互斥选项。
- 第一项是推荐项并以 ` (Recommended)` 结尾。
- 不创建 Other，客户端会自动提供。
- Tool call 必须单独出现。
- 危险确认复制完整 binding，安全取消项第一，实际确认项由 `question_id` 和 `confirm_option` 绑定。

同步更新：

- Chat Threads 的 Feature PRD 与 data model。
- AI Runtime 对活动 `project_api_v4` 和历史 v2/v3 恢复的描述。
- 任何声明 `project_api_v3` 为新 Run 当前协议的文档。
- 与用户输入工具、危险操作确认和 Trajectory Tool 生命周期有关的实现文档。

不要批量改写历史架构文档中作为时间点记录的 v2/v3 结论；只在当前事实文档中更新，必要时明确其历史性质。

## 12. 自动化测试

至少覆盖以下确定性测试。

### 12.1 Tool schema 与参数校验

- 新活动协议名称和四工具顺序正确。
- v4 `request_user_input` 只接受 `questions` 和可选 confirmation。
- 接受 1、2、3 个合法问题。
- 拒绝 0 或超过 3 个问题。
- 拒绝空/过长 header、非法或重复 id、空 question。
- 拒绝少于 2 或超过 3 个 options。
- 拒绝缺少 label/description、错误推荐项位置/后缀和额外字段。
- 拒绝旧 `input_type/question/options` 形状进入 v4。
- v3/v2 恢复仍接受旧形状并拒绝 v4 形状。

### 12.2 回答验证

- 多问题全部使用预设项。
- 多问题混合预设项与 Other。
- 每题 exactly one 回答。
- 拒绝缺失/未知 question id、跨题 option UUID、未知 UUID、同时 option+Other、空 Other 和超长 Other。
- Tool result 精确生成 `answers[question_id].answers[0]`。
- 浏览器不能通过伪造 label 授权或改变模型结果。

### 12.3 危险确认

- 只接受单问题、新单选结构、匹配 question id 和合法 confirm index。
- 安全推荐项在 index 0，实际确认项不是 index 0。
- 选择实际确认项后可消费一次确认。
- 选择取消、Other、错误问题、错误 option、过期 revision 或错误 fingerprint 均不能授权。
- 重放确认不能二次执行危险 Route。

### 12.4 生命周期与兼容

- v4 请求暂停并恢复同一个 Run。
- 一次多问题请求只生成一个 Tool call、请求记录和 Tool result。
- 重复回答不生成第二个 Resume Job。
- Abort/Cancel、Restart、Reconcile 和 Provider call id 恢复正确。
- 已冻结 v3/v2 Run 能列出、回答、恢复并继续使用旧 Tool result 形状。
- 数据库 Up migration 保留旧单选、多选、Other、pending 和已回答记录。
- 新建项目得到最新 schema；已有项目升级后数据完整。

### 12.5 REST、前端与实时同步

- API 路径、统一信封、snake_case 和 UUIDv7 约束正确。
- presentation projector 同时覆盖 v4 和 legacy 请求。
- 1–3 题 radio、自动 Other、完整性校验和一次性提交行为正确。
- 历史卡片能按题显示 v4 回答并继续显示 legacy 多选。
- 推荐项、键盘语义、可访问名称和窄栏布局有回归覆盖。
- WebSocket 事件只触发目标查询失效；无定时 HTTP 轮询。
- selected/hover 组合状态不会被源码顺序覆盖。

## 13. 验证命令

按修改范围先运行聚焦测试，再运行完整验证：

```bash
go test ./internal/agent ./internal/httpapi ./internal/dbmigrate
pnpm --dir web test
go test ./...
pnpm --dir web build
```

如果仓库已有更精确的测试命令，先运行它们再运行以上完整验证。

不要在本地运行 Cargo 或任何 Rust 编译、检查、测试命令。读取 `codex --version`、`codex features list` 或使用 Codex 自带的只读 schema 生成命令可以用于核对基线，但不能把临时生成物提交到仓库。

## 14. 推荐实施顺序

1. 冻结 v3/v2 Tool definitions，并建立 v4 protocol 常量与恢复矩阵。
2. 定义 v4 request/response 领域结构和纯校验函数，先写单元测试。
3. 完成数据库迁移和 legacy/v4 DTO 投影。
4. 接入创建请求、回答验证、规范 Tool result、Resume 和 Reconcile。
5. 适配危险 confirmation，并完成安全回归测试。
6. 更新 REST DTO 和 handler 测试。
7. 更新 ChatArea、presentation、i18n、样式和前端测试。
8. 更新 Base Prompt、PRD 和当前事实文档。
9. 运行完整测试与 build，检查 `git diff`，确认没有无关改动。
10. 所有验收项满足后才完成 Goal。

## 15. 完成定义

只有同时满足以下条件，任务才算完成：

- 新 Run 冻结新协议并向模型暴露 Codex 风格 `questions` schema。
- 一次请求可持久化、显示、提交并恢复 1–3 个互斥单选问题。
- 推荐项和自动 Other 的行为符合契约。
- 浏览器使用 option UUIDv7 提交，模型收到 question-id keyed 的 Codex 形状答案。
- 危险 Route 的确认强度不低于当前实现。
- v2/v3 pending 与历史请求仍可读取、回答和恢复。
- SQLite/REST 仍是事实源，实时同步没有引入 HTTP 轮询。
- API 信封、snake_case、内部 bigint/外部 UUIDv7 规则全部满足。
- 中英文 Prompt、PRD 和当前架构文档已同步。
- 相关 Go 测试、完整前端测试和前端 build 通过。
- 未运行 Cargo/Rust，本地临时 Codex schema 未提交。
- 最终交付说明列出协议版本、迁移、主要文件、验证命令与结果、保留差异和任何真实剩余风险。
- Goal 已在所有工作实际完成后标记为 `complete`。
