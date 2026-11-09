# 实现 Lumi Claude Code 插件

你正在 Lumi 项目中工作。请基于当前仓库的真实实现，新增面向 Claude Code 的 Lumi 绘本创作插件（`claude-plugins/`），并为新版 stdio `lumi mcp` 增加一个由 Lumi 后台调用已配置服务商完成的页面生图工具。

这是一项完整实施任务，不是只复制一份 Codex 插件目录。插件结构、共享 skill 生成、宿主标识、服务端生图工具、开发安装脚本、测试和文档必须一起闭环。

## 0. 使用 Goal 推进

本任务要求使用执行环境提供的 Goal 功能跟踪跨阶段实施。Goal 是执行环境的长期目标能力，不是要求在 Lumi 中新增 Goal 表、Goal API 或 Goal UI。

1. 开始时先调用 `get_goal`。
2. 不存在活动 Goal 时调用 `create_goal`；用户未指定 token budget，不要设置 `token_budget`。
3. 已有 Goal 正好覆盖本任务时继续该 Goal；存在无关且未完成的 Goal 时，不要覆盖或伪造完成状态，明确报告冲突。
4. 同时使用普通计划拆分实施阶段，最多一个阶段为 `in_progress`。Goal 保存长期目标，计划保存当前执行步骤。
5. 每阶段完成后更新计划，运行该阶段相关测试，记录仍未覆盖的真实缺口。
6. 只有全部验收项完成、相关测试通过且没有必需工作遗留时，才调用 `update_goal(status="complete")`。
7. 不要因任务较大、上下文压缩、运行时间长或预算接近结束而提前完成 Goal；只有同一真实阻塞反复出现时才可按环境规则标记 `blocked`。
8. 执行环境没有 Goal 工具时，改用计划或待办列表跟踪同一目标，并在最终报告中说明。

推荐 Goal objective：

```text
为 Lumi 新增 Claude Code 插件（正式版 lumi 与开发版 lumi-dev）：复用 stdio lumi mcp 与共享创作 skill，通过 --host 宿主标识开放仅限 Claude 的 Lumi 服务端页面生图工具，以 web_url 深链替代 MCP Apps，提供 make claude-dev 开发安装，保持 Codex/Cindy 插件与桌面 Web 行为不变。
```

## 1. 背景与已确认事实

以下事实已在讨论中核对，开工前仍须按当前代码复核：

- Claude Code 有插件体系：`.claude-plugin/plugin.json`、`skills/`、`commands/`、`agents/`、`hooks/hooks.json`、`.mcp.json`（可用 `${CLAUDE_PLUGIN_ROOT}`），本地 marketplace 使用 `.claude-plugin/marketplace.json`，通过 `/plugin marketplace add` 与 `/plugin install` 或 `claude plugin` CLI 安装。实施前用官方文档核实当前字段与 CLI 子命令，不凭记忆写 manifest。
- Claude Code 的 Read 工具可直接读取 PNG/JPEG/WebP/GIF，设定核对、合图查看、逐页看图审阅可用。
- **Claude Code 没有原生生图工具**，Codex 版 `prepare_page_image → 宿主 imagegen → import_image` 的链路不能直接沿用。
- 新版 stdio `lumi mcp`（`cmd/lumi/mcp.go`、`internal/textmcp`）当前不调用服务商：`internal/textmcp/prompt.go` 写明 `Do not call any provider`，`docs/features/codex-plugins/tools.md` 声明无 provider pipeline 或付费回退。`internal/textmcp` 不引用 jobqueue、provider 或 imagegen。
- CLI 共享后台在 `cmd/lumi/daemon.go` 调用完整的 `server.New`，已初始化 image client、`jobqueue` 任务管理、服务商服务与限流（`internal/server/server.go`）。生图能力在后台已存在，只是 textmcp 没有开放。
- HTTP 项目 MCP（`internal/mcpserver`）已通过 `request_api` 开放 `generation.comic_image.create`（`POST .../chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-generations`，见 `internal/agent/agent_api_registry.go`）。新工具应复用同一生成任务，不新增第二套生成实现。
- 当前创作协议版本为 `cmd/lumi/mcp.go` 中的 `creationProtocolVersion`；旧后台返回 `cli_upgrade_required`。
- 现有调用链没有任何宿主标识（无 `clientInfo`、`host` 字段）。
- Claude Code 不支持 MCP Apps。textmcp 中 `read_app_review`、`read_app_resource`、`write_app_resource`、`read_app_image`、`read_app_image_detail`、`watch_app_changes` 只服务 App 界面；`review_book/chapters/chapter/page/settings/candidates` 用于在 App 中打开共享工作区。
- CLI 共享后台已经提供完整 Web 页面，且在 `cmd/lumi/daemon.go` 中设置 `cfg.DesktopAuth = nil`，本机访问不需要桌面会话。`lumi open --json` 返回 `{endpoint}/projects/{project_uuid}/`（`cmd/lumi/open.go`）。前端需以 `-tags embed_frontend` 构建进 CLI（Codex 开发版如此；Cindy 随包 CLI 不内嵌前端）。
- Web 已有路由（`web/src/App.jsx` 等）：`/projects/:projectUuid/`、`chapters`、`chapters/:chapterUuid`、`chapters/:chapterUuid/sections/:sectionUuid`、`premise`、`premise/assets/:assetUuid`、`exports`，以及全局 `/settings/providers`。
- Codex 版 skill 已规定 MCP Apps 不可用时交付真实 Lumi Web 页面入口，不阻塞创作与导出，因此"只给 URL"是已支持的降级路径。

