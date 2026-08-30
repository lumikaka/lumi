---
git_commit_message: 'feat(project): 对话定稿后启动受控 YOLO'
plan_state: finished
---

# 对话式项目创建接入 YOLO 实施计划

> 兼容更新（2026-08-30）：本文记录的是首次接入时的历史实施边界，其中 bootstrap YOLO 的 `dedicated_thread`、立即结束 Turn 和不建立 await 的描述已被后续实现取代。当前事实以 Workflow / Chat Thread PRD 为准：升级后新建的 bootstrap YOLO 在原 conversation Thread 的来源 Turn 内以 `inline` 展示，持久等待终态并恢复同一 Run；Direct UI 与既有 dedicated YOLO 保持原行为。

## current_status

- 首页一句话创建已经把原始输入写入 draft 项目的普通 conversation Thread，并由首个 Turn 维护 Project Setup；`project_creation_bootstraps` 已用 `creation_session_uuid`、`thread_id` 和 `turn_id` 唯一标记这次 bootstrap。
- draft 阶段已有执行层门禁，只允许项目只读、Project Setup 更新/定稿和当前对话；定稿后项目立即变为 `ready`，首个 Run 随即重新获得全部 Project API，因此模型可以绕过固定编排连续创建 `vol01.ch02`、`vol01.ch03` 等资源。
- 当前初始化 Guide 在定稿后只要求“再继续故事、生产或导出工作”，没有规定必须启动 YOLO、不得手工生产以及启动成功后的停止条件。
- 现有 `yolo_project_initialization` 已实现项目就绪检查、输入/Prompt/模型冻结、固定六步调度、异步任务等待、步骤幂等、取消、重试、应用重启恢复和终态停止。它只创建或复用 `vol01.ch01`，生成 Story、Story Profile、Premise、1～6 个 Comic Sections，并只为第一个 Section 生成漫画成品图。
- `POST /api/v1/projects/:project_uuid/workflows` 已供首页直接启动 YOLO，但还没有适合 Agent 使用的 reviewed route：Agent 无法获得服务端绑定的创建会话幂等键，也没有首个 bootstrap Turn 专用的生产权限边界。
- 本计划中的“一张图片”指唯一拥有 `current image` 的第一个 Comic Section；YOLO 为 Premise 生成的 Setting Image 属于既有设定流程，不改变。若产品要求整个项目数据库只能出现一个图片文件，需要另行缩减 YOLO Premise 步骤，不属于本计划。

## overview

- 不新增 `project_initial_preview`、不复制 YOLO 实现，也不改变手动创建和首页现有 YOLO 入口。对话式创建只在 YOLO 前增加一段受控 Briefing：Agent 从 `original_input` 和 Project Setup 推断项目设定与故事方向，只有缺失或冲突会明显影响首章结果时才通过 `request_user_input` 询问。
- Agent 在启动前展示 Project Setup、YOLO Brief、字段来源和准确产出范围。用户选择“定稿并启动 YOLO”后，运行时先按现有 fingerprint-bound 协议执行 Setup finalization，再由同一 bootstrap Run 创建现有 `yolo_project_initialization` Workflow。
- YOLO 使用独立的 dedicated Workflow Thread，沿用现有进度、诊断、取消与重试 UI。创建成功后 bootstrap Turn 只回复“已启动”和 Workflow 链接并立即完成，不等待六步全部结束，也不再调用其他生产 API。
- 首个 bootstrap Turn 在项目变为 `ready` 后仍受特殊门禁：除只读请求和受审的 YOLO 创建 route 外，禁止 Chapter、Story、Premise、Section、图片、导出及维护写操作，也禁止直接调用 `image_gen`。后续新 Turn 不继承该限制，保持普通 ready 项目能力。
- YOLO 的幂等键由服务端根据可信 `creation_session_uuid` 生成；模型不能提交或改变幂等键。相同首个 Turn 重放、工具恢复、应用重启或模型重复调用都只能得到同一个 Workflow。

