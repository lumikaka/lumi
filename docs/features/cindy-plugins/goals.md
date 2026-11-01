# Lumi Cindy 插件 · 分步 Goal 提示词

## 使用方式

以下代码块是可以直接复制的 Goal **任务正文**；若当前会话支持 `/goal`，可在正文前添加该命令，也可以作为普通开发任务发送。它们不依赖 ChatGPT/Codex 自动化，不创建定时任务；实际执行和结果仍留在 Cindy 与当前 Lumi 仓库。

按 `0 → 1 → 2 → 3 → 4 → 4.5 → 5 → 6` 顺序，每次只提交一步。Step 7 可选，需单独提交。某一步的实际门槛未通过时，只完成独立工作并记录阻塞，不自动继续依赖该门槛的实现。

Step 0 已于 2026-11-01 完成，结论见 [实测报告](spike.md)；后续从 Step 1 单独提交。Step 0 的探针已卸载、临时数据已清理，不能直接安装旧审阅包继续运行。以下提示词区分可复用证据与本步新增验收；探针通过不等于产品实现通过。

Step 1–7 均受 [Codex 兼容门槛](plan.md#codex-兼容门槛) 约束：共用实现变更必须记录受影响的既有契约与对应回归结果；Cindy 验收不能替代 Codex 验收。

**本文件只保存将来使用的提示词，不表示现在执行这些动作。** Step 0、4.5、6 的文本包含测试安装范围；将对应文本作为任务提交后，按该范围和 Cindy 的真实权限/确认流程执行，不顺带发布市场。没有安装授权的其他步骤只开发与打包。

## Goal 0 · 验证 Cindy 插件的关键能力

对应计划：[Step 0](step0-spike.md)。

```text
在当前 Lumi 仓库完成 docs/features/cindy-plugins/step0-spike.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md 和当前 Cindy 提供的 cindy-plugin-creator Skill。确认真正执行插件的电脑、Cindy 版本与对应源码 commit，不以手机或底层引擎版本代替。

逐项验证随包 Node/Go CLI、共享 worker 的多任务隔离、sessionContext、工具和 Manual、授权媒体原件交接及参考图、Core media 与引擎原生生图的区别、taskView/WS 桥接、消息归档边界和安装流程。使用独立临时项目与 data-dir。此任务包含为验证安装本步生成的独立探针插件，并在验证结束后只移除该探针；先制作可审阅的包，走 Cindy 的正常安装确认，不修改其他插件或批准配置。

只写 docs/features/cindy-plugins/spike/ 下的探针和调研文档，不改 Lumi 产品代码。产出 docs/features/cindy-plugins/spike.md，每项写明通过/失败/未验证、版本、复现步骤、实际证据和对后续步骤的影响。没有实测的能力保留待验证；不运行 Cargo/Rust，不自动开始 Step 1，不发布插件。
```

## Goal 1 · 让 Cindy 工具连接 Lumi CLI

对应计划：[Step 1](step1-runtime-cli.md)。

```text
在当前 Lumi 仓库实现 docs/features/cindy-plugins/step1-runtime-cli.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、spike.md 和当前 cindy-plugin-creator Skill。Step 0 已验证当前 Mac arm64 上的随包 Node/Go CLI、两个真实本地任务的上下文及顺序项目隔离；真实同时并发、取消与只读/非本地上下文仍须补测。实际构建 sourceCommit 与官方 tag 不同，不能用 tag 代码冒充该构建源码或最低正式版本证明。

在 cindy-plugins/lumi/ 建立独立作者源码，采用浏览器 main.js + 随包 Node worker，衔接现有 lumi CLI/server。按项目管理固定 cwd 的 MCP 子进程，不改变 worker 的全局 cwd；透传真实调用上下文，正确处理取消、只读限制、大小限制和错误。区分 worker、项目连接和共享 server 的生命周期，避免一个任务结束时杀掉其他任务的服务，不直接读写 SQLite。

补齐 docs/features/cindy-plugins/development.md 的独立开发通道；当前 Step 2 包仍为 lumi/production，不能当作开发隔离已完成。从一份源码生成正式 lumi 与开发 lumi-dev，分别使用随包 bin/lumi、bin/lumi_dev，production + ~/.lumi 与 development + ~/.lumi_dev。实现 make cindy-dev 生成完整 build/cindy-dev/lumi-dev 作者目录，记录 channel、源 commit/dirty 与 CLI 指纹；worker 显式传环境和 data-dir，不依赖 APP_ENV，不开放任意路径给 Agent。实现显式 make cindy-dev-stop，沿用同一开发 data-dir；开发目录可能与 Codex 开发版共享，未经协调不自动停止。构建只生成作者目录，不暗中安装或发布。

把修改 → 构建 → Forge pack → 协调旧开发任务/后台 → Forge install → 实际调用验证写入开发说明。仅改 JS/Manual 也须重新打包安装；仅编译仓库 CLI 不会更新包内副本，安装新包也不会自动替换旧 detached server。诊断提供必要 channel/构建标识，不暴露私有路径或实例秘密。正式默认行为不变。

Step 0 实测通路是 CLI serve/status 加 REST/WS，没有验证 lumi mcp 的初始化、取消或协议适配；将这条 MCP 桥列为本步新增验证。后台通知不能阻止按需 Node 空闲回收，逻辑页 resident 不等于 Node 常驻；worker 崩溃、更新和插件卸载也不保证 detached CLI 退出。实现实例归属、引用管理、重启后的重新发现与绑定复核，以及只回收本插件应回收资源的策略，不依赖退出回调必达或全局 lumi stop。

完成双项目并发、同项目双任务、进程崩溃、取消、过期实例和协议错误的相关验证；增加 Cindy 与 Codex 客户端共用服务、Cindy 释放后 Codex 仍可调用的验证，无法证明独占的既有实例不得自动停止。交付 runtime.md 与 acceptance/step1-runtime.md，区分自动化与真实宿主结果。本步只开发和打包；没有另行安装委托时不安装。保留现有 Codex/桌面功能，不运行 Cargo/Rust，不自动开始 Step 2。
```

## Goal 2 · 绑定当前任务并初始化标准项目

对应计划：[Step 2](step2-project-binding.md)。

```text
在当前 Lumi 仓库实现 docs/features/cindy-plugins/step2-project-binding.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、spike.md、Step 1 的 runtime.md，以及当前 cindy-plugin-creator Skill 中的 sessionContext 与调用权限契约。

让当前 Cindy 任务通过 Host 提供的本地工作目录初始化或打开标准 Lumi 项目；不接受模型或页面伪造任意路径。复用现有 project_init/项目 Manager、项目锁和目录格式，支持幂等初始化与已有桌面作品。隔离 lumi init 中的 Codex guidance 副作用，使 Cindy 入口不自动创建或改写 .codex 配置，同时保留原有 Codex 行为。完善多任务绑定、只读/非本地拒绝、路径变化与重启校准。

已有项目初始化成功不代表当前 server 已打开项目；显式确认打开状态，覆盖项目空闲释放后重新打开、worker 重启后绑定复核。真实只读/非本地上下文仍待验收，不能把 Step 0 的伪造参数被 Host 覆盖当作这些场景已经通过。

验证空目录、冲突文件、已有项目、失败清理、双项目隔离和桌面重开；证明 Cindy 初始化不会写 Codex 专用配置，同时回归现有 lumi init 的 guidance 与用户配置保留行为，不通过全局关闭 guidance 实现隔离。交付 docs/features/cindy-plugins/project-binding.md 和对应测试结果。遵守统一 JSON/UUIDv7 规则，不改变项目格式，不运行 Cargo/Rust，不自动开始 Step 3。
```

## Goal 3 · 在 Cindy 对话中完成绘本文本

对应计划：[Step 3](step3-authoring-tools.md)。

```text
在当前 Lumi 仓库实现 docs/features/cindy-plugins/step3-authoring-tools.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、runtime.md、project-binding.md、docs/features/codex-plugins/tools.md，以及当前 Cindy 插件的工具发现和 Manual 文档。

使用 list_tools/call_tool 两段式目录提供项目、故事、章节、设定、页面、提示词与导出能力，复用 internal/textmcp 的业务契约；建立明确允许列表、各 action 的读写检查、schema 与自纠规则。render_prompt 保持提示词唯一来源，Agent 创作后以 expected_revision 写回，不调用 Lumi 内部模型 pipeline。编写可用的最小创作 Manual，保持串行路径。

Step 0 只证明工具发现、Manual、16 顶层工具上限与探针错误样本可用；产品业务 schema、错误映射、读写授权和版本保护须独立验收，不照搬探针的宽泛参数或错误包装。

共用层保留现有 Codex 工具名、schema、结果与 MCP Apps 元数据；Cindy 的 sessionContext 和权限检查留在适配层，不成为旧调用方的必填条件。按变更范围回归现有 MCP 读写和错误契约。

用临时项目跑通无服务商 setup、故事和设定、至少 3 页完整正文/画面描述、导出；验证版本冲突、取消后重读、跨项目拒绝和 WS 驱动状态更新。交付 docs/features/cindy-plugins/tools.md 与相关测试证据。固定文本测试不能写成真实 Agent 创作已验收。保留 Codex 行为，不运行 Cargo/Rust，不自动开始 Step 4。
```

## Goal 4 · 导入宿主生成图片并关联页面

对应计划：[Step 4](step4-image-import.md)。

```text
在当前 Lumi 仓库实现 docs/features/cindy-plugins/step4-image-import.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、spike.md、tools.md，及当前 Cindy 插件媒体/附件/Node 契约。Step 0 已分别验证 Core media 与引擎原生生成的 PNG 经授权交接到 Node，原件 SHA256 一致；尚未验证 Lumi Asset Store 导入或参考图参与真实编辑，额度来源仍未知。

实现显式 receive_media 工具：Agent 用 ghost_call.attachments 交接生成结果，插件只消费 Host 授权附件，把原件经有界分块与完整性校验交给 worker 暂存，再调用既有 import_image 保存到 Lumi Asset Store。业务参数使用目标 UUID、slot、expected_revision 和 fit_mode，不暴露任意源路径；保留原图、版本、keep/crop/pad 及参考图回送能力。插件不直接调用生成供应商，不扫描 Cindy 缓存。

探针给已知 PNG 的裸指纹补 .png 才读取成功，不得把该写法推广到所有格式；按当前公开媒体契约处理地址，并核对实际格式、MIME、尺寸与字节。参考图回送 Agent 成功不等于参考图编辑通过；未知哈希 404 也不等于跨插件归属拒绝已验收。

Cindy 附件限制只用于 Cindy 入口；保留 Codex 既有 import_image 调用方式，改动共用素材链路时回归原图保留、历史版本、比例处理和导出，不要求 Codex 提供 Cindy 附件上下文。

覆盖损坏/超限/重复块/取消/跨项目/版本冲突，并在有明确模型调用授权的真实 Cindy 会话中验证生图到导入、参考图编辑和导出；未执行的真实模型项标为待验收。交付 docs/features/cindy-plugins/media.md，分别说明 Core media 与引擎原生生图的实测范围，不承诺未知订阅额度。不运行 Cargo/Rust，不自动开始 Step 4.5。
```

## Goal 4.5 · 在真实 Cindy 验证任务页和实时桥接

对应计划：[Step 4.5](step4.5-task-view-acceptance.md)。

```text
在当前 Lumi 仓库完成 docs/features/cindy-plugins/step4.5-task-view-acceptance.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、spike.md、runtime.md、media.md，以及当前 Cindy 的 taskView/面板/Node 开发文档。

用两个临时项目与真实导入图片，在真正的 Cindy taskView 中验证开页、会话绑定、任务切换、BroadcastChannel/wake、图片权限与缓存、明暗主题、页面关闭/恢复、WS 提示转发和 REST 重读。覆盖首次 join、重新 join、窗口聚焦，以及超过 worker 空闲窗口后的连接状态。此任务包含安装并在验证后移除本步独立 UI 探针，使用可审阅包与 Cindy 正常安装确认，不改其他安装实例。

复用 Step 0 已取得的开页、wake、focus、light 主题、真实变更到 REST 重读及手动重新 join 的证据，重点补产品图片、双任务页隔离、dark 主题、自动恢复与 TanStack Query 失效。已知按需 Node 不靠后台通知续命，逻辑页 resident 不保证 Node 常驻；按当前宿主契约选择并实测连接生命周期，不能用定时业务读取保活。Node 的无 Origin 直连曾被 Lumi 拒绝；可复用现有项目范围提示票据，票据仅留 worker，不交页面或日志，不放宽 Origin/CSP。结束时先阻止旧页面重开连接，再按归属清理；卸载后复核残留 CLI。

产出 docs/features/cindy-plugins/task-view-acceptance.md 和 spike/ 下最小探针，给出 Step 5 固定通信方案与生命周期选择。区分浏览器自动化和真实宿主证据；无变化时不能有业务轮询，不关闭 CSP，不让页面直连 loopback。手机审阅另记边界，不冒充已支持。不运行 Cargo/Rust，不自动实现 Step 5。
```

## Goal 5 · 实现绘本任务审阅页

对应计划：[Step 5](step5-task-view.md)。

```text
在当前 Lumi 仓库实现 docs/features/cindy-plugins/step5-task-view.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、task-view-acceptance.md 和现有 web/mcp-app/；先确认 Step 4.5 的 UI 门槛已经通过。

在 Cindy taskView 中实现整本、单页、候选、设定四种视图，复用可共享的 React 展示逻辑并保留 Codex MCP Apps 构建。建立包内自包含 HTML 入口、只读页面桥、受控图片懒加载和项目隔离缓存。候选只做本地预览，实际选择由对话工具保存。状态同步使用 Node 转发 /api/v1/ws 提示，再让 TanStack Query 失效并经 REST 重读；覆盖首次/重连/聚焦校准。

落实 Step 4.5 实测通过的 Node 回收/恢复方案；恢复时重新确认任务绑定、项目打开状态与 WS 订阅，校准完成前不把旧缓存显示为已同步。关闭或停用后撤销页面读取和自动唤醒资格，晚到的 focus/wake 不得重新启动已释放的服务。Step 0 的手动重连不能替代自动恢复验收。

完成明暗主题、窄屏、组合 hover 状态、切任务不串图、连接失败恢复与完整文本兜底。Cindy 保持独立入口、宿主桥和构建目标，不覆盖 Codex 的 internal/textmcp/appdist/review.html；共享组件改动须通过既有 Codex MCP Apps 测试与构建，并核对资源/桥接契约。交付 docs/features/cindy-plugins/task-view.md、acceptance/step5-task-view.md 和相关测试。不要把桌面 taskView 宣称为手机页面，不直接连接 loopback，不轮询，不运行 Cargo/Rust，不自动开始 Step 6。
```

## Goal 6 · 打包、安装并验收完整创作流程

对应计划：[Step 6](step6-plugin-install.md)。

```text
在当前 Lumi 仓库完成 docs/features/cindy-plugins/step6-plugin-install.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、各步实际验收记录和当前 cindy-plugin-creator Skill 的 manifest/Manual/packaging/compatibility 文档。核查 Step 1–5 的真实阻塞，不把缺失证据跳过。

完善创作 Manual，制作包含 Node worker、目标架构 Go CLI、taskView 静态资源的 v3 .cindy 包；验证文件数、包体、执行位与最低正式 Cindy 版本。用户首次使用不需要 npm install/npx 或在线下载可执行代码，不向 ~/.codex 或全局 Skill 目录写配置。

先核对 development.md 的构建与隔离已经实现：共享源码生成正式 lumi 和开发 lumi-dev，开发包使用随包 lumi_dev、development 与明确的 ~/.lumi_dev 或专用测试 data-dir。当前计划中的 make cindy-dev / make cindy-dev-stop 未实现时先报告缺口，不能把 lumi/production 包充作开发版。补齐从修改源码、生成开发作者目录、Forge pack 到 Forge install 的完整日常流程；无需发布 patch 或市场上架。

Step 0 只验证当前 Mac arm64 构建；正式签名/公证、其他架构、单独停用仍待测，实际构建与 tag 不匹配不能证明最低正式版本。基础包不照搬探针为验证任务与 hooks 增加的权限。

此任务包括先 pack 提供审阅包，再从同一已审阅作者源码通过 Forge install 安装到当前 Cindy，并在已有同来源实例上进行明确的升级验收；遵循宿主正常权限确认和实例归属。Forge install 会重新打包，分别保存审阅包与安装时生成包的 SHA256、条目清单和安装回执；若指纹变化需核对内容，不能直接沿用首次 pack 的 SHA 或假定仅元数据变化。用临时作品完成真实 Agent 从想法到至少 3 页、宿主生图与导入、审阅修改、PDF/ZIP 导出、中断恢复和桌面重开。安装、模型调用或设备条件不可用时明确记录，继续完成独立打包和测试，不伪造通过。

本 Goal 的安装范围同时包含独立 lumi-dev 的安装和同来源更新验收，保留正式 lumi 的身份与来源；多实例按真实回执定位 target_instance_id，不擅自换 namespace 或创建副本。用独立测试作品验证两版并存；同版本修改 JS/Manual 后重新生成、pack/install，实际发现新内容。Go 改动另验证包更新不会自动替换旧 server：先协调全部开发客户端、释放旧页面及任务，再显式停止相同 environment/data-dir 的开发实例，重连核对新构建与代次。仅改 JS/Manual 不停止未变的共享 CLI。核对实际执行 Mac、插件 instanceId/namespace/source、审阅与安装包及 CLI 指纹；不把仓库文件已更新等同于安装副本或后台已更新。

验收本插件停用/启用与卸载保留作品：先阻止页面 focus/wake 重开服务，再按实例归属和剩余引用回收，卸载后核查工具不可用及残留进程；不能把卸载回执当作 CLI 已退出。卸载只走当前公开能力或用户操作的正常详情页；没有自动化入口时记录所需手动步骤，不删除 Host 安装目录或批准配置。

按 plan.md 的 Codex 兼容门槛汇总受影响链路的回归证据；覆盖 Cindy 停用/升级/卸载时 Codex 客户端仍可使用共享服务，核对两种插件的构建产物独立。真实 Codex 宿主未测项必须标为待验收，不能只凭 Cindy 成功声明兼容。

完成 development.md 的验收矩阵，开发构建、安装、更新、停止与卸载均不改变正式实例包/配置/启用状态，正式客户端仍可调用；Cindy 与 Codex 共用 development server 时也不得自动 stop。历史探针递增版本更新不能代替同版本刷新证据，未通过项单独记录。

交付可安装包、构建脚本、完整 Manual、docs/features/cindy-plugins/install.md、acceptance/step6-install.md 与文档入口，分别报告自动化、真实宿主、真实模型结果。当前任务不包含公开或组织发布，不卸载其他插件，不运行 Cargo/Rust，不自动开启聊天归档。
```

## Goal 7（可选）· 归档 Cindy 创作记录

对应计划：[Step 7](step7-session-archive.md)。

```text
在当前 Lumi 仓库实现可选的 docs/features/cindy-plugins/step7-session-archive.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、spike.md、Step 6 验收记录，以及当前 Cindy 的事件/消息钩子/任务读取契约。Step 0 已观察到 will-user/will-assistant 缺稳定消息 ID 与 seq，用户前置、助手后置，普通任务不可通过 tasks.readMessages 补读；完整增量归档尚不具备条件。先用临时任务补测重复文本、插话、中断、引擎失败、后续 hook 改写和可补读范围，分别记录复用证据与新增验收结果。

以明确标注缺口的有限创作记录或公开导出文件手动导入为候选范围；只有补齐稳定消息身份、提交事实与可靠补读证据后，才扩展为相应范围的增量归档。文本哈希只能校验内容，不能作为消息唯一标识或删除重复正文的依据；公开导出能力也须验证。真实能力不足时如实收敛范围，不扫描 Cindy 私有数据库，不伪造 Codex rollout。功能默认关闭，仅在明确启用的项目/任务范围采集；经 Lumi CLI/server 持锁写入独立只读记录，保留旧 Codex 导入和原生聊天模型。开启测试采集只针对本步临时任务，不收集用户已有对话。

实现幂等、缺口提示、禁用停止采集、失败不阻塞聊天、桌面只读回看与 WS 驱动重读；回归既有 Codex rollout fixture、重复导入及已有项目迁移/读取，不给旧记录增加 Cindy 必填字段。交付 docs/features/cindy-plugins/session-archive.md、脱敏 fixture、必要 migration/测试和真实覆盖范围。新增宿主权限/测试安装按既有授权与正常流程处理；未获安装授权先交付可审阅包。不要把未实现的完整历史写成已完成，不运行 Cargo/Rust。
```
