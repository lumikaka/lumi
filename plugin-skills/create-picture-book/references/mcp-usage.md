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

<!-- host:codex -->
定稿需要名称、生成语言、画风、创作要求和有效绘本规格完整；已有值直接沿用。`generation_scope` **不是该 MCP 工具的输入字段**，不要传入。服务端在 update 中固定 `project_only`，只表示不启动 Lumi 内部自动生成，不限制 Codex 继续完成故事、设定、分页和宿主生图。
<!-- host:claude -->
定稿需要名称、生成语言、画风、创作要求和有效绘本规格完整；已有值直接沿用。`generation_scope` **不是该 MCP 工具的输入字段**，不要传入。服务端在 update 中固定 `project_only`，只表示不启动 Lumi 内部自动生成，不限制继续完成故事、设定、分页和 `generate_page_image` 页面生图。
<!-- /host -->

创作范围分别写清设定图、封面图和正文图。“正文只生成第一页”仍包含首次创作的必要设定图；不要把固定的 `project_only` 当作只写文字，也不要自行添加额度限制。用户明确的纯文字或跳过设定图要求优先。

<!-- host:codex -->
用户附带参考图时，按 [设定图的用户参考](setting-references.md) 先实际读图并区分画风／身份／故事用途；用户要求优先；画风参考提取具体视觉特征（含独特对白字体）后写入 `overall_style`，参考用途写入 `generation_brief`，再定稿。正文依赖核对后的设定图；设定生图必须通过宿主图片参数携带相关原图。`context_setting_uuids` 只提供项目上下文，不会自动把图片传给 imagegen。
<!-- host:claude -->
用户附带参考图时，按 [设定图的用户参考](setting-references.md) 先用 Read 实际看图并区分画风／身份／故事用途；用户要求优先；画风参考提取具体视觉特征（含独特对白字体）后写入 `overall_style`，参考用途写入 `generation_brief`，再定稿。正文依赖核对后的设定图。`context_setting_uuids` 只提供项目上下文，不会把图片交给生图服务。
<!-- /host -->

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

<!-- host:codex -->
普通 Lumi MCP 调用从 `structuredContent` 取一次 JSON 信封；宿主仅提供文本时才解析 `content` 中的 JSON。避免用 `text(result)` 同时展开两份。按名称取当前工具声明，按 `prompt_group` 查模板，只回显本次要用的键和占位符。写入成功时回显目标 UUID、revision、警告及下一步需要的字段。分页保留游标和父集合 revision。出错时回显目标和原错误的 `code/message/details`；宿主调用本身失败时保留其诊断。

例如在 Codex 执行单元中，`renderArgs` 为本次实际参数，`taskKey` 为本次任务独有的键。工具名按当前连接的正式版或开发版选择。

```js
const result = await tools.mcp__lumi__render_prompt(renderArgs);
if (result.isError) {
  text({ target: renderArgs.target, error: result.structuredContent?.error ?? result.content[0]?.text });
  exit();
}
const envelope = result.structuredContent ?? JSON.parse(result.content[0].text);
if (!envelope.success) {
  text({ target: renderArgs.target, error: envelope.error });
  exit();
}
const task = envelope.data;
store(taskKey, task);
text({ taskKey, template: task.prompt_key, target_uuid: task.target_uuid, expected_revision: task.expected_revision });
```

同一轮中用 `load(taskKey)` 取回完整对象。委派时将该对象连同用户约束交给子任务；父代理自己执行时读取完整 `prompt`。写回使用已审阅的完整草稿。中断后从 Lumi 重读已保存的事实并核对版本，执行单元里的键只用于临时传值。
<!-- host:claude -->
普通 Lumi MCP 调用从 `structuredContent` 取一次 JSON 信封；宿主仅提供文本时才解析 `content` 中的 JSON，不重复展开两份。按名称取当前工具声明，按 `prompt_group` 查模板，只回显本次要用的键和占位符。写入成功时回显目标 UUID、revision、警告及下一步需要的字段。分页保留游标和父集合 revision。出错时回显目标和原错误的 `code/message/details`；宿主调用本身失败时保留其诊断。

