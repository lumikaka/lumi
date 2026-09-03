# Automatic Generation Workflow API

本 Contract 说明首页创建会话的 bootstrap 内部创建边界；在对话式初始化中，该请求由运行时生成，Agent 不得调用。普通 ready 对话和普通 UI 入口不能借用该授权。

## `POST /api/v1/projects/{project_uuid}/workflows`

在已确认的 Setup 上启动自动生成流程，并把当前 Chat Run 持久挂起至 Workflow 终态。运行时从 Setup 中已定稿的 `generation_brief` 生成 `story_prompt`。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `story_prompt` | body | 非空字符串，最长 4000 字符 | 是 | 基于 `original_input`、后续回答和已展示建议整理的故事 Brief，不得覆盖原始输入。 |
| `model` | body | 字符串，最长 512 字符 | 否 | 可选文本模型覆盖；省略时使用当前可信配置。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Workflow 公开 UUID。 |
| `data.thread_uuid` | UUIDv7 字符串 | 来源 conversation Thread UUID。 |
| `data.presentation_mode` | 字符串 | Agent 创建时固定为 `inline`。 |
| `data.kind` | 字符串 | 固定的自动生成 Workflow 类型。 |
| `data.title` | 字符串 | 从项目事实取得的 Workflow 标题。 |
| `data.status` | 字符串 | Workflow 状态；最终恢复时为成功、失败或取消终态。 |
| `data.current_step_key` | 字符串，可省略 | 当前或最后执行的步骤 key；尚无当前步骤时省略。 |
| `data.steps` | 数组 | 按 `position` 排序的公开步骤摘要。 |
| `data.steps[].uuid` | UUIDv7 字符串 | Workflow Step UUID。 |
| `data.steps[].step_key` | 字符串 | 稳定步骤 key。 |
| `data.steps[].position` | 整数 | 步骤顺序。 |
| `data.steps[].status` | 字符串 | 步骤状态。 |
| `data.steps[].progress` | 整数 | 进度值。 |
| `data.steps[].resource_uuid` | UUIDv7 字符串，可省略 | 步骤产出资源后出现的公开 UUID。 |
| `data.steps[].error_code` | 字符串，可省略 | 步骤失败时出现的安全错误码。 |

### request_api 示例

以下仅记录运行时内部形状，Agent 不得照此调用。

```json
{
  "method": "POST",
  "url": "/api/v1/projects/<project_uuid>/workflows",
  "request_body": {
    "story_prompt": "一只小狐狸穿过云海灯塔群，为迷路的月亮送回星光。"
  },
  "response_filter": ".data | {uuid,thread_uuid,presentation_mode,kind,title,status,current_step_key,steps}"
}
```

### 接口约束

- Agent 只读取本 Contract 以理解边界，不得自行调用该 route、拼装 `story_prompt` 或在确认后重新 GET Setup。仅运行时在 Project Setup finalization 已由用户确认并成功、且项目事实为 `ready` 时生成该请求。
- `request_body` 不接受 `title`、`provider_uuid`、`idempotency_key`、Chapter 数、Section 数或其他未列字段；这些值由服务端固定或解析。
- 响应过滤器先用于创建意图校验；提交成功后当前 Tool 进入持久 `waiting_for_workflow`，不会先向模型返回“已启动”结果。
- Workflow 复用当前 conversation Thread、Turn、Run 和 Tool Execution，以 `presentation_mode=inline` 展示在来源 Turn；不得创建或切换到独立 Thread。
- 服务端按可信 `creation_session_uuid` 绑定幂等键，运行时 Intent 同时绑定来源 Turn lineage 和精确 confirmation request UUID。重复回答、恢复、队列重投或应用重启只恢复同一 Workflow 和 `workflow_awaits`，不创建第二个 Workflow、await、Thread 或首步 Job。升级前已有独立 Workflow 保持原归属，不在恢复时迁移。
- 自动生成流程固定完成项目初始化、故事、Story Profile、Premise、正文页规划和初始图片，只创建或复用 `vol01.ch01`。它生成 1～6 个 `body` Section；普通绘本另有 `front_cover`，并默认生成封面与首个正文页成品图；`vertical_strip` 不建封面，只生成首个画面段落成品图；Premise Setting Image 仍是既有步骤。
- 创建提交后进入异步任务并释放 Agent worker；不得轮询状态、读取进度或手工模拟步骤。成功、失败或取消后，运行时用终态 Tool Result 恢复同一 Run，再输出一次最终答复；失败只提供安全错误码，后续使用既有 Workflow retry，不得创建第二个 Workflow。
- 首页或其他 Direct UI 直接调用公开 REST 时仍创建 dedicated Workflow Thread，且不建立 Chat await；该行为不适用于本 Agent Contract。