## product_flow

1. 首页一句话创建 draft 项目并进入 bootstrap conversation，现有 Saga 与导航保持不变。
2. Agent 读取初始化 Guide、Project Setup Contract 和当前 Setup，保留 `original_input` 原文，整理唯一的 Setup Draft 及 YOLO Brief 草稿。
3. 信息足够时不提问；信息不足时一次提出 1～3 个相互关联的问题。问题只覆盖会显著影响首章的主角/世界、核心目标或冲突、目标年龄/基调/结局方向，以及尚未确定的绘本结构字段。
4. 原则上只问一轮；只有用户答案发生冲突或仍无法形成合法绘本规格时才追加一轮。可以合理补全的剧情细节作为 `agent_proposed` 展示，不把流程变成长问卷。
5. Agent PATCH 完整的 Setup Draft 并展示最终摘要。确认问题使用稳定 `question_id=confirm_setup_and_start_yolo`，安全推荐项为“继续修改”，确认项明确写为“定稿并启动 YOLO”，并继续绑定 Setup finalization 的 route、revision 与 request fingerprint。
6. 用户确认后，运行时自动完成 Setup finalization。Agent 重新 GET 验证 `setup_status=ready`，将原始输入、用户补充和明确标注的 Agent 建议整理为不超过现有 YOLO 上限的 `story_prompt`。
7. Agent 通过 reviewed `POST /api/v1/projects/{project_uuid}/workflows` 启动 YOLO；title 使用定稿后的项目名，Provider/模型继续由现有项目模型解析，幂等键由服务端注入。
8. 创建成功后当前 Turn 返回 dedicated Workflow 引用并停止。YOLO 后端固定运行 `project_initialization → story → story_profile → premise → comic_sections → first_section_image`；不得由聊天 Agent逐步模拟这些步骤。
9. 用户未选择确认项、通过 Other 表示只想定稿、取消确认或要求继续修改时，不启动 YOLO。若需要“只定稿”，继续使用现有 Setup finalization 确认流程，并在 ready 后结束 Turn。

## agent_contract

### YOLO Brief 生成规则

- `original_input` 始终作为用户原始需求保留；Workflow `story_prompt` 可以把原文、后续回答和已展示的 Agent 建议整理成结构化 Brief，但不得回写或覆盖原文。
- 优先从原文确定主角、背景、目标、冲突、情绪基调、目标读者和结局方向。原文已经能形成连贯首章时不要追问。
- “帮我做一本儿童绘本”一类缺少故事核心的输入可以询问；“一只怕水的小狐狸为了救朋友渡河，温暖水彩”一类输入可直接形成 Setup Draft 并进入摘要。
- 不询问 YOLO 能安全决定的镜头细节、Section 标题、具体分镜数量或模型参数；不要求用户填写完整创作问卷。
- Setup 中的语言、画风、绘本形式、比例和形式专属字段仍以 Project Setup 为事实；Brief 不创建第二份结构配置。

### 启动和停止规则

- draft 阶段不得启动 YOLO，现有 `project_setup_incomplete` 门禁继续生效。
- bootstrap 首个 Turn 只有在同一 Run 已消费 `confirm_setup_and_start_yolo` 的确认答案、Setup finalization 成功且当前事实为 `ready` 时才能调用 Agent YOLO route。
- Agent YOLO route 创建 dedicated Workflow 后立即返回公开 `workflow_uuid`、`thread_uuid`、kind、status 和步骤摘要；Agent 不读取或轮询 Workflow 状态。
- YOLO 创建失败时报告真实安全错误并停止，不得退化为手工创建 Chapter、Section 或图片。
- YOLO 创建成功后，即使模型继续请求写操作，执行层也必须拒绝；相同 YOLO route 的恢复或重放返回同一个 Workflow。

## backend

### Bootstrap 上下文和执行门禁

