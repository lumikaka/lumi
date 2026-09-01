package promptcatalog

const premiseSettingImagePromptZH = `你是漫画项目的资产设定板设计师。请根据 Premise 生成一张供机器自动裁切的设定板。

优先级从高到低：可独立裁切 > 主体完整与可识别 > 画风表现 > 素材数量。

内容筛选：
- 只选择对剧情推进、世界观识别或后续复用最重要的 1 到 6 个设定项。
- 每个设定项必须是一个独立视觉主体：单个角色、单个完整道具、单个标志，或一个边界清楚的地点缩景。
- 不要把 premise 中的所有名词都画出来；省略路人、一次性小物、泛泛背景、纯氛围装饰和重复元素。

机器裁切硬约束（优先于画风描述）：
- 画布从边缘到边缘必须是均匀纯白 #FFFFFF；禁止米白、纸纹、渐变、边框、卡片、分隔线、色块、地台和跨主体阴影。
- 使用不可见的 3 列 × 2 行固定网格。每个主体独占一个格子；未使用的格子保持纯白。禁止绘制网格线或格子边框。
- 每个格子内，主体及其所有非白像素必须完整落在中央 76% 区域，四周各保留至少 12% 的纯白安全边距。
- 一个格子只能有一个主体。不同格子的主体、轮廓、武器、配件、阴影、粒子和光效都不得接触、重叠或跨格。
- 角色必须完整展示头发、四肢、脚部和不可分割的随身装备；道具必须完整装配；地点必须画成独立、边界闭合的缩景，不能变成覆盖整张画布的共享背景。
- 禁止分类合集、横向物件清单、零件散放、多人组合、连续叙事场景和拥挤海报构图。
- 禁止所有可见文字：标题、标签、编号、说明、图例、水印、署名、UI、拟声词以及任何类似字符的符号都不能出现；即使主体是标志或徽记，也不得包含可读文字。

构图要求：
- 主体尽量使用正面或稳定的三分之四视角，轮廓清楚，尺寸均衡，彼此保持宽阔白色间隔。
- 保留指定画风中的造型、线条、材质与色彩，但不要让画风要求破坏纯白背景、固定网格和安全边距。
- 最终只输出设定板图片，不要输出说明文字。

{{style_prompt}}

## Premise 文本

{{input_text}}`

const premiseSettingImagePromptZHPrevious = `你是漫画项目的设定图设计师。根据以下 premise 文本生成一张用于漫画制作的设定图。

先做筛选：
- 只生成对剧情推进、世界观识别或后续资产复用有明确价值的设定项，例如主要人物、核心地点、关键单体道具、重要势力标志、反复出现的视觉母题。
- 不要把 premise 里的所有名词都画出来；省略路人、一次性小物件、泛泛背景、纯氛围装饰、与主线关系弱的细节。
- 重要设定项优先控制在 6 到 12 个；如果 premise 信息量很大，最多可以到 16 个；如果 premise 本身很简短，可以少于 6 个。

版式要求：
- 输出应像可切图的资产设定板，不是说明海报、图鉴页或剧情场景图。
- 整张图必须使用纯白背景，像白色画布上的独立资产排布；不要米色、泛黄纸张、灰底、渐变、纹理、花纹、装饰边框、分隔线、底色块或背景阴影。
- 每个设定项独占一个清晰区域，作为单独完整主体出现；主体之间留出明显空白带和安全裁切边距。
- 避免元素重叠、遮挡、透视穿插、跨元素光效、共用底座、连续叙事场景、拥挤海报式构图或大背景覆盖全图。

禁止组合项：
- 不要生成“关键道具”“主要人物”“地点设定”“势力徽章”这类分类合集栏、编号章节、横向物件清单或带大标题的组合区块。
- 不要把多个互不从属的角色、道具、地点、符号或小物件合并成一个“组合项”。
- 不要画“若干零件”“一组工具”“一套法器”“杂物包”这种散放组合；复杂物件应画成完整装配后的单体，或只选择其中最关键的一件。
- 同一主体的随身物、武器或零件只有在不可分割时才可贴近主体一起出现，且不能形成独立合集。

文字要求：
- 每个设定项必须在主体附近放一个简短标题，标题必须使用当前项目语言，且应是该主体的名称，例如人名、地点名、关键道具名、势力名或视觉母题名。
- 标题只用于标注单个主体；不要添加分类标题、编号、章节名、图例、长说明文字或多物件统一标签。

{{style_prompt}}

## Premise 文本

{{input_text}}`

