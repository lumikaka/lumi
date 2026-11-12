# Codex 章内场景并行：实现与独立验收（2026-11-12）

## 结论

已在 Codex 插件加入用户明确选择的“独立场景最多两场并行”，**默认仍串行，多章仍串行**。完成三场四页的串行与显式并行真实对照；两组八页均实际宿主生图、立即挂载、查看 Lumi 当前采用图并保存 passed，最终又由本执行聊天独立读回和逐页看图，正文返修均为 0。场景写入所有权、最多两场、首图门槛及 B→C 依赖顺序的正常路径验收通过。

**严格受控的性能验收未通过。** 本轮并行组全章最后 passed 较串行少 228.195 秒（20.23%），但宿主 imagegen 请求重叠为 **0 秒**。串行 A2 多了 222 字的动态连续性补充，并行 A2 没有；生图底层模型/版本未公开，协调者也意外看到过串行文字摘要。不能据此声称并行生图带来性能收益或稳定节省比例。可确认的是本次真实流程完成更快，且可见累计 token 增加；保留默认串行和显式选项，不宣称性能优化验收成功。

## 前置 wait 与修改范围

前置聊天 `01a1cd55-a0dd-7843-9ad4-3c9ac7ec5097`（local）于完成后才交接；本执行聊天一直按外层首行 55000 ms、内层 45000 ms、原 cursor 续等，没有提前修改文件或启动对照。已读取其最终结论和真实报告 `/Users/qingyang/mine-work/lumi/codex-plugins/lumi/reports/wait-threads-acceptance.md`，报告副本保存在本证据目录 `wait-prerequisite.md`：原“雨站的钥匙”“灯亮以后”两章六页分镜已实际复跑、文字验收通过；额外章节等待 16→0，总聊天模型调用 61→54，全体输入 9,518,366→10,650,611（增加 11.90%）。该前置不包含图片验收，也未证明节省总 token。

前置完整快照：`/private/tmp/lumi-storyboard-host-pp8f129b/project/acceptance-skill/`；其 SKILL.md / serial-production.md 哈希分别为 `c19eebe51df6e5d813dd414b74361836536621b77f52327b4eab169edc9e2747` / `67e8006ee97ddc86e016e6628f7f198cf94ed347eac6f0ba10f50c7297e15b57`。本次扩展场景规则后文件整体哈希变化，但“章节等待示例”段逐字节保持不变，SHA-256 为 `539c3eacae9646168ff6b8ccd2369dcf70dd443b26de733a82c44b44140dc678`。前置报告也逐字节未改。

本次修改：

- `skills/create-picture-book/SKILL.md`：默认串行、显式最多两场例外、依赖判断、唯一页写入者、首图/用户节点、设定返修与恢复边界。
- `skills/create-picture-book/references/serial-production.md`：统一原“上一场通过才启动下一场”规则，补全选择记录、起始状态与依赖、活动名额、B→C 核实交接、全部受影响场景暂停、进行中请求与恢复导入核查、原设定代理返修、实际旧页复核、冲突/过期 preparation 处理、唯一所有者恢复。
- 本报告。前置新增的审计脚本和报告均保留。

只改 `codex-plugins/lumi/`；临时作品、脚本和证据均在 `/private/tmp`。未改 Cindy、Claude Code、共享 plugin-skills、后端、桌面 Web、其他插件或正在制作的“火星公主β”，未运行 Cargo/Rust 命令。没有共享 Web 变更。

使用方式：用户明确说“本章使用独立场景最多两场并行”。章节主聊天记录选择依据，先完成分镜和设定，再判断依赖；起始状态明确、无未决成图连续性依赖才可同时活动。依赖不明串行；有站位、伤势、持物或动作承接的后场，必须等依赖场当前采用版本 passed、主聊天从 Lumi 核实并保存交接后启动。每场逐页闭环，不开并行画页代理，不扩展到并行章节。这是插件调度选择，不是新增后端配置。

