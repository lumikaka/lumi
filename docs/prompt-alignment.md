# Lumi AI 提示词链路矩阵

## 运行边界

- 目标仓库：`/Users/qingyang/mine-work/lumi`
- Lumi 使用 Go/Echo/GORM/SQLite/River，并通过本机保存的 Cloudflare Account ID/API Token 接入 Cloudflare AI Gateway REST API。
- Story 链路把项目语言指令放在 JSON system prompt 之前；Premise、设定选择和图片链路把语言指令放在渲染后的 user/image prompt 之前。

## 规范目录

所有规范默认值位于 `internal/promptcatalog`。`catalog_test.go` 强制 zh-Hans/en 拥有相同 group、key 和占位符集合；严格渲染拒绝缺失或残留占位符。

| group | key | 占位符 | 规范来源 | Lumi 调用落点 | 状态与验证 |
|---|---|---|---|---|---|
| story | json_system | 无 | Builtin 中英文规范 | Story River worker、Yolo Story/Profile | 已实现；Story recording-provider 与 Yolo 集成测试 |
| story | story_profile | input_prompt, chapter_count | Builtin；`deep_seek_story_generator.ex:167`；`story_generation_runner.ex:131` | `story_profile_generation`、Yolo 首步 | 已实现；中英文冻结请求、JSON 校验、Profile/Chapter 落盘 |
| story | story_chapter | input_prompt, story_md, chapter_plan_json, generated_summaries_json, chapter_code | Builtin；`deep_seek_story_generator.ex:264` | `story_chapter_generation` | 已实现；中英文请求、版本追加、快照重试测试 |
| story | chapter_batch_plan | input_prompt, story_md, previous_chapter_json, target_chapter_codes_json, chapter_count | Builtin；`deep_seek_story_generator.ex:227`；runner chapter_plan | `story_chapter_batch_plan`、`POST /chapter-batches` | 已实现；服务端编号、严格数量/编号校验、批量创建测试 |
| story | next_story_chapter | story_md, previous_chapter_json, guidance_prompt, next_chapter_code | Builtin；`deep_seek_story_generator.ex:293` | Chapter generation 的 `prompt_key=next_story_chapter` | 已实现；上下文与任务快照测试 |
| story | profile_from_chapters | chapters_json | Builtin；`deep_seek_story_generator.ex:197`；runner profile regeneration | `story_profile_from_chapters`、Profile “从现有章节反推” | 已实现；英文最终请求与落盘测试 |
| chapter | json_system | 无 | Builtin chapter group | Comic storyboard worker/Yolo | 已实现；严格 JSON system 请求测试 |
| chapter | comic_storyboard | chapter_context_json, story_md, input_text, moment_count_plan, chapter_code, max_section_count, max_moments_per_section | Builtin；`deep_seek_story_generator.ex:325`；runner comic storyboard | `comic_storyboard_generation`、Comic “AI 生成分镜”、Yolo | 已实现；普通生成 1–24 sections（默认 6），Yolo 1–6 sections；连续编号、原子落盘测试 |
| chapter | section_premise_selection | max_files, section_id, titles, storyboard | Builtin；`section_image_generator.ex:177-224` | Comic image task 的文本模型选择阶段 | 已实现；冻结候选、严格原名/UUID/上限、选择结果重试复用测试 |
| chapter | before_image | 无 | Builtin `@before_image_prompt` | 作为 section_image 规范正文的一部分，同时可单独版本管理 | 已实现；中英文目录漂移测试 |
| chapter | section_image | style_prompt, reference_usage_text, section_id, storyboard | Builtin；`section_image_generator.ex:242` | Comic image task | 已实现；最终 image prompt、1:3、实际参考图字节测试 |
| premise | setting_image | style_prompt, input_text | Builtin；`premise_generator.ex:304`；`premise_generation_runner.ex:20` | `premise_setting_generation` | 已实现；中英文 image prompt 与任务快照测试 |
| premise | asset_breakdown | image_info_json, style_name, style_prompt, input_text | Builtin；`premise_generator.ex:323`；runner asset breakdown | `premise_asset_breakdown` | 已实现；实际设定图输入、JSON/crop 校验与测试 |
| premise | single_asset_generation | input_text, premise_context, style_prompt | ProjectChatArea 单项生成场景 | 非聊天生产任务；Project Chat 改用受控 `image_gen` | 已实现；生产任务继续冻结快照，聊天在同一 Agent turn 同步生图并写回 |
| premise_style | project_overall_style | 无 | `Stories.premise_overall_style` 与 ProjectChatArea scene | 全部 Premise/Comic 图片生成 | 已实现；规范 override 优先、旧 `premise_profiles.default_style` 兼容 |
| premise_style | simple_cel_anime | 无 | 内置画风预设 | Prompt 管理与生成输入 | 已实现；双语目录测试 |
| premise_style | hong_kong_comic | 无 | 内置画风预设 | Prompt 管理与生成输入 | 已实现；双语目录测试 |
| premise_style | minimal_japanese_handdrawn | 无 | 内置画风预设 | Prompt 管理与生成输入 | 已实现；双语目录测试 |
| agent | base | 无 | Agent 全局安全协议 | 所有 Agent Scene | 已实现；UUID、Overview Contract 索引 system 注入、revision、危险确认和真实状态规则 |
| agent | scene_project_assistant | project_uuid | Project Assistant Scene | 通用 Project Chat | 已实现；全局 Registry、无 subject、无 `image_gen` |
| agent | scene_premise_asset | project_uuid | Premise Asset Generation Scene | Premise 单项生成 | 已实现；512×512、白底单主体、`image_gen → request_api` |
| agent | scene_asset_reference | project_uuid, subject_uuid, asset_type, asset_title, asset_summary, asset_tags, current_file_uuid, asset_revision, overall_style | Asset Reference Scene | 绑定 Premise Asset 的引用会话 | 已实现；Subject 仅为默认对象，PATCH/POST/确认后软删除 |
| agent | scene_storyboard_reference | project_uuid, chapter_uuid, section_uuid | Storyboard Reference Scene | 绑定 Comic Section 的分镜会话 | 已实现；GET 最新事实后完整 POST，无 `image_gen` |
| agent | conversation_summary | summary | Agent runtime context packaging | 本地 Agent 上下文压缩 | 已实现；审计项保留与本地化测试 |

