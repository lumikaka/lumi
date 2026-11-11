<!-- host:codex -->
# 批量设定图：宿主生成，Lumi 裁切

首次创作需要页面生图、且多个必要主体缺少可用设定图时，默认使用本流程；用户明确要求批量设定图时也使用。正文“只生成第一页”不取消这一步，设定图不计入正文图数量。用户明确要求纯文字、跳过设定图或限制全部生图次数时遵循其范围。Codex 负责选主体、调用宿主 imagegen 和视觉定位；Lumi 负责保存原图、确定性裁切、设定版本及来源关系。无需服务商，也不启动 Lumi 内部生成 pipeline。

## 1. 规划本批

读取已保存故事和 `settings list/get`，整理本批 1–6 个重要角色、独立道具、边界清楚的地点缩景或参考主体。超过 6 个就分多张总览图；复杂场景、整幅环境图和单项细节精修采用单张流程。总览图各主体共享输出分辨率，细节不足的关键角色可在拆分后单独精修。

优先处理本次章节／目标页面会用到的视觉主体；已有可用 `current_variant.asset` 且符合本次参考要求的项直接复用。用户附带参考图时先按 [设定图的用户参考](setting-references.md) 区分用途、提取画风并检查已有设定；画风不符的项保留身份重绘，不能仅凭有图跳过。来源说明、版权备注和其他纯文字记录不纳入总览图。若页面已存在，可用 `image_target` 的 `setting_readiness` 查看候选和缺图情况，再传本页实际需要的 `setting_uuids` 检查，不能因其他章节资产缺图而重画整个项目。

不需要先创建占位设定。已有同一主体的设定时记录其 UUID、revision 和描述，拆分时明确更新它；不要仅按同名猜测身份。不要因为同名冲突就删除已有设定。

## 2. 渲染并生成总览图

从 `prompt_catalog` 确认 `premise/setting_image`，调用 `render_prompt`：

- `prompt_group=premise, prompt_key=setting_image, target=setting_board`。
- `variables.input_text` 放本批主体的完整描述及视觉身份要求（最多 12000 字符）；有用户参考时补上提取的具体视觉要求和各图用途对应关系，不把整本所有设定都塞进去。
- 按需用 `context_chapter_uuids/context_setting_uuids` 限定上下文。

