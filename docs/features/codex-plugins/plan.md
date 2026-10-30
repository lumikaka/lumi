Lumi Codex 插件方案

目标

让 Codex 用户无需配置 API key，直接使用自己的订阅额度，在对话中完成 Lumi 绘本创作。

产品形态

- 把 Lumi 拆成宿主插件，由对话驱动创作流程。
- 首先支持 Codex。
- 在现有桌面版之外再提供 CLI 版本，可以一键在本地启动 Lumi server。

核心流程

整个创作过程都在宿主对话中完成。

其他要求

- 生图使用宿主自带的能力，生成的图片通过工具导入 Lumi，并挂到对应位置。
- 数据保存在本地。
- 探索能否用 hook 把聊天记录也存进 project.sqlite，可能可以通过 Lumi CLI 实现。
- 大量使用 Codex 新支持的 MCP Apps。
- 必须以 project 目录为基础开发：一个 Codex project 就对应一个
- project 目录中可能需要放入供 Agent 使用的 skill。
- 不破坏现有 project 格式，必须与桌面版保持通用。
- 当前的 AGENTS.md 可能需要区分场景，例如写明在 Codex 中打开时遵循什么逻辑。
- 最终安装保证拥有codex 的用户可以通过一句提示词安装并使用
- 本地 cli 可能就不需要额外鉴权了，按本地 cli 的通用做法，就不是远端mcp了
- 当前已有的mcp逻辑不要管，那个旧的

常见问题

1. 一个项目覆盖多大范围？
   就是现在一个 project 文件夹的范围，沿用现有格式。
2. 首版和现有 Lumi 桌面项目是什么关系？
   首版可以不依赖桌面版。我不排斥单独做一个 CLI 版本：用户只装 流程，可以参考 hyperframes、openflim 这类 CLI。
3. 第一次创建作品的默认入口是什么？
   由 Codex 插件调用 Lumi CLI 完成。
4. 同时打开多个 Codex 项目时，本地 Lumi 服务怎么组织？
   由 CLI 负责。
5. 通用 Skill 和项目里的 Agent 指令怎么分工？
   本地路径中的 skill 用来告诉 Codex 如何使用 CLI 或插件。
6. 从想法到整本绘本，默认在哪些节点停下来审阅？
   按用户在提示词中的要求来定。确有必要时，可以向用户提问。
7. 宿主模式下还要保留 Lumi 自己的 web UI 吗？
   MCP Apps 有自己的 UI。
8. Lumi 内部现有的 LLM pipeline 怎么处理？
   考虑引导 Codex 用 subagent 来实现。或者通过 tool 来提供提示词，或者按你的推荐
9. 用宿主生图和 Lumi 的严格比例约束冲突
  可以引导建议，不要那么严格，用户不阻塞优先