- 扩展 `internal/agent/tools.go` 的进程内 `toolContext`，增加不对外序列化的 bootstrap 标记和 `creation_session_uuid`。`internal/agent/runtime.go:loadToolContext` 通过当前 `turn.id` 查询 `project_creation_bootstraps`；仅最初的 bootstrap Turn 命中，后续同 Thread Turn 不受限制。
- 在 `internal/agent/request_api_tool.go` 增加 ready bootstrap policy：GET 保持可用；非 GET 只允许 Project Setup finalization 的持久恢复和新的 YOLO 创建 route。其余 reviewed 或 discovered 写 route 均返回稳定错误 `bootstrap_production_requires_yolo`。
- 在 `internal/agent/image_tool.go` 或统一 Tool 执行入口增加相同检查，防止 ready 后绕过 `request_api` 直接使用 `image_gen`。
- 门禁只约束 Agent bootstrap Turn，不修改 HTTP Direct UI、Workflow Step 或后台 Worker。YOLO 内部通过 Service/Queue 写入，不经过聊天 Tool 门禁。

### Reviewed YOLO route

- 在 Agent API registry 增加 `RouteYoloWorkflowCreate`，覆盖现有 `POST /api/v1/projects/{project_uuid}/workflows` server route，使用严格 schema；Agent 请求只接受 `story_prompt`，可选接受现有允许的文本模型覆盖，但不接受 `title`、`provider_uuid`、`idempotency_key`、Chapter 数量或 Section 数量。
- route handler 从定稿 Project 读取 title，从当前 Run/项目设置解析模型，并从可信 bootstrap context 生成 `project-creation-yolo:<creation_session_uuid>`。调用现有 `CreateYoloWorkflow`，不得复制六步创建代码。
- route 在执行前验证：当前是 bootstrap 首个 Turn、Setup 已 ready、当前 Run 存在已成功消费且绑定 Setup finalization 的 `confirm_setup_and_start_yolo` 用户答案。缺失事实时返回稳定的 `bootstrap_yolo_not_authorized`。
- 为 Workflow 增加紧凑 Agent projector 和推荐 filter，只暴露公开 UUIDv7、kind、title、status、current_step_key、presentation_mode、thread_uuid 和必要步骤状态；不得暴露内部 ID、路径、冻结 Prompt、Provider 密钥或完整诊断 payload。
- `CreateYoloWorkflow` 继续创建 dedicated Thread，并继续使用现有 `WorkflowYolo`、`YoloStepKeys`、输入快照和恢复逻辑。仅把调用参数构造与 Agent route 接通，不引入新的 Workflow kind 或 preset。

### 幂等与恢复

- 相同 `creation_session_uuid` 始终映射到同一 YOLO idempotency key；Workflow 唯一查询继续以 `(project_id, kind, idempotency_key)` 为事实。
- 若进程在 Workflow 提交后、Tool Result 持久化前退出，恢复同一 Tool Execution 时必须读取并返回已有 Workflow，不创建第二个 Thread、Workflow 或首个 Step Job。
- 若 Setup 已 finalization、但 YOLO 在提交前因图片比例/模型预检失败，项目保持 ready，Agent 返回错误；修复配置后可从显式用户请求使用同一创建会话键再次启动。
- 若 YOLO 已创建后失败或中断，只使用既有 Workflow retry，不重新 POST 生成第二个 YOLO。
- 不给 `project_creation_sessions` 增加跨数据库 Workflow 外键；恢复关系以项目库的 bootstrap 记录和 Workflow 幂等键为事实，避免新增跨库 Saga 阶段。

## agent_docs

- 更新 `internal/agent/docs/guides/初始化新项目.md`：把定稿后的开放式“继续生产”替换为本计划的 Briefing、确认、YOLO route、失败处理与停止协议。
- 新增或扩展 Workflow/YOLO API Contract，记录 Agent 专用请求 schema、紧凑响应、ready 前置条件、dedicated 展示、幂等由服务端绑定以及禁止 HTTP 轮询。
- 更新 `internal/agent/agent_docs.go` 能力索引，使 `setup_status=draft` 或 bootstrap 原始输入命中初始化 Guide；普通 ready Thread 只有用户明确要求 YOLO 时才读取 YOLO Contract。
- 在中英文基础 Prompt 中仅保留跨能力的不变量：bootstrap 首个 Turn 不得手工生产、成功启动 Workflow 后必须停止。问题选择与字段细节留在 Guide，避免扩大每轮 system prompt。