先检查 `isError` 与信封 `success`；成功时只记下本次要用的 `prompt_key`、`target_uuid`、`expected_revision` 和 `write_back`，完整 `prompt` 留给执笔的代理读取。委派时将完整结果连同用户约束交给子任务；父代理自己执行时读取完整 `prompt`。写回使用已审阅的完整草稿。中断后从 Lumi 重读已保存的事实并核对版本。
<!-- /host -->

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
<!-- host:codex -->
| `setting_board` | 无；只配合 `premise/setting_image` | `setting_boards import`；设定集合 revision；额外返回供宿主生图使用的 `generation_prompt` |
<!-- host:claude -->
| `setting_board` | 无；只配合 `premise/setting_image` | `setting_boards import`；设定集合 revision；返回的 `generation_prompt` 可交给用户在其他工具中生成总览图（Claude Code 不能直接生成） |
<!-- /host -->
| `setting_breakdown` | `board_uuid`；只配合 `premise/asset_breakdown` | `apply_setting_breakdown`；批次 revision 与 `expected_premise_revision` |

```json
{"name":"render_prompt","arguments":{"prompt_group":"story","prompt_key":"story_profile","target":"story_profile","variables":{"chapter_count":"1"}}}
```

文本任务按返回的完整 `prompt` 执行，用 `write_back` 映射输出；页面生图使用 `generation_prompt` 和 `image_write_back`，片段组合使用 `fragment_prompt`，不要把外层任务说明传给生图工具。渲染不等于写入。`context_chapter_uuids` / `context_setting_uuids` 可缩小上下文，`[]` 表示省略该集合，目标自身仍包含。缺模板变量时根据目录及错误补齐；用户已明确的内容和执行授权继续沿用。

竖向条漫正文使用 `target="pages"`、实际 `chapter_uuid`、`chapter/comic_storyboard`。例如最多 12 页时传 `variables={"max_section_count":"12","moment_count_plan":"auto"}`。页数上限可取 1–24。Lumi 以章节 UUID 为种子，每组三页打乱 1、2、3 个关键视觉瞬间，把逐页数组及单页上限 3 填入提示词。实际写几页就取前几项；同章重试或调大上限时，前面各页的计划一致。用户指定逐页节奏时可显式传入计划和上限。把完整提示词交给写分镜的代理，并保留这份逐页计划。构图按各页的动作、视线与对白安排。

```json
{"name":"story_profile","arguments":{"action":"write","story_md":"# 回家的星星\n\n小狐狸帮助迷路的星星回家。","expected_revision":0}}
```

<!-- host:codex -->
`review_candidates` 按 `kind` 决定参数：`page` 要 `chapter_uuid/page_uuid`；`setting` 要 `setting_uuid`；`text` 要 2–12 个 `candidates`，每项为 `{"title":"...","text":"..."}`。点击预览不保存；共享工作区的“使用此稿”通过现有接口采用。对话编号选择用 `pages/settings {action:"get",include_versions:true,...目标 UUID}` 重读。返回的 `image_versions` 是分页的 `{items,total,snapshot,cursor_pagination}`，每项仅含 uuid、version_no、asset_uuid、is_current。按用户编号匹配 version_no，再用目标最新 revision 和 variant UUID 调用上表的 `select_image`；需要后续页时使用 after/limit，不从 UI 本地选中推断采用事实。

```json
{"name":"review_candidates","arguments":{"kind":"text","candidates":[{"title":"温暖结局","text":"星星点亮了狐狸回家的路。"},{"title":"幽默结局","text":"星星约狐狸下次带一把更大的伞。"}]}}
```
<!-- host:claude -->
对话编号选择用 `pages/settings {action:"get",include_versions:true,...目标 UUID}` 重读。返回的 `image_versions` 是分页的 `{items,total,snapshot,cursor_pagination}`，每项仅含 uuid、version_no、asset_uuid、is_current。按用户编号匹配 version_no，再用目标最新 revision 和 variant UUID 调用上表的 `select_image`；需要后续页时使用 after/limit。需要让用户比较候选时，用 `read_image` 自己看图并在对话中描述，或给出页面／设定的 `web_url` 让用户在网页中查看。
<!-- /host -->


## 批量设定图

<!-- host:codex -->
页面准备用 `image_target(target_uuid, slot=page_image, setting_uuids?)` 返回 `setting_readiness`；省略选择查看候选，传 UUID 数组检查本页依赖。缺图先生成设定图；`review_required` 表示有图待核对，按实际原图和用户要求核对通过后，携带当前版本与画风对应的 `setting_reviews` 调用 `prepare_page_image(page_uuid, setting_uuids, setting_reviews, additional_direction?)` 返回合图路径和 SHA-256；当前代理首次见到该组合或缺少已看记录时读图，每页通过 `referenced_image_paths` 仅引用同一路径；页面 `import_image` 原样带回 `generation_prompt` 和含 digest/signature 的 `preparation`。详见 [页面生图提示词](page-images.md)。`setting_uuids` 不用于 `slot=setting_image`，也不会持久化页面关联。

