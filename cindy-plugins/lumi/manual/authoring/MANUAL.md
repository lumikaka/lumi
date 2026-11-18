# Lumi 绘本创作

此流程由当前 Cindy Agent 编排，不配置 Lumi 模型服务商。文本任务可按宿主可用能力委派；输入事实保存后才启动下游任务，父代理核对草稿和 revision 后串行写回。不支持委派或用户要求直接完成时串行执行同一流程。项目事实保存在标准 Lumi 项目中，所有业务读写均通过工具完成；不要直接读写 SQLite、素材目录或 STORY.md 投影。

## 发现与项目识别

使用当前发现的精确插件身份和 namespace；正式 id 是 lumi，开发 id 是 lumi-dev，不能把开发调用发给正式版。下文示例中的 `<当前插件 id>` 替换为该身份。先调用 `ghost_call({ghost_id:"<当前插件 id>",tool:"list_tools",args:{}})` 查看类别，再按任务展开。类别结果包含实时 schema 与 RULES，业务读取与写回通过 `call_tool({name:"工具名",args:{业务参数}})`；图片交接使用独立的顶层 receive_media。

使用 Cindy 为当前任务选择的本地作品根目录。网站导出直接调用 export_website；其他创作先 project_overview；只有返回 project_not_initialized，且用户已委托在此创建作品时，才以用户原始创意调用 project_init。有效项目重复 init 不重置内容。目录与锁问题见 `ghost_manual({ghost_id:"<当前插件 id>",path:"project/MANUAL.md"})`。

## Setup 与故事

区分用户明确要求与 Agent 自选方案：`generation_brief` 分别记录两者；续做时回到原始用户要求校核，Agent 自己制定的页数、节奏、额度或省略步骤不能当作用户限制。未指定的可逆细节可在已有授权内合理决定；不可变规格或意图冲突仍需取得决定。已获得连续执行授权时，完成用户要求范围内的创作、逐页核对、修正和交付，不新增重复确认节点；实际权限拒绝、取消或结果未知仍按恢复规则处理。

1. project_setup get 查看 draft。按用户已给定的意图补齐名称、语言、整体画风、创作简报和绘本规格，带 setup.revision 调用 update。需要用户决定的规格先取得决定；已有授权无需重复确认。
2. 使用 update 返回的新 revision finalize。项目必须 ready 才可写故事、章节、设定和页面；最终格式遵循 Lumi 不可变约束。finalize 不启动模型或自动创作流水线。
3. 展开 prompts、story 类别。prompt_catalog 按组查看有效模板元数据和 placeholders；所有业务创作提示词只通过 render_prompt 获取。不得把本手册、旧计划或自身记忆当作模板来源。
4. story/story_profile 常用于故事方案。render_prompt target=story_profile，按目录补齐 variables，例如章节数量。返回的 prompt 自带任务输入、目标 UUID、expected_revision 和 write_back。当前 Agent 执行该任务，将完整 story_md 用 story_profile write 保存，然后 get 核对。
5. chapter_plans 是创作输出，不会自动建章。逐次读取 project_overview 的 project.revision，用 chapters create 保存章节计划。不要把 setup.revision 或 Story Profile revision 用于建章。

## 设定、章节与页面

