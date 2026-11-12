# Codex 章节等待验收（2026-11-12）

## 结论

wait 修改已落地，原两章已在隔离作品实际复跑，等待行为验收**通过**。分镜阶段独立核对状态：`passed`。本轮不包含生图、导出或图片审阅。

只修改 `codex-plugins/lumi/` 的两份技能文件与本目录验收材料。没有修改宿主 wait 工具、共享 `plugin-skills`、Cindy、Claude Code、后端或桌面 Web；未操作正在制作的“火星公主β”。未运行任何 Cargo/Rust 编译、检查或测试。无共享 Web 改动。

**检查限制：`node scripts/build-skills.mjs --check` 未通过。** 两份 Codex 产物已按本任务要求直接修改，共享源受本次范围约束没有同步；检查准确报出这两份漂移。未运行会覆盖补丁的生成命令。后续重新生成技能会丢失本次修改，恢复统一生成须另行同步共享源。本轮不能表述为全部仓库检查通过。

## 修改与实际加载

- `skills/create-picture-book/SKILL.md` 明确等待要求并链接可执行示例。
- `skills/create-picture-book/references/serial-production.md` 给出完整 `functions.exec` 示例：首行 `// @exec: {"yield_time_ms": 55000}`；内层 `timeoutMs: 45000`；保存真实目标与原样 cursor，超时用 `afterCursor` 续等；完成后核实 Lumi，needs_attention/等待用户保留阻塞，不启动后章。异常超过 55 秒时仅取回真实 cell，不重复发起。
- `reports/audit-wait-threads.py` 只读冻结宿主记录，按真实 yielded cell 将额外 `functions.wait` 关联到来源 exec，并读取解析后的 MCP 参数、结果、耗时和 cursor。

本轮快照为 `/private/tmp/lumi-storyboard-host-pp8f129b/project/acceptance-skill/SKILL.md`，由修改后的 Codex 插件直接复制；仅 `references/cli.md` 替换为隔离 CLI 入口。旧 xbakgo1q 快照与新快照除两份等待修改和隔离 CLI 路径外逐字节相同。未刷新用户已安装插件缓存，因此交付结论针对仓库修改及本轮明确加载的快照，不宣称其他已运行聊天已自动升级。

| 文件 | 本次快照 SHA-256 |
| --- | --- |
| SKILL.md | `c19eebe51df6e5d813dd414b74361836536621b77f52327b4eab169edc9e2747` |
| references/serial-production.md | `67e8006ee97ddc86e016e6628f7f198cf94ed347eac6f0ba10f50c7297e15b57` |

全部文件指纹：`/private/tmp/lumi-storyboard-host-pp8f129b/skill-fingerprints.json`。实际读取路径与宿主调用证据见同目录 `skill-load-evidence.json` 和冻结 rollout；若代理读取了其他已安装技能，按该证据披露，不把快照路径当作绝对指令隔离证明。

本轮五个创作会话均有实际读取指定快照 SKILL.md 和 serial-production.md 的记录；审计到的技能读命令没有读取已安装插件缓存。两名分镜代理的完整技能读取也计入真实用量，不假定它们只消耗了任务输入文件的 token。

## 原样例、环境及完成口径

证据链来自原聊天“补齐 Codex 分镜 subagent 流程”（`01a1ca18-4b22-7a20-bd0f-324b05bf23bd`，hostId=`local`）、旧报告 `docs/features/codex-plugins/acceptance/storyboard-subagent-20261111.md`、`scripts/codex-acceptance/storyboard_fixture.py` 与原提示词 `/private/tmp/lumi-storyboard-ab-gapi3cxu/prompt.txt`。本轮 `create_thread` 使用该提示词原文，没有另换样例、mock 或模型覆盖。

| 固定输入 | 原文长度（含换行） | SHA-256 |
| --- | ---: | --- |
| vol01.ch01 雨站的钥匙 | 700 | `442ae1cd60705c0bc431139219b1a9557198110b6d6f66ad0df8786504f75208` |
| vol01.ch02 灯亮以后 | 718 | `a0ab1b1095b6f36d71a6a7182c9a38d9bd316cc6f7b5ac11fb271536531837ea` |

