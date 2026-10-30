# 本地文本创作 MCP 工具

运行 `lumi mcp [--environment production|development] [--data-dir PATH] [--timeout 90s]`。stdin/stdout 只传 MCP JSON-RPC，诊断写 stderr。`--timeout` 是每次工具调用的实例启动及执行总时限，不限制整个 MCP 会话寿命。

进程启动时固定并规范化 cwd。工具没有项目路径参数，也不读取 MCP roots 或宿主 workspace 环境变量。首次调用自动通过 `cli-{environment}.json` 发现并验证 CLI 实例，必要时启动 `serve`。所有业务由 CLI 实例执行，通过项目 Manager 持锁访问 SQLite；多个 stdio 连接共享该实例。不会连接桌面实例，不需要凭证、grant 或 OAuth，转发请求没有 Origin。HTTP 的 `If-Match` 只标记实例代次，用于防止向已替换的实例发请求。

cwd 不是项目时返回 `project_not_initialized`，调用 `project_init` 后可在同一进程继续。已有桌面项目必须先在桌面版关闭；`project_locked` 的 details 包含释放方法。会话退出不会停止共享 CLI 实例，空闲回收沿用 Step 1。

## 共同约定

每个工具同时返回 MCP text 和 structured content，内容均为以下信封；失败时另有 `isError=true`：

```json
{"success":true,"data":{"uuid":"...","revision":1}}
```

```json
{"success":false,"data":null,"error":{"code":"production_conflict","message":"页面结构已被修改","details":"重新读取后重试。"}}
```

所有资源标识是公开 UUIDv7，字段使用 `snake_case`，不返回内部数据库 ID。单对象直接放在 data。写入成功返回资源摘要（UUID、revision、标题等），省略正文、画面描述、设定 summary 等长字段；要核对内容，调用对应 get。

输入和结果各最多 256 KiB；文本参数的 JSON Schema 上限为 120000 字符，同时受领域字段限制。较大内容拆成章节或页面；单次 get 仍超限时使用导出。列表参数为 `before` / `after` / `limit`：游标是返回的字符串，before 与 after 互斥，limit 默认 20、范围 1–100。返回：

```json
{"items":[],"cursor_pagination":{"per_page":20,"next_cursor":"20","prev_cursor":"0","has_more":true}}
```

游标基于当前排序的位置，不冻结列表快照；并发增删后应重新读取。`book_outline` 返回轻量结构，适合先定位 UUID，再 get 单个资源。

除了幂等初始化，所有内容写入必须显式提供刚读取的 `expected_revision`，包括合法的 0。不同资源的版本不可混用：

| 写入 | expected_revision 来源 |
| --- | --- |
| setup update/finalize | `project_setup get` 的 revision |
| Story Profile write | `story_profile get` 的 revision |
| 创建章节 | `project_overview.project.revision`；成功后项目 revision 增加 |
| 修改标题、正文、删除章节 | 章节 revision |
| 创建设定 | `settings list.premise.revision`；成功后 premise revision 增加 |
| 修改、删除设定 | 设定 revision |
| 创建页面 | `pages list.state.revision`；页面结构或内容变化都会更新此版本 |
| 修改、删除页面 | 页面 revision |

版本冲突时重读并重新判断；不要把旧模型输出直接换上新 revision 再覆盖。工具调用中断时写入可能已经提交，先重读确认结果。

## 工具清单

参数表中的 `?` 表示可选；未列出的字段由 JSON Schema 拒绝。完整可机读约束由 MCP `tools/list` 提供。

### project_init

参数：`idea`（非空原始创意），`name?`（默认目录名），`generation_language?`（`zh-Hans` / `en`，默认 `zh-Hans`）。

在 cwd 原地创建标准项目，并保留已有非受管文件。新项目进入 setup draft；受管路径冲突时不覆盖。对有效已有项目幂等打开，忽略新名称和创意，不重置数据。与 Step 2 的 `lumi init` 保持兼容：后者建立的 ready 项目可以直接编辑。

