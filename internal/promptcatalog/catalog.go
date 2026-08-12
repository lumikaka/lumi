package promptcatalog

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	LanguageChinese = "zh-Hans"
	LanguageEnglish = "en"

	GroupStory        = "story"
	GroupChapter      = "chapter"
	GroupPremise      = "premise"
	GroupPremiseStyle = "premise_style"
	GroupAgent        = "agent"
	GroupRuntime      = "runtime"

	PromptTypeTemplate = "template"
	PromptTypeFragment = "fragment"
	PromptTypePreset   = "preset"
)

// Definition describes one project-overridable prompt. DefaultValue is a
// complete builtin prompt for the requested project language.
type Definition struct {
	Group        string   `json:"prompt_group"`
	Key          string   `json:"prompt_key"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	PromptType   string   `json:"prompt_type"`
	DefaultValue string   `json:"default_value"`
	LegacyKeys   []string `json:"legacy_keys,omitempty"`
}

var placeholderPattern = regexp.MustCompile(`\{\{([^{}]*)\}\}`)

func NormalizeLanguage(language string) string {
	if strings.EqualFold(strings.TrimSpace(language), LanguageEnglish) {
		return LanguageEnglish
	}
	return LanguageChinese
}

func LanguageInstruction(language string) string {
	if NormalizeLanguage(language) == LanguageEnglish {
		return "Project language: English.\nWrite all newly generated human-readable story content, premise content, titles, dialogue, captions, sound effects, summaries, tags, explanations, and JSON string values in English. Keep JSON keys, API field names, enum values, ids, filenames, URLs, code-like identifiers, and any existing project/user-provided names or titles that must be referenced exactly as required by the schema or source content."
	}
	return "项目语言：简体中文。\n所有新生成的可读故事内容、设定内容、标题、对白、旁白、音效、简介、标签、说明和 JSON 字符串值都使用简体中文。JSON key、API 字段名、枚举值、id、文件名、URL、代码式标识符，以及必须精确引用的既有项目/用户提供名称或标题，都必须保持 schema 或来源内容要求的原样。"
}

func Definitions(language string) []Definition {
	language = NormalizeLanguage(language)
	definitions := make([]Definition, 0, 32)
	definitions = append(definitions, storyDefinitions(language)...)
	definitions = append(definitions, chapterDefinitions(language)...)
	definitions = append(definitions, premiseDefinitions(language)...)
	definitions = append(definitions, premiseStyleDefinitions(language)...)
	definitions = append(definitions, agentDefinitions(language)...)
	definitions = append(definitions, runtimeDefinitions(language)...)
	return definitions
}

func GroupDefinitions(group, language string) []Definition {
	group = strings.ToLower(strings.TrimSpace(group))
	all := Definitions(language)
	result := make([]Definition, 0, len(all))
	for _, definition := range all {
		if definition.Group == group {
			result = append(result, definition)
		}
	}
	return result
}

func Lookup(group, key, language string) (Definition, bool) {
	group = strings.ToLower(strings.TrimSpace(group))
	key = strings.ToLower(strings.TrimSpace(key))
	for _, definition := range Definitions(language) {
		if definition.Group == group && definition.Key == key {
			return definition, true
		}
	}
	return Definition{}, false
}

func Placeholders(value string) []string {
	seen := make(map[string]struct{})
	for _, match := range placeholderPattern.FindAllStringSubmatch(value, -1) {
		seen[match[1]] = struct{}{}
	}
	items := make([]string, 0, len(seen))
	for item := range seen {
		items = append(items, item)
	}
	sort.Strings(items)
	return items
}

// Render replaces every catalog placeholder and rejects incomplete requests.
// Extra values are permitted so a caller can share one context map across
// related prompts.
func Render(template string, values map[string]string) (string, error) {
	missing := make([]string, 0)
	for _, placeholder := range Placeholders(template) {
		value, ok := values[placeholder]
		if !ok {
			missing = append(missing, placeholder)
			continue
		}
		template = strings.ReplaceAll(template, "{{"+placeholder+"}}", value)
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("missing prompt placeholders: %s", strings.Join(missing, ", "))
	}
	if unresolved := Placeholders(template); len(unresolved) > 0 {
		return "", fmt.Errorf("unresolved prompt placeholders: %s", strings.Join(unresolved, ", "))
	}
	return strings.TrimSpace(template), nil
}

func WithLanguageInstruction(prompt, language string) string {
	return WithInstruction(prompt, LanguageInstruction(language))
}

func WithInstruction(prompt, instruction string) string {
	return strings.TrimSpace(strings.TrimSpace(instruction) + "\n\n" + strings.TrimSpace(prompt))
}