## 实际环境、加载版本与固定输入

证据根目录：`/private/tmp/lumi-scene-parallel-bctc71zw/`。隔离作品为其 `project/`，project_uuid 为 `01a1cd7a-f495-73f7-b2e3-bfabb082a7a0`。在同一个临时作品内用两个章 UUID 保存同一逻辑章的实验副本，串行组完成后才启动并行组，以共享完全相同的设定 UUID、采用版本和 revision；这是两个实验副本，不是并行制作多章。

原有内建 MCP 绑定源码目录，project_overview 返回未初始化；没有在源码目录初始化作品。实际作品事实通过临时记录桥接器转交独立 stdio MCP 会话：cwd 为上述临时作品，`mcp --host codex --environment development --data-dir /private/tmp/lumi-scene-parallel-bctc71zw/data`。宿主仍为本执行聊天及其 subagent，使用 Codex 原生 image_gen 和 view_image；没有另建 App 创作聊天或安装/替换用户插件缓存。验收范围是明确加载的仓库技能快照、真实宿主图像工具及真实 Lumi CLI/MCP 链路，不能代表既有 App 聊天自动升级。

客户端 `client.py` 只通过临时文件队列提交 MCP 请求，桥接器保留 request/result 日志并将 read_image 的 MCP 图片内容复制到临时 previews 后交 view_image。没有直接访问 project.sqlite、.lumi/ 或 assets/，没有 provider pipeline、付费 API 或 mock 图片。启动本机临时服务的沙箱限制经正常自动审批放行，未绕过 App 信任或签名设置。

| 项目 | 实际值 |
| --- | --- |
| Codex Desktop rollout CLI | 0.167.0-alpha.3.2 |
| 创作协调/场景模型 | 所有八个生产会话均 gpt-6.1-sol / xhigh（无覆盖） |
| Lumi CLI | /Users/qingyang/mine-work/lumi/build/codex-dev/lumi_dev |
| CLI SHA-256 | b14199832b05579bbd6802bb5e9ba77a533bba8ebe1336319c6151278744cd90 |
| MCP serverInfo | lumi-text / 1.0.0 |
| imagegen 底层模型/版本 | 宿主不暴露，无法证明严格锁版 |
| imagegen 工具契约 | 同一工具，完整描述冻结于 imagegen-tool-contract.json |
| Git HEAD | 44bacb4efb2798088f589788a6066eec13e57b5f |

两组协调主代理与六个场景代理都实际读取同一 `/private/tmp/lumi-scene-parallel-bctc71zw/acceptance-skill/` 快照；冻结 rollout 的读取命令及各代理指纹文件可复核。单章场景使用 collaboration.wait_agent/followup_task；两组都加载已修正的章节 wait 指引，但本轮单章内不需调用 wait_threads。实际章节 wait 的动态验证属于上述前置，不能把本轮加载等同于再次复跑章节等待。

| 快照文件 | SHA-256 |
| --- | --- |
| SKILL.md | 9e4b1d73ea1ec7cabf33681036cb35b5a8b9afbd7e14b4b9ef559b218cd94634 |
| references/serial-production.md | d3f78c4b7c59ad5be35227f9fc85512ba6d51b33522355184ffb8625d8c5be1b |
| references/page-images.md | ff581679e604af5cd703493897989c07eb265bd5f6bfacc238fc87ee075841a8 |

原文、规格和四页分镜保存于 `fixed-input.json`；独立逐页比较见 `input-equivalence.json`、两组 `independent-final-facts.json`。原文为：

> 第一天清晨，阿禾在家练习照明：他左手持折叠地图，右手提红灯笼站在木桌左侧。他把红灯笼放到桌上，左手仍拿地图。第二天傍晚，他重新右手提红灯笼、左手持折叠地图，在石桥左端朝右站定。随后从这个实际站姿向右迈一步，仍未走到桥中央。四页全程保持黄色雨衣、蓝帽、右腕白色绷带；无文字。第一天练习不决定第二天桥边的站位或持物起始状态。

