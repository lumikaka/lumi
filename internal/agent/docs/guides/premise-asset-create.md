# Guide：创建 Premise Asset

能力 ID：`premise_asset_create`

本 Guide 用于从文本、当前 Turn Reference 或 ready upload 创建一个新的 Premise Asset。API 字段以 `/api/v1/agent-docs/api/premise-asset.md` 和 `/api/v1/agent-docs/api/premise.md` 为准。

## 前置条件

- 已知当前 `project_uuid`、`asset_type`、标题，以及足以生成或导入图片的用户意图。
- 需要遵循项目画风时，先读取当前 Premise 的 `default_style`；需要避免重复时，读取已有设定项列表。
- 图片来源必须在以下来源中选择一种：纯文本生成、当前 Turn 中 image-capable Reference 引导生成、ready upload 直接导入。
- 信息不足且会实质改变结果时，单独调用 `request_user_input`。

## 标准步骤

1. 用 `request_api` 读取必要的 Premise 和已有设定项事实，并使用窄化 `response_filter`。
2. 根据图片来源处理：
   - 纯文本：调用 `image_gen`，传 `reference_uuids: []`；Prompt 包含完整用户要求和必要的 `default_style`。
   - 当前 Turn Reference：从当前 Turn 的 Reference 中显式选择零到四个有图片的 `resource_uuid`，按希望的图片输入顺序传入 `image_gen.reference_uuids`。不要选择历史 Turn、未知或没有图片的 Reference。
   - ready upload：不调用 `image_gen`，直接使用尚未消费的 `upload_uuid`。
3. 新建设定项图片默认要求 `image_gen.size` 为 `512x512`。除非用户明确要求改变，否则保持整体画风；默认生成纯白无纹理背景、单一完整主体、居中且留安全边距，不加入文字、边框、拼贴、多视图或无关对象。
4. 调用 `POST /api/v1/projects/{project_uuid}/premise-assets`。`file_uuid` 与 `upload_uuid` 必须且只能提供一个：
   - `file_uuid` 必须是当前 Thread 刚刚由 `image_gen` 返回、用途为通用 Chat 图片且尚未消费的文件 UUID。
   - `upload_uuid` 必须是当前项目中 ready 且尚未消费的上传 UUID。
5. 只有 POST 返回成功信封后才报告创建完成。

## 禁止捷径

- 已有设定项或图片必须先作为当前 Turn Reference，再由 `image_gen.reference_uuids` 选择；其现有 File UUID 不能直接作为 POST 的 `file_uuid`。
- 当前消息 Reference 或任意已有项目文件也不能直接作为 POST 的 `file_uuid`；要么先经 `image_gen` 产生当前 Thread 的新输出，要么使用 ready `upload_uuid`。
- 不得虚构 UUID、跳过实际图片生成、把 `image_gen` 成功等同于设定项创建成功，或同时提交 `file_uuid` 与 `upload_uuid`。

## 失败恢复

- `production_resource_not_found`：确认文件或上传确实属于当前项目且仍存在；不要把旧资源 UUID 当成生成结果。
- `production_validation_failed`：来源、会话或用途不匹配。若使用已有项目图，请让它成为当前 Turn Reference 后重新调用 `image_gen`，再提交新返回的 `file_uuid`。
- `production_state_conflict`：文件或上传已被消费；重新生成图片或重新上传，不要重用旧 UUID。
- POST 失败时不得声称资产已创建；修正来源后重试，避免重复生成无关图片。

## 对应 API Contract

- `/api/v1/agent-docs/api/premise.md`
- `/api/v1/agent-docs/api/premise-asset.md`
