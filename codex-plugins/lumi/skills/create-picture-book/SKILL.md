---
name: create-picture-book
description: 在 Codex App 中用 Lumi 从想法创作、修改、审阅和导出本地绘本，包括故事、设定、分页及宿主生图。用于新建绘本、续做当前 Lumi 项目或生成、更新本地阅读网站。
---

# Lumi 绘本创作

写作、推理和图片生成使用 Codex 的能力与额度；Lumi 负责项目事实、版本、图片挂载、审阅和导出。无需 API key，不配置 Lumi 模型服务商。本插件面向 macOS Codex App，并提供 Windows 原生 x64 安装预览；Windows 实机验收尚未完成，使用前按当前会话核实 MCP Apps、宿主读图和生图工具能力。不支持 WSL2、Windows ARM64、TUI 或 IDE 扩展。

## 导出阅读网站

用户说“生成网站”“导出成网站”“更新网站”，或上下文已明确是网站导出时，直接调用 `export_website({})`，不预查状态、不区分首次和更新、不重复确认。只说“导出绘本”、指定 PDF / ZIP / Markdown，或询问“能不能生成网站”，都不触发。它生成整本作品的本地静态网站；公网发布须另行提出。

默认不允许缺图。用户已明确接受“只导出已有图片”时直接传 `allow_missing_images=true`；否则仅在收到 `comic_export_incomplete` 后报告实际缺图数量，询问是否继续。收到 `comic_export_empty` 就说明至少需要一张有效正文图，不建议跳过。超时、断线或 `done=false` 时用原参数继续同一任务；如果传了 `idempotency_key`，重试须保持原键。只有 `done=true` 才交付返回的绝对 `output_directory` 和可打开的 `preview_url`，不以旧站仍能访问判断成功。在同一条成功回复中，将返回的完整 `publish_prompt` 原样放在独立的文本代码块中，方便用户复制到宿主对话、修改后再发送。提示词仅供复制，执行发布需要用户另行提出。

## 打开或创建项目

执行 CLI 命令前先读 [CLI 入口](references/cli.md)，使用其中的可执行文件和环境参数。下文的 `lumi` 表示该入口；正式版和开发版各有自己的入口，不能混用。

除上述网站导出外，先调用 `project_overview` 确认当前会话目录。stdio MCP 在启动时绑定 cwd，没有切换项目路径的参数；不能靠 shell `cd` 改变已有连接。目录不对时让用户在目标目录新开 App 会话。

- 返回 `project_not_initialized` 且用户要求创建时，用原始创意调用 `project_init`，得到 setup 草稿。不要先创建一个默认 ready 项目再覆盖用户规格。
- `project_init` 成功后，在同一目录通过 CLI 入口运行 `lumi init --json`，幂等补齐 `user-data/.gitkeep`、AGENTS.md 和保护配置；对已经存在的项目不会重置 setup 草稿、内容或规格。然后 `lumi project info --json` 核对配置结果；续做已有项目时也执行一次，用于升级旧版官方 AGENTS.md；返回 agents=updated 后重读该文件。自定义文件按返回的局部合并提示处理。不要假定 GUI 继承 PATH，使用入口文档的绝对路径。
- 没有插件工具而用户明确只要求 CLI 初始化时，先 `lumi project info --json` 确认目录；仅在未初始化且用户希望在此建立项目时运行 `lumi init --name 名称 --language zh-Hans --json`。CLI 在全新目录创建 ready 项目，先解释它采用的默认规格；需要自选绘本规格时等待插件可用，使用 `project_init → lumi init（补指引）→ project_setup`。
- 已有项目读取概况、`project_setup get`、`book_outline`、相关章节和设定，保留现有进度。`project_locked` 时按错误详情释放桌面版或其他 CLI 占用，不删锁文件。
- 检查 `lumi init` 返回的 guidance 和 warnings；配置冲突时保留现有设置，按返回的合并建议处理。提示用户在 App 信任项目并新开会话使配置生效；不能替用户设置 trust 或宣称写保护已生效。当前 MCP 会话可以继续创作，信任提示不替代用户的连续执行授权；用户/session 权限覆盖可能影响实际保护。

宿主没有命令执行工具时，不捏造 CLI 执行结果；若 `project_overview` 已确认本目录项目可用，记录指引升级／CLI 配置核验未执行，继续已获授权的 MCP 创作。项目仍未初始化时使用 `project_init → project_setup`；缺少的宿主配置由用户在有能力的环境补齐，不声称保护或配置已经生效。

