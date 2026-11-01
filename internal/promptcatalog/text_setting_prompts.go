package promptcatalog

// Text settings need their own catalog task: image generation and visual
// breakdown templates require artwork and cannot serve provider-free prose.
func textSettingsDefinition(language string) Definition {
	definition := Definition{Group: GroupPremise, Key: "text_settings", PromptType: PromptTypeTemplate}
	if language == LanguageEnglish {
		definition.Title = "Text setting cards"
		definition.Description = "Create reusable character, scene, prop and reference descriptions from the story without images."
		definition.DefaultValue = textSettingsEN
	} else {
		definition.Title = "文字设定卡"
		definition.Description = "从故事创建可复用的角色、场景、道具和参考文字描述，无需图片。"
		definition.DefaultValue = textSettingsZH
	}
	return definition
}

const textSettingsZH = `根据故事和任务输入，创作可复用的文字设定卡。

任务输入：
{{input_text}}

已保存的故事：
{{story_md}}

已有设定（JSON）：
{{premise_context_json}}

整体画风：
{{style_prompt}}

只输出 JSON：
{"settings":[{"asset_type":"character","title":"设定名称","summary":"完整文字描述","tags":["检索标签"]}]}

- asset_type 只允许 character、scene、prop、reference。
- 只创建故事需要的设定，避免重复已有身份；需要修改已有设定时沿用名称，由调用者重读其 UUID 和 revision 后更新。
- summary 保留身份特征、外观、动机、关系或空间细节，足够支持后续章节和页面创作；每项最多 12000 字符。
- 不要求参考图，不输出图片路径、图像生成任务、裁切坐标或虚构 UUID，不调用任何模型服务商。
- tags 使用简短检索词，最多 50 项。调用者逐项保存并核对，不自动执行批量写入。`

const textSettingsEN = `Create reusable text setting cards from the story and task input.

Task input:
{{input_text}}

Saved story:
{{story_md}}

Existing settings (JSON):
{{premise_context_json}}

Visual style:
{{style_prompt}}

Return JSON only:
{"settings":[{"asset_type":"character","title":"Setting name","summary":"Complete text description","tags":["search tag"]}]}

- asset_type must be character, scene, prop or reference.
- Include only settings needed by the story. Avoid duplicating existing identities; retain an existing title when proposing an update, so the caller can reread its UUID and revision.
- Preserve identity, appearance, motivation, relationships or spatial details in summary for subsequent chapters and pages; at most 12000 characters per item.
- No reference image is required. Do not emit image paths, image generation jobs, crop coordinates or invented UUIDs. Do not call a model provider.
- Use short search tags, at most 50 per item. The caller saves and verifies each item separately; do not execute a batch write.`
