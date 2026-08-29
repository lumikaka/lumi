package promptcatalog

import (
	agentprompts "lumi/internal/agent/prompts"
	"strings"
)

const (
	currentAgentGuideRuleZH     = "- 先识别用户目标对应的能力。用户要求执行能力索引中的创作功能时，必须先用 read_agent_doc 读取对应 Guide；再按 Guide 的顺序，在首次调用相关 API 前读取对应 API Contract；之后才能使用 request_api。即使已熟悉流程或接口也不得跳过，且不要重复读取已读文档。"
	previousAgentGuideRuleZH    = "- 先识别用户目标对应的能力。流程或来源约束不确定时，用 read_agent_doc 读取推荐 Guide；method、path、字段或响应不确定时，读取对应 API Contract。避免重复读取文档或进行无必要调用。"
	currentAgentGuideRuleEN     = "- First identify the capability that matches the user's goal. When the user asks to perform a creative function in the capability index, you must first use read_agent_doc to read its Guide, then read each relevant API Contract before that API is first called, and only then use request_api. Do not skip this order even when familiar with the workflow or API, and do not reread documents already read."
	previousAgentGuideRuleEN    = "- First identify the capability that matches the user's goal. When a workflow or source constraint is uncertain, use read_agent_doc to read a recommended Guide; when a method, path, field, or response is uncertain, read the relevant API Contract. Avoid repeated documentation reads and unnecessary calls."
	currentAgentInputRuleZH     = "- 只有需要用户做关键选择、信息确实不足或危险操作需要确认时，才单独调用 request_user_input；它不得与其他 Tool Call 同批出现。优先只问 1 个问题，只有问题直接相关时才在一次调用中组合 2–3 个；每题提供 2–3 个互斥选项，第一项是推荐项且 label 必须以精确的 ` (Recommended)` 结尾，其他项不得使用该后缀。不要创建 Other 选项，客户端会自动提供自由输入。危险 API 的 confirmation 必须原样使用 request_api 确认错误返回的 route、project_uuid、target_uuid、expected_revision 和 request_fingerprint，只能绑定唯一 question_id；第一项必须是安全推荐项，confirm_option 绑定非首项的明确危险操作。"
	previousAgentInputRuleZH    = "- 需要用户做关键选择、信息确实不足或危险操作需要确认时，单独调用 request_user_input；它不得与其他 Tool Call 同批出现。危险 API 的 confirmation 必须原样使用 request_api 确认错误返回的 route、project_uuid、target_uuid、expected_revision 和 request_fingerprint，并绑定确认选项索引。"
	currentAgentInputRuleEN     = "- Call request_user_input by itself only when a material choice or required fact is genuinely missing, or a risky action needs confirmation; never batch it with another tool call. Prefer one question and group two or three only when directly related. Give each question two or three mutually exclusive options, put the recommended option first, and end only its label with the exact suffix ` (Recommended)`. Do not create an Other option; the client supplies free-form Other automatically. For a risky API, copy route, project_uuid, target_uuid, expected_revision, and request_fingerprint exactly from the request_api confirmation error and bind the sole question_id; the first option must be the safe recommendation and confirm_option must identify a later explicit risky action."
	previousAgentInputRuleEN    = "- When a material choice is missing or a risky action needs confirmation, call request_user_input by itself; never batch it with another tool call. Copy route, project_uuid, target_uuid, expected_revision, and request_fingerprint exactly from the request_api confirmation error, then bind the confirming-option index."
	currentAgentUIRefRuleZH     = "- 成功的 request_api Tool Result 包含 ui_ref 时，最终答复第一次自然提及该次变更的资源时必须使用 `[自然语言名称](ui_ref.href)`，并逐字复制 href。每个资源最多链接一次；不得自行构造、猜测或修改 `@project` 引用，也不得另列“打开……”链接。没有 ui_ref 或操作未成功时不要创建项目引用。"
	currentAgentUIRefRuleEN     = "- When a successful request_api Tool Result contains ui_ref, use `[natural-language name](ui_ref.href)` the first time the final answer naturally mentions that changed resource, copying the href verbatim. Link each resource at most once; never construct, guess, or alter an `@project` reference, and never add a separate “Open …” link. Do not create a project reference when ui_ref is absent or the operation did not succeed."
	currentAgentBootstrapRuleZH = "- 首页创建会话的 bootstrap 首个 Turn 定稿后不得手工生产，只能按初始化 Guide 启动受控 YOLO；Workflow 创建成功后使用返回的 ui_ref 并立即结束当前 Turn。"
	currentAgentBootstrapRuleEN = "- After setup finalization, the first bootstrap Turn created from the home-page flow must not produce resources manually; it may only start the controlled YOLO flow described by the initialization Guide. Once the Workflow is created, use its returned ui_ref and end the current Turn immediately."
)

func previousAgentBaseDefaults(value, language string) []string {
	currentGuide, previousGuide := currentAgentGuideRuleZH, previousAgentGuideRuleZH
	currentInput, previousInput := currentAgentInputRuleZH, previousAgentInputRuleZH
	currentUIRef := currentAgentUIRefRuleZH
	currentBootstrap := currentAgentBootstrapRuleZH
	if language == LanguageEnglish {
		currentGuide, previousGuide = currentAgentGuideRuleEN, previousAgentGuideRuleEN
		currentInput, previousInput = currentAgentInputRuleEN, previousAgentInputRuleEN
		currentUIRef = currentAgentUIRefRuleEN
		currentBootstrap = currentAgentBootstrapRuleEN
	}
	previousBootstrap := strings.Replace(value, currentBootstrap+"\n", "", 1)
	previousUIRef := strings.Replace(previousBootstrap, currentUIRef+"\n", "", 1)
	previousInputRule := strings.Replace(previousUIRef, currentInput, previousInput, 1)
	return []string{previousBootstrap, previousUIRef, previousInputRule, strings.Replace(previousInputRule, currentGuide, previousGuide, 1)}
}

func agentDefinitions(language string) []Definition {
	english := language == LanguageEnglish
	meta := func(key, zhTitle, zhDescription, enTitle, enDescription string) Definition {
		value := agentprompts.MustRead(key, language)
		previousDefaults := []string(nil)
		if key == "base" {
			previousDefaults = previousAgentBaseDefaults(value, language)
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
