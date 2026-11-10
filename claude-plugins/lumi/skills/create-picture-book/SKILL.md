---
name: create-picture-book
description: 在 Claude Code 中用 Lumi 从想法创作、修改、审阅和导出本地绘本，包括故事、设定、分页及服务端页面生图。用于新建绘本、续做当前 Lumi 项目或生成、更新本地阅读网站。
---

# Lumi 绘本创作

写作、推理和看图使用 Claude Code 自身的能力；Lumi 负责项目事实、版本、图片挂载、审阅和导出，并通过 `generate_page_image` 调用用户在 Lumi 中配置的图片服务商生成页面图（由该服务商计费）。Claude Code 没有原生生图工具，本插件不使用 MCP Apps 或嵌入式工作区；需要查看或在网页中编辑时，用 `web_url` 给出 Lumi Web 链接。服务商未配置时给出 `web_url(view=providers)` 链接，由用户自行配置，不索取或保存 API key。

## 导出阅读网站

用户说“生成网站”“导出成网站”“更新网站”，或上下文已明确是网站导出时，直接调用 `export_website({})`，不预查状态、不区分首次和更新、不重复确认。只说“导出绘本”、指定 PDF / ZIP / Markdown，或询问“能不能生成网站”，都不触发。它生成整本作品的本地静态网站；公网发布须另行提出。

默认不允许缺图。用户已明确接受“只导出已有图片”时直接传 `allow_missing_images=true`；否则仅在收到 `comic_export_incomplete` 后报告实际缺图数量，询问是否继续。收到 `comic_export_empty` 就说明至少需要一张有效正文图，不建议跳过。超时、断线或 `done=false` 时用原参数继续同一任务；如果传了 `idempotency_key`，重试须保持原键。只有 `done=true` 才交付返回的绝对 `output_directory` 和可打开的 `preview_url`，不以旧站仍能访问判断成功。在同一条成功回复中，将返回的完整 `publish_prompt` 原样放在独立的文本代码块中，方便用户复制到 Codex、修改后再发送。提示词仅供复制，执行发布需要用户另行提出。

## 打开或创建项目

执行 CLI 命令前先读 [CLI 入口](references/cli.md)，使用其中的可执行文件和环境参数。下文的 `lumi` 表示该入口；正式版和开发版各有自己的入口，不能混用。

除上述网站导出外，先调用 `project_overview` 确认当前会话目录。stdio MCP 在启动时绑定 cwd，没有切换项目路径的参数；不能靠 shell `cd` 改变已有连接。目录不对时让用户在目标绘本目录新开 Claude Code 会话。

- 返回 `project_not_initialized` 且用户要求创建时，用原始创意调用 `project_init`，得到 setup 草稿。不要先创建一个默认 ready 项目再覆盖用户规格。
- `project_init` 成功后，在同一目录通过 CLI 入口运行 `lumi init --json`，幂等补齐 `user-data/.gitkeep`、AGENTS.md、仅含 `@AGENTS.md` 的 CLAUDE.md 和保护配置；对已经存在的项目不会重置 setup 草稿、内容或规格。然后 `lumi project info --json` 核对配置结果；续做已有项目时也执行一次 `lumi init --json`，用于升级旧版官方 AGENTS.md；返回 agents=updated 后重读该文件。CLAUDE.md 已存在且不含 `@AGENTS.md` 时返回 claude=manual，按提示由用户局部合并，不覆盖原文件。使用入口文档的绝对路径。
- 没有插件工具而用户明确只要求 CLI 初始化时，先 `lumi project info --json` 确认目录；仅在未初始化且用户希望在此建立项目时运行 `lumi init --name 名称 --language zh-Hans --json`。CLI 在全新目录创建 ready 项目，先解释它采用的默认规格；需要自选绘本规格时等待插件可用，使用 `project_init → lumi init（补指引）→ project_setup`。
- 已有项目读取概况、`project_setup get`、`book_outline`、相关章节和设定，保留现有进度。`project_locked` 时按错误详情释放桌面版或其他 CLI 占用，不删锁文件。
检查 `lumi init` 返回的 guidance、claude 和 warnings；配置冲突时保留现有设置，按返回的合并建议处理。插件的 PreToolUse hook 只在 Lumi 项目目录内拦截 Bash 直接访问受保护路径；`.claude/settings.json` 的 deny 规则由用户按插件 README 自行添加，不能替用户修改权限或宣称写保护已完整生效。新的 CLAUDE.md 在新会话中加载；当前会话可以继续创作。