## 2. 关键决策（用户已确认）

| 项目 | 本期方案 |
| --- | --- |
| 插件目录 | `claude-plugins/lumi`（正式）与 `claude-plugins/lumi-dev`（开发，只保存 manifest 与 MCP 配置差异） |
| MCP | 复用 stdio `lumi mcp`，不要求 OAuth，不依赖桌面端运行 |
| 宿主标识 | 各插件 `.mcp.json` 在 args 中固定传 `--host claude` / `--host codex` / `--host cindy`；`lumi mcp` 把它随每次创作请求转给后台。未传时视为未知宿主，保持现有行为 |
| 新工具 | 新增一个服务端页面生图 MCP tool（命名建议 `generate_page_image`，实施时可按现有命名风格调整） |
| 限制强度 | 轻量：工具对所有宿主注册；后台仅在 `host != "claude"` 时返回明确错误（如 `host_not_supported`）。不建权限体系、不做加密校验 |
| 生图服务商 | 使用 Lumi 已配置的服务商与默认模型（data-dir 对应的 appstore）；未配置时返回可操作的中文错误，引导用户在 Lumi 设置中配置 |
| 共享 skill | 不维护第二份创作 skill。拆分共享主体与宿主片段（生图、看图、执行层回显），构建时生成 Codex 版与 Claude 版 |
| MCP Apps | Claude 版完全不使用。`lumi mcp --host claude` 在本地不注册 App 专用工具与 `review_*`，改为注册一个只读 URL 工具（命名建议 `web_url`）。其他宿主的工具清单保持不变 |
| 展示与人工编辑 | 引导用户在浏览器打开 CLI 后台提供的 Lumi Web 页面查看、编辑、选版本、配置服务商 |
| Web | 不修改桌面 Web 的视觉、路由、交互与生图流程；只按现有路由拼接深链 |

## 3. 修改前必须确认的基线

先执行 `git status` 与相关 `git diff`，保留并兼容未提交改动，不覆盖、回退或格式化无关文件。阅读：

- `codex-plugins/README.md`、`codex-plugins/lumi/`、`codex-plugins/lumi-dev/`：manifest、`.mcp.json`、skill 与 references。
- `scripts/dev-codex.mjs`、`scripts/package-codex.mjs` 及其测试、`Makefile` 的 `codex-dev*` 目标：共享资源复制、占位符替换、本地 marketplace 生成与刷新。
- `cmd/lumi/mcp.go`：参数解析、`forwardCreation`、协议版本检查、结果大小限制。
- `internal/textmcp/schema.go`、`service.go`、`image_preparation.go`、`image.go`、`protocol.go`：工具声明、合图准备、`setting_reviews` 校验、导入与写回。
- `internal/agent/agent_api_registry.go` 中 `RouteComicImageGenerationCreate` 的处理路径，`internal/jobqueue` 中漫画图片生成任务的输入、合图、提示词组装与结果写回。
- `cindy-plugins/scripts/sync-contracts.mjs` 与 `cindy-plugins/tests/contract-drift.test.cjs`：确认新增工具对 Cindy 契约检查的影响。
- `docs/features/codex-plugins/tools.md`、`docs/local-project-mcp.md`。

**必须先核实**：服务端漫画图片任务使用的合图与提示词，是否与 `prepare_page_image` 一致（设定选择、编号顺序、画风、`additional_direction`）。一致时新工具只是薄封装；不一致时以 `prepare_page_image` 的准备结果为准传入任务，不让两条路径产生不同画面，并在报告中说明差异。

## 4. 实施阶段

### 阶段 1：宿主标识

1. `lumi mcp` 新增 `--host` 参数（允许值 `claude`、`codex`、`cindy`；其他值启动即报错）。
2. 随 `creationRequest` 转发给后台，进入 textmcp 调用上下文；仅用于该工具判定与日志诊断，不写入项目事实、不出现在实时 payload。
3. 更新 `codex-plugins/lumi*/.mcp.json` 与 Cindy 生成的 MCP 配置，补上各自 `--host`。确认 Codex 和 Cindy 现有工具行为不变。

