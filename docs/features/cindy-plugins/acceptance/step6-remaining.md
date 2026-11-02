# Step 6 验收边界与用户延后项

2026-11-02。已通过图片链路、导出、安装/更新、拒绝、停用/启用、卸载/重装与作品/双图恢复；普通正式任务也已从已有 draft 完成一章三页文字、setup 与指定跨项目读取拒绝，父任务独立核对通过。本次可执行范围已收尾；下表保留跳过、延后和未声明能力的真实边界。

确认范围：Step 5 暂时跳过，旧诊断进程退出确认、Codex 缺客户端两项保留 [待办](todos.md)，桌面重开已跳过；均未计为通过。正式恢复成功见[完整核对结果](step6-evidence/normal-task-production-recovery-success-20261102.json)。

| 项目 | 本轮结果 | 后续条件 |
| --- | --- | --- |
| 原正常作品的项目锁 | 用O_RDONLY/O_NOFOLLOW打开已有锁文件，取得并立即释放LOCK_EX/LOCK_NB；锁文件inode、内容、mtime及project.sqlite完整SHA256不变 | 已通过“检查时项目锁可取得”子项；不替代桌面GUI重开 |
| Lumi桌面重开 | 用户答复“没有 跳过吧”，明确跳过；未执行、未计为通过 | 本轮按用户要求跳过，不再索要桌面CI产物 |
| 正式/开发独立作品 | 自动化双向拒绝及重启隔离通过；普通开发作品真实主线通过；普通正式作品从原 draft 完成69次业务调用，9项全文、5个标题、12份前后快照核对通过；指定开发章节返回 LUMI_CHAPTER_NOT_FOUND，连接释放 | 本次指定范围通过；不扩大为任意资源/写入方向的宿主实测。开发服务代次未变，见[恢复证据](step6-evidence/normal-task-production-recovery-success-20261102.json) |
| Cindy/Codex真实共享客户端 | 用户答复“没有或暂不可用”；当前任务未直接注册Lumi的Codex MCP | 已转[待办](todos.md#codex-客户端与共享服务验收)；有真实客户端后按清单验收 |
| 读取阶段优雅停止与恢复 | 已在正常任务A基线标记后提交优雅停止，目标轮次结束；另行重读875字章节/三页全文与分镜/双图/大纲均匹配，release通过，原共享服务保留且open_projects=[]；项目锁再次可取得 | 该只读范围通过；不扩大到在途业务写入/RPC/媒体请求的强制取消 |
| 全部代理/诊断进程退出 | 上次记录中旧诊断PID20644为UE，父20640仍在；共享服务54965保留，open_projects=[] | 旧诊断退出确认已转[待办](todos.md#旧诊断进程退出确认)；需重新核对进程身份及实际退出结果 |
| 四视图 | 随 Step 5 按用户要求暂时跳过；任务页可信身份门槛未通过 | 将来恢复时先满足[宿主接入条件](../host-task-view-identity-needed.md)，再实施与验收 |

真实锁检查见 [项目锁证据](step6-evidence/project-lock-release-20261102.json)。应用版本/迁移静态预检见 [桌面预检](step6-evidence/desktop-reopen-preflight-20261102.json)，仅用只读数据库连接检查schema版本，未修改或迁移数据库；静态二进制标记不冒充动态兼容性验证。

新增双版本回归见[完整命令输出与构建指纹](step6-evidence/channel-isolation-20261102.json)。它在独立临时目录运行，未停止已安装的正式/开发共享服务，未改变两项待办或 Step 5 的跳过决定。

进程、共享服务、宿主更新检查、任务状态与用户跳过决定见 [运行条件](step6-evidence/remaining-readiness-20261102.json)。第一次读取取消试验自然完成，未发生取消，原结果保留为[自然完成记录](step6-evidence/normal-task-read-cancel-natural-completion-20261102.json)，不计取消通过。第二次由控制器监测标记并提交停止，见 [停止回执](step6-evidence/normal-task-read-cancel-stop-20261102.json)。

不运行本地Cargo/Rust，不修改Cindy私有数据库/安装代码，不替换正式包，不借终止共享服务或创建另一代理来补齐真实客户端证据。

恢复结果见 [完整成功证据](step6-evidence/normal-task-read-cancel-recovery-success-20261102.json)：父任务独立比对完整文字/分镜与ZIP、八份旧报告及PDF/ZIP摘要；未重放作品修改。Host停止元数据未保留精确中断原因，真实请求回执与当时running标记分别保存；执行任务另报告观察到turn_aborted。
