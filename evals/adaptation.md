# pi evals 参考与适配清单

2026-10-26 调整：删除依赖 pi 专有产品功能的模拟任务。默认回归组从 14 例缩减为 **8 例**，全部 6 个 `pi-reference` 变体退役。原始报告、执行记录、参考源码和散列清单保留，不改写历史成绩。

初始研究版本为 [38ff99177fba21e9c57a670f4e95adcf407c99c7](https://github.com/earendil-works/pi/tree/38ff99177fba21e9c57a670f4e95adcf407c99c7/packages/evals)，研究日期 2026-10-25。[reference.json](reference.json) 保存原始文件 Git blob SHA。后续双引擎历史运行固定 `cfee633b1ba9497b0c12462da97a08b418b5ebf8`，见 [历史记录](records/2026-10-25-github-baseline/README.md)。

## 保留的用例

| 用例 | 验证内容 | 与 pi 的关系 |
|---|---|---|
| `smoke` | 基础任务与精确输出格式 | 源自 smoke；共有核心对照 |
| `audit-match` | 文档与实现一致时给出有依据的结论 | 源自文档审计机制；共有核心对照 |
| `audit-mismatch` | 发现文档与实现不一致 | 同上 |
| `audit-inconclusive` | 证据缺失时保留不确定性 | 同上 |
| `batch-task` | 读取多个来源、计算、写入并验证，保留源数据 | Lumi 通用回归补充 |
| `context-handoff` | 多轮保存、纠正并读取状态 | 借鉴多轮输入组织；Lumi 通用回归 |
| `transient-tool` | 暂时工具故障后的重试恢复 | Lumi 通用回归补充 |
| `missing-resource` | 资源不存在时诚实报告 | Lumi 通用回归补充 |

共有核心对照已接入手动 CI：pi 固定源码 `cfee633b1ba9497b0c12462da97a08b418b5ebf8`，直接调用真实 `pi-agent-core.Agent` 与 `pi-ai`；Lumi 调用真实 `ExecuteJob`。两边共享现有 JSON、固定提示词、工具 schema、Go 环境和 Go 评分器。它属于通用核心能力基准，不是上游原始产品题集成绩；不得将旧模型配置/提示模板分数套用过来。

## 移除的用例

| 类别 | 用例 | 原因 |
|---|---|---|
| 添加模型 | `add-model`、`pi-add-model-with-docs`、`pi-add-model-without-docs` | 模拟模型注册，不是 Lumi Agent 的实际产品操作 |
| Provider 配置 | `configure-provider`、`custom-stream-provider` | 依赖模拟协议和声明式 Provider 运行时 |
| 提示模板 | `pi-prompt-template-project-*`、`pi-prompt-template-user-*`，共 4 例 | 依赖 pi 目录、资源发现、reload 和命令展开 |
| 扩展及修复 | `extension-reload`、`repair-config` | 依赖模拟 greeting 扩展，后者也建立在该功能上 |
| TUI 页脚 | `status-footer` | 模拟 pi 终端渲染，不能代表 Lumi 产品能力 |

共删除 12 个可运行定义（6 个默认回归用例 + 6 个历史对照变体），以及对应的配置、模板、Provider、扩展和页脚模拟执行/评分逻辑。`--suite pi-reference` 在调用模型前拒绝执行，旧 CI 专有任务构建入口已替换。

## 保留的机制与边界

| 机制 | 当前实现 |
|---|---|
| 组织与发现 | `cases/*.json` 完整描述目标、初始条件、可观察结果和评分依据；支持 list/filter/cases |
| 固定配置与重复 | 预先生成 case × repetition 计划；单一模型与配置；偶数轮反向顺序 |
| 真实 Agent | `agent.NewIsolatedSession` → `CreateTurn` → `ExecuteJob`；共用上下文、工具持久化、参数修复、预算与收尾 |
| 隔离 | 每次独立临时 SQLite、固定 prompt、虚拟文件和工具；不导入业务工具、业务数据或真实用户项目 |
| 多轮衔接 | 同例共享会话与文件；跨用例和重复隔离；仅 prompt 步骤 |
| 工具与故障 | 文件读写、列表、搜索、审计提交、记录获取；通过环境注入故障，模型响应保持真实 |
| 评分 | 最终文本、文件末态、调用名称/参数/次数、审计证据；不使用模型自评，不设统一通过门槛 |
| 记录与报告 | 完整规范化模型请求/响应、工具过程、持久化 items、耗时、可获取 Token；可读及结构化报告 |
| 故障归因 | Agent 错误保留评分；环境故障和缺失指标标记不可用，不补零 |
| 完整性与波动 | 精确计划、缺失/重复/损坏校验；分项有效数、均值、范围、标准差 |
| 手动入口 | 本地 `make eval-agent`；远程 `agent-baseline.yml` 运行 8 项 Lumi 回归和 4 项 pi 对照；pi 只在 GitHub 临时机器安装和构建，本机无 pi/Docker/Rust 构建 |
| 历史复查 | 保留 compare/report 和历史包读取；使用旧协议中的定义与散列，不依赖当前可执行题集 |

模拟工具和隔离环境仍是通用评测所需部分，此次移除的是不适用的产品任务。没有 shell、网络、任意生成代码执行或读取评分规则的工具，不声称是恶意代码的容器防护。

无真实运行记录时不宣称对照完成。新增场景或模型后保存新目录，不改写既有证据。