只通过 Lumi CLI / MCP 访问项目事实。**禁止直接读取或写入 `project.sqlite`（含 WAL/SHM 等）、`.lumi/`、`assets/`，也不得让 subagent 这样做。** 不用文件名推测图片。可向用户链接 `export_book` 返回的准确导出路径；这是工具授权的交付结果，无需自己打开内部目录。`STORY.md` 的标准写回使用 `story_profile write`。

## 创作与写回

遵循用户已经给出的语言、年龄、页数、风格、审阅和交付要求。没指定的可逆细节做合理假设并说明；绘本格式与比例定稿后不可变，若用户未指定也未授权自行决定，在 setup 定稿前确认。

**用户要求优先于参考图特征，参考图特征优先于代理默认值**；不能由年龄或“细节自定”推导冲突的造型、配色与字体。**用户附带参考图或图片链接时，先读 [设定图的用户参考](references/setting-references.md)**：按用户指定用途实际看图；画风参考先提取具体视觉特征，包括独特对白／旁白字体，新项目在 setup 定稿前保存到 `overall_style`，分页时保留到分镜。批量和单项设定图生成都要显式携带本次相关的视觉参考原图，不能只传文字描述。已有设定图也要检查是否符合目标画风，必要时保留身份重新绘制；设定图实际核对通过后，提交绑定当前版本与画风的 `setting_reviews`，正文沿用设定合图流程；首张正文须再核对画风与字体后才继续余页。没有用户参考图时按已保存的用户要求与画风核对。

分别规划**设定图、封面图、正文图**的范围。“条漫图片只生成到第一页”只限制正文图；首次创作默认先完成本次所需的角色、场景和关键道具设定图，再生成第一页。设定图不占正文页数或正文图数量，无需另加确认。只有用户明确要求纯文字、跳过设定图、直接使用文字参考，或限制了全部生图次数时，才按该要求调整；不得自行假设额度不足，或把正文限制扩写成“所有设定只做文字”。`generation_brief` 区分用户要求与代理的执行选择，不把自行省略设定图写成用户约束；恢复任务时也按原始用户要求校核 brief，代理自行写入的限制不视为用户要求。

首次选择 setup 参数、组织规格选项或写入前，先读 [MCP 调用参考](references/mcp-usage.md) 的「Setup 与规格」；其余操作按需读「动作参数与版本」和「渲染与写回」。参数声明显示 `unknown`、缺失或不足以确定必填项时，先查对应段落，不用连续失败的写调用猜参数。参考中的示例值不能替代本项目刚读取的 UUID 和 revision。

在 Codex 工具执行层原样传递下一步需要的完整结果。主线程已读过的原稿、工具声明，以及仅用于转交的长提示词、写回参数，均省去重复的文本回显。主线程执笔或审阅时读足原文、分镜和对白，逐页实际看图。成功与失败的回显示例见 [MCP 调用参考](references/mcp-usage.md) 的「工具结果回显」。

