# Lumi MCP 调用参考

按需阅读：首次规格选择／写入先读「Setup 与规格」；创建、编辑时查「动作参数与版本」；调用模板时查「渲染与写回」。本参考随插件发布，配合当前工具声明使用。声明中的基础 required 不代表所有动作的必填项；条件必填由工具说明列出，服务端仍强制校验。

示例采用 MCP `tools/call` 的 `name` 和 `arguments` 形式；向对应宿主工具只传 `arguments`。示例 UUID 和 revision 仅为示意，必须替换成刚读取或渲染得到的值。`expected_revision` 是整数，合法的 `0` 不能当成缺失值；setup 草稿的 revision 从 1 开始。成功结果在 `data` 中，失败结果为 `success=false`、`data=null` 和 `error`，失败后按错误核对契约或重新读取事实。

## Setup 与规格

先 `project_overview`；未初始化且用户要求创建时，`project_init` 必填 `idea`，可选 `name`、`generation_language`（`zh-Hans` / `en`）。它创建 setup 草稿，已有项目不会重置。

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

定稿需要名称、生成语言、画风、创作要求和有效绘本规格完整；已有值直接沿用。`generation_scope` **不是该 MCP 工具的输入字段**，不要传入。服务端在 update 中固定 `project_only`，只表示不启动 Lumi 内部自动生成，不限制 Codex 继续完成故事、设定、分页和宿主生图。

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
| `pages list` | `chapter_uuid` | 支持游标，返回 `state.revision` |
| `pages get` | `chapter_uuid`、`page_uuid` | 返回页面及其 revision |
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
| 编辑／删除／选择单个资源 | 对应 get 返回的 `data.revision`，或适用于该目标的 render_prompt revision |

创建返回的新对象 revision **不是父集合 revision**。下一次创建前重新读取父集合；不能给整批复用同一个旧 revision，也不能把页面 revision 用于创建下一页。冲突后核对内容是否仍适用，不能只换版本号强行覆盖。

创建设定用 `asset_type/title/summary`，不用 `type/description_md`：

```json
{"name":"settings","arguments":{"action":"create","asset_type":"character","title":"小狐狸","summary":"橘红毛、白胸口，穿黄色雨衣。","expected_revision":0}}
```

创建页面时，`description_md` 保存正文、对白和旁白；`storyboard_md` 保存完整画面描述，包括构图、动作、表情、光线和逐字绘入的文字。不要把整份分镜 JSON 放入其中一个字段。

```json
{"name":"pages","arguments":{"action":"create","chapter_uuid":"01989abc-def0-7000-8000-000000000001","title":"发现星星","description_md":"小狐狸：我陪你回家。","storyboard_md":"雨夜森林，两格从上往下阅读。第一格发现草丛中的星星；第二格狐狸撑起叶子伞，气泡逐字写：我陪你回家。","page_role":"body","expected_revision":0}}
```

## 渲染与写回

先查 `prompt_catalog` 中实际的 `prompt_group`、`prompt_key` 与占位符。`render_prompt` 基础必填为 `prompt_group`、`prompt_key`、`target`；`variables` 是字符串值对象。不要猜模板名或覆盖工具自动填入的项目事实。

`prompt_group` 是模板分组：`story` / `chapter` / `premise` / `premise_style` / `agent` / `runtime`。**`target` 是读写目标，不是模板分组**：Story Profile 使用 `target="story_profile"`，不能用 `"story"`。

| target | 额外必填 | 写回工具／action 与版本 |
|---|---|---|
| `setup` | 无 | `project_setup update`；setup revision |
| `story_profile` | 无 | `story_profile write`；profile revision |
| `chapter` | `chapter_uuid` | `chapters write`；章节 revision |
| `pages` | `chapter_uuid` | `pages create`；页面集合 revision |
| `page` | `chapter_uuid`、`page_uuid` | 文本用 `pages update`；`chapter/section_image` 生图用 `generation_prompt` 和 `image_write_back` 导入，见 [页面生图提示词](page-images.md)；单页 revision |
| `premise` | 无 | `settings create`；设定集合 revision |
| `setting` | `setting_uuid` | `settings update`；单项设定 revision |
| `setting_board` | 无；只配合 `premise/setting_image` | `setting_boards import`；设定集合 revision；额外返回供宿主生图使用的 `generation_prompt` |
| `setting_breakdown` | `board_uuid`；只配合 `premise/asset_breakdown` | `apply_setting_breakdown`；批次 revision 与 `expected_premise_revision` |

