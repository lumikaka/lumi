package promptcatalog

import (
	"fmt"
	"strings"
)

// PictureBookOptions is deliberately catalog-local so prompt resolution does
// not depend on the project persistence package. Empty options preserve the
// base catalog behavior, while vertical-strip options retain the protected
// strip prompt suite byte for byte.
type PictureBookOptions struct {
	Format                string
	AspectWidth           int
	AspectHeight          int
	LargeImageMinimalText bool
	InteractionMode       string
	ComicLayout           string
}

func pictureBookDirective(language string, options PictureBookOptions) string {
	ratio := fmt.Sprintf("%d:%d", options.AspectWidth, options.AspectHeight)
	if NormalizeLanguage(language) == LanguageEnglish {
		base := "This project produces complete picture-book pages. Treat every section as one final page canvas at an aspect ratio of " + ratio + ". All visible copy is rendered directly into that image. Never render a book mockup, cover mockup, page photographed in space, or decorative frame around the page."
		switch options.Format {
		case "classic_picture_book":
			if options.LargeImageMinimalText {
				return base + " Use one dominant scene and zero or one very short sentence. The illustration must overwhelmingly dominate the page. Do not use comic panels."
			}
			return base + " Use one dominant scene and one to three short narrative sentences whose meaning works together with the illustration. Do not use comic panels."
		case "wordless_picture_book":
			return base + " Tell the story only through actions, expressions, props, and scene changes. Render absolutely no readable text, title, caption, dialogue, speech bubble, label, sign, or sound-effect lettering. Do not use comic panels."
		case "interactive_picture_book":
			return base + " Use one dominant scene and render exactly one concise, child-friendly interactive question or instruction. " + interactiveDirectiveEN(options.InteractionMode) + " Do not reveal the answer and do not imply clickable hotspots or branching software behavior. Do not use comic panels."
		case "comic_story":
			if options.ComicLayout == "four_panel" {
				return base + " Build exactly four sequential comic panels with a clear reading order, concise dialogue bubbles, and continuous character action."
			}
			return base + " Build a page-comic layout of three to six panels with varied panel sizes, a clear reading order, concise dialogue bubbles, and continuous character action."
		}
	}
	base := "本项目生成完整绘本页面。每个 section 都是一张最终页面画布，宽高比为 " + ratio + "。所有可见文字直接绘入该图片。禁止生成书本样机、封面样机、摆拍页面或围绕页面的装饰相框。"
	switch options.Format {
	case "classic_picture_book":
		if options.LargeImageMinimalText {
			return base + " 使用一幅占绝对主导的场景，正文为 0–1 句极短文字，插画必须占据绝对视觉主导。禁止漫画分格。"
		}
		return base + " 使用一幅主场景并绘入 1–3 句简短叙事文字，让文字和插画共同完成叙事。禁止漫画分格。"
	case "wordless_picture_book":
		return base + " 完全依靠动作、表情、道具状态和场景变化讲故事。绝对禁止任何可读文字、标题、旁白、对白、气泡、标签、招牌或音效字。禁止漫画分格。"
	case "interactive_picture_book":
		return base + " 使用一幅主场景，并且只绘入一条简短、适龄的互动问题或指令。" + interactiveDirectiveZH(options.InteractionMode) + "不要揭示答案，也不要暗示可点击热点或软件分支。禁止漫画分格。"
	case "comic_story":
		if options.ComicLayout == "four_panel" {
			return base + " 严格使用四个连续漫画分格，阅读顺序明确，气泡对白简短，角色动作前后连贯。"
		}
		return base + " 使用 3–6 格大小有变化的页漫布局，阅读顺序明确，气泡对白简短，角色动作前后连贯。"
	}
	return base
}

func pictureBookCoverDirective(language string, options PictureBookOptions) string {
	ratio := fmt.Sprintf("%d:%d", options.AspectWidth, options.AspectHeight)
	if NormalizeLanguage(language) == LanguageEnglish {
		return "This is the picture book's front cover, not a body page. Generate one flat final cover canvas at an aspect ratio of " + ratio + ". Body-page rules for wordlessness, narrative-sentence count, interactive questions, or comic-panel count do not apply to the cover; follow the exact title-only copy and cover-composition rules below. Never render a book mockup, 3D book, unfolded jacket, spine, or back cover."
	}
	return "这是绘本封面，不是正文页。生成一张宽高比为 " + ratio + " 的平面最终封面画布。正文页的无字、叙事句数、互动提问或漫画分格数量规则不适用于封面；封面只遵循下方逐字标题和封面构图规则。禁止生成书本样机、立体书、展开封套、书脊或封底。"
}

