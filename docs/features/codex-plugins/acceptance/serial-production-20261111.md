# 按章、按场串行制作验收（2026-11-11）

共享源为 `plugin-skills/create-picture-book`，使用 `node scripts/build-skills.mjs` 同步 Codex 和 Claude Code 发布技能；开发插件继续复用发布技能。不修改 Lumi Web、数据库结构或服务端接口。

## 已实现行为

| 环节 | Codex | Claude Code |
| --- | --- | --- |
| 多章 | 同一作品目录的独立章节 thread，按章启动并复用已有身份 | 检查实际独立聊天能力；没有时当前聊天按章串行，Agent 不冒称聊天 |
| 章内 | 一个设定 subagent、每场独立 subagent，上一场通过后再开始下一场 | 同样的角色与串行门槛；没有委派能力时如实降级 |
| 页面 | prepare_page_image → 宿主生图 → import_image → 看当前采用图 → record_image_review | generate_page_image 自动挂载 → 看当前采用图 → record_image_review |
| 重试 | 先查已导入版本，不重复生成／导入；过期凭据重新准备 | 调用前保存全部原参数与幂等键，未决请求原样重放 |
| 设定返修 | 暂停原场景，原设定代理修正；按剧情适用范围复核已完成页，再恢复原场景 | 同样的影响复核；设定图仍用素材导入或 Lumi Web 生成 |

`user-data/lumi-production/` 保存聊天、分场、版本证据与交接；采用版本、revision 和页面审阅事实始终以 Lumi 重读结果为准。文本草稿不写 Lumi，设定代理不写页面，场景代理不改原文、分镜或设定。子任务缺宿主能力时只代执行所缺步骤，未看图不能写 passed。

## 自动化与前向检查

| 检查 | 结果与证明范围 |
| --- | --- |
| `node scripts/build-skills.mjs --check` | 通过；两个输出与共享源一致 |
| `node --test scripts/test-build-skills.mjs scripts/test-dev-codex.mjs scripts/test-dev-claude.mjs scripts/test-package-codex.mjs` | 16 项通过；宿主隔离、开发生成、刷新、保护及发布包资源可达 |
| 两端技能 `quick_validate.py` | 通过；技能元数据合法 |
| `go test ./internal/textmcp ./cmd/lumi ./internal/jobqueue` | 通过；涉及监听端口的测试在允许 loopback 的环境执行；沙箱内初次执行因 bind 被拒失败，不是功能失败 |
| `go test ./internal/textmcp -run TestSerialProductionSettingRepairAndImportedPageResume -count=1` | 通过；新增两章、三场 MCP 协议场景，见下文 |
| `serial_production_fixture.py --binary …`、`--verify …` | 通过；真实 stdio→CLI→SQLite 跨连接重读，采用版本不变，verify 无生成／导入调用 |
| Claude Code 2.1.322 `plugin validate`，lumi 与 lumi-dev 源模板 | 运行时 manifest 校验通过；目录提交检查另提示缺许可证，未选择许可证；dev 差异模板单独校验提示图标缺失，开发合并测试已验证共享图标随包复制 |
| 独立 Codex subagent 前向检查 | 受限模拟两端恢复轨迹，覆盖扩章复用、pending 未回报、设定返修与后期换装区分；据此修复 Claude 文档旧导入路径，以及缺 CLI／本地读图能力的降级入口 |

新增协议用例的第一章有两场，第二章一场。第一场中第二页在导入后、审阅前中断；重读得到同一采用版本和 pending。设定修正后旧 setting_reviews 与 preparation 被拒；已经 passed 的第一页不会自动失效，由影响队列显式复核、修图并保存新观察。第一章完成后，第二章准备结果的 sources 必须指向最终设定版本。

Go 场景由测试代码驱动，图像与视觉结论使用明确替身；它证明工具契约和持久化，不证明宿主会正确建聊、委派或判断图片质量。独立前向检查也是模拟，不能替代真实宿主验收。

## 真实宿主结果

### Codex：通过

用户通过 `codex app <作品目录>` 打开隔离目录后，`list_projects` 已核实登记。章节 thread 均用 `create_thread` 的同一 projectId 与 local 环境创建，无 worktree：

| 章节 | thread | 场景代理 | 最终结果 |
| --- | --- | --- | --- |
| 第 10 章 | `01a1c952-19ca-7c92-a810-78e28afb23a1` | `/root/chapter10_scene1` 码头两页；`/root/chapter10_scene2` 灯塔一页 | 3/3 passed |
| 第 11 章 | `01a1c985-c7fc-7a93-92bb-e262f7fe888f` | `/root/chapter11_scene1` 海上一页 | 1/1 passed |

