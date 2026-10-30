# Step 0 · Codex 关键行为调研（Spike）

## 目标

在写任何产品代码之前，用真实的 Codex CLI / App 验证后续步骤依赖的宿主行为，产出一份带证据的结论文档。本步只写文档和一次性验证脚本，不修改 Lumi 产品代码。

## 背景

- 方案见 `docs/features/codex-plugins/plan.md`：Lumi 作为 Codex 宿主插件，通过本地 `lumi` CLI 提供 stdio MCP，生图使用 Codex 自带能力，UI 使用 MCP Apps。

## 需要验证的问题

每个问题都给出：结论、验证步骤（可复制的命令或最小插件）、codex 版本号、不确定之处。

1. **stdio MCP 的 cwd**：插件 `.mcp.json` 声明的 stdio server 启动时，进程 cwd 是什么？能否拿到当前 Codex 会话的工作目录（环境变量、`roots`、初始化参数等）？多个会话在不同目录时，是每个会话一个进程还是共享？
2. **原生生图产物**：Codex 自带生图后，图片文件落在哪里（`savedPath`）？文件名、格式、尺寸可控程度？模型能否把这个本地路径作为参数传给 MCP 工具？MCP 进程（不在沙箱内）能否读取该路径？
3. **生图与参考图**：对话中已有的图片（例如通过 MCP 工具返回的图片内容）能否作为生图的参考？给出实测效果描述。
4. **MCP Apps**：Codex 哪些界面能渲染 MCP Apps（App / IDE / TUI）？iframe 能否加载 `http://127.0.0.1:<port>` 的图片，需要怎样的 CSP 声明？不能时 data URI 的体积上限？App 内能否回调工具？
5. **沙箱写保护**：在项目目录中放置 `.codex/config.toml` 或其他项目级配置，能否让 Codex 不直接写 `project.sqlite`、`.lumi/`、`assets/`？项目级配置是否需要用户信任？
6. **Subagent**：Codex 当前 subagent 的触发方式、上下文隔离程度、能否调用插件的 MCP 工具。
7. **Hooks**：可用的 hook 事件、输入中是否包含 transcript / rollout 路径、项目级 hook 的信任要求。
8. **插件安装**：从零开始安装一个本地 marketplace 插件的完整命令序列；在 Codex 对话中让模型执行这些命令时需要哪些批准。

## 交付

- `docs/features/codex-plugins/spike.md`：按上面编号逐条记录结论。
- 验证用的最小插件或脚本放在 `docs/features/codex-plugins/spike/`，不要放进 `codex-plugins/`。

## 禁止

- 不修改 `internal/`、`cmd/`、`web/`、`codex-plugins/` 下的任何文件。
- 不改动用户已有的 `~/.codex` 配置；需要时使用临时 `CODEX_HOME`。

## 完成标准

- 8 个问题都有结论；无法验证的明确写「未验证」及原因，不得猜测。
- 每条结论附 codex 版本号（`codex --version`）和可复现步骤。
- 文末列出对后续 Step 1–7 的影响（哪些假设成立，哪些需要调整设计）。
