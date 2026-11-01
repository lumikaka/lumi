# 批量设定图：宿主生成，Lumi 裁切

用户要求批量生成设定图，或为多个独立主体制作初版设定时使用。Codex 负责选主体、调用宿主 imagegen 和视觉定位；Lumi 负责保存原图、确定性裁切、设定版本及来源关系。无需服务商，也不启动 Lumi 内部生成 pipeline。

## 1. 规划本批

读取已保存故事和 `settings list/get`，整理本批 1–6 个重要角色、独立道具、边界清楚的地点缩景或参考主体。超过 6 个就分多张总览图；复杂场景、整幅环境图和单项细节精修采用单张流程。总览图各主体共享输出分辨率，细节不足的关键角色可在拆分后单独精修。

不需要先创建占位设定。已有同一主体的设定时记录其 UUID、revision 和描述，拆分时明确更新它；不要仅按同名猜测身份。不要因为同名冲突就删除已有设定。

## 2. 渲染并生成总览图

从 `prompt_catalog` 确认 `premise/setting_image`，调用 `render_prompt`：

- `prompt_group=premise, prompt_key=setting_image, target=setting_board`。
- `variables.input_text` 放本批主体的完整描述及视觉身份要求（最多 12000 字符），不把整本所有设定都塞进去。
- 按需用 `context_chapter_uuids/context_setting_uuids` 限定上下文。

把返回的 **`generation_prompt`** 交给宿主 imagegen；`prompt` 中另附的工具写回信息不属于图像内容。模板负责白底、不可见 3×2 网格和留白，目标比例为 3:2。若要沿用已有主体或画风，按主 skill 的方式 `read_image` 实际读入相关参考后生成。

这里无需 `image_target`，它服务于单项图片槽位；本批总览图的目标由 `render_prompt` 给出。

## 3. 立即导入总览图

宿主返回真实图片路径后，立即 `setting_boards(action=import)`：

- `path` 用本次工具返回的真实绝对路径，不猜名称、不扫描目录。
- `source_text/style_snapshot/expected_revision` 使用生成前 `write_back` 返回的值。
- `prompt` 保存本次实际提交给 imagegen 的完整提示词（最多 12000 字符）；需要调整提示词时保持本批主体及画风一致。
- `idempotency_key` 使用本次导入的稳定唯一键，并保留完整请求以便中断后重试。

导入保留完整原始画布，返回 `uuid`（board_uuid）、`revision`、`source`、`setting_image.asset`。已有当前总览图选择会保留；此批操作始终通过 board_uuid 定位自己的原图。

## 4. 看图并提交拆分

1. `read_image(asset_uuid=setting_image.asset.uuid)` 实际查看已保存总览图。不要只凭生成提示词推测框，也不要因为提示词要求网格就机械等分。
2. `render_prompt(prompt_group=premise, prompt_key=asset_breakdown, target=setting_breakdown, board_uuid=...)`。工具自动从该批次提供真实尺寸、冻结的源描述和画风；无需伪造 `image_info_json`。
3. 按模板输出 `assets`：类型、名称、描述、1–3 个标签、confidence 及 `crop_box`。坐标以完整原图为基准归一化到 0–1，包含主体及安全边距，框之间不重叠；只保留置信度至少 0.92 的独立主体。无法可靠识别的项单独补图，不为了凑数量裁断主体。
4. 对更新的已有主体，补上 `setting_uuid` 和刚读取的该设定 `expected_revision`；新主体省略两项。
5. 调用 `apply_setting_breakdown`，传 `board_uuid`、渲染返回的批次 `expected_revision`、`expected_premise_revision`、本次操作的稳定 `idempotency_key` 和 `assets`。全部设定和图片版本一次提交；更新会生成并选中新版本，不修改历史图片。

裁切由 Lumi 在原图像素上执行；不要用 imagegen 重新生成各裁切结果，也不要直接访问项目资产目录或 SQLite。矩形裁切没有自动补边、去背景或语义修正。

## 5. 审阅、修正和恢复

提交后用 `review_settings` 展示，用 `settings get/read_image` 检查裁切是否完整、是否混入邻近主体。修正裁切时沿用原 board_uuid，重读批次、设定集合及目标设定 revision，使用新幂等键，并显式指定要更新的 setting_uuid；只重做需要修正的项。需要重新设计主体时走单项生图。

- 导入或拆分响应中断：先 `setting_boards list/get`；保存了原请求就按**原键、原参数**重放。已成功的请求返回原结果，不重复创建设定或版本。
- `latest_breakdown` 为最后一次成功提交的 UUID 清单，可据此读取各项；当前图片可能被后续操作更改，以 `settings get` 为准。
- 已有总览图但未拆分：从看图继续，不重新生图。
- 版本冲突：重读事实，检查生成内容或裁切计划是否仍适用；确认适用后使用最新版本和新幂等键提交，不盲换 revision 覆盖用户修改。
- 生图文件已失效且导入未成功：说明缺失后重新生成。若原导入请求已成功，重放不需要再次读取宿主文件。

按用户已有授权连续完成各批；审阅停顿仍遵循主 skill 与用户约定，不额外增加逐批确认。
