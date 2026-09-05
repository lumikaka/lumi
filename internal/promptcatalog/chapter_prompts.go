package promptcatalog

const comicStoryboardPromptZH = `根据当前章节文本生成手机竖向条漫的漫画分集脚本。

当前 chapter：
{{chapter_context_json}}

当前 STORY.md：
<story-md>
{{story_md}}
</story-md>

原始剧情 / storyboard 文本：
{{input_text}}

本次每个 section 的关键视觉瞬间数量随机分布：
{{moment_count_plan}}

输出 JSON 字段：
{
  "chapter_code": "{{chapter_code}}",
  "title": "chapter 标题",
  "sections": [
    {
      "section_no": 1,
      "title": "section 标题",
      "storyboard": "markdown 格式的 section 漫画脚本"
    }
  ]
}

section 约束：
- sections 数量必须在 1 到 {{max_section_count}} 之间，由剧情节奏自然拆分
- 每个 section 是 image2 模型生成一张宽高比 1:3 图片的单位
- storyboard 必须是 markdown 文本，不要使用代码块
- 每个 storyboard 必须先写 "## Section 核心剧情目标"，再写 "## 关键视觉瞬间"
- 必须保留 "美术风格要和设定图保持一致。" 和 "涉及设定图：..." 这类生成约束
- 每个 section 的关键视觉瞬间数量必须按上面的随机分布执行；实际生成几个 sections，就使用前几个 section 对应的数量
- 单个 section 最多 {{max_moments_per_section}} 个关键视觉瞬间，不要写 4 个或更多
- 不同 sections 的瞬间数量要保持随机分布，允许有的 section 只有 1 个，有的 section 有 2-3 个，不要机械地每个 section 都写相同数量
- 瞬间标题必须使用 "**瞬间 N：标题**（画面占比 / 分镜形态 / 阅读位置说明）"
- 瞬间编号只在单个 section 内计数；每个 section 都必须从 **瞬间 1** 开始，然后在本 section 内递增为 2、3
- 每个瞬间用 bullet 字段描述：镜头与调度、人物与动作、场景设定 / 怪物设定 / 神灵设定 / 动作推进、光影与色彩、旁白框N、对白气泡N（角色名）、音效字N、分镜效果
- 对白要拆成短气泡，格式必须为 ` + "`* **对白气泡N（角色名）**：台词。`" + `，例如 ` + "`* **对白气泡1（赵云川）**：他是被人盯上了。`" + `
- 禁止把说话人写进台词正文，例如不要写成 ` + "`* **对白气泡1**：赵云川：他是被人盯上了。`" + `
- 如果需要内心独白，格式必须为 ` + "`* **内心气泡N（角色名）**：内心独白。`" + `；旁白框不写角色名
- 音效写成 【嗡——】、【轰隆】 这类漫画音效字
- 分镜始终服务于竖向手机条漫，说明阅读动线和画面占比`

