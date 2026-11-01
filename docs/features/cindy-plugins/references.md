# Cindy 插件开发依据与证据边界

## 核对基线

2026-11-01 通过 Cindy 只读 `check_app_update` 得到当前执行电脑版本 `0.1.121`；官方 `refs/tags/v0.1.121` 指向 commit `727afb1ad6902176e2ec55254154b900c489a998`。本计划查阅了随宿主提供的 `cindy-plugin-creator` Skill，并核对该 commit 的公开桥接、Node 请求处理、taskView 处理与部分测试，以及媒体、打包、移动页面和 Manual 文档。

这是**文档与源码核对**，没有运行 Lumi Cindy 插件探针。没有据此确认生图扣费、图片原件交接、CLI 随包执行、远程手机页面或完整聊天归档已经可用。

执行时先核实插件运行电脑，而非手机或底层 Codex/Claude 引擎的版本。若发现真实构建 `sourceCommit` 与 tag 不同，按 [源码核对规则][source] 重新固定来源；不得用 `main` 替代未知版本。

## 官方开发文档

以下均固定在上述 commit；实施时仍须阅读执行宿主当前随包 Skill，按真实版本处理差异。

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

## 已核对的源码入口

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

## 必须保留为待实测的结论

1. 完整 `.cindy` 包中 Go CLI 的体积、执行位、签名、公证与每个目标架构的启动结果。
2. 同一 worker 下两个项目并发调用、停用/升级/崩溃回收、共享 CLI 生命周期。
3. 授权媒体从 Cindy 原件到 Lumi Asset Store 的字节链；参考图回送、订阅与 Core media 路径分别验证。
4. taskView 的图片加载、后台唤醒、WS 变更转发、重连与聚焦校准。
5. 手机控制 Mac 时的执行归属与产物交付；手机审阅页不在基础支持范围内。
6. hooks 的真实投递范围、消息身份、失败/中断/后续钩子改写行为；普通用户任务能否可靠归档。

各步骤将结果写进各自的验收记录；在记录出现之前，本表不改成“已验证”。

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
