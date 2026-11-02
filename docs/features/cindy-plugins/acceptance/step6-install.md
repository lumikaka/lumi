# Step 6 安装验收状态

## 当前范围与新增回归

当前：普通正式任务已从原 draft 完成 setup、故事、设定、一章三页正文及分镜，父任务独立核对69次回执、9项完整内容、5个标题和12份隔离前后完整快照通过；指定开发章节返回 `LUMI_CHAPTER_NOT_FOUND`，连接释放，原报告不变。正式/开发服务均保持预期代次，见[恢复成功证据](step6-evidence/normal-task-production-recovery-success-20261102.json)。

首次失败由旧共享后台不识别项目绑定字段造成，[协议修复、正式包更新和授权后台切换](step6-shared-protocol-fix.md)完成后，用户在原任务直接执行[恢复指令](normal-task-production-recovery.md)并通过验收。此前投递拒绝保留原回执，未重试；插件回归81/81。Step 5、两项待办与桌面重开跳过保持原状态，本次按已确认范围交付，不将边界外能力计为通过。

此前双版本回归阶段为 **80/80 通过**，见[命令输出与构建指纹](step6-evidence/channel-isolation-20261102.json)。该阶段只运行作者构建、没有安装更新；之后的正式包安装、共享后台切换和普通作品恢复分别留有独立证据。以下按历史阶段保留原结果。

## 最新原生拒绝复核

首次安装拒绝和新增权限更新拒绝均已验证，分别记录。首次是用户先卸载再拒绝安装，GHOST_NOT_FOUND符合预期；恢复原基础包后，用户保留开发版直接导入测试包并明确回复“更新已拒绝”。随后原开发build_id/CLI仍可调用，测试构建未生效，正式版原构建、PDF/ZIP及七份旧报告摘要均未变，连接已释放；本轮没有重装或提交安装工具。见 [首次安装拒绝](step6-evidence/permission-manual-rejection-20261102.json)、[更新前基线](step6-evidence/permission-native-update-refusal-baseline-20261102.json) 与 [原生更新拒绝成功证据](step6-evidence/permission-native-update-refusal-success-20261102.json)。原生点击来自用户确认，未伪造MCP安装取消回执。下文保留先前各阶段已验证结果。

2026-11-02：两身份已安装、开发版同版本更新；自动化79项通过。正常任务三页文字、第1页真实生成、Lumi参考图参与Core编辑、候选2导入与切回原图、PDF/ZIP各一次导出均已通过。父任务独立核对两个文件、ZIP全文及原图，PDF仅1页、缺图2页。原生详情页停用开发版已验证GHOST_ASLEEP，正式Lumi仍可诊断，导出文件保留；重新启用后两版诊断均成功，原作品章节、三页文字、双图恢复读取已通过；原生卸载/同包重装及重装后的同一作品/双图重读也已通过。见 [媒体导出成功证据](step6-evidence/normal-task-reference-export-success-20261102.json)、[停用证据](step6-evidence/plugin-disable-20261102.json) 与 [当前汇总](step4-step7-status.md)。四视图、独立作品双实例及其余生命周期/共享矩阵仍待验，Step 6未完成，无发布。下文按历史阶段保留当时回执和构建。

## 真实安装与同版本刷新

执行端是 Mac arm64 的 Cindy 0.1.121；实际应用 sourceCommit 仍为 `0a7d5a419f5cd3e0fbebc9642dde8388376a68aa`。开发实例是 `lumi-dev` / `_root__lumi-dev` / `namespace:null`，作者来源是仓库 `build/cindy-dev/lumi-dev/`；没有改为正式身份或另装组织副本。

[首次成功回执](step6-evidence/install-success-20261102.json) 记录安装与初次诊断；[更新回执](step6-evidence/install-updates-20261102.json) 记录本轮同来源、同版本的 updated 结果。2026-11-01 的 [拒绝回执](step6-evidence/install-attempt.json) 是历史记录，不抹除，也不把当时未安装改写为已安装。本轮继续基于用户新的明确授权。

实际诊断返回 development、darwin/arm64、CLI SHA256 `484f41b98367503a0cbebfbce807c9efda7b0d3ca30b550a8567d4d79cae37c1`。初次安装 build_id 为 `c4974a78c0f1ddd87c0355467a4749a18e89be4609f8dd2f7acfacd241fe8eda`，并发名额修复后为 `3f42be1b7d4e885d58a798c10246a768e0e40e0a54fa7540f8515adccdadbf1a`；该次真实产品连续 6 次 runtime_status 均成功。最新启动取消修复已另更新为下文 `bb0262af…`，没有为 JS 更新停止共享默认服务。

