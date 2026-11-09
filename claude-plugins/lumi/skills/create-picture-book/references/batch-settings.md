# 设定图来源与降级（Claude Code）

Claude Code 没有原生生图工具，Lumi 也没有为批量设定总览图或单项设定图提供服务端生图工具。**不得假装批量设定图可用**，也不能把只有文字的设定当作图片就绪。首次创作需要页面生图时，按下面的顺序为必要主体准备设定图，并在交付中说明采用了哪种方式。

## 1. 规划

读取已保存故事和 `settings list/get`，列出本次章节／目标页面会用到的角色、独立道具和场景。已有可用 `current_variant.asset` 且符合本次参考要求的项直接复用；画风不符的项保留身份，按下文重新获取图片并更新同一 setting_uuid。来源说明、版权备注等纯文字记录不需要图片。用户附带参考图时先按 [设定图的用户参考](setting-references.md) 区分用途、提取画风。

## 2. 选择降级方式

1. **用户提供图片**：用户给出真实本地图片文件时，Read 看图确认后用 `image_target(target_uuid, slot=setting_image)` 取 revision，`import_image` 导入。一张图含多个主体时，可用 `setting_boards import` 保存原图，再实际看图并 `apply_setting_breakdown` 裁切入库（每批 1–6 项，`crop_box` 为归一化坐标，不能重叠，`confidence` 至少 0.92；参数与幂等规则见 [MCP 调用参考](mcp-usage.md#批量设定图)）。
2. **在 Lumi 中生成**：给出 `web_url(view=settings)` 或对应 `web_url(setting_uuid)`，说明本批主体与参考要求，请用户在 Lumi 桌面端或网页的设定页面用已配置的服务商生成（可在其中生成批量总览图）。用户回到对话后 `settings list/get` 重读，实际 `read_image` 查看后再继续。
3. **仅文字设定**：只有用户明确接受时才跳过设定图；正文传 `setting_uuids=[]` 并保留理由。

不得用其他宿主的生图参数、脚本或第三方 API 自行补图，不切换付费渠道，不直接访问项目资产目录或 SQLite。

## 3. 核对与续做

设定图入库后按 [设定图核对](setting-references.md#4-核对后再用于正文) 实际看图并记录观察，再进入页面生图。用户在 Lumi 中生成或修改后，先重读设定与最新 revision；revision 冲突时重读，不覆盖网页中的修改。中断后用 `setting_boards list/get` 检查已保存的总览图与 `latest_breakdown`，已保存的图片无需重新获取。
