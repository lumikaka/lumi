# Cindy 创作记录手动归档

2026-11-02：已交付公开文件手动导入这一受限路径：解析器、独立项目存储、CLI/server 与桌面只读展示均已实现。真实公开样本通过解析验证；合成样本通过真实 CLI/server、WS/REST、桌面重新打开，以及实际浏览器 1200/390/320 px 展示与 54 条文本分页验证。开发版 0.4.0 已原位更新到 d3452b2a…，宿主诊断核对新构建成功。默认不自动采集，未增加 hooks、subscribe 或任意任务历史权限，也未将真实聊天写入用户项目。此交付不包含自动增量捕获、完整 transcript 或 Step 5/6 的未通过项。

## 已验证的公开来源

用户明确允许只导出本次验收任务的用户与助手文本，并选择保留 Auto、先完成代码和归档验证。通过 cindy_helper 的 get_current_session_id 核对当前任务，再用 get_chat_history 的明确 session_ids、roles=[user,assistant] 和固定 to 查询；没有扫描其它任务或读取 Cindy 私有数据库。

本次实际返回稳定的消息 id、sessionId、role、content、createdAt。正文观察到字符串，以及含 text、images、files、slashCommandRanges 的对象两种形态。以同一截止时间分别单页读取和按 20/20/6 三页游标读取，共 46 条，ID 集合完全一致，无重复。解析后保留 45 条文本，明确省略 1 条注入上下文；没有输出真实正文或把它复制到仓库 fixture。

真实样本仅保存在 `/private/tmp/lumi-cindy-archive-20261102/public-history.json`；目录 0700、文件 0600。该临时文件不是随项目交付的正式归档，不能依赖其永久存在。仓库中的 [字段 fixture](../../../internal/cindyarchive/testdata/public-history.json) 使用合成正文、标识、路径和日期，仅保留实际观察到的字段结构。

结果见 [公开导出证据](acceptance/step7-evidence/public-export-20261102.json) 与 [验证命令](acceptance/step7-evidence/verification-20261102.json)。本次发生过解析器拒绝未知 to 字段的格式失败；随后根据实际回执补齐该字段，并要求与导出截止时间严格一致，没有放开未知字段校验。

## 文件格式和边界

文件格式名为 lumi.cindy-public-history，version=1，由当前 Agent 将公开 API 请求与选定响应字段包装而成。它是 Lumi 的手动导出包装格式，不是 Cindy 官方文件导出格式，也不是 Host 签名或读取权限凭据。

顶层记录 source、source_session_id、captured_at、cutoff_at 和 pages。每页保留 request 与 response；正文只保留所选任务的 user/assistant content，去掉 agentMeta、工具参数/结果、来源链接等非必要字段。读取时仍须由调用方独立提供明确选择的来源任务，不能以文件自报任务或 workingDir 获得项目访问权限。

完整仅表示这一公开文本投影的分页结束。to 筛选消息创建时间，不提供服务器原子快照，也不能证明后来没有编辑、rewind 或删除。它不覆盖 reasoning、系统/开发者消息、原始工具轨迹、图片/附件、权限卡、交互卡片或所有历史投递阶段。文件自报 complete 或文字 hash 都不能替代消息 ID 与游标校验。

## 已实现的解析器

[internal/cindyarchive](../../../internal/cindyarchive/export.go) 仅解析调用方显式提供的 Reader，没有目录扫描、Host API 调用、数据库写入或自动启用行为。

- 限定一个明确选择的 Host 任务；来源标识按不透明字符串处理，不伪装成 Lumi UUIDv7。
- 核对每页请求和回执的任务、角色、截止时间、顺序、limit 和游标，拒绝缺页、范围扩大、回执来源变化及乱序。
- 按来源消息 ID 去重；同文本不同 ID 都保留。同 ID 正文、角色或时间变化时失败，不静默覆盖。
- 正文保持首尾空白；明确省略已知注入上下文、非文本附件与内联媒体，返回省略计数和警告。来源正文 hash 仅用于变化检测，不作为消息身份。
- 未知格式/正文形态、重复 JSON key、无效 UTF-8、过深嵌套、超大文件或正文均失败；16 MiB 文件、256 KiB 单条正文、20000 条记录、1000 页上限，不截断后冒充完整。
- 错误只包含固定代码，摘要只包含计数，不输出正文、标题、来源标识或路径。底层读取错误也会转换为固定错误码。