规格为 wordless_picture_book、1:1、中文项目、平涂水粉/细深蓝轮廓/轻纸纹，四页各单幅且无字，不制作封面，不导出。A1/A2：第一天清晨家中提灯→放灯；B1：第二天傍晚重置为桥头站定，起始状态明确、不依赖 A 采用图；C1：承接本组 B 采用图向右迈一步。所有页面初始 revision=0、missing_image，没有预存正文图。用户授权自主完成质量核对，无中途用户审批节点；首张正文仍先通过并核实，B 只准备参考、到首图门槛后才发图。

两组设定采用身份完全一致，生产期间没有修改：

| 设定 | setting_uuid | 采用 variant_uuid | revision |
| --- | --- | --- | --- |
| 阿禾 | 01a1cd7d-168a-7470-a281-e1ced3d8f768 | 01a1cd80-250e-7b85-8c86-3dd850c159ab | 2 |
| 木屋/石桥双区参考 | 01a1cd80-d059-7dac-819b-e9b48bbfec4c | 01a1cd81-f1ad-78b6-8338-02411ac29b93 | 1 |

参考顺序均为角色→场景，style_digest 为 `e16ad57207fe3ec8659d83b671f4447ee79a1515d08a11590578e8e6f0c0dc42`；共同设定真实制作 3 次生图（角色首版画风偏水彩，原设定代理在同 UUID 修成 v2；场景 v1）。共同准备及其返修一致排除于两组制作耗时，实际用量单列一次。两组都经真实 setting_reviews/prepare_page_image，导入 prompt 与完整 signed preparation 已逐项核对一致。

输入控制的具体限制：`prompt-equivalence.json` 记录实际导入提示词指纹。A1/B1 提示词只因实验副本的“当前 Section”UUID 不同，归一化该身份后相同；A2 除 UUID 外，串行代理额外添加 222 字上一页实际连续性方向，并行没有；C 两组按各自 B 实际图生成交接，补充方向分别为 421/356 字，这是预期的闭环差异。A2 的可避免差异是本轮控制缺项，不能称完全相同生成输入的严格基准。

## 全章时间与真实请求身份

下表均为 2026-11-12 UTC，北京时间加 8 小时。统一起点取各组协调主代理真实 session_meta 创建时间，包含初次读指引、协调、准备、全部等待/技术失败/返修。终点取最后一个最终采用版本 passed 的 MCP 成功结果时间；这表示事务已完成的回执观测时间，实际提交位于对应 request/result 极短区间内，不伪造数据库提交时刻。另列主代理从 Lumi 重读并完成整章核实的时刻，避免把尚未核实当交付。

| 指标 | 串行 | 显式两场 |
| --- | --- | --- |
| 制作起点 | 02:08:30.668 | 02:30:56.766 |
| 最后当前采用 passed 回执 | 02:27:18.718888 | 02:45:56.621892 |
| 全章四页均 passed 耗时 | 18分48.051秒 | 14分59.856秒 |
| 主代理核实完成 | 02:29:20.879480 | 02:47:09.414693 |
| 到主代理核实完成耗时 | 20分50.211秒 | 16分12.649秒 |
| 已生成/当前采用passed | 4/4 | 4/4 |
| 正文返修页数/次数 | 0/0 | 0/0 |
| 最高同时占用场景 | 1 | 2（B准备后暂停也占名额） |
| 宿主请求实际重叠 | 0 秒 | 0 秒 |

单次最后 passed 耗时减少 228.195 秒（20.2291%）。没有删除技术异常、B 首图等待或恢复校准耗时，没有只取某张图速度。单次有先后顺序、代理执行路径/上下文/缓存差异，无法把全部时间差分摊到场景调度、准备重叠或模型随机性。

