package promptcatalog

const jsonSystemPrompt = `You are a professional comic story planner and chapter writer.
Return one valid JSON object only. Do not include markdown fences, comments, or prose outside JSON.`

const storyProfilePromptZH = `根据用户故事想法生成 STORY.md 和章节计划。

用户故事想法：
{{input_prompt}}

章节数量：{{chapter_count}}

输出 JSON 字段：
{
  "story_md": "完整 STORY.md Markdown 正文",
  "chapter_plans": [{"chapter_code": "vol01.ch01", "title": "章节标题", "outline": "本章大纲"}]
}

约束：
- story_md 必须是非空 Markdown 字符串，不要使用 Markdown 代码块包裹
- story_md 应完整覆盖一句话故事、故事梗概、世界观和重要人物小传，可按故事需要增加其他章节
- story_md 建议以 ` + "`# STORY.md`" + ` 开头，并使用清晰的 Markdown 标题组织内容
- story_md 应优先精炼，通常控制在 800-1200 个字符；内容简单时可以更短，只有确有必要才可更长，最多不得超过 6000 个字符
- chapter_plans 数量必须等于 {{chapter_count}}
- chapter_code 必须从 vol01.ch01 开始顺序递增
- 每个 chapter_plan 代表一个故事章节，不是后续制作脚本或视觉设定
- 每章只规划一个清晰的剧情单元，包含主要目标、关键阻碍或情绪转折，以及收尾钩子
- 如果用户故事想法很大，不要为了在 {{chapter_count}} 章内讲完整部作品而压缩剧情；chapter_plans 只覆盖这 {{chapter_count}} 个章节能自然承载的剧情，后续剧情留待未来章节
- outline 写成故事大纲，避免多线并进、长时间跳跃、设定倾倒或连续多个重大反转
- 所有字段必须有内容，content 不要出现在本次输出`

const storyProfilePromptEN = `Generate STORY.md and chapter plans from the user's story idea.

User story idea:
{{input_prompt}}

Chapter count: {{chapter_count}}

Output JSON fields:
{
  "story_md": "complete STORY.md Markdown content",
  "chapter_plans": [{"chapter_code": "vol01.ch01", "title": "chapter title", "outline": "chapter outline"}]
}

Constraints:
- story_md must be a non-empty Markdown string and must not be wrapped in a Markdown code fence
- story_md should cover the logline, synopsis, worldview, and important character bios, and may include other useful sections
- story_md should start with ` + "`# STORY.md`" + ` and use clear Markdown headings
- story_md should be concise and usually 800-1,200 characters; it may be shorter for a simple story, should be longer only when necessary, and must not exceed 6,000 characters
- The number of chapter_plans must equal {{chapter_count}}
- chapter_code must start from vol01.ch01 and increase sequentially
- Each chapter_plan represents a story chapter, not a later production script or visual setting
- Each chapter should plan one clear plot unit, including the main goal, key obstacle or emotional turn, and an ending hook
- If the user's story idea is large, do not compress the whole work just to fit it into {{chapter_count}} chapters; chapter_plans should only cover the plot that these {{chapter_count}} chapters can naturally carry, leaving later plot for future chapters
- Write outline as a story outline, avoiding parallel multi-line plotting, long time jumps, exposition dumps, or multiple consecutive major twists
- All fields must have content; content must not appear in this output`

const profileFromChaptersPromptZH = `根据已有章节正文反推漫画 STORY.md。

已有章节：
{{chapters_json}}

输出 JSON 字段：
{
  "story_md": "完整 STORY.md Markdown 正文",
  "chapter_plans": []
}

约束：
- 只返回 JSON object
- story_md 必须是非空 Markdown 字符串，不要使用 Markdown 代码块包裹
- story_md 应完整覆盖一句话故事、故事梗概、世界观和重要人物小传，可按故事需要增加其他章节
- story_md 应优先精炼，通常控制在 800-1200 个字符；内容简单时可以更短，只有确有必要才可更长，最多不得超过 6000 个字符
- chapter_plans 可为空数组`

const profileFromChaptersPromptEN = `Infer the comic STORY.md from the existing chapter prose.

Existing chapters:
{{chapters_json}}

Output JSON fields:
{
  "story_md": "complete STORY.md Markdown content",
  "chapter_plans": []
}

Constraints:
- Return only a JSON object
- story_md must be a non-empty Markdown string and must not be wrapped in a Markdown code fence
- story_md should cover the logline, synopsis, worldview, and important character bios, and may include other useful sections
- story_md should be concise and usually 800-1,200 characters; it may be shorter for a simple story, should be longer only when necessary, and must not exceed 6,000 characters
- chapter_plans may be an empty array`