首次修复包的审阅 ZIP 已按摘要固定保存。开发包审阅 SHA256 `b2b5d10610e3d7d1fedb231ef289684cadd77c5437038cff3c8129945673e820`，安装重打后为 `caa3885676f9b922e4c7e320fd78c1cac79ab6bdb726639bf8fc7c0f07b5b702`；两者 21 个条目逐字节相同，也与该次未再编辑作者树相同。ZIP 整体摘要有变化，因此保留两者而不声称文件完全相同。见 [首次比较](step6-evidence/install-package-comparison-20261102.json)。Host 安装回执没有返回安装副本的 ZIP 摘要，重打包摘要与运行 build_id 分别作为证据，不填造 Host 未返回的字段。

## 真实宿主发现并修复的缺陷

实际 Host 代启子进程返回窄代理，支持 on/once/off/kill，不是完整 Node ChildProcess。原 worker 在成功回包后的 finally 调用 removeListener，抛错后未删除在途调用，最终返回 `LUMI_RUNTIME_BUSY`。使用 Node EventEmitter 的旧 mock 没有暴露该差异。

现在改用 off，先释放调用名额，再执行有界、容错的子进程清理；完成后到达的 exit 不再取消旧调用。新增两项回归分别验证窄代理连续 12 次调用、清理抛错后名额仍可用。两项在修复前失败、修复后通过，该次完整输出为 `tests 74 / pass 74 / fail 0 / skipped 0`。见 [Host 代理核对](step6-evidence/host-child-proxy-20261102.json)、[当次自动化原件](step6-evidence/automated-before-startup-cancel-20261102.json)。Go 业务源码未改，前轮 [Go 回归](../acceptance/step4-evidence/go-tests.txt) 仍保留；未运行 Cargo/Rust。

### 启动期间取消的补充修复

随后新增三项回归，修复前均失败：Host 子进程在 spawnEntry 返回前已经退出、等待子进程时收到取消、等待子进程时关闭 worker。现在先注册 exit/error 监听，再核对代理 killed 状态和调用 AbortSignal，取消时不进入业务执行。测试使用 mock 生命周期验证竞态，不将其称为真实宿主取消验收。

最新随包正式/开发目录已重建并通过 Forge pack 和 21 条目逐项检查；完整回归为 `tests 77 / pass 77 / fail 0 / skipped 0`，见 [最新自动化](step6-evidence/automated.json)。开发版 0.4.0 已原位更新，实际 build_id 为 `bb0262af16aa166c93751fe10bba39c43529b757dc48e29863030e97d1a239eb`。连续 6 次真实 runtime_status 成功后已 runtime_release；server_running=false。见 [安装回执](step6-evidence/startup-cancel-install-20261102.json)、[实际调用](step6-evidence/startup-cancel-runtime-20261102.json)。没有恢复前次被拒绝的作品写入或付费生成。

最新开发包审阅 SHA256 为 `984462df6598de5e189d0333c585c8d01d7545e3a55d296f88d57eb2b7cda0f5`，安装重打后为 `e8ab476ed950a4bd68120dbe93e573030a7a34f9d3bf6abbfce847bfa6b1de5d`；21 条目内容和当次作者树一致，见 [最新包比较](step6-evidence/startup-cancel-package-20261102.json)。正式包只 pack，未安装。当时临时探针仍为前次 3f42be1b… 版本；下文另记恢复验收时对齐 bb0262af… 的更新，不把旧回执归到新构建。

## 隔离项目与真实创作进度

临时验收插件 `lumi-host-acceptance-20261101` 已安装及更新，实例 `_root__lumi-host-acceptance-20261101`、个人范围。产品 main.js 原文静态嵌入，Node/CLI 复用，另加公开 tasks 权限以取得 Host 创建的独立空目录；不发送消息、不启动其他 Agent。它是宿主集成 harness，不能替代普通产品 session_context 绑定或四视图的证明。