## api

- 现有公开 HTTP API 路径不变：`POST /api/v1/projects/:project_uuid/workflows` 继续创建 `yolo_project_initialization`，现有首页调用体与行为不回归。
- Agent reviewed overlay 使用更窄的内部 schema；请求和响应仍遵循统一 snake_case 信封。Agent Tool 只能引用当前 Project UUID，不能传入其他项目或内部 ID。
- 增加稳定 Agent 错误码：
  - `bootstrap_production_requires_yolo`：首次创建 Turn 尝试绕过 YOLO 直接写生产资源。
  - `bootstrap_yolo_not_authorized`：没有当前 Run 的明确确认事实，或不属于 bootstrap 首个 Turn。
- YOLO route 属于受控异步写操作；Setup finalization 仍是唯一需要 fingerprint-bound 危险确认的结构性操作。确认问题必须明确同时告知“定稿后将启动 YOLO”，从而避免第二次重复确认。

## ui

- 不新增 Initial Preview 页面或 Workflow 类型。现有 Workflow Thread、进度、步骤、诊断、取消和重试组件继续展示 YOLO。
- 扩展项目引用协议支持 `@project/workflows/{workflow_uuid}`。后端成功创建 Agent YOLO 时返回该 `ui_ref`；前端解析为当前项目 URL 的 `workflow_uuid` 查询参数，由现有 ChatArea 选择对应 dedicated Thread 并滚动到 Workflow 卡片。
- bootstrap Turn 最终消息第一次提及 YOLO 时使用 Tool Result 提供的 `ui_ref`，不自行拼接 URL。用户可以立即打开 Workflow，也可以留在原对话；不强制页面跳转。
- `workflow:queued`、`workflow:step_changed` 和终态事件继续只触发 TanStack Query 失效，Thread/Workflow 事实通过 REST 重读；不增加 HTTP 轮询。
- 如新增确认或 Workflow 链接按钮样式，显式覆盖 selected/active/`aria-pressed=true`:hover 组合状态并放在基础状态之后。

## tests

### Go

- Agent Guide/Prompt 注册测试：draft bootstrap 上下文能发现初始化 Guide；Guide 包含最少提问、`confirm_setup_and_start_yolo`、YOLO route、禁止手工生产和启动后停止规则。
- bootstrap context 测试：只有 `project_creation_bootstraps.turn_id` 对应的首个 Turn 带创建会话元数据；同 Thread 的第二个 Turn 恢复普通 ready 能力。
- 门禁表驱动测试：ready bootstrap Turn 拒绝 `chapter.create`、章节批量规划、Premise/Section/图片/导出写 route 和 `image_gen`；GET 可用；YOLO route 在满足授权时可用。
- 授权测试：未确认、选择安全项、Other、取消、错误 question id、未成功 finalization、其他 Run 和普通 Thread 均不能启动 bootstrap YOLO；成功消费绑定 finalization 的确认后可以启动。
- reviewed route schema 测试：模型不能传 `idempotency_key`、title、Provider、章节数或 Section 数；响应 projector 不包含内部 ID、路径、Prompt 快照和密钥。
- 幂等测试：同一创建会话重复 Tool Call、恢复同一 Tool Execution、应用重启重放和不同 `story_prompt` 重放都返回同一个 Workflow UUID、Thread UUID 和唯一首步 Job。
- 失败测试：图片比例或模型预检失败不创建 Workflow，且不得回退手工生产；Workflow 已失败时再次创建返回原 Workflow，重试继续走现有 retry API。
- YOLO 端到端测试继续断言：fresh project 最终只有一个 active Chapter 且 code 为 `vol01.ch01`；Section 数在既有 1～6 范围内；只有第一个 Section 有 current comic image；不存在 `vol01.ch02`；六步均终态完成。
- HTTP 回归测试：现有首页/Direct UI 的 `POST /workflows` 请求体、HTTP 201、统一信封、dedicated presentation、取消和重试保持不变。