只通过 Lumi CLI / MCP 访问项目事实。**禁止直接读取或写入 `project.sqlite`（含 WAL/SHM 等）、`.lumi/`、`assets/`，也不得让 subagent 这样做。** 不用文件名推测图片。可向用户链接 `export_book` 返回的准确导出路径；这是工具授权的交付结果，无需自己打开内部目录。`STORY.md` 的标准写回使用 `story_profile write`。

## 创作与写回

遵循用户已经给出的语言、年龄、页数、风格、审阅和交付要求。没指定的可逆细节做合理假设并说明；绘本格式与比例定稿后不可变，若用户未指定也未授权自行决定，在 setup 定稿前确认。

**用户要求优先于参考图特征，参考图特征优先于代理默认值**；不能由年龄或“细节自定”推导冲突的造型、配色与字体。**用户附带参考图或图片链接时，先读 [设定图的用户参考](references/setting-references.md)**：按用户指定用途用 Read 实际看图；画风参考先提取具体视觉特征，包括独特对白／旁白字体，新项目在 setup 定稿前保存到 `overall_style`，分页时保留到分镜。设定图的来源与降级方案见该参考；不能只凭文字描述宣称设定图就绪。已有设定图也要检查是否符合目标画风；设定图实际核对通过后，提交绑定当前版本与画风的 `setting_reviews`，正文沿用设定合图流程；首张正文须再核对画风与字体后才继续余页。没有用户参考图时按已保存的用户要求与画风核对。

分别规划**设定图、封面图、正文图**的范围。“条漫图片只生成到第一页”只限制正文图；首次创作默认先完成本次所需的角色、场景和关键道具设定图，再生成第一页。设定图不占正文页数或正文图数量，无需另加确认。只有用户明确要求纯文字、跳过设定图、直接使用文字参考，或限制了全部生图次数时，才按该要求调整；不得自行假设额度不足，或把正文限制扩写成“所有设定只做文字”。`generation_brief` 区分用户要求与代理的执行选择，不把自行省略设定图写成用户约束；恢复任务时也按原始用户要求校核 brief，代理自行写入的限制不视为用户要求。

首次选择 setup 参数、组织规格选项或写入前，先读 [MCP 调用参考](references/mcp-usage.md) 的「Setup 与规格」；其余操作按需读「动作参数与版本」和「渲染与写回」。参数声明显示 `unknown`、缺失或不足以确定必填项时，先查对应段落，不用连续失败的写调用猜参数。参考中的示例值不能替代本项目刚读取的 UUID 和 revision。

只回显下一步需要的字段。主线程已读过的原稿、工具声明，以及仅用于转交的长提示词、写回参数，均省去重复的文本回显。主线程执笔或审阅时读足原文、分镜和对白，逐页实际看图。成功与失败的回显示例见 [MCP 调用参考](references/mcp-usage.md) 的「工具结果回显」。

