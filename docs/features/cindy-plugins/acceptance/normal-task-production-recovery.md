# 从原正式 draft 恢复验收

2026-11-02，**恢复验收已通过，无需再次发送下方指令或重放写入。** 用户在原任务直接执行后生成恢复报告；父任务独立核对69次业务回执、9项完整内容和12份隔离前后快照通过，见[成功证据](step6-evidence/normal-task-production-recovery-success-20261102.json)。此前投递被 MCP 拒绝的回执、[协议修复](step6-shared-protocol-fix.md)和[后台切换](step6-evidence/shared-protocol-service-switch-20261102.json)均保留。下文为当次恢复指令历史，无需切换后台、新建任务或初始化。

以下指令已在原任务 `c18c29ad-7dcc-4ff0-8a32-8e6c14c8858c` 执行：

```text
继续当前目录已有的正式 Lumi draft，不重新初始化。仅使用实时发现的 ghost_id=lumi、namespace=null，先读取更新后的 runtime/project/authoring 手册及工具 schema。

先 runtime_status：必须 environment=production、server_compatible=true、runtime=ready。修复包 build_id=0278cd84b4b5f31b7d64d4f04ba8bdaf197f0679812154c8edd127c73de7f346、CLI SHA256=686a608b86fe3651d9f7cc89b57a20658a947ee1a1bea5b898597fa229f20b03。若仍 upgrade_required，释放当前连接并报告，不自行停止服务、不继续业务。

project_overview 与 project_setup get 重读事实，核对项目 UUID=01a19b9b-09a5-7e44-8ab7-cc44f90ff109。首轮报告为 project/setup revision=1、draft、故事/设定/页面计数均为0；本次以实际重读为准。UUID 不符、已有不属于本验收的内容或出现未知变化就停止报告。不得调用 project_init，也不使用首轮记录的 revision 直接写入。

根据实际 setup 状态继续：若仍为 draft，使用刚读取的 setup revision，update 名称“正式版独立作品验收”、语言 zh-Hans、画风“温暖水彩”、横版 classic_picture_book、创作简报“三页合成验收作品，不调用付费媒体”，再用成功 update 的新 revision finalize。若这些值已保存或已 ready，核对后跳过对应写入。任何失败或结果未知都先重读事实，不自动重放。

通过当前 Lumi 工具保存并逐字重读一段短故事、纸船角色设定、一个章节和三页正文/画面描述，包含“PRODUCTION-ONLY-20261102”标记。每步先 list/get，若已有本次验收内容则核对并复用 UUID，避免重复创建；有差异时停止报告。使用各目标刚读取的 revision，不直接写文件、SQLite、.lumi 或 assets。

正式项目与开发项目 01a199d7-323a-7017-a5da-1d93303f487e 必须不同。只在当前正式作品内对开发章节 UUID=01a199e6-89e9-7213-a333-924bffa24af8 执行一次 chapters.get，记录明确的资源不属于本项目/不存在语义拒绝；通用 invalid_arguments 不计隔离通过。不要写入该 UUID或打开开发作品目录。随后重读正式作品全文和 revision，确认未变。

不生图、不导入或导出、不安装或卸载插件、不停止服务、不启动其它 Agent。结束 runtime_release 并保留作品、共享服务和原报告。另存《正式版独立作品恢复验收报告.md》，记录包/后台兼容状态、setup 前后完整回执、故事/设定/章节/三页的完整文本比较、跨项目语义拒绝及 release 回执，并返回报告路径。只报告实际通过范围，不声称 Step 5、Codex 共享或旧诊断进程退出通过。
```
