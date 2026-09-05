# 工作流 — YOLO项目初始化

## overview

该 Feature 使用 `yolo_project_initialization` Workflow 根据用户输入生成并初始化项目 Story 与后续创作资源。它把项目初始化拆成可观测步骤，允许中断、恢复和诊断，而不是把整次生成压缩成不可恢复的单一请求。新建普通绘本 YOLO 默认创建或复用一个 `front_cover`，并为封面和第一张 `body` 生成当前图片；它不自动创建 `back_cover`。条漫 `vertical_strip` 不创建封面，只生成第一个 `body` 图片。

首页直接入口保持原行为并创建 dedicated Workflow Thread；对话式项目创建在 Project Setup 完整后，由 Agent Runtime 免确认定稿，并在再次调用模型前从已定稿 `generation_brief` 生成严格受审的创建 Tool Intent，不引入新的 Workflow kind 或执行器。模型不调用该 route。若创建 Session 包含视觉参考，新引用全部按附件顺序自动参与 Premise 设定图；Workflow 仍会冻结持久化计划以恢复历史自定义或排除记录。图片只影响视觉设定，不自动改写 Story。

对话式入口创建成功后使用 `inline` 展示在来源 Turn 内，并以 `workflow_awaits` 暂停同一 bootstrap Run。Turn REST DTO 投影为 `waiting_for_workflow`，Agent worker 释放且不轮询；成功、失败或取消终态再投递唯一 Chat Resume，由模型输出一次最终说明。Workflow Worker 仍是唯一生产者，聊天模型不能逐步模拟 YOLO，也不能循环创建 `vol01.ch02` 到 `vol01.ch06`。

## data_model

YOLO Workflow 使用 `workflows.input_snapshot` 固化用户输入、Prompt 与模型。新建快照为 v6：继续冻结绘本形式与 `chapter/cover_storyboard` Prompt，并从当前项目中受信任的 `creation_session_uuid` 加载完整 `creation_references`。快照按位置保存 Project binding/File UUIDv7、角色、标题、instruction、included 与来源；对新 Session 这些值固定为系统托管默认，对历史 Session 则原样冻结，excluded 项仅供审计。Direct UI 没有创建会话时生成无引用的 v6，行为与历史纯文本路径一致。`workflow_steps` 保存各阶段的业务 Task 或资源 UUID；封面脚本草案、页面 UUID、参考板 File UUID 和每个 Section 的图片任务 UUID 都作为步骤 checkpoint 持久化。Direct UI 创建时 `thread_id` 关联独立 Workflow Thread；bootstrap Chat Tool 创建时指向来源 conversation Thread，并由唯一 await 关联来源 Turn、Run 与 Tool Execution。

对话式入口的幂等键由服务端根据可信 `project_creation_bootstraps.creation_session_uuid` 生成：`project-creation-yolo:<creation_session_uuid>`。项目内 `(project_id, kind, idempotency_key)` 唯一约束保证 Tool Result 丢失、恢复重放、队列重投或应用重启都只恢复同一个 Workflow、来源 Thread、await 与首步 Job。运行时 Tool Intent 还绑定成功 finalization execution UUID 和 creation session UUID，执行前从持久化运行时定稿证据重新验证。模型不能覆盖 story prompt、title、Provider、幂等键或产出数量。升级前已经进入旧确认流程的记录仍可按原 confirmation 证据恢复；已创建的 dedicated YOLO 保持原归属，不回填 await 或迁移历史 Thread。

既有步骤固定为 `project_initialization → story → story_profile → premise → comic_sections → first_section_image`。fresh project 只创建或复用 `vol01.ch01`，生成 1～6 个 `body` Comic Sections。输入快照 v5–v6 的 `comic_sections` 步骤会为非 `vertical_strip` 项目生成封面脚本并创建或复用唯一 `front_cover`；若既有封面已有 current storyboard 或 current image，则直接复用。新生成的封面脚本标题必须与 Chapter 标题逐字相同。

