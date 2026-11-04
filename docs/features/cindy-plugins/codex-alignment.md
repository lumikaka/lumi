# Cindy 与 Codex 插件对齐

## 基线与目标

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

## 当前宿主依据

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