const comicStoryboardPromptEN = `Generate a comic episode script for a vertical mobile scrolling comic from the current chapter text.

Current chapter:
{{chapter_context_json}}

Current STORY.md:
<story-md>
{{story_md}}
</story-md>

Original plot / storyboard text:
{{input_text}}

Random distribution of key visual moments for each section in this run:
{{moment_count_plan}}

Output JSON fields:
{
  "chapter_code": "{{chapter_code}}",
  "title": "chapter title",
  "sections": [
    {
      "section_no": 1,
      "title": "section title",
      "storyboard": "section comic script in markdown format"
    }
  ]
}

Section constraints:
- The number of sections must be between 1 and {{max_section_count}}, split naturally according to story pacing
- Each section is the unit used by the image2 model to generate one image with a 1:3 aspect ratio
- storyboard must be markdown text; do not use code blocks
- Each storyboard must first include "## Section Core Plot Goal", then "## Key Visual Moments"
- Preserve generation constraints such as "The art style must remain consistent with the setting images." and "Referenced setting images: ..."
- The number of key visual moments in each section must follow the random distribution above; if fewer sections are actually generated, use the first corresponding section counts
- A single section may contain at most {{max_moments_per_section}} key visual moments; do not write 4 or more
- Keep the moment counts randomly distributed across sections; some sections may have only 1 moment while others have 2-3, and do not mechanically write the same count for every section
- Moment titles must use "**Moment N: Title** (screen share / panel form / reading-position note)"
- Moment numbering is local to a single section; every section must start from **Moment 1** and then increment within that section to 2 and 3
- Describe each moment with bullet fields: camera and staging, characters and actions, scene setting / monster setting / deity setting / action progression, light and color, narration box N, dialogue bubble N (speaker name), sound-effect text N, panel effect
- Split dialogue into short bubbles, and use the exact format ` + "`* **Dialogue Bubble N (Speaker Name)**: line.`" + `, for example ` + "`* **Dialogue Bubble 1 (Zhao Yunchuan)**: He is being watched.`" + `
- Do not put the speaker name inside the dialogue line, for example do not write ` + "`* **Dialogue Bubble 1**: Zhao Yunchuan: He is being watched.`" + `
- If an inner monologue is needed, use the exact format ` + "`* **Inner Bubble N (Character Name)**: inner monologue.`" + `; narration boxes do not include speaker names
- Write sound effects as comic SFX such as [HUMM--] or [BOOM]
- Panels must always serve a vertical mobile scrolling comic; describe the reading flow and screen share`

const coverStoryboardPromptZH = `为当前绘本设计一张可直接生成最终图片的封面分镜。

绘本标题（必须逐字保留）：
{{book_title}}

当前绘本（技术对象 chapter）：
{{chapter_context_json}}

当前 STORY.md：
<story-md>
{{story_md}}
</story-md>

用户最初的故事创意：
{{story_prompt}}

正文第一页 storyboard：
<first-body-storyboard>
{{first_body_storyboard}}
</first-body-storyboard>

只输出以下 JSON object：
{
  "title": "{{book_title}}",
  "storyboard": "markdown 格式的完整封面脚本"
}

封面规则：
- title 必须与给定绘本标题完全一致
- storyboard 必须描述一张平面的最终封面画布，不得生成书本样机、立体书、展开封套、书脊、封底、条码、出版社标识或作者署名
- 使用一个强而清晰的主视觉概括故事主题，并与正文第一页在角色身份、服装、道具、场景和画风上连续
- 不剧透结局，不把正文第一页机械复制到封面
- 明确描述标题的准确文字、位置、字号层级、留白，以及不得遮挡的人物脸部和核心视觉
- 除给定绘本标题外，不得自行添加副标题、宣传语、作者名、奖项或其他可读文字
- storyboard 必须是 markdown 文本，不要使用代码块`

const coverStoryboardPromptEN = `Design a front-cover storyboard that can be used directly to generate the final cover image for the current picture book.

Picture-book title (preserve it verbatim):
{{book_title}}

Current picture book (technical chapter object):
{{chapter_context_json}}

Current STORY.md:
<story-md>
{{story_md}}
</story-md>

The user's original story idea:
{{story_prompt}}

Storyboard of the first body page:
<first-body-storyboard>
{{first_body_storyboard}}
</first-body-storyboard>

Output only this JSON object:
{
  "title": "{{book_title}}",
  "storyboard": "complete front-cover script in markdown"
}

Front-cover rules:
- title must exactly match the supplied picture-book title
- storyboard must describe one flat final cover canvas; do not generate a book mockup, 3D book, unfolded dust jacket, spine, back cover, barcode, publisher mark, or author credit
- Use one strong, clear hero visual that captures the story theme and stays continuous with the first body page in character identity, costume, props, setting, and art style
- Do not spoil the ending or mechanically copy the first body page onto the cover
- Specify the exact title copy, placement, typographic hierarchy, whitespace, and the faces and core visual elements that the title must not cover
- Do not invent a subtitle, tagline, author name, award, or any other readable copy beyond the supplied picture-book title
- storyboard must be markdown text without code fences`

