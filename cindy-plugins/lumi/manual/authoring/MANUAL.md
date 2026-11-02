# Lumi 绘本创作

此流程由当前 Cindy Agent 串行执行，不需要多 Agent，也不配置 Lumi 模型服务商。项目事实保存在标准 Lumi 项目中，所有业务读写均通过工具完成；不要直接读写 SQLite、素材目录或 STORY.md 投影。

## 发现与项目识别

使用当前发现的精确插件身份和 namespace；正式 id 是 lumi，开发 id 是 lumi-dev，不能把开发调用发给正式版。下文示例中的 `<当前插件 id>` 替换为该身份。先调用 `ghost_call({ghost_id:"<当前插件 id>",tool:"list_tools",args:{}})` 查看类别，再按任务展开。类别结果包含实时 schema 与 RULES，业务读取与写回通过 `call_tool({name:"工具名",args:{业务参数}})`；图片交接使用独立的顶层 receive_media。

使用 Cindy 为当前任务选择的本地作品根目录。先 project_overview；只有返回 project_not_initialized，且用户已委托在此创建作品时，才以用户原始创意调用 project_init。有效项目重复 init 不重置内容。目录与锁问题见 `ghost_manual({ghost_id:"<当前插件 id>",path:"project/MANUAL.md"})`。

## Setup 与故事

1. project_setup get 查看 draft。按用户已给定的意图补齐名称、语言、整体画风、创作简报和绘本规格，带 setup.revision 调用 update。需要用户决定的规格先取得决定；已有授权无需重复确认。
2. 使用 update 返回的新 revision finalize。项目必须 ready 才可写故事、章节、设定和页面；最终格式遵循 Lumi 不可变约束。finalize 不启动模型或自动创作流水线。
3. 展开 prompts、story 类别。prompt_catalog 按组查看有效模板元数据和 placeholders；所有业务创作提示词只通过 render_prompt 获取。不得把本手册、旧计划或自身记忆当作模板来源。
4. story/story_profile 常用于故事方案。render_prompt target=story_profile，按目录补齐 variables，例如章节数量。返回的 prompt 自带任务输入、目标 UUID、expected_revision 和 write_back。当前 Agent 执行该任务，将完整 story_md 用 story_profile write 保存，然后 get 核对。
5. chapter_plans 是创作输出，不会自动建章。逐次读取 project_overview 的 project.revision，用 chapters create 保存章节计划。不要把 setup.revision 或 Story Profile revision 用于建章。

## 设定、章节与页面

1. 展开 settings。用 settings list 取得 premise.revision。通过 render_prompt prompt_group=premise、prompt_key=text_settings、target=premise 取得无图片的文字设定任务；按结果逐个保存角色、场景、道具或参考文字，每次 create 都重读父集合 revision。已有设定先 get，再带目标 revision update。文字设定不要求图片。
2. 逐章 render_prompt target=chapter。story/story_chapter 的任务输入按 prompt_catalog 补齐，包含章节计划和已生成摘要等；正文用 chapters write 保存。content 是完整章正文，不能只提交摘要。get 核对当前不可变正文版本。
3. render_prompt target=pages、chapter_uuid 使用当前章。chapter/comic_storyboard 按绘本规格返回页面结构；页数和视觉单位等任务输入由目录 placeholders 决定。保持模板要求的输出格式，再映射到 pages 工具字段。
4. 每页 create 前 pages list 取得最新 state.revision。description_md 保存完整叙述、对白和字幕；storyboard_md 保存完整构图、动作、表情、光线与画面文字。逐页保存并 get 核对。编辑已有页使用 page.revision，不能把章 revision 当作页 revision。
5. 展开 media 获取图片目标、参考图和授权交接规则。review 类别尚未实现，先通过完整文字及 read_image 审阅；不调用 MCP App-only 工具。

## 图片、参考图与比例

### 先组装页面图片提示词

`chapter/section_image` 是组合模板，不能只传 page_uuid 就直接渲染。先读取 `prompt_catalog` 的 placeholders，然后通过同一目标页面的 `render_prompt` 获取所需片段：

1. 正文页用 `chapter/before_image`，封面用 `cover_before_image`，封底用 `back_cover_before_image`。以返回的 `fragment_prompt` 作为 `variables.before_image_prompt`，保留项目自定义规则；不要复制外层 `prompt` 的 JSON 输出要求、任务上下文或写回说明。
2. 本次确实没有设定参考图时，渲染 `chapter/section_reference_absent`，以其 `fragment_prompt` 作为 `variables.reference_usage_text`。只有实际向生图模型提供了对应设定拼贴图时，才渲染 `section_reference_present` 并补齐 `reference_titles`；已有文字设定不代表提供了参考图片。
3. 调用 `render_prompt`，使用 `prompt_group=chapter`、`prompt_key=section_image`、`target=page`、实际 chapter_uuid/page_uuid，以及上述两个变量。`section_id`、storyboard、画风由服务端从当前页面/项目读取，不能伪造；旧 CLI 若仍要求 section_id，使用刚读取的页面公开 UUID，不能用序号或内部 id。
4. 图片模型只使用返回的完整 `generation_prompt`；导入使用 `image_write_back` 中的目标、slot 与 expected_revision。外层 `prompt` 和原 `write_back` 保留为任务说明，不因此更新页面文字。生图和导入保存同一完整提示词，最多12000字符。

若当前返回没有 `fragment_prompt` / `generation_prompt`，说明实际 CLI 尚未包含该能力；先核对构建并协调开发后台更新，不从输出里猜边界或硬编默认模板。提示词组装全部成功后再 Core prepare，避免拿到提交令牌后才发现输入不完整。

### 生成与交接

