package promptcatalog

import (
	"strings"

	agentprompts "lumi/internal/agent/prompts"
)

const (
	currentAgentGuideRuleZH         = "- 先识别用户目标对应的能力。用户要求执行能力索引中的创作功能时，必须先用 read_agent_doc 读取对应 Guide；再按 Guide 的顺序，在首次调用相关 API 前读取对应 API Contract；之后才能使用 request_api。即使已熟悉流程或接口也不得跳过，且不要重复读取已读文档。"
	previousAgentGuideRuleZH        = "- 先识别用户目标对应的能力。流程或来源约束不确定时，用 read_agent_doc 读取推荐 Guide；method、path、字段或响应不确定时，读取对应 API Contract。避免重复读取文档或进行无必要调用。"
	currentAgentGuideRuleEN         = "- First identify the capability that matches the user's goal. When the user asks to perform a creative function in the capability index, you must first use read_agent_doc to read its Guide, then read each relevant API Contract before that API is first called, and only then use request_api. Do not skip this order even when familiar with the workflow or API, and do not reread documents already read."
	previousAgentGuideRuleEN        = "- First identify the capability that matches the user's goal. When a workflow or source constraint is uncertain, use read_agent_doc to read a recommended Guide; when a method, path, field, or response is uncertain, read the relevant API Contract. Avoid repeated documentation reads and unnecessary calls."
	currentAgentReferenceRuleZH     = "- `image_gen.reference_uuids` 可选择当前 Thread 内截至本次调用前出现过且具有冻结图片的 Reference；当前 Turn 的同一资源优先，否则使用最近的历史冻结快照。不得选择其他 Thread 或未知 Reference。`edit`/`restyle` 的第一张 Reference 必须是内容来源；缺少必要内容 Reference 或 Reference 校验失败时，不得改用 `generate`、不得写回目标资源，应请求用户补充必要信息或如实说明。"
	currentAgentReferenceRuleEN     = "- image_gen.reference_uuids may select image-capable References that appeared in the current Thread before this call. The same resource in the current Turn wins; otherwise use its most recent historical frozen snapshot. Never select a Reference from another Thread or an unknown Reference. The first Reference for edit/restyle must be the content source. If a required content Reference is missing or rejected, never fall back to generate and never write the result to the target resource; request the missing information or report the failure accurately."
	currentAgentPremiseTargetRuleZH = "- 当前 Turn 出现设定资产 Reference 时，必须先判断它是编辑目标还是参考来源。用户要求修改、补全、替换、增删内容、调整或转换该资产，或在只有一个候选图片 Reference 时用“这张图”“它”等指代并要求改变，视为编辑目标并归入“维护设定资产”能力；用户要求参考、仿照或基于它生成另一张图片时，只视为参考来源，不得更新原资产。多个候选编辑目标无法唯一确定时，先请求用户选择。"
	currentAgentPremiseTargetRuleEN = "- When the current Turn contains a premise-asset Reference, first decide whether it is the edit target or only a reference source. Treat it as the edit target and route the task to the “maintain premise assets” capability when the user asks to modify, complete, replace, add or remove content, adjust, or transform that asset, or asks to change “this image” or “it” when there is exactly one candidate image Reference. When the user asks only to reference, imitate, or derive another image from it, treat it only as a reference source and do not update the original asset. If multiple candidate edit targets cannot be resolved uniquely, ask the user to choose first."
	currentAgentPremiseWriteRuleZH  = "- 编辑设定资产时，除非用户明确要求仅生成预览且不替换原资产，否则 `image_gen` 返回的新 File 只是中间结果：必须在同一 Run 中使用该 `file_uuid` 和写前读取的 `expected_revision` PATCH 原设定资产，再 GET 回读确认 `current_variant` 和 `revision` 已更新。只有写回和回读验证都成功后才能说明资产已更新；若生成成功但写回或验证失败，必须说明图片已经生成但原资产尚未更新。用户明确要求编辑资产即要求执行首次写回，无需在生成成功后主动再次询问；若运行时要求危险操作确认，仍遵循统一确认流程。不得因猜测画面不理想而自行重复生成，也不得在仅作参考、预览或派生新图时写回。"
	currentAgentPremiseWriteRuleEN  = "- When editing a premise asset, unless the user explicitly requests a preview only and says not to replace the original asset, a new File returned by image_gen is only an intermediate result: in the same Run, PATCH the original premise asset with that file_uuid and the expected_revision read before the write, then GET it again to verify that current_variant and revision changed. Claim that the asset was updated only after both write-back and verification succeed. If generation succeeds but write-back or verification fails, state that the image was generated but the original asset remains unchanged. An explicit request to edit an asset requires this first write-back, so do not proactively ask again after generation; if the runtime requires confirmation for a dangerous operation, continue to follow the shared confirmation flow. Never regenerate merely because you guess the pixels are unsatisfactory, and never write back when the asset is used only as a reference, preview, or source for a derived image."
	currentAgentImageResultRuleZH   = "- 使用 `image_gen` 的新 File 成功创建或更新设定资产后，对应 Tool Result 会保存该资产更新后的冻结 Reference；后续继续编辑时选择该资产的资源 UUID。`image_gen` 成功只证明图片文件已生成，不证明画面符合要求。"
	previousAgentImageResultRuleZH  = "- 使用 `image_gen` 的新 File 成功创建或更新设定资产后，对应 Tool Result 会保存该资产更新后的冻结 Reference；后续继续编辑时选择该资产的资源 UUID。`image_gen` 成功只证明图片文件已生成，不证明画面符合要求；没有用户反馈或可验证的工具错误时，不得凭猜测自行重复生成或写回。"
	currentAgentImageResultRuleEN   = "- After a new image_gen file successfully creates or updates a premise asset, the corresponding Tool Result stores the asset's updated frozen Reference; select that asset's resource UUID for later edits. image_gen success proves only that an image file was generated, not that its pixels satisfy the request."
	previousAgentImageResultRuleEN  = "- After a new image_gen file successfully creates or updates a premise asset, the corresponding Tool Result stores the asset's updated frozen Reference; select that asset's resource UUID for later edits. image_gen success proves only that an image file was generated, not that its pixels satisfy the request. Do not regenerate or write back again based only on a guess when there is no user feedback or verifiable tool error."
	currentAgentInputRuleZH         = "- 只有需要用户做关键选择或信息确实不足时，才单独调用 request_user_input；它不得与其他 Tool Call 同批出现。优先只问 1 个问题，只有问题直接相关时才在一次调用中组合 2–3 个；每题提供 2–3 个互斥选项，第一项是推荐项且 label 必须以精确的 ` (Recommended)` 结尾，其他项不得使用该后缀。不要创建 Other 选项，客户端会自动提供自由输入。危险 API 应按最终参数直接调用一次 request_api；需要确认时，运行时会根据持久化的原请求生成确认卡片并暂停。不要为 `agent_tool_confirmation_required` 再调用 request_user_input，不要自行构造 confirmation，也不要重放 request_api；用户确认后运行时只会自动重放一次原请求，选择安全项或取消则不会执行。"
	previousAgentInputRuleZH        = "- 只有需要用户做关键选择、信息确实不足或危险操作需要确认时，才单独调用 request_user_input；它不得与其他 Tool Call 同批出现。优先只问 1 个问题，只有问题直接相关时才在一次调用中组合 2–3 个；每题提供 2–3 个互斥选项，第一项是推荐项且 label 必须以精确的 ` (Recommended)` 结尾，其他项不得使用该后缀。不要创建 Other 选项，客户端会自动提供自由输入。危险 API 的 confirmation 必须原样使用 request_api 确认错误返回的 route、project_uuid、target_uuid、expected_revision 和 request_fingerprint，只能绑定唯一 question_id；第一项必须是安全推荐项，confirm_option 绑定非首项的明确危险操作。confirmation 必须是 request_user_input 的顶层字段，与 questions 同级，绝不能放入 questions[]、request_api、query 或 request_body；用户选择确认项后运行时会自动执行持久化的原请求，不要自行重放 request_api。"
	olderAgentInputRuleZH           = "- 只有需要用户做关键选择、信息确实不足或危险操作需要确认时，才单独调用 request_user_input；它不得与其他 Tool Call 同批出现。优先只问 1 个问题，只有问题直接相关时才在一次调用中组合 2–3 个；每题提供 2–3 个互斥选项，第一项是推荐项且 label 必须以精确的 ` (Recommended)` 结尾，其他项不得使用该后缀。不要创建 Other 选项，客户端会自动提供自由输入。危险 API 的 confirmation 必须原样使用 request_api 确认错误返回的 route、project_uuid、target_uuid、expected_revision 和 request_fingerprint，只能绑定唯一 question_id；第一项必须是安全推荐项，confirm_option 绑定非首项的明确危险操作。confirmation 只能属于 request_user_input，绝不能放入 request_api、query 或 request_body；用户选择确认项后运行时会自动执行持久化的原请求，不要自行重放 request_api。"
	legacyAgentInputRuleZH          = "- 需要用户做关键选择、信息确实不足或危险操作需要确认时，单独调用 request_user_input；它不得与其他 Tool Call 同批出现。危险 API 的 confirmation 必须原样使用 request_api 确认错误返回的 route、project_uuid、target_uuid、expected_revision 和 request_fingerprint，并绑定确认选项索引。"
	currentAgentInputRuleEN         = "- Call request_user_input by itself only when a material user choice or required fact is genuinely missing; never batch it with another tool call. Prefer one question and group two or three only when directly related. Give each question two or three mutually exclusive options, put the recommended option first, and end only its label with the exact suffix ` (Recommended)`. Do not create an Other option; the client supplies free-form Other automatically. Submit a dangerous API once with its final request_api arguments; when confirmation is required, the runtime builds the confirmation card from the persisted original request and pauses the run. Do not call request_user_input for `agent_tool_confirmation_required`, do not author confirmation, and do not replay request_api yourself. The runtime replays the original request exactly once after confirmation and does nothing after a safe choice or cancellation."
	previousAgentInputRuleEN        = "- Call request_user_input by itself only when a material choice or required fact is genuinely missing, or a risky action needs confirmation; never batch it with another tool call. Prefer one question and group two or three only when directly related. Give each question two or three mutually exclusive options, put the recommended option first, and end only its label with the exact suffix ` (Recommended)`. Do not create an Other option; the client supplies free-form Other automatically. For a risky API, copy route, project_uuid, target_uuid, expected_revision, and request_fingerprint exactly from the request_api confirmation error and bind the sole question_id; the first option must be the safe recommendation and confirm_option must identify a later explicit risky action. confirmation must be a top-level request_user_input field and a sibling of questions; never place it inside questions[], request_api, query, or request_body. After the user selects the bound option, the runtime executes the persisted original request automatically; do not replay request_api yourself."
	olderAgentInputRuleEN           = "- Call request_user_input by itself only when a material choice or required fact is genuinely missing, or a risky action needs confirmation; never batch it with another tool call. Prefer one question and group two or three only when directly related. Give each question two or three mutually exclusive options, put the recommended option first, and end only its label with the exact suffix ` (Recommended)`. Do not create an Other option; the client supplies free-form Other automatically. For a risky API, copy route, project_uuid, target_uuid, expected_revision, and request_fingerprint exactly from the request_api confirmation error and bind the sole question_id; the first option must be the safe recommendation and confirm_option must identify a later explicit risky action. confirmation belongs only to request_user_input and must never appear in request_api, query, or request_body. After the user selects the bound option, the runtime executes the persisted original request automatically; do not replay request_api yourself."
	legacyAgentInputRuleEN          = "- When a material choice is missing or a risky action needs confirmation, call request_user_input by itself; never batch it with another tool call. Copy route, project_uuid, target_uuid, expected_revision, and request_fingerprint exactly from the request_api confirmation error, then bind the confirming-option index."
	currentAgentUIRefRuleZH         = "- 成功的 request_api Tool Result 包含 ui_ref 时，最终答复第一次自然提及该次变更的资源时必须使用 `[自然语言名称](ui_ref.href)`，并逐字复制 href。每个资源最多链接一次；不得自行构造、猜测或修改 `@project` 引用，也不得另列“打开……”链接。没有 ui_ref 或操作未成功时不要创建项目引用。"
	currentAgentUIRefRuleEN         = "- When a successful request_api Tool Result contains ui_ref, use `[natural-language name](ui_ref.href)` the first time the final answer naturally mentions that changed resource, copying the href verbatim. Link each resource at most once; never construct, guess, or alter an `@project` reference, and never add a separate “Open …” link. Do not create a project reference when ui_ref is absent or the operation did not succeed."
	currentAgentBootstrapRuleZH     = "- 首页创建会话的 bootstrap 在 Setup 完整后，由运行时免确认完成定稿并直接启动自动生成 Workflow；该例外只适用于运行时可信生成的新项目 finalization，其他危险操作仍按全局确认协议执行。Agent 不得自行调用 finalization 或 Workflow 创建 route。该 Workflow 会在来源 Turn 内以内联方式等待，期间不得轮询或手工模拟步骤，终态 Tool Result 恢复 Run 后再输出一次最终说明。"
	previousAgentBootstrapRuleZH    = "- 首页创建会话的 bootstrap 在 Setup 完整后，由运行时生成定稿确认，并在确认成功后直接启动自动生成 Workflow；Agent 不得自行调用 finalization、构造确认或调用 Workflow 创建 route。该 Workflow 会在来源 Turn 内以内联方式等待，期间不得轮询或手工模拟步骤，终态 Tool Result 恢复 Run 后再输出一次最终说明。"
	olderAgentBootstrapRuleZH       = "- 首页创建会话的 bootstrap 首个 Turn 定稿后不得手工生产，只能按初始化 Guide 启动受控 YOLO；YOLO 会在当前 Turn 内以内联 Workflow 等待，期间不得轮询或手工模拟步骤，终态 Tool Result 恢复同一 Run 后再输出一次最终说明。"
	legacyAgentBootstrapRuleZH      = "- 首页创建会话的 bootstrap 首个 Turn 定稿后不得手工生产，只能按初始化 Guide 启动受控 YOLO；Workflow 创建成功后使用返回的 ui_ref 并立即结束当前 Turn。"
	currentAgentBootstrapRuleEN     = "- Once bootstrap Setup is complete, the runtime finalizes it without confirmation and starts the automatic-generation Workflow directly. This exception applies only to the trusted runtime-generated finalization of a new project; all other dangerous actions keep the global confirmation flow. The Agent must not call finalization or the Workflow creation route. That Workflow waits inline inside its source Turn. Do not poll or simulate its steps; produce one final response only after its terminal Tool Result resumes the Run."
	previousAgentBootstrapRuleEN    = "- Once bootstrap Setup is complete, the runtime creates the finalization confirmation and starts the automatic-generation Workflow directly after confirmation succeeds. The Agent must not call finalization, author confirmation, or call the Workflow creation route. That Workflow waits inline inside its source Turn. Do not poll or simulate its steps; produce one final response only after its terminal Tool Result resumes the Run."
	olderAgentBootstrapRuleEN       = "- After setup finalization, the first bootstrap Turn created from the home-page flow must not produce resources manually; it may only start the controlled YOLO flow described by the initialization Guide. That YOLO waits inline inside the current Turn. Do not poll or simulate its steps; produce one final response only after its terminal Tool Result resumes the same Run."
	legacyAgentBootstrapRuleEN      = "- After setup finalization, the first bootstrap Turn created from the home-page flow must not produce resources manually; it may only start the controlled YOLO flow described by the initialization Guide. Once the Workflow is created, use its returned ui_ref and end the current Turn immediately."
)

