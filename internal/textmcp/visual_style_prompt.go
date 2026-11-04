package textmcp

import "lumi/internal/promptcatalog"

// Host-only visual guidance: no tool workflow belongs in image-model inputs.
// Shared Web catalog defaults and saved user templates remain unchanged.
func hostVisualStylePrompt(language string) string {
	if promptcatalog.NormalizeLanguage(language) == promptcatalog.LanguageEnglish {
		return `## User direction and visual references
Explicit user requirements take precedence over reference traits; reference traits take precedence over agent-selected defaults. "Decide the details" does not authorize replacing the requested art style. Separate character identity and story choices from proportions, contour, palette, texture and lettering. Do not infer round heads, short limbs, plush fur, pastel colors or any other aesthetic from the reader's age, theme or a generic "cute/warm" label. Add visual requirements only when supported by the user or the inspected references.
Treat distinctive dialogue/caption lettering as part of a requested style reference: preserve stroke weight, letter shapes, spacing, baseline rhythm and speech-bubble treatment while using the new story's exact text and language. Do not replace it with generic bold/rounded typography merely for readability. Do not invent a font name or copy reference wording. Explicit user font/readability choices take precedence. Text-free setting boards remain text-free.
For setting boards, white cropping margins and grids constrain layout only; preserve the specified subject proportions, contours, palette and surface texture.`
	}
	return `## 用户要求与视觉参考
用户明确要求优先于参考图特征，参考图特征优先于代理自行选择的默认值。“细节你决定”不授权替换指定画风。分开处理角色身份、剧情选择与头身比例、轮廓、配色、质感、字体；不得由读者年龄、主题或“温暖可爱”等泛词推导大头短肢、毛绒、低饱和等审美要求。新增视觉要求必须有用户要求或实际看过的参考图作为依据。
用户指定画风参考中的独特对白／旁白字体也属于画风：延续笔画粗细、字形、字距、基线节奏与气泡表现，用本故事的准确文字和语言重新绘制。不要仅因易读而换成通用粗黑体或圆体，不猜字体名称，不复制参考原文。用户明确指定字体或可读性取舍时遵从用户。无字设定板仍不画字。
设定板的纯白裁切留边和网格只约束版式，不授权改造主体比例、线条、配色及表面质感。`
}

func hostVisualAuthoringPrompt(language string) string {
	workflow := `将画风和字体特征写入 overall_style 与页面分镜；生成前消除已保存画风、设定和分镜之间的冲突。导入后实际对照原参考图和用户要求，记录具体观察；导入成功、角色一致或笼统的“已核对”不构成画风验收。未通过的设定先纠正，再提交绑定当前版本和画风的 setting_reviews 给 prepare_page_image。首张正文须实际核对画风与字体后再生成余页。这是代理自检，不新增用户确认节点。`
	if promptcatalog.NormalizeLanguage(language) == promptcatalog.LanguageEnglish {
		workflow = `Record art and lettering traits in overall_style and page storyboards; resolve contradictions in saved style, settings and storyboards before generation. After import, compare actual setting images with original references and user requirements, recording concrete observations. Import success, consistent characters and a generic "checked" statement are not visual approval. Correct failed settings before page generation; submit version- and style-bound setting_reviews to prepare_page_image. Check the first page's art and lettering before generating more pages. This is agent review, not an additional user approval step.`
	}
	return hostVisualStylePrompt(language) + "\n\n" + workflow
}