1. 展开 settings。用 settings list 取得 premise.revision。通过 render_prompt prompt_group=premise、prompt_key=text_settings、target=premise 取得文字设定任务；按结果保存角色、场景、道具或参考文字，每次 create 都重读父集合 revision。已有设定先 get，再带目标 revision update。带页面图片的创作先准备必要设定图，文字存在不代表视觉准备完成；来源/版权备注无需生图。用户明确纯文字、跳过设定图或总生图次数限制按其范围执行，不把“正文只做第一页”解释为跳过设定图。批量流程见 [设定与视觉核对](references/settings.md)。
2. 逐章 render_prompt target=chapter。story/story_chapter 的任务输入按 prompt_catalog 补齐，包含章节计划和已生成摘要等；正文用 chapters write 保存。content 是完整章正文，不能只提交摘要。get 核对当前不可变正文版本。
3. render_prompt target=pages、chapter_uuid 使用当前章。chapter/comic_storyboard 按绘本规格返回页面结构。竖向条漫默认传 `variables={"max_section_count":"2","moment_count_plan":"auto"}`（这里的 2 仅示例，替换为本次正文页数上限，合法范围 1–24），省略 max_moments_per_section。已有共享服务按章节 UUID 生成每页 1～3 镜的稳定计划，并填入完整 prompt；保留该逐页数组，实际写几页就使用前几项。用户明确指定其他节奏时优先采用，显式传入 moment_count_plan 和对应 max_moments_per_section，不能再用 auto 覆盖。其他格式按 prompt_catalog placeholders 提供任务输入，不套用条漫自动计划。保持模板要求的输出格式，再映射到 pages 工具字段。
4. 每页 create 前 pages list 取得最新 state.revision。description_md 保存完整叙述、对白和字幕；storyboard_md 保存完整构图、动作、表情、光线与画面文字。逐页保存并 get 核对。编辑已有页使用 page.revision，不能把章 revision 当作页 revision。
5. 保存前逐页核对镜头数与计划一致，同时核对完整剧情、对白和跨页连续性。分镜密度在规划和核对阶段调整，通过动作组织、停顿和切镜安排实现，不通过截断对白、删除剧情凑数，也不机械等分为等高画格。此默认只用于新规划，不自动改写用户已有作品；已有页仅在用户要求的修改范围内处理。展开 media 获取图片目标、参考图和授权交接规则。pages 已支持 record_image_review；按下节实际看图后记录。review 类别提供工作区开页工具；MCP App-only 资源工具仅由页面调用。

## 可编辑工作区与对话引用

直接调用顶层 `review_book`（作品）、`review_chapters`（章节列表）、`review_chapter(chapter_uuid)`、`review_page(chapter_uuid,page_uuid)`、`review_settings(setting_uuid?)` 或 `review_candidates`。这些工具绑定 Cindy 标准 MCP Apps 结果页；使用 `ghost_call.tool` 指定同名工具，参数直接放 args，不能经 call_tool 包装。作品、章节、单页完成创作或导出后按本次范围打开对应工作区；不等待用户点击才继续已授权制作。

工作区复用 Lumi 的作品、章节、单页、设定及候选组件和 REST 业务接口；地址栏可选择复制，前进后退及详情返回使用卡片内导航。更多操作可切换作品/章节/设定、复制 Lumi Web 链接并重新连接。生成、授权图片交接与导出仍在当前对话通过对应工具完成。

点选页面、章节、设定、内容卡 @ 引用或文本候选，仅通过标准 `app.updateModelContext()` 更新当前对话的公开 UUID 和必要摘要；点击本身不发送消息、不保存修改、不采用版本。用户随后在同一对话继续，例如“刚才选的是哪一页”；根据收到的对象身份通过业务工具重读事实再行动。输入框标签样式由 Cindy 决定。文本候选的全文在页面专用 _meta 中交付，不把全部候选重复放入开页摘要。

保存修改、删除恢复、排序及候选采用均由用户明确操作触发，并使用当前 revision。页面每次调用由 Host 注入当前任务的本机目录和只读权限；卡片中的 expected_project_uuid 仅作身份核对，不能选择任务或目录。旧卡片遇到任务项目切换会拒绝请求，回到当前任务重新开页；只读任务只能浏览。WebSocket 变更提示触发 Query 失效和 REST 重读，并在首次 join、重新 join、窗口聚焦时校准；没有定时 HTTP 业务查询。卡片重开仍读取当前事实，不自动重放保存。

普通业务工具仍用于读取完整正文和审阅证据。不能把工作区开页摘要、包内 HTML、测试截图或页面自报 session 当成完整审阅/可信任务来源；最终是否在真实 Cindy 打开和保存成功以本轮验收记录为准。

