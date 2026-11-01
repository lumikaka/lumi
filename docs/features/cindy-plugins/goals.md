# Lumi Cindy 插件 · 分步 Goal 提示词

## 使用方式

以下代码块是可以直接复制的 Goal **任务正文**；若当前会话支持 `/goal`，可在正文前添加该命令，也可以作为普通开发任务发送。它们不依赖 ChatGPT/Codex 自动化，不创建定时任务；实际执行和结果仍留在 Cindy 与当前 Lumi 仓库。

按 `0 → 1 → 2 → 3 → 4 → 4.5 → 5 → 6` 顺序，每次只提交一步。Step 7 可选，需单独提交。某一步的实际门槛未通过时，只完成独立工作并记录阻塞，不自动继续依赖该门槛的实现。

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

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、spike.md 和当前 cindy-plugin-creator Skill。只使用 Step 0 已验证的运行时能力；缺少 Node/CLI/上下文前置证据时先报告缺口，不猜测接口。

在 cindy-plugins/lumi/ 建立独立作者源码，采用浏览器 main.js + 随包 Node worker，衔接现有 lumi CLI/server。按项目管理固定 cwd 的 MCP 子进程，不改变 worker 的全局 cwd；透传真实调用上下文，正确处理取消、只读限制、大小限制和错误。区分 worker、项目连接和共享 server 的生命周期，避免一个任务结束时杀掉其他任务的服务，不直接读写 SQLite。

完成双项目并发、同项目双任务、进程崩溃、取消、过期实例和协议错误的相关验证，交付 runtime.md 与 acceptance/step1-runtime.md，区分自动化与真实宿主结果。本步只开发和打包；没有另行安装委托时不安装。保留现有 Codex/桌面功能，不运行 Cargo/Rust，不自动开始 Step 2。
```

## Goal 2 · 绑定当前任务并初始化标准项目

对应计划：[Step 2](step2-project-binding.md)。

```text
在当前 Lumi 仓库实现 docs/features/cindy-plugins/step2-project-binding.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、Step 1 的 runtime.md，以及当前 cindy-plugin-creator Skill 中的 sessionContext 与调用权限契约。

让当前 Cindy 任务通过 Host 提供的本地工作目录初始化或打开标准 Lumi 项目；不接受模型或页面伪造任意路径。复用现有 project_init/项目 Manager、项目锁和目录格式，支持幂等初始化与已有桌面作品。隔离 lumi init 中的 Codex guidance 副作用，使 Cindy 入口不自动创建或改写 .codex 配置，同时保留原有 Codex 行为。完善多任务绑定、只读/非本地拒绝、路径变化与重启校准。

验证空目录、冲突文件、已有项目、失败清理、双项目隔离和桌面重开；证明 Cindy 初始化不会写 Codex 专用配置。交付 docs/features/cindy-plugins/project-binding.md 和对应测试结果。遵守统一 JSON/UUIDv7 规则，不改变项目格式，不运行 Cargo/Rust，不自动开始 Step 3。
```

## Goal 3 · 在 Cindy 对话中完成绘本文本

对应计划：[Step 3](step3-authoring-tools.md)。

```text
在当前 Lumi 仓库实现 docs/features/cindy-plugins/step3-authoring-tools.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、runtime.md、project-binding.md、docs/features/codex-plugins/tools.md，以及当前 Cindy 插件的工具发现和 Manual 文档。

使用 list_tools/call_tool 两段式目录提供项目、故事、章节、设定、页面、提示词与导出能力，复用 internal/textmcp 的业务契约；建立明确允许列表、各 action 的读写检查、schema 与自纠规则。render_prompt 保持提示词唯一来源，Agent 创作后以 expected_revision 写回，不调用 Lumi 内部模型 pipeline。编写可用的最小创作 Manual，保持串行路径。

用临时项目跑通无服务商 setup、故事和设定、至少 3 页完整正文/画面描述、导出；验证版本冲突、取消后重读、跨项目拒绝和 WS 驱动状态更新。交付 docs/features/cindy-plugins/tools.md 与相关测试证据。固定文本测试不能写成真实 Agent 创作已验收。保留 Codex 行为，不运行 Cargo/Rust，不自动开始 Step 4。
```

## Goal 4 · 导入宿主生成图片并关联页面

对应计划：[Step 4](step4-image-import.md)。

```text
在当前 Lumi 仓库实现 docs/features/cindy-plugins/step4-image-import.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、spike.md、tools.md，及当前 Cindy 插件媒体/附件/Node 契约。只有至少一条授权媒体原件链路已在 Step 0 验证后才接入产品。

实现显式 receive_media 工具：Agent 用 ghost_call.attachments 交接生成结果，插件只消费 Host 授权附件，把原件经有界分块与完整性校验交给 worker 暂存，再调用既有 import_image 保存到 Lumi Asset Store。业务参数使用目标 UUID、slot、expected_revision 和 fit_mode，不暴露任意源路径；保留原图、版本、keep/crop/pad 及参考图回送能力。插件不直接调用生成供应商，不扫描 Cindy 缓存。

覆盖损坏/超限/重复块/取消/跨项目/版本冲突，并在有明确模型调用授权的真实 Cindy 会话中验证生图到导入、参考图编辑和导出；未执行的真实模型项标为待验收。交付 docs/features/cindy-plugins/media.md，分别说明 Core media 与引擎原生生图的实测范围，不承诺未知订阅额度。不运行 Cargo/Rust，不自动开始 Step 4.5。
```

## Goal 4.5 · 在真实 Cindy 验证任务页和实时桥接

对应计划：[Step 4.5](step4.5-task-view-acceptance.md)。

```text
在当前 Lumi 仓库完成 docs/features/cindy-plugins/step4.5-task-view-acceptance.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、spike.md、runtime.md、media.md，以及当前 Cindy 的 taskView/面板/Node 开发文档。