const storyChapterPromptZH = `根据 STORY.md 和当前章节计划生成单章正文。

用户原始 prompt：
{{input_prompt}}

当前 STORY.md：
<story-md>
{{story_md}}
</story-md>

当前 chapter plan：
{{chapter_plan_json}}

已生成章节摘要：
{{generated_summaries_json}}

输出 JSON 字段：
{
  "chapter_code": "{{chapter_code}}",
  "title": "章节标题",
  "content": "单章故事正文",
  "content_format": "txt"
}

约束：
- chapter_code 必须等于当前 chapter plan
- content_format 必须是 "txt"
- content 必须是可直接保存的单章故事正文，只包含连续叙事文本，不要返回 markdown 代码块
- content 不要写成后续制作脚本、格式化制作清单、视觉拆解或提示词
- 本章只承载一个核心事件，严格服务当前 chapter plan
- 正文目标长度 800-1400 个中文字符，上限约 1600 个中文字符；宁可留下悬念，不要塞入超出一章故事的信息量
- 新信息保持克制：最多引入 1 个主要目标、1 个关键阻碍、1 个情绪转折和 1 个结尾钩子
- 避免把多场战斗、多次调查、多地转场、完整世界观解释或整条人物成长线塞进本章
- 已生成章节摘要只用于保持连续性，不要在本章复述既有剧情`

const storyChapterPromptEN = `Generate the prose for a single chapter from STORY.md and the current chapter plan.

Original user prompt:
{{input_prompt}}

Current STORY.md:
<story-md>
{{story_md}}
</story-md>

Current chapter plan:
{{chapter_plan_json}}

Summaries of generated chapters:
{{generated_summaries_json}}

Output JSON fields:
{
  "chapter_code": "{{chapter_code}}",
  "title": "chapter title",
  "content": "single-chapter story prose",
  "content_format": "txt"
}

Constraints:
- chapter_code must equal the current chapter plan
- content_format must be "txt"
- content must be directly saveable single-chapter prose and contain only continuous narrative text; do not return a markdown code block
- Do not write content as a later production script, formatted production checklist, visual breakdown, or prompt
- This chapter should carry only one core event and strictly serve the current chapter plan
- Target length for the prose is 800-1400 English words, with an upper limit around 1600 English words; prefer leaving suspense over forcing in more information than one chapter can carry
- Keep new information restrained: introduce at most 1 main goal, 1 key obstacle, 1 emotional turn, and 1 ending hook
- Avoid stuffing multiple battles, repeated investigations, many location changes, a full worldbuilding explanation, or an entire character-growth arc into this chapter
- Use generated chapter summaries only for continuity; do not retell existing plot in this chapter`

const chapterBatchPlanPromptZH = `根据用户提示词，为一组待创建的连续 Chapter 编写章节计划。

用户提示词：
{{input_prompt}}

可选 STORY.md（为空时不要假设它存在）：
<story-md>
{{story_md}}
</story-md>

可选上一章 JSON（可能为 null，或 content 为空）：
{{previous_chapter_json}}

服务端确定的目标编号：
{{target_chapter_codes_json}}

输出 JSON 字段：
{
  "chapter_plans": [
    {"chapter_code": "目标编号", "title": "章节标题", "outline": "本章大纲"}
  ]
}

约束：
- chapter_plans 数量必须恰好为 {{chapter_count}}
- chapter_code 必须逐项严格等于服务端给出的目标编号，不得改号、跳号或增删
- 每章只规划一个清晰剧情单元，包含主要目标、关键阻碍或情绪转折，以及结尾钩子
- 用户提示词是必需且最高优先级的创作方向
- STORY.md 和上一章仅在存在时用于增强连续性；缺失时仅根据用户提示词完成计划
- 只规划本批章节，不生成 STORY.md，不输出章节正文`

const chapterBatchPlanPromptEN = `Plan a consecutive set of new Chapters from the user's prompt.

User prompt:
{{input_prompt}}

Optional STORY.md (do not assume it exists when empty):
<story-md>
{{story_md}}
</story-md>

Optional previous chapter JSON (may be null or have empty content):
{{previous_chapter_json}}

Server-assigned target chapter codes:
{{target_chapter_codes_json}}

Output JSON fields:
{
  "chapter_plans": [
    {"chapter_code": "target code", "title": "chapter title", "outline": "chapter outline"}
  ]
}

Constraints:
- chapter_plans must contain exactly {{chapter_count}} items
- Each chapter_code must exactly match the corresponding server-assigned target code; do not renumber, skip, add, or remove codes
- Each chapter should plan one clear plot unit with a main goal, key obstacle or emotional turn, and ending hook
- The user prompt is required and is the highest-priority creative direction
- Use STORY.md and the previous chapter only when present to improve continuity; when absent, plan solely from the user prompt
- Plan only this batch; do not generate STORY.md or chapter prose`

