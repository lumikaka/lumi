# Codex 对齐验收

## 2026-11-18 实现审查交付

按用户安排，实现、代码审查、相关自动化与包交付已完成；完整真机验收由用户后续另开任务执行。没有把剩余人工项记为通过，也没有为本次审查重新安装插件、派发原生验收任务或切换共享后台。

功能对照、修复、包摘要与复核命令见 [实现审查报告](implementation-review.md)，后续按 [人工验收清单](manual-acceptance.md) 操作。原始日志与逐次回执仅在本地 `build/cindy-acceptance/` 归档，不再纳入 Git；本页保留结果摘要。

代码审查修复了整章漏页提前交接、晚到错误覆盖持久成功回执，以及确定因 PERMISSION_DENIED 失败的创建无法显式恢复。未知、取消和成功请求均不能借恢复入口替换；单元、真实 CLI 集成、重启、只读与跨项目负例已覆盖。

| 检查 | 交付时结果 |
| --- | --- |
| Node 全量 | 127/127 通过 |
| 前端全量 | 483/483 通过 |
| Codex 包兼容 | 2/2 通过 |
| Go 业务/CLI | 六包通过，schema 无测试文件 |
| authoring / contracts / skills | 全部通过 |
| Web、Codex MCP、Cindy 与随包 CLI 构建 | 通过；共享 Web/Go/Codex 源码相对基线没有增量 |
| Forge 打包与逐文件核对 | 三包均 32 条目、16 项工具；资源绑定、SDK、Manual、CLI 执行位和摘要通过 |

正式、标准开发与可选独立后台开发包的路径和摘要集中保留在审查报告。本次审查包均未安装；开发实例在该阶段仍为前次原生验收版本。用户选择保留当前后台，未安装或启动独立后台配置。历史局部改图需要后台协议 ≥11；当前保留后台的原生回执为 9，新包不自动停止旧服务。

## 审查前构建的原生验收摘要

以下结果来自真实隔离任务，不能代替最终审查包的完整人工验收。

| 用户操作 | 结果与范围 |
| --- | --- |
| 创建并绑定隔离作品、保存章页与正文分镜 | 原生通过，使用 Host 分配的独立任务目录 |
| 标准 MCP Apps 开页并在界面保存 | 用户协助真机通过；标题改为“蓝色方块·界面保存验收”，只读复查 revision 0→1，正文与分镜不变 |
| 点选后下一轮识别，点击本身不写入 | 用户协助真机通过；实际问句“刚才选的是哪一页”取得当轮注入的 comic_section、正确公开 UUID 和标题，复查 revision 仍为 1；用户确认点击未发消息 |
| 分镜角色创建/复用、发送、终态及文件交接 | 原生机制通过；同职责复用同一任务，一次发送、三次有界状态检查后 completed，原草稿文件核对通过；最终回复格式有缺项，不计作完整多章验收 |
| 更新后角色消息交接 | 原生通过；只交接可见 user/assistant 文本，保留原游标和文件身份 |
| 外项目读取隔离 | 原生负例通过；外项目页面不返回内容，外项目制作记录被拒，本任务项目绑定保持有效 |
| 多章独立/依赖场及导入后恢复 | 自动化两章四页通过；完整原生续验投递被取消，未重试，交用户后续验收 |
| 未采用历史版本矩形改图与显式采用 | 自动化通过；选区内 3,364 个像素变化、选区外 0，候选 adopted=false，显式采用才切换。原生仅完成历史 v1/当前 v2 的固定图导入，裁图准备被 LUMI_CLI_UPGRADE_REQUIRED 阻塞，无实际模型请求、v3 候选或采用 |
| Markdown/ZIP/PDF/网站 | 自动化通过，包含实际输出目录、发布提示词和阅读站章节排序 |

Computer Use 在当时的来源及隔离任务均返回 provider-not-enabled-for-session，保存和点选由用户操作后读回核对，没有自动点击录像。只读任务、切换项目、卡片重开、完整导航/授权生命周期、多章真实制作和实际模型历史改图仍需在新包上人工补验。

## 2026-11-17 续做状态（历史阶段）

状态：进行中。当前基线为 `5f26922f5bd1532a3b4984e0ca1da8376e58d41f`，历史比较为 `bb1ca879be47ceef0a1ff43ffab12cbb4fccc75c`。按用户操作的起点对照及下一阶段见 [对齐说明](../codex-alignment.md)。下面旧记录中的 taskView 限制不作为标准 MCP Apps 工具结果页的结论。

