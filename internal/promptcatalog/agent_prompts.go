package promptcatalog

import agentprompts "lumi/internal/agent/prompts"

func agentDefinitions(language string) []Definition {
	english := language == LanguageEnglish
	meta := func(key, zhTitle, zhDescription, enTitle, enDescription string) Definition {
		value := agentprompts.MustRead(key, language)
		if english {
			return Definition{Group: GroupAgent, Key: key, Title: enTitle, Description: enDescription, PromptType: PromptTypeTemplate, DefaultValue: value}
		}
		return Definition{Group: GroupAgent, Key: key, Title: zhTitle, Description: zhDescription, PromptType: PromptTypeTemplate, DefaultValue: value}
	}
	return []Definition{
		meta("base", "Agent 基础 Prompt", "跨 Scene 的稳定安全与工具规则。", "Agent base prompt", "Stable cross-Scene safety and tool rules."),
		meta("scene_project_assistant", "Project Assistant Scene", "通用项目助手 Scene。", "Project Assistant Scene", "Scene for the general project assistant."),
		meta("scene_premise_asset", "Premise Asset Generation Scene", "新建单个设定项的 Scene。", "Premise Asset Generation Scene", "Scene for creating one premise asset."),
		meta("scene_asset_reference", "Asset Reference Scene", "绑定设定项的 Scene。", "Asset Reference Scene", "Scene for a bound premise asset."),
		meta("scene_storyboard_reference", "Storyboard Reference Scene", "绑定 Section 的 Scene。", "Storyboard Reference Scene", "Scene for a bound storyboard Section."),
		meta("conversation_summary", "对话摘要包装", "压缩 Agent 上下文时包裹派生摘要。", "Conversation summary wrapper", "Wrap a derived summary when compressing Agent context."),
	}
}