每次宿主 imagegen 在同一 exec 中紧邻 await 前后记录毫秒 UTC；用真实输出路径关联 Lumi import，再从冻结 rollout 的 Extension 读取 generationId、Extension id 和外层 call_id。自建 request_key 不充当宿主身份。下表身份均来自原始宿主记录，不取重建图片回执。

| 组 | 页 | 真实调用开始 | 返回 | 秒 | 宿主 generationId | 宿主 Extension id |
| --- | --- | --- | --- | --- | --- | --- |
| 串行 | A1 | 02:12:45.521 | 02:13:16.144 | 30.623 | 1d8e1e8f-e20f-4577-90e3-cb7a543f83e3 | exec-50977351-4d5d-4668-b6a6-e9f068177059 |
| 串行 | A2 | 02:14:52.867 | 02:15:36.611 | 43.744 | 261afd9b-236b-4220-9d3f-9dceaec65864 | exec-5e868991-53cb-43a3-9e7d-79820980c1d5 |
| 串行 | B1 | 02:20:22.611 | 02:20:54.022 | 31.411 | a24aa130-7ec5-499a-b5ec-0317a22fbda2 | exec-f64d9cc1-96f6-42d1-8abc-c85d0f360b54 |
| 串行 | C1 | 02:25:33.385 | 02:26:01.372 | 27.987 | 6368f7aa-cbd4-4253-b5e5-e79cbfe14515 | exec-370eb884-2078-4d39-b4ef-d7fb931a14e1 |
| 两场 | A1 | 02:34:37.344 | 02:35:05.454 | 28.110 | e04a6239-7dbd-4ea9-82ef-635d34bada52 | exec-3b8a6a5a-b1a7-420a-b183-6c5c765556b6 |
| 两场 | A2 | 02:37:18.292 | 02:37:42.571 | 24.279 | d653ba6f-17fb-4153-af9b-ee920302eaef | exec-39c92aea-4f92-4e9e-af2b-1040bb637725 |
| 两场 | B1 | 02:37:57.948 | 02:38:31.659 | 33.711 | e1db509b-32d7-4ca6-a808-e951f15ac563 | exec-40c1fde0-d639-46d6-a446-54b8dc4d27fd |
| 两场 | C1 | 02:44:32.532 | 02:45:08.204 | 35.672 | d5666452-c3d5-4067-b7e6-85e43a4b00fb | exec-b80d307d-baf2-43f7-912c-49a7969a98c9 |

完整 call_id、线程、原始行号、实际输出路径与 MCP 导入 request_id 在 `independent-evidence-verification.json` 和两组 `usage-audit.json.imagegen_host_evidence`。并行 A2 返回 `02:37:42.571Z`，B 发起 `02:37:57.948Z`，间隔 **15.377 秒**；请求区间无交集。并行发生在场景代理的读取/准备/等待/审阅活动，不是宿主生图请求同时在途。没有为凑重叠重生 passed 页。

### B→C 依赖顺序（UTC）

| 证据 | 串行 | 两场 |
| --- | --- | --- |
| B 当前采用 passed 结果 | 02:21:53.702361 | 02:40:44.718084 |
| 主代理实看核实/保存交接 | 02:23:03.813109 | 02:41:58.058330 |
| C 宿主代理创建 | 02:23:25.453 | 02:42:07.270 |
| C 首次 imagegen | 02:25:33.385 | 02:44:32.532 |

两组顺序均严格成立。串行 B 客户端接收时间另为 `02:21:53.720334Z`，比原 MCP result `02:21:53.702361Z` 晚约 18 ms；报告表统一用 MCP result，不混用接收时间。C 创建前核实的版本与最终采用一致；没有因 B 代理口头“通过”提前启动。

## 全体可见 token 与统计口径

“创作执行树”含每组一个章节协调主代理及 A/B/C 全部场景代理，包含 B 的 READY 与恢复轮次、所有核实和最终回复，取各自完成后的最后累计计数，不叠加重复 token_count。输入含缓存，未缓存=输入−缓存；输出已含 reasoning，不重复加。模型调用计数是去重的成功累计用量更新，不代表没有上报用量的失败模型尝试。全部八个生产会话有真实用量，没有把缺失写成 0。