const premiseSettingImagePromptEN = `You are the asset setting-board designer for a comic project. Generate one board from the Premise for reliable automatic machine cropping.

Priority, from highest to lowest: independently cuttable subjects > complete and recognizable subjects > art-style expression > number of assets.

Content selection:
- Select only the 1 to 6 setting items most important to plot progression, world recognition, or later reuse.
- Every setting item must be one independent visual subject: one character, one fully assembled prop, one symbol, or one clearly bounded location miniature.
- Do not draw every noun from the premise; omit passersby, one-off small objects, generic backgrounds, purely atmospheric decoration, and duplicates.

Hard machine-cropping constraints (these override art-style directions):
- The canvas must be uniform pure white #FFFFFF from edge to edge. No off-white paper, texture, gradient, border, card, separator, color panel, pedestal, or shadow shared across subjects.
- Use a fixed invisible 3-column × 2-row grid. Each subject occupies exactly one cell; unused cells remain pure white. Do not draw grid lines or cell borders.
- Inside each cell, the subject and every non-white pixel belonging to it must stay within the central 76%, leaving at least 12% pure-white safety margin on all four sides.
- Each cell contains exactly one subject. Subjects, silhouettes, weapons, accessories, shadows, particles, and light effects from different cells must never touch, overlap, or cross cell boundaries.
- A character must include all hair, limbs, feet, and inseparable carried equipment; a prop must be fully assembled; a location must be an isolated, closed-boundary miniature rather than a shared background spanning the canvas.
- No category collections, horizontal object lists, scattered parts, multi-character groups, continuous narrative scenes, or crowded poster layouts.
- No visible text of any kind: no title, label, number, explanation, legend, watermark, credit, UI, sound effect, or text-like symbol. Even a selected symbol or emblem must not contain readable lettering.

Composition requirements:
- Prefer front or stable three-quarter views with clear silhouettes, balanced subject scale, and wide white gaps between cells.
- Preserve the requested shapes, linework, materials, and colors, but never let style directions violate the pure-white background, fixed grid, or safety margins.
- Output only the setting-board image, with no explanatory text.

{{style_prompt}}

## Premise Text

{{input_text}}`

const premiseSettingImagePromptENPrevious = `You are the setting image designer for a comic project. Generate a setting image for comic production from the following premise text.

First filter the subjects:
- Only generate setting items that have clear value for plot progression, world recognition, or later asset reuse, such as main characters, core locations, key single props, important faction emblems, and recurring visual motifs.
- Do not draw every noun in the premise; omit passersby, one-off small objects, generic backgrounds, purely atmospheric decoration, and details weakly related to the main plot.
- Prefer keeping important setting items between 6 and 12; if the premise contains a lot of information, at most 16; if the premise is very short, fewer than 6 is acceptable.

Layout requirements:
- The output should look like a cuttable asset setting board, not an explanatory poster, encyclopedia page, or plot scene image.
- The whole image must use a pure white background, like independent assets arranged on a white canvas; do not use beige, yellowed paper, gray backgrounds, gradients, textures, patterns, decorative borders, separators, base color blocks, or background shadows.
- Each setting item must occupy its own clear area and appear as an independent complete subject; leave obvious white-space bands and safe crop margins between subjects.
- Avoid overlapping elements, occlusion, perspective interpenetration, cross-element light effects, shared bases, continuous narrative scenes, crowded poster composition, or a large background covering the whole image.

Forbidden combined items:
- Do not generate category collection bars, numbered chapters, horizontal object lists, or large-title combination blocks such as "Key Props", "Main Characters", "Location Settings", or "Faction Emblems".
- Do not merge multiple unrelated characters, props, locations, symbols, or small objects into one "combined item".
- Do not draw scattered combinations such as "several parts", "a set of tools", "a set of ritual implements", or "miscellaneous bag"; complex objects should be drawn as a fully assembled single object, or only the most important component should be selected.
- Personal belongings, weapons, or parts belonging to the same subject may appear close to that subject only when inseparable, and must not form an independent collection.

Text requirements:
- Each setting item must have a short title near the subject. The title must use the current project language and should be the subject's name, such as a character name, location name, key prop name, faction name, or visual motif name.
- Titles are only for labeling single subjects; do not add category titles, numbers, chapter names, legends, long explanatory text, or unified labels for multiple objects.

{{style_prompt}}

## Premise Text

{{input_text}}`

