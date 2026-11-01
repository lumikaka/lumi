# Lumi 运行与恢复

本包实现本地运行、项目 setup 与文本创作，支持 macOS arm64。图片交接与任务审阅页尚未实现。文本流程见 `ghost_manual({ghost_id:"lumi",path:"authoring/MANUAL.md"})`。目录与任务身份由 Cindy 注入。初始化与创作规约见 `ghost_manual({ ghost_id: "lumi", path: "project/MANUAL.md" })`。

所有工具由 `list_tools` 发现，再通过 `call_tool({name:"工具名",args:{业务参数}})` 执行。先用 `runtime_status` 检查 MCP 初始化与共享服务状态。已有 Lumi 项目可用 `project_overview` 读取概况；没有项目时按项目手册调用 `project_init`。需要结束当前任务连接时使用 `runtime_release`。

`runtime_status` 可以用于只读任务，此时只检查 MCP 初始化和 ping，server_running 为 null，不检查可能创建数据目录锁的 CLI status。只读业务调用使用附加的只读 CLI 模式，只连接已运行的服务并查看已打开项目，不打开、迁移或修复作品。未打开时返回 read_only_project_not_open。不要用设置、嵌套参数、远端同名路径或伪造 `session_context` 绕过此限制。

Node 是拥有本机账号权限的可信程序，不是项目文件沙箱。插件仅运行随包 CLI，不接受任意命令、环境变量、SQL、二进制替换或目录参数。不要直接编辑项目 SQLite。

取消只影响当前调用；释放连接只影响当前任务。共享 CLI server 可能仍供其他 Cindy 任务或 Codex 使用，即使由本插件首次启动也不证明独占。插件不自动执行 `lumi stop`。旧 CLI 的错误提示如建议 stop，也不构成停止其他使用者服务的授权。

worker、MCP 代理和共享 server 是三个独立生命周期。worker 回收或崩溃后，下一次显式工具调用会重新检查任务目录、CLI 和实例身份；旧任务绑定不从磁盘恢复。代理空闲约 30 秒关闭，任务绑定空闲约 60 秒失效。服务本身沿用 Lumi 的项目空闲释放机制，可能继续存活。

失败结果以 `success:false` 判断。目录或实例身份改变时，重新读取；只读/非本地错误先修正任务条件。初始化或 setup 写入在超时、取消、崩溃和结果复核失败时返回 `result_unknown`，须先重读事实状态再决定是否重试，禁止自动重放。

此包是开发验收产物，未安装实测。打包成功不等于当前构建下的宿主取消、升级或卸载已通过；最低正式兼容版本也尚待正式发布证据确认。
