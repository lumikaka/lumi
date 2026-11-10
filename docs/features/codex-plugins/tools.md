# 本地创作 MCP 工具

运行 `lumi mcp [--host claude|codex|cindy] [--environment production|development] [--data-dir PATH] [--timeout 90s]`。stdin/stdout 只传 MCP JSON-RPC，诊断写 stderr。`--timeout` 是每次工具调用的实例启动及执行总时限，不限制整个 MCP 会话寿命。

进程启动时固定并规范化 cwd。工具没有项目路径参数，也不读取 MCP roots 或宿主 workspace 环境变量。首次调用自动通过 `cli-{environment}.json` 发现并验证 CLI 实例，必要时启动 `serve`。所有业务由 CLI 实例执行，通过项目 Manager 持锁访问 SQLite；多个 stdio 连接共享该实例。不会连接桌面实例，不需要凭证、grant 或 OAuth，转发请求没有 Origin。HTTP 的 `If-Match` 只标记实例代次，用于防止向已替换的实例发请求。

`--host` 标识宿主并随每次调用转发给 CLI 实例；未知值启动时报错，未传时工具清单与行为和改动前一致。`codex`、`cindy` 与未传时的清单相同。`claude` 去掉 MCP Apps 专用工具（`read_app_*`、`write_app_resource`、`watch_app_changes`）与 `review_*`，增加 [`web_url`](#web_url) 与 [`generate_page_image`](#generate_page_imageclaude-宿主)。

cwd 不是项目时返回 `project_not_initialized`，调用 `project_init` 后可在同一进程继续。已有桌面项目必须先在桌面版关闭；`project_locked` 的 details 包含释放方法。会话退出不会停止共享 CLI 实例，空闲回收沿用 Step 1。

## 共同约定

每个工具同时返回 MCP text 和 structured content，内容均为以下信封；`read_image` 成功时另带 MCP image content，UI 专用读取的完整数据仅放在 `_meta["lumi/app_response"]`（text 和 structured content 仅保留有界元数据），失败时另有 `isError=true`：

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
| 导入页面 / 设定图片 | 页面用 `prepare_page_image.image_write_back` 及原样 preparation；设定用 image_target 或对应设定 revision |

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

### review_book / review_chapters / review_chapter / review_page / review_candidates / review_settings

review_* 只打开当前 stdio cwd 绑定项目的工作区，不接收任意项目、路径或 URL。六个工具共用 `ui://lumi/review/<内容哈希>.html`，启动调用本身只读；页面的显式提交复用现有业务权限、revision 和确认流程。

| 工具 | 参数 | 共享页面与 Web 入口 |
| --- | --- | --- |
| `review_book` | 可保留历史分页参数 | 作品简介、规格、进度和章节/设定入口；`/projects/:project_uuid/` |
| `review_chapters` | 可保留历史分页参数 | 业务顺序章节列表、摘要、页数和图片完成数；`/projects/:project_uuid/chapters` |
| `review_chapter` | `chapter_uuid` | 共享逐页编辑器，默认选中第一页，可在章内切页；`/projects/:project_uuid/chapters/:chapter_uuid` |
| `review_page` | `chapter_uuid,page_uuid` | 当前图、页面内容与说明、版本、章内切换；章节的 `/sections/:page_uuid` |
| `review_settings` | `setting_uuid?` | 设定列表或详情；`/premise` 或 `/premise/assets/:setting_uuid` |
| `review_candidates` | `kind:"page"` + 章/页，`kind:"setting"` + 设定，或 `kind:"text"` + 2–12 项 `{title,text}` | 共享详情候选弹窗，或本地文字候选；预览不保存，显式采用才持久化 |

启动描述 `contract_version:3` 包含 `view,project_uuid,target_uuid,revision,title,summary,state,open_url,open_command,request,permissions,data_source`。摘要最多 300 字；完整正文、分镜、长列表、图片和全部历史不放入启动结果。文本候选正文在 `_meta["lumi/launch_arguments"]`，模型结果仅带 candidate_count。模型要阅读正文时仍用 `chapters/pages/settings` 的明确读取。

App 使用 MemoryRouter，实际导入 `SimpleProjectPages.jsx` 的同一套主体、表单和弹窗。Web 保留外壳，MCP 只装配路由与宿主适配，保留章内 section 栏。普通导航、翻页、编辑、保存、排序、回收站和版本操作不需要聊天消息或新增模型推理。点击候选图片仅预览；“使用此稿”复用版本选择接口。对话回复编号时用 `pages/settings get include_versions:true` 显式读取 `image_versions` 分页身份清单，按 version_no 匹配 variant UUID，再带目标最新 revision 调用 `pages/settings select_image`。默认 get 不附全部历史，身份清单不含图片或历史输入快照。

### read_app_resource / write_app_resource

App-only 资源适配器使用项目相对资源名（如 `chapters/<uuid>/comic-sections/<uuid>`），服务端注入绑定项目 UUID，经同一 Echo router、中间件、REST handler 和 SQLite。没有任意地址、项目参数、文件路径或请求头代理。只允许列明的业务资源与 HTTP 方法；拒绝跨项目资源、内部 id、永久清除和生成接口。read_app_resource 只读；write_app_resource 仅用户提交时调用，受 setup、项目权限和 revision 校验。

响应均为标准业务信封，放在 `_meta["lumi/app_response"]`；content 和 structuredContent 仅含 `{success:true,data:{state:"available",channel:"_meta"}}` 或错误摘要。完整 App 响应上限 8 MiB，不进入模型文本。不能仅凭 app-only 断言真实宿主不会把 `_meta` 放入模型上下文，当前实际验证边界见 [共享工作区验收](embedded-workspace.md)。宿主未转交 `_meta` 时给出错误和 Lumi 回退，不降级为重复发送大正文。

卡片及章内导航使用现有 REST 的 `?view=summary`，返回 `{items,total,snapshot,cursor_pagination}`，limit 1–100，before/after 互斥且校验范围；summary 不含完整正文与图片输入快照。页面从首组读完整集合，检查快照、数量和游标前进；当前页详情单独读取，历史在展开时读取。数据变化、删除和失败可重试，快速切换按目标 UUID 隔离请求、缓存与编辑器。

文件导入在工具参数中仅传文件元数据，字节通过请求 `_meta["lumi/upload"].data_base64` 传递；仅 asset-uploads POST 接受附件，最大 6 MiB，服务端转换为已有 multipart 上传流程，不接受本地路径。导出弹窗保留现有确认与 readiness，下载由宿主打开同一绑定项目的 Lumi 下载路径。AI 生成、聊天引用、系统配置使用同资源 Web 替代入口，界面不自动生图或切换渠道。

`read_app_review` 保留旧详情 DTO 的 App-only 兼容读取；正常工作区使用资源读取。新 schema 不提供 `mode`、reader 或作品工具的章节参数。旧 `review_book({chapter_uuid})`、旧 `mode:"preview"` 调用与历史 book/reader 快照都映射到相同范围的正常业务页面；指定章返回规范的 `review_chapter` 请求、章节身份及章节 URL，整本返回作品总览。旧 `/preview` URL 仅重定向到作品或章节，不渲染独立预览。

创作或导出完成后只打开与交付范围对应的页面：整本 `review_book()`，指定章 `review_chapter(chapter_uuid)`，单页（包括仅第一页）`review_page(chapter_uuid,page_uuid)`。章节列表使用 `review_chapters()`。不追加另一轮整本阅读。

仅“在 Lumi 中打开”或明确下载交给宿主打开实际运行端口，普通导航留在 App。WebSocket 提示触发 Query 失效并经 REST 重读 SQLite；首次 join、重新 join、窗口聚焦校准，无定时 HTTP 轮询。项目 GET 响应的 open_url 提供当前 REST 服务 origin，重连后相同资源链接随运行时端口校准。审阅入口不增加创作或导出的确认节点；UI 失败仍交付摘要、路径和真实 Lumi 入口。

### read_app_image

App-only、只读；参数 `asset_uuid` 和可选 `max_edge`（256–1024，默认 1024，缩略图请求 384）。项目边界、图片解码与 JPEG 缩小复用 `read_image`。完整图片信封位于 `_meta["lumi/app_response"]`，包含 asset_uuid、mime_type、width、height、data_uri；text/structuredContent 只有元数据，没有 Base64，不额外附 image content。JPEG ≤120 KiB，完整图片工具结果 ≤256 KiB。

图片通过宿主桥接，不直连 REST、文件或 localhost。前端最多 3 路并发、32 项 / 6 MiB LRU，取消排队和丢弃旧目标结果。历史图片结果的 structuredContent 兼容解码仍保留，新的主链路要求 UI `_meta`。模型参考图仍使用原有 read_image。

### watch_app_changes

App-only、只读，`action` 为 `open/next/close`；每次都传 cwd 绑定的 `project_uuid`。此工具只转发 WebSocket 提示，不读取业务状态、不写入项目。

- `open`：stdio 读取内部连接描述、连接 CLI `/api/v1/ws` 并完成项目 join，返回 `{project_uuid,subscription_uuid,cursor,state:"joined"}`。`subscription_uuid` 是公开 UUIDv7，绑定当前 MCP 会话与项目。连接描述、地址与临时票据不交给 iframe，内部 `read_app_connection` 不注册为 MCP 工具。
- `next`：另传 `subscription_uuid,cursor`，阻塞等待同一 WS 的真实变更或 ping。通过普通工具成功信封返回 `{project_uuid,subscription_uuid,cursor,state,event?}`；state 为 `changed/heartbeat`。界面收到结果后接续下一次事件等待，没有按定时器读取业务状态。只在 `changed` 时使 Query 失效；heartbeat 不触发 REST。保留游标及最新变更，即使事件发生在两次等待之间，后来的心跳也不会掩盖它。
- `close`：另传 `subscription_uuid`，释放连接，返回 `state:"closed"`。View teardown 显式关闭并取消等待；MCP 会话结束、两分钟未续接等待、CLI 断开或票据过期也释放资源。每进程最多 16 个订阅，每个订阅最多一个等待者。

服务器丢弃原始 WS payload，只转发公开项目 UUID、订阅 UUID、事件名和不涉及数据库 ID 的游标。临时票据仍限制单个项目、一小时有效，CLI 重启失效；不能加入 system 或其他项目。普通 web UI 与桌面的 Origin 校验保持原状。资源的三个 CSP 域列表均为空，HTML 为 `connect-src 'none'; img-src data:`。

首次 join、重新 join、变更提示、窗口聚焦或恢复可见时使 TanStack Query 失效，经只读工具→CLI REST 重读 SQLite。确认 CLI 订阅丢失后有限次数重连；宿主拒绝、超时或协议异常保留手动重连、正文与 Lumi 入口，不循环查询。0.5.3 的 progress 转发已在真实宿主观察到界面中断，0.5.4 改用普通工具结果；**新事件等待链路仍须真实宿主验收**，见 [mcp-apps.md](mcp-apps.md)。

### book_outline

按章传 `chapter_uuid` 时只返回该章页面。页面附 `variant_uuid`（有图时）和 `review_status`（missing_image / pending / needs_revision / passed）。`pages list(summary_only=true)` 同时返回集合 revision。`pages get(summary_only=true)` 返回完整核对观察。修订同一图版的既有核对意见时传 `expected_review_revision`（摘要中的 `review_revision`）。实际看图后用 `pages record_image_review(chapter_uuid,page_uuid,variant_uuid,expected_revision,verdict,observation)` 记录当前版本，换图或编辑页面后重新核对。

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

自动填入真实 `story_md/chapters_json/project_context_json/premise_context_json/style_name/style_prompt`；章节目标另有 `chapter_code/chapter_context_json/input_text`，单页目标可填 `storyboard`。任务专属占位符（例如 chapter_count、chapter_plan_json、generated_summaries_json、moment_count_plan）放在 variables 中。竖向条漫的 `pages + chapter/comic_storyboard` 支持 `moment_count_plan="auto"`，同时传 `max_section_count`（1–24 的整数字符串）；服务端按章生成稳定的逐页 1–3 镜头计划并填入单页上限 3。显式逐页计划按原值渲染。缺少输入返回 `prompt_inputs_missing`，不会默默编造。项目事实字段不可通过 variables 覆盖。

上下文默认包含全部 active 章节和设定。较大项目可用两个 context UUID 数组选择任务必需的资源；空数组排除对应集合，目标对象仍单独保留。模板内的源文本或目标自身超限时，先拆分创作任务。

示例：

```json
{"prompt_group":"story","prompt_key":"story_profile","target":"story_profile","variables":{"chapter_count":"1"},"context_chapter_uuids":[]}
```

常用文本模板：`story/story_profile`、`story/story_chapter`、`story/profile_from_chapters`、`chapter/comic_storyboard`、`chapter/cover_storyboard`。后者按项目绘本格式自动解析，包含完整页面和输出 JSON 约束。

### image_target

必填：`target_uuid`、`slot`（`page_image` / `setting_image`）。页面 UUID 可直接从 pages 或 book_outline 取得，不需要 chapter_uuid；设定 UUID 来自 settings。

可选：`setting_uuids`，仅用于 `page_image`，最多 100 个不重复的当前项目 active 设定 UUID，按参考图顺序传入。

返回 `{target_uuid,slot,expected_revision,target_ratio,recommended_ratio,generation_hint,suggested_fit,ratio_tolerance}`，页面额外返回 `setting_readiness`。ratio 为 `{width,height}`，不是输出像素要求。

选定设定后可传 `review_source_page_uuid` 取先前导入页面保存的逐项核对。版本、revision 和画风均匹配时返回 `items[].previous_review`，调用方确认参考依据仍相同后可直接复用。

`setting_readiness` 为只读检查，包含 `scope,status,items,available_count,missing_count,next_action,style_prompt,style_digest`；每项返回 `setting_uuid,title,asset_type,expected_revision,has_image`，有当前图片时附 `asset_uuid,variant_uuid`。核对阶段用 `read_image` 实际看图，记录与用户要求／原参考的具体对照；页面生成阶段随后调用 `prepare_page_image`，由服务端读取源图并合图，agent 不逐张读取。

| 选择 | scope / status | 下一步 |
| --- | --- | --- |
| 省略 `setting_uuids` | `project_candidates / selection_required` | 返回视觉候选，排除无图的 reference 文字备注；按当前页面选择设定。无候选时先提取视觉设定。 |
| 已选设定有缺图项 | `selected_settings / missing_images` | 批量或逐项补齐所选缺图，不要求补齐其他章节资产。 |
| 已选设定均有图片 | `selected_settings / review_required` | 核对当前设定图并提交 `setting_reviews`，通过后准备本页参考图。 |
| 显式传 `[]` | `selected_settings / no_settings_selected` | 核对页面是否无需视觉设定，不将空选择当成就绪批准。 |

该检查不持久化页面依赖或选图，不修改 revision，也不自动生成图片。首次创作默认先完成必要设定图；“正文只生成第一页”不取消设定图阶段。用户明确要求跳过或限制全部生图次数时按实际要求执行，不伪报就绪。

- 页面目标比例沿用项目绘本规格（包括封面、封底和 1:3 条漫）。
- 角色、道具和参考设定建议 1:1；场景建议沿用项目比例。设定没有持久化的强制画布约束，这里仅提供创作建议。
- 在宿主常见 3:2、1:1、2:3 中按比例的对数距离选最近一档，属于生成建议，不承诺精确像素或比例。generation_hint 提醒保留主体、画面文字和安全边距。
- 建议页面 `crop`，设定 `pad`；这是生图前的处理建议，**import_image 的默认行为仍是 keep**。
- 容差为相对目标比例的 2%。宿主路径不调用 `ResolveImageSize`，服务商生图的精确比例规则保持原有行为。

### prepare_page_image

必填 `page_uuid,setting_uuids`；非空选择还必须提供逐项通过的 `setting_reviews`；可选 `additional_direction`（最多 12000 字符）。明确选择本页所需的当前项目 active 设定，最多 12 项、不得重复，数组顺序就是标签顺序。缺少选择和 null 无效；明确 `[]` 返回真实无参考模式。缺图、跨项目、无效或不可读源图明确报错。

`setting_reviews` 每项为 `{setting_uuid,variant_uuid,expected_revision,style_digest,verdict,reference_basis,comparison}`。身份、版本和画风摘要来自 `image_target`；`verdict=passed` 才可继续，`needs_revision` 阻止准备。依据和具体观察各为非空字符串，最多 2000 字符。缺失或未通过返回 `setting_image_review_required`，设定 revision／采用图片／有效画风变化返回 `setting_image_review_stale`；必须实际重新核对，不能只替换版本。无参考选择可省略。服务端校验记录与事实的对应关系，不声称自动判断视觉相似度。

宿主条漫正文默认按项目画风、已保存分镜和 1:3 画布生成；自定义基础规则原样使用。Go 使用与 Web 相同的 production 合图实现，维持原始比例、完整主体、中文编号名称及留白，每次合成 PNG，并按最终文件字节计算 `reference_image_sha256`。同一项目在当前 CLI 进程内复用内容一致的临时文件，丢失或损坏时写入新路径。返回真实绝对 `reference_image_path`、`reference_mode`、有序 `sources`（设定 UUID/revision、图片 variant_uuid/version_no/file_uuid、标签和内容 SHA-256）、`image_info`、`target`、完整 `generation_prompt`、`image_write_back` 与 `preparation`。none 模式路径为空、省略合图 SHA-256、sources=[]、image_info=null。不注册项目 Asset，不返回合图 Base64、data URI 或 MCP image content。

提示词复用 Web 的有效角色基础规则、project_overall_style 及旧数据回退、语言指令、参考说明、已保存分镜、用户补充要求。当前代理实际看过同一摘要与路径时可跳过预览；缺少摘要或无法确认时查看返回路径。每页用 imagegen `referenced_image_paths=[同一路径]`，仅这一张参考图；none 模式省略参考参数。保持完整提示词原样，不在返回后追加说明或重写参考描述。

`preparation` 含项目/页面身份、revision、所选 UUID、版本绑定的核对记录、补充要求、SHA-256 digest 与服务进程 HMAC 签名。摘要覆盖当前页面、设定版本及源字节、标签顺序、有效模板/画风/语言、项目规格、完整提示词与合图输入/合成器版本；临时路径不参与来源身份。凭据不缓存，后台重启需重新准备。

### import_image

必填：`path,target_uuid,slot,expected_revision`；页面额外必填与本轮准备逐字一致的 `prompt` 和原样 `preparation`（含 digest/signature），设定图不要求 preparation。可选：`fit_mode`（`keep/crop/pad`）、`description`、`prompt`（原始提示词）。说明和提示词各最多 12000 字符，超过限制返回错误，不静默截断。

path 必须是宿主生图结果实际给出的本地绝对路径；可以来自结果提示文本。工具不接收 base64，不假定 `savedPath` 字段、宿主目录或文件名，也不扫描目录。文件必须是可读的普通文件，支持 PNG、JPEG、WebP；最多 32 MiB、4000 万像素，先校验限制再完整解码。以真实内容判断格式，即使 PNG 文件被命名为 `.jpg` 也按 PNG 提交；伪装成图片的文本和损坏图片会被拒绝。

返回 `{target,revision,variant_uuid,version_no,asset,input_size,fit_mode,ratio_deviation,ratio_matches,warnings}`，target 为 image_target 的比例与版本字段（不含 setting_readiness）并携带新 revision。`ratio_deviation=(输入宽/输入高)/(目标宽/目标高)-1`，正值表示更宽；`ratio_matches` 表示输入是否在容差内。超出容差时仍导入，并通过 warnings 返回偏差和下一次生图建议。

| fit_mode | 行为 |
| --- | --- |
| `keep`（默认） | 按原始像素和比例保存，偏差仅提示 |
| `crop` | 超出容差时居中裁切到目标比例；允许整数像素舍入 |
| `pad` | 超出容差时保持原图大小，居中放到白色补边画布；允许整数像素舍入 |

在容差内直接使用原图，即使请求 crop/pad 也不重复处理。补边后的画布也受 4000 万像素限制；超限时可缩小原图或选择 keep/crop。

所有文件都由 Asset Store 提交。处理后的图片是新的派生 Asset，并以 `source_asset_uuid` 关联不可变原图。每次导入增加图片版本并切换 current；历史版本不被覆盖。Asset 与业务版本、current 指针在同一最终 SQLite 事务中提交，事务内再次检查 revision；页面导入还会在预检及最终事务中重新组装当前事实并校验准备摘要和完整提示词。页面、设定当前图、规则等变化返回 `page_image_preparation_stale`，不创建图片版本。签名禁止仅替换 revision/digest 绕过；需重新准备，按本次合图判断是否需要预览，再生图。

Asset 的公开 metadata 保存 `origin:"host_generated"`、`original_prompt`、`description`、`fit_mode`、输入宽高和目标比例，不保存原始绝对路径。兼容现有数据库枚举：File source_type 使用 imported/derived，业务版本沿用 manual/replacement；页面和设定的版本 DTO 根据 origin 将 `source_type` 显示为 `host_generated`。web UI 当前图和历史版本显示“宿主生图 / Host generated”。无需数据库迁移，桌面版和 CLI 共用项目格式。

已有项目页允许在没有服务商配置时打开，仍经过项目打开、初始化和 setup 状态检查；宿主创作项目可直接在 CLI web UI 中审阅。

PDF 沿用现有确定性版式：按图片实际比例等比缩放、在槽位内居中、剩余区域留白，不拉伸、不因目标比例不同中断导出。图片内容丢失或损坏仍返回已有的内容不可用错误。

### read_image

必填：`asset_uuid`，可选 `max_edge`（256–1024，默认 1024）。Asset UUID 来自 `pages get.current_image.asset.uuid`、`settings get.current_variant.asset.uuid` 或图片版本列表，可读取历史图片；只允许当前项目内的图片 Asset，不提供任意路径读取。

成功返回 text/structured 信封 `{asset_uuid,mime_type,width,height}`，另附一个真正的 MCP `content.type=image` 块。预览等比缩小、透明区域铺白，并编码成 JPEG；最长边不超过 max_edge，编码最多 120 KiB，必要时进一步缩小。包含图片的整个 MCP 结果仍不超过 256 KiB；二进制不重复出现在 text/structured content 中。原图不变。

宿主创作流程：

1. 选择本页设定，补齐所选缺图；批量总览图需完成拆分。
2. `prepare_page_image` 返回临时合图路径、来源版本、完整提示词及签名摘要。
3. 当前代理首次遇到该摘要与路径时看图。每次 imagegen 只用 `referenced_image_paths=[同一路径]` 和返回的完整 generation_prompt。none 模式不传参考图。
4. 立即 `import_image`，传实际宿主输出路径、image_write_back、同一提示词与原 preparation。
5. 冲突或准备失效时使用新准备结果重新生图；不能仅换 revision/摘要。无 provider pipeline 或付费回退。

### generate_page_image（Claude 宿主）

仅 `--host claude` 列出；其他宿主或未传 `--host` 调用时被拒绝。Codex 仍只用宿主 imagegen，没有 provider 回退。需要创作协议 v6：旧 CLI 后台返回 `cli_upgrade_required`，其他工具不受影响。

必填 `page_uuid,setting_uuids,idempotency_key,expected_revision`；`setting_reviews`、`additional_direction` 与 `prepare_page_image` 相同，可选 `model`（所配置服务商的图片模型，默认项目图片模型）。服务端先执行与 `prepare_page_image` 相同的校验（设定核对缺失、过期或 `needs_revision` 被拒，页面 revision 冲突被拒），再创建现有页面生图任务：提示词固定为同一份 `generation_prompt`，参考为同一张合图，不运行 LLM 参考选择，因此与 Web/`prepare_page_image` 的合图和提示词一致。服务商未配置返回 `image_provider_unavailable`（未计费，details 给出 `web_url view=providers` 与重试说明）。

调用内等待任务完成。仍在运行时返回 `{task,page_uuid,done:false,next_action}`，用完全相同的参数和 `idempotency_key` 再次调用继续等待；同一 key 重放返回同一任务，不重复建任务或计费。成功返回 `{task,done:true,page_uuid,chapter_uuid,revision,image,is_current_image,read_image:{asset_uuid}}`，结果挂载为页面当前图片；随后 `read_image` 看图并 `pages record_image_review`。返回字段只含公开 UUID。任务失败返回 `page_image_generation_failed`。任务进度通过实时 hub 观察，不轮询。

### web_url

只读。可选 `page_uuid`、`chapter_uuid`、`setting_uuid` 或 `view`（`book/chapters/settings/exports/providers`），默认作品总览。返回 `{project_uuid,url,scope,chapter_uuid?,page_uuid?,setting_uuid?,note}`；地址由当前运行的 CLI 后台实际 endpoint 与公开 UUID 组成，对应 Web 路由 `/projects/:project_uuid/`、`chapters`、`chapters/:chapter_uuid`、`chapters/:chapter_uuid/sections/:page_uuid`、`premise`、`premise/assets/:setting_uuid`、`exports` 与全局 `/settings/providers`。页面与章节不匹配时报错。端口可能随重启变化，每次交付前重新获取，不写入文件。CLI 需以 `-tags embed_frontend` 构建才能打开页面。

### export_website（Codex / Claude Code v7，Cindy v8）

用户说“生成网站”“导出成网站”“更新网站”，或上下文已明确是网站导出时，直接调用 `export_website({})`，不预查状态、不区分首次和更新、不重复确认。只说“导出绘本”、指定 PDF / ZIP / Markdown，或询问“能不能生成网站”，都不触发。它生成整本作品的本地静态网站；公网发布须另行提出。

默认不允许缺图。用户已明确接受“只导出已有图片”时直接传 `allow_missing_images=true`；否则仅在收到 `comic_export_incomplete` 后报告实际缺图数量，询问是否继续。收到 `comic_export_empty` 就说明至少需要一张有效正文图，不建议跳过。超时、断线或 `done=false` 时用原参数继续同一任务；如果传了 `idempotency_key`，重试须保持原键。收到 `done=true` 后告诉用户网站已生成在 `output_directory` 返回的本地目录。Codex 和 Claude Code 同时提供可打开的 `preview_url`。新的独立请求可使用新键。定向重放会检查原任务是否仍为当前发布版本。

### export_book

必填：`format`（`markdown/zip/pdf`）、`allow_missing_images`（布尔）。可选 `chapter_uuid` 限定章节正文、页面和缺图检查；整书省略。返回 `{uuid,format,scope,chapter_uuid?,path,byte_size,page_count,missing_image_count}`，path 为项目 `.lumi/exports/` 下的绝对路径；不接受任意导出路径。

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
| `cli_upgrade_required` | 旧后台缺少当前创作能力（v4），请协调其他使用者再在同 environment/data-dir 执行 lumi stop 后重启 |
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
| `image_path_unavailable` / `asset_path_unsafe` | 文件不存在、不可读、不是普通文件，或路径不是实际绝对路径；从宿主结果获取路径并立即导入 |
| `asset_too_large` / `asset_pixels_too_large` | 文件字节、输入像素或处理后画布超限；缩小图片或改用 keep/crop |
| `asset_content_invalid` / `asset_type_not_allowed` | 内容损坏、不可解码或格式不支持；不要仅修改扩展名 |
| `asset_not_found` / `asset_object_unavailable` | read_image 的 UUID 不属于当前项目，或图片文件不可用 |
| `internal_error` | 本地存储等未预期失败；检查 CLI 日志 |

路径、项目格式和迁移失败还会保留既有项目错误码，如 `invalid_project_path/project_permission_denied/invalid_project/project_format_too_new/project_migration_failed`。

## 实时同步与验证

写入使用现有 `story:*`、`premise:*`、`comic:*` 和 `project:setup_changed` 变更提示，通过 `/api/v1/ws` 的 `topic/event/payload/ref/join_ref` 信封发送。已打开 UI 沿用 TanStack Query 失效与 REST 重读，并保留首次 join、重新 join、窗口聚焦校准；没有新增 HTTP 轮询。

`cmd/lumi/mcp_test.go` 使用 go-sdk CommandTransport 客户端执行真实 stdio → CLI 实例 → SQLite 链路：空目录初始化、无服务商 setup、渲染提示词、固定文本写回、三页正文和画面、导出并检查文件内容；同时测试双连接版本冲突、UI WebSocket/REST、桌面占锁和释放。结束后检查无服务商配置及零 LLM/生成任务记录。`internal/textmcp` 另测 schema、提示词覆盖、分页、大小限制与缩小范围恢复，以及高熵图片预览的大小限制和 MCP image content；`internal/production` 覆盖宿主图片比例正确与容差、keep/crop/pad、真实 PNG/JPEG/WebP、伪装扩展名、文件超限/像素超限/损坏/路径缺失、目标缺失、版本冲突、历史和原图保留。Step 4 的真实 stdio 链路另导入三种比例到页面，读取设定参考图，验证 WebSocket 变更提示与 UI REST 历史/来源，并导出不缺图的 PDF。

2026-10-30 本机验证：`go test ./...` 通过；前端 `node --test` 422 项通过，Vite 生产构建通过。CLI 内嵌 web UI 在零服务商配置下成功打开页面和角色设定，显示宿主导入的图片、历史版本和“宿主生图”来源；候选稿弹窗保持打开时再经真实 stdio 导入，列表从两版自动更新为三版，当前图片随之切换，无需手动刷新。另验证最终 Asset 提交事务中的版本冲突会同时回滚逻辑 File 和图片版本。