const premiseSettingReferenceUsageZH = `## 创建参考图

图片输入是一张由用户参考图组成的带标签参考板。请根据以下冻结计划使用各图片：
{{reference_plan}}

- character：保留身份、外貌、服装和标志性特征。
- scene：参考空间关系、建筑、材质、光照和环境气氛。
- prop：参考物件的形态、结构、材质和颜色。
- style：只参考线条、色彩、纹理和光照，不复制主体或构图。
- auto：作为通用视觉灵感，结合 premise 决定合理用途。

只参考各 tile 内的原始视觉内容；不要复制参考板的网格、标签、编号、留白或排版。最终仍需严格遵守上方设定图的白底、独立主体和可裁切要求。`

const premiseSettingReferenceUsageEN = `## Creation references

The image input is one labeled board composed from the user's reference images. Use each image according to this frozen plan:
{{reference_plan}}

- character: preserve identity, appearance, clothing, and signature traits.
- scene: reference spatial relationships, architecture, materials, lighting, and atmosphere.
- prop: reference form, construction, materials, and colors.
- style: reference only linework, palette, texture, and lighting; do not copy subjects or composition.
- auto: treat as general visual inspiration and infer a reasonable use from the premise.

Use only the original visual content inside each tile. Do not copy the board grid, labels, numbering, whitespace, or layout. The final image must still obey the white-background, independent-subject, cuttable setting-board requirements above.`

const premiseAssetBreakdownPromptZH = `你是漫画制作的视觉资产定位器。分析输入设定图，为每个重要且可复用的单一主体给出可直接执行的裁切框。

关键执行事实：系统会逐字使用 crop_box 做矩形裁切；不会自动添加 padding，不会吸附前景，不会去除背景，也不会执行 plan、tool_options 或 quality_checks。因此 crop_box 必须是包含安全边距的最终可用框，不能只是大致区域。

只返回一个 JSON object，不要返回 Markdown、解释或 schema 之外的顶层字段。格式：
{
  "assets": [
    {
      "type": "character",
      "title": "设定项名称",
      "summary": "可脱离图片搜索的简短简介",
      "tags": ["角色", "主角"],
      "crop_box": {"x": 0.1000, "y": 0.1000, "width": 0.2000, "height": 0.3000},
      "confidence": 0.9500
    }
  ]
}
示例中的中文字符串只表示字段含义；所有可读字符串必须使用当前项目语言。type 的枚举值和 JSON key 保持英文原样。

定位与裁切规则：
1. 返回 1 到 6 个最重要、最可复用且可以安全独立裁切的主体；一个 asset 只能包含一个主体。
2. 直接观察实际画面中的像素边界，不要假设图片严格遵循网格，也不要按等分格子机械切图。
3. 先定位主体全部可见内容的最小边界，包括头发、四肢、脚部、不可分割的武器或随身物、道具全部结构，以及属于该主体的局部阴影和光效。
4. 在该最小边界的四边各外扩主体宽度或高度的 5% 作为纯白安全边距，再换算为整图 0 到 1 的归一化 x、y、width、height；坐标尽量保留四位小数，并裁剪到 [0,1] 范围内。
5. 最终 crop_box 不得切断主体，不得包含标题、标签、编号、水印、边框、色块、相邻主体或超出安全边距的过量无关空白；不得把纯白背景本身作为 asset。不同 asset 的 crop_box 不得重叠。
6. 如果一个候选区域无法在不切断主体的前提下排除文字、装饰或相邻主体，则不要输出该候选项；只保留 confidence >= 0.92 的结果。
7. 不要把多个角色、物件清单、散放零件、合集栏、共享场景背景或纯白画布作为一个 asset。完整装配的复杂物件可以作为一个 asset。
8. title 必须根据 Premise 语义和主体身份生成，稳定、简短、符合当前项目语言；同一 assets 列表内不得重复，重复运行时同一主体应保持相同 title。不要依赖图片中的 OCR 文字，不要把识别到的标签或编号当作 title。
9. summary 必须能脱离图片独立搜索，且不得臆造图片和 Premise 均不支持的信息。
10. tags 只输出 1 到 3 个高区分度短词；第一个 tag 表示主要类型。中文优先使用角色、地点、道具、势力、标志、生物、服装、载具、视觉母题；英文使用对应短词。不要把 title、颜色、材质、姿态或细碎描述堆入 tags。
11. type 只能是 character、scene、prop、reference：角色和生物用 character，地点与场景用 scene，道具、服装和载具用 prop，其余重要视觉参考用 reference。
12. confidence 表示裁切框完整包含一个主体、排除相邻内容且边界判断可靠的置信度；不要为了凑数量降低标准。

本地图像元信息（只用于核对整图尺寸，不能替代视觉定位）：
{{image_info_json}}

画风：
{{style_name}}
{{style_prompt}}

Premise 文本：
{{input_text}}`

