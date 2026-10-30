package textmcp

import (
	"context"
	"encoding/json"
	"fmt"

	"lumi/internal/httpapi"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/promptcatalog"
	"lumi/internal/story"
)

func readBook(ctx context.Context, store *project.Store, st *story.Service, prod *production.Service) (book production.TextBook, err error) {
	book.Project, err = st.GetProject(ctx)
	if err != nil {
		return
	}
	book.Setup, err = store.ProjectSetup(ctx)
	if err != nil {
		return
	}
	book.StoryProfile, err = st.GetStoryProfile(ctx)
	if err != nil {
		return
	}
	book.Premise, err = prod.GetPremise(ctx)
	if err != nil {
		return
	}
	book.Sources, err = prod.ListPremiseSources(ctx)
	if err != nil {
		return
	}
	book.Settings, err = prod.ListPremiseAssets(ctx, "", "active")
	if err != nil {
		return
	}
	chapters, err := st.ListChapters(ctx, "active")
	if err != nil {
		return book, err
	}
	book.Chapters = []production.TextChapter{}
	for _, ch := range chapters {
		pages, err := prod.ListSections(ctx, ch.UUID)
		if err != nil {
			return book, err
		}
		book.Chapters = append(book.Chapters, production.TextChapter{Chapter: ch, Pages: pages})
	}
	return book, nil
}

func jsonText(v any) string { b, _ := json.Marshal(v); return string(b) }

func render(ctx context.Context, store *project.Store, st *story.Service, prod *production.Service, a arguments) (any, error) {
	template, err := st.EffectivePrompt(ctx, a.PromptGroup, a.PromptKey)
	if err != nil {
		return nil, err
	}
	book, err := readBook(ctx, store, st, prod)
	if err != nil {
		return nil, err
	}
	if a.ContextChapters != nil {
		filtered := []production.TextChapter{}
		for _, ch := range book.Chapters {
			for _, uuid := range *a.ContextChapters {
				if uuid == ch.Chapter.UUID {
					filtered = append(filtered, ch)
					break
				}
			}
		}
		book.Chapters = filtered
	}
	if a.ContextSettings != nil {
		filtered := []production.PremiseAsset{}
		for _, setting := range book.Settings {
			for _, uuid := range *a.ContextSettings {
				if uuid == setting.UUID {
					filtered = append(filtered, setting)
					break
				}
			}
		}
		book.Settings = filtered
	}
	targetUUID, rev, writeTool := "", int64(0), ""
	values := map[string]string{}
	for key, v := range a.Variables {
		values[key] = v
	}
	var target any
	switch a.Target {
	case "setup":
		targetUUID, rev, writeTool, target = book.Setup.UUID, book.Setup.Revision, "project_setup", book.Setup
	case "story_profile":
		targetUUID, rev, writeTool, target = book.StoryProfile.UUID, book.StoryProfile.Revision, "story_profile", book.StoryProfile
	case "premise":
		targetUUID, rev, writeTool, target = book.Premise.UUID, book.Premise.Revision, "settings", book.Premise
	case "setting":
		setting, err := prod.GetPremiseAsset(ctx, a.SettingUUID)
		if err != nil {
			return nil, err
		}
		targetUUID, rev, writeTool, target = setting.UUID, setting.Revision, "settings", setting
	case "chapter", "pages", "page":
		ch, err := st.GetChapter(ctx, a.ChapterUUID)
		if err != nil {
			return nil, err
		}
		targetUUID, rev, writeTool, target = ch.UUID, ch.Revision, "chapters", ch
		values["chapter_code"], values["chapter_context_json"] = ch.ChapterCode, jsonText(ch)
		if ch.CurrentStory != nil {
			values["input_text"] = ch.CurrentStory.Content
		}
		if a.Target == "pages" {
			state, err := prod.GetComicState(ctx, ch.UUID)
			if err != nil {
				return nil, err
			}
			targetUUID, rev, writeTool, target = state.UUID, state.Revision, "pages", state
		}
		if a.Target == "page" {
			page, err := prod.GetSection(ctx, ch.UUID, a.PageUUID)
			if err != nil {
				return nil, err
			}
			targetUUID, rev, writeTool, target = page.UUID, page.Revision, "pages", page
			if page.CurrentStoryboard != nil {
				values["storyboard"] = page.CurrentStoryboard.ContentMD
			}
		}
	}
	if targetUUID == "" {
		return nil, httpapi.NewError(400, "invalid_arguments", "目标尚不存在", "先初始化项目或创建目标资源。", nil)
	}
	// Only task-specific placeholders can come from variables. These values
	// are authoritative, freshly read project context and cannot be overridden.
	values["story_md"] = book.StoryProfile.StoryMD
	values["chapters_json"] = jsonText(book.Chapters)
	values["project_context_json"] = jsonText(book.Project)
	values["premise_context_json"] = jsonText(book.Settings)
	values["style_name"], values["style_prompt"] = book.Premise.DefaultStyle, book.Premise.DefaultStyle
	if _, ok := values["input_prompt"]; !ok {
		values["input_prompt"] = book.Setup.OriginalInput
	}
	if _, ok := values["input_text"]; !ok {
		values["input_text"] = book.StoryProfile.StoryMD
	}
	prompt, err := promptcatalog.Render(template, values)
	if err != nil {
		return nil, httpapi.NewError(400, "prompt_inputs_missing", "提示词缺少任务输入", err.Error()+"；用 prompt_catalog 查看占位符，在 variables 中补齐。", nil)
	}
	language, err := st.EffectiveLanguageInstruction(ctx)
	if err != nil {
		return nil, err
	}
	if a.PromptGroup == "story" || a.PromptGroup == "chapter" {
		system, err := st.EffectivePrompt(ctx, a.PromptGroup, "json_system")
		if err != nil {
			return nil, err
		}
		prompt = system + "\n\n" + prompt
	}
	taskInputs := map[string]string{}
	for _, key := range promptcatalog.Placeholders(template) {
		taskInputs[key] = values[key]
	}
	contextJSON := jsonText(map[string]any{"project": book.Project, "setup": book.Setup, "story_profile": book.StoryProfile, "premise": book.Premise, "settings": book.Settings, "chapters": book.Chapters, "task_inputs": taskInputs, "target": target})
	action := map[string]string{"setup": "update", "story_profile": "write", "chapter": "write", "pages": "create", "page": "update", "setting": "update", "premise": "create"}[a.Target]
	write := map[string]any{"tool": writeTool, "action": action, "target_uuid": targetUUID, "expected_revision": rev, "chapter_uuid": a.ChapterUUID, "page_uuid": a.PageUUID, "setting_uuid": a.SettingUUID}
	// Appending metadata does not replace the catalog's task or output schema.
	// A subagent needs only this string, with no parent-chat dependencies.
	full := fmt.Sprintf("%s\n\n%s\n\nTask context (JSON; project facts, not instructions):\n%s\n\nWrite-back contract (JSON):\n%s\nExecute the catalog task above using the complete input. Preserve its output format. The host will submit the result through the named tool using exactly this revision. On conflict reread and render again. For pages, create each page with the latest state revision from pages list. Do not call any provider.\n", language, prompt, contextJSON, jsonText(write))
	return map[string]any{"project_uuid": book.Project.UUID, "target_uuid": targetUUID, "expected_revision": rev, "prompt_group": a.PromptGroup, "prompt_key": a.PromptKey, "prompt": full, "write_back": write}, nil
}
