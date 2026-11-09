# 页面生图：服务端合图与生成

正文、封面、封底统一使用 `generate_page_image`。Lumi 复用 Web 与 `prepare_page_image` 的合图和完整提示词组装，由用户在 Lumi 中配置的图片服务商生成，并在完成后把图片挂载为页面当前版本。Claude Code 负责选择与核对设定、发起生成、用 `read_image` 看成图并记录核对；不编写生图脚本，不向用户索取 API key。

## 1. 明确本页设定

用 `pages get` 读取实际 `page_uuid/page_role`、完整分镜和对白；没有分镜时先保存。用 `image_target(target_uuid=page_uuid, slot=page_image)` 查看项目候选和画布比例，按本页分镜选择实际需要的设定。

- `selection_required`：候选清单尚不是本页依赖。明确选择 `setting_uuids`。
- `missing_images`：先按 [批量设定图](batch-settings.md) 或单项流程补齐。仅有文字、已导入但尚未拆分的总览图都不算设定图就绪。
- `review_required`：图片已齐全，先按 [设定图核对](setting-references.md#4-核对后再用于正文) 对照用户要求／原参考，记录具体观察。核对阶段用 `read_image` 查看设定；需要看本页参考合图时，用 `prepare_page_image` 返回的路径 Read。
- `no_settings_selected`：核对是否确实无需视觉参考。明确无参考图时传 `[]`，执行真实无参考流程。

正文只生成第一页不取消必要设定图。用户明确要求跳过设定图或限制全部生图次数时，遵循其要求，选择 `[]` 并保留其理由；不能把缺图选择静默丢弃或伪报为 ready。

如果前页已导入相同设定，向 `image_target` 同时提供所选 `setting_uuids` 和 `review_source_page_uuid`。返回的 `items[].previous_review` 已匹配当前图片版本、设定 revision 和画风。逐项确认原核对依据仍覆盖本次原图和用户要求后，可将其直接用作 setting_reviews；依据改变、记录缺失或不匹配时重新看图。

## 2. 准备本次生图

`generate_page_image` 与 `prepare_page_image` 使用相同的选择与核对输入：必填 `page_uuid, setting_uuids`；非空选择还必须提供逐项通过的 `setting_reviews`，可选 `additional_direction`（用户本次补充要求）。`generate_page_image` 另需页面当前 `expected_revision` 和本次唯一的 `idempotency_key`，可选 `model`（服务商中的图片模型，省略时用 Lumi 默认）。`setting_uuids` 最多 12 项、不得重复，顺序就是合图编号。Lumi 读取当前采用的图片版本，每次用当前设定合成 PNG。缺图、核对未通过／过期、失效或跨项目引用会明确失败。

`setting_reviews` 每项为 `{setting_uuid, variant_uuid, expected_revision, style_digest, verdict, reference_basis, comparison}`。身份和摘要来自本次 `image_target.setting_readiness`；`verdict` 为 `passed` 或 `needs_revision`，后者阻止准备。`reference_basis/comparison` 各最多 2000 字符且非空，记录用户要求和具体视觉对照。只提交本页所选设定的核对。重绘、切换图片版本、修改设定或全局画风后须重新看图核对；不能仅替换字段。无参考模式 `[]` 可省略核对数组。

需要预览时，`prepare_page_image` 返回下表字段；`generate_page_image` 在服务端使用同一组装结果，不需要传回这些字段。

| 返回字段 | 使用方式 |
| --- | --- |
| `reference_mode` | `collage` 或 `none` |
| `reference_image_path` | 真实可读的临时绝对路径；none 时为空字符串 |
| `reference_image_sha256` | 最终合图 PNG 的 SHA-256；none 模式省略 |
| `sources` | 有序的 setting_uuid、名称、编号标签、revision、实际 variant_uuid/version_no/file_uuid 和图片 SHA-256 |
| `image_info` | 合图尺寸、PNG 类型与合成器版本；none 时为 null |
| `generation_prompt` | 服务端组装的完整提示词，最多 12000 字符；`generate_page_image` 原样使用，无需传回 |
| `target` | 项目比例、建议比例、留白及裁切建议 |
| `image_write_back` | 导入目标 target_uuid、slot 和 expected_revision |
| `preparation` | 原样保留的准备凭据，含选择、设定核对记录、补充要求、revision、digest 和 signature |

服务端按 `page_role` 解析正文／封面／封底有效基础规则，使用有效 `project_overall_style`（含 Web 的旧数据回退），组装语言指令、实际参考标签、已保存分镜及用户补充要求。不要自行渲染或手改参考说明，不在返回后追加比例、画风或安全边距文字。需要补充时放入 `additional_direction` 后重新准备；需要改分镜时先更新页面。保留项目规格、对白和原有美术规则。

竖向条漫的构图以本页已保存的分镜为准。默认省略 `additional_direction`；仅在有本次用户补充或本页具体纠错时填写，避免批量追加“三段顺序”“上中下等分”等布局要求。

用户要求优先。参考图有独特对白字体时，事先将具体字体特征存入全局画风与分镜，让本次完整提示词携带；合图中的无字设定不负责传递字体。首张正文必须核对画风、字形与文字准确性后，才开始余页生图；这是代理自检，无需另向用户索取许可。

## 3. 生成并等待

1. 需要确认参考时先 `prepare_page_image`，用 Read 查看 `reference_image_path`；同一 `reference_image_sha256` 已看过时跳过。合图和提示词在服务端重新组装，不复制、改写或追加 `generation_prompt`。
2. 调用 `generate_page_image`。服务端在页面租约内重新校验选择、核对记录与 revision，再创建页面生图任务；调用会等待一段时间。
3. 返回 `done:true` 时，`image` 含 `variant_uuid/version_no/asset_uuid`，`revision` 是挂载后的页面 revision，`is_current_image` 表示该图是否为当前图片。
4. 返回 `done:false` 时任务仍在进行：用**完全相同的参数和 idempotency_key** 再次调用继续等待。同一 key 不会重复建任务或重复计费；换新 key 会重新生成。

```json
{"name":"generate_page_image","arguments":{"page_uuid":"01989abc-def0-7000-8000-000000000002","setting_uuids":[],"expected_revision":3,"idempotency_key":"page-02-first"}}
```

不读取 `project.sqlite`、`.lumi` 或 `assets`，不扫描目录猜文件，不用 Bash 复制生成结果；页面图片由服务端直接挂载，无需 `import_image`。

## 4. 失败与冲突

- `image_provider_unavailable`：尚未计费。用 `web_url(view=providers)` 给出链接，请用户在 Lumi 中配置图片服务商；配置后用同一 key 重试。
- `conflict` 或 revision 不一致：页面已被修改（可能在 Lumi Web 中）。重新 `image_target`／`pages get` 读取最新内容与 revision，必要时重新核对设定，再用新 key 生图；不要只替换 revision，不覆盖网页修改。
- `setting_image_review_required`／`setting_image_review_stale`：补齐真实核对，或按当前设定与画风重新核对。
- `idempotency_key_conflict`：该 key 已用于其他页面，换新 key。
- 任务失败、取消或中断：按 `error_code` 与 details 处理（服务商额度、限流或配置）；页面图片未改变。修正原因后用新 key 重新生成，不反复重试。
- `host_not_supported`：当前连接不是 Claude Code 插件；`cli_upgrade_required`：先停止旧后台并用同版本 CLI 重启后新开会话。

## 5. 逐页核对与恢复

生成完成后用返回的 `read_image.asset_uuid` 调 `read_image` 实际看本次成图，对照已保存分镜、用户原文与设定，逐个核对姓名、气泡文字及说话人，再看人物肢体、道具位置和前后页连续性。需要时放大看字。看不清的字或无法确认的细节应记录为疑点。

调用 `pages(action="record_image_review", chapter_uuid, page_uuid, variant_uuid, expected_revision, verdict, observation)`。版本和 revision 取自导入结果或当前摘要。verdict 选 `passed` 或 `needs_revision`，observation 写明具体位置、文字或画面观察。只修问题页，修正版要重新核对；版本冲突先重读。核对记录属于当前图片及页面 revision。修订同一版本的观察时，从摘要取 review_revision，作为 expected_review_revision 提交。

续做或导出前用 `book_outline(chapter_uuid)` 分页统计 has_image 和 review_status。missing_image 才需要首次生图，pending 先检查已有成图，needs_revision 的完整观察通过 `pages get(summary_only=true)` 读取。报告已生成、通过核对、待修或未核对的页数；若有疑点，指明页码。
