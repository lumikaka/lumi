# Agent Project API 当前架构（`project_api_v2`）

日期：2026-08-21  
步骤二基线：`86abf15f16694d72b0ae2e920ace064af364c20c`

## 结论

四个 Scene 的新 Run 统一使用 `project_api_v2` / `project_api_tools`，共享同一组四个工具，并可调用服务端当前注册的全部项目 API Route。原有 74 条显式 Agent Route 继续作为经过审查的安全与性能覆盖层。Scene 不再是工具或文档权限容器，只提供运行时身份、项目/Subject 事实、安全边界、图片参考策略和 Guide 推荐。

`request_api` 不访问 Lumi 的 localhost 或任意外部 URL。可用 Route 从 Echo 的 `/api/v1/projects/:project_uuid/...` 注册结果自动装配：已审查 Route 优先使用领域服务分发；其余 Route 或超出旧覆盖层 schema 的合法参数通过进程内 Echo handler 执行。

## 运行时分层

| 层面 | 权威职责 |
| --- | --- |
| Tool Protocol | 新 Run 固定快照 `project_api_v2` / `project_api_tools`；更早 Project API 协议不恢复 |
| SceneDefinition | `Key`、`Scopes`、Prompt key、`RecommendedGuideIDs`、Subject 约束、`ImageReferencePolicy` |
| Tool Set | 所有有效 Scene 按固定顺序获得 `request_api`、`read_agent_doc`、`image_gen`、`request_user_input` |
| Guide Registry | 声明 capability ID、说明、所需工具、上下文/输入前提和 Guide 路径 |
| Echo Project Routes | 项目 API 可用范围的唯一事实来源；新增或删除 Route 会同步改变 `request_api` 可用范围 |
| Agent API Overlay | 为已审查 Route 声明稳定 route ID、query/body schema、领域 handler、projector、risk、revision、异步和确认策略 |
| Domain Service | 校验项目/资源归属、业务状态、SQLite 事实、revision 和文件来源 |
| Prompt Catalog | Base Prompt 提供稳定规则；Scene Prompt 提供权威上下文并动态注入推荐 Guide 路径 |

新 Thread、Turn、steering 和正常 follow-up 都创建 v2 snapshot。queued follow-up 复用来源 Turn 的冻结 snapshot；restart 和 user-input resume 读取持久化 snapshot。已完成历史仍可查看，但更早 Project API Run 和旧文档工具名不会被恢复或映射。

## Scene 与 Guide

| Scene | Subject | 自动图片参考策略 | 推荐 Guide |
| --- | --- | --- | --- |
| `project_assistant` | 无 | `none` | 创建设定项、维护设定项、更新 Storyboard |
| `premise_asset_generation` | 无 | `message_attachments` | 创建设定项 |
| `asset_reference` | Premise Asset | `bound_asset_first` | 创建设定项、维护设定项 |
| `storyboard_reference` | Comic Section | `none` | 更新 Storyboard |

四个 Scene 都可调用全部四个工具；自动附件/绑定图策略不构成工具权限。

## Agent 文档体系

嵌入范围只有：

- `internal/agent/api-docs/*.md`：Overview 与领域 API Contract。
- `internal/agent/api-docs/guides/*.md`：可复用能力流程。

`/api/v1/agent-docs/overview.md` 包含能力索引和领域 API Contract 文档索引，不展开具体 Route。能力索引固定列为 `capability_id`、说明、所需工具、上下文/输入前提、Guide 路径；文档索引指向 `chapter.md`、`comic*.md`、`generation.md`、`premise*.md`、`project*.md`、`story.md`、`storyboard.md` 和 `task.md`。Runtime 在创建 Turn 时渲染完整 Overview，将其写入冻结 Prompt snapshot，并原样追加到每次模型请求的 system prompt；模型据此选择并读取目标领域 Contract，具体 method/path 只进入需要它的调用上下文。

三份 Guide：

- `premise-asset-create.md`：文本、当前消息附件、已有项目图片参考、ready upload 四种来源，以及创建图片的 `512x512` 默认要求。
- `premise-asset-maintain.md`：最新 revision、元数据 PATCH、图片替换、派生资产和显式软删除。
- `storyboard-update.md`：完整 Storyboard 读取、全量 Markdown 替换、revision 冲突和窄化响应。

每份 Guide 都包含标准步骤、禁止捷径、前置条件、失败恢复和对应 API Contract。文档读取只接受注册路径；Scene 文档、任意文件、Query、Fragment 与路径穿越不在注册表中。

