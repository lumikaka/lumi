# 参考图编辑、候选选择与真实导出验收

2026-11-02：**本项已完成，无需重发下列指令或重复编辑/导出。**原任务已单次Core编辑、导入候选2、切回原图，PDF/ZIP各导出一次；父任务独立检查交付文件通过，见 [成功证据](step6-evidence/normal-task-reference-export-success-20261102.json)。下文保留当次执行指令，目标为正常任务「Lumi acceptance · A」`a0964e96-ba88-4c58-af32-4c0d356b7530`，不是执行回执或后续生命周期操作指令。

```text
用户已明确要求继续下一项：参考图编辑和候选选择验收，再做真实PDF/ZIP导出。请在本任务使用正常 lumi-dev / namespace=null 串行执行，不启动其它Agent。

已知作品：project_uuid=01a199d7-323a-7017-a5da-1d93303f487e，chapter_uuid=01a199e6-89e9-7213-a333-924bffa24af8，第1页page_uuid=01a19a9f-e071-7ce8-9e8b-175e4009c456。已通过原图 Asset=01a19ad5-745d-71c1-b6ff-998d15b3f8d6，variant=01a19ad5-745c-7ba0-99c8-6b751bb67458，图片version=1，page.revision=1，原件SHA256=be9e712ebc226eca2ded54b64a1608a170fb8d194345144118ea01cf21998f16。上述仅供核对，不作为本次写入revision。开发build_id=29401ca7cbd2cff3f21d5d2f7c3e2237f31990bfd99ac1e005dfd5faa068c1a8。

1. 重读当前手册及project/pages/prompts/media/export类别。project_overview、pages.get和image_target确认项目与当前选择，记录完整页面正文、分镜及其UUID/hash、chapter.revision、各页顺序。发现本项已有编辑候选或导出报告、未知写入或不同当前选择时先核对后停止，不重复提交。不得重存章节、页面文字、画风或setup，不创建其它页图片。

2. 必须通过本次正常 lumi-dev read_image 重新读取原图Asset，实际查看受控预览并取得reference_delivery.deposited=true及reference_media_url。编辑输入必须使用这次Lumi读回的受控预览，不使用旧Core生成地址替代，不调用resolve_local_path、不传私有缓存路径。寄存失败则停止图片编辑，不能声称模型已收到参考图。记录输入预览尺寸、类型及受管引用，明确它不是原图。

3. 从本次render_prompt重新组装section_image基础要求：chapter/before_image和section_reference_absent各取fragment_prompt，再将其作为before_image_prompt/reference_usage_text补入chapter/section_image，取generation_prompt和image_write_back。本次输入是第1页既有页面图，不是角色设定拼贴；不能用section_reference_present虚称已提供设定拼贴。以完整generation_prompt为业务要求，再追加以下针对原分镜差异的本次编辑指令，生图和导入保存完全相同的最终字符串，总长不超过12000字符：
“以本次提供的Lumi页面预览为编辑底图，保留橙色幼狐、钴蓝围巾、黄铜提灯、树洞门口、向右小路、夜色和水彩纸纹。对照上方已保存分镜，只将奶奶收回到门内的温和侧身轮廓及前爪，移除门内额外的背景灯；门内暖光可以保留。保留两行正文原文及清晰度，不新增文字、人物、提灯或分格。保持横版4:3，不做整页全新构图。上方项目画风中的‘无图内文字’为已有矛盾，本轮不修改项目配置；执行本页分镜中明确要求的两行正文。”

4. 提示词和输入均准备完成后，用本轮Core list_models筛选image.edit。使用目录中的精确model_id/provider_id，prepare capability=image.edit，严格按本轮Guide把上述受控预览作为实际图片输入传入body；不能只在prompt文字提到参考URL、不能改用image.generate冒充编辑、不擅自换源。只有Guide支持该受管输入时才request。仅提交一次付费编辑请求。pending时同一invocation_id按建议间隔poll；拒绝、取消、超时、SUBMISSION_OUTCOME_UNKNOWN或未知付费/写入结果立即停止，不自动重提。确定未产生副作用的输入校验错误可按schema修正。

5. Core编辑成功后立即用顶层ghost_call tool=receive_media、顶层attachments=[唯一实际编辑结果]交给lumi-dev，args使用本次image_write_back的target_uuid/slot/expected_revision、fit_mode=keep和完整编辑提示词。不从结果猜路径、指纹或字节，不调用探针或直接写SQLite。导入后pages.get、read_image核对新Asset/variant/version/revision，记录原/编辑两版不可变UUID。再次读取原Asset可确认原件仍可读；没有公开版本列表工具时以实际前后回执和成功选择为证，不调用未开放review工具或隐藏API。

6. 做一次候选选择写入验收：编辑导入已使编辑版成为当前图；pages.get读取最新page.revision，再pages action=select_image，variant_uuid使用步骤1实际读到的原版variant。随后get和read_image确认已切回原图、revision已推进，页面文字/分镜未改。最终保持原图为当前选择，编辑图作为第二候选保留，不删除任一版本。若select_image被拒、冲突或结果未知，读取事实后停止，不盲目覆盖或继续导出。此项是对话工具中的选择验收，不宣称Cindy四视图已实现或画面细节已全通过审稿。

7. 确认最终选择后，book_outline核对全书，再分别export_book format=pdf和format=zip，allow_missing_images=true，各调用一次。本次只有第1页有图，PDF只包含可用正文图片，不把它称为完整三页绘本；ZIP包含三页完整文本、分镜和现有已选页面图片。不要为补齐缺页追加付费生成。记录两个完整成功回执、uuid、relative path、byte_size、sha256、missing_image_count及delivery字段。export_delivery_failed或结果未知先核对已生成文件，不自动重新导出。

8. 只读校验两个成功回执明确返回的当前任务lumi-exports文件，不扫描项目/Host目录，不读写SQLite。可使用本任务正常文件工具或Python读取这些确切任务输出：完整字节长度/SHA256与回执一致；PDF能解析、页数和实际插图数一致；ZIP能解压、含book.md/book.json和第1页图片，三页文字与分镜完整，图片为最终当前原图，图片摘要与原图SHA256一致（如导出格式有转换则如实报告实际对应关系）。没有可用PDF解析器或文件读取被拒时如实记未验证，不借其它入口绕过。两个相对路径通过Cindy当前任务文件列表交付，file_generated/prepared不等于用户手机已收到。

9. 另存《参考图编辑候选与导出验收报告.md》，包含实际模型/provider/capability、reference_media_url及确实传入编辑Guide图片字段的脱敏请求、Core完成、一次导入、一次select_image、前后重读、原/编辑候选与最终选择、全部revision变化、两个导出原始回执和文件检查结果。移除一次性提交token、凭证和巨大base64，正文/提示词可用完整hash避免重复。保留先前全部报告。明确编辑画面是否纠正预期差异、是否引入文字或主体变化，不能用工具成功替代审稿。执行结束runtime_release，保留共享服务与作品，最终给出报告和两个lumi-exports文件路径；若展示编辑图仅使用本次实际受管结果一次，不冒称用户已下载。
```
