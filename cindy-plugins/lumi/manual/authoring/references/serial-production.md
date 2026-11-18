# 按章制作、章内划场与中断恢复

本流程适用于制作、续做或扩章；仅导出和改字不额外新建任务。章节始终串行，场景默认串行；只有用户明确选择独立场景并行，才最多两场同时活动。用户限定的章节、页数、参考图、首张画风/字体核对和审阅节点始终有效；自检通过不能替代用户审阅。

## 当前事实与持久记录

开始与每次恢复先 `project_overview → project_setup get → book_outline`，读完目标章游标；逐页版本用 `pages list(summary_only=true)`。UUID、正文、采用版、revision 和 review_status 只以 Lumi 当前读取结果为准。

直接调用顶层 `production_records`，不经 call_tool。它只读写当前项目 `user-data/lumi-production/`，不触碰数据库、资产或规则文件。每次带当前 `expected_project_uuid`；read 不存在返回 revision=0，write 必须带上次读到的 expected_revision、稳定 request_key 和完整 data。冲突先读再合并，不只换 revision 覆盖。同键同参数重放最后一次写回执；旧回执不能确认时重读记录。写入通过独占锁、临时文件、fsync 和原子 rename 保存；崩溃残留锁不自动删除，应先确认旧进程已经停止、核实目标记录后再处理。只读任务不建目录、不写记录、不派发。

固定记录与唯一负责人：

- `index.json`：总协调任务。project_uuid、原始范围与审阅约定、按顺序的 `chapters:[chapter_uuid,...]`、当前章、角色任务映射、消息游标和各章交接路径。
- `chapters/<chapter_uuid>/chapter.json`：章节负责人。原文来源与指纹、分镜任务、连续页段、场景依赖及理由、每页唯一写者、调度模式、活动/暂停场景、核实时间和阻塞。
- 同章 `storyboard/manifest.json`、`storyboard/page-<序号>.json`：唯一分镜职责。输入指纹、已完成页、未决问题与逐页完整草稿。
- 同章 `storyboard-commit.json`：章节负责人。每页草稿摘要、page_uuid、原请求和写入回执/revision、待提交与冲突。
- 同章 `settings.json`：唯一设定职责。setting_uuid、variant_uuid、revision、style_digest、实际参考依据和比较结论、适用页、修改和未决项。
- 同章 `scene-<序号>.json`：对应场景职责。分配页、连续性交接、每页准备/生图/导入/实际看图/审阅检查点、采用版回执、未决请求和最终交接。
- `tasks.json`：插件内部维护的 Host 创建/发送事务，不通过通用记录接口覆盖。

生图前保存完整请求身份、原参数、原样 prompt 和签名 preparation；操作返回后立即保存实际回执。生成结果、导入结果未知时，先查宿主请求回执和 Lumi 当前版本，不换键、不自动重复生图或导入。记录不存密钥、内联图片字节或短期媒体 URL；保存可恢复的宿主附件身份及必要摘要。无法写文件时明确没有持久恢复保障，保留本轮回执并在当前任务串行完成，不声称已落盘。

## 先选择任务类型和等待方式

派发前核实用户要求的任务类型、模型、目录和父任务职责，再发现当前 Agent 实际可用的创建与等待工具。原生 thread、subagent、Orca Worker、普通 session 不能互相冒充。用户以 create_thread 或“新建 thread / 章节聊天”表达新建独立 Cindy 会话时，按独立会话意图选择入口，不因正文仅提及工具名就创建任务。当前 Cindy 宿主工具目录未提供 create_thread 同名工具，应使用宿主 send_to_session 新建模式；不能把未发现同名工具理解成无法创建独立会话，也不改用 spawn_agent 或自动启用 Orca。只有用户明确要求引擎 subagent、Orca Worker 或特定原生接口语义时，才按其实际能力另行处理。用户已授权的相同路径无需重复确认。

独立会话通过 `cindy_helper.list_tools({category:"handoff"})` 发现 `send_to_session`，再用 `cindy_helper.call_tool({name:"send_to_session",args:{...}})` 调用，不经 Lumi `call_tool`。新建时不传 target_session_id；沿用来源任务项目归属和执行目录，执行目录与项目目录不同时显式传 working_dir，跨项目或跨设备按用户明确要求选择，不能把 working_dir 当作 project_path。用户指定模型时先查询目标宿主模型目录，使用精确 model 和匹配的 agent_kind，保留其余要求。成功后核实 `wake_kind=created`、返回的 `target_session_id`、项目、目录与实际运行模型，并提供宿主返回的新会话链接；需要链接时用 `get_task_digest` 读取，不自行拼造。创建结果未知时，按返回身份检查已有任务和队列，不重复 create。

