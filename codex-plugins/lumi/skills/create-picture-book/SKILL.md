---
name: create-picture-book
description: 在 macOS Codex App 中用 Lumi 从想法创作、修改、审阅和导出本地绘本，包括故事、设定、分页及宿主生图。用于新建绘本或续做当前 Lumi 项目。
---

# Lumi 绘本创作

写作、推理和图片生成使用 Codex 的能力与额度；Lumi 负责项目事实、版本、图片挂载、审阅和导出。无需 API key，不配置 Lumi 模型服务商。本插件支持 macOS Codex App；不支持 TUI 或 IDE 扩展。

## 打开或创建项目

先调用 `project_overview` 确认当前会话目录。stdio MCP 在启动时绑定 cwd，没有切换项目路径的参数；不能靠 shell `cd` 改变已有连接。目录不对时让用户在目标目录新开 App 会话。

- 返回 `project_not_initialized` 且用户要求创建时，用原始创意调用 `project_init`，得到 setup 草稿。不要先创建一个默认 ready 项目再覆盖用户规格。
- `project_init` 成功后，在同一目录通过安装位置的绝对路径运行 `lumi init --json`，幂等补齐 Step 2 的 AGENTS 和保护配置；对已经存在的项目不会重置 setup 草稿、内容或规格。然后 `lumi project info --json` 核对配置结果；已有项目缺少保护指引时也走这条路径。不要假定 GUI 继承 PATH；默认 CLI 是 `~/.local/share/lumi-codex/current/bin/lumi`，自定义安装取安装器输出。
- 没有插件工具而用户明确只要求 CLI 初始化时，先 `lumi project info --json` 确认目录；仅在未初始化且用户希望在此建立项目时运行 `lumi init --name 名称 --language zh-Hans --json`。CLI 在全新目录创建 ready 项目，先解释它采用的默认规格；需要自选绘本规格时等待插件可用，使用 `project_init → lumi init（补指引）→ project_setup`。
- 已有项目读取概况、`project_setup get`、`book_outline`、相关章节和设定，保留现有进度。`project_locked` 时按错误详情释放桌面版或其他 CLI 占用，不删锁文件。
- 检查 `lumi init` 返回的 guidance 和 warnings；配置冲突时保留现有设置，按返回的合并建议处理。提示用户在 App 信任项目并新开会话使配置生效；不能替用户设置 trust 或宣称写保护已生效。当前 MCP 会话可以继续创作，信任提示不替代用户的连续执行授权；用户/session 权限覆盖可能影响实际保护。

只通过 Lumi CLI / MCP 访问项目事实。**禁止直接读取或写入 `project.sqlite`（含 WAL/SHM 等）、`.lumi/`、`assets/`，也不得让 subagent 这样做。** 不用文件名推测图片。可向用户链接 `export_book` 返回的准确导出路径；这是工具授权的交付结果，无需自己打开内部目录。`STORY.md` 的标准写回使用 `story_profile write`。

## 创作与写回

遵循用户已经给出的语言、年龄、页数、风格、审阅和交付要求。没指定的可逆细节做合理假设并说明；绘本格式与比例定稿后不可变，若用户未指定也未授权自行决定，在 setup 定稿前确认。

1. **想法 → setup**：读 `project_setup get`，根据用户意图和缺失字段整理设置。`project_setup update` 填名称、语言、画风、创作要求和绘本规格；以返回的新 revision 调 `finalize`。当前目录没有专门的 setup 模板，不猜造模板名，也不触发 Lumi 内部生成 pipeline。
2. **故事**：用 `render_prompt`（常用 `story/story_profile`）生成 Story Profile 与章节计划；`story_profile write` 保存 profile，`chapters create/write` 保存每章故事。渲染提示词不等于已写入，章节计划也不会自动执行。
3. **设定**：从已保存故事提取角色、场景、道具和参考，用 `prompt_catalog` 查实际 premise 模板，再 `render_prompt(target=premise)`。逐项 `settings create` 保存文字特征、色彩、服饰和画风，先为重复出现的角色/场景制作设定图。
4. **分页与画面描述**：按章节使用 `render_prompt`（常用 `chapter/comic_storyboard`，封面用目录中的对应模板）规划阅读节奏、正文和完整画面。`pages create` 中 `description_md` 是页面正文，`storyboard_md` 是完整画面描述，含构图、动作、表情、光线和逐字画面文字；封面/封底传正确 `page_role`。不要把整个输出 JSON 当作正文保存。
5. **生图与挂载**：按下节逐张生成并立即导入。按用户要求完成整本，不只生成示范页。
6. **审阅与导出**：按约定用 `review_settings`、`review_page`、`review_book` 展示；修正后 `export_book`。PDF 基于页面图片，文字要出现在图片中才会进入 PDF；同时用 Markdown 或 ZIP 交付全部可编辑文字。完整绘本交付用 `allow_missing_images=false`。缺页 PDF 仅当用户接受预览时设 true 并明确遗漏，PDF 至少需要一张正文图；中断时可用 true 导出 Markdown/ZIP 保存全部文字与已有图片。

