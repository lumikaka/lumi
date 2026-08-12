package promptcatalog

const premiseSettingImagePromptZH = `你是漫画项目的设定图设计师。根据以下 premise 文本生成一张用于漫画制作的设定图。

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

const premiseSettingImagePromptEN = `You are the setting image designer for a comic project. Generate a setting image for comic production from the following premise text.

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

const premiseAssetBreakdownPromptZH = `你是漫画制作资产整理员。请直接理解输入设定图，并承担完整的拆解决策：读取画面内容、判断版式、选择拆解策略、生成候选区域、过滤合并区域、分类命名、输出裁切坐标和质量检查结果。

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

const premiseAssetBreakdownPromptEN = `You are a comic-production asset organizer. Directly understand the input setting image and take full responsibility for the breakdown decisions: read the visual content, judge the layout, choose a breakdown strategy, generate candidate regions, filter and merge regions, classify and name assets, output crop coordinates, and provide quality-check results.

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
	return []Definition{
		meta("setting_image", "Premise 设定图生成", "根据 Premise 文本和整体画风生成一张可拆解的设定图。", "Premise setting image generation", "Generate one setting image that can be broken down from Premise text and the overall art style.", choose(premiseSettingImagePromptZH, premiseSettingImagePromptEN), "setting_generation"),
		meta("asset_breakdown", "Premise 设定项拆解", "根据设定图和 Premise 文本拆解可搜索、可复用的设定项。", "Premise setting asset breakdown", "Break down reusable searchable setting assets from the setting image and Premise text.", choose(premiseAssetBreakdownPromptZH, premiseAssetBreakdownPromptEN)),
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