1. **想法 → setup**：按参考中的合法组合和 `get → update → finalize` 顺序写入。已有明确规格直接沿用；形式与比例冲突时先说明可行选择及影响，不自行替换形式。当前目录没有专门的 setup 模板，不猜造模板名，也不触发 Lumi 内部生成 pipeline。
2. **故事**：用 `render_prompt`（常用 `story/story_profile`）生成 Story Profile 与章节计划；`story_profile write` 保存 profile，`chapters create/write` 保存每章故事。渲染提示词不等于已写入，章节计划也不会自动执行。
3. **设定规划**：从已保存故事提取本次章节／首图需要的角色、场景和关键道具；实际设定制作与核对由本章设定 subagent 在分镜保存、划场后统一完成。先 `settings list/get` 复用符合本次参考要求的已有图片，只有文字不算视觉准备完成。首次带页面生图的创作必须先补齐必要设定图。多个独立主体按 [批量设定图](references/batch-settings.md) 生成总览图、识图，再由 Lumi 裁切入库；每批 1–6 个，复杂场景或精修单独处理。已有文字设定按 UUID 和最新 revision 补图，批量拆分可直接创建新图文设定，不重复创建同名项。来源说明、版权备注等纯文字记录无需画图；用户明确的纯文字或跳过设定图要求按前述范围执行。
4. **分页与画面描述**：输入为小说原文，且缺少可用分镜或用户要求重做时，由**章节主聊天**按 [小说分镜委派](references/serial-production.md#小说分镜委派) 为本章启动一个 `spawn_agent(fork_turns="none")` 分镜 subagent；它只写逐页草稿，章节主聊天逐页核对、串行写回后再划场。已有可用分镜且只需配图时直接复用，不启动分镜代理；中断先复用草稿、进度和原代理。其他分页仍按实际 `render_prompt` 模板及 [MCP 调用参考](references/mcp-usage.md) 处理。竖向条漫默认 `moment_count_plan="auto"` 和正文页数上限，用户指定节奏优先；按剧情安排主画面、停顿和切镜，核对逐页瞬间数，不机械等分等高画格。正文、画面描述、`page_role` 分别映射，不把输出 JSON 整体当正文。
5. **生图与挂载**：页面先按 [页面生图提示词](references/page-images.md) 调用 `prepare_page_image`（先提交通过的 `setting_reviews`），当前代理首次遇到该摘要与路径时查看参考合图，再将同一路径和完整 `generation_prompt` 交给宿主生图；单项设定按下节生成。每张图生成后立即导入；逐页实际看成图并用 `pages record_image_review` 记录当前版本的核对结果，再推进下一页；批量设定总览图用 `setting_boards import`，随后识图并 `apply_setting_breakdown`。按用户要求完成整本，不只生成示范页。
6. **导出与展示**：按用户要求完成创作、生图挂载和保存，再按需 `export_book`。单章交付必须传该章 `chapter_uuid`；整书导出省略。核对返回的 scope、page_count、missing_image_count。完成后只调用一次与交付范围对应的正常页面入口：整本用 `review_book()`（作品总览），指定章节用 `review_chapter(chapter_uuid)`，仅生成或交付一页（包括仅第一页）用 `review_page(chapter_uuid,page_uuid)`；查看章节集合用 `review_chapters()`。导出完成沿用同一交付范围，不追加阅读预览或再次打开整本。用户明确要求的中途审阅仍可使用这些入口或 `review_settings`，完成后继续执行。PDF 基于页面图片，文字要出现在图片中才会进入 PDF；同时用 Markdown 或 ZIP 交付全部可编辑文字。完整绘本交付用 `allow_missing_images=false`。缺页 PDF 仅当用户接受预览时设 true 并明确遗漏，PDF 至少需要一张正文图；中断时可用 true 导出 Markdown/ZIP 保存全部文字与已有图片。

### 按章、按场制作与委派边界

**制作或续做章节前先读 [制作调度与恢复](references/serial-production.md)**。单章由当前聊天负责；涉及两章及以上时，当前聊天只管总进度，仍按章串行交接最终设定版本与连续性。每章一个设定 subagent，每场一个独立场景 subagent。场景默认串行，上一场通过并经主聊天从 Lumi 核实后才启动下一场；仅当用户明确选择“独立场景最多两场并行”时，按参考中的依赖判断启用章内例外，不扩展为并行章节。分镜和设定通过后先记录每场起始状态、依赖、依据及唯一页写入者；起始状态都确定且无未决图像连续性依赖的两场才可并行，依赖不明按串行。有站位、伤势、持物或动作承接的后场，须等依赖场通过，由主聊天核实当前采用版本、审阅状态并保存连续性交接后才启动。

场内始终逐页完成准备参考、生图、立即挂载、看实际采用图、保存核对，本页通过后再做下一页，不派并行画页代理。用户审阅节点和首张正文核对门槛仍有效。共享设定返修先暂停全部受影响场景，核查进行中请求及恢复导入，再交原设定代理修正；实际复核受影响已完成页、解决版本冲突和过期准备后恢复。中断先读 Lumi 当前采用版本、revision、审阅与未决请求，按唯一写入者恢复。

多章使用 `list_projects → create_thread`，在**同一 Lumi 项目目录**为每章建独立 thread；不为章节建 worktree，不同时启动后续章。先查交接记录与已有 thread，扩章和恢复时复用。总聊天只读章节摘要、阻塞、交接和 Lumi 摘要，不读小说原文全文、整章分镜草稿，不为章节渲染含原文的提示词。章节主聊天用不继承父历史的 `spawn_agent(fork_turns="none")` 委派小说分镜、设定与场景；每个活动场景仅可写本场分配页，不改其他场、共享设定、原文或分镜。

等待章节时使用 [章节等待示例](references/serial-production.md#章节等待示例)：外层 `functions.exec` 第一行显式写 `// @exec: {"yield_time_ms": 55000}`，内层 `wait_threads` 使用 `timeoutMs: 45000`。提前完成即返回；超时后原样沿用返回 cursor 作为 `afterCursor` 继续等待，按真实完成／needs_attention 状态处理，不用默认外层时限制造额外 `functions.wait`。

| 角色 | 可写范围 | 返回内容 |
| --- | --- | --- |
| 文本草稿 subagent | 仅任务专用草稿文件；不写 Lumi、不生图 | 完整结构化草稿或其绝对路径、待核对问题 |
| 小说分镜 subagent | 仅本章 `storyboard/` 草稿；只读本章原文、用户要求、必要前章交接与任务格式；不写 Lumi、不生图、不派子任务 | 草稿绝对路径、页数、待裁决问题；不返回整章分镜 |
| 章节设定 subagent | 本章所需设定的文字、图片及核对证据；不写页面 | 采用的设定 UUID／版本、变化、适用页段、受影响页 |
| 场景 subagent | 分配页的图片挂载及 `pages record_image_review`、本场交接文件；不改设定、原文或分镜 | 页码／UUID、采用版本、审阅状态、跨场细节或具体阻塞 |
| 章节主聊天 | 审核并串行写入文本、划场和协调；从 Lumi 核实图片与审阅事实 | 章节摘要、最终设定版本、跨章连续性、未完成项 |

完整新书创作中，Story Profile／章节草稿、设定**提取草稿**、每章分页与画面描述仍应委派文本 subagent；这些草稿任务不能代替设定制作或场景制作。按章顺序落地，不以独立草稿为由提前启动后章制作。小范围改字无需委派。

父代理先查 `prompt_catalog`，按实际模板及必填占位符调用 `render_prompt`；把完整 `prompt`、`project_uuid`、`target_uuid`、`expected_revision`、`write_back` 和用户约束交给文本 subagent。使用精简任务输入，不依赖父历史。可将完整渲染结果和所需原稿写入专用共享临时文件，写成功后传绝对路径；不能让子任务读取受保护的项目内部文件。共享文件系统不是隔离边界。

文本草稿由父代理逐页核对剧情、对白和连续性，保留原文时逐字核对，再按 `write_back` 串行写回。修改已有页用审稿时取得的页面 revision；每次创建前读最新父集合版本，冲突时重读并核对草稿是否仍适用。制作角色同样遵守版本规则，但在表内授权范围直接通过 Lumi 保存结果，无需等父代理转写图片。

上述小说分镜任务的父代理始终是章节主聊天；采用持久逐页草稿与进度，不能以通用文本任务允许返回完整草稿为由把它送回总聊天。已有小说原文无需再派 Story Profile／章节改写代理。小说分镜的触发、复用和恢复优先按串行参考执行，不因“完整新书”重复委派分页。

用户禁止 subagent、委派工具不可用或额度不足时，告知由章节主聊天串行承担相同职责；保留设定、场景和逐页审阅门槛，不因降级停止可完成的工作。若仅子任务缺少生图／读图权限，按串行参考中的宿主代执行流程处理，不伪报完成、不自行改用其他服务。缺占位符时查工具错误和 `prompt_catalog`，不猜模板名。

## 宿主生图与一致性

**页面生图前必须读 [页面生图提示词](references/page-images.md)**。先选择本页需要的 `setting_uuids`，缺图先补齐；按 [设定图核对](references/setting-references.md) 核对当前图片并记录依据与具体观察，然后调用 `prepare_page_image(page_uuid, setting_uuids, setting_reviews, additional_direction?)`。Lumi 使用与 Web 相同的有效规则和画风，动态现拼一张带编号、名称的 PNG，返回临时绝对路径、合图 SHA-256、来源版本清单、完整 `generation_prompt`、`image_write_back` 和带摘要签名的 `preparation`。明确无需图片参考时传 `[]`，走真实无参考流程。

当前代理在本对话已经实际看过相同 `reference_image_sha256` 和 `reference_image_path` 时跳过预览；首次、摘要缺失或中断后不确定时实际读图。每页都调用 App 原生 imagegen，`referenced_image_paths` **只传这一个相同路径**；none 模式不传参考参数。页面不再逐张 `read_image`，不使用 `num_last_images_to_include`，不改写参考说明，不编写拼图脚本，不直接读取项目数据库、`.lumi` 或资产目录。保留项目画布、对白和阅读结构，完整提示词不追加、不翻译、不摘要；本次补充要求通过 `additional_direction` 交给服务端。

页面导入原样带回同一 `generation_prompt` 和整个 `preparation`。服务端重算当前事实摘要，所选图片版本、页面或规则变化时拒绝导入且不创建图片版本；重新准备、读图和生图，不能仅替换 revision 或摘要。临时路径失效或 Lumi 服务重启时重新准备。

单项设定图先 `image_target(target_uuid, slot=setting_image)` 查询比例和 revision；已有主体参考通过 `settings get → read_image` 读取。用户附带视觉参考时，按 [设定图的用户参考](references/setting-references.md) 将相关原图与必要的主体参考显式传给宿主 imagegen，并说明各图用途。`target_ratio` 是项目画布，`recommended_ratio` 仅为宿主兼容建议。页面的 `image_target` 只负责候选／缺图检查，实际生成输入来自 `prepare_page_image`。

**页面和单项设定图生成完成后立即 `import_image`**：`path` 必须取宿主本次返回的真实绝对路径（也可能在 output hint 文本里），传目标 UUID、slot、revision、原始 prompt；页面还必须带回本轮完整 `preparation`。不能假定有 `savedPath` 字段，不能猜目录或扫描生成文件。宿主没提供可访问路径时说明缺失，不能伪造导入成功。

默认先用 `fit_mode=keep` 保留原图；已约定居中裁切/补边时用 `crop/pad`。查看导入返回的 `ratio_matches`、`ratio_deviation`、`warnings`：偏差不阻塞导入；页面适合在主体安全时 crop，设定或边缘文字宜 pad，必要时下一张调整构图。不要仅因像素不精确反复重生。处理后的图有独立版本，原图保留。

## 审阅与提问

用户提示词优先：明确要求直接做完/自行决定时连续推进；要求逐步确认时遵守其节点。**未指定时默认两个停顿点**：setup 定稿前汇总读者、格式、比例、页数与画风；完成故事、设定及一张代表性页面后展示审阅，再继续整本生图。完成用户要求的创作、保存和导出后打开对应范围的正常业务页面；不增加确认节点，不等待用户打开、翻页或操作 MCP Apps。用户要求一次生成时连续做完。

只在关键信息缺失、不可变规格、意图冲突、项目目录/占用阻塞或无法合理代决的内容选择时提问。已有授权不要重复索取；宿主批准按实际请求处理。MCP Apps 关闭、不可用或加载失败均不阻塞创作与导出；仍交付文本摘要、准确导出路径和工具返回的真实 Lumi Web 业务页面入口/命令，不把界面加载失败当作内容丢失。正常页面保留图片展示、点击大图和本地版本预览，不能以界面点击作为保存、审批或 `import_image` 的前置条件。`review_candidates` 的预览不写入；用户可在共享工作区显式“使用此稿”直接采用。若用户在对话回复编号，先用 `pages/settings get include_versions:true` 读取版本身份，按 version_no 核对候选 UUID，再用 `pages/settings select_image` 和最新 revision 持久化。普通编辑、保存、排序和版本操作由页面经宿主完成，无需额外模型回合。

## 额度耗尽、中断与续做

每段文本写完、每张图生成后及时提交。调用中断可能已经提交，先 get 核实，避免重复创建和重复导入。额度不足时停止反复重试，报告已经完成的章节/页 UUID、缺图与下一步；还能调用工具时导出 Markdown/ZIP，缺图用 true 并标注。不要自动切换到付费 API，也不要求 API key。

批量设定中断后先 `setting_boards list/get` 检查已保存总览图与 `latest_breakdown`；已保存图片无需重新生成。按批量参考中的幂等与版本规则恢复。

恢复时先按 [串行制作与恢复](references/serial-production.md) 复用章节聊天与场景记录，再从 `project_overview → project_setup get → book_outline` 开始；已知目标章时传 `chapter_uuid`，读完游标。以 `review_status` 区分 missing_image（生图）、pending（看已有图）、needs_revision（只修该页）、passed（跳过）；只有即将处理的页才读完整分镜。要取列表版本用 `pages list(summary_only=true)`。旧页缺少核对记录时先看已有图，不因 pending 重生。交付分别说明范围内已生成、核对通过的页数与待修疑点。重新取 revision；正文直接 prepare_page_image，渲染剩余文本任务并读取所需参考图，只补缺失或用户指定修改的内容。旧生图路径失效且未导入时再生成，不扫描宿主目录寻找替代文件。

作品工作区复用 Web 页面主体；仅隐藏全局顶栏、导航与聊天，保留章内 section 快速切换。`review_*` 仅返回身份、摘要和入口，完整正文、历史和图片由页面经宿主按需读取，不把大对象或 Base64 重复放进模型上下文。四层入口：`review_book()` 总览 → `review_chapters()` → `review_chapter(chapter_uuid)` → `review_page(chapter_uuid,page_uuid)`。章节入口保留章节 URL，复用 Web 逐页编辑器，默认选中第一页，可在章内切页；空章节显示页面管理。内部导航在 App 完成；“在 Lumi 中打开”使用工具给出的真实端口与资源 URL。新调用不传 `mode`，不进入独立阅读器。旧 `review_book({chapter_uuid})`、旧 preview 参数和历史 reader 结果兼容到相同范围的正常页面，指定章节始终进入该章节。
