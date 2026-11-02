# Cindy 图片交接与参考图

2026-11-02：Step 4的Core路径已通过：正常任务真实生成、授权导入和重读；Lumi受控预览实际进入Core image.edit，一次编辑导入候选2并成功切回原图；真实PDF/ZIP导出和独立文件核对通过。见 [首图回执](acceptance/step6-evidence/normal-task-first-image-success-20261102.json) 与 [编辑及导出证据](acceptance/step6-evidence/normal-task-reference-export-success-20261102.json)。PDF仅含第1页，ZIP保留三页全文，另两页缺图；不宣称全书插图、手机接收或订阅额度已验。自动化与Forge另见 [验收记录](acceptance/step4-image-import.md)。

## 原件到项目

Agent 先调用 image_target 和 render_prompt，使用当前可用媒体能力生成图片，然后调用顶层 receive_media。参数来自目标公开 UUID、slot、expected_revision 与可选 fit_mode/description/prompt；只接受 ghost_call 顶层 attachments 中恰好一张图片。call_tool 无法绕过附件入口，业务参数不能指定源文件路径、Host 指纹、base64、项目根或任务身份。

页面生图先渲染有效的 `chapter/before_image`（封面/封底使用对应片段）与本次实际参考图状态对应的片段，取各自 `fragment_prompt`，作为 `section_image` 的 `variables.before_image_prompt` 和 `variables.reference_usage_text`。没有真正提供参考图片时使用 `section_reference_absent`；文字设定不算图片参考。页面 UUID 对应的 `section_id` 由服务端读取，调用方不能覆盖。将最终 `generation_prompt` 原样交给图片模型及导入记录，使用 `image_write_back` 的目标、slot 和 revision。外层 `prompt` / 原 `write_back` 仍是 Agent 任务封装，不作为图片模型输入或页面文字更新指令。

提示词组装全部成功后才 Core prepare。`LUMI_PROMPT_INPUTS_MISSING` 是已知执行前校验错误，可以补参；拒绝、取消、超时或未知付费/写入结果不能自动重放。旧共享后台若未返回上述字段，先核对构建并协调后台更新，不硬编模板或从任务封装里猜截取边界。

main.js 从 Host 注入的附件指纹访问自身同源 `/media/<hash>.<suffix>`。没有后缀时仅依次尝试 png/jpg/jpeg/webp；只对 404 尝试下一个后缀，其他拒绝立即失败。不扫描媒体目录，不请求供应商。流式下载限制 20 MiB、核对有效 Content-Length、真实魔数和 MIME，并计算 SHA256 与授权指纹比较。没有可读取原件时保留现有作品，不能改传私有路径。

worker 接受内部 media_begin/chunk/commit/abort，随机 transfer 只在内存中存在，绑定当前 worker、规范化项目、任务对象与原始调用的摘要。同一任务的另一调用、另一任务、另一项目或重启后的 worker 不能接管。每块原始字节 48 KiB，base64 最多 64 KiB；包含上下文的 JSON 仍远低于 Node 256 KiB 参数上限。块索引必须连续，重复、乱序、非规范 base64、长度溢出均拒绝。

逻辑页与 worker 各自限制两条并行接收；worker 原图预留合计最多 40 MiB。这是逻辑容量限制，不是进程 RSS 上限，缓冲和解码仍有额外内存开销。未完成传输 90 秒空闲或 5 分钟总时限后失效；现有任务绑定约 60 秒空闲也会撤销。下载设 90 秒期限，单次 Node 请求沿用 60 秒及 worker 50 秒期限。所有 Node 调用使用当次 callId/cancelWithCall，callId 不落盘、不交页面。

commit 再校验总字节数、SHA256 和真实格式，才在本插件实例的 0700 临时目录创建 0600 源文件。现有 Lumi import_image 完整解码，限制 PNG/JPEG/WebP 与 4000 万像素，并复核项目归属和 expected_revision。成功返回原有 Asset、版本、尺寸与比例结果，增加 source_sha256/source_bytes/source_mime；暂存路径不回传。

