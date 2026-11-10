# Lumi MCP 调用参考

按需阅读：首次规格选择／写入先读「Setup 与规格」；创建、编辑时查「动作参数与版本」；调用模板时查「渲染与写回」。本参考随插件发布，配合当前工具声明使用。声明中的基础 required 不代表所有动作的必填项；条件必填由工具说明列出，服务端仍强制校验。

示例采用 MCP `tools/call` 的 `name` 和 `arguments` 形式；向对应宿主工具只传 `arguments`。示例 UUID 和 revision 仅为示意，必须替换成刚读取或渲染得到的值。`expected_revision` 是整数，合法的 `0` 不能当成缺失值；setup 草稿的 revision 从 1 开始。成功结果在 `data` 中，失败结果为 `success=false`、`data=null` 和 `error`，失败后按错误核对契约或重新读取事实。

## Setup 与规格

除网站导出直接调用 `export_website` 外，先 `project_overview`；未初始化且用户要求创建时，`project_init` 必填 `idea`，可选 `name`、`generation_language`（`zh-Hans` / `en`）。它创建 setup 草稿，已有项目不会重置。

```json
{"name":"project_init","arguments":{"idea":"小狐狸把迷路的星星送回天空","name":"回家的星星","generation_language":"zh-Hans"}}
```

`project_setup` 的顺序为 `get → update → finalize`：

| action | 必填及可写字段 | revision 来源 |
|---|---|---|
| `get` | 仅 `action` | 返回 setup 的 `data.revision` |
| `update` | `expected_revision`；至少一个 `project_name`、`generation_language`、`overall_style`、`generation_brief`、`picture_book` | 最近 get／update 的 setup revision |
| `finalize` | `expected_revision`；不在此动作提交字段修改 | **update 返回的新 revision** |

```json
{"name":"project_setup","arguments":{"action":"get"}}
```

定稿需要名称、生成语言、画风、创作要求和有效绘本规格完整；已有值直接沿用。`generation_scope` **不是该 MCP 工具的输入字段**，不要传入。服务端在 update 中固定 `project_only`，只表示不启动 Lumi 内部自动生成，不限制继续完成故事、设定、分页和 `generate_page_image` 页面生图。

创作范围分别写清设定图、封面图和正文图。“正文只生成第一页”仍包含首次创作的必要设定图；不要把固定的 `project_only` 当作只写文字，也不要自行添加额度限制。用户明确的纯文字或跳过设定图要求优先。

用户附带参考图时，按 [设定图的用户参考](setting-references.md) 先用 Read 实际看图并区分画风／身份／故事用途；用户要求优先；画风参考提取具体视觉特征（含独特对白字体）后写入 `overall_style`，参考用途写入 `generation_brief`，再定稿。正文依赖核对后的设定图。`context_setting_uuids` 只提供项目上下文，不会把图片交给生图服务。

先核对合法组合，再提供选项。定稿后正式规格不可改；用户已指定的形式与比例冲突时，说明可行选择及影响，获得针对冲突的决定后再定稿。选择比例不等于同意改形式；已有自主授权范围内的缺失选择可直接决定，不新增重复确认。

| `picture_book.format` | 可传参数与默认值 | 不可传的专属参数 |
|---|---|---|
| `classic_picture_book` | `aspect_ratio`；`large_image_minimal_text` 默认 `false` | `interaction_mode`、`comic_layout` |
| `wordless_picture_book` | 仅 `aspect_ratio` | 少字、互动、漫画布局参数 |
| `interactive_picture_book` | `interaction_mode` 默认 `find_it`，也可 `make_a_choice` / `guess` / `follow_along`；当前内部比例 4:3 | **`aspect_ratio`**、少字、漫画布局参数 |
| `comic_story` | `aspect_ratio`；`comic_layout` 默认 `page_comic`，也可 `four_panel` | 少字、互动参数 |
| `vertical_strip` | **只传 `{"format":"vertical_strip"}`，比例固定 1:3** | **`aspect_ratio` 以及所有其他绘本参数** |

