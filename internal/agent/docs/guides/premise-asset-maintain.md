# Guide：维护 Premise Asset

能力 ID：`premise_asset_maintain`

本 Guide 用于更新已有 Premise Asset 的元数据或图片、从 Reference 派生新设定项，以及在用户明确要求时软删除。API 字段以 `/api/v1/agent-docs/api/premise-asset.md` 为准。

## 前置条件

- 已知目标 `premise_asset_uuid`；Reference 只提供上下文，写入目标始终由 API 路径中的 UUID 显式指定。
- 每次写操作前都必须重新 GET 目标，读取 `uuid`、元数据、`current_variant` 和最新 `revision`。
- 图片替换可使用当前 Thread 尚未消费的通用 `image_gen` 输出；如果需要参考现有资源，由 Agent 从当前 Turn References 中显式选择。
- 删除仅在用户明确要求时执行；意图不明确时单独调用 `request_user_input`。

## 标准步骤

1. 用 `GET /api/v1/projects/{project_uuid}/premise-assets/{premise_asset_uuid}` 读取最新事实和 `revision`。
2. 根据目标选择操作：
   - 元数据更新：PATCH 仅提交需要改变的 `asset_type`、`title`、`summary` 或 `tags`，并提交最新 `expected_revision`。
   - 图片替换：调用 `image_gen`，用 `reference_uuids` 显式选择零到四个当前 Turn Reference。将本次调用新返回的、尚未消费的 `file_uuid` 放入 PATCH，并提交最新 `expected_revision`。
   - 派生设定项：调用 `image_gen` 生成新图片，再按创建流程 POST 新资产；原资产的图片 UUID 只能用于参考，不能作为 POST 的 `file_uuid`。
   - 软删除：先读取最新 revision，再 DELETE 并提交 `expected_revision`。按 `request_api` 返回的确认信息，单独调用 `request_user_input`；确认后重发完全相同的请求。
3. 对写入响应使用窄化 `response_filter`，并以成功信封中的最新资源为准。

## 禁止捷径

- 不得使用上下文中缓存的 revision 写入，也不得在 conflict 后盲目重试。
- 不得把现有资产图片的 UUID 直接写入 PATCH/POST；图片写回只接受当前 Thread 尚未消费的 `image_gen` 新输出。
- 不得从 Reference 推断写入目标，也不得把软删除描述为永久删除。
- 不得在用户只要求读取、解释或生成建议时主动修改或删除资源。

## 失败恢复

- revision conflict：重新 GET，比较用户目标与最新事实，再决定是否基于新 revision 重试。
- `production_validation_failed`：确认 PATCH 目标 UUID 正确，并重新调用 `image_gen` 获取当前 Thread 中用途匹配的新文件。
- `production_state_conflict`：图片已被消费；重新生成，不能复用该 `file_uuid`。
- 危险操作确认过期或指纹不匹配：重新 GET、重新发起 DELETE 获取新的确认信息，不能拼接或复用旧确认。

## 对应 API Contract

- `/api/v1/agent-docs/api/premise-asset.md`
- `/api/v1/agent-docs/api/premise.md`