## 按章制作与持久恢复

多章制作、小说分镜、按场配图与中断续做必须先读 [按章制作、章内划场与中断恢复](references/serial-production.md)。用户以 create_thread 表达新建独立 Cindy 会话时，使用宿主 send_to_session 新建模式，不传 target_session_id；保留指定模型、项目、执行目录与父任务职责，核实新会话回执并提供链接，不改用 spawn_agent。该映射不提供父子等待或自动完成回报，派发前另行核实；普通任务、原生 thread、subagent 和 Orca 不静默互换。

production_tasks 只描述本插件普通任务适配，不代表当前 Agent 全局能力；状态快照不能当等待接口，禁止父任务 sleep/查询/汇报循环。已核实回报渠道时保存待办并结束当前轮；无渠道时明确能力限制，不承诺自动续接。交接与逐页草稿通过 production_records 保存在 user-data/lumi-production/；仅使用 production_tasks 时要求当前任务串行写回 Lumi、子任务在 Host 分配目录中产草稿，不把这项限制套用于 send_to_session。章节始终串行，场景默认串行，用户明确选择后最多两个无依赖场景；有依赖的后场等待前场当前版本核实。先保存请求再派发，未知结果查原回执和 Lumi 当前事实，不重复建章、生图或导入。只读或宿主任务能力不可用时按参考中的等效流程执行，明确记录差异；父任务只创建和等待时不接手制作。

## 图片、参考图与比例

### 历史版本的矩形局部改图

先 `pages get(include_versions=true)`，在 `image_versions.items` 按用户指定的 version_no 取得本页 base_variant_uuid；可以选择未采用的历史版本。实际查看该版 asset_uuid 的完整页面，以整页左上角为原点给出归一化 `selection={x,y,width,height}`（0～1）。局部放大图坐标不能当作整页坐标。

调用 `prepare_page_image(page_uuid, base_variant_uuid, selection, additional_direction)`，setting_uuids 省略或 []，不传 setting_reviews。返回 `reference_mode=local_edit`，唯一参考是原尺寸上下文裁图，preparation.local_edit 包含基准版、原图摘要、原尺寸、选区、裁图位置和合成器版本。不得手改这些字段或签名。

Core 必须取得 `reference_delivery.kind=local_edit_crop`、status=ready 与 reference_media_url；原生通道仅在正式参数支持本机文件时引用同一 reference_image_path。先实际看裁图，再按本次模型 Guide 将唯一裁图作为图片输入，原样传 generation_prompt。输出保持完整上下文裁图的宽高比、主体位置及比例，不裁边、加边或重新构图；不使用整页 target_ratio 覆盖裁图比例。status=failed 时不提交 Core 请求。

生成后立即通过顶层 `receive_media` 和授权 attachments 导入，带同一完整 prompt、原签名 preparation、expected_revision 和 `fit_mode=keep`。Lumi 仅在矩形选区内合成，羽化也只在选区内，选区外像素保持一致，以无损 PNG 新增候选并返回 `adopted=false`，当前采用版保持原样。实际看候选、展示同位置同倍率对比，用户明确采用后才 `pages select_image`；导入候选不代表采用或核对通过。

局部改图旧 revision、取消或中断后，先重读本页当前采用版、image_versions 和已有写入回执；已导入就查看已有候选，不重复导入。生图提交结果未知（含 SUBMISSION_OUTCOME_UNKNOWN）先查宿主回执，不重复提交生图。原图或签名过期时重新准备；不能只换 revision、摘要或 preparation 重放旧输出。

### 先组装页面图片提示词

页面先选择实际所需的 setting_uuids，按 [设定与视觉核对](references/settings.md) 实际看图并记录 setting_reviews，再调用 `prepare_page_image(page_uuid, setting_uuids, setting_reviews, additional_direction?)`。最多 12 项，明确无参考图时为 []；缺图先补齐，不静默遗漏。Lumi 读取当前图片版本，现拼带编号名称的 PNG 并按 page_role、有效 project_overall_style、语言指令、参考标签、已保存分镜及用户补充要求组装完整提示词，复用 Web 的规则。

