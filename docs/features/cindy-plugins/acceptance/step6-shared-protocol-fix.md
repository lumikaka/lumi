# 正式共享服务的项目绑定协议修复

2026-11-02。首次正式验收报告记录了旧后台拒绝 setup 的失败，SHA256=`f04e3a3ff44e263f26cc5cfeb3bac360d863a51d2ba0f360936303aaf4744449`，保持不变。修复和后台切换后，原任务已从同一 draft 恢复，一章三页及指定章节隔离检查通过；父任务独立验证69次回执、9项完整内容与12份前后完整快照。最新结果见[恢复成功证据](step6-evidence/normal-task-production-recovery-success-20261102.json)，下文保留[原修复证据](step6-evidence/shared-protocol-fix-20261102.json)与切换过程。

## 已授权切换的实际结果

用户已明确确认切换正式服务并从原 draft 恢复。核对旧 PID `44608`、nonce 和 `open_projects=[]` 后，正常 stop 返回 `running=false`；已安装修复包随后启动新 production 后台 PID `9912`，新 nonce 与旧代次不同，`creation_protocol_version=1`。真实宿主返回 `runtime=ready`、`server_compatible=true`，当前源码任务连接已释放。开发服务仍为 PID `54965`、原 nonce，未停止或更新。见[切换回执](step6-evidence/shared-protocol-service-switch-20261102.json)。

当时向原任务投递恢复指令时，原生 `send_to_session` 返回 `user rejected MCP tool call`，没有重试或换入口写入。之后用户在原任务直接发送[恢复指令](normal-task-production-recovery.md)，任务完成并另存恢复报告；父任务独立比对完整提交/重读数据和隔离前后数据通过。原任务 finished、queue=0，原失败报告不变；父任务未直接执行作品业务写入。

## 已确认原因

修复前的正式共享服务为 production、PID `44608`，位于 `/Users/qingyang/.lumi`。随包 CLI 和 stdio MCP 握手成功不代表正在运行的旧后台支持项目绑定字段。首次未绑定的概况和初始化可用，绑定后 HTTP 请求增加 `expected_project_uuid`，旧后台在解析阶段拒绝，表现为 setup get/update 和后续读取的 `LUMI_INVALID_ARGUMENTS`。

对同一后台发送两次 `root_path=""` 的概况读取探测：不带绑定时返回正常的“启动目录必须是绝对路径”；增加绑定字段后返回 HTTP 400“创作请求无效”。两次均在打开项目前失败，前后 nonce 不变且 `open_projects=[]`。本轮没有重新初始化或读取、写入用户作品数据库。

## 修复与验证

后台状态新增 `creation_protocol_version=1`，明确支持项目 UUID 约束与只读传输。MCP 代理及 Cindy 适配层在派发业务前检查实际后台能力；缺少能力时返回 `cli_upgrade_required`。`runtime_status` 分别报告包指纹与 `server_compatible`，避免把旧后台描述为完全就绪。项目绑定约束继续保留，不靠删除字段兼容旧后台。

验证结果：

```text
go test ./cmd/lumi ./internal/textmcp -count=1
ok lumi/cmd/lumi 9.978s
ok lumi/internal/textmcp 11.644s

node --test cindy-plugins/tests/*.test.cjs
tests 81 / pass 81 / fail 0
```

新增 Go 回归覆盖旧接口失败模式、新代理在旧服务前拒绝所有业务且不停止服务，以及带真实绑定的 setup get/update/finalize。Node 回归覆盖诊断区分包和后台、派发前拒绝及只读诊断保持未知状态。完整回归同时包含真实 CLI 双版本独立作品、开发重启后正式正文保持、媒体和原 MCP 兼容链路。

正式、开发作者包均已构建和 Forge pack，21 项内容、权限、CLI 指纹与大小检查通过；本轮仅原位更新已授权的正式实例 `_root__lumi` / namespace=null / 0.4.0。实际安装重打包与审阅包的 21 项内容和权限相同。正式新 build_id=`0278cd84b4b5f31b7d64d4f04ba8bdaf197f0679812154c8edd127c73de7f346`，CLI SHA256=`686a608b86fe3651d9f7cc89b57a20658a947ee1a1bea5b898597fa229f20b03`。开发实例仍为原 `29401ca7…` 构建，未安装本轮开发包。

修复包安装后、后台切换前，真实宿主诊断曾返回 `runtime=upgrade_required`、`server_compatible=false`，读取在派发前返回 `LUMI_CLI_UPGRADE_REQUIRED`。这份历史证据证明兼容性检查生效；后续已按授权完成后台切换，原任务真实 setup 和三页恢复也已通过，分别以各自回执为依据。

## 恢复验收结果

原项目 `01a19b9b-09a5-7e44-8ab7-cc44f90ff109` 的 setup revision 1→2、最终 ready，初始化调用为0。普通正式作品的9项完整内容逐字一致；一次指定开发章节读取返回 `LUMI_CHAPTER_NOT_FOUND`，前后12份完整 data 不变，连接释放。验收后正式/开发服务均 `open_projects=[]` 且代次不变，见[服务观察](step6-evidence/production-recovery-service-state-20261102.json)。

无需重复停止服务、初始化或执行恢复写入。未扩展为所有资源与写入方向的宿主实测，未在该正式作品生成图片或导出。Step 5 与两项延后[待办](todos.md)保持原状态。
