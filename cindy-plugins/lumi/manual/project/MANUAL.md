# 当前任务的 Lumi 项目

先在 Cindy 为任务选择本机项目根目录：新建作品可选择空目录或只含不冲突文件的目录；已有作品选择直接包含 `project.sqlite` 的根目录。一个项目可以有多个 Cindy 任务，每个任务独立绑定。插件不接受目录或任务标识参数，也不会把作品默认创建到插件安装目录。SSH 和其他远程目录不映射为本地路径。

所有下文业务工具均通过 `call_tool({name:"工具名",args:{业务参数}})` 调用，schema 与规则先从 `list_tools({category:"project"})` 获取。

## 初始化与 setup

1. 调用 `project_overview` 读取已有作品。返回 `project_not_initialized` 时，确认当前目录是用户想要的作品目录，用用户原始创意作为 `idea` 调用 `project_init`。可提供 `name` 和 `generation_language`（`zh-Hans` / `en`）。初始化创建标准 draft，不调用生成服务。
2. 检查返回的 `setup`。按用户意图补齐名称、语言、整体画风、创作简报与绘本规格，调用 `project_setup` 的 `update`，使用刚读取的 `expected_revision`。
3. 将规格向用户说明，遵循当前任务已经给出的确认意图，用 update 返回的新 revision 调用 `finalize`。若信息不足，先向用户询问缺失内容。finalize 只完成标准 Lumi setup，不启动自动创作；最终格式沿用 Lumi 已有不可变规则。
4. 用 `project_overview` 复核名称、公开 UUID、项目 revision、setup 状态与数量。项目 revision 和 setup revision 是不同对象的版本，不可混用。

对有效项目重复 `project_init` 只打开并返回现状，不会重设原始创意、名称、故事、素材或规格。普通文件与 `.git` 可保留；无效项目目录中若已有 README.md、STORY.md、project.sqlite 及其侧文件、.lumi 或 assets，按原有冲突规则拒绝。请用户移动冲突文件或在 Cindy 选择另一个目录；不要删掉用户文件来强行初始化。

## 创作规约

所有项目业务读写通过 Lumi 工具完成。不要直接读取或写入 `project.sqlite`（含 WAL/SHM/journal）、`.lumi/` 或 `assets/`；`STORY.md` 是只读投影，正文应通过创作工具更新。文本创作流程见 `ghost_manual({ghost_id:"lumi",path:"authoring/MANUAL.md"})`；媒体交接尚未开放，不要用直接改数据库或素材目录来替代缺失能力。

本手册是行为指引，不是操作系统写保护。Node 具有本机账号权限；不假定 Codex、Claude 或其他引擎加载同一个项目指引文件。Cindy 路径不创建或改写 AGENTS.md、.codex/config.toml、hooks 或用户引擎配置目录。已有这些文件保持原文。不要调用带 Codex 指引副作用的 `lumi init` 作为替代。

## 目录变化、只读与桌面切换

每次调用都使用 Cindy 当次下发的真实任务、目录和只读状态。只读任务可运行诊断、释放连接，并读取当前 CLI 已打开的项目；不会启动 CLI、打开项目或恢复投影。未打开时需先由可写任务打开。初始化、setup update/finalize 和其余业务写入在派发前拒绝，不沿用先前写权限。

目录、符号链接目标或数据库文件身份变化会撤销旧绑定并报错。检查 Cindy 当前目录后重新调用 `project_overview`；旧 UUID、页面路由、KV 或模型自报标识不能恢复授权。复制项目即使 UUID 相同也必须分别绑定；Manager 不会将两个目录作为同一个打开项目。

已打开的原件若被移走，再把同 UUID 的副本放回原路径，服务端仍会拒绝旧连接，因为真实目录、数据库或锁文件已经变化。先恢复原路径，或协调持锁实例释放后再打开副本；仅释放 Cindy 当前任务连接不能替代这一步。

`project_locked` 表示其他实例占用作品。在 Lumi 桌面版关闭该项目或退出占用它的实例后重试；不删除锁文件、不抢锁。切回桌面时先释放当前任务连接，并让同项目的其他任务也结束使用，等待 CLI 释放空闲项目后再从桌面打开。`runtime_release` 只撤销当前任务的连接和在途请求，不清空作品、不停止共享服务；不要因错误提示提到 stop 就擅自停止其他任务或客户端的服务。

worker 重启后不从 KV/磁盘恢复授权，下一次工具调用重新核对当前目录、项目 UUID、CLI 实例与权限。取消、超时或崩溃返回 `result_unknown` 时，先重读概况/setup，再判断是否重试；不能把失败回执当成写入未发生，也不能自动重放。
