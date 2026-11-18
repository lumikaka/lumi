# Cindy 与 Codex 插件对齐

2026-11-18 最终交付：按用户最新安排，本次实现、代码审查、自动化和打包完成，用户后续另开任务人工验收。review 修复整章漏页提前交接、晚到错误覆盖成功回执和已知权限拒绝后无法显式恢复创建；Node 127/127、前端 483/483、Go 六包与三项同源门禁通过。正式、标准开发及可选独立后台开发包均已 Forge 打包并逐文件核对（各 32 条目、16 项工具），本次没有安装或切换后台。当前共享后台协议 9 仍阻塞需协议 11 的真实历史局部改图；不将该原生链路记为通过。详见[实现审查、功能对照及包路径](acceptance/implementation-review.md)、[人工验收清单](acceptance/manual-acceptance.md)及[分阶段验收证据](acceptance/codex-alignment.md)。下表是最初起点分类，不是最终实现状态。

## 2026-11-17 用户操作对照（本轮起点）

本轮实际工作树 HEAD 为 `5f26922f5bd1532a3b4984e0ca1da8376e58d41f`，起点无未提交修改。Codex 交付行为以此提交为准，`bb1ca879be47ceef0a1ff43ffab12cbb4fccc75c` 仅用于 Cindy 历史差异。下方 2026-11-04 记录为历史验收，不代表本轮结果。

| 用户操作 | 起点分类 | 已有实现与缺口 |
| --- | --- | --- |
| 创建作品并绑定当前任务 | 已具备，待本轮回归 | Node runtime/project 复用 Go；需核验本轮开发实例及只读与项目切换 |
| 写故事、章节、逐页正文和分镜、设定 | 已具备，待本轮回归 | 业务允许列表、revision、不可变版本及恢复已实现 |
| 在对话工具结果中打开可编辑工作区 | 部分可用 | 共享页面和隔离测试适配已实现；Cindy 入口仍显示不可连接，未声明标准 MCP Apps 资源及页面工具 |
| 工作区地址栏、前进后退、详情返回、操作菜单 | 缺失 Cindy 装配 | Codex 组件已实现，需接入 Cindy 的 MemoryRouter 和页面入口 |
| 点选页面、章、设定、@引用、文本候选加入当前对话 | 缺失 Cindy 装配 | Codex model-context 与共享选择行为已实现；Cindy 尚无 updateModelContext 通道 |
| 多章交接、分镜逐页起草、每章统一设定、逐场制作 | 部分可用 | 基础写入与按状态续做已实现；Manual 尚未对齐 serial-production 和真实 Cindy 任务/委派/等待/文件能力 |
| 中断后查事实、按页审阅、避免重复导入 | 部分可用 | runtime/media 与 Go 已有 revision 和恢复机制；需补多章与导入后中断样例 |
| 指定未采用历史版，按整页坐标矩形改图 | 部分可用 | Go 与 schema 已有签名 local_edit；Cindy reference.cjs 仅接受 collage，发现说明、Manual 及传图链路缺失 |
| 整页合图、无参考生图、设定视觉核对与图片版本采用 | 已具备，待本轮回归 | 原生文件/Core 受管媒体、prepare/receive_media 和选择版本已实现 |
| Markdown、ZIP、PDF 与任务文件交付 | 已具备，待本轮回归 | 复用 Go 导出和 Cindy 文件适配；生成与收到分开记账 |
| 本地阅读网站、发布提示词及章节顺序 | 已具备，待本轮回归 | export_website 已开放；需核对本轮产物目录和排序 |

宿主检查：check_app_update 实际返回 `0.1.139`，与需求中的 `0.1.130` 不同；本轮未更新应用。`/Applications/Cindy.app/Contents/Resources/cindy-source.json` 返回 `45376731c896138c86611c67833c34eb1d2d6abc`、构建时间 `2026-11-17T12:47:41.964+08:00`。该精确公开 commit 本轮网页读取失败，暂以当前随包契约和后续真实调用核验，不能宣称已核对匹配公开源码。标准工具结果页与旧 taskView 分开核验。

最终实现增量：局部裁图交接、工作区与上下文点选、多章/场景调度、逐页持久记录和任务回执恢复已完成代码及自动化。调度从 Lumi 当前事实检查跨章串行、默认逐场串行、明确选择后最多两场独立并行、依赖场放行与设定返修暂停；一页导入后重启没有重复建章、生图或导入。网站多章顺序、本地目录及发布提示词已回归。最终 Node 127/127、前端 483/483、Go 6 个测试包及三项门禁通过。

正式/开发 Forge 包均含 MCP Apps 页面、16 项工具声明、制作参考手册与全部运行文件，每包 32 个条目。Cindy 普通任务不能任意指定项目目录，也没有 wait_threads；等效流程是子任务在 Host 目录产草稿、当前 Lumi 任务串行提交，使用 getRun 与增量消息核对。agent.tasks 是新增能力，仍受实际宿主批准约束。