```json
{"name":"render_prompt","arguments":{"prompt_group":"story","prompt_key":"story_profile","target":"story_profile","variables":{"chapter_count":"1"}}}
```

文本任务按返回的完整 `prompt` 执行，用 `write_back` 映射输出；页面生图使用 `generation_prompt` 和 `image_write_back`，片段组合使用 `fragment_prompt`，不要把外层任务说明传给生图工具。渲染不等于写入。`context_chapter_uuids` / `context_setting_uuids` 可缩小上下文，`[]` 表示省略该集合，目标自身仍包含。缺模板变量时根据目录及错误补齐；用户已明确的内容和执行授权继续沿用。

```json
{"name":"story_profile","arguments":{"action":"write","story_md":"# 回家的星星\n\n小狐狸帮助迷路的星星回家。","expected_revision":0}}
```

`review_candidates` 按 `kind` 决定参数：`page` 要 `chapter_uuid/page_uuid`；`setting` 要 `setting_uuid`；`text` 要 2–12 个 `candidates`，每项为 `{"title":"...","text":"..."}`。界面切换只预览，选择图片的持久化写回使用上表的 `select_image`。

```json
{"name":"review_candidates","arguments":{"kind":"text","candidates":[{"title":"温暖结局","text":"星星点亮了狐狸回家的路。"},{"title":"幽默结局","text":"星星约狐狸下次带一把更大的伞。"}]}}
```


## 批量设定图

执行前读 [批量设定图流程](batch-settings.md)。这些工具只保存、裁切和绑定图片；生成及视觉定位由 Codex 完成。

| 工具 | 参数与版本来源 |
| --- | --- |
| `setting_boards list` | `action=list`；支持 `before/after/limit`，返回批次 UUID、revision、时间 |
| `setting_boards get` | `action=get, board_uuid`；返回 source、setting_image（含 asset）、revision、latest_breakdown |
| `setting_boards import` | `action=import, path, source_text, prompt, expected_revision, idempotency_key`；可选 `style_snapshot`；revision 来自 `settings list` 的 `premise.revision` 或生成前的 `render_prompt`；原图不裁切 |
| `apply_setting_breakdown` | `board_uuid, expected_revision, expected_premise_revision, idempotency_key, assets`；分别使用批次 revision 和设定集合 revision；每批 1–6 项 |

每项 asset 使用 `type, title, summary, tags, confidence, crop_box`；`crop_box` 为原始整图上的归一化 `x/y/width/height`，不能重叠，`confidence` 至少 0.92。更新已有设定时再加 `setting_uuid, expected_revision`（该设定当前 revision）；省略这两个字段只允许创建新设定，同名会报冲突。返回 `items` 中的 `setting_uuid/variant_uuid/asset_uuid` 均为实际写入结果，`revision` 属于该设定。

同一次请求重试必须保留原幂等键和全部原参数，包括 revision。导入成功后即使宿主文件过期，原请求仍可重放；幂等键不能用于替换同路径下的新图片。修改输入或开始新的拆分使用新键。`latest_breakdown` 是最后一次提交的结果，不代表其图片仍是用户当前选择；审阅或更新前重读 `settings get`。


## 最终绘本预览

完成用户要求的创作、保存与 `export_book` 后调用 `review_book`。整本传 `{}`；指定章节传 `{"chapter_uuid":"刚读取的章节 UUID"}`。不等待用户打开或操作 App，不新增确认节点；用户明确要求的中途审阅仍保留。

工具继续使用统一 JSON 信封，`data.items` 只携带当前采用的 `asset_uuid`、正文和真实 `open_url`。可选 `chapter_uuid` 为向后兼容的范围参数；新增 `scope`（`book`/`chapter`）、`total`，各页含 `chapter_title` 和正文 `page_number`（封面/封底无正文页码）。`target_uuid` 仍为项目 UUID。默认每批 20 页，按 `cursor_pagination` 继续读取；App 自动从首批读全后展示，跨章节保持顺序。模型需要完整文字时也必须继续分页，不能把首批当整本。

App 通过只读 `read_app_image` 加载 384/1024 最长边预览；缩略图在 App 内进入单页阅读。完整原图经工具返回的 Lumi 链接查看。UI 失败仍交付文本摘要、准确导出路径及 Web 入口；不改变 `import_image` 立即导入/采用语义。