1. **想法 → setup**：按参考中的合法组合和 `get → update → finalize` 顺序写入。已有明确规格直接沿用；形式与比例冲突时先说明可行选择及影响，不自行替换形式。当前目录没有专门的 setup 模板，不猜造模板名，也不触发 Lumi 内部生成 pipeline。
2. **故事**：用 `render_prompt`（常用 `story/story_profile`）生成 Story Profile 与章节计划；`story_profile write` 保存 profile，`chapters create/write` 保存每章故事。渲染提示词不等于已写入，章节计划也不会自动执行。
3. **设定**：从已保存故事提取本次章节／首图需要的角色、场景和关键道具；先 `settings list/get` 复用符合本次参考要求的已有图片，只有文字不算视觉准备完成。Claude Code 不能直接生成设定图：按 [设定图来源与降级](references/batch-settings.md) 导入用户提供的真实图片文件，或用 `web_url(setting_uuid)` / `web_url(view=settings)` 请用户在 Lumi 中生成后再重读；不得假装批量总览图或单项生图可用。已有文字设定按 UUID 和最新 revision 补图，不重复创建同名项。来源说明、版权备注等纯文字记录无需画图；用户明确的纯文字或跳过设定图要求按前述范围执行。
4. **分页与画面描述**：按章节使用 `render_prompt`（常用 `chapter/comic_storyboard`，封面用目录中的对应模板）规划阅读节奏、正文和完整画面。竖向条漫默认按 [MCP 调用参考](references/mcp-usage.md)传 `moment_count_plan="auto"` 和正文页数上限，由 Lumi 生成逐页镜头数。用户指定节奏时优先采用。写回前按计划核对各页瞬间数，并按剧情决定主画面、停顿和切镜位置，避免把瞬间机械等分为等高画格。按调用参考映射正文、画面描述和 `page_role`，不要把整个输出 JSON 当作正文保存。
5. **生图与挂载**：页面先按 [页面生图](references/page-images.md) 用 `image_target` 选择并核对设定，再调用 `generate_page_image`（提交通过的 `setting_reviews`、页面最新 revision 和本次唯一的 `idempotency_key`）。需要先看参考合图时调用 `prepare_page_image`，用 Read 查看返回的 `reference_image_path`。生成完成后用返回的 `read_image.asset_uuid` 调 `read_image` 实际看成图，并用 `pages record_image_review` 记录当前版本的核对结果，再推进下一页。按用户要求完成整本，不只生成示范页。
6. **导出与交付**：按用户要求完成创作、生图挂载和保存，再按需 `export_book`。单章交付必须传该章 `chapter_uuid`；整书导出省略。核对返回的 scope、page_count、missing_image_count。完成后只调用一次与交付范围对应的 `web_url`：整本省略范围参数（作品总览），指定章节传 `chapter_uuid`，仅生成或交付一页（包括仅第一页）传 `page_uuid`；把返回的 url 原样给用户。导出完成沿用同一交付范围，不追加整本链接。用户要求中途查看或在网页中编辑时同样给出对应范围的 `web_url`，设定传 `setting_uuid`。PDF 基于页面图片，文字要出现在图片中才会进入 PDF；同时用 Markdown 或 ZIP 交付全部可编辑文字。完整绘本交付用 `allow_missing_images=false`。缺页 PDF 仅当用户接受预览时设 true 并明确遗漏，PDF 至少需要一张正文图；中断时可用 true 导出 Markdown/ZIP 保存全部文字与已有图片。

### 应当启动 subagent 的环节

完整新书创作中，**应当启动 subagent** 执行 Story Profile / 章节草稿、设定提取、每章分页与画面描述这些文本任务。只有输入事实已经写入后才能开始下游任务；可并行处理独立章节的草稿。小范围改字无需委派。

父代理先查 `prompt_catalog`，按实际模板及必填占位符调用 `render_prompt`；把返回的完整 `prompt`、`project_uuid`、`target_uuid`、`expected_revision`、`write_back` 和用户约束交给 subagent。优先使用不继承父历史的任务输入，避免依赖未传递的对话。要求 subagent 按模板输出结构化草稿，**不要写项目、不要生图、不要启动 Lumi provider pipeline**。subagent 的共享文件系统不是隔离边界。

长任务可将完整渲染结果和用户约束写入本次任务专用的共享临时文件，附上所需原稿的可读位置。写入成功后传绝对路径，子任务从文件读全文。子任务提交完整草稿供父代理审阅和写回；使用共享文件时，交接消息只列草稿位置和需要核对的问题。无法共享文件时直接传全文。分镜写回前，父代理逐页核对剧情、对白和前后页连续性；用户要求保留原文时逐字核对指定对白。

父代理核对输出与用户要求、渲染时的目标和 revision，再按 `write_back` 和调用参考中的版本来源串行写回。修改或删除已有页时，直接用审稿时从 `pages list` 或 `pages get` 取得的该页 revision。每次创建都需最新父级版本。版本冲突时重读受影响的内容，核对草稿是否仍适用后再继续。

用户要求不使用 subagent、宿主没有委派工具或额度不足时，**走串行路径**：父代理自己执行同一个 `render_prompt` 任务并核对写回。告知采用串行方式，不因缺少 subagent 停止可完成的创作。缺占位符时根据工具错误和 `prompt_catalog` 补齐；不要猜模板名或覆盖工具自动填入的项目事实。

## 服务端页面生图与一致性

**页面生图前必须读 [页面生图](references/page-images.md)**。先选择本页需要的 `setting_uuids`，缺图先补齐；按 [设定图核对](references/setting-references.md) 用 Read 或 `read_image` 实际看当前设定图并记录依据与具体观察，然后调用 `generate_page_image(page_uuid, setting_uuids, setting_reviews, expected_revision, idempotency_key, additional_direction?, model?)`。Lumi 使用与 Web 及 `prepare_page_image` 相同的合图、有效规则、画风和完整提示词，由用户配置的图片服务商生成，并把结果挂载为页面当前图片。明确无需图片参考时传 `[]`，走真实无参考流程。