const premiseAssetBreakdownPromptZHPrevious = `你是漫画制作资产整理员。请直接理解输入设定图，并承担完整的拆解决策：读取画面内容、判断版式、选择拆解策略、生成候选区域、过滤合并区域、分类命名、输出裁切坐标和质量检查结果。

系统会把你输出的 crop_box 交给本地图像工具裁切 PNG，并生成 manifest、检测框预览、contact sheet 和 report。本地图像工具不负责理解图片或决定拆解策略，只执行你的 JSON 结果。

返回一个 JSON object，且只返回 JSON。格式：
{
  "plan": {
    "layout": "grid|sections|white_background_objects|complex_collage",
    "strategy": ["semantic_regions", "connected_components", "blank_band_split", "multi_strategy_merge"],
    "tool_options": {"padding_ratio": 0.04, "merge_related_parts": true, "remove_background": false},
    "quality_focus": ["avoid_cutting_subjects", "avoid_duplicate_crops"],
    "notes": ["short note"]
  },
  "assets": [
    {
      "filename": "character-main.png",
      "title": "设定项名称",
      "summary": "可被搜索的简介，包含外观、身份、用途、与故事的关系",
      "tags": ["角色", "主角"],
      "crop_box": {"x": 0.0, "y": 0.0, "width": 0.5, "height": 0.5},
      "confidence": 0.82
    }
  ],
  "quality_checks": ["是否主体被切断", "是否空白过多"]
}
示例 JSON 中的中文字符串只是字段含义占位；实际输出必须使用当前项目语言。

规则：
- 只拆解重要且可复用的设定项，忽略装饰性背景、纯氛围碎片、重复小物件和与主线关系弱的元素。
- 纯白背景只是画布，不是设定项；不要把背景、纸纹、边框、分隔线、底色块或阴影底板输出为 asset。
- 不要把分类标题、编号章节、横向物件清单或“关键道具”这类合集栏当作一个 asset；如果画面里出现合集栏，应只选其中真正重要的单个主体分别拆解。
- 不要输出由多个互不从属主体拼成的组合 asset；不要输出“若干零件”“一组工具”“一套法器”“杂物包”这类散放组合。
- 如果复杂物件被画成零件散放，只拆最关键的单个部件；如果部件已经装配成完整单体，则按完整单体拆解。
- assets 数量优先控制在 6 到 12 个；如果画面中的重要设定项很多，最多可以到 16 个；如果重要设定项更少，可以少于 6 个。
- filename 使用 kebab-case，扩展名使用 .png。
- title 必填，使用稳定、简短、可读且符合当前项目语言的设定项名称；同一 project 内 title 会作为唯一键，重复拆解时同 title 会覆盖旧设定项。
- tags 是设定项列表顶部的筛选项，不是搜索关键词堆砌；每个 asset 只输出 1 到 3 个高区分度且符合当前项目语言的短词。
- 第一个 tag 必须是主要类型；中文项目优先使用：角色、地点、道具、势力、标志、生物、服装、载具、视觉母题；英文项目使用对应英文短词，例如 Character、Place、Prop、Faction、Symbol、Creature、Costume、Vehicle、Motif。
- 额外 tag 最多 2 个，只保留确实适合跨多个 asset 筛选的稳定维度，例如主角、反派、科技、魔法、都市、荒野、宗教、军用。
- 不要把 title、人名、地名、一次性专名、颜色、材质、姿态、情绪、画风词或 summary 中的细碎描述放进 tags，除非它们会成为用户主动筛选的一组资产。
- 整个 assets 列表的唯一 tag 词表应尽量控制在 6 到 10 个，最多 12 个；如果某个词只会命中一个资产且 title 已能搜索到，通常不要作为 tag。
- summary 必须能脱离图片独立搜索。
- crop_box 使用 0 到 1 的归一化坐标，表示该设定项在整张图中的大致区域；无法判断时给空对象。
- crop_box 应覆盖完整主体，并包含武器、随身道具、局部光效等同一主体相关部分；裁切框之间尽量保持独立，避免把多个无关主体混在一起。
- plan 必须由你根据图片内容决定，不要依赖本地图像工具替你判断。
- strategy 可从 grid_cut、connected_components、blank_band_split、subject_segmentation、multi_strategy_merge、semantic_regions 中选择一个或多个。
- quality_checks 需要覆盖主体是否切断、空白是否过多、是否多个主体混在一起、是否漏掉重要区域、是否重复裁切。
- 不要臆造设定图中没有出现或 premise 文本没有支持的信息。

本地图像元信息（仅供参考，不能替代你对图片的视觉理解）：
{{image_info_json}}

画风：
{{style_name}}
{{style_prompt}}

Premise 文本：
{{input_text}}`