把返回的 **`generation_prompt`** 交给宿主 imagegen；`prompt` 中另附的工具写回信息不属于图像内容。模板负责白底、不可见 3×2 网格和留白，目标比例为 3:2。若要沿用已有主体或画风，按主 skill 的方式 `read_image` 实际读入相关参考。用户附带本次相关视觉参考时，必须按 [显式传图规则](setting-references.md#3-显式传给宿主-imagegen) 使用 `referenced_image_paths` 或 `num_last_images_to_include` 携带原图；每批都检查，不能只传 `generation_prompt` 或以 `context_setting_uuids` 代替传图。

这里无需 `image_target`，它服务于单项图片槽位；本批总览图的目标由 `render_prompt` 给出。

## 3. 立即导入总览图

宿主返回真实图片路径后，立即 `setting_boards(action=import)`：

- `path` 用本次工具返回的真实绝对路径，不猜名称、不扫描目录。
- `source_text/style_snapshot/expected_revision` 使用生成前 `write_back` 返回的值。
- `prompt` 保存本次实际提交给 imagegen 的完整提示词（最多 12000 字符）；需要调整提示词时保持本批主体及画风一致。
- `idempotency_key` 使用本次导入的稳定唯一键，并保留完整请求以便中断后重试。

导入保留完整原始画布，返回 `uuid`（board_uuid）、`revision`、`source`、`setting_image.asset`。已有当前总览图选择会保留；此批操作始终通过 board_uuid 定位自己的原图。

成功回显只取后续识图要用的字段。假设 `board` 为导入成功后的 `data`：

```js
text({ board_uuid: board.uuid, revision: board.revision, asset_uuid: board.setting_image.asset.uuid });
```

## 4. 看图并提交拆分

1. `read_image(asset_uuid=setting_image.asset.uuid)` 实际查看已保存总览图。不要只凭生成提示词推测框，也不要因为提示词要求网格就机械等分。
2. `render_prompt(prompt_group=premise, prompt_key=asset_breakdown, target=setting_breakdown, board_uuid=...)`。工具自动从该批次提供真实尺寸、冻结的源描述和画风；无需伪造 `image_info_json`。
3. 按模板输出 `assets`：类型、名称、描述、1–3 个标签、confidence 及 `crop_box`。坐标以完整原图为基准归一化到 0–1，包含主体及安全边距，框之间不重叠；只保留置信度至少 0.92 的独立主体。无法可靠识别的项单独补图，不为了凑数量裁断主体。
4. 对更新的已有主体，补上 `setting_uuid` 和刚读取的该设定 `expected_revision`；新主体省略两项。
5. 调用 `apply_setting_breakdown`，传 `board_uuid`、渲染返回的批次 `expected_revision`、`expected_premise_revision`、本次操作的稳定 `idempotency_key` 和 `assets`。全部设定和图片版本一次提交；更新会生成并选中新版本，不修改历史图片。

裁切由 Lumi 在原图像素上执行；不要用 imagegen 重新生成各裁切结果，也不要直接访问项目资产目录或 SQLite。矩形裁切没有自动补边、去背景或语义修正。

执行层保留完整拆分请求和返回的版本映射。成功时主线程回显批次 UUID、写入项数及需复核的主体；失败时回显原错误详情。随后实际查看裁切结果，记录具体问题。

## 5. 审阅、修正和恢复

提交后用 `review_settings` 展示，用 `settings get/read_image` 检查裁切是否完整、是否混入邻近主体；有用户视觉参考时，同时按提取的特征核对画风和身份，符合后再用于正文。修正裁切时沿用原 board_uuid，重读批次、设定集合及目标设定 revision，使用新幂等键，并显式指定要更新的 setting_uuid；只重做需要修正的项。需要重新设计主体或修正画风时，携带相关用户原图走单项生图。

完成必要设定图后继续分页与画面描述。目标页面生图前，带所需 `setting_uuids` 再调用 `image_target`；确认缺图已补齐，调用 `prepare_page_image` 现拼本页合图；首次或不确定是否已看时，宿主查看该摘要与路径对应的合图。每次生图都用 `referenced_image_paths` 仅传这一个路径。页面生图和导入使用同一份服务端完整提示词及准备摘要，见 [页面生图](page-images.md)。总览图已导入但未完成拆分，不等于各设定已有可复用图片。

- 导入或拆分响应中断：先 `setting_boards list/get`；保存了原请求就按**原键、原参数**重放。已成功的请求返回原结果，不重复创建设定或版本。
- `latest_breakdown` 为最后一次成功提交的 UUID 清单，可据此读取各项；当前图片可能被后续操作更改，以 `settings get` 为准。
- 已有总览图但未拆分：从看图继续，不重新生图。
- 版本冲突：重读事实，检查生成内容或裁切计划是否仍适用；确认适用后使用最新版本和新幂等键提交，不盲换 revision 覆盖用户修改。
- 生图文件已失效且导入未成功：说明缺失后重新生成。若原导入请求已成功，重放不需要再次读取宿主文件。

按用户已有授权连续完成各批；审阅停顿仍遵循主 skill 与用户约定，不额外增加逐批确认。

设定板导入后、拆分前按 [设定图核对](setting-references.md#4-核对后再用于正文) 对照用户要求及原参考确认画风；拆分后检查裁切完整性。图片存在不代表通过核对，进入正文须提交当前版本对应的 `setting_reviews`。
<!-- host:claude -->
# 设定图来源与降级（Claude Code）

Claude Code 没有原生生图工具，Lumi 也没有为批量设定总览图或单项设定图提供服务端生图工具。**不得假装批量设定图可用**，也不能把只有文字的设定当作图片就绪。首次创作需要页面生图时，按下面的顺序为必要主体准备设定图，并在交付中说明采用了哪种方式。

## 1. 规划

读取已保存故事和 `settings list/get`，列出本次章节／目标页面会用到的角色、独立道具和场景。已有可用 `current_variant.asset` 且符合本次参考要求的项直接复用；画风不符的项保留身份，按下文重新获取图片并更新同一 setting_uuid。来源说明、版权备注等纯文字记录不需要图片。用户附带参考图时先按 [设定图的用户参考](setting-references.md) 区分用途、提取画风。

## 2. 选择降级方式

1. **用户提供图片**：用户给出真实本地图片文件时，先按 [设定图来源](setting-references.md) 看图确认；仅缺本地读图工具时沿用其中已授权导入后 `read_image` 核对的分支，未看不得用于正文。用 `image_target(target_uuid, slot=setting_image)` 取 revision，`import_image` 导入。一张图含多个主体时，可用 `setting_boards import` 保存原图，再实际看图并 `apply_setting_breakdown` 裁切入库（每批 1–6 项，`crop_box` 为归一化坐标，不能重叠，`confidence` 至少 0.92；参数与幂等规则见 [MCP 调用参考](mcp-usage.md#批量设定图)）。
2. **在 Lumi 中生成**：给出 `web_url(view=settings)` 或对应 `web_url(setting_uuid)`，说明本批主体与参考要求，请用户在 Lumi 桌面端或网页的设定页面用已配置的服务商生成（可在其中生成批量总览图）。用户回到对话后 `settings list/get` 重读，实际 `read_image` 查看后再继续。
3. **仅文字设定**：只有用户明确接受时才跳过设定图；正文传 `setting_uuids=[]` 并保留理由。

不得用其他宿主的生图参数、脚本或第三方 API 自行补图，不切换付费渠道，不直接访问项目资产目录或 SQLite。

## 3. 核对与续做

设定图入库后按 [设定图核对](setting-references.md#4-核对后再用于正文) 实际看图并记录观察，再进入页面生图。用户在 Lumi 中生成或修改后，先重读设定与最新 revision；revision 冲突时重读，不覆盖网页中的修改。中断后用 `setting_boards list/get` 检查已保存的总览图与 `latest_breakdown`，已保存的图片无需重新获取。
<!-- /host -->
