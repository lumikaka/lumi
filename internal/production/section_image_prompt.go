package production

import (
	"context"
	"fmt"
	"strings"

	"lumi/internal/project"
	"lumi/internal/promptcatalog"
	"lumi/internal/story"
)

// RenderSectionImagePrompt is shared by durable Web snapshots and host preparation.
// Keep snapshot compatibility and whitespace stable: the resulting bytes are
// the prompt sent to the image model and saved with its output.
func RenderSectionImagePrompt(snapshot GenerationSnapshot, references []PremiseAssetReference) (string, error) {
	var err error
	prompt := ""
	if snapshot.Version >= 2 && strings.TrimSpace(snapshot.PromptTemplate) != "" {
		referenceUsage := sectionReferenceUsage(references, snapshot.GenerationLanguage)
		if snapshot.Version >= 3 {
			referenceUsage, err = RenderSectionReferenceUsage(references, snapshot.ReferencePresentPrompt, snapshot.ReferenceAbsentPrompt)
			if err != nil {
				return "", fmt.Errorf("Section 参考图说明提示词快照无法渲染: %w", err)
			}
		}
		prompt, err = promptcatalog.Render(snapshot.PromptTemplate, map[string]string{
			"style_prompt": snapshot.StyleSnapshot, "reference_usage_text": referenceUsage,
			"section_id": snapshot.ResourceUUID, "storyboard": snapshot.StoryboardMD,
		})
		if err != nil {
			return "", fmt.Errorf("Section 图片提示词快照无法渲染: %w", err)
		}
		languageInstruction := snapshot.LanguageInstruction
		if strings.TrimSpace(languageInstruction) == "" {
			languageInstruction = promptcatalog.LanguageInstruction(snapshot.GenerationLanguage)
		}
		prompt = promptcatalog.WithInstruction(prompt, languageInstruction)
		if guidance := strings.TrimSpace(snapshot.Prompt); guidance != "" && guidance != strings.TrimSpace(snapshot.StoryboardMD) {
			if snapshot.Version >= 3 {
				additional, renderErr := promptcatalog.Render(snapshot.AdditionalDirectionPrompt, map[string]string{"guidance_prompt": guidance})
				if renderErr != nil {
					return "", fmt.Errorf("Section 用户补充要求提示词快照无法渲染: %w", renderErr)
				}
				prompt += "\n\n" + additional
			} else if promptcatalog.NormalizeLanguage(snapshot.GenerationLanguage) == promptcatalog.LanguageEnglish {
				prompt += "\n\n## Additional user direction\n" + guidance
			} else {
				prompt += "\n\n## 用户补充要求\n" + guidance
			}
		}
	} else {
		// Version 1 Comic snapshots store the pre-catalog prompt pieces rather
		// than a rendered template; keep this deterministic compatibility path
		// solely for retries of those already-durable tasks.
		prompt = strings.TrimSpace(snapshot.Prompt + "\n\nStoryboard:\n" + snapshot.StoryboardMD + "\n\nStyle:\n" + snapshot.StyleSnapshot + "\n\n" + project.GenerationLanguageVisualInstruction(snapshot.GenerationLanguage))
		if len(references) > 0 {
			prompt += fmt.Sprintf("\n\nUse the single Section setting collage to keep visual continuity with its %d labeled premise items.", len(references))
		}
	}
	return prompt, nil
}

func sectionReferenceUsage(references []PremiseAssetReference, language string) string {
	if len(references) == 0 {
		if promptcatalog.NormalizeLanguage(language) == promptcatalog.LanguageEnglish {
			return "No setting reference images are available for this generation. Generate only from the storyboard and art style, and do not claim consistency with character or scene references that were not provided."
		}
		return "本次没有可用的设定参考图。仅根据 Storyboard 与画风生成，不要声称遵循未提供的角色或场景设定。"
	}
	titles := make([]string, 0, len(references))
	for index, reference := range references {
		titles = append(titles, fmt.Sprintf("%d. %s", index+1, reference.Title))
	}
	if promptcatalog.NormalizeLanguage(language) == promptcatalog.LanguageEnglish {
		return "One Section-specific setting collage image is provided. It contains the following setting items; use their labeled visual references to preserve identity and core visual features:\n" + strings.Join(titles, "\n")
	}
	return "本次提供一张 Section 专属设定拼贴图，其中包含以下带标签的设定项；请依据拼贴图保持它们的身份与核心视觉特征：\n" + strings.Join(titles, "\n")
}