保留返回的 generation_prompt、image_write_back 和整个 preparation（含 setting_reviews、digest/signature）。原生引擎只有在实际支持本机路径输入时，才查看并显式引用同一个 reference_image_path；none 模式省略参考参数。Core 必须同时取得 reference_delivery.kind=full_collage、status=ready 与 reference_media_url：这是宿主入库的同一张完整 PNG，摘要、尺寸和 byte_size 随回执返回。先实际查看该合图，再按本次模型 Guide 将该 URL 放入图片输入；generation_prompt 原样传入。路径、设定预览或文字不能替代合图。status=failed 表示准备业务已成功但 Core 交接失败，未提交生图；保留内容，按 instructions 处理，不宣称已传参考。传输自动化不等于真实模型请求已验收。不要逐张读取设定源图、改写参考说明、访问项目资产目录或写拼图脚本。当前通道无法取得合图时说明阻塞并保留内容，不虚构参考传递或改用付费 pipeline。

生成和 receive_media 使用同一完整 prompt，页面还需原样传 preparation。服务端预检和最终事务都重算摘要；页面、设定当前版本、语言/画风/规则发生变化时拒绝导入，需重新准备、读图和生图，不能只换 revision/摘要。服务重启后也需重新准备。提示词最多 12000 字符，不截断对白或规则。

### 生成与交接

1. image_target 使用目标公开 target_uuid 与 page_image / setting_image 槽位，获取最新 expected_revision、推荐比例与构图提示。页面完整提示词来自 prepare_page_image；设定提示词可用 render_prompt；需要文本修改时先按其 write_back 保存，再重新读取图片目标版本。
2. 页面遵循上节的单张临时合图路径；单项设定可用 read_image 读取项目设定或已有插图的 Asset UUID。它附给当前 Agent 一张受控 JPEG 预览，最长边不超过 1024、最多 120 KiB；不是原图。Core image.edit 使用返回的 reference_media_url；引擎原生生图使用当前对话刚读到的参考图机制。reference_delivery.deposited=false 时仍可看图，不能假设 Core 已取得输入。
3. 由当前 Cindy Agent 调用当前可用生图能力。Core 使用本次模型目录与 Guide；已经选定的 provider/model 不擅自换源。引擎原生路径与 Core 分开验收，不根据聊天模型名称宣传订阅生图。
4. 生图成功后立即调用 `ghost_call({ghost_id:"<当前插件 id>",tool:"receive_media",args:{target_uuid,slot,expected_revision,fit_mode:"keep",prompt,preparation},attachments:[实际生成图片引用]})`。attachments 必须在 ghost_call 顶层，恰好一张；不要放进业务 args，不自报路径、指纹、base64 或 session_context。call_tool 无法代替附件入口。
5. 单张最多 20 MiB，只收实际 PNG/JPEG/WebP，完整解码不超过 4000 万像素。比例偏差不代表失败；默认 keep，用户需要时 crop/pad，沿用不可变原图与历史。成功回执的 Asset UUID、版本、尺寸和 source_sha256 表示 Lumi 已落盘；再 get 核对当前选择，不重复导入。
6. production_conflict 保留用户最新选择；correction.recovery_file 给出当前任务 lumi-media-recovery/ 下原图的相对路径、SHA256 和大小。重读目标并判断后，可把这个已知恢复文件再次作为顶层附件交接；不能只换 revision 自动覆盖。恢复文件失败时明确保留原 Host 附件，不能承诺其永久有效。取消、结果未知及 worker 重启后先读当前选择和历史，再决定下一步。

## 页面成图核对与续做