const premiseAssetBreakdownPromptEN = `You are a visual asset localizer for comic production. Analyze the input setting image and provide a directly executable crop for each important, reusable, single subject.

Critical execution fact: the system applies crop_box verbatim as a rectangular crop. It does not add padding, snap to foreground, remove backgrounds, or execute plan, tool_options, or quality_checks. Therefore crop_box must be the final usable box with its safety margin already included, never an approximate region.

Return exactly one JSON object. Do not return Markdown, explanations, or top-level fields outside this schema:
{
  "assets": [
    {
      "type": "character",
      "title": "setting item name",
      "summary": "short description searchable without the image",
      "tags": ["Character", "Protagonist"],
      "crop_box": {"x": 0.1000, "y": 0.1000, "width": 0.2000, "height": 0.3000},
      "confidence": 0.9500
    }
  ]
}
All human-readable strings must use the current project language. Keep type enum values and JSON keys exactly as shown in English.

Localization and cropping rules:
1. Return 1 to 6 of the most important, reusable subjects that can be safely cropped independently. One asset may contain only one subject.
2. Inspect actual visual pixel boundaries. Do not assume the image follows a perfect grid, and do not mechanically divide it into equal cells.
3. First locate the minimum box covering all visible subject content, including hair, limbs, feet, inseparable weapons or carried items, the entire prop structure, and local shadows or effects belonging to that subject.
4. Expand every side of that minimum box by 5% of the subject width or height as pure-white safety margin, then convert it to whole-image normalized x, y, width, and height in [0,1]. Prefer four decimal places and clip the result to the image bounds.
5. The final crop_box must not cut the subject and must exclude titles, labels, numbers, watermarks, borders, color panels, neighboring subjects, and excessive irrelevant whitespace beyond the safety margin. Never treat the pure-white background itself as an asset. Crop boxes for different assets must not overlap.
6. If a candidate cannot exclude text, decoration, or a neighboring subject without cutting the intended subject, omit it. Keep only results with confidence >= 0.92.
7. Never output multi-character groups, object lists, scattered parts, collection bars, a shared scene background, or the pure-white canvas as one asset. A fully assembled complex object may be one asset.
8. Derive title from the Premise meaning and subject identity. Keep it stable, short, and in the current project language; titles must be unique within the assets list, and the same subject should keep the same title across reruns. Do not rely on OCR text in the image or use a detected label or number as the title.
9. summary must be independently searchable and must not invent details unsupported by both the image and the Premise.
10. Output only 1 to 3 distinctive short tags. The first tag is the primary type. Prefer Character, Place, Prop, Faction, Symbol, Creature, Costume, Vehicle, or Motif for English output and their equivalents for Chinese output. Do not stuff titles, colors, materials, poses, or minor description details into tags.
11. type must be character, scene, prop, or reference: use character for characters and creatures, scene for places and environments, prop for props, costumes, and vehicles, and reference for other important visual references.
12. confidence measures whether the box fully contains exactly one subject, excludes neighboring content, and has reliable boundaries. Never lower the standard merely to reach a target count.

Local image metadata (use only to verify full-image dimensions; it cannot replace visual localization):
{{image_info_json}}

Art style:
{{style_name}}
{{style_prompt}}

Premise text:
{{input_text}}`

