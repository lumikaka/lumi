# 用户参考、设定总览图和视觉核对

用户原图先实际查看并区分用途：画风控制造型、线条、色彩、材质和字体；主体参考控制身份、服饰和结构；故事截图只提供情节，除非用户要求参考其视觉。用户明确要求优先于原图特征，再优先于代理自选细节。“直接做完”不授权替换指定画风。

新项目在 setup 定稿前将具体视觉特征保存到 overall_style，将原图名称/编号、用途和用户限制保存到 generation_brief。字体的笔画、字距、基线和气泡特征同时进入页面 storyboard_md。只写“温暖手绘”不足以核对；不要自行添加低饱和、毛绒或大头短肢等风格。已定稿项目不重跑 setup；与全局画风冲突时先使用真实可用的画风编辑入口解决，能力不足则说明阻塞。

实际生图必须把相关原图传入当前通道：原生引擎用其正式图片参考参数；Cindy Core 按该 provider/model 的 Guide 使用已授权媒体引用。每批核实请求包含原图，文字、附件名、设定 UUID 或曾看过图片均不算传图。Core 与原生路径分开验证；不得把 Codex 的临时路径或参数名直接用作 Core 媒体引用。无法覆盖全部参考时保留已保存内容，并说明缺少的图片输入。

## 批量生成和原子拆分

1. 先 settings list/get 复用符合要求的已有项。每批 1–6 个独立主体，复杂场景或精修单独处理。render_prompt 使用 premise/setting_image、target=setting_board；variables.input_text 明确本批主体、视觉要求及每张用户参考用途。使用返回的完整 generation_prompt，不能覆盖服务端画风。
2. 生图后立即顶层 receive_media，slot=setting_board；source_text、style_snapshot、expected_revision 来自 write_back，prompt 保存实际完整提示词，idempotency_key 是本次稳定唯一键，图片只放 ghost_call 顶层 attachments。不要将 Go write_back 的 path 导入直接复制到 call_tool。
3. 原图会同时保留在任务 lumi-media-recovery/ 的摘要命名文件中，使重启后原键/原参数交接使用稳定来源路径。业务事实仍由标准项目保存。记录 board_uuid；setting_boards list/get 返回 source、setting_image.asset、revision、latest_breakdown。总览图已导入不代表各项设定就绪。
4. read_image 实际查看已保存总览图，核对用户原参考和画风。render_prompt 使用 premise/asset_breakdown、target=setting_breakdown、board_uuid。识别完整原图中的主体及安全边距，按实际画面提交 0–1 归一化 crop_box，不按网格机械等分；置信度至少 0.92、框不重叠、每项 1–3 标签。
5. apply_setting_breakdown 提交 board_uuid、批次 expected_revision、expected_premise_revision、稳定 idempotency_key 和 assets。更新已有项同时指定 setting_uuid 和最新该设定 expected_revision；新项省略二者。裁切、所有设定及图片版本一次提交，不调用模型重画裁切。
6. settings get/read_image 检查裁切完整、无邻近主体污染及画风。裁切不修复画风；不合格项保留历史，用相关用户原图重绘同一设定 UUID。修正裁切使用原 board_uuid、最新批次/集合/目标 revision 和新键。

导入/拆分中断先 setting_boards list/get 检查。确认要恢复时按原键、原参数重放；已提交请求返回原结果，不重复建项。不同参数不能复用键。版本冲突先读事实并判断方案，不只更换 revision。最新采用结果仍以 settings get 为准。只读任务、跨项目引用、未定稿项目均不能绕过业务校验。

## 页面参考前的核对

先 image_target(target_uuid=page_uuid,slot=page_image,setting_uuids=[...])。setting_readiness 给出 style_prompt/style_digest 及每项 setting_uuid、variant_uuid、expected_revision。review_required 表示有图片，尚未通过核对。

实际查看每项当前图，对照用户要求与原图的造型/五官比例、线条粗细颜色、配色饱和度、材质明暗和身份。每项 setting_reviews 为 `{setting_uuid,variant_uuid,expected_revision,style_digest,verdict,reference_basis,comparison}`。reference_basis 记录原图和用户明确覆盖要求；comparison 记录“目标特征—实际观察—结论”，各非空且最多 2000 字符。verdict 仅 passed / needs_revision，通过才用于正文。无原图时以已保存画风为依据。

prepare_page_image 将记录绑定当前图片、revision 和画风并纳入签名；服务端验证对应关系，视觉判断仍由看图的代理负责。设定修改、重绘/采用或画风变化后重新核对，不能只改 verdict、版本或摘要。无参考 [] 仅用于确实无需参考或用户明确允许的范围，不能绕过缺图/失败核对。首张正文生成后核对整体画风、字体及文字准确性，再继续后续页面；按已有授权自检修正，不新增逐图确认节点。

页面完整合图由 prepare_page_image 在内部按任务和实际调用分块交接，再通过 Host deposit_media 入库。Core 仅使用 status=ready 的 full_collage 回执及其 reference_media_url；按本次 Guide 明确传图，检查实际请求包含该 URL 与完整 generation_prompt。原生引擎显式传同一 reference_image_path，不复制 Core 的参数结构。交接失败不提交 Core request；取消/任务切换/释放及 worker 重建后，旧传输令牌失效，重新准备当前事实。内部 reference_* 方法不属于模型工具，不能主动调用或自报合图路径。
