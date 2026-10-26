# 首批 Agent 表现观察

> 历史记录说明：下文为 2026-10-25 题集的观察。相关 pi 专有模拟任务已退役，不作为当前产品能力结论；新题集与真实对照见 [2026-10-26 记录](records/2026-10-26-shared-agent-benchmark/README.md)。

这些是固定 `qwen3.7-plus`、固定基础提示词和隔离模拟环境中的观察，不是对模型总体能力的结论。本轮仅交付评测体系，没有据此修改业务 Agent 策略或放宽判据。

## 配置修复的重复诊断及预算收尾

证据：[repair-config 第一次结果](records/2026-10-25-qwen3.7-plus/tasks/repair-config/run-001/result.json)、[完整请求和工具轨迹](records/2026-10-25-qwen3.7-plus/tasks/repair-config/run-001/trace.jsonl)。

任务要求先加载配置，依据错误和文档修复 `config/hello.json`，重新加载，并通过工具向 Ada 问好。初始文件含尾逗号，文档明确要求严格 JSON。

实际过程：

1. 第一次模型响应调用 `eval_reload` 和 `eval_read docs/extensions.md`。加载结果为 `config/hello.json: invalid JSON`，文档内容也正常返回。
2. 第二次模型请求已经包含上一批两个工具的完整结果；模型仍重复同一批加载和读文档操作，没有读或修改损坏配置。工具参数与结果均相同。
3. Lumi 生产运行器的 `DefaultMaxNoProgressRounds=2` 触发无进展预算。第三次请求移除工具定义，并追加固定的预算收尾指令，要求诚实说明未完成工作。
4. 模型仍返回了 `eval_read config/hello.json` 工具调用。生产运行器没有执行它，任务以 `agent_turn_budget_exceeded` 结束。

该次实际执行了四个工具调用、三个模型请求，没有写入配置。`repaired` 与 `valid_configuration` 得分均为 0，`reload_diagnostic` 的两次调用计数分为 1。最后一项不能解释为修复成功，也不会覆盖前两项失败。真实 Provider 请求均有正常响应；故障属于 Agent 行为及其与运行器预算的交互，不是环境故障。

[第二次结果](records/2026-10-25-qwen3.7-plus/tasks/repair-config/run-002/result.json) 和 [第二次轨迹](records/2026-10-25-qwen3.7-plus/tasks/repair-config/run-002/trace.jsonl) 显示：相同配置和初始条件下，Agent 用六次模型请求完成读取、修复、加载和问好，各分项均为 1。两次结果的差异验证了重复运行及波动报告机制；两个样本不足以估计稳定失败率。

后续可独立验证的改进建议：

- 针对相同诊断结果重复出现的情形，实验一个明确提示“已有诊断，下一步应改变状态或说明无法推进”的恢复策略；继续保留当前停止保护，并用更大重复次数评估额外请求成本与修复收益。
- 单独测试 Provider 在无工具定义、仅保留历史工具消息时的收尾行为；评估 Provider 支持的显式禁用工具选择选项。保持当前对非法收尾工具调用的拒绝，不能把它记为实际执行。
- 将“诊断后采取修复动作”和“预算后返回诚实纯文本”拆成针对性回归场景。原始失败用例和记录应保留，不通过改变正确结果或补造模型输出消除失败。

## 评测设计问题与能力问题分开

开发时发现过未说明 `input` 数组类型的用例歧义，以及无参数工具 schema 使用 null 的兼容性问题；两者已在完整批次前修复，详情见 [验证记录](verification.md)。早期机械零分不作为本文件的 Agent 能力证据。