用两个临时项目与真实导入图片，在真正的 Cindy taskView 中验证开页、会话绑定、任务切换、BroadcastChannel/wake、图片权限与缓存、明暗主题、页面关闭/恢复、WS 提示转发和 REST 重读。覆盖首次 join、重新 join、窗口聚焦，以及超过 worker 空闲窗口后的连接状态。此任务包含安装并在验证后移除本步独立 UI 探针，使用可审阅包与 Cindy 正常安装确认，不改其他安装实例。

产出 docs/features/cindy-plugins/task-view-acceptance.md 和 spike/ 下最小探针，给出 Step 5 固定通信方案与生命周期选择。区分浏览器自动化和真实宿主证据；无变化时不能有业务轮询，不关闭 CSP，不让页面直连 loopback。手机审阅另记边界，不冒充已支持。不运行 Cargo/Rust，不自动实现 Step 5。
```

## Goal 5 · 实现绘本任务审阅页

对应计划：[Step 5](step5-task-view.md)。

```text
在当前 Lumi 仓库实现 docs/features/cindy-plugins/step5-task-view.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、task-view-acceptance.md 和现有 web/mcp-app/；先确认 Step 4.5 的 UI 门槛已经通过。

在 Cindy taskView 中实现整本、单页、候选、设定四种视图，复用可共享的 React 展示逻辑并保留 Codex MCP Apps 构建。建立包内自包含 HTML 入口、只读页面桥、受控图片懒加载和项目隔离缓存。候选只做本地预览，实际选择由对话工具保存。状态同步使用 Node 转发 /api/v1/ws 提示，再让 TanStack Query 失效并经 REST 重读；覆盖首次/重连/聚焦校准。

完成明暗主题、窄屏、组合 hover 状态、切任务不串图、连接失败恢复与完整文本兜底。交付 docs/features/cindy-plugins/task-view.md、acceptance/step5-task-view.md 和相关测试，验证共享组件改动未破坏 Codex。不要把桌面 taskView 宣称为手机页面，不直接连接 loopback，不轮询，不运行 Cargo/Rust，不自动开始 Step 6。
```

## Goal 6 · 打包、安装并验收完整创作流程

对应计划：[Step 6](step6-plugin-install.md)。

```text
在当前 Lumi 仓库完成 docs/features/cindy-plugins/step6-plugin-install.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、各步实际验收记录和当前 cindy-plugin-creator Skill 的 manifest/Manual/packaging/compatibility 文档。核查 Step 1–5 的真实阻塞，不把缺失证据跳过。

完善创作 Manual，制作包含 Node worker、目标架构 Go CLI、taskView 静态资源的 v3 .cindy 包；验证文件数、包体、执行位与最低正式 Cindy 版本。用户首次使用不需要 npm install/npx 或在线下载可执行代码，不向 ~/.codex 或全局 Skill 目录写配置。

此任务包括通过 Forge 将审阅后的确切包安装到当前 Cindy，并在已有同来源实例上进行明确的升级验收；遵循宿主正常权限确认和实例归属。用临时作品完成真实 Agent 从想法到至少 3 页、宿主生图与导入、审阅修改、PDF/ZIP 导出、中断恢复和桌面重开。安装、模型调用或设备条件不可用时明确记录，继续完成独立打包和测试，不伪造通过。

交付可安装包、构建脚本、完整 Manual、docs/features/cindy-plugins/install.md、acceptance/step6-install.md 与文档入口，分别报告自动化、真实宿主、真实模型结果。当前任务不包含公开或组织发布，不卸载其他插件，不运行 Cargo/Rust，不自动开启聊天归档。
```

## Goal 7（可选）· 归档 Cindy 创作记录

对应计划：[Step 7](step7-session-archive.md)。

```text
在当前 Lumi 仓库实现可选的 docs/features/cindy-plugins/step7-session-archive.md。

先读根 AGENTS.md、docs/features/cindy-plugins/plan.md、references.md、spike.md、Step 6 验收记录，以及当前 Cindy 的事件/消息钩子/任务读取契约。先用临时任务验证真实消息身份、提交时序、重复文本、中断、后续 hook 改写和可补读范围；不能假定 hooks 等于完整 transcript，不能把 tasks.readMessages 用于任意用户任务。

根据证据实现增量归档、明确标注缺口的有限创作记录，或公开导出文件的手动导入。真实能力不足时如实收敛范围，不扫描 Cindy 私有数据库，不伪造 Codex rollout。功能默认关闭，仅在明确启用的项目/任务范围采集；经 Lumi CLI/server 持锁写入独立只读记录，保留旧 Codex 导入和原生聊天模型。开启测试采集只针对本步临时任务，不收集用户已有对话。

实现幂等、缺口提示、禁用停止采集、失败不阻塞聊天、桌面只读回看与 WS 驱动重读。交付 docs/features/cindy-plugins/session-archive.md、脱敏 fixture、必要 migration/测试和真实覆盖范围。新增宿主权限/测试安装按既有授权与正常流程处理；未获安装授权先交付可审阅包。不要把未实现的完整历史写成已完成，不运行 Cargo/Rust。
```