执行前读 [批量设定图流程](batch-settings.md)。这些工具只保存、裁切和绑定图片；生成及视觉定位由 Codex 完成。
<!-- host:claude -->
页面生图用 `image_target(target_uuid, slot=page_image, setting_uuids?)` 返回 `setting_readiness`；省略选择查看候选，传 UUID 数组检查本页依赖。缺图先补齐设定图；`review_required` 表示有图待核对，按实际原图和用户要求核对通过后，携带当前版本与画风对应的 `setting_reviews` 调用 `generate_page_image(page_uuid, setting_uuids, setting_reviews, expected_revision, idempotency_key, additional_direction?, model?)`。需要预览参考合图时先用相同参数调用 `prepare_page_image` 并 Read 返回路径。详见 [页面生图](page-images.md)。`setting_uuids` 不用于 `slot=setting_image`，也不会持久化页面关联。

执行前读 [设定图来源与降级](batch-settings.md)。这些工具只保存、裁切和绑定已有图片；Claude Code 不能生成总览图，用户在其他工具或 Lumi 中生成后才可导入。
<!-- /host -->

| 工具 | 参数与版本来源 |
| --- | --- |
| `setting_boards list` | `action=list`；支持 `before/after/limit`，返回批次 UUID、revision、时间 |
| `setting_boards get` | `action=get, board_uuid`；返回 source、setting_image（含 asset）、revision、latest_breakdown |
| `setting_boards import` | `action=import, path, source_text, prompt, expected_revision, idempotency_key`；可选 `style_snapshot`；revision 来自 `settings list` 的 `premise.revision` 或生成前的 `render_prompt`；原图不裁切 |
| `apply_setting_breakdown` | `board_uuid, expected_revision, expected_premise_revision, idempotency_key, assets`；分别使用批次 revision 和设定集合 revision；每批 1–6 项 |

每项 asset 使用 `type, title, summary, tags, confidence, crop_box`；`crop_box` 为原始整图上的归一化 `x/y/width/height`，不能重叠，`confidence` 至少 0.92。更新已有设定时再加 `setting_uuid, expected_revision`（该设定当前 revision）；省略这两个字段只允许创建新设定，同名会报冲突。返回 `items` 中的 `setting_uuid/variant_uuid/asset_uuid` 均为实际写入结果，`revision` 属于该设定。

同一次请求重试必须保留原幂等键和全部原参数，包括 revision。导入成功后即使宿主文件过期，原请求仍可重放；幂等键不能用于替换同路径下的新图片。修改输入或开始新的拆分使用新键。`latest_breakdown` 是最后一次提交的结果，不代表其图片仍是用户当前选择；审阅或更新前重读 `settings get`。


<!-- host:codex -->
## 最终绘本预览

完成用户要求的创作、保存和所需导出后，只打开一次与交付范围对应的正常业务页面：整本用 `review_book({})`，指定章节用 `review_chapter({chapter_uuid})`，仅生成或交付一页（包括仅第一页）用 `review_page({chapter_uuid,page_uuid})`。导出不改变展示范围，不在章节或单页之后追加整本审阅。不等待用户打开或操作 App，不新增确认节点；用户明确要求的中途审阅仍保留。

工具继续使用统一 JSON 信封，`data.items` 只携带当前采用的 `asset_uuid`、正文和真实 `open_url`。可选 `chapter_uuid` 为向后兼容的范围参数；新增 `scope`（`book`/`chapter`）、`total`，各页含 `chapter_title` 和正文 `page_number`（封面/封底无正文页码）。`target_uuid` 仍为项目 UUID。默认每批 20 页，按 `cursor_pagination` 继续读取；App 自动从首批读全后展示，跨章节保持顺序。模型需要完整文字时也必须继续分页，不能把首批当整本。

App 通过只读 `read_app_image` 加载 384/1024 最长边预览；缩略图在 App 内进入单页阅读。完整原图经工具返回的 Lumi 链接查看。UI 失败仍交付文本摘要、准确导出路径及 Web 入口；不改变 `import_image` 立即导入/采用语义。

### 四层共享工作区