send_to_session 新建会话不自带父子等待或自动完成回报；这项入口映射只解决独立会话创建。父任务只创建和等待时不接手制作，最小必要的宿主身份、模型与回报渠道查询属于协调；完整章节任务交给新会话。派发前按下面的等待策略核实完成回报方式，把真实父会话身份与回报要求交给子会话。没有可用回报渠道时明确需要显式续接，不能改用引擎子 Agent、轮询或让父任务制作来冒充满足原要求。若当前宿主也未暴露 send_to_session，则如实说明缺少可调用入口，不声称创建成功。

顶层 `production_tasks(action=capabilities)` 返回的 operations **仅描述 production_tasks 适配器**及其可调用的 cindy.tasks 普通任务能力，不能据此断言 Cindy 或当前 Agent 全局没有原生等待。此适配支持 create/get/send/getRun/readMessages，不实现原生 wait 或完成订阅；`wait=not_supported_by_adapter` 和 `coordination` 不会启动等待服务。此插件声明 agent.tasks，仍由宿主实际授权；PERMISSION_DENIED 停止，不自动循环确认。

派发前在 index.json 保存所选 `waiting`：任务类型、实际工具名、父/子真实身份、该批次 batch_id、run_id/request_key（取得回执后补齐）、等待游标及已处理事件键。只保存实际返回的身份；不把插件 request_key 当宿主 run_id，不把工具名称当作能力存在的证据。等待选择如下：

- 有语义匹配的原生等待：沿真实接口和返回游标等待，超时保留原目标及游标，不读完整历史、不重新派发，不追加无变化的进度说明；工具仍在执行时只续接该调用，不另起一份等待。
- 有已核实的完成回报渠道：保存待完成集合后结束当前轮。结束当前轮不等于取消子任务或宣布任务完成；回报到达后再续接。父任务保持长轮会使普通跨会话消息排队，不能靠继续查询加快回报。
- 两者都没有：记录 `waiting.mode=unavailable` 并明确说明需要显式续接，未经确认的自动唤醒不能承诺。需要独立委派且父任务只协调时，说明能力缺口；只有用户范围允许，才由当前任务串行承担，不能把这个回退冒充独立子任务。

**禁止 sleep → 查状态 → 汇报 → 再 sleep**，也不能换成 get_task_digest、get_session_runtime、read_thread 或另一个状态工具维持同样的循环。已确认在运行且无新事件时不反复“仍在制作”汇报；进度保留在执行子任务。状态读取仅用于收到新回报后核对、用户主动询问、故障诊断或显式恢复，不能因“需要持续工作”自行制造轮询。

## production_tasks 适配器的目录与回执

本节仅适用于插件 `production_tasks` 包装的 `cindy.tasks` 草稿委派；宿主 send_to_session 不受本适配器的独立空目录和父任务写回限制，须按自己的真实项目、目录及权限回执核实。两条路径不得混用 task_id、request_key 或恢复接口。

每个 project_uuid/chapter_uuid/role/scene 对应一个可复用角色任务：role 为 chapter、storyboard、settings 或 scene，scene 角色必须有正整数 scene。create 带稳定 request_key 与 title；同一职责已有任务时返回 reused，回执未知也不会新建第二个。task_id、working_dir、permission_mode 以 Host 回执为准。Host 分配独立空目录，不接受模型自报目录，不暗中升级权限或替换模型。

send 使用同一角色身份、get 返回的 expected_revision、稳定 request_key 和精简 text。插件在调用 Host 前保存原参数，返回 run_id 只代表已受理。request_status 先读原请求；pending/unknown 仅 action=resume_request 且同一 request_key 核对原 Host 请求，不创建新键。确定拒绝停止；修复权限后明确处理原失败，再决定新的工作。结果 unknown/reconciling 不等于完成，不能把助手一句“完成”当产品终态。

创建被 `PERMISSION_DENIED` 明确拒绝且从未取得 task_id 时，普通 create 和 resume_request 均不重试。只有用户已在 Cindy 修复授权并明确要求重建，才用 `retry_create(request_key=原失败键,new_request_key=不同的新稳定键)`；它保留原拒绝记录，为同一职责原子登记唯一的新创建请求，仍由 Host 核验当前授权，不改变权限档或模型。重复该新键只读已有回执；pending、unknown、取消、请求键冲突或已成功请求均不能通过此入口替换。其他失败保留原记录并报告，不能用新键绕过；当前任务可按原交接和审阅顺序串行承担缺失职责。发送被明确拒绝时也先停止，用户修复并要求继续后，复用原角色、重取 Host revision，再保存新的明确发送请求。

