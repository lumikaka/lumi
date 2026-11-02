# Step 4–7 当前验收状态

## 正式独立作品恢复验收已通过

最新：用户已在原正式任务执行恢复指令，另存《正式版独立作品恢复验收报告.md》。父任务独立解析69次原始业务回执，确认从同一 draft 续接、初始化次数为0；setup ready，9项完整内容、5个标题、9处 revision 来源均核对通过。一次跨项目章节读取返回明确的 `LUMI_CHAPTER_NOT_FOUND`，前后12份完整响应 data 完全相同，包含正文、UUID、revision 与列表。连接已释放，原失败报告摘要未变。见[正式恢复成功证据](step6-evidence/normal-task-production-recovery-success-20261102.json)。

正式项目仍为 `01a19b9b-09a5-7e44-8ab7-cc44f90ff109`，setup revision 1→2，最终 ready；一章三页及一项角色设定完整保存、重读。正式修复包与后台均兼容，production PID `9912` 与开发 PID `54965` 均保持原代次、`open_projects=[]`。见[验收后服务观察](step6-evidence/production-recovery-service-state-20261102.json)。首次失败和被拒的恢复投递仍保留历史证据，没有被改写成成功或重发。

原开发任务的8份报告、PDF/ZIP字节数与SHA256亦再次核对未变，ZIP CRC有效，见[原产物保留检查](step6-evidence/production-recovery-preserved-artifacts-20261102.json)。这是产物完整性核对；父任务没有重新打开或读取任一作品数据库。

按用户已确认范围，Step 4 的 Core 图片链路、Step 6 已验收的本地安装/恢复与双作品流程、Step 7 的公开文件手动归档已交付。本轮不再要求重复恢复、初始化、切换后台或付费生成。Step 5 暂时跳过、两项待办及桌面重开跳过均保留未验状态；指定章节的语义拒绝不扩大为任意资源和写入方向的全面隔离，读取阶段停止也不扩大为在途写入/RPC/Core 强制取消。完整能力标准仍保留在各步骤文档，下方历史自动化记录保持各自范围。

[收尾核对记录](step6-evidence/step4-step7-closeout-20261102.json)列出本轮报告、两份审阅包、原开发产物和文档链接检查，以及所有用户跳过、延后与未声明范围。

## 当前范围决定

2026-11-02，用户要求先跳过 Step 5；可信任务页身份与四视图保持未完成，恢复条件继续保留。旧诊断进程退出确认、Codex 缺客户端及共享服务验收已转入 [待办清单](todos.md)，本轮不继续执行这两项。以下历史验收证据保留，跳过或转待办不计为通过。