只读开发验证器位于 cindy-plugins/archive-verify，不进入产品包，也不提供归档导入：

```sh
go run ./cindy-plugins/archive-verify --file /path/to/public-history.json --source-session <明确选择的来源任务>
go test ./internal/cindyarchive ./internal/sessionimport ./internal/dbmigrate
```

## 原有能力限制与兼容

Step 0 的 will-user-message 是提交前事件，will-assistant-message 是回复后处理；缺少可靠消息 ID/seq 和普通任务补读，不能用 hooks 声称完整增量归档。tasks.readMessages 仍只允许本插件拥有的任务。本轮没有为归档启用 hooks、创建发送消息或接管已有用户任务。

原 Codex 导入器 internal/sessionimport/rollout.go 识别 session_meta、UUIDv7 与项目目录，现有 REST 读取专属于 codex_session。本次 Cindy Host 任务 ID 为不透明标识，没有伪造 rollout 复用这些字段。Cindy 使用独立表和线程类型；原 Codex 格式、命令与读取路由保持兼容，并纳入回归测试。

## 手动导入到明确选择的项目

源码与本轮更新后的开发 CLI 均包含以下命令。在明确选择的目标 Lumi 项目目录中执行，文件和来源任务均须显式传入：

```sh
lumi cindy-session import --file /path/to/public-history.json --source-session <明确选择的来源任务> --json
```

development 构建须使用 lumi_dev 或相应的 --environment development / --data-dir；不使用生产默认目录进行开发验收。命令不会读取 Host 历史，只读取所选文件；导出文件须先经公开接口和用户授权取得，命令不会把包装文件视为 Host 的权限证明。无文件、无来源、格式不匹配或来源不符时，在启动服务和打开项目前失败。打开目标项目须取得现有项目锁，不能借文件中的 workingDir 或来源标识切换目标。

写入仅通过 CLI server 的 POST /api/v1/cindy-session-imports；顶层字段 root_path、source_session_id、export，须带当前实例 If-Match nonce，服务再次验证文件。桌面服务器只注册 GET /api/v1/projects/:project_uuid/chat_threads/:thread_uuid/cindy_session_items，不提供自动采集或任意本机路径导入。基础 Cindy 工具目录也没有新增归档写入工具。

## 存储、重复投递和恢复

迁移 20261102000052 增加 cindy_session 线程、cindy_sessions、cindy_session_items、cindy_session_imports。内部主外键为自增 INTEGER（SQLite 的 bigint 关系），外部记录和导入回执使用独立 UUIDv7；来源消息/任务标识只存在独立来源列。保留旧线程图、Codex 内容、外键和已删除高 ID 的自增水位；有 Cindy 记录时拒绝有损降级。没有新增原生 chat_runs、chat_turns 或 chat_items。

- 文件摘要用于导入回执幂等；相同文件重投返回原 import_uuid、imported=0、replayed=true，不更新记录时间、不发送变更提示。CLI 传输会规范 JSON 空白，因此不是用户原始文件字节签名。
- 同一来源消息 ID 已保存且正文/角色/时间一致时不重复保存；同文本不同 ID 全部保留。新文件可以补入较早消息，展示按来源时间和来源 ID 排序，不按数据库插入顺序排序。
- 同 ID 内容变化返回 cindy_archive_source_changed；已保存且处于本次截止范围内的消息在新文件中缺失，返回 cindy_archive_source_missing。整次事务回滚，保留旧文本；不以新快照静默覆盖 rewind、删除或编辑。
- 较早截止文件不会倒退已显示的覆盖时间。每个已提交文件有独立回执，但页面展示的是多次手动导入保留的文本集合，不能理解为 Host 当前完整状态的镜像。
- 格式/范围/写入错误和取消都不返回部分成功。提交结果丢失时，重试同一文件；失败不影响创作工具，也没有后台采集队列、溢出补读或自动重试。
- 持久化使用 database/sql 参数化事务，避免失败 SQL 的 GORM 插值日志。对外只返回固定错误码；CLI 回执不带正文、标题、来源标识、文件路径或内部 ID。成功与失败路径均纳入隐私测试。

## 桌面只读展示

ChatArea 的线程列表和详情标记“Cindy · 只读”，展示所选来源公开返回的用户/助手文本、截止时间、最新覆盖文件省略计数和范围说明。没有聊天输入框、追问队列或虚假运行轨迹；服务器同样拒绝对此类型提交聊天、追问或读取原生运行轨迹。正文按普通文本显示，HTML 会转义，图片与附件不会加载。