### 阶段 2：服务端页面生图工具

1. 输入与 `prepare_page_image` 对齐：`page_uuid`、`setting_uuids`、`setting_reviews`（非空选择时逐项 `passed`）、可选 `additional_direction`、可选 `model`、`idempotency_key`、基于最新读取的 `expected_revision`。复用同一套核对、过期与跨项目校验，不放宽任何前置条件。
2. 内部创建现有漫画图片生成任务，不新增 provider 调用代码，不新增 HTTP API 或数据库迁移。需要新增持久化字段时先停下说明原因。
3. 异步语义：生成可能较慢。优先方案是工具在单次调用内等待任务进入终态（受 MCP 请求超时约束），超时则返回 `task_uuid` 与当前状态，并提供只读查询（复用现有任务读取，或扩展已有只读工具）。不得引入定时 HTTP 轮询；后台内部等待应基于任务完成事件。
4. 成功结果返回挂载后的页面图片公开 UUIDv7、revision 与供 `read_image` 使用的引用，便于 Claude 立即看图并 `pages record_image_review`。只返回公开 UUID，不暴露内部 `id`。
5. 错误：`host_not_supported`、服务商未配置、额度或限流、任务失败、revision 冲突，均返回统一信封与可操作的中文说明。
6. 幂等：同一 `idempotency_key` 与输入重放返回同一任务，不重复计费。
7. `creationProtocolVersion` 加一；只有调用新工具时要求新版本，旧工具对旧后台保持兼容。

### 阶段 2.5：去除 MCP Apps，改为 Web 深链

1. 工具注册按 `--host` 在 `lumi mcp` 进程本地决定：`claude` 时不注册 `read_app_*`、`write_app_resource`、`watch_app_changes`、`review_*`，也不声明 MCP Apps 资源；`codex`、`cindy` 与未传 `--host` 的工具清单与 MCP Apps 声明逐项不变（加测试断言）。
2. 新增只读工具 `web_url`：可选 `chapter_uuid`、`page_uuid`、`setting_uuid`、`view`（`book`、`chapters`、`settings`、`exports`、`providers`），返回 `{url, scope}`。地址由后台当前实际 endpoint 与项目公开 UUID 拼接，复用 `lumi open` 的地址来源；只使用公开 UUIDv7。
3. 核实 MCP 中的 `page_uuid` 与 Web 路由 `sections/:sectionUuid` 的对应关系，以及设定 UUID 与 `premise/assets/:assetUuid` 的对应关系。不一致时按实际映射转换，不得新增或修改 Web 路由。参数互相矛盾（如页面不属于该章节）时返回校验错误。
4. 端口可能随后台重启变化：每次交付链接前重新调用 `web_url`，不得把 URL 写入项目文件、`AGENTS.md`、`CLAUDE.md` 或 skill。
5. 无需修改 Web。若确实需要改共享前端代码，先停下说明原因，并按 AGENTS.md 检查受影响页面。

### 阶段 3：共享 skill 拆分

1. 把 `codex-plugins/lumi/skills/create-picture-book` 中宿主相关段落（宿主 imagegen、`view_image`、执行层 `store/load`、`referenced_image_paths`、Codex App 信任与会话说明）提取为宿主片段。
2. 构建脚本按宿主拼装。**Codex 版生成结果必须与当前内容逐字一致**（加测试断言），保证 Codex 行为零变化。
3. Claude 版片段写明：不使用 MCP Apps；完成交付或用户要查看、编辑时，调用一次 `web_url` 给出对应范围的链接（整本 → 作品总览，单章 → 章节，单页 → 页面），服务商未配置时给出 `providers` 链接。用户表示在网页中改过或回到对话继续时，先重读相关对象与最新 revision 再写入；revision 冲突时重读，不覆盖用户在网页中的修改。用户回复候选编号时沿用现有 `get include_versions:true → select_image` 流程。
4. Claude 版片段：用 Read 看合图与成图；页面生图改为 `image_target → 设定核对 → generate_page_image → read_image 看图 → record_image_review`；批量设定总览图没有服务端工具时明确降级方案（只做单项设定，或提示在 Lumi 桌面端生成后导入），不得假装可用。
5. 项目指引：`lumi init` 在 Claude 宿主下额外生成仅含 `@AGENTS.md` 的 `CLAUDE.md`（已存在时只做局部合并提示，不覆盖）。确认此改动不影响 Codex 与 Cindy 初始化结果。

### 阶段 4：Claude 插件与开发安装