本阶段的原始日志、截图和回执可在 Git 提交 `9be0aac` 中回查。2026-11-18 用户允许更新开发版后，重新核对了审阅包，并向 Forge 提交 local lumi-dev 的原位更新。Host 再次返回 `MUTATION_CANCELLED`，随后只读查询仍列出旧版的三个工具。用户开启 Computer Use 后，该任务仍报告 `provider-not-enabled-for-session`。本阶段的界面验证仅在独立浏览器中完成。

### 本轮已完成的代码与自动化

| 范围 | 结果与证据 |
| --- | --- |
| local_edit 原尺寸裁图交接 | 自动化通过。reference reader 接受服务端选择的 local_edit PNG，校验临时路径、普通文件、原尺寸与 Go 返回 SHA256；传输标记为 local_edit_crop，保留完整 prompt/签名 preparation |
| 历史未采用版 → 矩形改图候选 → 显式采用 | 自动化通过。隔离真实 CLI/SQLite 取历史 v1，导入 v3 为 adopted=false，当前 v2 未变；3,364 个选区内像素发生改变，选区外变化为 0；显式 select_image 后才切换 |
| 导入后中断恢复与旧 revision | 自动化通过。重建 worker 后先读 pages 当前版本及全部历史，只有一个 v3 候选；旧签名/revision 重放被 production_conflict 拒绝，未新增版本；这是固定素材测试，不是实际模型生成 |
| 整页合图 / 无参考 / 错误传图 | 自动化通过。保留 full_collage 与 none 行为，覆盖损坏/不完整字节、摘要错误、错误媒体类型、Host 拒绝及传输释放 |
| 发现说明、Manual 与来源漂移 | 已补齐 local_edit 的参数、整页坐标、唯一裁图、原样提示词/签名、fit_mode=keep、候选不采用及未知结果不重提；新增规则断言后更新 page-images.md、schema.go 摘要并固定局部合成实现来源 |
| 标准 MCP Apps 工作区与点选 | 源码和自动化通过。6 个顶层 review_* 开页、5 个 app-only 资源/图片/WS 工具；每次使用 Host 注入上下文。官方 App SDK 负责连接、调用与 updateModelContext；不发送消息。独立浏览器保存、返回/前进、菜单、重连、点选及 A/B 切换通过，仍不是原生 Cindy 验收 |
| 正式/开发构建与 Forge 包 | 两包均通过 Forge 与逐文件检查。每包 32 条目，含 MCP Apps 页面资源、16 项工具声明、SDK 导入、制作参考 Manual 和运行文件。CLI 执行位和摘要均已核对。此前两个阶段的包分别有 27、28 条目。 |
| Cindy 真机三条链路 | 仍未通过。正常 Forge 更新 local lumi-dev 返回 MUTATION_CANCELLED，已停止且未重试；实时发现确认仍是旧版 3 工具。不能把内部适配器、Host mock 或独立浏览器记作真机通过 |
| 多章独立/依赖场调度及一页后中断 | 自动化通过。两章四页，默认串行；显式选择后 A/B 放行、C 等待 B 当前版 passed 与核实交接，前章未完不放行后章，设定返修暂停受影响场。导入一页后重建 runtime，再读取事实审阅已有图；最终仍为两章、四次模拟生成和四次导入 |
| 持久记录与普通任务回执 | 自动化通过。固定项目目录、只读、UUID 身份、CAS/幂等、符号链接拒绝；原 Host 请求先落盘、角色任务复用，未知发送原键核对后只有一次模拟执行。PERMISSION_DENIED 不自动重试；使用真实 main.js 与 mock SDK，不是原生委派证据 |
| 网站目录、提示词及多章排序 | 自动化通过。逆序创建 vol01.ch02/ch01，阅读站与 Lumi 顺序一致为 ch01/ch02，RSS 按站点章节顺序，固定 website/ 本地目录、发布提示词及资源链接均核对 |

实际宿主 check_app_update 返回 `currentVersion=0.1.139`、`no_installable_update`；本轮没有更新应用。随包 cindy-source.json 为 `45376731c896138c86611c67833c34eb1d2d6abc`，构建于 `2026-11-17T12:47:41.964+08:00`。该精确公开源码本轮网页读取失败。随包 MCP Apps 契约已读取，生产入口仍待实际调用核验；旧 0.1.121 验收不是当前宿主证据。

