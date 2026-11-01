# Step 0 · Cindy 关键行为实测记录

日期：2026-11-01。**Step 0 已完成记录与清理；未实测子项保留未验证。** 只新增本文件和 `spike/`，不修改 Lumi 产品代码，不进入 Step 1，不发布插件，不运行 Cargo/Rust。

## 环境与证据等级

本轮执行电脑是运行 `/Applications/Cindy.app/Contents/MacOS/Cindy` 的 Mac（宿主 PID `39732`），macOS `14.5` / build `23F79`，CPU `arm64`。只读 `check_app_update`、应用 Info.plist 均报 Cindy **0.1.121**。手机是否参与控制未验证，未用手机版本或 Codex 引擎版本替代宿主。

实际应用资源 `cindy-source.json`：`sourceCommit=0a7d5a419f5cd3e0fbebc9642dde8388376a68aa`，`builtAt=2026-10-30T01:16:08.865+08:00`。官方 `refs/tags/v0.1.121` 实际指向 `727afb1ad6902176e2ec55254154b900c489a998`，**二者不同**。按实际构建 commit 查询官方仓库 `nodeRuntimeBroker.ts` 返回 `404 No commit found for the ref`，因此未获得可核对的该构建公开源码，不能把 tag 或 main 当作实际源码。遵循 Skill 的来源不可核实降级路径：继续随包契约和运行时验证，源码支持结论保留未验证。[原始回执](spike/evidence/host-source.json)、[官方 tag](https://api.github.com/repos/makecindy/cindy/git/ref/tags/v0.1.121)。

已读根 AGENTS.md、plan.md、references.md、step0-spike.md 与当前 `cindy-plugin-creator`；应用资源与向本会话导出的 SKILL.md SHA256 都为 `cbdaae022b89900b546d3664a647b8174e444fbd057d04b4a9cbdd4fdaa7f7cc`。手册列出的接口仅作契约证据。Node 手册中“安装不会追加确认弹窗”与较新的打包章“首次安装列权限确认”有措辞差异；安装行为以真实 Forge 回执为准。

本次 CLI 取当前 Lumi `68235b5b4e9bc45d871895c8cb99ae4d6c0ba6e7`，Go `1.27.3 darwin/arm64` 构建。构建时只有新探针文件未跟踪，Go metadata 带 `+dirty`；不能据此宣称提交树全净。CLI 没有 `--version` 子命令，用 `go version -m`、源码 commit 和二进制 SHA 固定身份。shell Node `25.2.1` 仅用于本地测试。真实插件 worker 返回 Cindy 随包 **Node v24.19.0**、darwin/arm64、execPath 为 Cindy Helper，ppid=39732，与执行宿主对应（[回执](spike/evidence/host-environment.json)）。

证据等级：**D** 随包文档；**S** 精确版本源码（本次 Cindy 构建未取得）；**A** 本地真实 CLI/Node 自动化；**H** Cindy 真实宿主；**M** 真实模型生成。低等级不替代高等级。

## 结论总表

每项“通过”只覆盖证据列描述的范围；未执行的子项明确保留未验证。

| 项目 | 当前结论 | 证据与限制 | 后续影响 |
| --- | --- | --- | --- |
| 执行电脑/应用版本 | **通过（H）** | Mac arm64，进程、bundle、宿主版本三方核对 | 首轮仅覆盖该 Mac |
| 实际构建对应公开源码 | **未验证** | 构建 commit 与 tag 不同；实际 ref 404 | 不作精确实现/最低公开版本承诺 |
| 随包 Node 执行 Go CLI | **通过（H/A，arm64 本机）** | 随包 Node 24.19.0 执行 bin/lumi，mode 755，双 CLI 成功 | 运行门槛通过，分发架构/公证另测 |
| sessionContext | **通过（H，两个真实本地任务及伪造覆写拒绝）；只读/非本地未验证** | 父任务与临时 b 的 session/workdir hash 不同，Host 覆盖伪造值 | 不把伪造只读参数当真实只读任务 |
| 共享 worker / 多任务隔离 | **通过（H，真实两任务顺序调用及合成并发）；真实并发未验证** | 父任务/b 共用 boot；b 独立项目与服务，同时复用 a 服务；crash/更新/idle 后 CLI 均存活 | 不依赖 worker 退出回收 CLI |
| 工具、Manual、schema、16 上限 | **通过（H，本轮探针范围）** | info、Manual、分类目录、错误 schema/业务失败、17 项拒绝均有回执 | 产品工具仍需各自参数验证 |
| 授权媒体原件、参考图 | **通过（H/M，PNG 原件和回送）；Asset Store 未验证** | 两条生成路径→attachments→插件公开端点→Node，SHA 一致；images 回送成功 | 合法原件链路成立；Step 4 还需 Asset Store |
| Core 与引擎原生生图 | **通过（M/H，两条链路）；额度未知** | Core qwen-image-3/xd；原生 image_gen 不返回真实 provider/model；均完成附件交接 | 不宣传订阅生图或收费归属 |
| taskView / wake / 图片 / 主题 | **通过（H，真实页通信、wake、focus、light）；图片/dark/双页切换未验证** | 用户可见项目 a，页面返回 script-loaded/wake/reply；无手势 focus 被拒 | 通信可用，媒体页和产品 UI 仍需验收 |
| WS 提示桥 | **通过（H，mutation→页面→REST、重新 join）；通知续命失败** | revision 3→4 后 reason=ws-change；重新 join 后 reason=join；120 秒空闲回收 | 常驻逻辑页不等于常驻 Node，须明确连接生命周期 |
| 消息归档 | **通过（H，范围、自建历史、真实双向 hooks）；完整归档失败/不成立** | 普通任务不可读；hook 缺稳定消息 ID/seq；观察到用户前置、助手后置 | 仅有限归档；插话、中断、其他 hook 改写未验证 |
| 手机控制电脑 | **未验证** | 没有已验证的手机测试入口；computer/android provider 未启用 | 不承诺手机原生审阅 |
| 安装/更新/停用/卸载 | **通过（H，安装、四次同源更新、卸载）；单独停用未验证** | 0.0.1 installed；0.0.2–0.0.5 updated；正常卸载后 info/call 均 GHOST_NOT_FOUND | 卸载不保证回收 detached CLI，另行核查 |

## 1. Node、CLI、执行位与实例隔离

版本：上述 Lumi commit / Go 1.27.3，shell Node 25.2.1，探针 `0.0.1`。完整命令见 [README](spike/README.md)。复现：构建 CLI → 运行 `scripts/configure.py` → 运行 `scripts/local-check.cjs`。后者用同一 worker 并发初始化 a/b，再并发查询 a/a/b，以不同 UUID、不同服务 PID、同 a PID 复用作断言，finally 总会 stop。

实际输出：

```text
PASS: parallel isolated CLI servers/projects; same-project reuse; WS join; REST calibration; media chunk integrity
PASS: both probe CLI instances stopped
```

[完整自动化回执](spike/evidence/local-check.json)。首次运行受 Agent shell 沙箱限制，临时 CLI 日志为 `listen tcp 127.0.0.1:0: bind: operation not permitted`；经正常执行审批运行同一脚本后可启动。这个失败不是 Cindy Node 的失败。重复运行发现已存在项目初始化不会自动重新打开，探针增加显式 `POST /api/v1/open-projects` 后验证通过；未修改产品行为。

CLI 仅经显式 data-dir/cwd 运行，环境使用系统 PATH 和临时 HOME/TMPDIR。项目创建使用现有 `/api/v1/project-initializations`，避免 `lumi init` 写 Codex guidance。未修改主工程、真实作品或 SQLite schema。

Forge 包约 20 MiB，解压约 38 MiB，10 条目；CLI zip Unix mode `100755`。[包清单与 SHA256](spike/evidence/pack-0.0.1.json)。`codesign -dv` 显示 `Signature=adhoc`、无 TeamIdentifier；无 quarantine xattr。**未验证 Developer ID、公证、下载后 Gatekeeper、x86_64 或其他 OS**，本机可执行不等于正式分发可执行。

证据复核发现当前本地 0.0.1 包 SHA 与最初 pack 回执不同（Forge install 会重新打包，但本次未保存旧字节，不能证明仅 zip 元数据发生变化）。[当前包快照](spike/evidence/package-snapshot.json) 单列两者；不覆盖历史回执。0.0.2–0.0.5 与各自记录一致，全部包中的 CLI 字节与执行位一致。不要使用 0.0.1 旧 SHA 校验现在的文件。

真实 Host 的同一 boot 并发操作已通过（[回执](spike/evidence/host-parallel_projects.json)）；CLI SHA256 为 `88a8f2ec9664d9ad3ed3a09dca3f49aacf1bcf4125c53b1d69c92a81837dd62d`。主动 worker exit(73) 返回 PROCESS_EXITED，下一次调用产生新 boot。**两个 detached CLI 的 PID/nonce 保持，仍为 running:true**（[crash](spike/evidence/host-crash.json)、[重试](spike/evidence/host-after-crash.json)）。同源更新后再次换 boot，CLI 仍在（[更新后](spike/evidence/host-after_update.json)）。因此“worker 退出就回收所有原生 CLI”在本场景**失败**，产品需要显式管理共享 server 生命周期。Node 空闲 120 秒与通知续命已在 §4 补测；停用和同项目真实双任务退出仍未验证。合成并发与真实顺序会话分别列证。

## 2. 上下文、工具、Manual 与任务页

复现入口（安装确认完成后）：`ghost_info` → `ghost_manual` → `ghost_call(tool=list_tools)` → `ghost_call(tool=call_tool,args={name:context})`。记录宿主 `session_id/workdir_is_local/workdir_is_read_only`；workdir 保存存在性、SHA256 与脱敏占位，不能保存真实用户路径。原始 callId 只当次传递，不落盘/交页面。

`main.js` 只相信 Host 注入的 `args.session_context`；页面 session 是路由键，不能授权目录。`view` 在当前在途调用建立绑定；页面 `/wake` 后发带 reqId 的 BroadcastChannel 请求，重发仅等待同一请求的响应并去重，无周期业务查询；首次加载和 focus 校准。页面不直接连 loopback，不嵌入 MCP Apps。

Forge 首次两次失败：缺 `manual.items[].dir`、缺 `manual.items[].name`。修正清单后 pack 成功。另对独立且从不安装的 `spike/negative-17-tools` 调用 Forge pack，得到 `MANIFEST_INVALID: tools 必须是 1–16 项的数组`（[回执](spike/evidence/negative-17-tools.json)）。主机 `ghost_info` 可发现两个元工具和 probe Manual；[Manual 正文](spike/evidence/host-manual.json)、[分类目录](spike/evidence/host-catalog-runtime.json)、[错误 schema + RULES](spike/evidence/host-status-invalid.json) 与媒体 PROBE_FAILED 均已实测。可安装探针仅两个元工具。

[当前上下文](spike/evidence/host-context.json) 返回 session_id=`a04c93b4-6b67-4c15-b87a-0cdd22677748`、workdir SHA256=`14604bbd29f96dbd79af03c56a7401329328d78a0814e988a0b9371a6a20d18a`、local=true/read_only=false、callId 存在。两个真实本地任务/目录、页面 focus、wake 和 light 主题后来已验证，见 §4；只读/远程上下文、双任务页切换、真实手势主动 focus、页内图片和 dark 主题仍未实测。运行时目录返回 computer 与 android 为 `provider-not-enabled-for-session`，未为测试改变工具或批准配置。

## 3. 媒体与两条模型路径

探针已实现 `receive_media`：只从顶层 attachments 注入值读取本插件公开媒体端点；检查 20 MiB 上限、以 48 KiB 分块送 Node、核对接收大小并记录 SHA256/魔数/实际 MIME 与尺寸。测试数据写到独立 tmp，尚未导入 Lumi Asset Store，不能宣称 Step 4 完成。

`0.0.1/0.0.2` 在裸 hash 端点读图返回 404，改为已知 PNG 的 `/media/<hash>.png` 后 `0.0.3` 成功；这属于探针 URL 修正，不是改 Host 权限。`reference` 已把收到的原生原件寄存（deduplicated=true、hash/bytes 不变），并在 tool-result 顶层 images 返回，Agent 实际收到图片（[回执](spike/evidence/host-reference.json)）。Host 给模型的图片允许压缩，不能把模型上下文图片字节当原件；Lumi 原件以 Node SHA 为准。零哈希 404 只能证明未知资源拒绝，不能证明跨插件越权拒绝；不得借其他插件私有缓存寻找负例。媒体过期、跨插件归属、重复交接和大原件仍待测。

Core `media.list_models(image.generate)` 实际目录：`qwen/qwen-image-3` / provider `xd`，`qwen/qwen-image-3-pro` / `xd`，`openai/gpt-image-2` / `openai`。对 qwen-image-3 / xd 调用 prepare 已返回 `prepared`、Guide revision `2026-08-11.2`（[回执](spike/evidence/core-prepare.json)）；已使用同一 invocation_id 发起一次 request 并返回 complete（[回执](spike/evidence/core-request.json)），没有重复提交。

引擎原生 `image_gen` 实际生成了狐狸拿蓝书 PNG，`1254×1254`、`2,031,418` bytes，SHA256 `9bba276dc1d324b0e7c0d9362c90e1b5f40df0bbb35e33ec0298b02698a8b8f1`。已肉眼核对图像，复制到 [native-fox.png](spike/media/native-fox.png)，精确提示词和数据见 [回执](spike/evidence/native-image.json)。该接口仅返回图片与保存提示，没有实际 provider/wire model/billing 字段，所以这些值全部记录 **unknown**。没有申请 API key、没有用 CLI fallback。

两者是不同调用路径，不从模型名字或当前聊天引擎推断额度、Provider 账户或扣费。实际 `ghost_call.attachments` 已接受原生生成后复制进 spike 的本地文件；Host 注入 hash，再经插件公开媒体端点读取，Node SHA 与原文件一致（[原生交接](spike/evidence/host-native_v3.json)）。Core 受管 URL 同样交接成功，PNG `1024×1024`、`826,433` bytes，SHA256 `f09a1d3de26dbe010ed12cf9ff6594ea4b064f927c260ca41670e0b055b9652c`（[Core 交接](spike/evidence/host-core_v3.json)、[独立临时目录原件副本](spike/media/core-fox.png)）。无扫描缓存、无 resolve_local_path。

## 4. WS 桥接与生命周期

本地首次直接 `new WebSocket(ws://127.0.0.1:<port>/api/v1/ws)` 失败；现有 Lumi `OriginAllowed` 拒绝空 Origin（[实现](../../../internal/realtime/websocket.go:65)）。Node 的 browser-compatible WebSocket 不允许设置任意 Origin，探针改为经现有 CLI `creation-requests/read_app_connection` 取得仅针对一个项目的提示票据，再连同一路径。票据不交页面、不入证据文件，未修改 Origin/CSP。[票据范围](../../../cmd/lumi/app_connection.go:18)。

自动化和真实 Host 都收到 `phx_reply` / `payload.status=ok`（[Host join](spike/evidence/host-ws_connect.json)），信封含 topic/event/payload/ref/join_ref，项目 topic 使用 UUIDv7；其后 REST 读取 `open-projects` 成功。main.js 收到了 Node 转发的 join notification（[回执](spike/evidence/host-view_receipts.json)）。taskView open 成功，focus=true 无手势返回 PERMISSION_DENIED；0.0.1–0.0.3 页面回执始终 pages=[]（[最新](spike/evidence/host-receipts_v3.json)），不能判断实际页面是否可见/脚本是否运行。尚无真实 mutation → WS notification → main.js → taskView → REST 重读的闭环证据；探针页也未使用 TanStack Query，不能代替后续产品校准测试。

下述 0.0.5 补测覆盖真实页面、聚焦与重新 join；CLI 崩溃恢复、自动重连、停用仍未验证。不得用定时 HTTP 刷新补缺口。

### 0.0.4 补测（真实 Host）

复现：`ws_connect(slot=a)` → `calibrate` → `mutate` → `status`。mutate 仅对带 owner 标记的临时 a 项目写 description，带 expected_revision，不改名/目录。revision 1→2；[变更后回执](spike/evidence/ws-turn2-after.json) 的 `story:project_changed` payload 只有 `project_uuid` 与 revision，无内部 id。页面没有回执，因此这个结果仅证明 Node 连接收到真实业务通知，**没有证明页面触发 REST 重读**。

空闲实验使用另一个声明入口 `node/lifecycle.cjs`，以免主 worker 的调用干扰。调用 `lifecycle_start` 后不再调用该入口；它每 2 秒发 JSON-RPC notification，计划 80 次，每 5 次写证据。08:02:33.865 UTC 开始，最后日志为第 55 次、08:04:23.967；随后只读进程列表中 PID 89073 消失，08:07:44 的 `lifecycle_report` 返回新 boot、新 PID 6063、ticks=0。期间没有安装更新或再次调用该入口。[原始日志](spike/evidence/lifecycle.jsonl)、[观测说明](spike/evidence/lifecycle-observation.json)、[重启回执](spike/evidence/lifecycle-report.json)。**“后台通知可维持 on-demand Node 不被 120 秒空闲回收”失败（H）**；准确终止毫秒未捕获，不能由 10 秒间隔日志伪造精度。

主 worker 也在无请求时重启，detached CLI PID 93140 仍存活。[静置后回执](spike/evidence/ws-turn2-quiet.json) boot 已改变、读取计数归零，不能用于断言同一进程静置期间没有轮询。对长期 WS 连接的影响已经明确：需要重新建立订阅并校准；单纯保持插件逻辑页常驻不足以保证 Node 连接。无变化读取计数、实际页面重连仍待验证。

两个真实任务由 `task_create(slot=a/b)` 使用公开 `cindy.tasks.create({isolatedWorkspace:true})` 创建，再各调用一次 `task_send`。不同 taskId、workdir SHA 与运行身份均有回执（[a 创建](spike/evidence/task-create-a.json)、[b 创建](spike/evidence/task-create-b.json)），permissionMode 均为 ask，没有调整配置。两者停在首个 ghost_info 的权限确认，尚未执行 session_probe（[等待边界](spike/evidence/task-approval-boundary.json)）。预期后续分别记录当前 Host context、own_project 与 shared_project，并核对 boot/时间重叠；此时**不能宣称真实双任务共享 worker 或隔离通过**。

### 0.0.5 后续实测与最终边界

以上等待态随后收到 `user rejected MCP tool call`，两个初始 run 正常收尾，但业务探针未执行。没有自动重试。用户在 b 先明确允许重试 ghost_info，再明确允许 list_tools/session_probe；b 执行成功，之后用户要求只汇总结论、不再调用工具。[完整范围内回执](spike/evidence/host-v5-second-task_read.json)。a 保留未执行，不用 b 的成功填充 a。

真实父任务和临时 b 使用同一 boot=`2cd8d211-bd5f-4f47-aefe-b561372fb68e`；两个 session_id 与 workdir SHA 不同且 local=true/read_only=false。b 的独立项目 UUID=`01a19610-532f-7ea6-908f-5fc720e3d908`、PID 28069，与父任务使用的 a 项目 UUID=`01a19610-5331-7d1f-b989-e4a46c44effb`、PID 93140 不同；b 同时访问共享 a 时复用 PID 93140。[两会话证据](spike/evidence/real-session-isolation.json)。这是**两个真实会话的顺序隔离和共享服务通过**；两段在途调用未证实重叠，真实同时并发、其中一个任务退出对其他引用的影响仍未验证，合成并发另列。

用户确认页面可见且 focus 校准成功，首次 items=[]；对应 CLI 配置有 15 秒项目空闲宽限（[源码](../../../cmd/lumi/daemon.go:26)），worker 被回收后连接断开、项目可能被释放。该解释与证据一致，但没捕获那次具体 close 日志，故作为推断。重新打开项目并 join 后，用户再次确认手动校准显示正确 a UUID。实际 [页面回执](spike/evidence/host-v5-visible-receipts.json) 包括 script-loaded、wake=running、first-load/focus/manual、reply=ok，主题为 light；此前 pages=[] 不能据此判为 CSP 故障。

复现闭环：保持真实 taskView 打开 → `mutate(a)` → `view_receipts/status` → `ws_connect(a)` 主动关闭旧连接并重新 join → 再读回执。revision 3→4，页面收到 ws-change 并请求 REST 重读；business_reads 9→11（mutate 的 GET 和页面 GET 各一次）；重新 join 的新 join_ref 又触发一次 GET，计数到 12。[闭环校准](spike/evidence/ui-loop-calibrations.json)、[页面含 ws-change/join 回执](spike/evidence/ui-loop-view_receipts-1793520996730.json)。没有把 WS payload 当项目事实，也没有用 HTTP 定时业务轮询。这证明手动重建连接后的校准，**没有证明探针支持自动重连**。任务页原件图片展示、dark 主题与双任务页切换未实测；本页不用 TanStack Query，产品层仍须实现其失效/重读机制。

在父任务调用中人为传入假的 session_id、workdir、local=false、read_only=true，Host 返回仍是当前真实上下文（[回执](spike/evidence/context-forgery.json)）。这验证 Host 覆盖调用者伪造值；不等于测过真实只读或远程任务。

## 5. 消息归档与手机边界

初版没有 hooks；0.0.5 才按本步范围加入并主动启用临时任务白名单，不默认采集实际聊天。随包 D 证据：will-user 在提交前，will-assistant 为落库后异步处理；后续 hook 可再次改写，seq/dropped 不提供稳定 transcript 重放；文档明确部分 `/goal` 自动轮次与后台任务不投。`tasks.readMessages` 的契约范围是本插件自建本机任务，不是任意普通任务历史。

实测 [capabilities/readMessages](spike/evidence/host-archive_boundary.json)：targets 为 own-plugin-local-session，当前普通任务读取得到 Task not found；当前 SDK Error 没有返回可记录的 code，不自行补码，没有保存消息正文。后续 0.0.5 的用户/助手样本见下文；插话、中断、引擎失败、后续改写与缺口仍待测。Step 7 只能保留有限归档或手动导入方向。测试其他 hook 不得改动已有插件。

手机控制电脑尚未发起读取/导出；缺测试设备证据，不把桌面任务页当手机 UI。未来测试须同时记录实际执行电脑、临时目录与受管产物的交付位置。

0.0.4 实际 `readMessages/listRuns` 可读取本插件创建的 a/b 任务（[a](spike/evidence/task-read-waiting-a.json)、[b](spike/evidence/task-read-waiting-b.json)）：包含 user、assistant、tool_use，messages.items[].id、createdAt、inputMessageId 和 runId。重复读取尚未完成的相同内容观察到 ID 不变；不是对所有终态、编辑后消息或历史重放的保证。这里只保存合成测试提示，不读取其他用户任务；作为调用凭据传入的 invocationId 已脱敏，不用于归档主键。

0.0.5 hooks 包经正常确认更新成功（[回执](spike/evidence/update-0.0.5.json)）：launch=resident，声明 turn/session/activity 和两个消息钩子。默认不采集；`hooks_enable` 只把已登记的两个临时任务加入内存白名单。所有钩子先 allow，白名单之外不访问正文；白名单内只记录 data_keys、seq/dropped、可选 messageId、文本字节数/SHA256，不保存正文。Node 再核对 taskId。

已实际收到用户、助手、thinking 边界、approval 边界、turn-end、session-switched（[原始 hooks 元数据](spike/evidence/hook-events.jsonl)）。will-user / will-assistant 的 data_keys 都只有 model/sessionId/text，message_id=null，seq=null；did 事件有 seq。本次文本哈希能与测试历史比对，但不能充当消息身份，重复正文会碰撞成同一内容标识。用户 hook 时间 08:13:37.679 早于消息 createdAt 08:13:37.708；助手消息先记 08:13:25.765，hook 后至 08:13:26.984，与前置用户/后置助手契约一致。只证明观测顺序，不是原子提交或必达保证。已覆盖工具权限拒绝后的助手收尾，**未覆盖引擎失败终态、插话、中断、丢事件重放或其他 hook 链式改写**；不修改其他插件制造样本。完整普通任务归档不成立，Step 7 仍只考虑有限归档或手动导入。

## 6. 安装、更新与清理台账

- 独立 ID：`lumi-step0-probe-20261101`，命令 `lumi-step0-probe`；不使用产品 `lumi` ID，不改旧插件。
- Forge scaffold/pack、用户正常安装确认已完成；[0.0.1 安装](spike/evidence/install-0.0.1.json)回 action=installed，instanceId=`_root__lumi-step0-probe-20261101`、namespace=null、enabled=true。
- 后续更新只能从 `spike/probe` 作者目录经 Forge，保留确认后的同一实例；不编辑 Host 安装目录或批准文件。
- 同源 0.0.2–0.0.5 均回 updated、同一实例、enabled=true（[最新](spike/evidence/update-0.0.5.json)）；新增 hooks 走正常确认，探针未设置 KV，因此数据保留尚未独立断言。最终用户从 Cindy 插件详情正常卸载唯一的 root 探针 0.0.5；没有重新安装。
- 本轮两个 CLI 均已再次显式 [stop](spike/evidence/final-stop.json)，[status](spike/evidence/final-status.json) 都是 exit_code=3/running=false；只读 ps 中 PID 93140/28069 均消失。卸载后还需防止页签 focus 意外重开而再次复核。
- 两个隔离 Cindy 测试任务 a=`dc429a41-0051-47b4-a91a-d7fefe0d4ce4`、b=`2804af10-ad08-4ff0-97d1-b4d1f4288bca` 均 completed 后仅这两项被[可逆归档](spike/evidence/archive-temporary-tasks.json)，保留历史数据；a 未执行，b 经用户重新授权成功。hooks 也收到两个 did-session-archived。未改其他历史任务。
- 卸载后 [ghost_info](spike/evidence/uninstall-discovery.json) 与一次 [ghost_call](spike/evidence/uninstall-call.json) 均为 GHOST_NOT_FOUND。公开工具无卸载入口、computer provider 未启用，所以请用户操作正常插件详情；没有直接删 Host 目录或修改批准配置。单独“停用但保留安装”未测，不用卸载冒充停用验收。
- 卸载后仍发现新 PID 52274：最后 stop 之后页面曾再次校准并启动临时 a。用仓库保留的同一 CLI、同一 a/data-dir 执行自身 stop；Agent shell 首次因 loopback connect 沙箱限制失败，经正常执行审批后成功（[回执](spike/evidence/uninstall-cli-stop.json)）。[最终 ps](spike/evidence/uninstall-final-processes.json) 无探针 worker/CLI 匹配；没有按 PID 强杀、没有删锁文件冒充释放。此例再次证明卸载不保证回收 detached CLI。
- [清理回执](spike/evidence/cleanup.json)：保存最终日志后，核对 owner/真实路径，仅删除本次临时根、专用 Go cache、作者目录生成的 probe-config.json。仓库保留源码、证据、两张原件，以及被 Git 忽略的五个审阅包/CLI；复现需重新 configure、pack，旧包配置指向已删除临时根。两个已归档测试任务与 Host 受管生图保留，不扫描或删除 Host 私有媒体缓存。

## 对 Step 1–7 的影响

| 步骤 | 当前门槛与下一步 |
| --- | --- |
| 1 | 随包 CLI/两个真实上下文、顺序项目隔离通过；实际并发/引用回收仍待测；必须显式管理服务生命周期；尚不开始 Step 1 |
| 2 | 未开始：标准项目可在 tmp 初始化；不得照搬 `lumi init` 的 Codex guidance；绑定可信上下文仍待测 |
| 3 | 未开始：两段工具、Manual 与错误契约已取得 Host 样本；产品业务工具另验 |
| 4 | M→授权 attachments→原件 SHA 有两条链路成立；Lumi Asset Store 导入仍未验证，不把临时文件复制当产品导入 |
| 4.5/5 | 页面通信、聚焦、真实 WS→REST 与手动重新 join 通过；通知不续命已证，须设计 Node 连接恢复；图片/dark/双页/产品 Query 仍待验 |
| 6 | pack、安装、同源更新、卸载通过；单独停用/正式签名与其他架构待验，最低正式版本不能由不匹配的 tag 猜测 |
| 7 | 未开始，默认不采集；不承诺普通任务完整历史、可靠消息队列或零缺口归档 |

本次只完成 Step 0，不自动进入后续步骤。每个未验证子项都需要对应真实条件，不能据通过项推定通过。

同一 worker 的静置区间没有新增业务 GET（[时间与计数](spike/evidence/no-poll-observation.json)）；只证明该观测窗口，不声称任意条件都无轮询。最终记录检查命令 `python3 docs/features/cindy-plugins/spike/scripts/verify-records.py` 校验 JSON/JSONL、两张原件 SHA、当前包/CLI 字节与执行位、JS 语法、文档链接和任务凭据脱敏；不把这些静态检查等同于未执行的 Host 能力。