v6 `premise` 步骤先逐项校验快照中的 included File，再幂等注册为来源型 Premise Asset；新 Session 的所有附件都是 included，历史 Session 继续尊重其冻结值。随后把最多 16 张图按冻结顺序合成为 `creation_reference_board/v1` PNG，作为唯一新增的 `imagegen.ImageInput` 发送给 setting image Provider。参考提示词逐项说明历史角色和 instruction，并明确不得复制参考板的网格、标签、编号或留白。只要存在 included 创建参考，setting generation 与对应 breakdown 都必须执行；同名 breakdown 命中来源 Asset 时保留用户来源 current variant，不提交或选择新 crop。无引用 v6 与 v1–v5 保持旧判断。

`first_section_image` 作为稳定 step key 继续保留：输入快照 v5 中它会为封面和第一张正文页批量创建缺失的图片任务，已有 current image 的目标不重复生成；`vertical_strip` 只处理第一个 `body`。它保留 `section_uuid|image_variant_uuid` 对首个正文页的兼容投影，正文图片任务实际存在时也保留指向该任务的 `task_uuid`；同时返回封面/正文页、装订顺序，以及本次实际创建任务的公开 UUID。v1–v4 输入快照继续按旧语义恢复，只生成旧 `first_section_uuid` 指向页面的图片。Premise Setting Image 继续属于设定步骤，不计入封面或正文页图片约束。

## api

| 接口 | 方法 | 说明 |
|---|---|---|
| `/api/v1/projects/:project_uuid/workflows` | POST | 创建 `yolo_project_initialization` Workflow；对话式 bootstrap 请求由运行时从 `generation_brief` 生成，服务端补齐项目名、当前 Run 与创建会话幂等键。 |
| `/api/v1/projects/:project_uuid/workflows/:workflow_uuid` | GET | 读取初始化进度、步骤和公开错误。 |
| `/api/v1/projects/:project_uuid/workflows/:workflow_uuid/cancellations` | POST | 取消初始化。 |
| `/api/v1/projects/:project_uuid/workflows/:workflow_uuid/retries` | POST | 使用冻结输入重试。 |

## ui

| 页面 / 入口 | 说明 |
|---|---|
| 首页 / Direct UI 快速生成 | 以 dedicated Thread 和步骤状态展示初始化，普通绘本完成时显示封面与第一张正文页图片，失败或中断时提供受控重试；没有可信创建会话时不接受任意参考 File 参数。 |
| bootstrap 对话 | Workflow 卡片按 `origin_turn_uuid` 显示在来源 Turn 内，标题保持原 conversation Thread 标题；带 `workflow_uuid` 的导航仍停留在该 Thread。状态由 WebSocket 提示失效并经 REST 重读。 |

## others

Workflow 只编排；总纲、Chapter、Premise 或漫画结果各自按所属 domain 的 revision、版本和幂等约束提交。创建参考相关事件至少包括 `creation_references_frozen` 与生产任务的 `creation_references_composed`，只记录公开 UUID、用途、顺序、数量和合成器版本。

项目必须为 `setup_status=ready`，且对话式入口还必须存在运行时可信生成并成功完成的 finalization 事实。同 Run 路径使用 origin bootstrap；升级后的明确恢复 Turn 可使用 thread lineage，但必须精确绑定成功 finalization execution UUID。缺少前者返回 `project_setup_incomplete`，缺少后者返回 `bootstrap_yolo_not_authorized`。Workflow 创建后的其他生产写操作和 `image_gen` 仍由 bootstrap 门禁拒绝；Workflow 终态后，同一 Run 只读取按 position 排序的安全步骤摘要并完成最终答复。普通危险操作与非运行时 finalization 不继承免确认例外。

参考 File 不存在、未 finalize、无法读取或 MIME 不匹配时以 `yolo_reference_unavailable` 失败；图片解码或参考板合成/持久化失败为 `yolo_reference_board_failed`；Provider 明确拒绝参考图片输入为 `image_reference_unsupported`。三者都允许重试当前步骤，且不得捕获后改发纯文本请求。取消和重试可保留已提交的来源 Asset 与参考板，通过 binding、任务 UUID 和 step output 去重。