- 作品简介、规格、进度：`review_book({})`。
- 章节编号、标题、摘要、页数、图片完成数：`review_chapters({limit:20})`。
- 章节工作台：`review_chapter({chapter_uuid})` 保持章节范围，默认选中第一页，支持章内切页。
- 单页当前图片、完整正文、分镜和图片版本：`review_page({chapter_uuid,page_uuid})`。App 保留左侧本章 section 快速切换，并支持返回章节和章内上一页/下一页；章节视图也保留该侧栏。
- 图片在正常章节和单页页面展示；点击大图、候选稿预览和版本选择继续使用现有页面能力。
- 设定与候选仍用 `review_settings` / `review_candidates`。本地预览不会改变采用结果；界面显式采用会保存。对话编号选择重读后再 `select_image`。

新调用不传 `mode`，不使用 reader 视图。旧指定章 `review_book({chapter_uuid})`、旧 preview 参数和历史 reader 结果都映射到同一章节的正常页面；旧整本入口映射到作品总览。工具不接收任意项目或路径，Web 入口使用返回的实际 URL。


`review_*` 的 `contract_version:3` 是小型启动描述，包括项目/目标 UUID、revision、标题、简短摘要、状态与实际 Web URL。四层界面实际导入 Web 的同一套页面主体，普通编辑、排序、软删除/恢复、脚本与图片版本操作直接经宿主桥接完成，不要求用户每次回聊天。

完整业务对象通过 App-only `read_app_resource` / `write_app_resource` 的 `_meta["lumi/app_response"]` 交给 UI；图片亦在 `_meta`，不复制到模型可见文本或 structuredContent。模型要读具体正文时使用原有 `chapters/pages/settings` 读取工具。App-only 可见性不保证宿主一定排除上下文，真实宿主验证状态见仓库验收文档。

文件导入支持宿主附件单次 6 MiB；下载交给宿主打开同一 Lumi 资源。AI 生成、聊天引用和系统配置保留“在 Lumi 中打开”入口，不自动触发生成或切换付费渠道。宿主不可用不阻塞用户已授权的创作流程。
<!-- host:claude -->
## 交付与 Web 链接

完成用户要求的创作、保存和所需导出后，只调用一次与交付范围对应的 `web_url`：整本 `web_url({})`，指定章节 `web_url({chapter_uuid})`，仅生成或交付一页（包括仅第一页）`web_url({page_uuid})`。导出不改变交付范围，不在章节或单页之后追加整本链接。不等待用户打开链接，不新增确认节点。

```json
{"name":"web_url","arguments":{"page_uuid":"01989abc-def0-7000-8000-000000000002"}}
```

返回 `{project_uuid, url, scope, note}`，按范围附 `chapter_uuid/page_uuid/setting_uuid`。`view` 可取 `chapters`、`settings`、`exports`、`providers`，与 UUID 范围互斥；页面与 `chapter_uuid` 不匹配时报 `invalid_arguments`。url 使用当前后台的实际端口，原样交给用户，不拼写或缓存。

用户在网页中编辑后回到对话时，先 get 相关对象并取最新 revision；写入冲突时重读并合并，不覆盖网页修改。

`generate_page_image` 返回 `{task, done, page_uuid, chapter_uuid, revision, image:{variant_uuid,version_no,asset_uuid}, is_current_image, read_image, next_action}`；`done:false` 时只含 task、page_uuid 和 next_action，用相同参数和 key 重放。所有字段只含公开 UUID。
<!-- /host -->

章节交付的 `export_book` 传目标 `chapter_uuid`；省略即整书。三种格式均返回 scope、page_count、missing_image_count。

## 整本阅读网站

用户说“生成网站”“导出成网站”“更新网站”，或上下文已明确是网站导出时，直接调用 `export_website({})`，不预查状态、不区分首次和更新、不重复确认。只说“导出绘本”、指定 PDF / ZIP / Markdown，或询问“能不能生成网站”，都不触发。它生成整本作品的本地静态网站；公网发布须另行提出。

默认不允许缺图。用户已明确接受“只导出已有图片”时直接传 `allow_missing_images=true`；否则仅在收到 `comic_export_incomplete` 后报告实际缺图数量，询问是否继续。收到 `comic_export_empty` 就说明至少需要一张有效正文图，不建议跳过。超时、断线或 `done=false` 时用原参数继续同一任务；如果传了 `idempotency_key`，重试须保持原键。只有 `done=true` 才交付返回的绝对 `output_directory` 和可打开的 `preview_url`，不以旧站仍能访问判断成功。