两组均为 `classic_picture_book`、`square`、中文水彩、setup 已定稿、两章原文已保存、零页面、零设定。各章保留 4 句直接引语；旁白允许删减，不得新增事件。共同终点为两章各恰好 3 页正文分镜经章节主聊天逐页核对并通过 Lumi 串行保存、读回一致、划场及跨章交接保存；在设定/场景制作与生图前停止。初始事实在 `initial-facts.json`，终点事实在 `final-facts.json`、`snapshot.json` 和 `quality-check.json`。

实际创作模型均由宿主默认设置产生，详见身份表；原基线为 `gpt-6.1-sol / xhigh`。新旧 Codex Desktop rollout 的 CLI 版本均为 `0.167.0-alpha.3.2`；MCP `serverInfo` 同为 `lumi-text / 1.0.0`。CLI 路径均指向 `/Users/qingyang/mine-work/lumi/build/codex-dev/lumi_dev`，新运行二进制 SHA-256 为 `b14199832b05579bbd6802bb5e9ba77a533bba8ebe1336319c6151278744cd90`。旧验收未冻结二进制哈希，无法补证当时字节完全相同；没有重建二进制。实现时 Git HEAD 为 `44bacb4efb2798088f589788a6066eec13e57b5f`。其余版本见 `versions.json`。

准备时沙箱禁止本机服务监听，按授权提升运行隔离 fixture；`codex app` 因现有 App 签名验证失败而无法添加目录，CUA 也明确禁止操作 Codex App。没有绕过安全设置。用户手动添加目录并回复“已添加验收目录”后，`list_projects` 核对准确路径再建聊。准备耗时不进入创作窗口；失败的 `_iiy9r3m` 初始化未启动创作聊天。

## 真实指标

直接基线是已有专职分镜流程的 `/private/tmp/lumi-storyboard-host-xbakgo1q`，不是更早的 t5xb9tfz。后者只作原样例来源确认；它原本就有显式 55 秒等待、额外 wait 为 0，不能误标成此次修改前的 16 次基线。

| 指标 | 修正前原两章 | 修正后原两章 |
| --- | ---: | ---: |
| 章节等待提前返回造成的额外 functions.wait | 16 | 0 |
| 其他 functions.wait | 0 | 6 |
| 总聊天 wait_threads 次数 | 29 | 26 |
| 总聊天模型调用（有效用量更新去重） | 61 | 54 |
| 全体创作累计输入（含缓存） | 9,518,366 | 10,650,611 |
| 其中缓存输入 | 9,249,280 | 10,242,048 |
| 未缓存输入 | 269,086 | 408,563 |
| 输出（已含 reasoning） | 75,102 | 89,817 |
| 总聊天单次输入峰值 | 94,247 | 111,077 |
| 全体创作模型调用（不含辅助审查） | 139 | 149 |
| 完成耗时 | 2 时 0 分 30.196 秒 | 0 时 28 分 55.612 秒 |

本轮全体创作累计输入增加 1,132,245（11.90%）；总聊天有效模型调用从 61 到 54。额外章节等待是否消除与全流程总 token 是否下降分别判定，不能互相替代。

按职责拆分，总聊天累计输入实际减少 253,429；两个章节聊天累计输入增加 655,282；两个分镜代理累计输入增加 730,392。章节模型调用从 62 到 68，分镜代理从 16 到 27，总聊天减少的调用被其他角色的增量抵消。真实首章逐页核对中发生了原代理返修，且本轮两名分镜代理都实际读取完整技能；这些均计入用量。单次运行无法精确分摊返修、上下文读取、缓存和模型路径变化的各自影响，不能将全体输入增加简单归因于 wait 的文字补丁。

全体创作口径包含总聊天、所有章节聊天及所有分镜 subagent。另将宿主权限辅助审查单列并给出含辅助审查的总计；实现/审计聊天不属于样例创作执行树，和旧验收口径一样不计入创作 token。

| 口径/角色 | 输入（含缓存） | 缓存输入 | 未缓存输入 | 输出 |
| --- | ---: | ---: | ---: | ---: |
| 旧基线 / root | 4,471,584 | 4,396,032 | 75,552 | 16,769 |
| 旧基线 / chapter | 4,419,936 | 4,275,200 | 144,736 | 32,190 |
| 旧基线 / subagent | 626,846 | 578,048 | 48,798 | 26,143 |
| 旧基线 / host_auxiliary | 1,540,995 | 1,404,672 | 136,323 | 3,712 |
| 旧基线 / 含宿主辅助审查总计 | 11,059,361 | 10,653,952 | 405,409 | 78,814 |
| 修正后 / root | 4,218,155 | 4,058,624 | 159,531 | 19,790 |
| 修正后 / chapter | 5,075,218 | 4,898,688 | 176,530 | 37,421 |
| 修正后 / subagent | 1,357,238 | 1,284,736 | 72,502 | 32,606 |
| 修正后 / host_auxiliary | 1,811,449 | 1,655,168 | 156,281 | 3,924 |
| 修正后 / 含宿主辅助审查总计 | 12,462,060 | 11,897,216 | 564,844 | 93,741 |