支持比例输入的形式：省略时默认 4:3；`{"mode":"landscape"}` 为 4:3，`{"mode":"square"}` 为 1:1，`{"mode":"portrait"}` 为 3:4。预设模式不得同时提供 width／height。自定义使用 `{"mode":"custom","width":2,"height":3}`，width／height 必须是 1–100 的整数，比例在 1:3 至 3:1 之间，服务端会约分。不要把字符串 `"2:3"` 当作比例对象。

用户要求条漫时的完整 update 示例（不附加比例参数）：

```json
{"name":"project_setup","arguments":{"action":"update","expected_revision":1,"project_name":"回家的星星","overall_style":"温暖水彩","generation_brief":"小狐狸帮助星星回家，正文 12 页。","picture_book":{"format":"vertical_strip"}}}
```

只有用户明确选择**页漫 2:3**，或已明确同意从其他形式改为页漫时，才采用下面的替代 update；它不是前一个例子失败后的自动兜底：

```json
{"name":"project_setup","arguments":{"action":"update","expected_revision":1,"picture_book":{"format":"comic_story","aspect_ratio":{"mode":"custom","width":2,"height":3},"comic_layout":"page_comic"}}}
```

读取 update 的新 revision；例如返回 2 时：

```json
{"name":"project_setup","arguments":{"action":"finalize","expected_revision":2}}
```

## 动作参数与版本

下表字段均指 `arguments`；每个动作还需 `action`。列表可选 `after` / `before` / `limit`，前两个互斥，使用返回的 cursor；默认每页 20、最大 100。

| 工具／action | 条件必填字段 | 可选字段／业务要求 |
|---|---|---|
| `story_profile get` | 无 | 返回完整 `story_md` 和 revision |
| `story_profile write` | `story_md`、`expected_revision` | 完整替换 profile，同时更新 STORY.md 投影 |
| `chapters list` / `get` | list 无；get 要 `chapter_uuid` | list 支持游标 |
| `chapters create` | `chapter_code`、`title`、`expected_revision` | `content`、`content_format`（`txt` / `md`，默认 txt） |
| `chapters update` | `chapter_uuid`、`title`、`expected_revision` | 更新标题 |
| `chapters write` | `chapter_uuid`、`content`、`expected_revision` | `content_format`（默认 txt）；完整章节正文 |
| `chapters delete` | `chapter_uuid`、`expected_revision` | 软删除 |
| `settings list` / `get` | list 无；get 要 `setting_uuid` | list 支持游标 |
| `settings create` | `asset_type`、`title`、`summary`、`expected_revision` | `asset_type`: `character` / `scene` / `prop` / `reference`；可选 `tags` 字符串数组 |
| `settings update` | `setting_uuid`、`expected_revision` | 至少一个 `asset_type`、`title`、`summary`、`tags` |
| `settings delete` | `setting_uuid`、`expected_revision` | 软删除 |
| `settings select_image` | `setting_uuid`、`variant_uuid`、`expected_revision` | 先重读，核对用户选择的候选 UUID |
| `pages list` | `chapter_uuid` | 支持游标，返回 `state.revision`；续做用 `summary_only:true` 只读版本、缺图和核对状态 |
| `pages get` | `chapter_uuid`、`page_uuid` | 返回页面及其 revision |
| `pages record_image_review` | `chapter_uuid, page_uuid, variant_uuid, expected_revision, verdict, observation` | 实际看图后记录 passed / needs_revision，返回独立的 review_revision；修订已有观察时传 expected_review_revision |
| `pages create` | `chapter_uuid`、`title`、`description_md`、`storyboard_md`、`expected_revision` | 可选 `page_role`: `body` / `front_cover` / `back_cover` |
| `pages update` | `chapter_uuid`、`page_uuid`、`expected_revision` | 至少一个 `title`、`description_md`、`storyboard_md`、`page_role` |
| `pages delete` | `chapter_uuid`、`page_uuid`、`expected_revision` | 软删除 |
| `pages select_image` | `chapter_uuid`、`page_uuid`、`variant_uuid`、`expected_revision` | 先重读，核对用户选择的候选 UUID |

