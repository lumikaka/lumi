# Step 6 · Codex 插件、Skill 与一句话安装

## 目标

打包新的 Lumi Codex 插件和创作 skill，提供安装脚本，使拥有 Codex 的用户通过一句提示词即可完成安装，并在对话中从想法走到导出整本绘本。

## 背景

- 方案：`docs/features/codex-plugins/plan.md`。分工：本地 skill 告诉 Codex 如何使用 CLI 和插件；审阅节点按用户提示词决定，确有必要时向用户提问；LLM pipeline 由 Codex subagent 结合 `render_prompt` 完成。
- 调研结论：`docs/features/codex-plugins/spike.md`（插件安装命令、subagent、所需批准）。
- Step 1–5 已提供 `lumi` CLI、`lumi init`、stdio MCP 工具、图片导入和 MCP Apps。
- 旧插件 `codex-plugins/lumi-prod-plugin`、`lumi-dev-plugin` 是 HTTP OAuth 方式，可以参考其目录结构（`.codex-plugin/plugin.json`、`.mcp.json`、`skills/`、`assets/`），但不修改它们。

## 范围

1. 新插件放在新目录（例如 `codex-plugins/lumi/`，命名需与旧插件区分）：
   - `.mcp.json` 使用 stdio，`command` 指向安装后的**绝对路径**（macOS GUI 应用不继承 shell PATH），不指定 `cwd`（调研证实此时进程 cwd 即会话工作目录）。插件安装时会被复制到 `CODEX_HOME/plugins/cache/`，绝对路径须由安装脚本在生成本地 marketplace 时写入，不在仓库中写死用户路径。
   - manifest 文案说明：无需 API key，使用 Codex 自身额度，数据保存在本地。
2. 创作 skill，内容覆盖：
   - 何时调用 `lumi init`、如何确认当前目录是否为项目。
   - 完整流程：想法 → setup → 故事 → 设定 → 分页与画面描述 → 生图 → 导入挂载 → 审阅 → 导出。
   - 何时使用 subagent、如何调用 `render_prompt` 并写回结果。调研表明 subagent 只在用户或 skill 指令明确要求时才会启动，skill 必须写明哪些环节「应当启动 subagent」，同时保留不启动 subagent 的串行路径。
   - 生图前查询目标比例与推荐生图宽高比，生图后立即用宿主返回的实际路径导入，处理比例偏差的建议。
   - 角色 / 场景一致性：生图前先 `read_image` 相关设定图，再以最近图片为参考生图。
   - 审阅节点：遵循用户提示词；未指定时的默认停顿点；何时向用户提问。
   - 禁止直接读写 `project.sqlite`、`.lumi/`、`assets/`。
   - 额度耗尽或中断后如何从项目当前状态续做。
3. 安装脚本（例如 `scripts/install-codex.sh`；首版只支持 macOS 上的 Codex App，其他平台待 Codex App 支持并验证后再做）：
   - 下载对应架构的 `lumi` 二进制到用户目录，校验 SHA256。
   - 注册 marketplace 与安装插件优先使用 Codex App 内置的 `codex`（`/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex`），找不到时再回退到 PATH 上的 `codex`，并在输出中记录实际使用的路径与版本。两者可能版本不同但共用 `~/.codex`。
   - 注册 marketplace 并安装插件；已安装时执行升级，保持幂等。
   - 不修改用户 Codex 配置中与 Lumi 无关的内容。
4. 「一句提示词」的文案：放在 README 与官网文档 `site/` 中，如实列出需要用户批准的步骤，不承诺零批准或当前会话立即可用：
   - 下载二进制、写用户 `~/.codex`（注册 marketplace、启用插件）可能需要宿主批准；
   - 安装完成后需在 Codex App 中新开会话才能使用插件；
   - 项目写保护需要用户信任项目（见 Step 2）；
   - 明确说明只支持 Codex App，不支持 Codex TUI 与 IDE 扩展。
5. 发布：在现有 `Makefile` 的 tag / build 流程中增加 `lumi` 二进制和插件包的产出；签名与公证如需在 CI 完成，写清 CI 步骤，不在本地运行 Cargo / Rust 命令。

## 禁止

- 不修改旧插件与旧 MCP 逻辑。
- 安装脚本不得要求或收集任何 API key。
- 不运行任何 Cargo / Rust 命令。

## 完成标准

- 插件 manifest 与 SKILL 通过 Codex 的插件 / skill 校验器（如可用）。
- 在临时 `CODEX_HOME` 与临时安装目录中运行安装脚本，两次执行结果一致（幂等），卸载或清理步骤可用。
- 自动化冒烟测试：用 App 内置 `codex` 的 `app-server`（临时 `CODEX_HOME`）在空目录 thread 中依次调用插件工具：初始化项目 → 写一页文本 → 导入一张测试图片 → 导出 PDF；记录命令、输出与 codex 版本。
- Codex App 人工验收（由用户在真实 App 中执行，写入 `install.md`）：用一句提示词完成安装并记录批准次数与界面；新开会话后由模型自主完成从想法到导出，其中至少一张图片由 App 原生生图并导入。未执行的项明确标为待人工确认。
- 新增 `docs/features/codex-plugins/install.md`：安装、升级、卸载、常见问题。