Step 5 的跳过范围为生产任务页读桥及整本、单页、候选、设定四视图的实现与宿主验收。现有对话创作、图片读取和导出路径保留；将来恢复本步时，先验证可信任务页身份，再按 [Step 5 完成标准](../step5-task-view.md#完成标准)实施与验收。

| 待办 | 当前状态与依据 | 恢复与完成条件 |
| --- | --- | --- |
| 旧诊断进程退出确认 | 待办、未通过。上次观察 PID `20644` 为 `UE`、父进程为 `20640`；本次未重新检查进程 | 先核对进程身份及 PID 复用，再确认原进程和专属代理退出，保留正常共享服务；见[待办详情](todos.md#旧诊断进程退出确认) |
| Codex 客户端与共享服务验收 | 待办、未通过。用户确认客户端“没有或暂不可用”，真实双客户端共享尚未验证 | 具备真实 Lumi 开发版 Codex 客户端后，核对同一开发服务，保存 Cindy 操作前后 Codex 持续可用的回执；见[待办详情](todos.md#codex-客户端与共享服务验收) |

## 双版本独立作品自动化已补齐

新增[两份真实 CLI 的隔离回归](../../../../cindy-plugins/tests/channels-integration.test.cjs)：正式与开发分别保存完整章节，双向跨项目读取/写入被拒绝；仅重启临时开发服务后，开发作品 UUID、正文与 revision 保留，正式服务 nonce 和正文完整回执保持不变。测试使用独立临时 HOME/data-dir，未安装新包或修改真实作品。完整插件回归本轮为 **80/80 通过**，见[命令输出与构建指纹](step6-evidence/channel-isolation-20261102.json)。

这项结果属于真实 CLI 自动化；普通 Cindy 正式作品首轮曾因旧共享服务失败，之后[真实恢复验收已通过](step6-evidence/normal-task-production-recovery-success-20261102.json)。原[首次验收指令](normal-task-production-isolation.md)与[恢复指令](normal-task-production-recovery.md)保留为历史，无需重复执行。Step 5 和上方两项待办保持原状态。

## 最新原生拒绝复核

首次安装拒绝和新增权限更新拒绝均已验证，分别记录。首次是用户先卸载再拒绝安装，GHOST_NOT_FOUND符合预期；恢复原基础包后，用户保留开发版直接导入测试包并明确回复“更新已拒绝”。随后原开发build_id/CLI仍可调用，测试构建未生效，正式版原构建、PDF/ZIP及七份旧报告摘要均未变，连接已释放；本轮没有重装或提交安装工具。见 [首次安装拒绝](step6-evidence/permission-manual-rejection-20261102.json)、[更新前基线](step6-evidence/permission-native-update-refusal-baseline-20261102.json) 与 [原生更新拒绝成功证据](step6-evidence/permission-native-update-refusal-success-20261102.json)。原生点击来自用户确认，未伪造MCP安装取消回执。下文保留先前各阶段已验证结果。

2026-11-02。Step 4 的真实Core生成、授权导入、参考编辑、候选选择与PDF/ZIP导出链路已通过；Step 7 已按用户选择交付公开文件手动归档，Step 6 的正式独立作品恢复也已核对通过。本次按确认范围收尾；Step 5 暂时跳过，旧诊断退出和 Codex 共享验收列为待办，其余未验证边界见下表，均不计为通过。开发作品三页完整文字、第1页两版图片，最终选回原图，page.revision=3；正式作品为独立的一章三页文字，未执行图片或导出。历史回执保留各自当时的范围。

| 步骤 | 已交付和验证 | 剩余完成条件 |
| --- | --- | --- |
| Step 4 图片链路 | 已完成已验Core路径：真实生成→授权导入→重读；Lumi受控预览实际进入Core编辑，候选2导入后切回原图；真实PDF/ZIP通过独立文件核对；异常边界另有固定素材自动化 | 本步验收通过；不包含引擎原生/订阅额度、全三页插图或手机接收声明 |
| Step 5 任务审阅页 | 暂时跳过；真实诊断页、light 主题和公开上下文回执已保留，生产读桥保持关闭 | 将来恢复时，先实测可信任务页身份，再实现整本、单页、候选、设定四视图，以及图片、WS/Query 校准和任务隔离 |
| Step 6 安装交付 | macOS arm64 两身份安装、开发同版本更新、双实例只读诊断；正常开发三页、真实媒体/候选/PDF/ZIP通过；停用/启用、卸载/重装、双图保留、首次安装及新增权限更新拒绝、读取阶段停止恢复、项目锁释放通过；双版本 CLI 隔离、正式协议修复与后台切换、普通正式作品三页及指定跨项目章节拒绝均通过；自动化81/81 | 已按本次确认范围交付；旧诊断退出和 Codex 共享保留[待办](todos.md)，四视图随 Step 5 暂时跳过，桌面重开已跳过；不宣称其它取消范围或任意资源全面隔离 |
| Step 7 手动归档 | 显式公开文件解析、独立存储/迁移、CLI 导入、只读展示、UUID 游标和实时重读；真实公开样本解析及合成浏览器/重开验收 | 已按用户选择交付有限手动路径；不包含自动捕获、完整历史或把真实聊天写入正式项目 |

## 本次新增的恢复验证

[进程中断测试](../../../../internal/cindyarchive/process_exit_test.go)使用独立合成项目，在一次归档事务已写入线程、来源会话和两条记录但尚未提交时终止测试子进程。新管理器成功取得项目锁并重开，四张相关表没有半份记录。重导同一文件得到完整三条记录，再次重导不重复保存。

最终命令输出：

```text
go test ./internal/cindyarchive -count=1
ok  lumi/internal/cindyarchive  2.788s

go test ./cmd/lumi -run 'TestManualCindyImportCLIWebSocketReadOnlyAndDesktopReopen|TestCindyImportRequiresExplicitFileAndSourceWithoutStartingServer' -count=1
ok  lumi/cmd/lumi  0.910s
```

CLI 测试首次因沙箱禁止 loopback 监听返回 EPERM，经正常 require_escalated 重跑通过。完整记录见 [恢复证据](step7-evidence/process-exit-20261102.json)。只增加测试和文档，没有改动生产实现或已安装副本，也没有借合成测试重放被拒的 A/B 写入。

## 正常作品任务章节保存已通过

用户提供原 A 任务的《章节保存验证报告.md》，内含正常 lumi-dev / namespace=null / call_tool → chapters.write 的完整成功回执。读取空章节后渲染 story/story_chapter，使用 expected_revision=0 保存一次；回执 revision=1、version_no=1、char_count=875。报告记录保存后 get 的完整正文逐字比较为 true，content_hash=e01ce56fd5ca340bb75ea5af8de7754a96389ff56feaad8bab2ad7bcf79dfd98。来源任务公开摘要亦确认报告已生成，任务本轮已结束。

本源码任务核对了用户提供的报告和完整保存回执，没有通过探针再次写入或另行读取正文。归一化证据见 [正常保存回执](step6-evidence/normal-task-chapter-save-20261102.json)。仅此项通过；未创建页面、未生图、未导出。报告仅核实 read_only=false，未核实目标任务的 Ask/Auto/Full Access 档位，不能推断旧拒绝的根因已解决。

## 后续验收与权限观察

1. 本源码任务此前显示 Ask、公开 runtime 返回 auto/generation=0；验收探针写入和 send_to_session 转交曾被 MCP 层拒绝。正常作品任务随后已直接保存成功，不再将旧拒绝列为正常章节保存的全局阻塞，也不要求提供或修改 Cindy 源码。见 [权限诊断](step6-permission-mismatch.md)。
2. 任务页仍只有可自报的 `session` 查询和同源通信，没有已验证的 Host 页面身份。用户已选择保留 Cindy 任务页并等待宿主支持，不改成其它应用审阅，也不降低读取门槛。见 [任务页验收](../task-view-acceptance.md)。
3. 本次 `check_app_update` 仍为 `no_installable_update`、当前 0.1.121。当前 Agent 的 Cindy 工具也没有停用/启用/卸载入口，不能借其它应用的插件管理工具代替。见 [本次宿主复核](step6-evidence/host-readiness-20261102.json)。

正常作品任务的章节保存验证已结束。后续可在同一作品任务逐项验证页面保存、生图、参考编辑和导出，不以重新切权限或提供宿主源码为前置条件。本源码任务此前的转交拒绝仍保留为独立记录，不自动重发，见 [转交回执](step6-evidence/normal-task-handoff-20261102.json)。四视图仍需满足原任务隔离验收条件，安装生命周期操作仍走 Cindy 正常管理入口；现有项目、两身份和共享正式服务保留。

后续三页正文/画面描述验收已完成：目标正常任务生成了页面报告，三个创建回执分别使用父集合 expected_revision=0/1/2，集合最终 revision=3；六项逐字重读比较均为 true。当时章节保持revision=1、三页尚无图片；之后第1页已导入，见下文。见 [三页保存证据](step6-evidence/normal-task-pages-save-20261102.json)。先前 [页面验收派发](step6-evidence/normal-task-pages-dispatch-20261102.json) 中的 waiting-permission 是当时状态，现已结束。

## 第1页提示词修正与开发包更新

主人随后在原任务直接执行图片验收，报告显示 `render_prompt` 漏传变量，返回 `LUMI_PROMPT_INPUTS_MISSING`，缺少 `before_image_prompt`、`reference_usage_text`、`section_id`。Core 仅查询模型并 prepare，没有 request/poll；导入为0。重读 page.revision 0 → 0、仍无图片，原正文和画面描述不变。这是确定的执行前输入校验失败，不是已观察到的权限拒绝或未知付费结果。见 [归一化失败证据](step6-evidence/normal-task-first-image-validation-20261102.json)。

已修正 Lumi 提示词组合：片段模板新增 `fragment_prompt`；页面 `section_id` 从真实公共 UUID 派生；`section_image` 新增独立 `generation_prompt` 和 `image_write_back`，保留原任务封装兼容性。Manual 明确先取得有效基础规则和实际参考状态片段，组装成功后才 Core prepare；没有图片参考不能宣称已提供参考图。新增 Go 和真实 CLI 回归确认项目模板覆盖有效、伪造页面输入被忽略、正文/画面描述/revision 不变、过长提示词在生图前拒绝。

Go 回归通过，完整插件自动化 `tests 79 / pass 79 / fail 0`。另修正设定历史降级测试的旧假设，明确回退到对应迁移再验证保护，未更改降级生产逻辑。见 [Go 输出](step6-evidence/image-prompt-go-tests-20261102.txt) 和 [自动化](step6-evidence/automated.json)。

开发版 0.4.0 已经 Forge 原位更新到 `_root__lumi-dev` / namespace=null。实际 runtime_status 返回 build_id=`29401ca7cbd2cff3f21d5d2f7c3e2237f31990bfd99ac1e005dfd5faa068c1a8`，CLI SHA256=`d26c4636d6b0116520a3a2baef7cf203676ec6a91d3bc06ea4c58f9aef395c47`。审阅包与安装重打包21个条目内容、权限一致，见 [包比较](step6-evidence/image-prompt-development-update-20261102.json)。正式作者目录也重新构建供自动化使用，但没有把本次新构建安装到正式实例。

旧专用 development 服务在目标任务已结束/release、队列为空且 open_projects=[] 后，通过 `make cindy-dev-stop` 正常停止；只针对 `/private/tmp/lumi-cindy-step4-dev-data`。当时原生 status 和安装副本诊断均确认 server_running=false，正式服务未停止。后续原作品任务已正常重连并执行新字段对应的三次渲染和图片导入，见下文成功复验；不改写此前停止时的历史观察。

修正后的转交曾再次返回 `user rejected MCP tool call`，未提供原因，未自动重发。见 [当次完整修正与转交记录](step6-evidence/image-prompt-correction-20261102.json)。随后主人已在 [原作品任务](cindy://session/a0964e96-ba88-4c58-af32-4c0d356b7530?device=f2895f1dedcb8e7170726d62a917f1f2090e0404760b5f22cc6d378294b1d9fc) 直接发送复验指令并完成；无需再次转交或重复生成第1页。

## 第1页真实生图、导入和重读已通过

用户提供独立的《图片生成导入复验报告.md》，原失败报告保留。已核对完整成功导入回执、图片读取、页面当前选择、直接比较及最终大纲；公开任务摘要同时确认本轮结束、连接释放、队列为空。归一化证据见 [真实图片成功回执](step6-evidence/normal-task-first-image-success-20261102.json)。本源码任务只解析报告，没有重放付费生成或导入。

本次正常 lumi-dev 使用上述新构建，依次渲染 before_image、section_reference_absent、section_image，各1次，1473字符的 generation_prompt 与导入元数据逐字一致。实际模型为 `openai/gpt-image-2` / provider `openai`，Core request=1、poll=0、receive_media=1；本次未提供参考图片。PNG 原件2861885字节、1448×1086、精确4:3，keep 导入，source_sha256=`be9e712ebc226eca2ded54b64a1608a170fb8d194345144118ea01cf21998f16`。Core 地址中的摘要与导入原件摘要一致。

Asset=`01a19ad5-745d-71c1-b6ff-998d15b3f8d6`、variant=`01a19ad5-745c-7ba0-99c8-6b751bb67458`，导入、read_image 与页面选择一致；page.revision 0 → 1、图片version=1。正文、完整画面描述及其UUID保持不变。受控JPEG预览768×576已寄存，不能当作原始PNG或参考编辑成功。第2/3页仍无图、revision=0，章节revision=1；release 返回 shared_retained。

画面观察来自执行任务的预览报告：两行正文目视可读，但奶奶的可见部分多于分镜，门内多了一盏背景灯；未进行独立OCR或全书角色一致性审校。项目画风中“无图内文字”仍与模板/分镜的绘字要求矛盾，本次没有改动设置。因此只认定第1页工具链通过，保留画面差异和后续审稿范围。下一项为 Lumi 参考图实际参与一次编辑/候选选择及真实生成图导出；Step 5 可信任务页门槛、剩余安装生命周期矩阵仍待验。

## 参考图编辑、候选选择与导出验收

用户随后明确要求继续参考图编辑与候选选择，再做真实PDF/ZIP。已准备 [完整验收指令](normal-task-reference-export-next.md)：本次Lumi read_image的受控预览实际进入Core image.edit，最多一次付费编辑；新图导入为第二候选后，用最新revision切回原图，保留两版并以原图为最终选择；PDF/ZIP各导出一次，allow_missing_images=true。当前只有第1页有图，PDF不冒称完整三页，ZIP保留三页全文和现有图。文件需按实际回执的路径、大小及SHA256核对，prepared不等于手机收到。

当次向既有任务的send_to_session返回 `user rejected MCP tool call`，未提供具体原因；没有自动重发。见 [历史派发回执](step6-evidence/normal-task-reference-export-dispatch-20261102.json)。随后主人已在原任务直接发送指令，现已完成本项，不再要求重复派发。

正常任务另存《参考图编辑候选与导出验收报告.md》，Core `image.edit` request=1、receive_media=1、select_image=1、PDF/ZIP各导出1次，poll=0。输入确为本轮Lumi read_image寄存的768×576受控JPEG，实际传入编辑Guide的image字段。编辑PNG为2437133字节、1448×1086，Asset=`01a19aeb-076a-7744-a773-c972e1fd8551`、variant=`01a19aeb-076a-7412-8cd5-74466c68394e`，版本2；随后以expected_revision=2成功选回原版variant，第1页revision 1 → 2 → 3，正文与画面描述逐字不变。原图最终选中，编辑图保留候选2，连接已release。

本源码任务独立完整读取了两个明确交付文件。PDF：361740字节，SHA256=`67f061080fff48d722bd2ace6c25f936e55608b5594d3952659d7db1d6da3255`，pdfinfo解析为1页，内嵌1262×947 JPEG；这是原图的缩放压缩版本，不与PNG宣称无损相同。ZIP：2864730字节，SHA256=`51bd06fd9a26996505b25b1ac626b55d71926bd7668493cc01a96eb03541dc54`，CRC通过，包含book.md/book.json和第1页原PNG；PNG摘要与首次导入一致。875字章节及三页正文/分镜摘要均与原保存记录一致，Markdown也包含完整文本。两份文件的missing_image_count均为2，不作为完整三页插图作品。

完整证据和独立文件检查见 [编辑、候选与导出成功记录](step6-evidence/normal-task-reference-export-success-20261102.json)。Step 4 的Core主线至此通过；画面修正效果来自执行任务预览报告，未作独立OCR或全书角色审校。文件已准备到原任务lumi-exports，回执仍为user_received=false，未宣称手机已下载。

下一项是Step 6的原生插件停用/启用及后续卸载保留项目验收。已重新发现宿主工具：有Forge安装/更新，没有停用/启用/卸载接口；浏览器工具控制网页，不代替Cindy插件管理界面。主人已原生停用并重新启用：停用时实时GHOST_ASLEEP，启用后恢复ready且开发构建未变，正式Lumi始终可诊断，两个导出摘要未变。原作品恢复报告已通过：章节/三页文字与版本不变，原图仍被选中，原图及候选2均可读，连接已释放。主人已原生卸载，GHOST_NOT_FOUND/正式版正常/导出保留均核实；同包重装installed并恢复ready，专用共享服务PID/nonce保持，首次重装读取在MCP拒绝后停止，原回执保留为[拒绝证据](step6-evidence/plugin-reinstall-recovery-refusal-20261102.json)。主人随后在原任务直接发起新请求，本轮已完成项目重开、875字章节/三页全文与分镜比较、两版预览读取、最终原图与revision核对，连接已释放；七份旧报告hash独立核对未变，见 [重装恢复成功证据](step6-evidence/plugin-reinstall-recovery-success-20261102.json)，详见 [生命周期验收](step6-lifecycle.md)。Step 5继续按用户既定选择等待可验证的宿主任务页身份，不要求提供Cindy源码或降低读取门槛。

## 新增权限更新观察

此前两次Agent更新均返回updated，原包恢复属于历史检查点，不作拒绝证据。随后原生首次安装拒绝与保留旧包的更新拒绝已分别通过；本轮原开发构建和CLI仍可调用、测试构建未生效，正式版及导出/报告摘要不变，连接已释放，没有重装。准备包、历史回执和本轮完整结果见 [新增权限更新记录](step6-permission-update.md)。

## 本轮剩余验收条件与用户跳过项

原正常作品的系统项目锁已实际取得并立即释放，锁检查自身未改变锁文件inode/内容/mtime或完整project.sqlite摘要；恢复读取并释放后再次取得独占锁，见 [项目锁检查](step6-evidence/project-lock-release-20261102.json)。原生桌面重开已由用户跳过；Step 5 现亦暂时跳过，均未计为通过。用户确认无可用 Codex 开发客户端，旧诊断 PID `20644` 的上次观察为 `UE`、退出未确认；这两项现已转入上方待办表及[待办清单](todos.md)。正常任务A在读取基线标记后收到优雅停止请求，目标轮次结束；另行恢复的875字章节/三页完整正文与分镜/双图/大纲均匹配，连接已释放，原共享服务保留且无打开项目。见 [读取阶段停止及恢复证据](step6-evidence/normal-task-read-cancel-recovery-success-20261102.json)，不扩大为在途写入/RPC/Core请求被强制中断。完整记录与宿主前置见 [剩余验收](step6-remaining.md)。