### 本轮可复核命令

- `node --test --test-reporter=spec cindy-plugins/tests/*.test.cjs` 当时 **120/120 通过**，覆盖制作记录与依赖检查、任务回执丢失与原键续接、局部改图、页面上下文转交、根 _meta 隔离、文本候选完整交付及顶层开页。单独重跑的 WS 断言也通过，确认 next 进入等待前没有读取项目概览。
- `go test ./internal/textmcp ./internal/production ./internal/files ./internal/project ./internal/promptcatalog ./cmd/lumi ./cindy-plugins/schema` 当时 **6 个测试包通过**，schema 包无测试文件。另以 `-run 'LocalEdit|LocalImageEdit' -count=1` 验证局部改图与 16 位/透明像素隔离。
- `cd web && node --test --test-reporter=spec` 当时 **483/483 通过**，覆盖官方 SDK 适配、点选无业务调用、超时停止排队点选和主题映射。该阶段未改动共享 Web/Codex 页面、路由及样式源码。
- `node cindy-plugins/scripts/check-authoring.mjs`：**12 组规则通过**；`node cindy-plugins/scripts/sync-contracts.mjs --check`：**31 个 Go 业务契约、6 个开页、5 个 app-only 工具及 2 个制作适配契约通过**；`node scripts/build-skills.mjs --check`：**通过**。
- `node cindy-plugins/scripts/build.mjs --channel production` 与 `node cindy-plugins/scripts/build.mjs --channel development --data-dir /private/tmp/lumi-cindy-alignment-20261117-dev-data` 均**通过**。随后分别调用 Forge pack，逐条核对包与作者目录、CLI SHA256/755、制作 Manual 和完整 schema。
- 前端构建：本 shell 无 `pnpm` 命令，`corepack pnpm` 在仓库根报告 8.10.5，未替换项目依赖。执行等效的已安装构建入口 `cd web && node node_modules/vite/bin/vite.js build && node mcp-app/build.mjs`：**Web 与 MCP 页面构建通过**，Vite 8.3.5；保留原有大 chunk 提示。

当前包路径：`cindy-plugins/lumi/lumi-0.4.0.cindy`、`build/cindy-dev/lumi-dev/lumi-dev-0.4.0.cindy`。分别运行 `python3 cindy-plugins/scripts/verify-package.py alignment-20261117 production` / `development` 均通过。正式包 SHA256 为 `b80c4f3c4d2783fef6c40a3fcf1cfafd8a2798481bf3d5ccfe140296b7327d98`；开发包为 `5a1d625dd54f64e52500e41e300768bb30e7e2162631e2f1fbe7a8101aa53bb7`。包检查不表示安装成功；本次构建已包含章节/场景实现。开发包 build_id 为 2c7c77f60dfff0a5356ae8f96fb99cf41915c3f5c73c4cfff78c7c95d1f171af。

### 独立浏览器与原生安装边界

独立浏览器使用模拟页面授权，实际读写经过 Lumi CLI、REST 和 SQLite。A 第 1 页标题从 `a page 1` 保存为 `Cindy saved identity page`，revision 为 3。B 同页仍为 `b page 1`，revision 为 2。点选回执携带 `resource_type=comic_section`、公开 uuid、项目/章节 UUID 和标题。全程仅在明确点击保存后发生一次业务写入。

从详情直接打开时，可以返回章节列表，再前进回原页。地址栏保留完整可复制值，操作菜单和重新连接通过。测试夹具注入暗色桌面主题和 320 px 浅色主题。窄屏的 viewport 和 bodyWidth 均为 320 px，无横向溢出。验收后关闭了本轮创建的两张浏览器页，并按记录的 PID/nonce 清理夹具和隔离 CLI。

失败步骤是 `ghost_forge_install({dir:"…/build/cindy-dev/lumi-dev",namespace:"local"})`：Host 返回 `MUTATION_CANCELLED`，说明用户取消本次安装/更新。未重试，未换来源或另装副本。随后实时 ghost_info 确认 local lumi-dev 仍只有 list_tools/call_tool/receive_media，旧目录仍说明 review 未实现；不能用它验证新 MCP Apps 功能。已有旧版对话创作、授权图片交接与任务文件导出可继续使用；新工作区当前可通过独立夹具复核。需要用户重新允许更新该开发实例，才可执行真正的卡片保存、下一轮对话识别及历史版局部改图采用。