每页图片成功导入后，先用 `pages get(summary_only=true)` 读取当前 `asset_uuid`、`variant_uuid` 和 `revision`，调用 `read_image(asset_uuid)` 实际查看成图。对照该页完整正文、storyboard_md 和用户要求，检查镜头数与计划、画风、人物身份、肢体与道具连续性，以及文字、说话人和指定对白；不能凭生成成功回执或只看参考合图宣称成图通过。看不清文字时不得记 passed，应说明待核对项。看图后重读当前版本；图片或页面已变化则重新看图。

通过发现的 `call_tool` 调用 `pages`，使用 `action="record_image_review"`、当前 `chapter_uuid`、`page_uuid`、`variant_uuid`、`expected_revision`（该页 revision）、`verdict="passed"` 或 `"needs_revision"` 和具体 `observation` 保存核对结果，再推进下一页。首次记录省略 expected_review_revision；修订已有核对记录时先 get 摘要，带 `expected_review_revision=review_revision`。记录绑定当前采用的图片版本和页面 revision，不能把旧图的通过结论复制给新图；需要修图时只修对应页，重新 prepare_page_image、生成、交接并实际看图记录。

续做从 `project_overview → project_setup get → book_outline` 开始，已知目标章时传 chapter_uuid 并读完 cursor_pagination。需要逐页版本时用 `pages list(summary_only=true)`，只对即将处理的页 get 完整正文和分镜。按事实状态 `review_status` 决定下一步：

| review_status | 下一步 |
| --- | --- |
| missing_image | 缺图：补齐本页依赖后生成并交接，再实际看图记录。 |
| pending | 已有图待核对：先 read_image 看已有图并记录，不能当成缺图重画。 |
| needs_revision | 读取完整 observation，只修该页已确认的问题，再核对当前版本。 |
| passed | 跳过；用户要求修改或当前版本变化时再核对。 |

旧页没有记录、重选图片或改动正文/分镜后都可能 pending，不能据此自动重画。交付时分别报告范围内已生成页数、核对通过页数、缺图和待修项；生成数不等于通过数。

## 文本审阅与导出

用 book_outline 检查全书结构和缺文状态，再分页读取 chapters、settings、pages；逐页检查完整正文、画面描述、角色一致性、前后因果和页数。book_outline 不含正文，不能只看目录便宣称完成审阅。发现问题后重读目标并带最新 revision 修正，保留已有内容和不可变历史；delete 使用业务软删除。

export_book 按用户交付范围执行：单章必须传该章 `chapter_uuid`，整书省略；先按同一范围读取 book_outline 和页列表，成功后核对回执 `scope`、`chapter_uuid`、`page_count`、`missing_image_count` 与预期一致，不将单章导出当作整书完成。导出接口只检查缺图，不替代上面的成图核对。

- 完整绘本：范围内正文、分镜完整，所有应交付图片已挂载且当前版本核对 passed，使用 `allow_missing_images=false`，确认 missing_image_count=0。PDF 使用页面图片，只有图片里的文字会进入 PDF；同时按用户要求用 Markdown/ZIP 交付完整可编辑文字。
- 缺图预览：只有用户接受预览或已授权中断时交付进度时才用 `allow_missing_images=true`，明确 page_count、missing_image_count、实际出图和待核对情况，不称完整绘本。PDF 至少需要一张正文图，缺失页不进入 PDF；不能把计划页数当作 PDF 实际图页数。
- 纯文字作品：用户明确要求纯文字时，`format=markdown` 或 `zip`、`allow_missing_images=true` 可在零插图时完成完整文本交付；核对范围内全部正文和分镜均保留，明确未包含的插图，不要求为此补图。中断时同样可保存全部文字和已有图片，但仍按进度交付报告。

导出会落盘，是写操作。成功后原文件保存在项目标准导出目录，适配层将完整字节原子复制到当前任务可见的 `lumi-exports/`，返回相对 path、byte_size 和 SHA256；不接受自定义路径。