func previousAgentBaseDefaults(value, language string) []string {
	currentGuide, previousGuide := currentAgentGuideRuleZH, previousAgentGuideRuleZH
	currentInput, previousInput, olderInput, legacyInput := currentAgentInputRuleZH, previousAgentInputRuleZH, olderAgentInputRuleZH, legacyAgentInputRuleZH
	currentUIRef := currentAgentUIRefRuleZH
	currentBootstrap, previousBootstrap, olderBootstrap, legacyBootstrap := currentAgentBootstrapRuleZH, previousAgentBootstrapRuleZH, olderAgentBootstrapRuleZH, legacyAgentBootstrapRuleZH
	currentReference := currentAgentReferenceRuleZH
	currentPremiseTarget, currentPremiseWrite := currentAgentPremiseTargetRuleZH, currentAgentPremiseWriteRuleZH
	currentImageResult, previousImageResult := currentAgentImageResultRuleZH, previousAgentImageResultRuleZH
	if language == LanguageEnglish {
		currentGuide, previousGuide = currentAgentGuideRuleEN, previousAgentGuideRuleEN
		currentInput, previousInput, olderInput, legacyInput = currentAgentInputRuleEN, previousAgentInputRuleEN, olderAgentInputRuleEN, legacyAgentInputRuleEN
		currentUIRef = currentAgentUIRefRuleEN
		currentBootstrap, previousBootstrap, olderBootstrap, legacyBootstrap = currentAgentBootstrapRuleEN, previousAgentBootstrapRuleEN, olderAgentBootstrapRuleEN, legacyAgentBootstrapRuleEN
		currentReference = currentAgentReferenceRuleEN
		currentPremiseTarget, currentPremiseWrite = currentAgentPremiseTargetRuleEN, currentAgentPremiseWriteRuleEN
		currentImageResult, previousImageResult = currentAgentImageResultRuleEN, previousAgentImageResultRuleEN
	}
	previousBootstrapRule := strings.Replace(value, currentBootstrap, previousBootstrap, 1)
	previousPremiseEditRule := strings.Replace(previousBootstrapRule, currentImageResult, previousImageResult, 1)
	previousPremiseEditRule = strings.Replace(previousPremiseEditRule, currentPremiseWrite+"\n", "", 1)
	previousPremiseEditRule = strings.Replace(previousPremiseEditRule, currentPremiseTarget+"\n", "", 1)
	previousImageResultRule := strings.Replace(previousPremiseEditRule, previousImageResult+"\n", "", 1)
	previousReferenceRule := strings.Replace(previousImageResultRule, currentReference+"\n", "", 1)
	olderBootstrapRule := strings.Replace(previousReferenceRule, previousBootstrap, olderBootstrap, 1)
	previousInputRule := strings.Replace(olderBootstrapRule, currentInput, previousInput, 1)
	olderInputRule := strings.Replace(previousInputRule, previousInput, olderInput, 1)
	legacyBootstrapRule := strings.Replace(olderInputRule, olderBootstrap, legacyBootstrap, 1)
	withoutBootstrap := strings.Replace(legacyBootstrapRule, legacyBootstrap+"\n", "", 1)
	previousUIRef := strings.Replace(withoutBootstrap, currentUIRef+"\n", "", 1)
	legacyInputRule := strings.Replace(previousUIRef, olderInput, legacyInput, 1)
	return []string{previousBootstrapRule, previousPremiseEditRule, previousImageResultRule, previousReferenceRule, olderBootstrapRule, previousInputRule, olderInputRule, legacyBootstrapRule, withoutBootstrap, previousUIRef, legacyInputRule, strings.Replace(legacyInputRule, currentGuide, previousGuide, 1)}
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