当前独立开发与自动化已完成；下一步需要重新允许更新 local lumi-dev，之后执行隔离真实作品的三条链路及章节委派。原生只读/换项目/卡片重开/跨任务生命周期仍待验，不能据标准 SDK 的单元测试宣称宿主已注入可信任务上下文。新增 agent.tasks 也需要 Host 实际批准；拒绝只影响任务委派，可在当前任务按同样记录与审阅顺序串行完成。

### 多章制作的当前宿主适配与复核入口

包内 `authoring/references/serial-production.md` 以当前共享 serial-production 为来源，新增第 12 组门禁及省略规则的负向断言后固定来源摘要。入口为顶层 production_records / production_tasks，list_tools(category=production) 返回 schema 和规则；call_tool 包装明确拒绝，不代理任意 SDK、路径或任务 ID。

`production_records` 只允许 index、按章 chapter/settings/scene、分镜 manifest/逐页草稿及写入进度，存于当前项目 user-data/lumi-production/。独占锁、fsync、原子替换、revision 和 request_key 保存记录；不自动清除崩溃残留锁。scope 切换与只读在每次 Node 派发前核对。调度每次从实际 Lumi 页摘要重读，没有定时业务轮询。

`production_tasks` 使用实际发布的 cindy.tasks.capabilities/create/get/send/getRun/readMessages；Host 自有任务权限和来源校验保留，不切模型、不提升权限。创建和发送原参数在 SDK 调用前保存，角色槽位避免重复建任务；unknown/pending 阻止同一职责新请求，request_status 读原回执，resume_request 原键核对。确定权限拒绝记录 failed 并停止；未知结果对 Host 报失败，不伪造成功。分页消息游标原样保存与传回。

当前 Host 没有任意目录 create_thread 或 wait_threads。等效流程是子任务在 Host 分配的独立目录用普通文件能力产草稿，当前 Lumi 任务核对后串行写回；普通任务不支持时当前任务串行承担同样职责。该回退保留写入/审阅/恢复顺序，但不具有 Codex 同项目章节聊天与分镜/设定/场景代理的相同上下文隔离，不能声称 token 改善。原生 Host 任务、文件可读性与委派终态仍待实际授权后验证。

自动化 `production-integration` 使用真实产品 main.js、Node 适配、CLI、SQLite、媒体导入及网站构建，Cindy 任务 SDK 和生图均为确定性 mock。诊断输出含 `native_host=false` 和 `real_model=false`。单位测试覆盖两个名额、依赖版变更、共享设定暂停保留名额、只读和项目身份、路径及原键恢复。

尚需真实开发实例完成：

1. review_page 打开隔离项目，实际保存标题并经业务工具重读；再测试只读、换项目、卡片重开、跨任务及首次/rejoin/聚焦同步。
2. 点选有明确 UUID 的页，确认没有消息或作品写入；下一轮用户输入“刚才选的是哪一页”，读取 Host 模型上下文中的同一对象。
3. 指定未采用历史版矩形改图，原裁图/提示词/签名通过实际生图通道，导入 adopted=false 并核对当前版未变；用户显式采用后才切换。
4. Host 普通任务创建/复用及消息游标、草稿文件交接，两章独立/依赖场与一页导入后中断恢复；核对实际生成/导入回执无重复。

## 2026-11-04 历史验收（保留原记录）

当时状态：blocked（宿主条件及剩余真实验收）。可独立开发、自动化、构建/安装核对和当轮授权的一次 Core 合图验收已完成；整体目标未完成。历史 Step 5 跳过不构成本次豁免。

## 证据边界

- 当前宿主只读更新检查：运行 0.1.121，目标 0.1.125；未安装更新。
- 实际应用 sourceCommit：`0a7d5a419f5cd3e0fbebc9642dde8388376a68aa`；精确公开 ref 返回 404。未将其它版本源码当作匹配证据。
- 本次开始时仓库干净；开始时已安装开发版/探针是旧包。随后已走正常 Forge 更新并逐项核对；见下文，不能以版本号相同推断内容相同。
- 自动化固定素材、真实模型调用、原生 UI 与实际文件读取必须分别列出；打包不等于安装或验收。

## 验证清单