`expected_revision` 取自本次 `image_target` 或 `pages get`。每次新的生图使用新的 `idempotency_key`；同一请求超时、返回 `done:false` 或会话中断后，用**完全相同的参数和 key** 重放以继续等待或取回结果，不换新 key（会重复生成并计费）。revision 冲突说明页面在 Lumi Web 中被修改：重读页面与最新 revision，必要时重新核对后再生图，不覆盖用户的网页修改。

返回 `image_provider_unavailable` 时说明尚未计费，给出 `web_url(view=providers)` 链接请用户在 Lumi 中配置图片服务商，配置后用同一 key 重试。其他失败按返回的 `error.code/details` 处理；不编写拼图或生图脚本，不直接读取项目数据库、`.lumi` 或资产目录。

单项设定图没有服务端生图工具。先 `image_target(target_uuid, slot=setting_image)` 查询比例和 revision；用户提供真实本地图片文件时，用 `import_image` 传该绝对路径、目标 UUID、slot、revision 和说明来源的 prompt；否则按 [设定图来源与降级](references/batch-settings.md) 请用户在 Lumi 中生成。不能伪造导入成功。

默认先用 `fit_mode=keep` 保留原图；已约定居中裁切/补边时用 `crop/pad`。查看导入返回的 `ratio_matches`、`ratio_deviation`、`warnings`：偏差不阻塞导入；设定或边缘文字宜 pad。处理后的图有独立版本，原图保留。

## 审阅与提问

用户提示词优先：明确要求直接做完/自行决定时连续推进；要求逐步确认时遵守其节点。**未指定时默认两个停顿点**：setup 定稿前汇总读者、格式、比例、页数与画风；完成故事、设定及一张代表性页面后展示审阅，再继续整本生图。完成用户要求的创作、保存和导出后给出对应范围的 `web_url`；不增加确认节点，不等待用户打开链接。用户要求一次生成时连续做完。

只在关键信息缺失、不可变规格、意图冲突、项目目录/占用阻塞或无法合理代决的内容选择时提问。已有授权不要重复索取；宿主批准按实际请求处理。Lumi Web 无法打开不阻塞创作与导出；仍交付文本摘要、准确导出路径和 `web_url` 返回的链接，不把网页加载失败当作内容丢失。用户表示已在网页中修改，或回到对话继续时，先重读相关章节、页面或设定及最新 revision 再写入；revision 冲突时重读，不覆盖用户在网页中的修改。若用户在对话回复候选编号，先用 `pages/settings get include_versions:true` 读取版本身份，按 version_no 核对候选 UUID，再用 `pages/settings select_image` 和最新 revision 持久化。

## 额度耗尽、中断与续做

每段文本写完、每张图生成后及时提交。调用中断可能已经提交，先 get 核实，避免重复创建；页面生图中断时用原 `idempotency_key` 和原参数重放。服务商额度不足或限流时停止反复重试，报告已经完成的章节/页 UUID、缺图与下一步；还能调用工具时导出 Markdown/ZIP，缺图用 true 并标注。不要自行切换服务商或模型，也不向用户索取 API key。

批量设定中断后先 `setting_boards list/get` 检查已保存总览图与 `latest_breakdown`；已保存图片无需重新生成。按批量参考中的幂等与版本规则恢复。

恢复时从 `project_overview → project_setup get → book_outline` 开始；已知目标章时传 `chapter_uuid`，读完游标。以 `review_status` 区分 missing_image（生图）、pending（看已有图）、needs_revision（只修该页）、passed（跳过）；只有即将处理的页才读完整分镜。要取列表版本用 `pages list(summary_only=true)`。旧页缺少核对记录时先看已有图，不因 pending 重生。交付分别说明范围内已生成、核对通过的页数与待修疑点。重新取 revision；正文用 `generate_page_image`，渲染剩余文本任务并读取所需参考图，只补缺失或用户指定修改的内容。

Lumi Web 是查看与编辑入口：`web_url` 只返回现有页面的地址，范围依次为作品总览（省略参数）、章节集合（`view=chapters`）、章节（`chapter_uuid`）、页面（`page_uuid`，可带 `chapter_uuid` 校验）、设定（`setting_uuid` 或 `view=settings`）、导出（`view=exports`）和服务商（`view=providers`）。地址来自当前运行的 CLI 后台，重启后端口可能变化；每次交付前重新调用，不写入文件，不手拼地址。