func pictureBookBackCoverDirective(language string, options PictureBookOptions) string {
	ratio := fmt.Sprintf("%d:%d", options.AspectWidth, options.AspectHeight)
	if NormalizeLanguage(language) == LanguageEnglish {
		return "This is the picture book's back cover, not a body page or front cover. Generate one flat final back-cover canvas at an aspect ratio of " + ratio + ". Body-page rules for wordlessness, narrative-sentence count, interactive questions, or comic-panel count do not apply to the back cover; render only copy explicitly required by the storyboard. Never render a book mockup, 3D book, unfolded jacket, spine, or front cover."
	}
	return "这是绘本封底，不是正文页或正封面。生成一张宽高比为 " + ratio + " 的平面最终封底画布。正文页的无字、叙事句数、互动提问或漫画分格数量规则不适用于封底；只绘制 storyboard 明确要求的文字。禁止生成书本样机、立体书、展开封套、书脊或正封面。"
}

func interactiveDirectiveZH(mode string) string {
	switch mode {
	case "make_a_choice":
		return "画面必须给出两个清晰可辨的选择，并询问读者会选择哪一个。"
	case "guess":
		return "画面必须给出足以观察和推理的线索，并邀请读者猜一猜。"
	case "follow_along":
		return "画面必须展示一个简单、安全、可以跟着做的动作，并邀请读者模仿。"
	default:
		return "画面必须包含一个不被高亮或圈出的寻找目标，并邀请读者找一找。"
	}
}

func interactiveDirectiveEN(mode string) string {
	switch mode {
	case "make_a_choice":
		return "Show two clearly distinguishable choices and ask which one the reader would choose."
	case "guess":
		return "Show observable clues that support a guess and invite the reader to guess."
	case "follow_along":
		return "Show one simple and safe physical action and invite the reader to follow along."
	default:
		return "Include one search target without highlighting or circling it, and invite the reader to find it."
	}
}

const pictureBookStoryboardZH = `{{picture_book_directive}}

根据当前绘本正文规划页面。

当前绘本（技术对象 chapter）：
{{chapter_context_json}}

当前 STORY.md：
<story-md>
{{story_md}}
</story-md>

原始剧情 / 页面脚本文本：
{{input_text}}

每页计划的视觉单位数量：
{{moment_count_plan}}

输出 JSON 字段：
{
  "chapter_code": "{{chapter_code}}",
  "title": "绘本标题",
  "sections": [
    {
      "section_no": 1,
      "title": "页面标题",
      "storyboard": "markdown 格式的完整页面脚本"
    }
  ]
}

规则：
- sections 数量必须在 1 到 {{max_section_count}} 之间，由剧情节奏自然拆分；一个 section 就是一页
- storyboard 必须是 markdown 文本，不要使用代码块
- 每页视觉单位数量必须严格遵循 moment_count_plan，单页不得超过 {{max_moments_per_section}}
- 明确描述页面核心剧情目标、构图、人物动作表情、场景变化、光影色彩、需要逐字绘入的文字以及阅读顺序
- 文字与画面规则必须服从开头的绘本形式约束
- 保持角色身份、服装、道具、场景空间和整体画风在相邻页面连续一致`

const pictureBookStoryboardEN = `{{picture_book_directive}}

Plan pages from the current picture-book prose.

Current picture book (technical chapter object):
{{chapter_context_json}}

Current STORY.md:
<story-md>
{{story_md}}
</story-md>

Original plot / page-script text:
{{input_text}}

Planned visual-unit count for each page:
{{moment_count_plan}}

Output JSON fields:
{
  "chapter_code": "{{chapter_code}}",
  "title": "picture-book title",
  "sections": [
    {
      "section_no": 1,
      "title": "page title",
      "storyboard": "complete page script in markdown"
    }
  ]
}

Rules:
- The number of sections must be between 1 and {{max_section_count}}, split naturally by story rhythm; one section is one page
- storyboard must be markdown text without code fences
- Each page must strictly follow moment_count_plan and may not exceed {{max_moments_per_section}} visual units
- Clearly describe the page's plot goal, composition, character action and expression, scene changes, light and color, exact visible copy, and reading order
- Text and visual behavior must obey the picture-book format directive at the beginning
- Keep character identity, costume, props, scene space, and overall art style continuous across adjacent pages`