### Web

- 项目引用测试覆盖合法/非法 `@project/workflows/{workflow_uuid}`、UUIDv7 大小写和路径穿越，并验证解析后保留当前项目已有 query 参数。
- ChatArea 测试验证 Agent 回复中的 Workflow 引用能选择对应 dedicated Thread，workflow query/event 只触发相关 Query 失效，不产生 `refetchInterval`。
- 创建流程回归：详细输入可以直接进入摘要；信息不足时显示最少问题；用户继续修改时不启动；确认后只出现一个 YOLO Workflow；手动创建和首页原 YOLO tab 不受影响。

## acceptance

- 详细首页输入无需额外问卷即可展示 Setup + YOLO Brief 摘要；模糊输入只询问会显著改变首章的少量问题。
- 用户没有明确选择“定稿并启动 YOLO”时，项目中没有 YOLO Workflow、Chapter、Comic Section 或生成任务。
- 用户确认后只创建一个 `yolo_project_initialization` Workflow；bootstrap Agent 自身不创建任何 Chapter、Section 或图片资源。
- Workflow 成功后 fresh project 只有 `vol01.ch01`，不存在 `vol01.ch02`；只有第一个 Comic Section 具有漫画 current image，其他 Section 不生成漫画图。
- 模型即使尝试连续创建 `vol01.ch02` 到 `vol01.ch06`，执行层也全部拒绝，数据库事实不增长，当前 Turn 不进入手工生产循环。
- Workflow 创建成功后 bootstrap Turn 完成，用户能通过回复中的安全项目引用打开 dedicated YOLO Thread，并使用现有进度、取消、诊断和重试能力。
- 刷新、断网重连、应用退出重启和 Tool Result 丢失均不会产生第二个项目、bootstrap Thread、YOLO Workflow 或 Chapter。

## docs_and_verification

- 更新 `docs/prds/projects/features/对话式项目创建与设置定稿.md`：把“未得到明确意图时不自动启动 YOLO”细化为 YOLO 前置 Briefing、明确确认和 bootstrap 首 Turn 门禁。
- 更新 `docs/prds/workflows/features/YOLO项目初始化.md` 与 `docs/prds/workflows/features/可恢复多步工作流.md`：记录对话式创建入口、服务端幂等来源、dedicated presentation 和不等待终态的行为。
- 检查 `docs/prds/projects/overview.md`、`docs/prds/workflows/overview.md`、Chat Thread/AI Runtime PRD，只同步受本功能影响的生命周期、授权和恢复事实。
- 完成后执行：
  - `gofmt` 覆盖修改的 Go 文件。
  - `go test ./internal/agent ./internal/httpapi ./internal/jobqueue ./internal/projectcreation`。
  - `go test ./...`。
  - `pnpm --dir web test`。
  - `pnpm --dir web build`。
- 遵守仓库约束，不在本地运行 Cargo 或 Rust 编译、检查及测试命令。

## non_goals

- 不新增 `project_initial_preview`、YOLO preset 或第二套 Workflow executor。
- 不改变现有 YOLO 的六个步骤、1～6 个 Comic Sections、Premise Setting Image 或首图策略。
- 不让聊天 Agent 手工模拟 Story、Premise、Section 和图片生成步骤。
- 不给应用库创建会话增加 Workflow 外键，不引入跨数据库事务。
- 不自动重命名项目目录，不改变正式绘本规格不可修改的约束。
- 不在用户未明确确认时启动 YOLO，不使用 HTTP 定时轮询同步 Workflow 状态。