## 取消、冲突和回收

正常成功、已知业务失败和异常完成均清理暂存文件；未提交块只存在内存，worker 退出后消失。提交期间进程被强制结束可能留下临时文件；下一次媒体提交只清理同一实例命名空间内有合法标记且 PID 已不存在的目录，不删除活进程或来源未知的文件。不能以退出回调或卸载回执证明清理已完成。

写入期间目标被修改时，原业务服务返回 production_conflict，当前选择不被覆盖。适配层把原件保存在当前任务 `lumi-media-recovery/<SHA256>.<ext>`，失败回执的 correction.recovery_file 给出相对路径、完整 SHA256、大小和恢复说明。此文件不是已挂载的 Asset，也不表示用户已经收到；重读目标并判断后，可以把已知恢复文件再次作为授权附件交接。恢复失败须如实报告，不把 Host 临时缓存称为永久备份。runtime_release 撤销旧传输；未知写入结果先重读当前版本，不能自动重放。

keep/crop/pad、比例偏差提示、图片去重及不可变原图/派生图历史均使用现有 production/files 服务。Cindy 不直接写素材索引，原 Codex import_image 的路径调用与默认行为不变。

## 参考图

read_image 继续使用现有 CLI 有界预览：最长边 256–1024，JPEG 最多 120 KiB，MCP 完整结果最多 256 KiB。Cindy 协议适配器提取 MCP ImageContent，main.js 在 Host tool-result 顶层 images 交给模型，预览 base64 不重复出现在业务 JSON 中。

manifest 声明 `cindy.media: [deposit]`；同一受控预览通过 deposit_media 寄存，成功时返回 reference_media_url 与 `reference_delivery:{kind:bounded_preview,deposited:true}`，供 Core image.edit 输入。寄存失败仍保留模型看图路径，deposited=false；这不代表 Core 已取得参考图。引擎原生编辑使用刚读到的对话参考图。寄存对象是预览，不能称为原图。

当前实现没有模型 API key，没有额外生成供应商，没有后台生成 pipeline。Core 与引擎原生路径分别记录实际 provider/model、输入、输出和结果；未知额度来源保持未知。

## 验证矩阵

| 场景 | 当前证据 | 范围 |
| --- | --- | --- |
| 原件分块、摘要、大小、MIME、拒绝授权 | media-main.test.cjs、media.test.cjs | 浏览器桥与 worker fixture |
| 重复/乱序块、跨任务/调用、取消、过期、死 worker 清理 | media.test.cjs、media-integration.test.cjs | 自动化，不是宿主生命周期证明 |
| PNG/JPEG/WebP、keep/crop/pad、损坏图、像素与文件限制 | Node 真实 CLI 集成及 production/textmcp Go 回归 | 项目业务与原 Codex 路径 |
| 826433 字节原件 → Asset Store → 完整 HTTP 读回 hash | media-integration.test.cjs | 固定素材，非本轮真实生成 |
| 受控预览、两版候选、冲突原图恢复、PDF/ZIP | media-integration.test.cjs | 隔离项目和专用 CLI |
| 当前产品包真实 Host 授权附件与模型输出 | 正常任务第1页通过；PNG2861885字节、1448×1086，Core/导入摘要一致，Asset/variant/revision重读一致 | 实际openai/gpt-image-2 / openai；首次生成无参考输入 |
| Lumi 参考图实际参与一次 Core 编辑 | 本次受控JPEG确实传入image.edit的image字段，单次编辑/导入成功；候选2后按最新revision切回原图 | 768×576预览作输入，非原始PNG；编辑PNG2437133字节，1448×1086，保留两版 |
| 真实生成图PDF/ZIP与候选选择 | page.revision 1→2→3；PDF/ZIP各导出1次并独立核对摘要、PDF结构及ZIP全文/原图 | PDF1页、ZIP三页全文及原图，缺图2页；prepared不代表手机收到 |
| 额度来源、其他架构、手机图片交付 | 未验证 | 不宣传为已支持 |