外层本执行聊天承担实验观察与独立看图，按两组 observer-start/end 累计计数差另列，并给出包含它的总计，防止漏掉协调/观察成本。外层已载入大量前置等待/实现历史，且串行期间编写了部分审计工具，不能拿这部分输入差推断调度节省。它的冻结前缀、窗口和哈希在 observer-freeze.json。实现、前置等待和共同准备在两个窗口前/外另计，不混入某组；共同设定及规则审阅仅实际发生一次，不虚构两次执行。用量统计到组内代理最终收尾，耗时主指标按用户要求止于最后 passed，二者都是两组相同口径。

| 范围 | 输入含缓存 | 缓存输入 | 未缓存输入 | 输出 | 成功用量更新 |
| --- | --- | --- | --- | --- | --- |
| 串行：协调主代理+全部场景 | 9,818,170 | 9,488,256 | 329,914 | 44,572 | 121 |
| 两场：协调主代理+全部场景 | 13,200,462 | 12,852,096 | 348,366 | 60,625 | 156 |
| 串行：外层观察窗口 | 8,584,340 | 8,566,912 | 17,428 | 15,633 | 47 |
| 两场：外层观察窗口 | 7,105,746 | 7,092,992 | 12,754 | 16,315 | 34 |
| 串行：上述全体总计 | 18,402,510 | 18,055,168 | 347,342 | 60,205 | 168 |
| 两场：上述全体总计 | 20,306,208 | 19,945,088 | 361,120 | 76,940 | 190 |
| 共同设定+规则审核（实际一次） | 2,664,874 | 2,544,384 | 120,490 | 14,581 | 37 |

创作执行树累计输入增加 **34.45%**，包含外层窗口总计增加 **10.34%**，本轮不支持省 token 的结论。宿主生图服务内部 token/模型/版本没有公开，不计为 0，也不把上述 Codex 文本模型累计当作图像服务的完整账单用量。元数据扫描未发现本生产窗口同目录的额外 host_auxiliary 会话；若宿主存在不落到可见 rollout 的内部成本，本次无法计入。

| 组 | 角色/代理路径 | 真实线程 UUID | 输入 | 缓存 | 未缓存 | 输出 | 调用 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 串行 | /root/serial_run | 01a1cd83-838e-7840-8a73-9301c4ae9619 | 5,182,060 | 5,091,968 | 90,092 | 16,776 | 59 |
| 串行 | /root/serial_run/scene_a | 01a1cd85-504e-7b80-9153-5f4700c409ef | 1,803,616 | 1,731,840 | 71,776 | 10,276 | 24 |
| 串行 | /root/serial_run/scene_b | 01a1cd8b-fc75-79c1-bc82-45c5dd5caf49 | 1,253,467 | 1,158,784 | 94,683 | 8,470 | 17 |
| 串行 | /root/serial_run/scene_c | 01a1cd91-2ae3-7750-9088-0fa40672ff0c | 1,579,027 | 1,505,664 | 73,363 | 9,050 | 21 |
| 两场 | /root/parallel_run | 01a1cd98-0de1-7062-b216-c9ce5e113bc1 | 6,189,012 | 6,102,784 | 86,228 | 28,322 | 72 |
| 两场 | /root/parallel_run/scene_a | 01a1cd99-d3b7-73e3-8ca4-93ee8aa94f8e | 2,479,164 | 2,412,160 | 67,004 | 12,522 | 33 |
| 两场 | /root/parallel_run/scene_b | 01a1cd99-f064-7e93-873f-b9f1801ce1b7 | 2,542,314 | 2,449,792 | 92,522 | 11,382 | 28 |
| 两场 | /root/parallel_run/scene_c | 01a1cda2-48fb-7022-8ea2-513fd92ade89 | 1,989,972 | 1,887,360 | 102,612 | 8,399 | 23 |

