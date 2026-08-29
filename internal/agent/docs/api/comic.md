# Comic API

文中“获得确认”均指：先提交参数完整的 `request_api`；若返回 `agent_tool_confirmation_required`，该次不会执行写操作，此时再按 Overview 的全局协议把 confirmation 只传给 `request_user_input`；确认后由运行时自动执行原请求。

使用 `request_api` 调用。将占位符替换为公开 UUIDv7；修改前先读取目标 Section 的最新 `revision`，冲突后重新读取。

## 状态与页面

`page_role` 只支持 `front_cover`、`body` 和 `back_cover`。普通绘本分别称“封面”、“正文页”和“封底”；`vertical_strip` 只允许 `body`，对用户称“画面段落”。`section_no` 是包含封面和封底的绝对装订顺序，不是正文页码。

- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic`
  - 使用 `.data | {uuid,chapter_uuid,status,has_premise_assets,premise_asset_count,revision,updated_at}`。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections`
  - 使用 `.data.items[] | {uuid,chapter_uuid,section_no,page_role,title,description_md,revision}`。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections`
  - `request_body`：`title` 必填；`description_md`、`storyboard_md`、`page_role` 可选，省略 `page_role` 时默认为 `body`。
  - 普通绘本的空页面序列必须先创建 `body`；已有至少一个 active `body` 后，才可创建 `front_cover` 或 `back_cover`。条漫始终只能创建 `body`。
  - 正文页示例：`{"title":"雨夜相遇","description_md":"场景与动作说明","storyboard_md":"完整分镜 Markdown","page_role":"body"}`。
  - 已有正文页后的封面示例：`{"title":"封面","storyboard_md":"完整封面页面脚本","page_role":"front_cover"}`。
- `PATCH /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}`
  - `request_body`：`expected_revision` 必填；按需传 `title`、`description_md`、`page_role`。
  - 示例：`{"title":"新标题","page_role":"back_cover","expected_revision":3}`。
- `PUT /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-section-order`
  - 会整体重排正文页，调用前必须获得确认。
  - `request_body`：`{"section_uuids":["<section_uuid_1>","<section_uuid_2>"]}`；Agent 应提交排序后的完整 active `body` UUID 列表，数量为 1–200。
  - Agent 请求只携带当前全部 active `body` 的公开 UUIDv7，不携带封面或封底；服务端自动保持封面在首位、封底在末位。
- `DELETE /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}`
  - 调用前必须获得确认。
  - `request_body`：`{"expected_revision":3}`。

单个 Section 的完整读取见 `/api/v1/agent-docs/api/comic-section.md`。

每个绘本最多一个 active 封面和一个 active 封底。普通绘本的首个页面必须是 `body`；一旦已有正文页，不能删除最后一个 active `body`，也不能把它改成特殊页，因此不会留下“只有封面或封底”的序列。条漫只允许 `body`，但可删除最后一个画面段落回到 `empty`。创建或更新角色时由服务端检查特殊页唯一性并归一化首尾顺序；不要使用内部 `id` 或猜测 UUID。

## 页面图片版本

- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/images`
  - 导入当前项目尚未消费的 ready upload。
  - `request_body`：`{"upload_uuid":"<upload_uuid>","expected_revision":3}`。
- `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-variants`
  - 使用 `.data.items[] | {uuid,version_no,source_type,generation_uuid,asset,created_at}`。
- `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-variants/{variant_uuid}/selections`
  - `request_body`：`{"expected_revision":3}`。

图片生成任务会冻结目标 Section 的 `page_role`。任务运行期间不要修改该角色；若提交结果时角色已漂移，服务端会拒绝把结果写回错误的页面类型，应按任务错误重读 Section 后重新生成。

写入和选择操作返回更新后的 Section；使用 `.data | {uuid,chapter_uuid,section_no,page_role,title,current_storyboard,revision}`。
