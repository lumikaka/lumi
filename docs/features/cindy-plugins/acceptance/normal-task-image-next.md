# 第 1 页真实图片生成与导入验收

2026-11-02：**本项现已完成，无需再次发送下面指令或重复付费生成。**主人已在原任务完成单次Core生成、导入和重读，page.revision 0 → 1，见 [成功证据](step6-evidence/normal-task-first-image-success-20261102.json)。原失败报告保留；下文作为当次修正后的验收流程保留，不作为后续图片编辑或导出的指令。

正常作品任务的三页文字已保存、重读通过；首次图片尝试因缺少变量在执行前停止，Core request和导入为0。以下流程随后由主人在原任务「Lumi acceptance · A」（a0964e96-ba88-4c58-af32-4c0d356b7530）执行；本文件本身不是执行回执。

```text
继续在本任务使用正常 lumi-dev（namespace=null）验收《借给月亮的一盏灯》第1页《借一盏灯》的真实生图与导入。

先重读作品、章节和页面，核对 project_uuid=01a199d7-323a-7017-a5da-1d93303f487e，chapter_uuid=01a199e6-89e9-7213-a333-924bffa24af8，page_uuid=01a19a9f-e071-7ce8-9e8b-175e4009c456。若已有图片先核对后停止，不重复付费。保留既有章节和三页文字。

先核对运行中的开发构建，再 image_target。通过 render_prompt 渲染 chapter/before_image 和 chapter/section_reference_absent，均以 target=page、实际 chapter_uuid/page_uuid 调用，各取返回的 fragment_prompt。本次未提供参考图片，不能使用 section_reference_present。再渲染 chapter/section_image，variables.before_image_prompt 和 variables.reference_usage_text 分别填入上述两个实际片段；section_id 由新服务端从页面公共 UUID 派生。核对返回 generation_prompt 包含当前保存的画面描述和完整页面文字，且没有 Agent 任务封装、JSON-only 输出或禁止调用 provider 的外层指令。图片只用 generation_prompt，不复制外层 prompt。记录 image_write_back，不按原 write_back 更新页面文字。字段缺失就停止并报告运行构建，不能硬编模板或自行切后台。

提示词组装成功后，才由当前 Agent 使用 Cindy Core media 的本次可用模型目录及新的 prepare Guide 生成1张图；不要沿用先前未使用的 prepare token，保持实际选择的 model_id/provider_id，不擅自换源。只提交1次付费请求；pending 使用同一 invocation_id 按建议间隔 poll，拒绝、取消、超时或提交结果未知都停止，不重提。确定的执行前输入校验错误可以按 schema 修正；不能把这种修正扩大为重放未知付费请求。不要用其它生图工具或脚本代替本项 Core 链路。

成功后立即用 ghost_call 顶层 tool=receive_media、顶层 attachments=[实际受管图片引用] 交给 lumi-dev；业务 args 使用 image_write_back 的 target_uuid、slot=page_image、expected_revision，fit_mode=keep，prompt 保存本次完整 generation_prompt。不要把附件放进 args，不自报路径、指纹或 base64，不调用 resolve_local_path，不使用验收探针或直接文件/数据库写入。发生版本冲突或结果未知时先读取事实，不自动重新生成或导入。

导入后 pages.get 核对 Asset/variant UUID、尺寸、当前选择和 page.revision，再 read_image 读取受控预览并查看画面。发生冲突或结果未知时先读取事实，不自动重新生成或导入。

另存《图片生成导入复验报告.md》，保留原失败报告。记录实际开发 build_id/CLI 指纹、提示词三个渲染回执（可用字符数/hash 代替重复全文）、实际模型/provider、调用次数、Core 完成与导入回执、受管图片引用、Asset/variant UUID、source_sha256/source_bytes、前后 revision 和预览观察。去掉凭证及巨大 base64。只声明本页生成导入结果，不声称参考编辑、其它页图片、PDF/ZIP或四视图已通过。不宣传未经核实的订阅额度来源。

本次不生成其它页、不编辑、不导出，不启动其它 Agent。结束时 runtime_release，保留作品和共享服务；用实际受管图片引用展示一次成果，并给出报告文件路径。
```