const coverStoryboardHeadingsZH = `- storyboard 必须使用 Markdown 二级标题（` + "`##`" + `）组织内容，不要使用一级标题；至少依次包含：` + "`## 画布基础设定`" + `、` + "`## 场景与光影`" + `、` + "`## 核心视觉与人物构图`" + `、` + "`## 标题排版与留白`" + `、` + "`## 细节点缀`" + `
- 每个二级标题必须单独占一行，并把对应内容写在标题下方；禁止用粗体标签或“字段名：内容”代替二级标题`

const coverStoryboardHeadingsEN = `- The storyboard must organize its content with Markdown level-two headings (` + "`##`" + `), without level-one headings, and include at least these headings in order: ` + "`## Canvas Basics`" + `, ` + "`## Setting and Lighting`" + `, ` + "`## Hero Visual and Character Composition`" + `, ` + "`## Title Layout and Whitespace`" + `, and ` + "`## Finishing Details`" + `
- Put every level-two heading on its own line and write its content below it; do not replace headings with bold labels or ` + "`Field: content`" + ` lines`

const coverBeforeImagePromptZH = `## 封面图片生成规则

1. 当前输入是绘本 front cover 的完整 storyboard。生成一张可直接阅读和导出的平面最终封面图片，不是正文页。
2. 严格按 storyboard 逐字绘制绘本标题；不得改写、漏字、增加副标题、宣传语、作者名、奖项、出版社标识或其他可读文字。
3. 使用一个清晰有力的主视觉概括故事主题。标题应有明确层级与充足留白，不得遮挡人物脸部、核心动作或关键道具。
4. 封面角色身份、服装、道具、场景与画风必须和设定参考及正文第一页连续一致，但不得机械复制正文第一页。
5. 禁止生成书本样机、立体书、摆拍页面、展开封套、书脊、封底、条码、价格贴纸、装饰相框或多个独立缩略图拼贴。
6. 正文页的无字、叙事句数、互动提问、漫画分格数量或连续剧情推进规则不适用于封面；以 storyboard 的封面构图和逐字标题为准。
7. 有设定参考图时只参考主体身份与视觉特征，不得复制设定拼贴图的网格排版。`

const coverBeforeImagePromptEN = `## Front-cover Image-generation Rules

1. The current input is the complete storyboard for the picture book's front cover. Generate one flat final cover image ready for reading and export, not a body page.
2. Render the picture-book title from the storyboard verbatim. Do not rewrite it, omit characters, or add a subtitle, tagline, author name, award, publisher mark, or any other readable copy.
3. Use one clear, strong hero visual that captures the story theme. Give the title a clear hierarchy and sufficient whitespace, and do not cover faces, core actions, or key props.
4. Keep character identity, costume, props, setting, and art style continuous with setting references and the first body page, but do not mechanically copy the first body page.
5. Do not generate a book mockup, 3D book, photographed page, unfolded jacket, spine, back cover, barcode, price sticker, decorative frame, or collage of independent thumbnails.
6. Body-page rules for wordlessness, narrative-sentence count, interactive questions, comic-panel count, or continuous plot progression do not apply to the cover. Follow the storyboard's cover composition and verbatim title instead.
7. When setting references are present, use them only for subject identity and visual features; never copy the reference collage grid.`

const backCoverBeforeImagePromptZH = `## 封底图片生成规则

1. 当前输入是绘本 back cover 的完整 storyboard。生成一张可直接阅读和导出的平面最终封底图片，不是正文页，也不是正封面。
2. 只绘制 storyboard 明确要求的可读文字并逐字保留；不得自行添加故事正文、互动提问、对白、宣传语、作者名、奖项、出版社标识、条码、价格或其他文案。
3. 使用安静、收束性的辅助视觉延续整本书的主题与气氛，保持适当留白和较低视觉密度；不得把正封标题机械重复为封底主标题，除非 storyboard 明确要求。
4. 封底角色身份、服装、道具、场景与画风必须和设定参考及正文连续一致，但不得机械复制正文页面或正封面。
5. 禁止生成书本样机、立体书、摆拍页面、展开封套、书脊、正封面、装饰相框或多个独立缩略图拼贴。
6. 正文页的无字、叙事句数、互动提问、漫画分格数量或连续剧情推进规则不适用于封底；以 storyboard 的封底构图和明确文案为准。
7. 有设定参考图时只参考主体身份与视觉特征，不得复制设定拼贴图的网格排版。`