const nextStoryChapterPromptZH = `根据用户提示词生成下一章正文；如果提供了 STORY.md 或上一章正文，则用于增强连续性。

可选 STORY.md（可能为空）：
<story-md>
{{story_md}}
</story-md>

可选上一章 JSON（可能为 null，或 content 为空）：
{{previous_chapter_json}}

用户提示词（必填，作为本章最高优先级的创作方向）：
{{guidance_prompt}}

下一章 chapter_code：
{{next_chapter_code}}

输出 JSON 字段：
{
  "chapter_code": "{{next_chapter_code}}",
  "title": "章节标题",
  "content": "单章故事正文",
  "content_format": "txt"
}

约束：
- chapter_code 必须等于下一章 chapter_code
- content_format 必须是 "txt"
- content 必须是可直接保存的单章故事正文，只包含连续叙事文本，不要返回 markdown 代码块
- content 不要写成后续制作脚本、格式化制作清单、视觉拆解或提示词
- 如果上一章正文存在，应承接其事件、角色状态和结尾钩子，但不要大段复述
- 如果 STORY.md 存在，应遵循其设定；缺失 STORY.md 或上一章正文时允许仅根据用户提示词创作
- 本章只承载一个核心事件，严格服务用户提示词和已有的可选上下文
- 正文目标长度 800-1400 个中文字符，上限约 1600 个中文字符；宁可留下悬念，不要塞入超出一章故事的信息量
- 新信息保持克制：最多引入 1 个主要目标、1 个关键阻碍、1 个情绪转折和 1 个结尾钩子`

const nextStoryChapterPromptEN = `Generate the next chapter prose from the user prompt. Use STORY.md or previous chapter prose, when provided, to improve continuity.

Optional STORY.md (may be empty):
<story-md>
{{story_md}}
</story-md>

Optional previous chapter JSON (may be null or have empty content):
{{previous_chapter_json}}

User prompt (required and the highest-priority creative direction for this chapter):
{{guidance_prompt}}

Next chapter chapter_code:
{{next_chapter_code}}

Output JSON fields:
{
  "chapter_code": "{{next_chapter_code}}",
  "title": "chapter title",
  "content": "single-chapter story prose",
  "content_format": "txt"
}

Constraints:
- chapter_code must equal next_chapter_code
- content_format must be "txt"
- content must be directly saveable single-chapter prose and contain only continuous narrative text; do not return a markdown code block
- Do not write content as a later production script, formatted production checklist, visual breakdown, or prompt
- When previous chapter prose exists, continue its events, character states, and ending hook without retelling it at length
- When STORY.md exists, follow its setting; when STORY.md or previous prose is absent, generation may rely solely on the user prompt
- This chapter should carry only one core event and strictly serve the user prompt and available optional context
- Target length for the prose is 800-1400 English words, with an upper limit around 1600 English words; prefer leaving suspense over forcing in more information than one chapter can carry
- Keep new information restrained: introduce at most 1 main goal, 1 key obstacle, 1 emotional turn, and 1 ending hook`

func storyDefinitions(language string) []Definition {
	english := language == LanguageEnglish
	choose := func(chinese, englishValue string) string {
		if english {
			return englishValue
		}
		return chinese
	}
	titles := map[string][4]string{
		"json_system":           {"JSON 系统提示词", "约束 Story 生成任务只返回 JSON object。", "JSON system prompt", "Constrains Story generation tasks to return only a JSON object."},
		"story_profile":         {"STORY.md 与章节计划", "AI Story 生成的第一步，基于故事想法生成 STORY.md 和 chapter plans。", "STORY.md and chapter plan", "The first step of AI Story generation: generate STORY.md and chapter plans from a story idea."},
		"story_chapter":         {"单章正文", "AI Story 生成章节正文时使用，基于 STORY.md、chapter plan 和已生成摘要续写。", "Single chapter prose", "Used when AI Story generates chapter prose from STORY.md, the chapter plan, and generated summaries."},
		"chapter_batch_plan":    {"批量 Chapter 计划", "批量创建 Chapter 时，基于提示词和可选上下文规划服务端指定的连续章节。", "Batch Chapter plan", "Plans server-assigned consecutive Chapters from a prompt and optional story context."},
		"next_story_chapter":    {"下一章正文", "AI 追加下一章正文时使用，基于当前 STORY.md 和上一章正文续写。", "Next chapter prose", "Used when AI appends the next chapter from the current STORY.md and previous chapter prose."},
		"profile_from_chapters": {"基于章节生成 STORY.md", "上传章节后或手动重新生成 STORY.md 时使用。", "Generate STORY.md from chapters", "Used after uploading chapters or manually regenerating STORY.md."},
	}
	definition := func(key, value string) Definition {
		copy := titles[key]
		if english {
			return Definition{Group: GroupStory, Key: key, Title: copy[2], Description: copy[3], PromptType: PromptTypeTemplate, DefaultValue: value}
		}
		return Definition{Group: GroupStory, Key: key, Title: copy[0], Description: copy[1], PromptType: PromptTypeTemplate, DefaultValue: value}
	}
	return []Definition{
		definition("json_system", jsonSystemPrompt),
		definition("story_profile", choose(storyProfilePromptZH, storyProfilePromptEN)),
		definition("story_chapter", choose(storyChapterPromptZH, storyChapterPromptEN)),
		definition("chapter_batch_plan", choose(chapterBatchPlanPromptZH, chapterBatchPlanPromptEN)),
		definition("next_story_chapter", choose(nextStoryChapterPromptZH, nextStoryChapterPromptEN)),
		definition("profile_from_chapters", choose(profileFromChaptersPromptZH, profileFromChaptersPromptEN)),
	}
}