const premiseAssetBreakdownPromptENPrevious = `You are a comic-production asset organizer. Directly understand the input setting image and take full responsibility for the breakdown decisions: read the visual content, judge the layout, choose a breakdown strategy, generate candidate regions, filter and merge regions, classify and name assets, output crop coordinates, and provide quality-check results.

The system will pass your crop_box output to a local image tool to crop PNGs and generate a manifest, detection-box preview, contact sheet, and report. The local image tool does not understand the image or decide the breakdown strategy; it only executes your JSON result.

Return one JSON object, and only JSON. Format:
{
  "plan": {
    "layout": "grid|sections|white_background_objects|complex_collage",
    "strategy": ["semantic_regions", "connected_components", "blank_band_split", "multi_strategy_merge"],
    "tool_options": {"padding_ratio": 0.04, "merge_related_parts": true, "remove_background": false},
    "quality_focus": ["avoid_cutting_subjects", "avoid_duplicate_crops"],
    "notes": ["short note"]
  },
  "assets": [
    {
      "filename": "character-main.png",
      "title": "setting item name",
      "summary": "searchable summary including appearance, identity, purpose, and relation to the story",
      "tags": ["Character", "Protagonist"],
      "crop_box": {"x": 0.0, "y": 0.0, "width": 0.5, "height": 0.5},
      "confidence": 0.82
    }
  ],
  "quality_checks": ["whether subject is cut off", "whether there is too much whitespace"]
}
Chinese strings in the example JSON are only placeholders for field meaning; actual output must use the current project language.

Rules:
- Break down only important and reusable setting items; ignore decorative backgrounds, pure atmosphere fragments, repeated small objects, and elements weakly related to the main plot.
- Pure white background is only the canvas, not a setting item; do not output background, paper texture, borders, separators, base color blocks, or shadow plates as assets.
- Do not treat category titles, numbered chapters, horizontal object lists, or collection bars such as "Key Props" as one asset; if a collection bar appears in the image, select only the truly important single subjects within it and break them down separately.
- Do not output a combined asset made from multiple unrelated subjects; do not output scattered combinations such as "several parts", "a set of tools", "a set of ritual implements", or "miscellaneous bag".
- If a complex object is shown as scattered parts, break down only the most important single component; if parts are already assembled into a complete single object, break down the complete object.
- Prefer keeping assets between 6 and 12; if the image contains many important setting items, at most 16; if there are fewer important items, fewer than 6 is acceptable.
- filename must use kebab-case and the .png extension.
- title is required, stable, short, readable, and must match the current project language; within the same project, title is used as a unique key, and repeated breakdowns with the same title overwrite the old setting item.
- tags are top-level filters for the setting asset list, not a pile of search keywords; output only 1 to 3 highly distinctive short terms in the current project language for each asset.
- The first tag must be the main type; for Chinese projects, prefer: 角色、地点、道具、势力、标志、生物、服装、载具、视觉母题; for English projects, use corresponding English short terms such as Character, Place, Prop, Faction, Symbol, Creature, Costume, Vehicle, Motif.
- Additional tags are limited to at most 2 and should only keep stable dimensions suitable for filtering across multiple assets, such as Protagonist, Villain, Technology, Magic, Urban, Wilderness, Religion, Military.
- Do not put title, personal names, place names, one-off proper nouns, colors, materials, poses, emotions, art-style terms, or tiny details from summary into tags unless they form a group of assets users would actively filter for.
- The unique tag vocabulary for the whole assets list should preferably stay between 6 and 10 terms, at most 12; if a term hits only one asset and title can already find it, it usually should not be a tag.
- summary must be searchable independently of the image.
- crop_box uses normalized coordinates from 0 to 1 and represents the approximate region of this setting item in the whole image; if impossible to judge, return an empty object.
- crop_box should cover the complete subject and include weapons, personal props, local light effects, and other parts related to the same subject; keep crop boxes independent and avoid mixing multiple unrelated subjects.
- plan must be decided by you from the image content; do not rely on the local image tool to decide for you.
- strategy may choose one or more from grid_cut, connected_components, blank_band_split, subject_segmentation, multi_strategy_merge, semantic_regions.
- quality_checks must cover whether the subject is cut off, whether whitespace is excessive, whether multiple subjects are mixed together, whether important regions are missed, and whether crops are duplicated.
- Do not invent information that does not appear in the setting image or is not supported by the premise text.

Local image metadata (for reference only; it cannot replace your visual understanding of the image):
{{image_info_json}}

Art style:
{{style_name}}
{{style_prompt}}

Premise text:
{{input_text}}`