const backCoverBeforeImagePromptEN = `## Back-cover Image-generation Rules

1. The current input is the complete storyboard for the picture book's back cover. Generate one flat final back-cover image ready for reading and export, not a body page or front cover.
2. Render only readable copy explicitly required by the storyboard and preserve it verbatim. Do not invent story prose, an interactive question, dialogue, a tagline, author name, award, publisher mark, barcode, price, or any other copy.
3. Use a quiet, resolving supporting visual that extends the book's theme and atmosphere, with appropriate whitespace and lower visual density. Do not mechanically repeat the front-cover title as a back-cover headline unless the storyboard explicitly requires it.
4. Keep character identity, costume, props, setting, and art style continuous with setting references and body pages, but do not mechanically copy a body page or the front cover.
5. Do not generate a book mockup, 3D book, photographed page, unfolded jacket, spine, front cover, decorative frame, or collage of independent thumbnails.
6. Body-page rules for wordlessness, narrative-sentence count, interactive questions, comic-panel count, or continuous plot progression do not apply to the back cover. Follow the storyboard's back-cover composition and explicit copy instead.
7. When setting references are present, use them only for subject identity and visual features; never copy the reference collage grid.`

const settingSelectionPromptZH = `你是漫画设定项选择器。请根据当前 storyboard，从给定的 comic-settings 设定项标题中选出本 section 会实际涉及的设定元素。

规则：
- 只能选择“可选设定项标题”列表中真实存在的标题。
- 不要创造新标题，不要返回文件名或路径。
- 优先选择 storyboard 直接提到的人物、神灵、怪物、场景、阵营、道具和关键视觉设定。
- 不要因为整体风格相似就选择无关元素。
- 最多选择 {{max_files}} 个标题；如果只需要更少，就返回更少。
- 只输出 JSON，不要代码块，不要解释过程。
- titles 必须从“可选设定项标题”列表中原样复制，不要为了适配项目语言而翻译或改写。
- reason 必须使用当前项目语言；JSON key 保持示例中的英文 key。

JSON 格式：
{
  "sectionId": "{{section_id}}",
  "titles": ["设定项标题"],
  "reason": "一句话说明选择依据"
}

## Section
{{section_id}}

## 可选设定项标题
{{titles}}

## Storyboard
{{storyboard}}`

const settingSelectionPromptEN = `You are a comic setting-asset selector. Based on the current storyboard, select the setting elements from the given comic-settings titles that this section actually involves.

Rules:
- Select only real titles from the "Available setting item titles" list.
- Do not invent new titles, and do not return filenames or paths.
- Prefer characters, deities, monsters, scenes, factions, props, and key visual settings directly mentioned by the storyboard.
- Do not select unrelated elements only because the overall style is similar.
- Select at most {{max_files}} titles; if fewer are needed, return fewer.
- Output only JSON. Do not include code blocks or explanations.
- titles must be copied exactly from the "Available setting item titles" list; do not translate or rewrite them to fit the project language.
- reason must use the current project language; JSON keys must remain the English keys shown in the example.

JSON format:
{
  "sectionId": "{{section_id}}",
  "titles": ["setting item title"],
  "reason": "one-sentence reason for the selection"
}

## Section
{{section_id}}

## Available setting item titles
{{titles}}

## Storyboard
{{storyboard}}`