| 写操作 | 使用的 `expected_revision` |
|---|---|
| 创建章节 | `project_overview` 的 `data.project.revision` |
| 创建设定 | `settings list` 的 `data.premise.revision` |
| 创建页面 | 对应章节 `pages list` 的 `data.state.revision` |
| 写 Story Profile | `story_profile get` 的 `data.revision`，或对应 render_prompt 的 `expected_revision` |
| 更新／删除已有页 | 审稿时按 UUID 从 `pages list` 的 `data.items[].revision` 或 `pages get` 的 `data.revision` 取值；同一页再次修改时用上次更新返回的 revision |
| 其他单资源编辑／删除／选择 | 对应 get 返回的 `data.revision`，或适用于该目标的 render_prompt revision |

整章改稿时按游标读齐目标页，以页 UUID 对应各自的 revision，逐页提交。若已读到所需正文和分镜，可直接使用这次列表结果；若只读了摘要，按需获取待审页全文。遇到版本冲突，用 `pages get` 重读该页；涉及页序或跨页合并时也核对相关页，再决定剩余改动。

创建返回的新对象 revision **不是父集合 revision**。下一次创建前重新读取父集合；不能给整批复用同一个旧 revision，也不能把页面 revision 用于创建下一页。冲突后核对内容是否仍适用，不能只换版本号强行覆盖。

创建设定用 `asset_type/title/summary`，不用 `type/description_md`：

```json
{"name":"settings","arguments":{"action":"create","asset_type":"character","title":"小狐狸","summary":"橘红毛、白胸口，穿黄色雨衣。","expected_revision":0}}
```

创建页面时，`description_md` 保存正文、对白和旁白；`storyboard_md` 保存完整画面描述，包括构图、动作、表情、光线和逐字绘入的文字。不要把整份分镜 JSON 放入其中一个字段。

```json
{"name":"pages","arguments":{"action":"create","chapter_uuid":"01989abc-def0-7000-8000-000000000001","title":"发现星星","description_md":"小狐狸：我陪你回家。","storyboard_md":"雨夜森林，两格从上往下阅读。第一格发现草丛中的星星；第二格狐狸撑起叶子伞，气泡逐字写：我陪你回家。","page_role":"body","expected_revision":0}}
```

## 工具结果回显

普通 Lumi MCP 调用从 `structuredContent` 取一次 JSON 信封；宿主仅提供文本时才解析 `content` 中的 JSON，不重复展开两份。按名称取当前工具声明，按 `prompt_group` 查模板，只回显本次要用的键和占位符。写入成功时回显目标 UUID、revision、警告及下一步需要的字段。分页保留游标和父集合 revision。出错时回显目标和原错误的 `code/message/details`；宿主调用本身失败时保留其诊断。

先检查 `isError` 与信封 `success`；成功时只记下本次要用的 `prompt_key`、`target_uuid`、`expected_revision` 和 `write_back`，完整 `prompt` 留给执笔的代理读取。委派时将完整结果连同用户约束交给子任务；父代理自己执行时读取完整 `prompt`。写回使用已审阅的完整草稿。中断后从 Lumi 重读已保存的事实并核对版本。

## 渲染与写回

先查 `prompt_catalog` 中实际的 `prompt_group`、`prompt_key` 与占位符。`render_prompt` 基础必填为 `prompt_group`、`prompt_key`、`target`；`variables` 是字符串值对象。不要猜模板名或覆盖工具自动填入的项目事实。

`prompt_group` 是模板分组：`story` / `chapter` / `premise` / `premise_style` / `agent` / `runtime`。**`target` 是读写目标，不是模板分组**：Story Profile 使用 `target="story_profile"`，不能用 `"story"`。

