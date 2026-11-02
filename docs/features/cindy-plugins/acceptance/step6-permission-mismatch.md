# Cindy 权限界面与运行时不一致

2026-11-02，主人报告已把本任务切为 Ask，随后再次确认 MacBook Pro 上当前任务输入框工具栏显示 Ask。公开运行能力连续返回 `permission.mode=auto`、`generation=0`。在新的继续验收指令下，仅通过正常 `ghost_call` 入口尝试一次章节保存，仍返回 `user rejected MCP tool call`，没有拒绝原因或审核者身份。此后停止重试。

## 当前任务与复现证据

- 执行电脑：MacBook Pro，Cindy 0.1.121，arm64。
- 当前任务：`023b85ac-4b64-4218-bc5e-4a05f26ca72b`，标题「授权安装开发包并继续真实宿主验收」，引擎 Codex，工作目录 `/Users/qingyang/mine-work/lumi`。
- 主人报告的是当前任务工具栏的 Ask；A/B 空任务自己的 `permission_mode=ask` 只用于恢复原任务，不能充当调用任务权限的证明。
- `get_current_session_id` 确认工具绑定当前任务；`get_task_digest` 确认本次用户输入与任务标题，未读取其它任务聊天。
- `get_capabilities({scope:"runtime",server:"cindy_helper"})` 连续返回 `{mode:"auto",generation:0}`，包括被拒写入后的查询。
- 重读章节并重渲染提示词后，以 `expected_revision=0` 保存完整 826 字正文；正常工具入口在 MCP 层拒绝。没有调用 CLI、HTTP 或其它写入入口替代该调用。

完整工具回执、正文摘要指纹与清理结果见 [本次证据](step6-evidence/ask-runtime-mismatch-20261102.json)。没有将本任务真实聊天正文纳入证据。

## 已核对的宿主实现

只读检查已安装 `app.asar` 中公开应用代码 `.vite/build/bootstrap-electron-BJb-edRz.js`。`runtimeCapabilities` 每次读取与调用实例匹配的 Maker session 的 `stablePermissionModeState`，并非历史任务记录；`setPermissionModeTracked` 在执行成功后更新 mode 并增加 generation。`MAKER_INVOKE.SET_PERMISSION_MODE` 会先更新活动 runtime，再持久化和广播。

实际构建 sourceCommit 为 `0a7d5a419f5cd3e0fbebc9642dde8388376a68aa`，匹配公开 ref 不可取得；这只是已发布编译代码和运行回执的诊断，不能代替完整源码调试。尚未确定界面与运行时分歧的具体原因，也不能从通用拒绝文本判定是人工拒绝还是自动审核拒绝。没有读取或修改 Cindy 私有数据库、设置、任务历史或应用代码。

当前 Agent 的 helper/control 目录不提供权限修改工具；`set_session_runtime` 只允许修改模型等运行设置。不能通过内部 IPC、私有接口或直接改数据绕过正常权限入口。

## 当时验收结果与后续

A/B 原项目恢复和跨项目章节隔离再次通过；B 读取 A 返回 `LUMI_CHAPTER_NOT_FOUND`。保存后重读 A，章节仍为 `revision=0`、`current_story=null`，页面列表仍为空；未发起付费媒体请求。作者探针测试 `node --test cindy-plugins/tests/host-probe.test.cjs` 为 3/3 通过。

A/B 已通过 `release_fixture` 撤销连接并保留项目、共享服务。没有重复安装、创建任务、重启 Cindy 或停止现有正式后台。

真实章节、分页、生图、参考编辑与导出仍未通过。下一步需要在宿主侧定位并同步当前任务的权限状态；主人已经确认界面 Ask，不再要求重复给出同一验收许可或重复切换。Step 5 可信页面身份仍是另一项独立门槛，不能把权限问题修复视为四视图已验收。

## 正常作品任务的转交尝试

主人随后要求在正常 lumi-dev 作品任务中先验证保存。已通过公开 get_task_digest/get_session_runtime 找到原 A 任务「Lumi acceptance · A」，确认空闲、active=false、无历史用户输入或排队消息。未新建任务，也未更改其模型、权限和目录。

向该已存在任务调用 send_to_session 投递唯一一条验证指令时，在 MCP 层返回 `user rejected MCP tool call`、isError=true；未提供拒绝原因或审核者身份。没有改用 steer、其它任务或工具重发。随后重查目标仍为 idle、active=false、last_user_ask=null、queued_count=0；当前调用任务的公开权限仍为 auto/generation=0。见 [完整回执](step6-evidence/normal-task-handoff-20261102.json)。

这次失败发生在任务转交，尚未进入正常 lumi-dev 工具调用，不能记成“正常作品任务写入也失败”。下一步可由主人直接在原 A 任务发送同范围验证指令，目标 Agent 通过自身真实上下文及正常权限入口完成一次保存并重读。当前证据不能确定通用拒绝的根因，也不能据此要求提供或修改 Cindy 源码。

## 正常作品任务保存成功后的结论

用户随后在原 A 任务直接执行正常 lumi-dev 调用，并提供《章节保存验证报告.md》。完整成功回执已核对：章节从 revision=0、current_story=null，变为 revision=1、version_no=1、char_count=875；仅保存一次。目标任务报告保存后 get 的正文逐字一致，写入和重读 hash 均为 e01ce56fd5ca340bb75ea5af8de7754a96389ff56feaad8bab2ad7bcf79dfd98。见 [正常保存证据](step6-evidence/normal-task-chapter-save-20261102.json)。本源码任务未重放写入，未复制完整故事正文。

正常入口章节保存已通过，不再将本源码任务的探针调用或转交拒绝泛化为所有写入受阻。目标任务只核实业务 read_only=false，Ask/Auto/Full Access 名称未核实；本次成功也不证明前述权限差异的具体原因已确定。后续页面、媒体和导出继续逐项验收，不能以本报告声称已通过；无须先提供或修改 Cindy 源码。