const beforeImagePromptZH = `## 生成规则

1. 基于当前输入的 section storyboard，生成竖向手机条漫长图。画面宽高比为 1:3。成图必须是可阅读的连续漫画页面，不是单张插画、封面、海报、设定集、角色立绘页或画册排版。

2. 画面采用成熟 2D 动画截图式彩色条漫质感：赛璐璐上色，清晰线稿，角色轮廓明确，平涂色块干净，阴影克制，画面简洁清爽，适合手机端上下滚动阅读。

3. 整体必须是竖向连续叙事结构。每个 section 已提炼 3-5 个关键视觉瞬间，用连续镜头串联剧情，使场景、人物动作、表情、光影和空间关系自然过渡。

4. 禁止生成设定集网格、角色三视图、角色排排站、海报拼贴、多个独立缩略图、横版漫画页、规则九宫格、装饰边框、相框式排版。

5. 允许使用手机条漫常见分镜方式，包括黑色留白、自然硬切、宽幅场景镜头、竖向拉伸镜头、黑底字幕区、局部特写、大场面远景。但分镜必须服务剧情推进，不得做成机械矩形拼贴或封面式构图。

6. 每个视觉瞬间都必须推动剧情。读者应能通过角色动作、表情、场景变化、道具状态、已有对白和音效理解事件发展。不要只堆叠氛围画面。

7. 如果 storyboard 内容较多，优先保留剧情转折、主要角色动作、核心道具、关键表情和重要对白。宁可减少视觉瞬间，也不要压缩文字或堆满小画面。

8. 画面应有明确阅读节奏：建立场景 → 角色行动 → 冲突升级 → 关键反应或结果。重要剧情画面应占据更大空间，次要过渡画面可压缩。

9. 角色必须保持辨识度。脸部、发型、体型、服装层次、武器、道具和姿态要清楚，不要被特效、烟雾、背景或文字遮挡。

10. 场景应有清晰可辨的材质差异，例如天空、云层、烟尘、火光、碎石、金属、布料、玻璃、建筑、能量粒子、战斗痕迹等。材质以简洁线条、明确色块和少量高光区分，不追求写实纹理；背景不能只是模糊色块或纯氛围纹理。

11. 光影保持明确但克制。能量光、火光、霓虹、夕阳、爆炸、星光或环境反光可形成少量边缘光、高光和阴影，但整体应偏动画扁平感，避免厚重暗部和电影级写实光影。

12. 主要对白、音效和字幕必须字号偏大、对比清楚、留白充足，适合手机阅读。文字不得压在人物脸部、核心道具、重要动作或视觉中心上。

13. 对白气泡、内心气泡、旁白字幕的数量由当前 storyboard 中的明确提示控制。如果 storyboard 已列出气泡数量、编号或具体对白，必须严格按该数量和顺序呈现。

14. 不要新增对白气泡、内心气泡或旁白字幕。不要主动把人物震惊、催促、疑问、恐惧、判断或叙述句改写成新的气泡文本。

15. 已有对白必须逐字保留，不得摘要、改写、扩写、拆分或合并。对白内容保真优先级高于画面密度。

16. 如果单个气泡文字较长，应通过放大气泡、增加留白、减少视觉瞬间、调整构图来保证可读性。不得压扁、拉窄、倾斜或缩小主要对白文字。

17. 如果当前输入没有明确要求对白或旁白，可以生成无文字页面，不要为了漫画感主动添加台词。

18. 中文文字必须清晰端正，避免乱码、错字、变形、过度艺术字化或难以阅读的书法化处理。

19. 画面风格应避免明亮低幼、Q版萌系、廉价平涂卡通、纯游戏立绘、欧美超级英雄海报、单幅概念艺术、电影海报或封面式构图。允许成熟赛璐璐动画式平涂色块。

20. 最终画面应像一张完整的手机竖向条漫 section：从上到下阅读流畅，剧情清楚，人物动作明确，文字可读，分镜自然，具有连续漫画叙事感。


## 分镜构图要求

1. 竖向长条韩漫式分镜，不要传统四格漫画，不要上下等高矩形分栏，不要整齐网格。

2. 使用不规则分镜：大面积留白、斜切分镜、破框人物、无边框过渡、局部特写插入、环境大景与小特写交错。

3. 画面阅读方向从上到下呈 S 型流动，而不是一块一块平铺。


## 设定图约束

1. 当前项目的角色设定、服装设定、道具设定、场景设定和色彩规范是统一美术标准，生成时必须保持一致。

2. 当输入包含参考图时，它是一张 Section 专属设定拼贴图，标签列出了本次实际涉及的设定项。仅把各图片格作为角色、服装、道具、材质、色彩、世界观和光影风格参考，不得模仿拼贴图的网格排版。

3. 如果 storyboard 与设定图发生冲突：
   - 剧情事件、动作、镜头、情绪可以按 storyboard 执行；
   - 角色身份识别特征不得被 storyboard 改写；
   - 不得改变角色发型、发色、脸型、体型、核心服装结构、武器、神契纹样和主色调；
   - 年龄、受伤、疲惫、战损只能作为表面状态变化，不能把角色重塑成另一个人。

4. 同一角色在不同镜头中必须保持发型、脸型、体型、服饰结构、武器和核心识别特征一致。

5. 同一场景在不同镜头中必须保持空间关系、光源方向、材质质感和环境气氛一致。

6. 不得把设定拼贴图其他图片格中角色的外貌、铠甲、翅膀、发色、武器或神将特征转移到主角身上。

7. 允许根据剧情表现角色的战损、疲惫、受伤、沾血、灰尘、衣物破损、能量缠绕，但这些变化不得覆盖角色原本的发型、脸型、服装结构和核心符号。

8. 能量特效不得遮挡角色脸部、发型、手部动作、神契纹样、武器和关键道具。`