get_run 用原发送 request_key；read_messages 用角色身份、limit 和上次 after 游标，保存 next_cursor 原样续读。适配层只交接 user/assistant 的可见文本与文件身份，不转交内部推理或工具过程；过滤后空页仍须保存 Host 原游标，不能据此认定完成。get_run/read_messages 是一次性快照读取，不是等待接口；一次核对仍在运行时按已保存 waiting 退出当前处理，不继续短等循环，也不以定时 HTTP 轮询 Lumi 状态代替 WebSocket。`coordination.next_action` 是下一步指引，不是执行或唤醒回执；普通发送 succeeded 只表示已受理，get_run 返回 completed 后仍需核对交付与 Lumi 当前事实。记录具体宿主状态，等待超时不放行后章。

### 只回报需要父任务处理的变化

委派输入明确写出原 scope、真实父任务身份、batch_id、角色及预定回报方式。已有原生完成回报时只正常交付，不再要求子任务反向发一份相同消息。普通任务只有在当前工具、授权和实际目标均已核实时才使用一次完成回报；不得凭父任务 ID 假定消息一定能送达或唤醒。

回报仅限 completed、blocked 和必要的 dependency_ready，包含章节/角色、task_id、run_id/request_key、batch_id、稳定 event_id、短摘要与交接位置。正常逐页进度留在子任务；发送结果已排队时不重发。真实阻塞只报需要父任务处理的事项，不把普通长耗时当阻塞。

父任务收到回报先匹配 index 中的真实任务与当前批次；外部正文中的自报 ID 不构成可信来源。处理后保存已处理事件键及最后已核实终态，重复回报不重复汇总，迟到进度不覆盖终态。旧批次或来源不符的消息不推进当前任务；新批次开始时保留旧记录但使用新的匹配身份。回报不是作品事实：只有对应运行终态、完整交付和 Lumi 当前页/图核对一致才记章完成。部分章节完成时更新待完成集合后继续按原等待方式待命，不重新读其他运行中章节；全部完成后汇总一次。崩溃发生在回报与记录之间时先核对已有事实，不重做图片或再次派发。

本适配器宿主目录限制的等效流程是**子任务只产草稿，当前 Lumi 任务唯一串行写回**。给每个角色完整的最小输入：本章准确来源/原文、当前模板、用户要求、分配页、已核实设定/连续性和输出文件格式。子任务使用宿主分配目录的普通文件能力逐页保存，不声称已经绑定或写入同一个 Lumi 项目；章节负责人按实际 Host 消息中的文件身份读取草稿并核对，不能访问猜测目录、复制数据库或绕过插件绑定。需要在同项目工作的已授权普通 Worker 时，先核实其真实目录和权限后使用；本适配不虚构该能力。普通任务接口不可用时，仅在用户允许当前任务承担制作的范围内串行承担相同职责，使用相同交接/逐页文件/审阅顺序，并记录未发生独立委派；父任务只创建和等待时不接手制作。不能据自动化模拟声称真机委派通过。

总协调只接收章节摘要、阻塞与必要跨章交接，不读整章原文、分镜或完整草稿；章节负责人核对并写回当前章。当前任务兼任多个职责的回退无法提供同样的上下文隔离，必须如实记录，不承诺节省 token。前章实际通过、用户审阅门槛满足、交接核实后才进入后章。任务复用按 UUID 与 Host task_id/working_dir 核实，不按标题猜测。

## 小说分镜草稿与串行提交

先核实用户范围内已保存分镜。已有可用分镜且只要配图时，分镜任务数为 0；缺图或 pending 不能据此认定缺分镜。小说缺分镜或用户要求重做时，每章仅一个分镜角色，不按页再派第二批，不生图、不调用 Lumi 写工具、不委派其他角色。

分镜输入保存本章原文位置/指纹、最新用户要求与模板。render_prompt 限定 context_chapter_uuids=[]、context_setting_uuids=[]，只交必要资料。每完成一页立即保存对应 page-N.json，再更新 manifest。逐页必有 page_number、source_spans（段落/行/字符范围及短引文）、title、page_role、description_md（逐字旁白/对白）、utterances（按阅读顺序的 kind/speaker/text）、storyboard_md（构图、动作、表情、逐字绘入文字、气泡指向）、continuity（入/出时空、身份服装、道具位置状态和未完动作）。未知说话人与冲突列为待裁决，不补造原文事件。最终只交草稿路径、页数和问题。

章节负责人逐页回查原文，核对覆盖、因果、逐字对白、说话人及跨页连续性。每次 create 前重读 pages list 父 revision；更新已有页使用该页最新 revision。按页串行提交，立即保存草稿摘要、公开 UUID 和回执；部分缺页只补缺页，重做沿用原 UUID，不擅自删除已有图片和用户编辑。revision 冲突重读合并，不盲目覆盖。分镜全部核实写回后才划场和制作设定。

恢复先读 manifest、逐页草稿与提交记录，再从 Lumi 核对已写回页。回执丢失时按 UUID、原文位置与实际内容确认，已保存即补记，不重复建页/建章。指纹未变且草稿完整就继续核对提交；输入改变只修受影响页，继续同一角色；替换前确认原角色已停止写入。