func premiseDefinitions(language string) []Definition {
	english := language == LanguageEnglish
	choose := func(chinese, englishValue string) string {
		if english {
			return englishValue
		}
		return chinese
	}
	meta := func(key, zhTitle, zhDescription, enTitle, enDescription, value string, legacy ...string) Definition {
		if english {
			return Definition{Group: GroupPremise, Key: key, Title: enTitle, Description: enDescription, PromptType: PromptTypeTemplate, DefaultValue: value, LegacyKeys: legacy}
		}
		return Definition{Group: GroupPremise, Key: key, Title: zhTitle, Description: zhDescription, PromptType: PromptTypeTemplate, DefaultValue: value, LegacyKeys: legacy}
	}
	settingImage := meta("setting_image", "Premise 设定图生成", "根据 Premise 文本和整体画风生成一张可拆解的设定图。", "Premise setting image generation", "Generate one setting image that can be broken down from Premise text and the overall art style.", choose(premiseSettingImagePromptZH, premiseSettingImagePromptEN), "setting_generation")
	settingImage.PreviousDefaultValues = []string{choose(premiseSettingImagePromptZHPrevious, premiseSettingImagePromptENPrevious)}
	assetBreakdown := meta("asset_breakdown", "Premise 设定项拆解", "根据设定图和 Premise 文本拆解可搜索、可复用的设定项。", "Premise setting asset breakdown", "Break down reusable searchable setting assets from the setting image and Premise text.", choose(premiseAssetBreakdownPromptZH, premiseAssetBreakdownPromptEN))
	assetBreakdown.PreviousDefaultValues = []string{choose(premiseAssetBreakdownPromptZHPrevious, premiseAssetBreakdownPromptENPrevious)}
	return []Definition{
		settingImage,
		{Group: GroupPremise, Key: "setting_reference_usage", Title: choose("创建参考图使用说明", "Creation-reference usage"), Description: choose("仅在 Premise 设定图包含首页参考图时追加。", "Appended only when a Premise setting image includes home-page references."), PromptType: PromptTypeFragment, DefaultValue: choose(premiseSettingReferenceUsageZH, premiseSettingReferenceUsageEN)},
		assetBreakdown,
		meta("single_asset_generation", "单项 AI 生成", "Premise ChatArea 创建新设定项或新图片版本时使用。", "Single asset generation", "Used by Premise ChatArea to create a setting item or image version.", choose(singleAssetPromptZH, singleAssetPromptEN)),
	}
}

const singleAssetPromptZH = `根据用户要求和当前 Premise 上下文生成一个主体明确、身份特征稳定、适合后续漫画视觉引用的正方形设定图。

用户要求：
{{input_text}}

Premise 上下文：
{{premise_context}}

{{style_prompt}}`

const singleAssetPromptEN = `Generate one square setting image with a clear subject, stable identity features, and strong suitability as a later comic visual reference, following the user request and current Premise context.

User request:
{{input_text}}

Premise context:
{{premise_context}}

{{style_prompt}}`
