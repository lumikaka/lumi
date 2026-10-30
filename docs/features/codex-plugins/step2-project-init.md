# Step 2 · 在当前目录初始化项目 `lumi init`

## 目标

让 Codex 的工作目录直接成为一个 Lumi 项目：`lumi init` 在 cwd 原地创建项目，生成供 Codex 使用的轻量指引，且产出的项目与桌面版完全通用。

## 背景

- 一个 Codex project 对应一个 Lumi 项目目录，沿用现有项目格式（`README.md`、`STORY.md`、`project.sqlite`、`assets/`、`.lumi/`）。
- 现有创建流程 `internal/project/manager.go` 的 `Create` / `CreateWithInput` / `CreateDraftAt` 都是在**父目录下用 `os.Mkdir` 新建子目录**，不支持把已有目录原地变成项目。创建与打开的完整语义见 `docs/project-storage.md`。
- Step 1 已提供 `lumi serve` / `status`。

## 范围

1. `lumi init [--name <名称>] [--language <语言>]`：
   - cwd 已是 Lumi 项目：幂等返回项目信息，不做修改。
   - cwd 为空目录或只包含无冲突文件（如 `.git/`、`.codex/`、`AGENTS.md`、其他用户文件）：原地创建项目。
   - 与项目受管文件冲突（例如已有非 Lumi 的 `project.sqlite`、`README.md`、`STORY.md`）：拒绝并给出明确错误，不覆盖任何已有文件。
   - 创建后通过 Step 1 的实例打开项目，并写入最近项目索引。
2. 在 `internal/project` 中新增「原地初始化」能力，复用现有的写目录契约、加锁、migration、事务创建 `projects` 与 `local_user` 的步骤；失败时只清理本次新建的文件。
3. `lumi project info [--json]`：输出 cwd 项目的 UUID、名称、路径、是否打开。
4. 生成 Codex 指引（仅在不存在时写入，已存在则不覆盖并提示）：
   - `AGENTS.md`：简短说明这是 Lumi 项目；在 Codex 中所有项目读写必须通过 Lumi 插件工具；禁止直接读写 `project.sqlite`、`.lumi/`、`assets/`，`STORY.md` 只读。
   - 如果 `docs/features/codex-plugins/spike.md` 证实项目级 `.codex/config.toml` 能限制写入，则一并生成；否则跳过并在交付说明中记录。
   - 这些文件不属于项目格式的一部分：桌面版创建的项目没有它们也必须能正常使用，可通过 `lumi init` 在已有项目上补齐。

## 禁止

- 不修改 `project.sqlite` 的格式和 `format_version` 语义。
- 不修改旧 MCP 逻辑（`internal/mcpserver`、`codex-plugins/`）。
- 不改变桌面版现有的新建项目行为。
- 不运行任何 Cargo / Rust 命令。

## 完成标准

- `go test ./internal/project/... ./cmd/lumi/...` 通过，测试至少覆盖：空目录、含 `.git` 的目录、已是项目（幂等）、存在冲突文件、中途失败的清理。
- 测试证明：`lumi init` 创建的项目可以被桌面版的打开流程（`OpenSelected` 路径：UUID 校验、加锁、migration）正常打开；桌面版创建的项目可以被 `lumi init` 补齐指引文件而不改动其他内容。
- 更新 `docs/features/codex-plugins/cli.md`。
