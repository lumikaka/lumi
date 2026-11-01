# Cindy 插件开发依据与证据边界

## 核对基线

2026-11-01 的 Step 0 已完成独立探针实测与卸载清理，详见 [实测报告](spike.md)。下列三种依据分别记录，不互相替代：

| 依据 | 固定身份 | 可支持的结论 |
| --- | --- | --- |
| 历史公开源码参考 | 官方 `refs/tags/v0.1.121` → `727afb1ad6902176e2ec55254154b900c489a998` | 本计划曾核对该 commit 的公开桥接、Node、taskView、订阅/任务处理与部分测试；不是实际构建源码证明 |
| Step 0 实际执行端 | macOS 14.5、arm64，Cindy `0.1.121`；应用 sourceCommit=`0a7d5a419f5cd3e0fbebc9642dde8388376a68aa`，builtAt=`2026-10-30T01:16:08.865+08:00` | 进程、bundle、版本及构建记录核对通过；该 ref 的公开源码查询返回 404，精确源码与最低正式兼容版本仍未验证 |
| 随包契约与运行证据 | 当前导出的与应用内 `cindy-plugin-creator` SKILL.md SHA256 相同；探针 0.0.1–0.0.5、Host Node `v24.19.0` | 仅支持下文及 spike.md 中实际测试到的行为，不外推至产品、其他架构或其他版本 |

[宿主与源码回执](spike/evidence/host-source.json)、[Node 运行身份](spike/evidence/host-environment.json)。探针 CLI 使用 Lumi commit `68235b5b4e9bc45d871895c8cb99ae4d6c0ba6e7`、Go `1.27.3`；完整版本、原件与包指纹见报告。

后续执行仍须核实真正运行插件的电脑，而非手机或底层引擎版本。按 [源码核对规则][source] 固定实际来源；无法取得匹配源码时使用随包契约和运行证据并保留限制，不以 tag 或 main 替代。Step 0 探针已卸载、临时根已删除，重跑需重新配置并打包，不能直接复用旧审阅包的临时路径。

## 官方开发文档

以下链接保留在历史公开参考 commit `727afb1ad6902176e2ec55254154b900c489a998`；它们不是实际构建 `0a7d5a4…` 的实现证据。实施时先阅读执行宿主当前随包 Skill，结合实测处理差异。

| 主题 | 文档 | 对本项目的约束 |
| --- | --- | --- |
| 来源与设计 | [版本核对][source]、[设计对齐][design]、[Manifest][manifest] | 先核对执行宿主；新包 v3；版本下限用真实验收和正式发布证据确定 |
| 工具与工作流 | [工具发现][discovery]、[Manual][manual] | 顶层工具最多 16 项；大工具面用 `list_tools/call_tool`；工作流放 Manual |
| 当前任务 | [sessionContext][context]、[在途调用][call] | 只信 Host 上下文；local/read-only 分开处理；callId 不可保存或交给页面复用 |
| 本地程序 | [Node worker][node]、[沙箱边界][sandbox] | main.js 与 Node 分离；worker 跨会话共享；Node 是可信本机代码 |
| 审阅界面 | [taskView][taskview]、[面板通信][panels] | 包内 HTML；BroadcastChannel 通信；页面不直接访问 SQLite/CLI/loopback |
| 媒体 | [媒体模型与交接][media]、[运行时与媒体寄存][runtime] | 当前 Agent 执行生成，附件显式过户；旧示例不取代专门媒体契约 |
| 存储 | [文件与作品库][files] | 私有 data 卸载回收；项目事实继续保存在用户项目目录 |
| 消息与任务 | [事件订阅][events]、[消息钩子][hooks]、[任务能力][tasks] | did 事件不提供全文；hooks 有范围/超时；readMessages 受插件任务归属限制 |
| 分发 | [打包安装][packaging] | 随包原生程序须预置执行位；不从已安装目录开发；打包不自动安装/发布 |
| 手机扩展 | [移动页面][mobile] | 独立页面与桥接、48 KiB 消息限制、mobilePageId 来源保持；不能直接复用桌面 taskView 的支持结论 |

## 历史参考 commit 中已核对的源码入口

以下为阅读所得，没有运行这些上游测试，也没有取得实际构建对应源码；Host 实测另列。