1. `claude-plugins/lumi/.claude-plugin/plugin.json`、`.mcp.json`（`["mcp", "--host", "claude"]`）、skills、图标（复用现有品牌资源）。
2. 写保护：提供 `.claude/settings.json` 的 deny 建议（`project.sqlite*`、`.lumi/**`、`assets/**` 的 Read/Edit），以及插件 PreToolUse hook 拦截 Bash 中直接访问上述路径的命令。hook 必须先判断当前目录是否为 Lumi 项目，非 Lumi 目录直接放行。
3. CLI 构建必须带 `-tags embed_frontend`（同 `codex-dev-build`），否则 `web_url` 链接无法打开页面；插件 manifest 不声明 MCP Apps 或交互界面能力。
4. `scripts/dev-claude.mjs` 与 `make claude-dev` / `claude-dev-build` / `claude-dev-refresh` / `claude-dev-stop`：生成 `build/claude-dev/marketplace`，开发版固定 `--environment development --data-dir`，使用 Claude Code 官方 CLI 注册与刷新，不手改插件缓存。参数与 `CODEX_DEV_*` 风格保持一致。
5. 开发版与 Codex、Cindy 开发版共用 `~/.lumi_dev` 后台；脚本输出中提示协议升级后需协调停止旧后台。

### 阶段 5：Cindy 契约与文档

1. 运行 `node cindy-plugins/scripts/sync-contracts.mjs --check`。新工具引起漂移时，按现有机制同步契约或标注宿主限定，不放宽检查本身。
2. 新增 `claude-plugins/README.md`，更新 `docs/local-project-mcp.md`、`docs/features/codex-plugins/tools.md`（说明服务端生图仅限 Claude 宿主，Codex 依旧无 provider 回退）。

## 5. 约束

- 遵守 AGENTS.md：REST 资源化、统一 JSON 信封、`snake_case`、对外只用 UUIDv7、实时 payload 不含内部 `id`、状态同步由 WebSocket 触发 TanStack Query 失效后 REST 重读，不得定时轮询。
- 不修改桌面 Web 的视觉、布局、路由、默认交互和功能。若改动了共享代码，必须检查受影响的 Web 页面，仅验证插件不能替代 Web 检查。
- 不在本地运行 Cargo 或 Rust 编译、检查及测试命令。
- 不提交真实 API key、凭据或个人项目数据；测试使用临时 HOME、data-dir 与项目目录，并只清理自己创建的资源。
- 不发布 Release、不推送、不上传任何 marketplace。

## 6. 验收

自动化（按实际新增的测试名调整）：

```sh
go test ./cmd/lumi ./internal/textmcp ./internal/jobqueue ./internal/production
node --test scripts/test-package-codex.mjs scripts/test-dev-codex.mjs scripts/test-dev-claude.mjs
node cindy-plugins/scripts/sync-contracts.mjs --check
```

覆盖至少：

- `--host` 解析与转发；未传 `--host` 时现有工具行为不变。
- 新工具：`host=codex/cindy/未知` 被拒；`setting_reviews` 缺失、过期或 `needs_revision` 被拒；服务商未配置的错误；幂等重放不重复建任务；revision 冲突；成功后页面图片挂载与返回字段只含公开 UUID。生成调用使用假 image client，不访问真实服务商。
- 协议版本：旧后台调用新工具得到 `cli_upgrade_required`，旧工具不受影响。
- skill 生成：Codex 版与改动前逐字一致；Claude 版不含 Codex 专属工具名。
- 写保护 hook：Lumi 项目内拦截、非 Lumi 目录放行。
- 工具清单：`--host claude` 不含 App 专用工具与 `review_*`、含 `web_url`；`codex`、`cindy`、未传时与改动前一致。
- `web_url`：各 scope 的地址格式、页面与章节不匹配时报错、只含公开 UUID、使用当前实际 endpoint。

手动（无法自动化的项明确标记为待验，不得推断为通过）：

- `make claude-dev` 后在独立测试绘本目录新开 Claude Code 会话，完成 初始化 → 故事 → 设定 → 分页 → 服务端生图 → 看图审阅 → 导出 的最小闭环。
- `web_url` 返回的作品、章节、页面、设定、服务商链接在浏览器中都能打开到正确页面；在网页中修改正文或切换图片版本后，Claude 继续创作时能重读到修改，不发生覆盖。
- 桌面版 Lumi 同时运行且使用同一 data-dir 时的行为（是否出现 `project_locked`、链接指向哪个实例），如实记录。
- 同一后台下 Codex 开发版插件仍可正常创作与宿主生图，MCP Apps 工作区照常可用。
- 桌面 Web 的页面生图与项目页面表现不变。

## 7. 最终报告

列出：改动文件与理由、`prepare_page_image` 与服务端任务的合图和提示词核对结论、协议版本变化及对 Codex/Cindy 开发后台的影响、全部测试命令与结果、仍待手动验证的项目和已知限制（如批量设定总览图的降级方案）。