分别报告“文件已生成”和“用户已收到”。file_generated=true、delivery.status=prepared 只表示文件已生成且准备交付，user_received 仍为 false。指引用户在 Cindy 当前任务的文件列表打开返回的相对路径，手机使用原生远程任务文件浏览器。此通道已实测完整读取三页作品；收到仍须本次实际读取回执或用户确认，不能由 prepared 推断，更不能把 Mac 绝对路径称作手机可下载地址。聊天中的本地文件链接仅用于定位，不能替代文件列表交付。

export_delivery_failed 的 correction.generated_export 表示原文件已生成、可见副本准备失败；保留原文件，核对文件列表及目录权限，不为修复交付自动重新导出。恢复前先确认已有生成结果。

## 静态阅读网站

用户说“生成网站”“导出成网站”“更新网站”，或上下文已明确是网站导出时，直接调用 `call_tool({name:"export_website",args:{}})`，不预查状态、不区分首次和更新、不重复确认。只说“导出绘本”、指定 PDF / ZIP / Markdown，或询问“能不能生成网站”，都不触发。它生成整本作品的本地静态网站；公网发布须另行提出。

默认不允许缺图。用户已明确接受“只导出已有图片”时直接传 `allow_missing_images=true`；否则仅在收到 `comic_export_incomplete` 后报告实际缺图数量，询问是否继续。收到 `comic_export_empty` 就说明至少需要一张有效正文图，不建议跳过。超时或 `done=false` 用原参数（包括原幂等键）继续同一任务。收到 `done=true` 后，直接告诉用户网站已生成在 `output_directory` 返回的本地目录，可在该目录打开 `index.html` 阅读。在同一条成功回复中，将返回的完整 `publish_prompt` 原样放在独立的文本代码块中，方便用户复制到 Codex、修改后再发送。提示词仅供复制，执行发布需要用户另行提出。

## 冲突、中断与额度恢复

- revision 冲突：保留本地草稿，get/list 重读目标与父集合。已有内容优先；比较差异再决定合并、另写或放弃，不得只换新 revision 重发旧文本。同项目第二任务遵循相同规则。
- 取消、超时、CLI/worker 崩溃、result_unknown：写入可能已提交。先重读目标、父列表和当前版本内容，确定结果；结果未知时不自动重写。网站任务用原参数续接；已有回执时使用回执中的 idempotency_key。恢复后重新获取可信任务绑定，不沿用缓存写权限。
- Cindy Agent 额度耗尽：暂停当前创作，保留已保存 UUID、阶段和未保存草稿。额度恢复后从事实状态接续，重新 render_prompt 获取当前版本；不配置 Lumi 服务商作为替代。
- result_too_large：减小 limit，沿 cursor_pagination 逐页读取；render_prompt 用 context_chapter_uuids/context_setting_uuids 缩小上下文，空数组表示不带该类上下文。不能截掉作品正文并说已完整读取。单项仍超限时先导出完整作品，使用任务文件列表交付。arguments_too_large 在派发前拒绝，按返回 schema 调整单次作品拆分；不要静默裁掉正文。
- prompt_inputs_missing / invalid_arguments：确认工具明确在输入校验阶段返回、没有执行写入或 Core request 时，按返回 schema/占位符补齐后继续。这种补参不等于重放未知写入或付费提交。权限拒绝、取消、超时、result_unknown 和 SUBMISSION_OUTCOME_UNKNOWN 仍停止，不能用“补参”绕过。
- 手机聊天工具记录的 8 KiB 展示投影可能出现 `[remote content truncated: payload too large]`；它不是完整正文或完整调用记录。Agent 实时工具结果保留全文；手机审阅完整作品应打开任务文件，不从截断的聊天记录猜剩余内容。
- 只读任务可查看当前 CLI 已打开的作品。若项目未打开，先由可写任务打开；只读调用不会启动服务、迁移、补记录或修复投影。写操作始终在业务派发前拒绝。不得通过 finalize、select_image、export_book 或初始化绕过只读权限。

恢复时只重读缺失信息；不要清空作品、删除锁或停止共享 CLI。运行管理见 `ghost_manual({ghost_id:"<当前插件 id>",path:"runtime/MANUAL.md"})`。