Host 创建的任务 A 为 `a0964e96-ba88-4c58-af32-4c0d356b7530`，B 为 `862d6471-47aa-4291-a2c0-ac4f906de846`。A 保存了《借给月亮的一盏灯》的 setup、STORY.md、一个章节和三个文字设定；最终 page_count=0，章节 current_story=null、revision=0。B 是独立项目；通过 B 读取 A 的章节/设定分别返回 LUMI_CHAPTER_NOT_FOUND、LUMI_PRODUCTION_RESOURCE_NOT_FOUND，没有返回 A 的内容。

更新探针后，原 create 请求键重放返回 Request key was used with different input；没有换键重复创建任务。探针现支持仅用前次 task_id 回执经公开 tasks.get 的本插件归属检查、标题校验恢复；不接受模型填写目录。两任务原 ID 和 ask 权限保持不变。此恢复通过，不将 create 重放冲突写成幂等通过。

章节 write 调用随后收到非 JSON 的 `user rejected…` 拒绝文本，编排在 JSON.parse 处停止；当时未保存完整原始拒绝回执，这一证据缺口明确保留。随后 get 确认章节仍为空，没有重试该写入，没有通过 shell 或其它入口绕过。依赖这次写入的分页、付费生成、参考编辑、图片接收和导出没有开展。已创作但未提交的正文随 [宿主运行记录](step6-evidence/host-run-20261102.json) 保留，继续时先重读版本。

Core 本轮 list_models 对生成和编辑均只列出 `openai/gpt-image-2` / provider `openai`，prepare 返回 Guide revision 3；未调用 request，不产生本次生成结果。不能沿用前轮 Qwen 可用结论，也不能把 prepare 称为生图通过。

## 任务页与收尾

诊断 taskView 已在实际宿主打开并回报 light 主题、wake running、公开上下文 locale/region。它还发出了 A/B 的其他 session 声明，没有 Host 页面身份凭据，project_content_read=false。生产业务读桥保持关闭，四视图未实施；见 [Step 4.5 状态](../task-view-acceptance.md)。未验证 dark、切换、最大化、图片和持续 WS，不以诊断页替代产品 UI。

A、B 连接均已 release，保留项目。专用 `/private/tmp/lumi-cindy-host-acceptance-20261101` 的 CLI 经 nonce 条件停止，重读 after_running=false；没有触及默认开发、正式或 Codex 服务，也没有删除任务或作品。见 [清理回执](step6-evidence/probe-cleanup-20261102.json)。两个插件仍安装；现有工具发现与 self-management 索引未提供停用/卸载接口，未直接操作 Host 安装目录，不声称卸载已通过。