两章各有一个设定代理：`/root/chapter10_settings`、`/root/chapter11_settings`。章节主聊天先读保存分镜划场，再委派设定与场景，子任务使用不继承历史的输入。第 10 章完成时间早于第 11 章启动时间；总聊天在 Lumi 独立核实前章三页通过后才创建后章，后章接收精简章节摘要、最终设定身份与连续性。

本次使用项目级透明 stdio 记录器连接真实 Lumi CLI/MCP，章节聊天读取本次同步后的技能路径。实际使用宿主 imagegen；所有采用图经 Lumi 导入并由场景代理实际看图，之后才保存 passed，章节主聊天再核实。首个人物设定请求被宿主拒绝且未产出，记录失败后在同一宿主工具中重试成功，未把失败当作已完成。

| 范围 | 最终采用版本／页面 revision | prepare／import 次数 | 审阅 |
| --- | --- | --- | --- |
| 第 10 章码头页 1 | v3 / r4 | 2 / 2（含银钥匙更正） | passed |
| 第 10 章码头页 2 | v2 / r3 | 1 / 1 | passed |
| 第 10 章灯塔页 3 | v1 / r2 | 1 / 1 | passed |
| 第 11 章海上页 1 | v1 / r2 | 1 / 1 | passed |

最终通过以下实际场景：

- **场中设定更正**：第一场页 1 通过后暂停，将铜钥匙更正为银色钥匙。唤回原设定代理，保留同一人物设定 UUID，实际看图后采用 v3/r5。原第一场代理复核已完成页 1，保存具体问题并修正，通过后才处理页 2；未重启全场或提前启动灯塔场。
- **导入后中断恢复**：页 2 真实图导入后仅保存回执并结束该轮，Lumi 为 pending；下一轮复用同一章节 thread 和原场景代理，先看已有采用图再保存 passed。总聊天独立核实恢复前后 variant 均为 `01a1c976-6ab5-7603-87a3-46cf2745e948`，新增 prepare／import 为 0／0，场景记录中的新增宿主生图为 0。本次用受控检查点结束与恢复会话轮次，未模拟操作系统崩溃。
- **页、场、章顺序**：请求日志显示页 1 最终通过早于页 2 准备，页 2 通过早于灯塔页准备，灯塔页通过后才进入第 11 章。每个最终采用图均有导入之后、passed 之前的实际 `read_image`。
- **跨章连续性**：第 11 章准备请求中的 setting_reviews 指向最终人物 variant `01a1c96c-9ea3-7da0-9cf2-677de3d57e43`（v3/r5），未沿用旧铜色 v2。保留红外套、右手银钥匙与蓝船；海上页中蓝船继续离岸，白塔红顶灯塔退至后方远景。后章设定代理实际核对后复用合格参考，没有重复画人物。

总聊天最终通过新的 Lumi MCP 连接核实四页全部 passed、无待审／待修／缺图。原始 stdio 日志、检查点快照、角色交接及 `host-audit.json` 留在隔离夹具根目录（`lumi-serial-host-o_5s770t`），仓库只保存本摘要。审计同时断言同目录、两章顺序、三场独立身份、原角色复用、最终版本交接、实际读图时序和页 2 恢复计数。

### Claude Code：代码与自动化通过，真实宿主留后续

已同步 Claude 插件并运行相关自动化与 manifest 校验。检测到本机 CLI 2.1.322，其帮助包含后台独立会话与恢复入口，但当前未登录；技能要求检查当前真正可调用的聊天／CLI 能力，有能力则按章建立可恢复会话，没有则当前聊天按章串行，不能把 Agent 当成章节聊天。

用户明确将真实 Claude 宿主验收移出本 goal，留待其在 Claude 中执行。本次未创建真实 Claude 章节会话，未调用真实图片服务商；服务端自动挂载、原参数与幂等键重放、设定素材／Web 来源的结论来自源码契约、自动化与受限前向检查，不能宣称 Claude 真实制作闭环已通过。

## 复跑与 Claude 后续验收步骤

以下流程用于复跑及后续 Claude 真实宿主验收；本次 Codex 已完成上述场景。

1. 用隔离作品与本次技能发起两章请求，确保第 10 章至少两场。确认创建／复用两章身份，同目录，无 worktree；后章在前章完成交接后才启动。
2. 设定代理实际看图保存证据；第一场逐页生成、挂载、看实际采用图、写核对。注入设定更正，确认暂停本场、唤回原设定代理、显式复核影响页并恢复原场景，不提前启动第二场。
3. 在一页挂载后、审阅／回报前中断。恢复先查 Lumi，pending 先看图，同一页不得新增生成任务；Claude 若任务未决，用完整原参数与原 key 取回。
4. 第一章由章节主聊天从 Lumi 核实后只向总聊天回章节摘要；第二章接收最终采用设定及跨章连续性。记录真实 thread／agent 身份、工具调用顺序、当前图片版本与审阅回执，不以文字承诺代替事实。