## Premise Asset 文件来源契约

创建时 `file_uuid` 与 `upload_uuid` 必须且只能提供一个：

- `file_uuid`：当前会话 `image_gen` 新返回、用途与当前上下文匹配且尚未消费的文件。
- `upload_uuid`：当前项目 ready 且尚未消费的上传。

已有设定项的 `current_variant.asset.uuid`、当前消息附件或其他项目文件不能直接充当 `file_uuid`；已有项目图片只能先作为 `image_gen.reference_file_uuids`，再提交新输出。

后端先按当前项目和 UUID 查询文件，再依次校验会话、绑定上下文、kind/purpose 和消费状态：

- 文件真正不存在或不属于当前项目：`production_resource_not_found`。
- 同项目文件但会话、绑定或用途不合法：`production_validation_failed`。
- 合法生成文件已经绑定到 variant：`production_state_conflict`。

图片替换还要求输出来自绑定目标 Premise Asset 的 `asset_reference` 上下文。

## Prompt 升级

新的 Base Prompt 要求先识别能力；Overview 文档索引已随 system prompt 注入，流程不确定时读取 Guide，调用 `request_api` 前读取目标领域 API Contract。Scene Prompt 不复制创建、替换或 Storyboard 全量更新步骤，也不声明某个工具不可用。

旧中英文内置默认值登记在 `PreviousDefaultValues`。项目打开时，只为仍等于旧默认值的 Prompt 创建迁移版本；用户自定义 Prompt 原样保留。自定义内容若引用已移除的名称，需要用户手动编辑或恢复默认值。

## Route、安全与响应

- 当前全部 Echo Project Route 都可调用；74 条审查 Route 覆盖 Project、Story/Chapter、Premise、Premise Asset、Comic/Storyboard/Snapshot/Export、Task 与 Project Asset 核心能力。
- URL 必须是当前项目的规范相对路径；query 只能放在独立 object 中。审查 Route 优先使用严格 schema，超出覆盖层的字段由真实 REST handler 校验。
- 所有请求与响应只使用 UUIDv7 和公开字段；投影递归排除内部 `id`、路径、metadata、secret 和 credential。
- 每次 `request_api` 必须提供从 `.data` 开始的有限 `response_filter`。
- 写入 intent 先持久化；execution UUID / idempotency key 防止重启后重复创建。
- 危险 Route 以 SHA-256 请求指纹绑定 route、project、target、method/path/query/body、revision 与确认选项，必须单独调用 `request_user_input`。
- 未配置显式风险覆盖层的 GET Route 默认为只读低风险；其余 Route 默认要求请求指纹绑定的用户确认。Provider/密钥、任意本地路径及所有非项目 API 仍不可调用。

## 历史恢复边界

- 新 Run：只创建 `project_api_v2` snapshot。
- v2 Run：可按冻结 snapshot 在 restart、steering、follow-up 和 user-input resume 中恢复。
- 更早 Project API Run：明确拒绝恢复；不提供工具别名或协议映射。
- 已完成历史：数据库审计项可照常查看，不重写 tool name、arguments 或 result。
- `legacy_typed_tools`：只由它自己的持久化历史 snapshot 进入隔离 recovery 路径，不参与新 Run 装配，也不提供已移除名称的兼容。

## 自动化验证

确定性测试覆盖：

- 四个有效 Scene 的相同四工具顺序，以及 `RecommendedGuideIDs` 与 Guide Registry 一致。
- Overview、三份 Guide 和全部 API Contract 可读；未注册路径、Scene 路径、Query、Fragment、编码和路径穿越拒绝。
- Overview 能力/Contract 文档索引、不包含具体 Route、完整 system prompt 注入与 Turn 级快照、Guide 渲染、模板变量和 96 KiB 文档上限。
- Scene Prompt 只包含权威上下文与推荐 Guide；旧默认自动迁移、自定义 Prompt 保留。
- 更早 Project API 协议拒绝、v2 正常恢复、旧文档工具名拒绝。
- 已有设定图直接 POST 的准确来源错误，以及“读取 Guide → 已有项目图片作为参考 → 新输出创建资产”的完整回归。
- 当前消息附件、绑定资产自动参考、图片替换、派生设定项、Storyboard 全量更新和危险操作确认。

未运行 Cargo 或任何 Rust 编译、检查及测试。