### 应当启动 subagent 的环节

完整新书创作中，**应当启动 subagent** 执行 Story Profile / 章节草稿、设定提取、每章分页与画面描述这些文本任务。只有输入事实已经写入后才能开始下游任务；可并行处理独立章节的草稿。小范围改字无需委派。

父代理先查 `prompt_catalog`，按实际模板及必填占位符调用 `render_prompt`；把返回的完整 `prompt`、`project_uuid`、`target_uuid`、`expected_revision`、`write_back` 和用户约束交给 subagent。优先使用不继承父历史的任务输入，避免依赖未传递的对话。要求 subagent 按模板输出结构化草稿，**不要写项目、不要生图、不要启动 Lumi provider pipeline**。subagent 的共享文件系统不是隔离边界。

父代理核对输出与用户要求、渲染时的目标和 revision，再按 `write_back` 映射到 MCP 写工具，串行写回。下一次创建章节前重读 `project_overview.project.revision`，创建设定前重读 `settings list.premise.revision`，创建页面前重读 `pages list.state.revision`。创建返回的对象 revision 不等于父级集合 revision，也不能给整批复用旧 revision。版本冲突先重读事实并检查草稿是否仍适用，不只把 revision 换新强行覆盖。

用户要求不使用 subagent、宿主没有委派工具或额度不足时，**走串行路径**：父代理自己执行同一个 `render_prompt` 任务并核对写回。告知采用串行方式，不因缺少 subagent 停止可完成的创作。缺占位符时根据工具错误和 `prompt_catalog` 补齐；不要猜模板名或覆盖工具自动填入的项目事实。

## 宿主生图与一致性

每张图先 `image_target(target_uuid, slot)`，保存 `expected_revision`，读取 `target_ratio`、`recommended_ratio`、`generation_hint` 和 `suggested_fit`。提示词写出目标比例及宿主推荐比例，让主体、文字和装订边留出安全区；推荐值是比例，不是像素保证。

需要角色/场景一致性时，先 `settings get` 取得 `current_variant.asset.uuid`，再 `read_image(asset_uuid)` 把相关设定图实际放进当前对话。可补充上一页 `current_image.asset.uuid` 的参考。紧接着调用 App 原生生图，以最近图片为参考（如 `num_last_images_to_include`），确认最近几张确实是本页所需图，遵守宿主参考图数量上限；跨会话/压缩后重新读图，不把 UUID 当成生图工具可见的图像。不要直接读取资产目录来传路径。

**生成完成后立即 `import_image`**：`path` 必须取宿主本次返回的真实绝对路径（也可能在 output hint 文本里），传目标 UUID、slot、revision、原始 prompt。不能假定有 `savedPath` 字段，不能猜目录或扫描生成文件。宿主没提供可访问路径时说明缺失，不能伪造导入成功。

默认先用 `fit_mode=keep` 保留原图；已约定居中裁切/补边时用 `crop/pad`。查看导入返回的 `ratio_matches`、`ratio_deviation`、`warnings`：偏差不阻塞导入；页面适合在主体安全时 crop，设定或边缘文字宜 pad，必要时下一张调整构图。不要仅因像素不精确反复重生。处理后的图有独立版本，原图保留。

## 审阅与提问

用户提示词优先：明确要求直接做完/自行决定时连续推进；要求逐步确认时遵守其节点。**未指定时默认两个停顿点**：setup 定稿前汇总读者、格式、比例、页数与画风；完成故事、设定及一张代表性页面后展示审阅，再继续整本生图。交付前展示全书审阅，不把额外确认设为导出前置条件。

只在关键信息缺失、不可变规格、意图冲突、项目目录/占用阻塞或无法合理代决的内容选择时提问。已有授权不要重复索取；宿主批准按实际请求处理。MCP Apps 不可见时使用完整文本及工具返回的 Lumi 链接/命令，不把界面加载失败当作内容丢失。`review_candidates` 切换只是本地预览；用户回复编号后重读，核对候选 UUID，再用 `pages/settings select_image` 和最新 revision 持久化。

## 额度耗尽、中断与续做

每段文本写完、每张图生成后及时提交。调用中断可能已经提交，先 get 核实，避免重复创建和重复导入。额度不足时停止反复重试，报告已经完成的章节/页 UUID、缺图与下一步；还能调用工具时导出 Markdown/ZIP，缺图用 true 并标注。不要自动切换到付费 API，也不要求 API key。

恢复时从 `project_overview → project_setup get → book_outline` 开始，分页读取有需要的章节/设定/页面；以当前工具事实判断缺失环节。重新取 revision、重新渲染剩余任务和读取参考图，只补缺失或用户指定修改的内容。旧生图路径失效且未导入时再生成，不扫描宿主目录寻找替代文件。
