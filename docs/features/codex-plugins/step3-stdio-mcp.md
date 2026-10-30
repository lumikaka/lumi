# Step 3 · 新的 stdio MCP 与文本创作链路

## 目标

提供 `lumi mcp`：一个绑定当前项目目录、用户无需鉴权操作的本地 stdio MCP server，让 Codex 不依赖任何 LLM 服务商配置，完成从想法到整本绘本**全部文本内容**（setup、故事、设定、页面与画面描述）的创作，并能导出。

## 背景

- 方案：`docs/features/codex-plugins/plan.md`。要点：本地 CLI 不做额外鉴权；旧的 MCP 逻辑不要管；推理全部由 Codex 完成。
- 调研结论：`docs/features/codex-plugins/spike.md`，尤其是 stdio MCP 的 cwd 行为。
- Step 1 提供实例托管，Step 2 提供 `lumi init` 与项目信息。
- Lumi 现有生成链路的提示词在 `internal/promptcatalog`（story / chapter / premise / style / picture_book 等），由内置 runtime 调用服务商执行。
- 导出能力在 `internal/production/export.go`、`export_pdf.go`。

## 设计约束

1. **项目绑定**：调研证实插件 `.mcp.json` 不指定 `cwd` 时，stdio 进程的 cwd 就是会话工作目录；初始化参数没有 roots / workspace，也没有可用的环境变量。`lumi mcp` 启动时固定 cwd 并据此找到项目，需要时自动 `serve` 并打开项目；不依赖 roots / 环境变量，也不接受模型在工具参数中传入任意项目路径。cwd 不是项目时，工具返回明确错误，并提示先调用初始化工具（初始化后同一进程无需重启即可使用）。
2. **传输**：stdio MCP 进程只做转发，业务全部在 Step 1 的实例内执行（同一会话可能有多条 MCP 连接）；不得直接打开 `project.sqlite` 绕过实例持有的项目锁。
3. **用户无需鉴权**：信任边界等同于本机用户运行 CLI；不复用旧 MCP 的 grant / OAuth。stdio 进程只连接 Step 1 的 CLI 实例（通过 `cli-{environment}.json` 发现），无需凭证，不连接桌面版实例；转发请求不设置 `Origin` 头。项目被桌面版占用时，工具返回 Step 1 的 `project_locked` 错误及释放指引。
4. **提示词单一来源**：新增 `render_prompt` 类工具，用 `internal/promptcatalog` 的现有模板和项目上下文渲染完整提示词（含输出格式要求），交给 Codex 或其 subagent 执行；Codex 再通过写入工具提交结果。不要在 skill 里复写提示词。调研证实 subagent 能调用插件工具，但不提供隔离：渲染结果须自带完整任务输入、目标 UUID 与 revision，不得依赖父对话上下文，并发写入靠版本保护解决。
5. **无服务商 setup**：项目 setup 在没有配置任何服务商时也能完成并进入可编辑状态。
6. **写入安全**：写入带 `expected_revision` 等版本保护；返回字段使用公开 UUIDv7，不暴露内部 `id`；结果有大小上限，超限时提示缩小范围或分页。
7. **工具粒度**：面向创作流程设计，数量适中、命名清晰，参数用 JSON Schema 明确描述。建议覆盖：
   - 项目：初始化、查看概况、完成 setup
   - 故事：`STORY.md` 对应的 Story Profile、章节与正文
   - 设定：角色 / 场景等设定的增删改查
   - 页面：页面与画面段落的增删改查、整本结构概览
   - 提示词：`render_prompt`
   - 导出：PDF 等现有导出格式，返回本地文件路径
8. 变更后通过现有 WebSocket 变更提示通知已打开的 web UI，遵守 `AGENTS.md` 的实时同步规则。

## 禁止

- 不修改 `internal/mcpserver`、`codex-plugins/` 与旧 MCP 文档；可以复用其中的内部函数，但不改变其行为。
- 不调用任何 LLM / 图片服务商。
- 不运行任何 Cargo / Rust 命令。

## 完成标准

- `go test ./...` 中新增包的测试通过。
- 新增一个使用 `github.com/modelcontextprotocol/go-sdk` 客户端的端到端集成测试：在临时目录 init 项目 → 完成 setup → 调用 `render_prompt` → 用固定文本模拟模型输出并写回 → 建立至少 3 页完整文本 → 导出（允许缺图）→ 断言导出文件存在。全程不出现服务商调用。
- 测试证明：同一项目在 CLI 实例的 web UI 中打开时，MCP 写入后 UI 能收到变更提示；项目被桌面版占用时，工具返回 `project_locked` 及释放指引。
- 新增 `docs/features/codex-plugins/tools.md`：工具清单、参数、返回、错误码。