提交后广播 cindy_session:changed，payload 仅含 project_uuid 和 thread_uuid。前端使对应列表、详情和 cindy-session-items Query 失效，再通过 REST 读取 SQLite；首次 join、重新 join 和窗口聚焦沿用项目级校准，无定时 HTTP 或 Host 历史轮询。禁用/卸载插件不会删除项目记录，因为没有驻留采集和卸载删除逻辑。

本轮验证包含：迁移前后 Codex 数据保留、有损降级拒绝、并发导入、精确重放、同文不同消息、较早消息补入、游标和项目隔离、来源变更/缺失拒绝、存储中途失败回滚、取消、真实 CLI/server 通知与 REST 读取、停止 CLI 后桌面重新打开、日志无正文，以及实际 JSX 的只读/转义/覆盖范围渲染。详见 [持久化验证证据](acceptance/step7-evidence/persistence-20261102.json)。

随后补齐实际进程中断恢复：在合成项目的一次导入已经写入两条未提交记录时终止测试子进程，新管理器可取得项目锁并重开，线程、来源会话、记录和导入回执均无部分落盘。重导同一文件完整保存三条，再次导入返回原回执且不重复保存。`go test ./internal/cindyarchive -count=1` 与 CLI 的 WS/REST/重开验收通过，见 [进程中断证据](acceptance/step7-evidence/process-exit-20261102.json)。故障屏障只存在于测试子进程的合成数据库，没有新增生产故障注入入口，也不代表 Cindy 宿主中断或真实聊天导入验收。

## 实际浏览器验收与交付边界

[浏览器验收回执](acceptance/step7-evidence/browser-acceptance-20261102.json)记录了隔离的本机服务和合成项目。先导入 3 条文本，再在页面打开时追加 1 条；列表和详情经 WS 提示自动 REST 重读，展示新的截止时间、媒体省略和空文本计数。后续追加 50 条，实际点击“加载更多历史”，经公开 UUIDv7 游标读取全部 54 条；两个同文用户消息均保留。

桌面与手机宽度均无记录区域横向溢出；输入框数量为 0，脚本文本显示为原文且未执行，没有附件图片加载。截图仅含合成内容：[桌面](acceptance/step7-evidence/ui-desktop-20261102.webp)、[320 px 手机宽度](acceptance/step7-evidence/ui-phone-320-20261102.webp)。这是 Chrome 中的实际响应式页面，不是物理 iPhone 验收，也不是 Cindy taskView；使用 Lumi 现有浅色主题，没有宣称新增全应用暗色模式。

开发包更新见[真实回执](acceptance/step7-evidence/development-update-20261102.json)，21 项审阅/安装包内容一致，新 CLI SHA256 为 b74ab5a4…；新命令已进入开发包。插件随包 CLI 仍不内嵌前端，页面验收另外使用 embed_frontend 构建，不把本机独立网页当作 Cindy 任务页。正式作者包也已重建、pack 并校验；用户随后明确允许正式身份并存验收，个人实例 lumi 已安装并实际诊断为 production，lumi-dev 为 development。现有 production 服务保留，未声称其后台已升级，尚未完成独立作品的双实例主线；没有发布。

79 项插件自动化已通过，记录同时区分正式与开发 CLI 构建，见[自动化](acceptance/step7-evidence/automated.json)。浏览器验收标签已关闭，专用 CLI 已停止，discovery 已撤销、singleton lock 已释放；合成项目保留。没有停止默认、正式或 Codex 服务。上轮旧 status 诊断进程仍为 UE；作者探针改成原子替换原生二进制，新的复制产物 --help 正常，不据此推断旧进程原因或声称它已退出。

本次手动降级路径的实现与上述范围验收已完成。用户实际采用时仍需自行明确选择来源文件/任务与目标项目；此次真实样本仅按授权保存在私有临时目录，没有借验收扩大到正式项目持久化。后续 Step 4 的真实 Core 生成、参考编辑、候选选择与 PDF/ZIP、普通 Cindy 正式独立作品的一章三页恢复均已通过各自范围验收。Step 5 按用户要求暂时跳过，旧诊断退出与 Codex 客户端共享另列[待办](acceptance/todos.md)；归档仍不包含自动捕获或完整历史，最终边界以[当前验收汇总](acceptance/step4-step7-status.md)为准。