返回：`{project, setup, setting_count, page_count}`。project 包含 `uuid/name/revision/generation_language/setup_status/picture_book/chapter_count`；setup 包含 `uuid/project_uuid/status/setup_status/revision/draft_values/missing_information` 等。

### project_overview

参数：`{}`。返回与初始化相同的概况，含 setup 原始输入。打开 cwd 项目并获取请求租约。

### project_setup

| action | 参数 | 返回 |
| --- | --- | --- |
| `get` | 无 | setup 完整状态 |
| `update` | `expected_revision`，以及至少一个设置字段 | 更新后的 setup 摘要 |
| `finalize` | `expected_revision` | 定稿后的 setup 摘要 |

设置字段：`project_name?`、`generation_language?`、`overall_style?`、`generation_brief?`、`picture_book?`。前三项和 brief 为最终设置；完成所有缺失字段后再 finalize。文本长度沿用项目约束：名称 120、风格 12000、brief 4000 字符。

`picture_book` 包含：

- `format`：`classic_picture_book`、`wordless_picture_book`、`interactive_picture_book`、`comic_story` 或 `vertical_strip`。
- `aspect_ratio?`：`{mode,width?,height?}`；mode 为 `landscape/square/portrait/custom`，自定义比例才传 width/height。
- `large_image_minimal_text?`：经典绘本的布尔设置。
- `interaction_mode?`：`find_it/make_a_choice/guess/follow_along`。
- `comic_layout?`：`four_panel/page_comic`。

update 固定 `generation_scope=project_only`，finalize 直接使用现有设置持久化逻辑并初始化提示词目录，不解析服务商、不启动生成工作流。绘本规格定稿后保持现有不可变约束。

### story_profile

`{action:"get"}` 读取标准 Story Profile（包括 `uuid/revision/story_md/projection_state`）。

`{action:"write",story_md,expected_revision}` 写完整 Story Profile，并同步 `STORY.md`。文件投影冲突和保存失败沿用现有错误；SQLite 始终保存事实版本。

### chapters

| action | 参数 |
| --- | --- |
| `list` | 分页参数可选；返回 active 章节 |
| `get` | `chapter_uuid` |
| `create` | `chapter_code,title,expected_revision`，`content?,content_format?` |
| `update` | `chapter_uuid,title,expected_revision` |
| `write` | `chapter_uuid,content,expected_revision`，`content_format?` |
| `delete` | `chapter_uuid,expected_revision` |

chapter_code 如 `vol01.ch01`，项目内 active 编号唯一。content_format 为 `txt/md`，默认 txt。章节返回 `uuid/chapter_code/title/revision/current_story`；current_story 含不可变正文版本的 UUID、content、content_format。write 增加正文历史版本，delete 放入回收站。

### settings

| action | 参数 |
| --- | --- |
| `list` | 分页参数可选；返回 `{items,cursor_pagination,premise:{uuid,revision,default_style}}` |
| `get` | `setting_uuid` |
| `create` | `asset_type,title,summary,expected_revision`，`tags?` |
| `update` | `setting_uuid,expected_revision`，至少一个 `asset_type/title/summary/tags` |
| `delete` | `setting_uuid,expected_revision` |

asset_type 为 `character/scene/prop/reference`。title 最多 160 字符，summary 最多 12000 字符；tags 为字符串数组。纯文本创建时 `current_variant=null`，不要求图片或服务商。返回公开 Premise Asset，含 `uuid/asset_type/title/summary/tags/revision/current_variant`。delete 放入回收站。

### pages

所有 action 都需要 `chapter_uuid`。

| action | 其他参数 |
| --- | --- |
| `list` | 分页参数可选；返回 `{items,cursor_pagination,state}`，state 包含页面集合 UUID 和 revision |
| `get` | `page_uuid` |
| `create` | `title,description_md,storyboard_md,expected_revision`，`page_role?` |
| `update` | `page_uuid,expected_revision`，至少一个 `title/description_md/storyboard_md/page_role` |
| `delete` | `page_uuid,expected_revision` |