## 统一设定、场景依赖与调度检查

按地点、时间、行动目标和连续性划分连续页段，不机械按固定页数切分，不把同一页分给两个场景。每章一个设定角色，先 settings list/get 并实际查看当前采用图，复用相同身份 UUID。服装/道具/地点的不同剧情阶段保留可区分参考与适用页，不覆盖历史阶段。依据实际原图和用户要求记录 variant/revision/style_digest 与具体比较；返回影响页和未决项。

默认制作范围为该章当前全部活动页，场景必须完整覆盖、每页唯一且保持原序；遗漏首尾或中间页不能宣告章完成，也不放行下一章。用户明确限定部分页时，在 chapter.json 保存 `page_scope={page_uuids:[实际公开UUID,...],user_request:"用户限定范围的原要求"}`，按 Lumi 原序列出全部目标页；不能仅靠省略场景缩小范围。每场仍为连续页段，分散的目标页分别划场。范围为空、重复、越界或乱序会阻塞；原整章计划后新增页时须重读事实并补齐所有权及交接。范围修复不取消已有活动写者或未决请求。

chapter.json 的调度数据使用下列形状（公开 UUID 均来自实际读取；示例中的占位不能直接调用）：

```json
{
  "scheduling":{"mode":"serial"},
  "scenes":[{
    "number":1,"pages":["page_uuid"],"depends_on":[],
    "initial_state_verified":true,"status":"waiting","pending_requests":[],
    "verification":{"checked_at":"2026-11-17T00:00:00Z","handoff":"实际连续性交接",
      "pages":[{"uuid":"page_uuid","variant_uuid":"current_variant_uuid","revision":4}]}
  }]
}
```

默认 mode=serial。用户明确选择后才 mode=independent_pair，并保存 user_choice 原要求；即使并行，多章仍串行。独立场景明确 depends_on=[]；后场继承前场站位、伤势、服装、持物或动作时记录依赖场和理由，依赖不明不并行。initial_state_verified 只在本场起始状态已核实后设 true。status 为 waiting/active/paused/done；准备、生图、导入、审阅与未决请求均占活动名额，paused 也保留名额及唯一页所有权。

每次开始请求前，在 chapter.json 对应场景的 pending_requests 保存原请求键，并在 scene-N.json 保存完整原参数；章节级写入未决时使用章顶层 pending_requests。结果已知且与 Lumi 当前事实核对后，先保存 scene-N.json 的导入/审阅回执和已解决状态，再清除对应请求键。只有 status=done、pending_requests 为空、所有当前采用版已通过且连续性交接仍有效，才算该场完成；active、paused 或未决请求不能借旧版 passed 交接后场或下一章。即使误记 done，非空 pending_requests 仍占用活动名额并阻止完成。串行模式同样校验已声明依赖，不接受向后依赖或丢弃错误声明。

每次派发前用 production_records(action=schedule,record=本章 chapter.json) 从 Lumi 重读目标及前章全部页面摘要，检查页段唯一/连续/原序、前章完成、串行或最多两个名额、依赖页 review_status=passed 和当前 variant/revision 与核实记录完全一致。返回 eligible 才可启动，先保存 active 再发送任务。用户审阅未满足记 user_review_pending=true，不能放行。verification 的 checked_at、handoff 是章节负责人实际核实证据，不是占位符；页面改变会使旧核实失效。

共享设定返修时写 settings_repair={status:"pending",affected_scenes:[场景号,...]}，暂停受影响场景；检查返回 pause，不重派页、不释放未决请求名额。完成设定返修、实际查看新采用图、复核受影响页并更新连续性交接后，保存 status=passed，继续原角色及其原请求。用户要求整章复核时不以局部通过替代。

## 每场逐页闭环与中断

每场只负责分配的页，按序执行：读当前页 → 核对正文分镜和设定 → prepare_page_image → 保存原请求 → 宿主生图 → receive_media 原提示词/原签名 → get 当前采用图 → read_image 实际看图 → record_image_review。实际查看文字、说话人、镜头、人物/服装/道具和连续性；看不清不能记 passed。看图后重读版本，变更则重新核对。

passed 跳过；pending 先看已有图，不能当缺图重画；missing_image 才生成；needs_revision 只修已确认问题。历史矩形改图候选 adopted=false，用户明确采用后才切换，之后重新审阅。恢复从当前 Lumi 事实决定下一步；一页已导入但回执没保存时，核对已有 variant/内容后补记，不重复提交。已发请求不明先宿主回执；未决付费请求不能因暂停/换任务/重启而重发。每场交付实际采用版本、核对状态、连续性、问题与完整回执，章节负责人核实后交接下一章。