func RenderSectionReferenceUsage(references []PremiseAssetReference, presentTemplate, absentTemplate string) (string, error) {
	if len(references) == 0 {
		return promptcatalog.Render(absentTemplate, map[string]string{})
	}
	titles := make([]string, 0, len(references))
	for index, reference := range references {
		titles = append(titles, fmt.Sprintf("%d. %s", index+1, reference.Title))
	}
	return promptcatalog.Render(presentTemplate, map[string]string{"reference_titles": strings.Join(titles, "\n")})
}

// EffectiveImageStyle preserves the Web's canonical prompt and legacy fallback.
func EffectiveImageStyle(ctx context.Context, store *project.Store, st *story.Service, legacyStyle, language string) (string, error) {
	style, err := st.EffectivePrompt(ctx, promptcatalog.GroupPremiseStyle, "project_overall_style")
	if err != nil {
		return "", err
	}
	var canonicalVersions int64
	if err := store.DB().WithContext(ctx).Table("project_prompt_versions").Where("project_id=(SELECT id FROM projects WHERE uuid=?) AND prompt_group=? AND prompt_key=?", store.ProjectUUID(), promptcatalog.GroupPremiseStyle, "project_overall_style").Count(&canonicalVersions).Error; err != nil {
		return "", err
	}
	legacyStyle = strings.TrimSpace(legacyStyle)
	if canonicalVersions == 0 && legacyStyle != "" && legacyStyle != strings.TrimSpace(DefaultPremiseStyle(language)) {
		return legacyStyle, nil
	}
	return style, nil
}

// SectionImageRules resolves the same effective templates for every caller.
func (service *Service) SectionImageRules(ctx context.Context, pageRole string) (GenerationSnapshot, error) {
	return service.sectionImageRules(ctx, pageRole, false)
}

func (service *Service) sectionImageRules(ctx context.Context, pageRole string, host bool) (GenerationSnapshot, error) {
	var snapshot GenerationSnapshot
	st := story.NewService(service.store)
	p, err := st.GetProject(ctx)
	if err != nil {
		return snapshot, err
	}
	premise, err := service.GetPremise(ctx)
	if err != nil {
		return snapshot, err
	}
	snapshot.Version, snapshot.GenerationLanguage = 5, p.GenerationLanguage
	snapshot.StyleSnapshot, err = EffectiveImageStyle(ctx, service.store, st, premise.DefaultStyle, p.GenerationLanguage)
	if err != nil {
		return snapshot, err
	}
	snapshot.PromptTemplate, err = st.EffectivePrompt(ctx, promptcatalog.GroupChapter, "section_image")
	if err != nil {
		return snapshot, err
	}
	key := "before_image"
	switch pageRole {
	case PageRoleFrontCover:
		key = "cover_before_image"
	case PageRoleBackCover:
		key = "back_cover_before_image"
	}
	before, err := st.EffectivePrompt(ctx, promptcatalog.GroupChapter, key)
	if err != nil {
		return snapshot, err
	}
	if host && key == "before_image" {
		before = promptcatalog.HostPageBaseRules(before)
	}
	snapshot.PromptTemplate = strings.ReplaceAll(snapshot.PromptTemplate, "{{before_image_prompt}}", before)
	snapshot.ReferencePresentPrompt, err = st.EffectivePrompt(ctx, promptcatalog.GroupChapter, "section_reference_present")
	if err != nil {
		return snapshot, err
	}
	snapshot.ReferenceAbsentPrompt, err = st.EffectivePrompt(ctx, promptcatalog.GroupChapter, "section_reference_absent")
	if err != nil {
		return snapshot, err
	}
	snapshot.AdditionalDirectionPrompt, err = st.EffectivePrompt(ctx, promptcatalog.GroupChapter, "section_additional_direction")
	if err != nil {
		return snapshot, err
	}
	snapshot.LanguageInstruction, err = st.EffectiveLanguageInstruction(ctx)
	return snapshot, err
}

// HostSectionImageRules resolves the host-page rules used by prepare_page_image.
// A server-side task created on behalf of a host must use these exact rules so
// both paths render the same prompt bytes for the same page facts.
func (service *Service) HostSectionImageRules(ctx context.Context, pageRole string) (GenerationSnapshot, error) {
	return service.sectionImageRules(ctx, pageRole, true)
}