| 场景 | 当前状态 |
| --- | --- |
| 正式/开发不同身份、CLI、environment、data-dir | 构建及自动化通过 |
| 开发首次安装、工具发现及诊断 | 真实 Host 通过 |
| 同来源同版本更新、新 build_id 实际调用 | 真实 Host 通过；6 次连续诊断成功 |
| 正式与开发同时安装并各自调用 | 两身份安装、两份真实 CLI 独立作品与重启隔离通过；普通开发媒体主线和普通正式三页恢复均通过；正式项目内读取指定开发章节被语义拒绝，前后完整内容不变，见[恢复证据](step6-evidence/normal-task-production-recovery-success-20261102.json) |
| Go 旧后台切换、共享 Codex 持续可用 | 空闲专用development后台已正常停止，新开发包已安装并在原作品任务完成新提示词及图片调用；Codex 缺客户端，共享验收已转[待办](todos.md#codex-客户端与共享服务验收) |
| 新增权限更新拒绝 | 原生文件导入拒绝通过：原开发构建/CLI继续可调用、无重装，正式版及导出摘要不变；首次安装拒绝另留证据 |
| 停用/启用、卸载保留项目与残留进程 | 原生停用/启用、卸载/同包重装和作品/双图保留通过；旧诊断进程退出确认已转[待办](todos.md#旧诊断进程退出确认) |
| 真实 Agent setup、故事设定与双项目资源隔离 | 部分通过；正常任务章节、三页文字和第1页真实图片已保存重读，双项目资源拒绝有独立探针记录 |
| 真实三页、生成、参考编辑、候选选择、PDF/ZIP | 三页文字、第1页真实生成/参考编辑/两版候选切换、真实PDF/ZIP通过；PDF1页、另两页缺图，四视图未实现 |
| 读取阶段优雅停止与恢复 | 正常A任务基线标记后请求停止，再重读章节/三页/双图均匹配并release；实际项目锁可取得，原共享服务保留；不覆盖在途写入强中断 |
| 原生桌面重开 | 用户明确跳过，未执行、未计通过 |
| 四视图与可信页面身份 | 随 Step 5 按用户要求暂时跳过；门槛未通过，生产读桥关闭 |
| Step 7 会话归档 | 默认不采集；公开文件手动路径已交付，独立存储、开发包更新及实际浏览器响应式/分页验收通过；不含自动捕获和完整历史 |

## 新许可后的实际拒绝与当前方向

用户随后明确选择允许 A/B 继续写入、生图和参考编辑，并允许仅导出本次任务的公开用户/助手文本，见 [范围许可](step6-evidence/resume-authorization-20261102.json)。探针已用同一 bb0262af… 产品代码重新构建、打包并原位更新；21 项内容比较一致，见 [新探针包](step6-evidence/resumed-probe-package-20261102.json)。恢复的是原 A/B 任务，没有新建或启动 Agent；章节 get 仍为 revision=0、current_story=null。

重渲染章提示词后，实际 chapters.write 在 MCP 层再次返回原文 `user rejected MCP tool call`、isError=true，没有提供拒绝原因。此次已完整保存原始回执和随后 get，确认没有写入，见 [拒绝证据](step6-evidence/renewed-write-refusal-20261102.json)。公开运行能力说明显示当前权限 mode=auto。不能再把这一拒绝归因为“用户未明确允许”，也不把它写成业务版本冲突或模型失败。

在这一结果之后，用户选择“保留 Auto，先完成代码和归档验证”，见 [当前方向](step6-evidence/approval-route-20261102.json)。不更改任务权限，不再重试章节写入，不发媒体 request。真实三页媒体主线按此选择暂停；不重复请求已经给过的许可。公开归档样本及受限解析器先通过验证，随后新增独立项目持久化、手动 CLI 导入和桌面只读展示，合成项目端到端通过；见 [Step 7](../session-archive.md)。真实文本仍未写入用户项目，不能以样本解析或合成测试声称真实项目归档已验收。

A/B 已再次 release。作者目录的原生 status 诊断命令无响应，专用服务自身的公开身份接口正常且没有打开的项目；据此匹配 environment/nonce 后停止该专用服务，并核实 discovery 撤销、singleton lock 可取得。默认开发、正式服务与项目未改动。见 [本次收尾](step6-evidence/resumed-probe-cleanup-20261102.json)。本次创建的只读 status 子进程 PID 20644（父进程 20640）请求终止后仍为 UE 状态，退出未确认；没有强杀 Cindy 应用，不把服务停止等同于所有诊断进程已退出。完整运行记录见 [resumed-run](step6-evidence/resumed-run-20261102.json)。

任务页仍另需可验证的 Host 页面身份契约，不能通过扩大读取范围绕过。Step 7 的手动归档路径已独立实现，开发包更新及实际浏览器验收也已完成；真实写入主线按上述选择保持暂停。本轮再次只读查询上轮遗留的 PID 20644，仍为同一 status 命令、父 PID 20640 和 UE 状态；未再次终止进程或重启 Cindy。


## 归档代码后的开发包更新

2026-11-02 再次构建开发版，保持 lumi-dev / 0.4.0 / development / 专用 step4 data-dir。经 Forge pack、逐文件审阅和 target_instance_id=_root__lumi-dev 原位更新；实际宿主诊断返回 build_id=d3452b2a24b56a97099e568521578daeafdb02ee4dfbb19845c72da80bb94bbd、CLI SHA256=b74ab5a4ac0af9ce6068cf020c999309d0640ffbcb92a19e4b32f91782fac436。更新前后对应共享服务均未运行，没有停止默认或 Codex 服务。

审阅 ZIP 与 install 重打 ZIP 的 SHA256 分别为 178e0d04… 和 6acea446…，21 项内容全部一致，见 [开发更新回执](step7-evidence/development-update-20261102.json)。正式作者包也以当前 Go 源码重建并 pack，尚未安装正式身份，不把它列为双实例并存验收。当前完整插件自动化为 79 项，见 [Step 7 自动化](step7-evidence/automated.json)，与历史 77 项分别保留。

独立 embed_frontend 验收程序通过合成归档的实际浏览器展示、增量实时读取及 54 条文本分页；见 [浏览器记录](step7-evidence/browser-acceptance-20261102.json)。它不替代 Cindy taskView 的可信页面身份，也不是 A/B 真实章节和图片链路的验收。专用页面服务已停止并核对锁释放。

探针作者构建改为以新 inode 原子替换原生 CLI，避免就地改写仍可能使用中的可执行文件；旧打开句柄保留旧字节和符号链接拒绝测试通过，重建的新副本 --help 正常。见 [原生复制记录](step7-evidence/probe-native-copy-20261102.json)。这次只重建作者探针，没有重新安装它或恢复 A/B 写入；旧 PID 20644 仍为 UE（只读复核 56:05），不声称已解决其退出问题或确定根因。


## 主人选择恢复验收后的状态

归档手动路径交付后，用户明确选择：本任务切为 Ask 后继续 A/B 真实创作；Step 5 保留 Cindy 任务页并等待可信宿主接口；允许本地安装个人正式身份 lumi 做双版本验收。见 [选择回执](step7-evidence/remaining-directions-20261102.json)。

正式 0.4.0 已经 Forge 首次安装为 _root__lumi / namespace=null；与 _root__lumi-dev 同时启用。两者真实 runtime_status 分别返回 production / bbf23278… / c9065d2e… 和 development / d3452b2a… / b74ab5a4…。现有 production 服务 running=true，release 后仍 true；开发专用服务为 false。没有停止默认/正式/Codex 服务，未将新 CLI 安装说成已有 Go 后台已切换，也未读取现有生产作品。见 [正式安装](step7-evidence/production-install-20261102.json) 与 [并存诊断](step7-evidence/coexistence-diagnostics-20261102.json)。

临时验收探针也经 Forge 原位更新到同一 d3452b2a… 产品构建；仅凭原 Host 回执恢复 A，runtime_status 验证新 CLI 后再次 release，未写章节、页面或图片。见 [探针更新](step7-evidence/probe-update-20261102.json)。

当时本任务实际 permission.mode 仍为 auto。已核对 helper/control、自我管理索引及 set_session_runtime 的完整 schema；后者只支持模型/引擎/推理设置，没有权限字段。Host 支持界面中途切换，不等于 Agent 有对应修改工具。主人随后报告已切为 Ask，并再次确认当前任务在 MacBook Pro 的工具栏显示 Ask；公开 runtime 仍连续返回 auto/generation=0。按新的继续指令经正常 ghost_call 入口调用验收探针尝试一次章节保存，仍被拒绝；重读确认无落盘，A/B 再次 release。详见 [权限不一致诊断](step6-permission-mismatch.md) 和 [完整回执](step6-evidence/ask-runtime-mismatch-20261102.json)。不再要求主人重复许可或重复切换。A 临时任务自己的 permission_mode=ask 不是本任务审批模式已切换的证明。

## 正常 lumi-dev 作品任务章节保存

主人随后直接在原 A 作品任务验证正常 lumi-dev 入口，并提供《章节保存验证报告.md》。已核对唯一一次 chapters.write 的完整成功回执：expected_revision=0，返回 revision=1、version_no=1、char_count=875，时间为 2026-11-02T04:22:14.563237Z。目标任务报告重读全文逐字一致，hash 为 e01ce56fd5ca340bb75ea5af8de7754a96389ff56feaad8bab2ad7bcf79dfd98。见 [正常保存证据](step6-evidence/normal-task-chapter-save-20261102.json)。

本次仅测章节保存，未分页、生图、导出或再次写入。报告核实非只读业务权限，未核实宿主 Ask/Auto/Full Access 档位。正常作品任务已可保存章节，不再要求先修改 Cindy 或提供其源码；此前探针与转交拒绝的根因仍未知。当前 A 正文 revision=1，后续须重读，不沿用旧的 revision=0 或旧未保存草稿。

## 同一正常任务的页面验收已派发

用户要求继续 Step 4–7 后，向原 A 任务派发了后续三页保存和重读验收，不重放前次章节保存。send_to_session 本次成功，回执为 ok=true、wake_kind=already-active、实际目标仍为 a0964e96-ba88-4c58-af32-4c0d356b7530；目标已开始本轮并显示 waiting-permission / active=true。当前等待主人处理目标任务的原生权限确认，页面创建结果尚未返回。见 [派发回执](step6-evidence/normal-task-pages-dispatch-20261102.json)。

范围仅为按现行 render_prompt 生成三页正文/完整画面描述，逐页读取最新父集合 state.revision、创建、get 比对并生成《页面保存验证报告.md》。不生图、不导出，不更改权限、模型或目录；已有内容、拒绝或未知结果先重读后停止，不盲目重试。当前成功转交不代表页面保存已通过，也不证明此前拒绝的具体原因已经确定。

## 正常作品任务三页保存结果

目标任务随后完成并生成《页面保存验证报告.md》。本源码任务核对报告内3次完整创建回执、最终 book_outline、文本结构与逐页比较结果。每页仅创建一次，使用父集合 expected_revision=0、1、2，集合最终为 revision=3；新页 page.revision 均为0，画面描述版本均为1。三个页面 UUID 依次为 01a19a9f-e071-7ce8-9e8b-175e4009c456、01a19aa0-32aa-76c6-a920-e23e33f14694、01a19aa0-7ae4-7cc1-b5ad-1480534da4d3。

报告记录页面正文与完整画面描述共六项逐字重读比较全部为 true；最终列表再次比较通过。原章节保持 revision=1、875字与原 hash。三页均有文字/画面描述且无图片；runtime_release 返回 released=true/shared_retained。归一化证据见 [三页保存回执](step6-evidence/normal-task-pages-save-20261102.json)。父任务只读取报告，没有重复创建页面或直接读取数据库。

随后派发第1页真实 Core 生图→导入→预览验收时，send_to_session 在 MCP 层返回 user rejected MCP tool call，未提供原因；重查目标本轮已结束、queued_count=0，最新结果仍为页面报告。未重发转交，未提交本次 Core 请求，不能记成生图或导入失败。见 [本次转交拒绝](step6-evidence/normal-task-first-image-handoff-20261102.json)。当时需主人在原作品任务直接发送[修正后的验收指令](normal-task-image-next.md)；后续已成功执行，见下节，无需重复派发。

## 第1页成功复验与开发后台重连

主人直接运行后的首试停在执行前缺参校验，记录见 [失败证据](step6-evidence/normal-task-first-image-validation-20261102.json)。修正提示词片段返回和组装说明、补充Go/真实CLI测试后，开发包经Forge更新到 `29401ca7cbd2cff3f21d5d2f7c3e2237f31990bfd99ac1e005dfd5faa068c1a8`，CLI SHA256=`d26c4636d6b0116520a3a2baef7cf203676ec6a91d3bc06ea4c58f9aef395c47`。当次安装、21条目内容/权限比较及旧专用后台停止记录见 [修正检查点](step6-evidence/image-prompt-correction-20261102.json)，该检查点当时尚未启动新后台。

随后主人已在原任务直接发起复验，无需再次派发第1页生成。新版正常任务三次提示词渲染、一次Core生成和一次顶层receive_media全部成功；1473字符提示词与导入元数据一致。实际 `openai/gpt-image-2` / `openai`，PNG2861885字节、1448×1086、keep，Asset/variant与当前页及图片重读一致，revision 0 → 1、图片版本1，正文/分镜不变。连接已释放、共享服务保留，第2/3页仍无图。完整范围见 [成功证据](step6-evidence/normal-task-first-image-success-20261102.json)。

这证明修正后的CLI已在正常作品任务生效，不代表全部共享客户端升级矩阵完成。原图的奶奶露脸/背景灯差异随后已在一次参考编辑中验证修正，见下节。四视图与手机实际下载未验；受控预览寄存和实际编辑分别记账。

## 参考编辑、候选选择和真实PDF/ZIP已通过

用户直接在原任务执行后，新增《参考图编辑候选与导出验收报告.md》。本次受控Lumi预览确实进入Core image.edit的image字段；request、receive_media、select_image均各1次，poll=0。候选2导入后，以expected_revision=2选回原版variant，page.revision 1→2→3，正文与分镜未变。两版均保留，最终原图用于导出。

PDF/ZIP各调用1次，父任务完整读取已交付副本，摘要/大小与原回执一致。PDF361740字节、1张A4页面，嵌入缩放压缩JPEG；ZIP2864730字节、CRC有效，book.md/book.json包含875字章节和三页完整正文/分镜，PNG摘要与原图首次导入一致。missing_image_count=2，未补图，未宣称完整三页插图或手机已收到。见 [完整成功记录](step6-evidence/normal-task-reference-export-success-20261102.json)。

## 原生停用验证

重新发现Host工具与插件管理帮助后，确认当前有Forge安装/更新，没有停用、启用或卸载调用接口；浏览器自动化只控制网页，不控制Cindy原生设置。主人在Mac的插件详情页关闭Lumi（开发版）后，实时ghost_info返回GHOST_ASLEEP。正式lumi runtime_status仍ready、production、原build_id/CLI指纹，server_running=true；两个既有导出文件SHA256/大小未变。见 [停用检查点](step6-evidence/plugin-disable-20261102.json)。主人已在同一详情页重新启用，lumi-dev实时发现和运行诊断恢复，新开发构建指纹未变；正式Lumi亦保持原构建与ready状态。见 [重新启用回执](step6-evidence/plugin-reenable-20261102.json)。向原任务派发本轮项目/双图读取的回执为queued，未重复派发；任务现已结束并保存恢复报告，章节及三页文字/分镜、所有版本和当前原图选择一致，原图与候选2均可读取。见 [恢复证据](step6-evidence/plugin-project-recovery-20261102.json)。后续原生卸载、同包重装及项目保留已通过，见下节。不由停用状态推断后台已退出、项目已重开或卸载通过。

## 原生卸载与同包重装

在启用后的作品与双图恢复报告核对通过、任务结束/release后，主人已在原生详情页只卸载lumi-dev。实时发现返回GHOST_NOT_FOUND，正式版诊断仍ready/原构建，PDF/ZIP大小与摘要不变。专用development服务仍运行且无打开项目，符合shared_retained；没有停止或强杀服务。见 [卸载证据](step6-evidence/plugin-uninstall-20261102.json)。

沿用开发安装委托，从同一未再编辑作者目录Forge重装，回执installed、enabled=true、instanceId=_root__lumi-dev、namespace=null。21条目内容及权限与审阅包一致，开发runtime ready，build_id仍29401ca7…；原生status的PID/nonce与卸载后观察相同。见 [安装回执](step6-evidence/plugin-reinstall-20261102.json) 和 [包比较](step6-evidence/lifecycle-reinstalled-package-20261102.json)。重装后原作品/双图读取已成功派发，但首次ghost_info返回user rejected MCP tool call后停止；业务读取与runtime_release均为0。见 [重装恢复拒绝](step6-evidence/plugin-reinstall-recovery-refusal-20261102.json)。该拒绝记录保留。主人随后已在原任务直接发送新请求，另存《插件重装恢复复验报告.md》；本轮正常目录发现与项目重开成功，875字章节、三页完整正文/分镜、原图和候选2的受控预览均核对通过。当前原图仍选中，页面revision保持[3,0,0]，release返回true/shared_retained。父任务核对公开报告并独立验证七份旧报告hash未变。见 [重装恢复成功证据](step6-evidence/plugin-reinstall-recovery-success-20261102.json)。项目保留以本轮业务读取为依据，不由安装或runtime ready推断。

## 新增权限更新观察

此前两次Agent更新均返回updated，原包恢复属于历史检查点，不作拒绝证据。随后原生首次安装拒绝与保留旧包的更新拒绝已分别通过；本轮原开发构建和CLI仍可调用、测试构建未生效，正式版及导出/报告摘要不变，连接已释放，没有重装。准备包、历史回执和本轮完整结果见 [新增权限更新记录](step6-permission-update.md)。

## 本轮剩余验收条件与用户跳过项

原正常作品的系统项目锁已实际取得并立即释放，锁检查自身未改变锁文件inode/内容/mtime或完整project.sqlite摘要；恢复读取并释放后再次取得独占锁，见 [项目锁检查](step6-evidence/project-lock-release-20261102.json)。用户明确跳过原生桌面重开，未计为通过；用户当前无可用Codex开发客户端，真实共享项仍未验证。旧诊断PID20644仍为UE，退出未确认。正常任务A在读取基线标记后收到优雅停止请求，目标轮次结束；另行恢复的875字章节/三页完整正文与分镜/双图/大纲均匹配，连接已释放，原共享服务保留且无打开项目。见 [读取阶段停止及恢复证据](step6-evidence/normal-task-read-cancel-recovery-success-20261102.json)，不扩大为在途写入/RPC/Core请求被强制中断。完整记录与宿主前置见 [剩余验收](step6-remaining.md)。