| 范围 | 当前状态 |
| --- | --- |
| Go 实际契约 → Cindy 快照/manifest/main 漂移检查 | 实际 Go 生成与 --check 通过，setting_reviews 未丢字段 |
| 两项设定板导入/拆分/核对与页面参考传递 | 自动化通过；真实 Host 固定素材的两项裁切/看图/核对/完整合图入库通过；一次真实 Core image.edit 显式传入同一合图成功，原生引擎未验 |
| 初始化 → 三页正文/分镜/授权导入 → Markdown/ZIP/PDF | 独立真实 CLI/SQLite、固定输出与附件传输自动化通过；非真实自主模型创作 |
| revision、重复请求、取消、未知结果重读 | 工具自动化通过；含原键重放、worker 重建后板导入、冲突与未知结果重读；工作区待验 |
| 双任务/项目数据、图片、缓存隔离 | 工具和工作区缓存隔离自动化通过；Host A 对 B 的设定、图片和页面读取均拒绝；可信原生页面身份仍未验 |
| 工作区保存、预览/采用、排序、删除/恢复 | 内部适配器与真实 CLI/REST/SQLite 自动化通过；浏览器标题保存、图片采用通过；原生 Cindy 仍待验 |
| 首次 join、重连、聚焦、关闭、切换、worker 恢复 | 独立会话复用既有同步/校准；关闭与晚到响应隔离通过；浏览器静默 219 秒无业务重读，A/B 切换与真实 WS 重读通过；原生生命周期待验 |
| Web/Codex 实际页面回归 | 当前 470 项前端测试、Web/MCP 构建、2 项 Codex 包测试通过；共享 Web/Codex 源码和产物未变化；原生 Codex 客户端未重新验收 |
| 开发构建、Forge 包、实例/CLI/后台版本 | 正式/开发构建及 27 条目包通过；开发/探针原位更新并与作者树逐项核对；真实后台 CLI SHA256 和协议版本 3 匹配；正式安装未改 |
| 当前原生宿主页面身份与读写授权生命周期 | 未核验，生产桥关闭 |
| 真实模型及实际文件交付 | 本轮显式授权最多 1 次 Core 请求，实际 request=1/poll=0，已导入并核对候选；原生引擎和本轮控制端文件读取未验 |

## 本次命令与结果

- `node cindy-plugins/scripts/verify.mjs codex-alignment`：本轮最终 94/94 通过，完整测试见 [automated.json](codex-alignment-evidence/automated.json)。包含完整合图的精确字节传输、损坏/不完整/Host 拒绝/错误摘要、并发预算、令牌隔离、释放和权限降级，以及探针条件 schema。另含真实工作区保存/预览/采用/排序/删除恢复及 WS 重读；脚本采用沿用业务创建新的 restore 版本，图片采用保留原版本身份。覆盖初始化/故事/章/三页、两项批次裁切、原子失败回滚、冲突、取消、未知结果、双任务/项目、两环境和服务重启。首次清理检查在已确认服务关闭期间遇到 discovery 瞬态错误，已改为等待明确停止，未把该错误当作停止成功；[修复前记录](codex-alignment-evidence/automated-before-shutdown-wait-fix.json) 保留。随后全量并发暴露 status 身份探测争抢生命周期锁，已仅为该身份读取加入有限次重试，不重放业务或启停服务；[修复前记录](codex-alignment-evidence/automated-before-status-contention-fix.json) 保留。
- `go test ./internal/textmcp ./internal/production ./internal/files ./internal/project ./internal/promptcatalog ./cmd/lumi ./cindy-plugins/schema`：6 个业务/CLI 包通过，schema 命令包无测试文件。
- `cd web && node --test --test-reporter=spec`：470/470 通过，包含 Cindy 请求适配、默认只读、能力拒绝、同项目任务缓存隔离、关闭后晚到结果、pagehide/组件清理幂等以及主题转换。`pnpm build`：Web 和 Codex MCP 均成功；共享源码与原 MCP 产物无 diff。`node --test scripts/test-package-codex.mjs`：2/2 通过。前一轮 465 项回归摘要保留于 [regression-20261104.json](codex-alignment-evidence/regression-20261104.json)。
- `node cindy-plugins/scripts/build.mjs --channel production` 和 `--channel development --data-dir /private/tmp/lumi-cindy-alignment-dev-data`：构建通过；仅在测试独立目录启动服务，不停止用户共用服务。
- Host `ghost_forge_pack` 后，`python3 cindy-plugins/scripts/verify-package.py codex-alignment [development]`：正式/开发包逐文件、身份、环境、CLI 执行位/SHA256、build_id 与大小上限通过。包摘要分别见 [正式包](codex-alignment-evidence/package.json)、[开发包](codex-alignment-evidence/package-development.json)；已按摘要保存审阅副本于 build/cindy-reviewed/，不表示安装。

