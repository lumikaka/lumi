# Guide：更新 Storyboard

能力 ID：`storyboard_update`

本 Guide 用于读取 Comic Section 的完整当前 Storyboard，并以完整 Markdown 创建新的 Storyboard 版本。API 字段以 `/api/v1/agent-docs/comic-section.md` 和 `/api/v1/agent-docs/storyboard.md` 为准。

## 前置条件

- 已知目标 `project_uuid`、`chapter_uuid` 和 `section_uuid`。绑定 Section 是默认操作对象，不是 API 权限边界。
- 用户已要求落地修改；仅讨论、评审或起草时不要写入。

## 标准步骤

1. 调用 `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}`，使用 `.data | {uuid,title,current_storyboard,revision}` 读取完整 Storyboard 和最新 revision。
2. 基于完整 `current_storyboard` 生成完整替换后的 Storyboard Markdown。保留用户未要求改变且仍有效的内容，不提交局部 diff。
3. 调用 `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/storyboard-variants`，提交完整 `content_md` 和刚读取的 `expected_revision`。
4. 写入响应使用窄化投影，例如 `.data | {uuid,current_storyboard,revision}`；只有成功信封才能视为更新完成。

## 禁止捷径

- 不得在写前省略完整 GET，不得只读取摘要字段后覆盖 Storyboard。
- 不得提交补丁片段、省略未修改段落，或使用对话上下文中的旧 revision。
- 不得把 POST 已排队、失败或 conflict 描述为已经更新。

## 失败恢复

- revision conflict：重新 GET 完整 Storyboard，重新合并用户意图与最新内容，再用新的 revision 提交；不得原样重放旧请求。
- 目标不存在或归属不匹配：核对 Chapter 与 Section UUID，不要猜测或跨项目替换。
- 响应过大：保持 GET 包含完整 `current_storyboard`，但让其他字段和写入响应继续窄化；不要为了缩小响应而丢失被替换正文。

## 对应 API Contract

- `/api/v1/agent-docs/comic-section.md`
- `/api/v1/agent-docs/storyboard.md`
