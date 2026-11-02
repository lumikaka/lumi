# 页面生图提示词

适用于正文、封面、封底的宿主生图。Lumi 的 `chapter/section_image` 组合项目当前生效的基础规则、画风、参考图说明和已保存分镜；Codex 负责把渲染结果交给宿主生图并导入。不要仅凭 `comic_storyboard` 的输出自行重写生图提示词，也不要把条漫默认简化为固定三格、四格或 Q 版画风。

## 读取页面和规则

先 `pages get` 读取实际 `chapter_uuid`、`page_uuid`、`page_role`、完整分镜及 revision，再 `image_target(target_uuid=page_uuid, slot=page_image)` 查询比例。画面描述缺失时先保存分镜；需要按用户要求修改内容时，先更新页面再重新渲染。

检查返回的 `setting_readiness`：

| status | 含义与下一步 |
| --- | --- |
| `selection_required` | 当前为项目候选，不是本页依赖。按分镜选择实际需要的 `setting_uuids` 再查；没有候选时先从故事提取视觉设定。 |
| `missing_images` | 已选设定中有缺图项。先按 [批量设定图](batch-settings.md) 或单项流程补齐，再重查；仅有文字描述不算完成。 |
| `ready` | 已选设定都有当前图片。按 `items[].asset_uuid` 实际 `read_image`，这不代表图片已传给宿主。 |
| `no_settings_selected` | 显式选择了空列表，不能据此宣称已准备好。核对页面是否确实无需可复用视觉设定。 |

`items` 包含 `setting_uuid/title/asset_type/expected_revision/has_image`，有图时附 `asset_uuid`；`available_count/missing_count` 只统计当前 scope。省略 `setting_uuids` 时 `scope=project_candidates`，纯文字 `reference` 备注不列入候选；显式选择时 `scope=selected_settings`，按所选 UUID 检查，不受其他章节缺图影响。准备结果不持久化选图，也不修改页面或设定。

只有用户明确要求跳过设定图／直接使用文字参考，或明确的全部生图次数限制需要如此时，才能在缺图情况下继续。记录实际用户要求，准确使用无参考图说明，不通过空选择伪装为 `ready`；“正文只生成第一页”不是跳过理由。

读取 `prompt_catalog(prompt_group=chapter)` 的模板和 placeholders。以下 `render_prompt` 均使用 `target=page`、同一组实际 `chapter_uuid/page_uuid`，不能用 `target=pages`，也不能只传 `target_uuid`。

按页面角色选择基础片段，调用 `render_prompt`，保存返回的 **`data.fragment_prompt`**：

| page_role | prompt_group / prompt_key |
| --- | --- |
| `body` | `chapter/before_image` |
| `front_cover` | `chapter/cover_before_image` |
| `back_cover` | `chapter/back_cover_before_image` |

模板通过 Lumi 读取项目自定义版本和绘本规格；不要复制仓库默认文字或凭记忆补规则。用户明确的画风要求继续遵循，但不要为了采用自己偏好的画风而丢掉模板中的阅读动线、对白保真和构图约束。

## 准确描述实际参考图

- 没有向生图工具提供参考图片时，渲染 `chapter/section_reference_absent`，保存其 `fragment_prompt`。只有文字设定、图片 UUID 或网页链接不代表已提供图片。
- 实际提供一张带标签的设定拼贴图时，渲染 `chapter/section_reference_present`，通过 `variables.reference_titles` 填写该图中本页涉及的设定项，保存其 `fragment_prompt`。
- 宿主使用多张独立设定图、上一页或外部角色参考图时，仍读取 `section_reference_present` 的有效片段；仅将其中关于“一张拼贴图／标签”的交付描述改为真实的图片数量、顺序和用途，保留身份一致性要求。不要声称提供了不存在的拼贴图，也不要因此重写基础规则或整份提示词。

项目图片通过 `read_image(asset_uuid)` 实际放入当前对话；外部参考遵守宿主读图规则。临近生图时核对最近图片确实是本页所需参考，再选择宿主参考图参数；压缩或跨会话后重新读图。

## 组合并使用完整提示词

调用 `render_prompt`：

| 参数 | 值 |
| --- | --- |
| `prompt_group` | `chapter` |
| `prompt_key` | `section_image` |
| `target` | `page` |
| `chapter_uuid` / `page_uuid` | 刚读取的实际 UUID |
| `variables.before_image_prompt` | 上述基础片段的完整 `fragment_prompt` |
| `variables.reference_usage_text` | 上述与实际参考图一致的说明 |

`section_id`、`storyboard`、`style_prompt` 由服务端读取当前页面和项目，不在 `variables` 中伪造或覆盖。目录若有项目自定义模板的其他占位符，按实际任务补齐。

返回值各有用途：

- **`generation_prompt`**：传给宿主生图的完整视觉提示词。保留所有基础规则、画风、参考说明和分镜；不翻译、摘要或另写一份替代。
- **`image_write_back`**：导入用的 `target_uuid`、`slot=page_image` 和 `expected_revision`。核对其目标及版本与本次 `image_target` 一致；不一致时重读页面并重新渲染，不能只换 revision。
- `prompt` / `write_back`：外层任务上下文和文本写回说明。不要把其中的 JSON 输出要求或写回指令传给图片模型，也不要因此调用 `pages update`。

可在完整 `generation_prompt` 末尾追加本次宿主的比例、安全边距和参考图传递说明，不能覆盖原有规则。明确项目 `target_ratio`；`recommended_ratio` 仅是宿主兼容建议。需要兼容输出时保留完整构图和文字，再按已约定的 `keep/crop/pad` 导入，不因建议比例擅自改变条漫阅读结构。

最终提示词最多 12000 字符，生图和导入保存同一完整字符串。超限时先精简已保存分镜中的冗余描述或缩小任务，再重新渲染，不直接截断基础规则或对白。若缺少 `fragment_prompt`、`generation_prompt` 或 `image_write_back`，先核对并更新实际 CLI；本页暂不生图，不从外层任务文字猜边界或自行编造模板。

生成完成后立即 `import_image`：使用宿主本次返回的真实路径、`image_write_back` 的目标和版本，以及实际发送的完整提示词。继续遵循主技能中的导入、比例检查和审阅流程。