- [公开 preload 桥][preload]：`node.request`、`taskView`、`tasks.readMessages` 等入口存在；单有入口不证明特定权限或操作可用。
- [Node broker][broker]：已启用且声明 `node` 才处理请求，支持 JSON-RPC 与 MCP stdio；完整生命周期仍需实测。
- [taskView handler][view-handler] 与 [taskView tests][view-tests]：按任务打开、peek、真实手势聚焦与拒绝条件。未执行这些测试，不把阅读测试视为本次通过。
- [订阅网关][gateway]：用户和助手钩子、裁决与失败处理；不能外推为完整 transcript 或可靠消息队列。
- [任务能力处理][task-handler]：能力面标注 `own-plugin-local-session`，不能读取任意用户任务历史。

## Lumi 现有实现入口

| 可复用内容 | 入口 | 规划注意点 |
| --- | --- | --- |
| CLI / 实例 / 项目初始化 | [CLI 文档](../codex-plugins/cli.md)、[cmd/lumi](../../../cmd/lumi) | `lumi init` 当前还调用 Codex guidance；Cindy 入口必须隔离这些副作用 |
| 创作工具与 revision | [工具契约](../codex-plugins/tools.md)、[internal/textmcp](../../../internal/textmcp) | 保留统一业务信封、提示词来源和版本冲突规则 |
| 项目锁和目录格式 | [internal/project](../../../internal/project) | Node 不另开 SQLite 连接；旧桌面项目可直接打开 |
| 图片导入和导出 | [internal/production](../../../internal/production) | 复用原图保留、比例处理、历史版本和格式验证 |
| 四种审阅界面 | [web/mcp-app](../../../web/mcp-app)、[当前验收记录](../codex-plugins/mcp-apps.md) | 组件可复用，MCP Apps 桥接和宿主证据不可直接迁移到 Cindy |
| Codex 会话导入 | [当前交付范围](../codex-plugins/session-import.md)、[internal/sessionimport](../../../internal/sessionimport) | 现为手动 rollout 导入，不能把 Cindy 文本伪装成 Codex rollout |

## Codex 兼容验证入口

以下来自当前仓库的代码与测试入口核对，不表示本次文档修订已经执行产品回归。按实际共用代码变更选择检查，并在各步记录命令、结果与未覆盖范围。

| 共用部分 | 现有验证入口 | 新适配的隔离要求 |
| --- | --- | --- |
| CLI / MCP / guidance / 实例 | [CLI 测试目录](../../../cmd/lumi)、[初始化测试](../../../internal/project/init_test.go) | 保留默认初始化与配置合并；补 Cindy/Codex 共用服务及释放隔离场景 |
| 创作工具 / MCP Apps 资源 | [工具服务测试](../../../internal/textmcp/service_test.go)、[App 契约测试](../../../internal/textmcp/app_test.go)、[WS 测试](../../../internal/textmcp/app_watch_test.go) | 保留旧调用方 schema、结果、资源元数据与版本保护，不引入 Cindy 必填上下文 |
| 图片与导出 | [MCP 图片测试](../../../internal/textmcp/image_test.go)、[宿主图片导入测试](../../../internal/production/host_image_test.go)、[PDF 导出测试](../../../internal/production/export_pdf_test.go) | Cindy 附件校验在适配入口；旧导入方式和素材/导出语义继续可用 |
| 审阅组件与构建 | [MCP App 测试](../../../web/mcp-app/app.test.js)、[构建脚本](../../../web/mcp-app/build.mjs)、[前端命令](../../../web/package.json) | 既有 `build:app` 写入 `internal/textmcp/appdist/review.html`；Cindy 使用独立产物，修改共用组件后验证 Codex 测试与构建 |
| 可选会话归档 | [导入测试](../../../internal/sessionimport/service_test.go)、[rollout fixture](../../../internal/sessionimport/testdata/README.md) | 保留 Codex 记录、幂等导入与项目迁移兼容 |