| target | 额外必填 | 写回工具／action 与版本 |
|---|---|---|
| `setup` | 无 | `project_setup update`；setup revision |
| `story_profile` | 无 | `story_profile write`；profile revision |
| `chapter` | `chapter_uuid` | `chapters write`；章节 revision |
| `pages` | `chapter_uuid` | `pages create`；页面集合 revision |
| `page` | `chapter_uuid`、`page_uuid` | 文本用 `pages update`；页面生图用 `prepare_page_image` 的 `generation_prompt/image_write_back/preparation` 导入，见 [页面生图提示词](page-images.md)；单页 revision |
| `premise` | 无 | `settings create`；设定集合 revision |
| `setting` | `setting_uuid` | `settings update`；单项设定 revision |
| `setting_board` | 无；只配合 `premise/setting_image` | `setting_boards import`；设定集合 revision；返回的 `generation_prompt` 可交给用户在其他工具中生成总览图（Claude Code 不能直接生成） |
| `setting_breakdown` | `board_uuid`；只配合 `premise/asset_breakdown` | `apply_setting_breakdown`；批次 revision 与 `expected_premise_revision` |

```json
{"name":"render_prompt","arguments":{"prompt_group":"story","prompt_key":"story_profile","target":"story_profile","variables":{"chapter_count":"1"}}}
```

文本任务按返回的完整 `prompt` 执行，用 `write_back` 映射输出；页面生图使用 `generation_prompt` 和 `image_write_back`，片段组合使用 `fragment_prompt`，不要把外层任务说明传给生图工具。渲染不等于写入。`context_chapter_uuids` / `context_setting_uuids` 可缩小上下文，`[]` 表示省略该集合，目标自身仍包含。缺模板变量时根据目录及错误补齐；用户已明确的内容和执行授权继续沿用。

竖向条漫正文使用 `target="pages"`、实际 `chapter_uuid`、`chapter/comic_storyboard`。例如最多 12 页时传 `variables={"max_section_count":"12","moment_count_plan":"auto"}`。页数上限可取 1–24。Lumi 以章节 UUID 为种子，每组三页打乱 1、2、3 个关键视觉瞬间，把逐页数组及单页上限 3 填入提示词。实际写几页就取前几项；同章重试或调大上限时，前面各页的计划一致。用户指定逐页节奏时可显式传入计划和上限。把完整提示词交给写分镜的代理，并保留这份逐页计划。构图按各页的动作、视线与对白安排。

```json
{"name":"story_profile","arguments":{"action":"write","story_md":"# 回家的星星\n\n小狐狸帮助迷路的星星回家。","expected_revision":0}}
```

对话编号选择用 `pages/settings {action:"get",include_versions:true,...目标 UUID}` 重读。返回的 `image_versions` 是分页的 `{items,total,snapshot,cursor_pagination}`，每项仅含 uuid、version_no、asset_uuid、is_current。按用户编号匹配 version_no，再用目标最新 revision 和 variant UUID 调用上表的 `select_image`；需要后续页时使用 after/limit。需要让用户比较候选时，用 `read_image` 自己看图并在对话中描述，或给出页面／设定的 `web_url` 让用户在网页中查看。


## 批量设定图

页面生图用 `image_target(target_uuid, slot=page_image, setting_uuids?)` 返回 `setting_readiness`；省略选择查看候选，传 UUID 数组检查本页依赖。缺图先补齐设定图；`review_required` 表示有图待核对，按实际原图和用户要求核对通过后，携带当前版本与画风对应的 `setting_reviews` 调用 `generate_page_image(page_uuid, setting_uuids, setting_reviews, expected_revision, idempotency_key, additional_direction?, model?)`。需要预览参考合图时先用相同参数调用 `prepare_page_image` 并 Read 返回路径。详见 [页面生图](page-images.md)。`setting_uuids` 不用于 `slot=setting_image`，也不会持久化页面关联。

执行前读 [设定图来源与降级](batch-settings.md)。这些工具只保存、裁切和绑定已有图片；Claude Code 不能生成总览图，用户在其他工具或 Lumi 中生成后才可导入。