## 独立浏览器工作区

[浏览器记录](codex-alignment-evidence/browser-workspace-20261104.json) 的身份由夹具模拟，业务链路使用真实产品适配器、CLI、REST 和 SQLite；不计作 Cindy 原生授权通过。实际操作验证：标题从 a page 1 保存为 Cindy edited title；采用后标记由第 2 稿变为第 1 稿；外部修改后页面显示 a external WS change；切换 B 后显示 b chapter/b page 1 且仍使用第 2 稿。新夹具记录静默 219429 ms、6 次初始业务读、后续只有 WS 等待，未使用业务轮询。

暗色检查发现透明白底压缩为八位色值后标签对比度不正确，已修复 Cindy 独立产物的颜色映射并加入测试。复查标签采用暗色表面、浅色文字；[390 px 暗色](codex-alignment-evidence/cindy-dark-390.webp) 与 [320 px 浅色](codex-alignment-evidence/cindy-light-320.webp) 均无页面水平溢出。页面横向缩略图轨道保留原交互，窄屏结果不代表手机原生支持。

本轮执行时 HEAD 已由其它工作前进到 ed935e1469b3f514432c94792f3482da13b26b0d，仅涉及 Windows 安装器校验；基线仍固定为 b4480d9。build-info 和包证据记录实际构建版本。

## 当前宿主复核记录

`node cindy-plugins/scripts/verify-host-taskview.mjs docs/features/cindy-plugins/acceptance/codex-alignment-evidence/host-contract-20261104.json`：实际应用包路由和 taskView 的 session 查询审计通过，可信页面身份仍未核验；[证据](codex-alignment-evidence/host-contract-20261104.json)。这不是穷尽所有未公开能力的声明。

早先已安装历史诊断探针 open_probe 返回当前任务开页成功，随后 report 的 observations 为空，只说明请求已接受。[早先回执](codex-alignment-evidence/host-probe-20261104.json) 不计作生产工作区或页面身份验收。更新探针后可收到 loaded/light、public_context_keys=[locale,region]、has_preload=false 的非可信页面观察，仍不是 Host 身份证明，也没有释放作品给页面。后续项目和 Core 验收与该阶段分开记账。

## 完整合图及真实 Core 验收

开发包经 Forge 正常更新到个人实例 _root__lumi-dev，探针同样原位更新，没有新增实例或 Agent。最终 build_id 为 c97cb3e4ad9b68aa6df77d392b2260a43cf014e1372cc41777b0e4093e427752；随包 CLI SHA256 为 4142d15db995b5fac7fb8535efd5b4fcd07edb6e8ce5edd6aed9bba31cc4812b。正式审阅包 SHA256 为 528d881918804b4d5cb143756fb059b825eff05a8cefd763bc16064b049a8d96，开发审阅包为 c2e2c0977f843a49054abbe44242f05f75ee51f1b159ef6f48e7ed3f5df728cf。安装时重新打包的 ZIP 摘要可变，逐文件内容必须一致。见 [最终回执](codex-alignment-evidence/media-final-install-20261104.json) 和 [安装文件比对](codex-alignment-evidence/media-installed-files-20261104.json)。

可重复核对命令：`python3 scripts/cindy-acceptance/verify-installed.py '<当前已确认的 Cindy owner>/cindy-brain/_ns/_root'`。本次正式包 build_id/CLI 与本轮首次只读观察一致；没有误将 11 月 2 日某次较早安装记录当作当前正式基线。只比较开发/探针全部条目，正式仅复核构建和 CLI 字节，不扩大为正式 UI 全量验收。

旧探针 /private/tmp/lumi-cindy-host-acceptance-20261101 中发现协议不兼容的共享服务。runtime_status 明确 upgrade_required，project_overview 未派发；未停止、升级或删锁。探针改用 /private/tmp/lumi-cindy-alignment-host-data，仍只恢复 Host 拥有的原 A/B 任务，由 Go 项目锁检查访问。真实后台协议版本 3，实际可执行文件 SHA256 与随包 CLI 一致；[字节及后台核对](codex-alignment-evidence/media-byte-verification-20261104.json)。