description_md 保存页面正文、对白或旁白；storyboard_md 保存完整画面段落，包括构图、动作、表情、光线及逐字画面文字。update 在同一事务内修改页面字段和追加 storyboard 版本；冲突时全部回滚。返回 `uuid/chapter_uuid/section_no/page_role/title/description_md/revision/current_storyboard/current_image` 等，画面段落在 `current_storyboard.content_md`。

page_role 默认 `body`，也支持 `front_cover/back_cover`。封面、封底唯一性及至少保留一页正文等规则沿用现有绘本约束；删除为软删除并重新规范页码。

### book_outline

参数：分页参数可选。整本 active 章节与页面按阅读顺序展平。章节项包含 `kind:"chapter",uuid,chapter_code,title,revision,has_text`；页面项包含 `kind:"page",uuid,chapter_uuid,section_no,page_role,title,revision,has_text,has_storyboard,has_image`。

### prompt_catalog

参数：`prompt_group?` 与分页参数。group 支持 `story/chapter/premise/premise_style/agent/runtime`。

返回有效目录元数据：`prompt_group/prompt_key/title/description/prompt_type/placeholders/is_custom`。读取项目覆盖和当前绘本格式，提示词正文通过 render_prompt 获取，不在 skill 中复制。

### render_prompt

必填：`prompt_group,prompt_key,target`。可选：`variables`（string 到 string 的任务输入）、`chapter_uuid`、`page_uuid`、`setting_uuid`、`context_chapter_uuids`、`context_setting_uuids`。

| target | 目标参数 | 写回工具 / action |
| --- | --- | --- |
| `setup` | 无（要求项目有 setup 草稿） | project_setup / update |
| `story_profile` | 无 | story_profile / write |
| `chapter` | chapter_uuid | chapters / write |
| `pages` | chapter_uuid | pages / create，目标版本为页面集合版本 |
| `page` | chapter_uuid、page_uuid | pages / update |
| `setting` | setting_uuid | settings / update |
| `premise` | 无 | settings / create，目标版本为 premise 版本 |

返回 `{project_uuid,target_uuid,expected_revision,prompt_group,prompt_key,prompt,write_back}`。prompt 自带有效目录模板、语言和 JSON system 约束、完整任务输入、项目上下文、目标 UUID 与版本及写回契约，可单独交给执行者，无需父对话历史。输出格式要求来自原模板；工具只渲染，不调用模型。调用者将 JSON 中的文本字段映射到对应写入工具，不会自动执行章节计划或覆盖全部页面。

自动填入真实 `story_md/chapters_json/project_context_json/premise_context_json/style_name/style_prompt`；章节目标另有 `chapter_code/chapter_context_json/input_text`，单页目标可填 `storyboard`。任务专属占位符（例如 chapter_count、chapter_plan_json、generated_summaries_json、moment_count_plan）放在 variables 中。缺少输入返回 `prompt_inputs_missing`，不会默默编造。项目事实字段不可通过 variables 覆盖。

上下文默认包含全部 active 章节和设定。较大项目可用两个 context UUID 数组选择任务必需的资源；空数组排除对应集合，目标对象仍单独保留。模板内的源文本或目标自身超限时，先拆分创作任务。

示例：

```json
{"prompt_group":"story","prompt_key":"story_profile","target":"story_profile","variables":{"chapter_count":"1"},"context_chapter_uuids":[]}
```

常用文本模板：`story/story_profile`、`story/story_chapter`、`story/profile_from_chapters`、`chapter/comic_storyboard`、`chapter/cover_storyboard`。后者按项目绘本格式自动解析，包含完整页面和输出 JSON 约束。

### export_book

必填：`format`（`markdown/zip/pdf`）、`allow_missing_images`（布尔）。返回 `{uuid,format,path,byte_size,missing_image_count}`，path 为项目 `.lumi/exports/` 下的绝对路径；不接受任意导出路径。

