# Cindy 文本创作工具契约

Step 3，2026-11-01。执行 Host 为 Cindy 0.1.121，构建 sourceCommit `0a7d5a419f5cd3e0fbebc9642dde8388376a68aa`；对应公开源码仍返回 404，依当前随包 Skill 与该应用包构建产物核对接口，不以 main 或其他 tag 冒充匹配源码。已核对 Host 错误码格式和失败时只转发 errorCode/message 的行为，因此自纠 schema 与 RULES 也进入 message。共用业务参数及返回见 [现有工具契约](../codex-plugins/tools.md)；权威 schema 为 `internal/textmcp.Specs()`，构建时生成 `cindy-plugins/lumi/node/business-specs.json`，不修改既有 MCP 工具注册或业务参数。验证记录见 [Step 3 验收](acceptance/step3-authoring-tools.md)。

## 顶层目录与允许列表

manifest 仅注册两个顶层工具：`list_tools({category?})` 和 `call_tool({name,args?})`。前者不传 category 返回类别名称、数量和状态；指定类别返回工具名称、说明、完整 input_schema（含条件必填）和必要 RULES。后者只执行明确允许列表中的名称，args 为 JSON 对象，省略相当于 `{}`。

| 类别 | 当前工具 | 状态 |
| --- | --- | --- |
| project | project_init、project_overview、project_setup、runtime_status、runtime_release | 可用 |
| story | story_profile、chapters、book_outline | 可用 |
| settings | settings | 可用 |
| pages | pages | 可用 |
| prompts | prompt_catalog、render_prompt | 可用，限定文字目标 |
| media | 无 | not_implemented |
| review | 无 | not_implemented，文字审阅使用业务 list/get |
| export | export_book | 可用，原生任务文件通道已实测 |

允许列表固定在 `node/catalog.cjs`。新 MCP 工具不会因出现在 schema 快照或 tools/list 中而自动开放。禁止 import_image、setting_boards、apply_setting_breakdown、read_app_connection、watch_app_changes、read_app_image 和其余未列工具；不接收任意 MCP method、CLI、HTTP、路径、SQL 或项目绑定。

## 有意差异

- Cindy 只声明目录与分发器，原业务工具通过 call_tool 进入；runtime 两项也移入 project 类别。
- project_init、project_overview 和 project_setup 沿用 Step 2 的公开状态投影，内部路径与实例身份留在 Node；其他业务返回沿用 Lumi 信封。写入结果可能是既有业务摘要，完整正文须 get/list 重读核对。
- render_prompt target 仅提供 setup、story_profile、chapter、pages、page、setting、premise；媒体目标及 board_uuid 在后续媒体交接完成前拒绝。传入的目标和上下文 UUID 先通过当前项目的业务 get 核对，不沿用共享渲染器对未知上下文 UUID 静默过滤的行为。其余参数、变量与业务输出不另造协议。
- 业务目录新增 `premise/text_settings` 文字设定任务，中英文默认模板由 promptcatalog 管理，支持项目覆盖；不把图片生成或拆图模板用于无图文本设定。
- 不启用 MCP App 审阅界面。book_outline 和业务 list/get 支持当前 Agent 串行文字审阅。
- export_book 原业务返回的 uuid、format、byte_size、mime_type 等保留，path 改为当前项目可见副本的相对路径 `lumi-exports/text-<uuid>.<ext>`。仅从绑定项目标准 `.lumi/exports/` 中复制成功回执对应的完整文件；拒绝链接、目录替换、大小变化及覆盖，流式复制后原子发布。不新增任意路径读取或命令能力。新增 file_generated、user_received 和 delivery 字段，见下文。

## 读写授权

Node 校验 schema 后按 action 判定权限，业务派发之前拒绝只读写操作；不能靠名称前缀或先前任务权限授权。

| 工具 | 只读业务操作 | 写操作 |
| --- | --- | --- |
| project_init | 无 | 初始化（即使已有项目亦要求写权限） |
| project_setup | get | update、finalize |
| story_profile | get | write |
| chapters | list、get | create、update、write、delete |
| settings、pages | list、get | create、update、delete、select_image |
| project_overview、book_outline、prompt_catalog、render_prompt | 全部 | 无 |
| export_book | 无 | 所有格式均落盘 |
| runtime_status、runtime_release | 诊断、当前任务连接释放 | 无作品写入 |

只读业务使用附加的 `lumi mcp --read-only` 模式，仅连接已运行的 CLI、读取该 CLI 已打开的项目；不会启动服务、打开项目、迁移或恢复 STORY.md 投影。服务端再次按操作拒绝写入；Manager lease 内固定 SQLite 连接并启用 query_only，结束及取消后清理，避免懒加载意外写入。权限变化取消旧在途调用并重新校准绑定，读写代理分开。项目未打开时返回 read_only_project_not_open，先由可写任务打开；离线时不会创建锁来探测状态。普通 MCP 默认模式、公开业务 schema 与旧接口保持。

