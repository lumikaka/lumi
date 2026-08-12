package jobqueue

import (
	"context"
	"encoding/json"
	"strings"

	"lumi/internal/project"
	"lumi/internal/promptcatalog"
)

const (
	premiseSettingImagePromptKey       = "setting_image"
	legacyPremiseSettingImagePromptKey = "setting_generation"
	premiseAssetBreakdownPromptKey     = "asset_breakdown"
)

func defaultPremiseStyle(language string) string {
	return promptcatalog.DefaultProjectStyle(language)
}

func premiseLanguageInstruction(language string) string {
	return promptcatalog.LanguageInstruction(language)
}

func defaultPremisePrompt(key, language string) string {
	definition, ok := promptcatalog.Lookup(promptcatalog.GroupPremise, key, language)
	if !ok {
		return ""
	}
	return definition.DefaultValue
}

func loadPremisePromptTemplate(ctx context.Context, store *project.Store, key, language string) string {
	keys := []string{key}
	if key == premiseSettingImagePromptKey {
		keys = append(keys, legacyPremiseSettingImagePromptKey)
	}
	for _, candidate := range keys {
		var template string
		_ = store.DB().WithContext(ctx).Table("project_prompt_versions").
			Select("prompt").
			Where("project_id = (SELECT id FROM projects WHERE uuid = ?) AND prompt_group = 'premise' AND prompt_key = ?", store.ProjectUUID(), candidate).
			Order("version_no DESC").Limit(1).Scan(&template).Error
		if strings.TrimSpace(template) != "" {
			return strings.TrimSpace(template)
		}
	}
	return defaultPremisePrompt(key, language)
}

func renderPremisePrompt(template string, values map[string]string) string {
	rendered, err := promptcatalog.Render(template, values)
	if err != nil {
		return ""
	}
	return rendered
}

func renderPremiseSettingImagePrompt(template, inputText, style, language, languageInstruction string) string {
	if strings.TrimSpace(style) == "" {
		style = defaultPremiseStyle(language)
	}
	body := renderPremisePrompt(template, map[string]string{
		"input_text":   strings.TrimSpace(inputText),
		"style_prompt": strings.TrimSpace(style),
	})
	if strings.TrimSpace(languageInstruction) == "" {
		languageInstruction = premiseLanguageInstruction(language)
	}
	return promptcatalog.WithInstruction(body, languageInstruction)
}

func renderPremiseAssetBreakdownPrompt(template, inputText, style, language, languageInstruction string, imageInfo map[string]any) string {
	if strings.TrimSpace(style) == "" {
		style = defaultPremiseStyle(language)
	}
	encoded, _ := json.Marshal(imageInfo)
	styleName := "项目整体画风"
	if promptcatalog.NormalizeLanguage(language) == promptcatalog.LanguageEnglish {
		styleName = "Project overall art style"
	}
	body := renderPremisePrompt(template, map[string]string{
		"input_text":      strings.TrimSpace(inputText),
		"style_name":      styleName,
		"style_prompt":    strings.TrimSpace(style),
		"image_info_json": string(encoded),
	})
	if strings.TrimSpace(languageInstruction) == "" {
		languageInstruction = premiseLanguageInstruction(language)
	}
	return promptcatalog.WithInstruction(body, languageInstruction)
}
