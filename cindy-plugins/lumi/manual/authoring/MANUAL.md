# Lumi 文本作品创作

此流程由当前 Cindy Agent 串行执行，不需要多 Agent，也不配置 Lumi 模型服务商。项目事实保存在标准 Lumi 项目中，所有业务读写均通过工具完成；不要直接读写 SQLite、素材目录或 STORY.md 投影。

## 发现与项目识别

先调用 `ghost_call({ghost_id:"lumi",tool:"list_tools",args:{}})` 查看类别，再按任务展开，例如 `ghost_call({ghost_id:"lumi",tool:"list_tools",args:{category:"project"}})`。类别结果包含实时 schema 与 RULES，后续均通过 `ghost_call({ghost_id:"lumi",tool:"call_tool",args:{name:"工具名",args:{业务参数}}})` 调用。

使用 Cindy 为当前任务选择的本地作品根目录。先 project_overview；只有返回 project_not_initialized，且用户已委托在此创建作品时，才以用户原始创意调用 project_init。有效项目重复 init 不重置内容。目录与锁问题见 `ghost_manual({ghost_id:"lumi",path:"project/MANUAL.md"})`。

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
5. media 与 review 类别当前未实现，不调用占位工具，不用任意路径导入图片，也不调用 MCP App-only 工具。文字正文和画面描述可以先完成。

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
- 手机聊天工具记录的 8 KiB 展示投影可能出现 `[remote content truncated: payload too large]`；它不是完整正文或完整调用记录。Agent 实时工具结果保留全文；手机审阅完整作品应打开任务文件，不从截断的聊天记录猜剩余内容。
- 只读任务可查看当前 CLI 已打开的作品。若项目未打开，先由可写任务打开；只读调用不会启动服务、迁移、补记录或修复投影。写操作始终在业务派发前拒绝。不得通过 finalize、select_image、export_book 或初始化绕过只读权限。

恢复时只重读缺失信息；不要清空作品、删除锁或停止共享 CLI。运行管理见 `ghost_manual({ghost_id:"lumi",path:"runtime/MANUAL.md"})`。
