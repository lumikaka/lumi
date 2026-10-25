# 历史对照交付记录 — 2026-10-25

本次没有安装或运行 pi，也没有运行 Cargo/Rust 命令。公开历史成绩、Lumi 真实执行、离线测试替身分别保存，不混为同一种证据。

## 可复查产物

- [历史对照报告](../../records/2026-10-25-pi-historical-comparison/comparison.md) 与 [结构化对照](../../records/2026-10-25-pi-historical-comparison/comparison.json)
- [主批次完整报告](../../records/2026-10-25-pi-reference-qwen3.7-plus/report.md)：每个 result 和 trace 的链接均在报告内
- [最终协议元数据验证](../../records/2026-10-25-pi-reference-protocol-check/report.md)
- [公开基线目录](catalog.json)、[原始源代码清单](reference-manifest.json)、[检查记录](../../records/pi-history-checks/checks.json)

## 真实执行

主批次使用 aliyun_bailian / qwen3.7-plus；每个变体重复 3 次，固定 max_tokens=4096、每轮最多 16 次模型请求、每例 120 秒；temperature 未显式设置。三个场景、两个文档变体共 18 次，全部留下记录：14 次 scored、4 次 agent_error、0 次 environment_error。没有证据缺口，命令退出 0；该退出码不表示任务全部成功。

| 场景 | 文档条件 | Lumi 全项满分 / 有效次数 | pi 摘要 | 结论范围 |
|---|---|---:|---:|---|
| 项目模板 | 有 | 2/3 | 66.7%，n=3 | 仅并列参考 |
| 项目模板 | 无 | 2/3 | 100%，n=3 | 仅并列参考 |
| 用户模板 | 有 | 3/3 | 100%，n=3 | 仅并列参考 |
| 用户模板 | 无 | 1/3 | 100%，n=3 | 仅并列参考 |
| 模型配置 | 有 | 0/3 | 100%，n=1 | 仅并列参考 |
| 模型配置 | 无 | 0/3 | 0%，n=1 | 仅并列参考 |

8 次满足全部观察条件。共 142 次真实模型请求；各任务耗时合计 731.574 秒，输入 369,575 Token、输出 37,754 Token，其中缓存输入 250,624 Token。此处只陈述本地观察，不把比例差、耗时或 Token 差归因于两个 Agent 的能力。pi 的模型、执行配置、逐次记录及用量没有取得；PR #9898 的 13/15 和 2/15 仅作为整体摘要单列。

主批次启动时，协议顶层仍沿用回归组的参考版本和默认提示词字段；每个 case 的 `reference`、`system_prompt` 及模型请求 trace 已正确保存实际执行条件。随后仅修正协议元数据表达：`prompt_mode=per_case`，实际提示词逐例冻结，顶层参考版本指向所移植的 PR 分支。使用最终代码另做 1 次独立真实运行验证了这些字段；该次 scored、任务得分 0，完整保留，未混入主批次的 18 次统计。主批次的旧协议和散列没有被重写。

## 观察到的问题

1. **配置字段猜测导致任务不满足要求。** [模型配置有文档 run-001](../../records/2026-10-25-pi-reference-qwen3.7-plus/tasks/pi-add-model-with-docs/run-001/result.json) 写入 `maxOutput`，原任务观察需要 `maxTokens`；价格对象还缺少 `cacheRead/cacheWrite`。Agent 最终说明声称已完成，但加载后的观察未满足要求。原 TypeBox schema 的四项价格字段可在固定 [model-config.ts](reference/packages/coding-agent/src/core/model-config.ts) 核对。建议后续研究：配置字段应引用可获得的明确结构，无法核实时在最终说明中保留不确定性。当前仅提供文档子集也是实验限制，不能把错误完全归因于 Agent。
2. **无文档时放错模板目录。** [用户模板无文档 run-002](../../records/2026-10-25-pi-reference-qwen3.7-plus/tasks/pi-prompt-template-user-without-docs/run-002/result.json) 写入 `~/.pi/agent/prompt-templates/current-time.md`，本场景要求 `prompts/current-time.md`，因此 reload 后没有模板，实际用户消息也未展开。建议把路径定位和加载后确认分开观测；不能仅凭文件写入成功宣称命令可用。
3. **重复查询导致无进展终止，收尾继续尝试工具。** [项目模板有文档 run-002](../../records/2026-10-25-pi-reference-qwen3.7-plus/tasks/pi-prompt-template-project-with-docs/run-002/trace.jsonl) 的前两次请求重复相同两个 ls 调用；预算收尾阶段仍尝试查询文档目录，最终记录 `agent_turn_budget_exceeded`。建议后续研究重复空结果后的策略切换及收尾约束遵循。本轮保留生产行为，没有放宽预算、改 Agent 或伪造回答来提高分数。

这些是此模型与当前 Go 模拟环境组合的证据；不声称 pi 在完全相同条件下会成功。两次出现相同比例也不证明能力相当。

## 验证

- `go test ./...` 通过，日志保存在 [go-test.txt](../../records/pi-history-checks/go-test.txt)。最终报告/协议字段修改后，又通过 `go test ./internal/agentevals ./cmd/lumi_evals`。
- `go vet ./internal/agentevals ./internal/agent ./cmd/lumi_evals`、`git diff --check` 通过。
- 已验证虚拟路径越界、只读文档、reload 前后生效、实际模板内容展开、错误目录和重名模板、旧模型保留、基线散列、用例映射漂移、缺失指标与不可比较条件。
- 对照报告离线重建后，除输出位置决定的相对链接之外，结构化内容完全一致；主批次全部 protocol、report、result、trace 文件的散列保持不变。
- 旧 14 例报告仍可读取。历史比较不会把原通用场景当成 pi 原题；旧报告已有的两次环境错误仍保留并导致退出 1。

## 尚缺的上游证据

要形成可直接计算差值的基线，仍需取得 pi 的受测模型/Provider、实际运行提交、完整参数与文档条件、逐次原始结果，并审核工具及环境等价性。当前交付完成了历史数据收集、保留原题的本地适配、真实执行和近似对照入口；没有宣称已经测得纯 Agent 能力差距。
