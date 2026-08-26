# Agent Project API Tool 第二阶段迁移报告

日期：2026-08-20  
基线：`29a0755f9c5c593a2df600bcf6c1481de16d0228`（`feat: Tool 改造第一步`）

> 本文是步骤二历史记录。当前运行时契约已经收敛为 `project_api_v2`；最终架构见 `agent-project-api-phase-3.md`。为避免把历史术语误当成可用接口，本文统一使用当前工具名和当前 Scene/Guide 结构描述对应能力。

## 当时完成的基础能力

- 建立全局 Agent Project API Registry；`request_api` 只匹配显式注册的 method + path，并直接调用进程内领域服务，不进行 HTTP loopback。
- URL、query、body、UUIDv7、项目归属、响应投影、revision、幂等和危险操作确认均由全局层校验，不依赖 Scene 授权。
- Story、Chapter、Premise、Premise Asset、Comic Section、Storyboard、Generation 与 Task 的首批 19 条 Route 完成迁移。
- Subject 从权限边界改为默认目标；同项目内其他资源是否可访问由 Route 与领域服务决定。
- queued follow-up、restart 与 user-input resume 使用冻结的 Prompt/Tool snapshot，避免恢复时读取新的默认配置。

## 当前结构对步骤二的后续修正

| 层面 | 步骤二产物 | 当前 `project_api_v2` |
| --- | --- | --- |
| Scene | 混合 Prompt、工具集合与 Route/Doc 推荐 | 只保留身份、Subject 上下文、安全边界、图片参考策略和 `RecommendedGuideIDs` |
| 工具 | Scene 间存在 3/4 工具差异 | 四个有效 Scene 统一为 `request_api`、`read_agent_doc`、`image_gen`、`request_user_input` |
| 文档发现 | Scene 推荐 API 索引 | Overview 能力/API 索引 + Guide Registry + API Contract |
| 图片尺寸 | 设定项 Scene 运行时强制 `512x512` | `image_gen` 无 Scene 尺寸分支；创建 Guide 明确默认 `512x512` |
| Prompt | Scene 内复制操作流程 | 流程抽入三份 Guide；Scene Prompt 只注入权威上下文和推荐 Guide 路径 |
| 恢复 | Project API 快照按当时协议恢复 | 只有 `project_api_v2` 可继续运行；更早 Project API Run 不恢复 |

## 当前四 Scene Tool 矩阵

| Scene | `request_api` | `read_agent_doc` | `image_gen` | `request_user_input` |
| --- | :---: | :---: | :---: | :---: |
| `project_assistant` | ✓ | ✓ | ✓ | ✓ |
| `premise_asset_generation` | ✓ | ✓ | ✓ | ✓ |
| `asset_reference` | ✓ | ✓ | ✓ | ✓ |
| `storyboard_reference` | ✓ | ✓ | ✓ | ✓ |

消息附件和绑定资产的自动图片引用仍由 `ImageReferencePolicy` 控制：`premise_asset_generation` 接收当前消息附件，`asset_reference` 以前置绑定资产图片为第一参考，另外两个 Scene 不自动绑定参考图。工具是否可调用不再由该策略决定。

## 当前 Guide 映射

当前 Guide 已按前端创作功能重组为 14 份中文文档，实际能力 ID 与路径以 `/api/v1/agent-docs/overview.md` 渲染的 Guide Registry 为准。旧英文 Guide 路径不再注册，也不提供兼容别名。

Overview 的能力索引由同一 Guide Registry 生成；`read_agent_doc` 只读取注册的 Overview、Guide 和 API Contract。Scene Markdown、任意文件路径、Query、Fragment、编码路径、反斜杠和路径穿越均拒绝。

## 安全不变量

- 所有外部标识使用 UUIDv7；内部 bigint `id`、路径、metadata 和凭据不会进入请求或响应。
- 写操作先读取最新事实和 revision；冲突后重新读取再决定是否重试。
- 危险 Route 使用稳定请求指纹绑定 Route、项目、目标、method/path/query/body、revision 和确认选项。
- 已有 Premise Asset 图片只能作为当前 Turn Reference 传给 `image_gen.reference_uuids`，不能直接作为创建或替换的 `file_uuid`。
- `file_uuid` 必须来自当前会话、用途匹配且尚未消费的 `image_gen` 输出；真正不存在、来源不合法和已消费分别返回不同领域错误。

## 验证原则

步骤二的旧成本数字和运行时强制逻辑不再作为当前基线。当前验收以四 Scene 相同工具顺序、Guide/Route 注册表一致性、Prompt 默认值迁移、`project_api_v2` 恢复边界、文件来源错误分类及完整业务回归为准。

未运行 Cargo 或任何 Rust 编译、检查及测试。