## 选择、覆盖、版本与快照

- `GET /api/v1/projects/:project_uuid/prompts` 返回全部规范项的语言默认值、当前有效值、占位符、legacy key 和当前版本。
- 自定义值通过 append-only `project_prompt_versions` 保存，支持历史列表与“恢复为新版本”。迁移 09 保留 `premise/setting_generation` 读取兼容，新写入使用 `premise/setting_image`；迁移 10 增加 Agent group，并在 down/up 时以 `premise/agent.*` 可逆保存。
- 项目语言切换只迁移默认值；用户自定义版本不被覆盖。任务创建时冻结 generation_language、system/user/image prompt、Provider endpoint/model/参数、章节/Profile revision、Storyboard、候选设定项和文件 UUID。
- River 重试读取快照，不重新读取项目语言、Prompt 版本或候选目录。Section 参考选择结果写入任务事件并在重试时复用；Story workflow 模型 JSON 写入 `story_prompt_results` 并可幂等恢复内容提交。
- Story、Profile、批量章节计划和 Comic storyboard 的 LLM 日志使用各自真实 task kind 作为 scenario，便于逐步骤核验最终请求；迁移升级/回滚会显式备份恢复既有任务事件、Agent 审计和日志。
- Project Chat 创建 Turn 时渲染只含能力与领域 Contract 文档入口的完整 `agent/api-docs/overview.md`，随 Prompt snapshot 冻结，并注入该 Turn 每次模型调用的 system prompt；具体 Route 不进入 Overview，模型通过 `read_agent_doc` 只读取当前目标领域的详细 API Contract。
- API/WS 只投影 UUIDv7；数据库关联继续使用 bigint `id`。所有新增 REST 接口位于 `/api/v1` 并使用统一 success/data/error 信封。

## 单机 Provider 适配

- Story/selection 使用 Cloudflare AI Gateway `/ai/v1/chat/completions`；Base URL 由 Account ID 固定派生，不接受任意 OpenAI-compatible 服务地址。
- Cloudflare 图片生成无论是否包含参考图，均调用 `/ai/v1/responses` 的 `image_generation` tool；参考图以冻结的 `input_image` data URL 发送。Aliyun Bailian adapter 继续把冻结图片编码为 message content data URL。
- 图片与引用文件在任务创建时冻结公开 file UUID，worker 从本地 Asset Store 读取实际字节。API key 只在执行时从全局 secret store 解析，从不进入项目库或公开 payload。
- Project Chat 的两个图片 scene 不进入 River 生产队列：当前设定图、当前消息附件与显式引用按固定顺序解析，`image_gen` 同步写入 Asset Store；随后统一通过 `request_api` 调用当前服务端项目 API。命中审查覆盖层时直接分发领域服务，其他真实项目 Route 通过进程内应用路由执行；可更新当前项、派生创建或经确认软删除。所有新 Run 都使用 Project API Prompt/Tool；升级前已持久化的旧 typed-tool execution 仅由隔离 recovery-only 适配器恢复。
- Comic moment 分布在任务创建时冻结为 `[2,3,1,2,3,1]`，避免 River 重试改变最终请求，使单机 durable task 具有确定性。

## 验证索引

- Catalog：`internal/promptcatalog/catalog_test.go`
- Story/Profile/Batch/Next/Comic 快照与请求：`internal/jobqueue/integration_test.go`
- Premise/Section/图片引用：`internal/jobqueue/production_integration_test.go`
- Project Chat 图片附件、同步生图、全局 Project API 边界、派生创建、写回/软删除与幂等恢复：`internal/agent/agent_test.go`、`internal/agent/scene_migration_test.go`、`internal/agent/agent_api_phase3_test.go`、`internal/production/production_test.go`
- Yolo：`internal/jobqueue/agent_integration_test.go`、`internal/agent/agent_test.go`
- Provider 参考图载荷：`internal/imagegen/client_test.go`
- Prompt 默认/自定义/恢复/语言切换/legacy：`internal/story/story_test.go`、`internal/httpapi/story_test.go`
- 数据迁移：`internal/dbmigrate/migrate_test.go`

本矩阵中的步骤均已落入可调用 service/task/API 或现有界面入口，不包含仅记录但未实现的生成步骤。