const beforeImagePromptEN = `## Generation Rules

1. Based on the current section storyboard, generate a vertical mobile scrolling comic long image. The image aspect ratio is 1:3. The result must be a readable continuous comic page, not a single illustration, cover, poster, setting collection, character standee sheet, or artbook layout.

2. The image should use a mature 2D animation screenshot-like color comic texture: cel shading, clear line art, distinct character silhouettes, clean flat color blocks, restrained shadows, and a clean refreshing image suitable for vertical scrolling on mobile.

3. The whole image must be a vertical continuous narrative structure. Each section has extracted 3-5 key visual moments; connect the plot with continuous shots so scene, character action, expression, lighting, and spatial relationships transition naturally.

4. Do not generate setting-sheet grids, character turnaround views, lineups of characters, poster collages, multiple independent thumbnails, horizontal comic pages, regular nine-grid layouts, decorative borders, or framed layouts.

5. You may use common mobile scrolling-comic panel methods, including black negative space, natural hard cuts, wide establishing shots, vertically stretched shots, black-background caption zones, partial close-ups, and large-scale distant views. Panels must serve plot progression and must not become mechanical rectangular collage or cover-style composition.

6. Every visual moment must advance the plot. Readers should understand event progression through character actions, expressions, scene changes, prop states, existing dialogue, and sound effects. Do not merely stack atmospheric images.

7. If the storyboard contains a lot of content, prioritize plot turns, main character actions, core props, key expressions, and important dialogue. Prefer reducing visual moments over compressing text or crowding the page with small images.

8. The image should have clear reading rhythm: establishing the scene -> character action -> conflict escalation -> key reaction or result. Important plot images should take more space, while minor transitions may be compressed.

9. Characters must remain recognizable. Faces, hairstyles, body types, clothing layers, weapons, props, and poses must be clear and not blocked by effects, smoke, backgrounds, or text.

10. Scenes should have clearly distinguishable materials, such as sky, clouds, dust, firelight, rubble, metal, fabric, glass, buildings, energy particles, and battle traces. Distinguish materials with simple lines, clear color blocks, and limited highlights rather than realistic texture; backgrounds must not be only blurry color blocks or pure atmospheric textures.

11. Lighting should stay clear but restrained. Energy light, firelight, neon, sunset, explosions, starlight, or environmental reflections may create limited rim light, highlights, and shadows, but the whole image should lean toward an animation flat-color feeling and avoid heavy dark areas or cinematic realistic lighting.

12. Main dialogue, sound effects, and captions must use relatively large text, clear contrast, and sufficient whitespace for mobile reading. Text must not cover faces, core props, important actions, or visual centers.

13. The number of dialogue bubbles, inner-thought bubbles, and narration captions is controlled by explicit instructions in the current storyboard. If the storyboard lists bubble counts, numbering, or exact dialogue, strictly follow that number and order.

14. Do not add dialogue bubbles, inner-thought bubbles, or narration captions. Do not proactively rewrite shock, urging, questions, fear, judgment, or narration into new bubble text.

15. Existing dialogue must be preserved verbatim; do not summarize, rewrite, expand, split, or merge it. Dialogue fidelity has higher priority than image density.

16. If a single bubble contains long text, ensure readability by enlarging the bubble, adding whitespace, reducing visual moments, and adjusting composition. Do not squash, narrow, tilt, or shrink main dialogue text.

17. If the current input does not explicitly require dialogue or narration, generate a textless page; do not add lines just to make it feel more like comics.

18. English text must be clear and upright, avoiding gibberish, misspellings, deformation, over-stylized lettering, or hard-to-read decorative treatment.

19. Avoid bright childish looks, chibi/cute style, cheap flat-color cartooning, pure game character art, American superhero poster style, single concept-art illustration, movie poster, or cover-style composition. Mature cel-shaded animation-like flat color blocks are allowed.

20. The final image should look like a complete vertical mobile scrolling comic section: smooth top-to-bottom reading, clear plot, distinct character actions, readable text, natural panels, and continuous comic storytelling.


## Panel Composition Requirements

1. Use vertical long-strip Korean-webtoon-like paneling; do not use traditional four-panel comics, equal-height top/bottom rectangular divisions, or neat grids.

2. Use irregular panels: large negative space, diagonal cuts, characters breaking frames, borderless transitions, inserted close-ups, and alternating environment wide shots with small close-ups.

3. The image reading direction should flow in an S shape from top to bottom rather than being laid out block by block.


## Setting Image Constraints

1. The current project's character settings, costume settings, prop settings, scene settings, and color specifications are the unified art standard and must remain consistent during generation.

2. When a reference image is provided, it is one Section-specific setting collage whose labels identify the setting items involved in this generation. Use each image cell only as a reference for characters, costumes, props, materials, colors, worldbuilding, and lighting style; do not imitate the collage grid layout.

3. If the storyboard conflicts with setting images:
   - Plot events, actions, camera shots, and emotions may follow the storyboard;
   - Character identity features must not be rewritten by the storyboard;
   - Do not change a character's hairstyle, hair color, face shape, body type, core costume structure, weapons, divine-contract patterns, or main color scheme;
   - Age, injuries, fatigue, battle damage may appear only as surface-state changes and must not remake the character into someone else.

4. The same character must keep hairstyle, face shape, body type, clothing structure, weapons, and core identity features consistent across different shots.

5. The same scene must keep spatial relationships, light-source direction, material feel, and environmental atmosphere consistent across different shots.

6. Do not transfer the appearance, armor, wings, hair color, weapons, or divine-warrior features from characters in other cells of the setting collage onto the protagonist.

7. You may show battle damage, fatigue, injury, bloodstains, dust, clothing tears, or energy entanglement according to the plot, but these changes must not cover the character's original hairstyle, face shape, clothing structure, and core symbols.

8. Energy effects must not block character faces, hairstyles, hand actions, divine-contract patterns, weapons, or key props.`

