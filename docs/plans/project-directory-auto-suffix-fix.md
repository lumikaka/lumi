---
git_commit_message: 'fix(project): 创建项目目录冲突时自动编号'
plan_state: finished
---
# 新建项目自动目录后缀修复计划

## current_status

- 本计划最初针对旧的单项目 Manager 编写；当前 `CreateWithInput` 已使用多项目 open registry。新建项目只加入注册表，不改变其他已打开项目，下面的目录占用与清理约束继续有效。
- 手动创建、YOLO 创建和直接 API 调用都先经过 `POST /api/v1/projects`，最终共用 `CreateWithInput`，无需维护三套目录规则。
- `docs/project-storage.md` 只声明不会接管或清理既有同名目录，尚未记录自动编号、候选上限及失败时保留当前项目的边界；`docs/plans/` 当前没有需要保留的旧冲突方案。

## overview

- 在 `internal/project` 增加单一目录占用步骤：依次尝试规范化后的基础名称、`-2`、`-3`，直至 `-1000`。后缀不生成 `-1`，编号空缺无需回收或重排。
- 每个候选直接调用 `os.Mkdir(path, 0o755)` 原子占用，不先用 `Stat`、`Exists` 等方式预检。`os.ErrExist` 表示候选已占用，无论占用者是普通文件、目录还是其他文件系统节点都继续下一个候选；其他错误立即按现有权限或路径错误返回。
- 先占用候选目录，再注册仅针对该目录的失败清理，之后初始化目标 Store/Runtime。候选耗尽、父目录不可写或其他占用失败不得停止或关闭任何已打开项目、修改最近项目索引或发送生命周期事件。
- 只有 `os.Mkdir` 成功返回的实际路径可赋给清理变量。后续 layout、UUID、锁、数据库、open hook 或最近项目写入失败时，只允许 `removeNewProject` 删除本次新建目录；任何被跳过的候选都不能进入清理路径。
- 后续初始化继续使用原始项目名称写入 `README.md`、项目库及最近项目索引；仅 `root_path` 使用实际占用路径。创建成功后把新项目加入 open registry，并发送该 UUID 的 opened 生命周期事件。
- 并发创建由文件系统的原子 `os.Mkdir` 决定候选归属。不同 manager 或进程可能因失败留下编号空缺，但不能覆盖、接管或删除其他调用已占用的候选。

## api

- `POST /api/v1/projects` 的请求字段保持 `name`、`parent_path`、`generation_language` 不变；手动、YOLO 和直接调用自动获得同一行为。
- 成功响应仍为 HTTP 201 和统一成功信封。`data.name` 保持用户输入的展示名称，`data.root_path` 返回基础目录或实际带后缀目录。
- 在 `internal/project/errors.go` 增加 `project_directory_name_exhausted`。基础名称及 `-2` 至 `-1000` 全部被占用时，`internal/httpapi/projects.go` 将该错误映射为 HTTP 409，并通过统一失败信封返回中文消息与可诊断详情。
- 权限和其他文件系统错误继续使用 `project_permission_denied` 或 `invalid_project_path`，不得误报为候选耗尽。

## ui

- 创建表单和 YOLO 调用链不增加参数或分支；两者继续复用 `createProject`，并以响应中的实际 `root_path` 更新当前项目与最近项目。
- 在 `web/src/i18n/errorLocalization.js` 和 `web/src/i18n/messages/errors.js` 注册 `project_directory_name_exhausted` 的中英文提示，说明名称编号已用尽，可更换项目名称或父目录后重试。
- 不把目录后缀回写到项目名称输入框、项目卡片名称或 YOLO workflow title。

## others

### 测试

- 在 `internal/project/manager_test.go` 覆盖：基础路径空闲时无后缀；基础路径存在时选择 `-2`；基础路径与 `-2` 存在时选择 `-3`；同名普通文件和目录均被跳过。
- 对既有候选写入哨兵内容并记录权限，创建前后验证内容、类型和权限不变；成功结果同时断言 `Name` 为原名、`RootPath` 为实际候选。
- 通过失败 open hook 等可控初始化故障验证：只删除本次占用的候选，基础路径和其他既有候选均保留；若 Runtime 停止失败，则保留目标 Store、锁与目录等待安全关闭。
- 预占基础名称及 `-2` 至 `-1000`，验证返回 `project_directory_name_exhausted`；构造不可写父目录，验证返回权限错误。两种失败都要断言既有 open registry、最近项目记录、Runtime 和生命周期事件不变。
- 使用多个 manager 并发在同一父目录创建同名项目，断言每次成功结果的 `RootPath` 唯一、目录内容完整且无覆盖。
- 在 `internal/httpapi/projects_test.go` 断言候选耗尽使用 HTTP 409、标准失败信封和新错误码，并保留已有 HTTP 201 成功契约测试。
- 为前端错误映射补充本地化测试，确认中英文均命中新错误码而不是通用 409 文案。
- 完成后执行 `go test ./...`、`pnpm --dir web test` 和 `pnpm --dir web build`。

### 文档

- 更新 `docs/project-storage.md` 的“创建”章节：记录原子候选顺序、`-1000` 上限、展示名称与目录名分离、并发编号空缺可接受，以及失败清理只作用于本次占用目录。
- 删除文档中“普通同名冲突直接返回错误”的旧描述，明确普通文件、目录和其他已存在节点一律只视为已占用，不修改、不接管、不删除。
- 记录候选占用失败不得改变任何既有已打开项目、最近项目索引和生命周期状态。

## prds

- 本修复不改变项目领域模型或用户创建字段；实现完成后检查 `docs/prds/overview.md`，仅在其中存在同名冲突或目录命名约定时同步改为自动编号规则，避免引入无关 PRD 内容。
