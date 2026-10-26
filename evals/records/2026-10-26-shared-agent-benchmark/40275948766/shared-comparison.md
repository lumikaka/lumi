# Lumi / pi 共有任务对照

比较范围：真实 Agent 核心；4 个通用任务。每项 0–1，1 表示该项观察条件满足，无统一通过门槛。

| 用例 / 分项 | Lumi 均值（有效/计划） | pi 均值（有效/计划） | Lumi − pi |
|---|---:|---:|---:|
| audit-inconclusive / verdict | 1.000 (3/3) | N/A | N/A |
| audit-inconclusive / read_documentation | 1.000 (3/3) | N/A | N/A |
| audit-inconclusive / read_implementation | 1.000 (3/3) | N/A | N/A |
| audit-inconclusive / evidence | 1.000 (3/3) | N/A | N/A |
| audit-match / verdict | 1.000 (3/3) | N/A | N/A |
| audit-match / read_documentation | 1.000 (3/3) | N/A | N/A |
| audit-match / read_implementation | 1.000 (3/3) | N/A | N/A |
| audit-match / evidence | 1.000 (3/3) | N/A | N/A |
| audit-mismatch / verdict | 1.000 (3/3) | N/A | N/A |
| audit-mismatch / read_documentation | 1.000 (3/3) | N/A | N/A |
| audit-mismatch / read_implementation | 1.000 (3/3) | N/A | N/A |
| audit-mismatch / evidence | 1.000 (3/3) | N/A | N/A |
| smoke / answer | 1.000 (3/3) | N/A | N/A |

## 执行与波动

| 引擎 / 用例 | 状态 | 耗时均值/范围/标准差（秒） | 输入 / 输出 Token 总计 |
|---|---|---|---|
| lumi / audit-inconclusive | scored, scored, scored | 18.43 / 17.00–19.52 / 1.06 | 10584 / 2523 |
| lumi / audit-match | scored, scored, scored | 11.80 / 10.71–13.83 / 1.43 | 7544 / 1596 |
| lumi / audit-mismatch | scored, scored, scored | 14.28 / 12.64–15.26 / 1.17 | 7673 / 1949 |
| lumi / smoke | scored, scored, scored | 2.78 / 2.18–3.33 / 0.47 | 339 / 255 |
| pi / audit-inconclusive | environment_error, environment_error, environment_error | 0.06 / 0.06–0.07 / 0.00 | N/A / N/A |
| pi / audit-match | environment_error, environment_error, environment_error | 0.06 / 0.06–0.06 / 0.00 | N/A / N/A |
| pi / audit-mismatch | environment_error, environment_error, environment_error | 0.06 / 0.06–0.06 / 0.00 | N/A / N/A |
| pi / smoke | environment_error, environment_error, environment_error | 0.06 / 0.06–0.07 / 0.00 | N/A / N/A |

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
- [pi/audit-inconclusive/run-001](pi/tasks/audit-inconclusive/run-001/result.json): environment_error; adapter_process: pi adapter exited 1
- [pi/audit-inconclusive/run-002](pi/tasks/audit-inconclusive/run-002/result.json): environment_error; adapter_process: pi adapter exited 1
- [pi/audit-inconclusive/run-003](pi/tasks/audit-inconclusive/run-003/result.json): environment_error; adapter_process: pi adapter exited 1
- [pi/audit-match/run-001](pi/tasks/audit-match/run-001/result.json): environment_error; adapter_process: pi adapter exited 1
- [pi/audit-match/run-002](pi/tasks/audit-match/run-002/result.json): environment_error; adapter_process: pi adapter exited 1
- [pi/audit-match/run-003](pi/tasks/audit-match/run-003/result.json): environment_error; adapter_process: pi adapter exited 1
- [pi/audit-mismatch/run-001](pi/tasks/audit-mismatch/run-001/result.json): environment_error; adapter_process: pi adapter exited 1
- [pi/audit-mismatch/run-002](pi/tasks/audit-mismatch/run-002/result.json): environment_error; adapter_process: pi adapter exited 1
- [pi/audit-mismatch/run-003](pi/tasks/audit-mismatch/run-003/result.json): environment_error; adapter_process: pi adapter exited 1
- [pi/smoke/run-001](pi/tasks/smoke/run-001/result.json): environment_error; adapter_process: pi adapter exited 1
- [pi/smoke/run-002](pi/tasks/smoke/run-002/result.json): environment_error; adapter_process: pi adapter exited 1
- [pi/smoke/run-003](pi/tasks/smoke/run-003/result.json): environment_error; adapter_process: pi adapter exited 1

## 解释边界

- This is a shared generic Agent-core benchmark, not the original pi product-task suite or a full application comparison.
- The task, fixtures, fixed system prompt, tool definitions, Go tool implementations and Go grading code are shared.
- pi uses its real Agent with streaming pi-ai; Lumi uses its real durable Agent with non-streaming requests. Native validation, context assembly, repair, retry and finalization policies remain different.
- Both request the same endpoint/model, omit temperature/reasoning fields, and cap output at 4096 tokens and logical model requests at 16 per turn. Provider-level retries can issue additional HTTP attempts.
- The model ID is a mutable provider alias. Engines run sequentially and are not a randomized causal experiment.
- Input/output token totals use provider prompt_tokens/completion_tokens, including cache and any reported reasoning tokens. Missing metrics and all costs are unavailable, never zero-filled.
- Audit evidence scoring only checks one final submission citing src/names.py and format_name; it does not prove the explanation is semantically correct.
- Small repeat counts show observed variation only. Low scores do not fail CI; incomplete or environment-failed evidence does.

## 证据问题

- pi/audit-inconclusive/1: environment_error
- pi/audit-inconclusive/2: environment_error
- pi/audit-inconclusive/3: environment_error
- pi/audit-match/1: environment_error
- pi/audit-match/2: environment_error
- pi/audit-match/3: environment_error
- pi/audit-mismatch/1: environment_error
- pi/audit-mismatch/2: environment_error
- pi/audit-mismatch/3: environment_error
- pi/smoke/1: environment_error
- pi/smoke/2: environment_error
- pi/smoke/3: environment_error