func chapterDefinitions(language string) []Definition {
	english := language == LanguageEnglish
	choose := func(chinese, englishValue string) string {
		if english {
			return englishValue
		}
		return chinese
	}
	meta := func(key, zhTitle, zhDescription, enTitle, enDescription, promptType, value string) Definition {
		if english {
			return Definition{Group: GroupChapter, Key: key, Title: enTitle, Description: enDescription, PromptType: promptType, DefaultValue: value}
		}
		return Definition{Group: GroupChapter, Key: key, Title: zhTitle, Description: zhDescription, PromptType: promptType, DefaultValue: value}
	}
	before := choose(beforeImagePromptZH, beforeImagePromptEN)
	section := "{{before_image_prompt}}" + choose("\n\n{{style_prompt}}\n\n## Section 设定拼贴图内容\n{{reference_usage_text}}\n\n## 当前 Section\n{{section_id}}\n\n## Storyboard\n{{storyboard}}", "\n\n{{style_prompt}}\n\n## Section setting collage contents\n{{reference_usage_text}}\n\n## Current Section\n{{section_id}}\n\n## Storyboard\n{{storyboard}}")
	referencePresent := choose("本次提供一张 Section 专属设定拼贴图，其中包含以下带标签的设定项；请依据拼贴图保持它们的身份与核心视觉特征：\n{{reference_titles}}", "One Section-specific setting collage image is provided. It contains the following setting items; use their labeled visual references to preserve identity and core visual features:\n{{reference_titles}}")
	referenceAbsent := choose("本次没有可用的设定参考图。仅根据 Storyboard 与画风生成，不要声称遵循未提供的角色或场景设定。", "No setting reference images are available for this generation. Generate only from the storyboard and art style, and do not claim consistency with character or scene references that were not provided.")
	additionalDirection := choose("## 用户补充要求\n{{guidance_prompt}}", "## Additional user direction\n{{guidance_prompt}}")
	coverStoryboardPrevious := choose(coverStoryboardPromptZH, coverStoryboardPromptEN)
	coverStoryboard := coverStoryboardPrevious + "\n" + choose(coverStoryboardHeadingsZH, coverStoryboardHeadingsEN)
	coverStoryboardDefinition := meta("cover_storyboard", "绘本封面分镜", "根据绘本标题、故事和正文第一页生成封面 storyboard。", "Picture-book cover storyboard", "Generate a front-cover storyboard from the picture-book title, story, and first body page.", PromptTypeTemplate, coverStoryboard)
	coverStoryboardDefinition.PreviousDefaultValues = []string{coverStoryboardPrevious}
	return []Definition{
		meta("json_system", "JSON 系统提示词", "约束漫画分集脚本生成任务只返回 JSON object。", "JSON system prompt", "Constrains comic episode script generation tasks to return only a JSON object.", PromptTypeTemplate, jsonSystemPrompt),
		meta("comic_storyboard", "漫画分集脚本", "从章节正文或输入文本生成手机竖向条漫的 section storyboard。", "Comic episode script", "Generate section storyboards for a vertical mobile scrolling comic from chapter prose or input text.", PromptTypeTemplate, choose(comicStoryboardPromptZH, comicStoryboardPromptEN)),
		coverStoryboardDefinition,
		meta("section_premise_selection", "Section 设定项选择", "根据当前 section storyboard 从 Premise 设定项中选择参考文件。", "Section setting asset selection", "Select reference files from Premise setting assets according to the current section storyboard.", PromptTypeTemplate, choose(settingSelectionPromptZH, settingSelectionPromptEN)),
		meta("before_image", "Section 图片基础规则", "组合进 Section 图片模板的基础生成规则。", "Section image base rules", "Base generation rules composed into the Section image template.", PromptTypeFragment, before),
		meta("cover_before_image", "封面图片基础规则", "生成 front cover 图片时替代正文页面基础规则。", "Front-cover image base rules", "Replace body-page base rules when generating a front-cover image.", PromptTypeFragment, choose(coverBeforeImagePromptZH, coverBeforeImagePromptEN)),
		meta("back_cover_before_image", "封底图片基础规则", "生成 back cover 图片时替代正文页面基础规则。", "Back-cover image base rules", "Replace body-page base rules when generating a back-cover image.", PromptTypeFragment, choose(backCoverBeforeImagePromptZH, backCoverBeforeImagePromptEN)),
		meta("section_reference_present", "有设定参考图时的说明", "Section 图片存在设定拼贴图时组合进生成提示词。", "Setting-reference-present instruction", "Composed into the Section image prompt when a setting collage is available.", PromptTypeFragment, referencePresent),
		meta("section_reference_absent", "无设定参考图时的说明", "Section 图片没有设定拼贴图时组合进生成提示词。", "Setting-reference-absent instruction", "Composed into the Section image prompt when no setting collage is available.", PromptTypeFragment, referenceAbsent),
		meta("section_additional_direction", "用户补充要求包装", "用户为 Section 图片补充要求时组合进生成提示词。", "Additional user direction wrapper", "Composed into the Section image prompt when the user adds generation guidance.", PromptTypeFragment, additionalDirection),
		meta("section_image", "Section 图片生成", "根据基础规则、section storyboard、Section 设定拼贴图和画风生成竖向条漫图片。", "Section image generation", "Generate a vertical scrolling comic image from base rules, the section storyboard, one Section setting collage, and art style.", PromptTypeTemplate, section),
	}
}