## 画面、连续性与写入所有权的独立复核

本执行聊天逐页通过 Lumi read_image 获取当前采用图、实际 view_image 后保存 `serial/independent-observations.json` 与 `parallel/independent-observations.json`。每条绑定 variant_uuid/revision，最终再次与 Lumi 当前摘要核对；两组均最终 v1、页面 revision=1、review_revision=1。没有把宿主原图、导入成功或提示词当成 passed。

- A1：木屋晨光、人物在空桌左侧朝右，黄雨衣、蓝帽、深蓝裤、棕靴、右腕白绷带；解剖右手唯一红灯笼、左手米白折痕地图。
- A2：同一木屋与桌子关系；灯笼已放桌面近身处，底部接触阴影，右手已松开；左手地图不换手。串行右手悬在灯柄上方，并行右手已垂下，均符合固定分镜。两组都无分格、无文字、无多余道具。
- B：桥从左端向右岸延伸，蓝绿河在下，人物在左三分之一、朝右双脚站稳；右手灯笼下垂在大腿外、穗近膝，左手地图在胸前，右腕白绷带清楚。具体前景近栏方柱及腰后远栏关系分别记录本组交接。
- C：分别对照本组实际 B，人物仍在桥左半段、未到中央，已朝右迈一步；仍未越过近栏首柱，右手灯笼和左手地图不换手，服装/帽裤靴/右腕绷带不变，无新增伤势。允许画面机位轻微变化，不宣称像素级相同机位；方向、入口、持物与动作连续。

原始 MCP 日志核对了每次图片写入/审阅的 actor 与分配页：无两个代理写同一页，场景未修改其他场、共享设定、原文、分镜或 setup。初始与最终文字分镜逐字一致；每次导入均能关联一次真实 host 输出，prompt 和整个 preparation 原样，所选设定版本/顺序不变；上一页 passed 后才进行本场下一页。`independent-evidence-verification.json` 包含这些断言结果、页 UUID、审阅观察、回执与请求行号；没有用静态/mock 调度替代真实流程。

## 失败、证据限制与未覆盖分支

1. **没有真实 imagegen 请求重叠**。本轮仅验证最多两场的正常调度与流程时间，不验证图像服务并发性能。A2/B 的实测 15.377 秒空隙完整保留。
2. **严格控制不足**：底层 imagegen 模型/版本不可锁定；A2 动态补充不一致；只有一对、串行先跑，模型执行路径/缓存未受控。20.23% 是本次观测，不是稳定性能承诺，也不能从相同原文/分镜推出生成输入逐字完全相同。
3. **协调者非完全盲化**：并行协调代理一次未限定范围的 collaboration.list_agents 意外看到已完成串行代理的文字摘要/采用身份。事件已向本执行聊天报告并保存 `parallel/contamination-event.json`；A/B 任务此前已冻结且 fork_turns=none，未读串行成图/文件；C 的依赖由本组 Lumi B 当前图独立重读和实看。本执行聊天又独立核对了本组 B/C。不能宣称完全盲化实验，未丢弃该轮或删去耗时。
4. **回执保存技术失败**：并行 A 尝试保存大 Base64 回执遇 OS 参数长度/Node REPL 64KB 限制，B 也触发一次长度保护。均先保留真实 output_hint/路径、完成原图导入，没有重复生图或导入。A 的 full receipt 图像字段从真实返回 PNG 重建，文件已显著标注 reconstructed_from_actual_output=true / not_original_host_receipt；不把它当原始宿主回执。请求身份/时间来自冻结原始 rollout 的 Extension 及调用文本，B/C 原返回元数据也保留。汇总脚本一次 actor 过滤断言失败后修正，无业务写入；所有时间计入。
5. 共享设定首版返修在共同准备阶段完成，不是“两场生图进行中返修”的动态测试。共享返修暂停/取消或等待未决请求、导入恢复、过期 preparation、版本冲突和中断双写防护已补全并由独立规则代理审核，本轮没有主动故障注入，不能声称这些分支已端到端通过。用户节点没有动态触发，首张正文门槛有实际验证。
6. 临时证据目录可被系统清理，报告保留精确路径与冻结哈希；未替用户安装插件缓存。技能来自共享生成源，但任务禁止修改共享源，因此 `node scripts/build-skills.mjs --check` 仍失败并准确报两份 Codex 漂移；未来直接重新生成会覆盖此 Codex 补丁，不能宣称全仓库检查通过。