业务修改仍由既有 story/production/setup 服务发出 `/api/v1/ws` 公开 UUID 提示，原 CLI web UI 按事件使 TanStack Query 失效，再通过 REST 重读 SQLite 事实；不在插件增加定时业务查询。首次 join、重新 join 与窗口聚焦校准沿用原客户端流程。

## 错误、结果与恢复边界

未知工具返回 unknown_tool、允许名称和 list_tools 入口；非法 action、额外字段、错误类型、UUID 格式、缺少条件必填、数组/字符串/数字边界在业务派发前拒绝。错误结果附 correction，包含正确工具 schema、完整 RULES 与类别发现入口。跨项目 UUID 再由既有业务资源查询拒绝。

Lumi 保持 `{success,data,error?}`，MCP IsError 必须与 success 对应。main.js 遇 success:false 以 Host `ok:false`、`errorCode=LUMI_<大写业务码>`、可行动 message 交卷（Host 插件错误码只接受大写；message 保留原始小写业务码），附带 schema 和规则自纠；不会将失败当作 Host 成功。Host 网络/Node 失败同样失败。取消和超时等未知写入结果不得自动重放；共享服务身份变化也先重读。

输入经 Node transport 限制为 256 KiB，MCP 请求参数为 240 KiB（含工具包装与项目 UUID）；后者在连接、写入派发前检查，arguments_too_large 附 schema/RULES，不误报未知写入结果。业务和 MCP 结果沿用 `textmcp.MaxBytes=256 KiB`，包含 MCP text/structuredContent 的完整编码开销。Node 单行保护约 1 MiB。超限失败，不截断正文。列表沿 cursor_pagination 缩小 limit；提示词缩小上下文；单项仍超限时使用完整文本导出。

当前 Host 编译函数审计用 125087 字节的构造输入验证 dispatcher → ghost_call → Agent MCP text 结果完整，并验证失败的 isError、错误码和 schema 消息转发。该测试模拟 transport 与未使用的媒体辅助函数，不冒充已安装插件 E2E。手机工具记录另有 8 KiB 展示投影，超限明确附截断标记，不能拿来核对完整正文；原生任务文件通道独立读取全部字节，已用真实随包文件服务核对 SHA256，并取得用户手机完整打开确认。

## 文件生成与用户接收

成功导出返回 `file_generated:true,user_received:false,delivery:{status:"prepared",channel:"cindy_task_files",relative_path,byte_size,sha256,receipt:null,instructions}`。在当前任务文件列表打开 relative_path，手机使用 Cindy 原生远程文件浏览器；prepared 不代表控制端已经读取。接收确认必须来自真实读取回执或用户明确确认，插件没有伪造自动手机回调。不能承诺 Mac 绝对路径或聊天本地链接在手机可下载。

原文件已生成但可见副本准备失败时，以 Host 失败 export_delivery_failed 返回 correction.generated_export（uuid、format、大小、项目内原路径、file_generated:true、user_received:false），delivery.status=failed。保留原文件并先核对，不自动再次导出。调用取消、身份变化等不能确认结果时仍按 result_unknown 先重读。

完整流程由 [插件创作 Manual](../../../cindy-plugins/lumi/manual/authoring/MANUAL.md) 承载，不复制 promptcatalog 模板。工具只读取渲染任务、保存当前 Agent 输出；CLI 不调用 Lumi 模型或生图 pipeline。

## 验证范围

新增 `catalog.test.cjs` 覆盖发现与状态、allowlist、schema 错误修正及所有 action 的读写分类；`main.test.cjs` 覆盖两顶层协议、可信上下文与 Host 失败映射。`authoring.test.cjs` 用隔离真实 CLI 完成无服务商 setup、故事/章节/角色写回、三页正文与画面描述，逐项核对 Markdown 导出内容；同项目双任务冲突保留内容，跨项目 UUID 被拒绝。

另有 Go 专项验证已提交故事读取、不恢复外部 STORY.md、拒绝打开已关闭项目、拒绝所有写操作及取消后的可写连接恢复。只读读取也纳入真实 CLI Node 端到端。

另覆盖真实 CLI 大结果拒绝、缩小 limit 后逐页恢复三个十万字节正文、缩小提示词上下文、已生成文件的交付失败恢复、完整文件 SHA256 与原子发布。WS 到既有 REST UI 的 Go/前端回归及 Forge 包校验见验收记录。

该端到端是固定输出模拟，**不是实际 Agent 自主创作验收**，后者留到 Step 6。文件接收有独立用户手机确认；编译 Host 函数审计、原生文件服务实测及用户确认分别记账。插件未安装、未发布，正式最低兼容版本仍须发布证据。