自动化客户端可以验证 MCP 协议与共享服务行为，但不能代替真实 Codex 宿主的加载、交互和完整流程验收。两者分开报告，具体门槛见 [plan.md](plan.md#codex-兼容门槛)。

## Step 0 实测结果与后续验收边界

本表只描述 2026-11-01 的测试范围；详细复现步骤和证据等级见 [spike.md](spike.md)。未列为通过的能力不因接口存在而自动成立。

| 项目 | 已取得的证据 | 后续仍须验证 |
| --- | --- | --- |
| Node / CLI | Host Node 执行随包 Go CLI；本机 arm64、CLI 字节与执行位、包体核对通过 | `lumi mcp` 初始化/取消/协议适配；签名、公证、其他架构与最低正式版本 |
| 上下文与项目 | 两个真实本地任务的不同 session/workdir hash、顺序隔离与共享服务；同 worker 合成并发；伪造上下文被 Host 覆盖 | 真实同时并发、取消/退出引用回收、只读与非本地上下文；产品初始化/绑定与桌面互通 |
| 生命周期 | worker 崩溃/更新后可重启；按需 Node 后台通知未阻止 120 秒空闲回收；detached CLI 在 worker 退出/卸载后仍可能存活 | 产品归属与回收策略、自动发现/恢复、关闭页面后不再拉起服务；不依赖逻辑页 resident 保证 Node 常驻 |
| 工具与 Manual | info、分类目录、Manual、错误样本与 17 顶层工具拒绝 | 产品业务 schema、读写授权、错误映射和版本保护；不照搬探针的简化实现 |
| 原件与参考图 | Core 与引擎原生生成 PNG 均经授权交接，Node 原件 SHA256 一致；参考图回送 Agent 通过 | Asset Store 导入/导出、参考图实际编辑、其他格式、大原件、过期、跨插件归属；额度来源未知 |
| taskView / WS | 真实开页、wake、focus、light 主题；mutation→页面→REST、手动重新 join 校准；113.684 秒静置未新增业务 GET | 产品取图、dark、双页切换/隔离、自动恢复、TanStack Query；已测窗口不代表任意状态无轮询 |
| 消息 | 自建任务历史可读、普通任务不可读；真实双向 hooks，用户前置/助手后置，消息 hook 缺稳定消息 ID 与 seq | 插话、中断、引擎失败、后续 hook 改写、丢失/补读；完整增量归档不具备条件，有限记录/公开导入仍须设计验收 |
| 安装与清理 | 首次安装、四次同源更新及正常卸载；新增 hooks 走确认；卸载后发现残留 CLI 并另行 stop，最终进程为空 | 单独停用/启用、产品数据保留与升级兼容、正式分发；卸载回执不能代替进程检查 |
| 手机控制 Mac | Step 0 未取得手机读取/导出与交付的专项验收证据 | 手机产物交付；独立手机审阅页不在基础支持范围内 |

需要落实到后续提示词的具体约束：

- Step 0 实测 CLI serve/status 加 REST/WS，不是 `lumi mcp` 适配。已有项目初始化成功后仍须确认已打开；worker 恢复要重新校准绑定和项目状态。
- Node 的无 Origin WS 连接曾被 Lumi 拒绝，探针使用现有单项目提示票据接入 `/api/v1/ws`；票据只留 worker，不交页面或日志，不放宽 Origin/CSP。
- 探针只为已知 PNG 裸指纹补 `.png`；产品必须遵守当前媒体地址契约并核对真实格式，不能推广此后缀假设。未知哈希 404 不能证明跨插件权限隔离。
- Forge install 重新打包。0.0.1 当前包与首次 pack SHA 不同且旧字节未保留，因此无法证明仅元数据变化；后续分别保存审阅包与安装生成包并核对条目/指纹（[当前快照](spike/evidence/package-snapshot.json)）。
- 消息正文 hash 不能作为唯一消息键；保留真实重复文本。只有新增稳定身份、提交事实与可靠补读证据后才扩展增量归档；基础包不照搬测试 hooks/自建任务权限。

各步骤保留自动化、真实宿主、真实模型的证据区分；只补测新增适配或受变更影响的行为。Step 0 完成不自动启动 Step 1，也不免除 Step 4.5 和完整产品验收。

[source]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-source-verification.md
[design]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-design.md
[manifest]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-manifest.md
[discovery]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-tool-discovery.md
[manual]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-manual.md
[context]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-session-context.md
[call]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-agent-call-context.md
[node]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-node-worker.md
[sandbox]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-sandbox.md
[taskview]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-task-view.md
[panels]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-panels.md
[media]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-media.md
[runtime]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-runtime.md
[files]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-files-and-library.md
[events]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-event-subscriptions.md
[hooks]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-message-hooks.md
[tasks]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-agent-tasks.md
[packaging]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-packaging.md
[mobile]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/resources/system-skills/cindy-plugin-creator/references/plugin-mobile-pages.md
[preload]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/src/preload/ghostPreload.ts
[broker]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/src/main/cindy-brain/nodeRuntimeBroker.ts
[view-handler]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/src/main/cindy-brain/taskViewSlot.ts
[view-tests]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/src/main/cindy-brain/__tests__/taskViewSlot.test.ts
[gateway]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/src/main/cindy-brain/subscriptionGateway.ts
[task-handler]: https://github.com/makecindy/cindy/blob/727afb1ad6902176e2ec55254154b900c489a998/apps/desktop/src/main/cindy-brain/taskSlot.ts