前次原生阶段已解除安装取消：当时安装回执为 updated，运行指纹与该阶段包一致；本次审查包只打包交付，尚未安装。真实工作区保存与点选引用由用户协助完成并取得实际回执；多章完整原生样例、实际模型历史局部改图和权限/切换生命周期由用户另开任务补验。Host mock、独立浏览器和单角色任务成功分别记账，不能代替剩余链路。功能差异、包路径与命令见 [验收记录](acceptance/codex-alignment.md)。

## 基线与目标（2026-11-04 历史阶段）

2026-11-04 开始，基线提交 `b4480d9af23db3706a242d6e8e530ff55cfcc43f`，工作树干净。以该提交的 `codex-plugins/lumi/skills/create-picture-book/`、`internal/textmcp/` 和可编辑 MCP 作品工作区为依据。历史 Step 5 的暂时跳过保留为历史，不限制本次范围。

本次交付覆盖标准项目创作、设定与页面媒体、作品/章节列表/章节/单页/设定/候选工作区，以及保存、排序、删除恢复、版本预览与采用。Codex 和桌面 Web 的视觉、路由及默认交互是兼容约束；手机仅保留已有任务文件交付，桌面 taskView 不代表手机工作区。

## 源码差异清单

| 能力 | 基线证据 | 分类与待办 |
| --- | --- | --- |
| 初始化、任务绑定、setup、故事、章节、分页、文本修改 | Cindy `node/runtime.cjs`、`catalog.cjs` 复用 Go MCP | 已复用；补足本次三页完整流程验收 |
| 图片导入、比例处理、原图恢复、图片版本采用 | Cindy `main.js`、`node/media.cjs`、Go `image.go` | 已有单图流程；重新验证授权、冲突、中断和未知结果 |
| setting_reviews 与 preparation | Go `schema.go`、`image_preparation.go` 已支持；Cindy 快照及 main/manifest 未同步 | 需要补齐，消除多层 schema 漂移 |
| 批量设定图、识图拆分、原子裁切入库 | Codex batch-settings 参考、Go setting_boards/apply_setting_breakdown；Cindy 明确禁用 | 需要开放显式业务工具，并使用授权附件导入总览图 |
| 用户原图、画风、视觉核对 | Codex setting-references；Cindy Manual 尚未完整覆盖 | 需要补齐流程；描述不能替代真实传图 |
| prepare_page_image 参考合图 | Go 返回临时路径；Cindy 直接沿用 Codex 路径说明 | 需要 Cindy 通道适配；分别核验原生引擎与 Core 实际参考参数 |
| Markdown/ZIP/PDF、任务文件、手机远程文件 | Cindy export.cjs、历史 Step 3/6/7 | 保留宿主差异；本次独立夹具重新验证，不以历史回执代替 |
| 作品、章节列表、章节、单页、设定、候选及编辑操作 | web/mcp-app/main.jsx、workspace.js、共享 SimpleProjectPages | Cindy 尚未实现；独立入口/桥/构建，复用共享主体 |
| WS → Query 失效 → REST 重读 | Codex realtime.js/sync.js，Go app_watch.go | Cindy 尚未接入；覆盖首次/rejoin/聚焦/切换/关闭/worker 恢复，无业务轮询 |
| 页面任务身份与权限生命周期 | 历史 task-view-acceptance 和 host-task-view-identity-needed | 需重新核验；未有可信 Host 证明时生产桥关闭 |
| 开发/正式隔离、随包 CLI | runtime-config、build-info、channels 测试 | 保留；核对安装实例、包及实际后台，禁止自动停共享服务 |

分类中的“已有”仅描述基线实现，不代表本次真实宿主验收通过。

## 宿主依据（2026-11-04 历史阶段）

