package project

import "strings"

const (
	GenerationLanguageSimplifiedChinese = "zh-Hans"
	GenerationLanguageEnglish           = "en"
	DefaultGenerationLanguage           = GenerationLanguageSimplifiedChinese
)

func NormalizeGenerationLanguage(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "", "zh", "zh-CN", "zh_CN", "zh-Hans", "zh_Hans", "zh-HANS":
		return DefaultGenerationLanguage, true
	case "en", "en-US", "en_US", "english":
		return GenerationLanguageEnglish, true
	default:
		return "", false
	}
}

func GenerationLanguageInstruction(value string) string {
	language, valid := NormalizeGenerationLanguage(value)
	if !valid {
		language = DefaultGenerationLanguage
	}
	if language == GenerationLanguageEnglish {
		return "Project language: English.\nWrite all newly generated human-readable story content, premise content, titles, dialogue, captions, sound effects, summaries, tags, explanations, and JSON string values in English. Keep JSON keys, API field names, enum values, ids, filenames, URLs, code-like identifiers, and any existing project/user-provided names or titles that must be referenced exactly as required by the schema or source content."
	}
	return "项目语言：简体中文。\n所有新生成的可读故事内容、设定内容、标题、对白、旁白、音效、简介、标签、说明和 JSON 字符串值都使用简体中文。JSON key、API 字段名、枚举值、id、文件名、URL、代码式标识符，以及必须精确引用的既有项目/用户提供名称或标题，都必须保持 schema 或来源内容要求的原样。"
}

func GenerationLanguageVisualInstruction(value string) string {
	language, valid := NormalizeGenerationLanguage(value)
	if !valid {
		language = DefaultGenerationLanguage
	}
	if language == GenerationLanguageEnglish {
		return "Any visible words, captions, signs, labels, dialogue, or sound effects in the generated image must be in English. Keep established proper nouns unchanged."
	}
	return "生成图片中出现的可见文字、旁白、招牌、标签、对白或音效必须使用简体中文；既有专有名词保持原样。"
}

func GenerationLanguageLabel(value string) string {
	language, valid := NormalizeGenerationLanguage(value)
	if valid && language == GenerationLanguageEnglish {
		return "English"
	}
	return "简体中文"
}