输入包含缓存；未缓存输入 = 输入 − 缓存输入；output 已含 reasoning，不重复加算。每个会话只取测量窗口末次 `total_token_usage`，不对重复累计值相加。模型调用计数采用非零、不同的累计输入/缓存输入/输出三元组数量，旧基线 62 次用量事件去重后是 61 次；这是宿主可见的成功用量更新口径，未报告用量的失败尝试无法计数。来源类型参见 [OpenAI App Server turn events](https://learn.chatgpt.com/docs/app-server#turn-events)。

耗时从总聊天 `session_meta.timestamp` 到其该轮最终 `task_complete`：旧 `2026-11-11T10:27:07.980Z` → `2026-11-11T12:27:38.176Z`；新 `2026-11-12T01:22:17.806Z` → `2026-11-12T01:51:13.418Z`。和旧报告一样，实际断网、中断及恢复保留在窗口内；旧基线包含长时间断网后的收尾，新旧时长差不能全部归因于 wait 修改。后续配图阶段均排除，未删去返修或普通等待以美化结果。这是单次历史基线对照，不能推断稳定节省比例或账单成本。

### 等待证据与逐页验收

新总聊天的 26 次已解析 wait_threads 调用中，cursor 续等逐次匹配前次返回值：`True`。工具错误/needs_attention 的实际发生情况以 `wait-audit.json` 各条结果为准；未实际遇到的分支只检查指引，不宣称完成动态故障注入。

全部 26 次均在外层首行使用 55,000 ms，并在实际内层参数使用 45,000 ms。没有章节等待的错误或 needs_attention。另有 6 次 functions.wait 均来自初始 `list_threads` 查询超过默认外层返回时限，完整保留在总聊天的 54 次有效模型调用和 token 中；它们不属于章节 wait_threads 提前返回。不是“所有 functions.wait 都为 0”。

真实完成返回耗时为 0.030124 秒, 15.905624 秒；详见 `resolved_wait_events` 的状态、参数、耗时及原始行号。其中若目标在调用前已完成，即证明立即读取完成状态，不将其解释成等待过程中事件的唤醒延迟。

独立文本核对逐页保存于 `independent-review.json`，绑定描述/分镜哈希与页面 revision，并与最终 MCP 快照核对。检查全部八句直接引语的原字、标点、顺序与说话人归属，以及使用气泡的页面中气泡指向的分镜说明；钥匙从陈渡右手交林遥右手，到后章右侧口袋；蓝包从林遥左手到灯室干木桌、放回信系口后回左手；先问目的、到门廊比锁但不开门、次章开锁登塔、循信找小窗圆镜、转镜朝海反射点灯；终点二人仍在灯室，右林遥左陈渡，未新增离塔或登船。文字核对不代表图片审阅。两章实际划场与逐页 UUID 见 chapter.json/quality-check.json。

两章各 2 场：首章连续页段为 [1,2] / [3]，次章为 [1] / [2,3]，不重叠不漏页。两章 index 状态均为 storyboard_checkpoint_complete。总聊天可见工具回显中，两章原文均为 0/10 段，无整章分镜；没有总聊天调用 chapters 或含原文 render_prompt 的记录，见 usage-audit.json.root_content_audit。

## 聊天与代理身份

所有以下会话的 hostId 均为 `local`，项目 cwd 均为各组自己的隔离目录；新组不是源码仓库，也没有 worktree。包含自动权限审查，避免将这些真实用量遗漏为零。

| 组别/职责 | thread UUID | 父聊天（subagent） | 代理路径 | 模型/推理 | 模型调用 |
| --- | --- | --- | --- | --- | ---: |
| 旧 / chapter / 雨站的钥匙 | `01a1ca27-23b2-7652-a5c6-13cf3bc5d6ac` | — | — | gpt-6.1-sol/xhigh | 33 |
| 旧 / chapter / 灯亮以后 | `01a1ca2f-d414-7bc2-95c3-a6e6503824e2` | — | — | gpt-6.1-sol/xhigh | 29 |
| 旧 / host_auxiliary | `01a1ca26-096d-7942-9f66-c0d7908e9c85` | — | — | codex-auto-review/low | 11 |
| 旧 / host_auxiliary | `01a1ca27-72ef-7da1-9874-9cf0b59dfed8` | — | — | codex-auto-review/low | 20 |
| 旧 / host_auxiliary | `01a1ca30-0f9c-7880-bc7a-139efd291d5b` | — | — | codex-auto-review/low | 19 |
| 旧 / root | `01a1ca25-a846-71e3-ac08-02d773ff8c49` | — | — | gpt-6.1-sol/xhigh | 61 |
| 旧 / subagent | `01a1ca28-d573-7c41-b690-c7c73b72fb20` | 01a1ca27-23b2-7652-a5c6-13cf3bc5d6ac | /root/ch01_storyboard | gpt-6.1-sol/xhigh | 9 |
| 旧 / subagent | `01a1ca31-331f-7c11-a131-d1231caa7d1b` | 01a1ca2f-d414-7bc2-95c3-a6e6503824e2 | /root/ch02_storyboard | gpt-6.1-sol/xhigh | 7 |
| 新 / chapter / 雨站的钥匙 | `01a1cd5b-cbd8-7f00-95d2-8990660a3267` | — | — | gpt-6.1-sol/xhigh | 36 |
| 新 / chapter / 灯亮以后 | `01a1cd68-57de-7bd0-8440-c74382ca85ef` | — | — | gpt-6.1-sol/xhigh | 32 |
| 新 / host_auxiliary | `01a1cd59-863a-7291-847c-4dd68b05d5e5` | — | — | codex-auto-review/low | 12 |
| 新 / host_auxiliary | `01a1cd5c-24e2-7742-8b7e-3b24fde310be` | — | — | codex-auto-review/low | 21 |
| 新 / host_auxiliary | `01a1cd68-c270-7343-b219-fd466b654fe1` | — | — | codex-auto-review/low | 20 |
| 新 / root | `01a1cd59-3448-7513-87b1-3999fc478a09` | — | — | gpt-6.1-sol/xhigh | 54 |
| 新 / subagent | `01a1cd5d-fa07-75d1-b955-32a9c39e8e29` | 01a1cd5b-cbd8-7f00-95d2-8990660a3267 | /root/vol01_ch01_storyboard | gpt-6.1-sol/xhigh | 13 |
| 新 / subagent | `01a1cd6a-895b-7d81-b70e-4c576fcd47bc` | 01a1cd68-57de-7bd0-8440-c74382ca85ef | /root/ch02_storyboard | gpt-6.1-sol/xhigh | 14 |

## 原始证据与复现

- 直接基线冻结用量：`/private/tmp/lumi-storyboard-host-xbakgo1q/usage-audit.json`、`usage-audit-rollouts/`；本次重新审计结果：`/private/tmp/lumi-storyboard-host-pp8f129b/baseline-wait-audit.json`。其额外 wait 每条均有 wait call_id、真实 cell、来源 exec call_id 和原始行号。
- 本次完整证据根目录：`/private/tmp/lumi-storyboard-host-pp8f129b/`。`manifest.json`、`thread.json`、`host-mcp.jsonl`、`initial-facts.json`、`final-facts.json`、`snapshot.json`、`independent-review.json`、`quality-check.json`、`usage-audit.json`、`usage-audit-rollouts/`、`wait-audit.json`、`skill-load-evidence.json`、`verification.json`。冻结 rollout 各自 SHA-256 在 usage-audit/wait-audit；后续使用这些冻结前缀，不能混入新轮次。
- 章节协调与交接位于新项目 `user-data/lumi-production/`；不是项目内部数据库文件。没有直接读写 `project.sqlite`、`.lumi/` 或 `assets/`。所有事实读取和写回经 Lumi CLI/MCP。

初始化命令（在源码仓库 cwd 执行；只读现有夹具脚本，临时产物在 /private/tmp）：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/codex-acceptance/storyboard_fixture.py --binary /Users/qingyang/mine-work/lumi/build/codex-dev/lumi_dev --baseline-skill /Users/qingyang/mine-work/lumi/codex-plugins/lumi/skills/create-picture-book
```

对返回的新目录核对初始事实、添加 App 项目，`list_projects` 取得准确 projectId，以原 `/private/tmp/lumi-storyboard-ab-gapi3cxu/prompt.txt` 创建总聊天；不覆盖模型/推理设置。总聊天及两章按本次快照运行。实际本轮总聊天为 `01a1cd59-3448-7513-87b1-3999fc478a09`。第二轮比较请用新 fixture，不能把本轮六页作品当作零页面起点。

原提示词 SHA-256：`87c92a2c966e6825fc464724ef9e9702267005f12cddf7885458c6a62eff6f40`；本轮亦保存在 `original-prompt.txt`。原文如下，后续保持相同验收输入：

```text
这是 Lumi Codex 插件的隔离验收，请实际完成创作工具操作。只使用当前项目 acceptance-skill/SKILL.md 及其引用文件中的这一份技能快照（优先于其他同名已安装技能）；先读取 acceptance.json 获取项目、章节顺序和技能路径。项目已初始化、setup 已定稿，小说原文已在这两个章节中，初始没有页面。
把两章小说分别改编为恰好 3 页正文分镜；保留全部直接引语原字、标点与正确说话人，保留关键事件因果和人物、服装、手别、钥匙与蓝包的状态连续性，旁白可删减但不得新增原文事件。规格沿用现有 setup。多章按技能的独立章节聊天流程串行执行。我授权在这个隔离项目内创建和继续所需章节聊天及 subagent；全部使用同一项目目录，沿用宿主默认模型与推理设置。
本次统一完成节点：两章分镜均已逐页核对并通过 Lumi 串行写回，保存各章划场和跨章连续性交接。到此停止，不创建设定图或场景制作代理，不生图、不导出、不打开审阅面板。这是分镜阶段的验收节点，不能据此宣称整本制作或审图完成。不要修改小说原文、setup 或别的项目。完成后只报告章节摘要、页数、阻塞、交接路径与实际创建的聊天/代理身份。
```

完成后冻结统计（`--end-at` 必须取真实创作总聊天完成事件；下列是本轮真实终点）：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/codex-acceptance/storyboard_fixture.py --snapshot /private/tmp/lumi-storyboard-host-pp8f129b/manifest.json
PYTHONDONTWRITEBYTECODE=1 python3 scripts/codex-acceptance/storyboard_audit.py --manifest /private/tmp/lumi-storyboard-host-pp8f129b/manifest.json --root-thread 01a1cd59-3448-7513-87b1-3999fc478a09 --sessions-dir /Users/qingyang/.codex/sessions/2026/11/12 --end-at 2026-11-12T01:51:13.418Z --output /private/tmp/lumi-storyboard-host-pp8f129b/usage-audit.json
PYTHONDONTWRITEBYTECODE=1 python3 codex-plugins/lumi/reports/audit-wait-threads.py --usage-audit /private/tmp/lumi-storyboard-host-pp8f129b/usage-audit.json --output /private/tmp/lumi-storyboard-host-pp8f129b/wait-audit.json
```

## 检查结果与限制

- `node --test scripts/test-dev-codex.mjs scripts/test-package-codex.mjs`：5/5 通过，覆盖开发包/发布包及引用可达；未安装或替换用户插件缓存。
- `quick_validate.py codex-plugins/lumi/skills/create-picture-book`：通过。
- 实际示例提取后 `node --check --input-type=module`：通过；审计脚本 Python AST 语法检查通过；审计脚本在两份旧真实日志和本轮冻结日志运行，按真实 yielded cell 区分额外/普通 wait。
- `git diff --check`：通过；修改范围仅 Codex 插件和报告。
- `node scripts/build-skills.mjs --check`：返回 1，仅报告上述两份 Codex 文件与受范围限制未改的共享源不同。未修改构建脚本以掩盖结果。
- 本轮没有生成/导入图片，也没有图片 passed 结论；不验证完整配图流程。没有主动触发 needs_attention、断网或 >55 秒异常等待。真实初始 `list_threads` 查询产生的普通 functions.wait 计入指标，不能归到章节等待消除目标里。

报告绝对路径：`/Users/qingyang/mine-work/lumi/codex-plugins/lumi/reports/wait-threads-acceptance.md`。
