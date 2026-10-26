# Lumi / pi 共有任务对照

比较范围：真实 Agent 核心；4 个通用任务。每项 0–1，1 表示该项观察条件满足，无统一通过门槛。

| 用例 / 分项 | Lumi 均值（有效/计划） | pi 均值（有效/计划） | Lumi − pi |
|---|---:|---:|---:|
| audit-inconclusive / verdict | 1.000 (3/3) | 1.000 (3/3) | +0.000 |
| audit-inconclusive / read_documentation | 1.000 (3/3) | 1.000 (3/3) | +0.000 |
| audit-inconclusive / read_implementation | 1.000 (3/3) | 1.000 (3/3) | +0.000 |
| audit-inconclusive / evidence | 1.000 (3/3) | 1.000 (3/3) | +0.000 |
| audit-match / verdict | 1.000 (3/3) | 1.000 (3/3) | +0.000 |
| audit-match / read_documentation | 1.000 (3/3) | 1.000 (3/3) | +0.000 |
| audit-match / read_implementation | 1.000 (3/3) | 1.000 (3/3) | +0.000 |
| audit-match / evidence | 0.667 (3/3) | 1.000 (3/3) | -0.333 |
| audit-mismatch / verdict | 1.000 (3/3) | 1.000 (3/3) | +0.000 |
| audit-mismatch / read_documentation | 1.000 (3/3) | 1.000 (3/3) | +0.000 |
| audit-mismatch / read_implementation | 1.000 (3/3) | 1.000 (3/3) | +0.000 |
| audit-mismatch / evidence | 1.000 (3/3) | 1.000 (3/3) | +0.000 |
| smoke / answer | 1.000 (3/3) | 1.000 (3/3) | +0.000 |

## 执行与波动

| 引擎 / 用例 | 状态 | 耗时均值/范围/标准差（秒） | 输入 / 输出 Token 总计 |
|---|---|---|---|
| lumi / audit-inconclusive | scored, scored, scored | 16.93 / 16.17–17.85 / 0.70 | 10667 / 2223 |
| lumi / audit-match | scored, scored, scored | 11.87 / 11.61–12.38 / 0.36 | 7504 / 1563 |
| lumi / audit-mismatch | scored, scored, scored | 11.93 / 10.70–13.68 / 1.27 | 7661 / 1625 |
| lumi / smoke | scored, scored, scored | 3.47 / 2.81–4.46 / 0.72 | 339 / 274 |
| pi / audit-inconclusive | scored, scored, scored | 18.34 / 13.70–22.15 / 3.50 | 14945 / 2132 |
| pi / audit-match | scored, scored, scored | 14.94 / 13.63–16.38 / 1.13 | 8915 / 1968 |
| pi / audit-mismatch | scored, scored, scored | 14.36 / 14.16–14.64 / 0.20 | 8634 / 1831 |
| pi / smoke | scored, scored, scored | 4.54 / 4.47–4.63 / 0.07 | 339 / 561 |

## 每次执行

- [lumi/audit-inconclusive/run-001](lumi/tasks/audit-inconclusive/run-001/result.json): scored; 
- [lumi/audit-inconclusive/run-002](lumi/tasks/audit-inconclusive/run-002/result.json): scored; 
- [lumi/audit-inconclusive/run-003](lumi/tasks/audit-inconclusive/run-003/result.json): scored; 
- [lumi/audit-match/run-001](lumi/tasks/audit-match/run-001/result.json): scored; 
- [lumi/audit-match/run-002](lumi/tasks/audit-match/run-002/result.json): scored; 
- [lumi/audit-match/run-003](lumi/tasks/audit-match/run-003/result.json): scored; 
- [lumi/audit-mismatch/run-001](lumi/tasks/audit-mismatch/run-001/result.json): scored; 
- [lumi/audit-mismatch/run-002](lumi/tasks/audit-mismatch/run-002/result.json): scored; 
- [lumi/audit-mismatch/run-003](lumi/tasks/audit-mismatch/run-003/result.json): scored; 
- [lumi/smoke/run-001](lumi/tasks/smoke/run-001/result.json): scored; 
- [lumi/smoke/run-002](lumi/tasks/smoke/run-002/result.json): scored; 
- [lumi/smoke/run-003](lumi/tasks/smoke/run-003/result.json): scored; 
- [pi/audit-inconclusive/run-001](pi/tasks/audit-inconclusive/run-001/result.json): scored; 
- [pi/audit-inconclusive/run-002](pi/tasks/audit-inconclusive/run-002/result.json): scored; 
- [pi/audit-inconclusive/run-003](pi/tasks/audit-inconclusive/run-003/result.json): scored; 
- [pi/audit-match/run-001](pi/tasks/audit-match/run-001/result.json): scored; 
- [pi/audit-match/run-002](pi/tasks/audit-match/run-002/result.json): scored; 
- [pi/audit-match/run-003](pi/tasks/audit-match/run-003/result.json): scored; 
- [pi/audit-mismatch/run-001](pi/tasks/audit-mismatch/run-001/result.json): scored; 
- [pi/audit-mismatch/run-002](pi/tasks/audit-mismatch/run-002/result.json): scored; 
- [pi/audit-mismatch/run-003](pi/tasks/audit-mismatch/run-003/result.json): scored; 
- [pi/smoke/run-001](pi/tasks/smoke/run-001/result.json): scored; 
- [pi/smoke/run-002](pi/tasks/smoke/run-002/result.json): scored; 
- [pi/smoke/run-003](pi/tasks/smoke/run-003/result.json): scored; 

## 解释边界

- This is a shared generic Agent-core benchmark, not the original pi product-task suite or a full application comparison.
- The task, fixtures, fixed system prompt, tool definitions, Go tool implementations and Go grading code are shared.
- pi uses its real Agent with streaming pi-ai; Lumi uses its real durable Agent with non-streaming requests. Native validation, context assembly, repair, retry and finalization policies remain different.
- Both request the same endpoint/model, omit temperature/reasoning fields, and cap output at 4096 tokens and logical model requests at 16 per turn. Provider-level retries can issue additional HTTP attempts.
- The model ID is a mutable provider alias. Engines run sequentially and are not a randomized causal experiment.
- Input/output token totals use provider prompt_tokens/completion_tokens, including cache and any reported reasoning tokens. Missing metrics and all costs are unavailable, never zero-filled.
- Audit evidence scoring only checks one final submission citing src/names.py and format_name; it does not prove the explanation is semantically correct.
- Small repeat counts show observed variation only. Low scores do not fail CI; incomplete or environment-failed evidence does.