- Markdown：setup、Story Profile、设定源文、画风、全部 active 设定、章节正文、页面正文及画面描述。
- ZIP：`book.md`、使用公开 UUID 的 `book.json` 和现有页面图片（`images/<chapter_code>/...`）。没有任何图片也可导出全部文本。
- PDF：复用现有图片快照与 PDF 排版；至少要有一张可用正文图片。`allow_missing_images=true` 允许跳过缺图页面，PDF 本身不排版创作文本；全部文本使用 Markdown/ZIP 交付。

至少建立一个页面后导出。false 时有缺图返回 `comic_export_incomplete`。文本导出为本地交付文件，不创建服务商任务，也不加入旧的异步导出任务列表。

## 错误码

| code | 含义与处理 |
| --- | --- |
| `invalid_arguments` | JSON Schema、必填字段、UUID、action、版本类型或字段组合无效；修正参数 |
| `arguments_too_large` / `result_too_large` | 超过 256 KiB；拆分、分页、缩小提示词上下文或导出 |
| `project_not_initialized` | cwd 未初始化；调用 project_init 后继续 |
| `project_initialization_conflict` | 受管路径已有冲突文件；先处理冲突，不会覆盖 |
| `project_locked` | 其他 Lumi 实例占用；按 details 在桌面版关闭或用相应 CLI stop 释放 |
| `instance_unavailable` / `cli_instance_changed` | CLI 无法启动/访问，或发现代次已变化；重读后重试 |
| `cli_upgrade_required` | 运行中的 CLI 版本没有创作接口；同 environment/data-dir 执行 lumi stop 后重启 |
| `project_setup_incomplete` / `invalid_project_setup` | 设置未定稿或缺字段；先完成 setup |
| `project_setup_revision_conflict` / `picture_book_profile_immutable` | setup 过期或正式规格已定稿 |
| `project_revision_conflict` / `chapter_revision_conflict` / `story_profile_revision_conflict` / `production_conflict` | 项目、章节、Profile、设定或页面版本过期；重读并重新规划 |
| `chapter_conflict` / `chapter_state_conflict` / `production_state_conflict` | 编号或资源状态冲突；按错误详情修正 |
| `chapter_not_found` / `production_resource_not_found` | UUID 不属于目标资源或已经删除 |
| `story_md_conflict` / `story_projection_failed` | STORY.md 外部修改或投影失败；保留草稿，按既有 UI 的文件冲突流程处理 |
| `prompt_inputs_missing` | 模板占位符未提供；查 prompt_catalog 并补 variables |
| `comic_export_empty` / `comic_export_incomplete` / `comic_export_unavailable` | 无页面、PDF 无正文图、缺图或图片不可用 |
| `validation_failed` / `production_validation_failed` / `invalid_picture_book_profile` | 领域内容或绘本规格无效；按 details 修正 |
| `project_initializing` | 内置初始化工作流仍占用编辑权；等待完成或通过现有 UI 接管 |
| `internal_error` | 本地存储等未预期失败；检查 CLI 日志 |

路径、项目格式和迁移失败还会保留既有项目错误码，如 `invalid_project_path/project_permission_denied/invalid_project/project_format_too_new/project_migration_failed`。

## 实时同步与验证

写入使用现有 `story:*`、`premise:*`、`comic:*` 和 `project:setup_changed` 变更提示，通过 `/api/v1/ws` 的 `topic/event/payload/ref/join_ref` 信封发送。已打开 UI 沿用 TanStack Query 失效与 REST 重读，并保留首次 join、重新 join、窗口聚焦校准；没有新增 HTTP 轮询。

`cmd/lumi/mcp_test.go` 使用 go-sdk CommandTransport 客户端执行真实 stdio → CLI 实例 → SQLite 链路：空目录初始化、无服务商 setup、渲染提示词、固定文本写回、三页正文和画面、导出并检查文件内容；同时测试双连接版本冲突、UI WebSocket/REST、桌面占锁和释放。结束后检查无服务商配置及零 LLM/生成任务记录。`internal/textmcp` 另测 schema、提示词覆盖、分页、大小限制与缩小范围恢复；`internal/production` 验证已有图片可进入文本 ZIP，以及复用 PDF 渲染。
