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
		meta("base", "Agent 基础 Prompt", "项目对话的稳定安全与工具规则。", "Agent base prompt", "Stable project-chat safety and tool rules."),
		meta("conversation_summary", "对话摘要包装", "压缩 Agent 上下文时包裹派生摘要。", "Conversation summary wrapper", "Wrap a derived summary when compressing Agent context."),
	}
}