B 项目以固定几何素材完成 setup、完整章正文及三页正文/分镜；总览图经 Core import_image（不调用模型）登记，再通过顶层 attachments 导入，原子裁切两项，实际查看总览及裁切预览，记录具体 setting_reviews。完整合图 792×420、12345 字节，SHA256 为 5a2f7c8d47b4b3dd56c3d18880282e1386619bce8fac116f8278595f12ea283d；Go 原文件和 Host 受管 URL 回执一致。A 读取 B 的设定、图片和页面均明确不存在。三页固定图导入和三种无缺图导出见 [固定素材真实 Host 记录](codex-alignment-evidence/media-host-fixed-20261104.json)，不算真实模型生成。

随后收到本轮[最多 1 次 Core 请求的明确授权](codex-alignment-evidence/media-model-authorization-20261104.json)，重新准备第 1 页及查看同一合图，重新取得 openai/gpt-image-2 / openai 的 Guide revision 3。实际 image.edit body 的 image 是上述完整合图受管 URL，prompt 原样使用 generation_prompt，size=1024x768、quality=low。request=1、poll=0，无重提。返回实际 PNG 为 1448×1086、1022629 字节（保持 4:3，不能把请求尺寸当成实际尺寸），source_sha256 与 Core 输出 URL 的 98f17a91…7d971c0 一致。顶层附件导入后 page.revision 1→2，当前第 2 稿，正文/分镜未变；再采用原稿并切回真实稿，revision 2→3→4，历史均保留。已查看受控预览，红蓝几何主体及准确旁白可见；不声明独立 OCR 或模型内部利用证明。见 [实际请求及导入](codex-alignment-evidence/media-model-request-20261104.json)、[真实模型验收](codex-alignment-evidence/media-host-real-20261104.json)。

最后再次导出 Markdown/ZIP/PDF，均 missing_image_count=0；ZIP 752108 字节、PDF 82095 字节。它们包含一页真实模型图和两页固定测试图，不能宣称三页均真实生成。回执仍为 file_generated=true、delivery.status=prepared、user_received=false；本轮未取得控制端下载/手机读取证明。未再生成其它图片，原生引擎通道未验。

## 阻塞与最小解除条件

页面生产读写需要 Host 证明请求发送者所属插件实例、任务、有效页面和当前读写权限，并在切任务/关页/停用/卸载时撤销。当前尚未核验该能力；不采信页面自报 session 或广播。先完成不依赖它的工具、共享 UI、独立测试及构建，随后据真实证据更新此处。

匹配宿主实现源码尚不可取得。需要实际构建对应的可核验公开实现或正式契约及运行时能力证据；本任务不修改或升级 Cindy 应用。

剩余解除条件：

1. 提供实际 Cindy 0.1.121 / sourceCommit=0a7d5a41… 的可核验实现和可信页面任务身份、读写授权、切任务/关页/停用/卸载撤销契约；之后才能连接生产桥并执行原生桌面工作区全操作及生命周期验收。不能用 URL、广播、fixture key 或任务路由字段代替。
2. 原生引擎通道需要真实可用的引擎及其正式参考输入契约、明确的本轮模型预算；本次授权仅覆盖一次 Core 请求且已使用。查看同一临时合图不算原生生图参考已传入。
3. 需在正常 Lumi 开发任务中复验普通 Host 上下文绑定及实际文件列表读取，另有真实 Codex 客户端时执行宿主回归。探针的 Host-owned A/B 上下文不冒充普通任务注入或 Codex 原生客户端。B 的最终 PDF 为 lumi-exports/text-01a1a71e-2fb5-7278-8acc-df83fb60068c.pdf，ZIP 为 lumi-exports/text-01a1a71e-1dee-759d-ba61-00c50d71e35d.zip；在原 B 任务文件列表读取并核对摘要后才记录控制端收到。手机无需也未宣称具备桌面工作区。

最后已释放本次 B 及开发诊断连接，项目、候选和文件保留，所有共享后台保留；未修改宿主应用、发布、推送、合并或执行 Cargo/Rust。Formal/开发安装边界及所有固定素材/真实模型证据持续分列，不能将本状态改为完成。