| 工具 | 参数与版本来源 |
| --- | --- |
| `setting_boards list` | `action=list`；支持 `before/after/limit`，返回批次 UUID、revision、时间 |
| `setting_boards get` | `action=get, board_uuid`；返回 source、setting_image（含 asset）、revision、latest_breakdown |
| `setting_boards import` | `action=import, path, source_text, prompt, expected_revision, idempotency_key`；可选 `style_snapshot`；revision 来自 `settings list` 的 `premise.revision` 或生成前的 `render_prompt`；原图不裁切 |
| `apply_setting_breakdown` | `board_uuid, expected_revision, expected_premise_revision, idempotency_key, assets`；分别使用批次 revision 和设定集合 revision；每批 1–6 项 |

每项 asset 使用 `type, title, summary, tags, confidence, crop_box`；`crop_box` 为原始整图上的归一化 `x/y/width/height`，不能重叠，`confidence` 至少 0.92。更新已有设定时再加 `setting_uuid, expected_revision`（该设定当前 revision）；省略这两个字段只允许创建新设定，同名会报冲突。返回 `items` 中的 `setting_uuid/variant_uuid/asset_uuid` 均为实际写入结果，`revision` 属于该设定。

同一次请求重试必须保留原幂等键和全部原参数，包括 revision。导入成功后即使宿主文件过期，原请求仍可重放；幂等键不能用于替换同路径下的新图片。修改输入或开始新的拆分使用新键。`latest_breakdown` 是最后一次提交的结果，不代表其图片仍是用户当前选择；审阅或更新前重读 `settings get`。


## 交付与 Web 链接

完成用户要求的创作、保存和所需导出后，只调用一次与交付范围对应的 `web_url`：整本 `web_url({})`，指定章节 `web_url({chapter_uuid})`，仅生成或交付一页（包括仅第一页）`web_url({page_uuid})`。导出不改变交付范围，不在章节或单页之后追加整本链接。不等待用户打开链接，不新增确认节点。

```json
{"name":"web_url","arguments":{"page_uuid":"01989abc-def0-7000-8000-000000000002"}}
```

返回 `{project_uuid, url, scope, note}`，按范围附 `chapter_uuid/page_uuid/setting_uuid`。`view` 可取 `chapters`、`settings`、`exports`、`providers`，与 UUID 范围互斥；页面与 `chapter_uuid` 不匹配时报 `invalid_arguments`。url 使用当前后台的实际端口，原样交给用户，不拼写或缓存。

用户在网页中编辑后回到对话时，先 get 相关对象并取最新 revision；写入冲突时重读并合并，不覆盖网页修改。

`generate_page_image` 返回 `{task, done, page_uuid, chapter_uuid, revision, image:{variant_uuid,version_no,asset_uuid}, is_current_image, read_image, next_action}`；`done:false` 时只含 task、page_uuid 和 next_action，用相同参数和 key 重放。所有字段只含公开 UUID。

章节交付的 `export_book` 传目标 `chapter_uuid`；省略即整书。三种格式均返回 scope、page_count、missing_image_count。

## 整本阅读网站

用户说“生成网站”“导出成网站”“更新网站”，或上下文已明确是网站导出时，直接调用 `export_website({})`，不预查状态、不区分首次和更新、不重复确认。只说“导出绘本”、指定 PDF / ZIP / Markdown，或询问“能不能生成网站”，都不触发。它生成整本作品的本地静态网站；公网发布须另行提出。

默认不允许缺图。用户已明确接受“只导出已有图片”时直接传 `allow_missing_images=true`；否则仅在收到 `comic_export_incomplete` 后报告实际缺图数量，询问是否继续。收到 `comic_export_empty` 就说明至少需要一张有效正文图，不建议跳过。超时、断线或 `done=false` 时用原参数继续同一任务；如果传了 `idempotency_key`，重试须保持原键。只有 `done=true` 才交付返回的绝对 `output_directory` 和可打开的 `preview_url`，不以旧站仍能访问判断成功。在同一条成功回复中，将返回的完整 `publish_prompt` 原样放在独立的文本代码块中，方便用户复制到 Codex、修改后再发送。提示词仅供复制，执行发布需要用户另行提出。
