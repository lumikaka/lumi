package promptcatalog

import (
	agentprompts "lumi/internal/agent/prompts"
	"strings"
)

const (
	currentAgentGuideRuleZH  = "- 先识别用户目标对应的能力。用户要求执行能力索引中的创作功能时，必须先用 read_agent_doc 读取对应 Guide；再按 Guide 的顺序，在首次调用相关 API 前读取对应 API Contract；之后才能使用 request_api。即使已熟悉流程或接口也不得跳过，且不要重复读取已读文档。"
	previousAgentGuideRuleZH = "- 先识别用户目标对应的能力。流程或来源约束不确定时，用 read_agent_doc 读取推荐 Guide；method、path、字段或响应不确定时，读取对应 API Contract。避免重复读取文档或进行无必要调用。"
	currentAgentGuideRuleEN  = "- First identify the capability that matches the user's goal. When the user asks to perform a creative function in the capability index, you must first use read_agent_doc to read its Guide, then read each relevant API Contract before that API is first called, and only then use request_api. Do not skip this order even when familiar with the workflow or API, and do not reread documents already read."
	previousAgentGuideRuleEN = "- First identify the capability that matches the user's goal. When a workflow or source constraint is uncertain, use read_agent_doc to read a recommended Guide; when a method, path, field, or response is uncertain, read the relevant API Contract. Avoid repeated documentation reads and unnecessary calls."
)

func previousAgentBaseDefault(value, language string) string {
	current, previous := currentAgentGuideRuleZH, previousAgentGuideRuleZH
	if language == LanguageEnglish {
		current, previous = currentAgentGuideRuleEN, previousAgentGuideRuleEN
	}
	return strings.Replace(value, current, previous, 1)
}

func agentDefinitions(language string) []Definition {
	english := language == LanguageEnglish
	meta := func(key, zhTitle, zhDescription, enTitle, enDescription string) Definition {
		value := agentprompts.MustRead(key, language)
		previousDefaults := []string(nil)
		if key == "base" {
			previousDefaults = []string{previousAgentBaseDefault(value, language)}
		}
		if english {
			return Definition{Group: GroupAgent, Key: key, Title: enTitle, Description: enDescription, PromptType: PromptTypeTemplate, DefaultValue: value, PreviousDefaultValues: previousDefaults}
		}
		return Definition{Group: GroupAgent, Key: key, Title: zhTitle, Description: zhDescription, PromptType: PromptTypeTemplate, DefaultValue: value, PreviousDefaultValues: previousDefaults}
	}
	return []Definition{
		meta("base", "Agent 基础 Prompt", "项目对话的稳定安全与工具规则。", "Agent base prompt", "Stable project-chat safety and tool rules."),
		meta("conversation_summary", "对话摘要包装", "压缩 Agent 上下文时包裹派生摘要。", "Conversation summary wrapper", "Wrap a derived summary when compressing Agent context."),
	}
}