1. image_target 使用目标公开 target_uuid 与 page_image / setting_image 槽位，获取最新 expected_revision、推荐比例与构图提示。画面提示词来自 render_prompt；需要文本修改时先按其 write_back 保存，再重新读取图片目标版本。
2. 用 read_image 读取项目设定或已有插图的 Asset UUID。它附给当前 Agent 一张受控 JPEG 预览，最长边不超过 1024、最多 120 KiB；不是原图。Core image.edit 使用返回的 reference_media_url；引擎原生生图使用当前对话刚读到的参考图机制。reference_delivery.deposited=false 时仍可看图，不能假设 Core 已取得输入。
3. 由当前 Cindy Agent 调用当前可用生图能力。Core 使用本次模型目录与 Guide；已经选定的 provider/model 不擅自换源。引擎原生路径与 Core 分开验收，不根据聊天模型名称宣传订阅生图。
4. 生图成功后立即调用 `ghost_call({ghost_id:"<当前插件 id>",tool:"receive_media",args:{target_uuid,slot,expected_revision,fit_mode:"keep",prompt},attachments:[实际生成图片引用]})`。attachments 必须在 ghost_call 顶层，恰好一张；不要放进业务 args，不自报路径、指纹、base64 或 session_context。call_tool 无法代替附件入口。
5. 单张最多 20 MiB，只收实际 PNG/JPEG/WebP，完整解码不超过 4000 万像素。比例偏差不代表失败；默认 keep，用户需要时 crop/pad，沿用不可变原图与历史。成功回执的 Asset UUID、版本、尺寸和 source_sha256 表示 Lumi 已落盘；再 get 核对当前选择，不重复导入。
6. production_conflict 保留用户最新选择；correction.recovery_file 给出当前任务 lumi-media-recovery/ 下原图的相对路径、SHA256 和大小。重读目标并判断后，可把这个已知恢复文件再次作为顶层附件交接；不能只换 revision 自动覆盖。恢复文件失败时明确保留原 Host 附件，不能承诺其永久有效。取消、结果未知及 worker 重启后先读当前选择和历史，再决定下一步。

## 文本审阅与导出

用 book_outline 检查全书结构和缺文状态，再分页读取 chapters、settings、pages；逐页检查完整正文、画面描述、角色一致性、前后因果和页数。book_outline 不含正文，不能只看目录便宣称完成审阅。发现问题后重读目标并带最新 revision 修正，保留已有内容和不可变历史；delete 使用业务软删除。

export_book format=markdown 或 zip、allow_missing_images=true 可在没有插图时导出完整文本作品。PDF 仍需已有正文图片。导出会落盘，是写操作。成功后原文件保存在项目标准导出目录，适配层将完整字节原子复制到当前任务可见的 `lumi-exports/`，返回相对 path、byte_size 和 SHA256；不接受自定义路径。

分别报告“文件已生成”和“用户已收到”。file_generated=true、delivery.status=prepared 只表示文件已生成且准备交付，user_received 仍为 false。指引用户在 Cindy 当前任务的文件列表打开返回的相对路径，手机使用原生远程任务文件浏览器。此通道已实测完整读取三页作品；收到仍须本次实际读取回执或用户确认，不能由 prepared 推断，更不能把 Mac 绝对路径称作手机可下载地址。聊天中的本地文件链接仅用于定位，不能替代文件列表交付。

export_delivery_failed 的 correction.generated_export 表示原文件已生成、可见副本准备失败；保留原文件，核对文件列表及目录权限，不为修复交付自动重新导出。恢复前先确认已有生成结果。

## 冲突、中断与额度恢复

- revision 冲突：保留本地草稿，get/list 重读目标与父集合。已有内容优先；比较差异再决定合并、另写或放弃，不得只换新 revision 重发旧文本。同项目第二任务遵循相同规则。
- 取消、超时、CLI/worker 崩溃、result_unknown：写入可能已提交。先重读目标、父列表和当前版本内容，确定结果；结果未知时不自动重写。恢复后重新获取可信任务绑定，不沿用缓存写权限。
- Cindy Agent 额度耗尽：暂停当前创作，保留已保存 UUID、阶段和未保存草稿。额度恢复后从事实状态接续，重新 render_prompt 获取当前版本；不配置 Lumi 服务商作为替代。
- result_too_large：减小 limit，沿 cursor_pagination 逐页读取；render_prompt 用 context_chapter_uuids/context_setting_uuids 缩小上下文，空数组表示不带该类上下文。不能截掉作品正文并说已完整读取。单项仍超限时先导出完整作品，使用任务文件列表交付。arguments_too_large 在派发前拒绝，按返回 schema 调整单次作品拆分；不要静默裁掉正文。
- prompt_inputs_missing / invalid_arguments：确认工具明确在输入校验阶段返回、没有执行写入或 Core request 时，按返回 schema/占位符补齐后继续。这种补参不等于重放未知写入或付费提交。权限拒绝、取消、超时、result_unknown 和 SUBMISSION_OUTCOME_UNKNOWN 仍停止，不能用“补参”绕过。
- 手机聊天工具记录的 8 KiB 展示投影可能出现 `[remote content truncated: payload too large]`；它不是完整正文或完整调用记录。Agent 实时工具结果保留全文；手机审阅完整作品应打开任务文件，不从截断的聊天记录猜剩余内容。
- 只读任务可查看当前 CLI 已打开的作品。若项目未打开，先由可写任务打开；只读调用不会启动服务、迁移、补记录或修复投影。写操作始终在业务派发前拒绝。不得通过 finalize、select_image、export_book 或初始化绕过只读权限。

恢复时只重读缺失信息；不要清空作品、删除锁或停止共享 CLI。运行管理见 `ghost_manual({ghost_id:"<当前插件 id>",path:"runtime/MANUAL.md"})`。
