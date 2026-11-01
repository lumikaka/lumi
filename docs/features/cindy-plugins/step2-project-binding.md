# Step 2 · 当前任务的项目绑定与初始化

## 目标

让当前 Cindy 任务目录成为标准 Lumi 项目，正确打开已有作品；同项目可有多个任务，不同项目互不混淆，并保持桌面版兼容。

## 前置与背景

完成 [Step 1](step1-runtime-cli.md)。参考现有 [CLI 行为](../codex-plugins/cli.md)、`cmd/lumi/project.go`、`cmd/lumi/guidance.go` 和 `internal/project/init.go`。

现有 `lumi init` 会补 Codex 的 `AGENTS.md` 与 `.codex/config.toml`。Cindy 路径不能不加区分地调用它，然后宣称没有引擎配置副作用。

## 范围

1. 从 Host `session_context` 取得任务和工作目录，规范化路径、校验本地与当前读写状态，再进入 worker。首版要求任务已选择本地项目根；无目录时说明如何在 Cindy 选择，不擅自创建默认作品到插件安装目录。
2. 复用现有 `project_init` 与项目 Manager 完成原地 draft 初始化、setup 和打开。已有有效项目幂等返回，不重置故事、素材或规格。普通文件冲突按原有规则拒绝，仅回收本次创建的文件。
3. 隔离宿主指引副作用：优先复用不生成 Codex 配置的业务初始化路径；确需扩展 CLI 时显式区分宿主指引模式，并保持现有 Codex 默认行为兼容。Cindy 入口不得隐式创建或改写 `.codex/config.toml`、hook 配置及用户引擎目录。
4. 创作规约优先放插件 Manual。项目 `AGENTS.md` 如需补充，只写宿主通用的最小说明且保留原文；不覆盖已有文件，不假定每种引擎加载同一文件，不把文本指引说成系统写保护。
5. 内部绑定至少关联真实任务、规范项目根、project UUID、CLI 实例身份和本次权限上下文。项目根与 UUID 校验不符时拒绝访问；不得把两份复制项目的相同 UUID 默认为同一目录。
6. 页面、KV 或模型自报的 session/project 标识不能新建授权绑定。重启后缓存仅作为线索，必须从可信上下文重新校准；工具调用前重查只读状态，不沿用旧写权限。
7. 项目仍由 CLI server 持锁。桌面占用时返回 `project_locked` 和释放指引，不抢锁；关闭当前任务只解绑该任务，不清空项目数据或停止其他任务服务。
8. 项目状态返回公开 UUIDv7、名称、setup 状态、revision 与必要限制；内部路径仅留在本机适配层。Lumi 业务 JSON 保持统一信封。

## 交付

- Cindy 项目初始化/概况工具及绑定管理。
- 必要的宿主指引分离改动和 Codex 兼容测试。
- `docs/features/cindy-plugins/project-binding.md`：目录规则、只读模式、工作目录变化、项目锁、重启恢复与桌面切换。

## 禁止

不改变项目 `format_version` 语义，不为 Cindy 另造项目格式，不直接写 SQLite，不修改既有 Codex 指引，不把远程路径映射成本地路径，不运行 Cargo/Rust。

## 完成标准

覆盖空目录、普通非冲突文件、已有 Lumi 项目、冲突文件、符号链接与目录变化、重复初始化、中途失败清理、只读/非本地拒绝。双任务同项目与双项目并发互不串绑；Cindy 创建的项目能经桌面 Manager 重开，桌面作品可被 Cindy 打开；Cindy 初始化不生成 Codex 专用配置，旧 Codex 初始化行为保持。