## 检查与复核入口

- `node --test scripts/test-dev-codex.mjs scripts/test-package-codex.mjs`：5/5 通过。
- `quick_validate.py codex-plugins/lumi/skills/create-picture-book`：通过。独立规则代理 `/root/review_rules` 只读审核默认/例外、A/B/C、首图节点、返修和恢复，没有必须修正的问题；它是静态审核，不算 E2E。
- 前置 wait 示例段和原报告与前置快照逐字节比较通过；新技能快照两份文件与当前源码逐字节一致。
- 真实证据交叉核对脚本 `verify_evidence.py`：原文/分镜未改、设定采用身份/顺序一致、原 prompt/整个 preparation 导入一致、唯一页写入者、场景上限 1/2、B→C 顺序、当前四页 passed、真实请求身份/边界关联均通过。脚本先因 serial B/C 使用 snake_case 时间键而报错，兼容实际字段后重新验证；未改业务事实。
- `git diff --check HEAD`：交付前通过。无共享代码改动，无 Web 回归检查需求；未运行 Cargo/Rust。
- `node scripts/build-skills.mjs --check`：失败，范围仅上述两份 Codex 文件与未修改共享源不一致，未运行会覆盖补丁的生成命令。

原始证据：

- 根目录 `manifest.json`、`fixed-input.json`、`initial-facts.json`、`input-equivalence.json`、`settings.json`、`independent-settings.json`、`skill-fingerprints.json`、`versions.json`、`mcp-server-info.json`、`mcp-tools.json`、`imagegen-tool-contract.json`、`mcp-events.jsonl`。
- 两组 `summary.json`、`timeline.jsonl`、场景任务/scene-A/B/C.json、初始校准、B→C 交接、independent-final-facts.json、independent-observations.json、usage-audit.json、rollouts/、observer-start/end.json、observer-freeze.json、observer-rollout.jsonl。并行另有 merged-timeline.jsonl、host-boundaries.json、contamination-event.json 及明确标注的回执文件。
- `independent-evidence-verification.json`、`prompt-equivalence.json`、`preparation-usage.json`、`preparation-rollouts/`、`implementation-checks.json`。冻结 rollout 各 SHA-256 在对应 usage-audit.json/observer-freeze.json/preparation-usage.json。
- 协调交接在隔离作品 `user-data/lumi-production/<chapter_uuid>/chapter.json`；这不是内部数据库目录。

实际执行命令均在上述临时目录脚本中留存；业务初始化使用 setup.py 经 client.py 调用真实 MCP，宿主创作由两个独立不继承历史的协调代理按同一 run-instructions.md 分别执行，再各派三场代理。审计复算：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 /private/tmp/lumi-scene-parallel-bctc71zw/audit.py serial
PYTHONDONTWRITEBYTECODE=1 python3 /private/tmp/lumi-scene-parallel-bctc71zw/audit.py parallel
PYTHONDONTWRITEBYTECODE=1 python3 /private/tmp/lumi-scene-parallel-bctc71zw/verify_evidence.py
```

audit.py 从真实宿主 live rollout 生成冻结副本，复算时应先保存现有冻结文件，避免后续续聊产生新轮次覆盖本轮。当前所有创作代理已完成，无未决生图、导入或返修。

报告绝对路径：`/Users/qingyang/mine-work/lumi/codex-plugins/lumi/reports/scene-parallel-acceptance.md`。