const pictureBookBeforeImageZH = `{{picture_book_directive}}

## 通用页面生成规则

1. 当前输入是一张完整页面的 storyboard，生成一张可直接阅读和导出的最终页面图片。
2. 严格保持 storyboard 中的剧情、人物身份、动作、表情、场景、道具和逐字文字，不擅自补充对白或旁白。
3. 人物脸部、核心动作和关键道具不得被文字、特效或无关装饰遮挡。
4. 相邻页面中的人物发型、脸型、体型、服装结构、主色、武器和场景空间关系必须一致。
5. 有设定参考图时只参考其中主体身份与视觉特征，不得复制设定拼贴图的网格排版。
6. 使用清晰线稿、稳定色块、适龄且可读的构图；避免照片写实、海报、角色立绘、设定集和多张独立缩略图拼贴。
7. 任何需要绘入的文字都必须使用项目语言，字号清楚、对比充分、留白合理，并逐字保留 storyboard 内容。`

const pictureBookBeforeImageEN = `{{picture_book_directive}}

## General Page-generation Rules

1. The current input is the storyboard for one complete page. Generate one final page image ready for reading and export.
2. Preserve the storyboard's plot, character identities, actions, expressions, setting, props, and verbatim copy. Do not invent dialogue or narration.
3. Text, effects, and decoration must not cover faces, core actions, or key props.
4. Keep hairstyle, face shape, body type, costume structure, main colors, weapons, and scene-space relationships consistent across adjacent pages.
5. When setting references are present, use them only for subject identity and visual features; never copy the reference collage grid.
6. Use clear linework, stable color blocks, age-appropriate readable composition, and avoid photorealism, posters, character standees, setting sheets, or collages of independent thumbnails.
7. Any visible copy must use the project language, be clearly sized with sufficient contrast and whitespace, and preserve the storyboard wording verbatim.`

// DefinitionsForPictureBook returns the complete catalog. The protected
// vertical-strip branch returns Definitions directly, keeping every existing
// strip prompt byte unchanged.
func DefinitionsForPictureBook(language string, options PictureBookOptions) []Definition {
	definitions := Definitions(language)
	if options.Format == "" || options.Format == "vertical_strip" {
		return definitions
	}
	directive := pictureBookDirective(language, options)
	english := NormalizeLanguage(language) == LanguageEnglish
	for index := range definitions {
		definition := &definitions[index]
		switch {
		case definition.Group == GroupChapter && definition.Key == "comic_storyboard":
			definition.DefaultValue = strings.ReplaceAll(choosePictureBook(english, pictureBookStoryboardZH, pictureBookStoryboardEN), "{{picture_book_directive}}", directive)
			definition.Title = choosePictureBook(english, "绘本页面规划", "Picture-book page planning")
			definition.Description = choosePictureBook(english, "从绘本正文规划完整页面。", "Plan complete pages from picture-book prose.")
		case definition.Group == GroupChapter && definition.Key == "cover_storyboard":
			definition.DefaultValue = strings.TrimSpace(pictureBookCoverDirective(language, options) + "\n\n" + definition.DefaultValue)
		case definition.Group == GroupChapter && definition.Key == "cover_before_image":
			definition.DefaultValue = strings.TrimSpace(pictureBookCoverDirective(language, options) + "\n\n" + definition.DefaultValue)
		case definition.Group == GroupChapter && definition.Key == "back_cover_before_image":
			definition.DefaultValue = strings.TrimSpace(pictureBookBackCoverDirective(language, options) + "\n\n" + definition.DefaultValue)
		case definition.Group == GroupChapter && definition.Key == "before_image":
			definition.DefaultValue = strings.ReplaceAll(choosePictureBook(english, pictureBookBeforeImageZH, pictureBookBeforeImageEN), "{{picture_book_directive}}", directive)
			definition.Title = choosePictureBook(english, "页面图片基础规则", "Page image base rules")
			definition.Description = choosePictureBook(english, "组合进完整页面图片模板的基础规则。", "Base rules composed into a complete page image prompt.")
		case definition.Group == GroupStory:
			definition.DefaultValue = strings.TrimSpace(directive + "\n\n" + definition.DefaultValue)
		}
	}
	return definitions
}

func choosePictureBook(english bool, chinese, englishValue string) string {
	if english {
		return englishValue
	}
	return chinese
}

func LookupForPictureBook(group, key, language string, options PictureBookOptions) (Definition, bool) {
	group = strings.ToLower(strings.TrimSpace(group))
	key = strings.ToLower(strings.TrimSpace(key))
	for _, definition := range DefinitionsForPictureBook(language, options) {
		if definition.Group == group && definition.Key == key {
			return definition, true
		}
	}
	return Definition{}, false
}