2026-11-04 只读 Host 检查：本机 Cindy `0.1.121`，可更新目标 `0.1.125`；本任务未升级。`/Applications/Cindy.app/Contents/Resources/cindy-source.json` 为 `0a7d5a419f5cd3e0fbebc9642dde8388376a68aa`（构建时间 2026-10-30）。[精确公开 commit](https://github.com/makecindy/cindy/commit/0a7d5a419f5cd3e0fbebc9642dde8388376a68aa) 本次读取返回 404，不能以 main 或另一个 tag 代替。随包 Skill 的 taskView 仍只说明 session 路由查询，没有提供已核验的页面发送者任务证明。

这不是“不可能存在接口”的结论。生产桥开放前必须取得实际执行构建的公开契约、匹配实现和真实双任务身份/授权/撤销回执；页面 URL、session 字段、广播或可重放 secret 均不能成为权限来源。已安装 `lumi-dev` 及历史探针不证明新包或新增能力已验收。

## 执行计划

1. 从 Go 契约生成 Cindy schema、附件 manifest 和浏览器校验说明，加入读取实际 Go 输出的漂移检查；保持显式工具允许列表。
2. 开放设定板授权导入、查询、拆分与恢复，补齐视觉核对和两条媒体通道的 Manual；独立项目验证至少两项设定、三页及三种导出。
3. 建立独立 Cindy 工作区入口与适配接口，复用共享页面、图片缓存和 Query 同步；先完成隔离测试，生产桥按宿主身份门槛失败关闭。
4. 回归 Web 与 Codex，检查明暗主题、窄屏、hover 状态、生命周期及权限；自动化与真实宿主证据分开记录。
5. 构建开发版和 Forge 包，固定摘要供审阅；需要安装时走 Cindy 正常安装流程，核对实例/随包 CLI/运行后台；更新 Manual、开发说明及本次验收记录。

## 持续同步

Go `textmcp.Specs()` 是业务 schema 来源，新增 MCP 工具不自动开放给 Cindy。Cindy 维护明确允许列表和宿主差异转换；构建/检查必须比较实际 Go 输出与随包快照、manifest、main 的附件 schema。Codex 工作区变动时同时复跑共享页面兼容测试和 Cindy 独立适配器测试；宿主版本变动只触发重新核验，不自动开放生产读写。

## 2026-11-04 实施增量

已补齐 setting_reviews/preparation 的同源 schema、设定板授权附件导入、批次查询、原子拆分及稳定路径幂等恢复。render_prompt 的批次写回说明会转换成 Cindy receive_media，不再引导普通工具调用 path 导入；完整 generation_prompt 保持 Go 原值。Manual 已补充用户参考、画风/字体、必要设定图、视觉核对与批次恢复。

固定几何素材通过完整三页流程、两项裁切和三种导出；核对记录明确标为模拟，不算真实模型视觉验收。现有图片、只读、冲突、取消、未知结果、双项目、开发/正式隔离测试亦通过。

随后已完成 [Cindy 工作区](workspace.md) 的独立入口、构建、共享页面装配和内部授权适配接口。普通工具不能调用页面内部方法，生产页不按 URL/session/BroadcastChannel 建立连接。隔离真实 CLI 测试覆盖编辑、不可变脚本恢复、图片采用、排序、软删除/恢复与 WS 提示后的 REST 重读。浏览器验证标题保存、稿件采用、A/B 切换、外部 WS 修改以及明暗/窄屏；静默 219 秒只有 WS 等待，没有业务重读。合成页面授权与真实 Host 验收分开记账。

期间仓库由其它工作前进到 `ed935e1469b3f514432c94792f3482da13b26b0d`，仅修改 Windows 安装器校验相关文件；本次对齐基线仍是 b4480d9，构建记录使用实际 HEAD。Web/Codex 共享页面、入口及样式源码未改动，原构建与测试通过。

完整合图到 Core 的媒体交接现已实现：内部 reference_prepare/chunk/release 只读取成功的 Go 准备结果，限定系统临时目录、普通文件及 PNG 尺寸，按真实任务和调用绑定；48 KiB 分块后双端核对 SHA256，再使用 Host deposit_media。普通工具不开放这些方法，不接受模型路径；释放、权限改变、取消、worker 关闭及 90 秒期限使旧传输失效。Core 仅使用 full_collage/status=ready 的受管 URL，失败显式保留 failed 状态，原生路径与 Core 参数分开说明。

开发实例与验收探针已通过正常 Forge 流程原位更新，安装条目、CLI 与实际后台指纹已核对。旧探针 data-dir 中的共享后台协议不兼容，新运行时拒绝派发业务；未停止它，探针改用本次专属 /private/tmp/lumi-cindy-alignment-host-data。仅恢复原来 Host 拥有的 A/B 任务，无新任务或 Agent。B 使用固定几何图完成两项裁切、实际查看及核对、完整合图 Host 入库、三页正文/分镜/授权图片导入与三种导出；A 对 B 的设定/图片/页面读取均拒绝。真实宿主完整合图为 792×420、12345 字节，原文件 SHA256 与受管入库回执一致。

上述固定素材阶段与实际模型验收分开记录。随后用户明确授权本轮最多一次 Core 请求，实际 image.edit request=1、poll=0，按当前 Guide 显式传入同一完整合图及 generation_prompt，生成后授权导入第 1 页第 2 稿，原件摘要与 Core 输出一致；正文和分镜未变。已核对图片并完成候选采用/切回及新的无缺图三种导出。本次授权已使用，不自动追加模型请求。原生引擎生成、实际控制端文件读取、可信页面身份和可编辑工作区原生生命周期仍未通过，生产桥保持关闭，整体未完成。详情见 [本次验收](acceptance/codex-alignment.md)。
