package promptcatalog

func runtimeDefinitions(language string) []Definition {
	english := language == LanguageEnglish
	title := "项目生成语言约束"
	description := "组合进 Story、Premise、Chapter、Agent 与 Yolo 请求的项目级语言要求。"
	if english {
		title = "Project generation language instruction"
		description = "Project-level language requirement composed into Story, Premise, Chapter, Agent, and Yolo requests."
	}
	return []Definition{{
		Group: GroupRuntime, Key: "project_language_instruction", Title: title,
		Description: description, PromptType: PromptTypeFragment, DefaultValue: LanguageInstruction(language),
	}}
}
