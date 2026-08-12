package promptcatalog

const simpleCelStyleZH = `## 整体画风

成熟 2D 动画截图、赛璐璐上色、清晰线稿、简化五官、平涂色块、夸张轮廓、漫画分镜、干净边线、低纹理、少量阴影、简洁清爽。

不要照片级写实，不要真实皮肤毛孔，不要复杂布料纹理，不要油画厚涂，不要 3D 渲染，不要电影级写实光影，不要厚重暗部，不要过度细节，不要暗黑概念设定图，不要低幼廉价平涂卡通。`

const simpleCelStyleEN = `## Overall Art Style

Mature 2D animation screenshot look, cel shading, clean line art, simplified facial features, flat color blocks, exaggerated silhouettes, comic paneling, crisp outlines, low texture, limited shadows, clean and refreshing.

Do not use photorealism, realistic skin pores, complex fabric texture, thick oil-paint impasto, 3D rendering, cinematic realistic lighting, heavy dark shadows, excessive detail, dark concept-art design, or cheap childish flat-color cartooning.`

const hongKongStyleZH = `## 整体画风

香港漫画风格，强调强烈的动作张力、硬朗夸张的人物造型与高对比度的明暗表现，常以密集线条、速度线和爆炸式构图营造热血冲击感。
色彩通常浓烈鲜明，角色表情戏剧化，融合武侠、黑帮、都市江湖与英雄传奇气质，呈现出豪迈、激烈、充满街头能量的视觉风格。`

const hongKongStyleEN = `## Overall Art Style

Hong Kong comics style, emphasizing strong action impact, rugged exaggerated character design, and high-contrast light and shadow, often using dense linework, speed lines, and explosive compositions to create a hot-blooded sense of force.
Colors are usually intense and vivid, character expressions are dramatic, and the style blends wuxia, gangland, urban jianghu, and heroic-legend energy, producing a bold, fierce, street-powered visual style.`

const minimalJapaneseStyleZH = `## 整体画风
简约的日系动漫手绘风，线条比较随意、轻淡，有草稿感。
配色低饱和，人物表情呆萌冷淡，整体氛围清新又有点慵懒。`

const minimalJapaneseStyleEN = `## Overall Art Style
Minimal Japanese anime hand-drawn style, with casual, light lines and a sketch-like feeling.
Low-saturation colors, cute but detached character expressions, and an overall mood that feels fresh and slightly lazy.`

func premiseStyleDefinitions(language string) []Definition {
	english := language == LanguageEnglish
	choose := func(chinese, englishValue string) string {
		if english {
			return englishValue
		}
		return chinese
	}
	meta := func(key, zhTitle, enTitle, value string) Definition {
		title := zhTitle
		description := "Premise 内置画风。"
		if english {
			title = enTitle
			description = "Built-in Premise art style."
		}
		return Definition{Group: GroupPremiseStyle, Key: key, Title: title, Description: description, PromptType: PromptTypePreset, DefaultValue: value}
	}
	overallTitle, overallDescription := "项目整体画风", "Premise、设定图与 Section 图片共用的项目整体画风。"
	if english {
		overallTitle, overallDescription = "Project overall art style", "Project-wide art style shared by Premise, setting images, and Section images."
	}
	return []Definition{
		{Group: GroupPremiseStyle, Key: "project_overall_style", Title: overallTitle, Description: overallDescription, PromptType: PromptTypeFragment, DefaultValue: choose(simpleCelStyleZH, simpleCelStyleEN)},
		meta("simple_cel_anime", "简绘赛璐璐动漫", "Clean Cel Anime", choose(simpleCelStyleZH, simpleCelStyleEN)),
		meta("hong_kong_comic", "香港漫画风格", "Hong Kong Comics Style", choose(hongKongStyleZH, hongKongStyleEN)),
		meta("minimal_japanese_handdrawn", "简约日系手绘", "Minimal Japanese Hand-drawn", choose(minimalJapaneseStyleZH, minimalJapaneseStyleEN)),
	}
}

func DefaultProjectStyle(language string) string {
	definition, _ := Lookup(GroupPremiseStyle, "project_overall_style", language)
	return definition.DefaultValue
}
