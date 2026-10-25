# Lumi Agent evaluation report

Model: `qwen3.7-plus` (aliyun_bailian). Planned: 28; recorded: 5. Repetitions: 2.

Protocol: `805ba05526eacc51d4c3dcdeb2939a0c2a1389c8d45700b576c9850bedf68eab`

Scores describe each criterion on [0,1]; there is no global acceptance threshold. Agent errors retain observable scores. Environment failures are unscored and excluded from score means. Incomplete cohorts cannot support a complete-run claim. N/A means unavailable, never zero. One repetition cannot establish stability; standard deviation is population SD and is N/A for fewer than two observations. Cost is N/A because this harness has no frozen pricing source.

## Environment / evidence gaps

- add-model/run-002: expected one result, found 0
- audit-inconclusive/run-002: expected one result, found 0
- audit-match/run-002: expected one result, found 0
- audit-mismatch/run-002: expected one result, found 0
- batch-task/run-001: environment_error
- batch-task/run-002: expected one result, found 0
- configure-provider/run-001: expected one result, found 0
- configure-provider/run-002: expected one result, found 0
- context-handoff/run-001: expected one result, found 0
- context-handoff/run-002: expected one result, found 0
- custom-stream-provider/run-001: expected one result, found 0
- custom-stream-provider/run-002: expected one result, found 0
- extension-reload/run-001: expected one result, found 0
- extension-reload/run-002: expected one result, found 0
- missing-resource/run-001: expected one result, found 0
- missing-resource/run-002: expected one result, found 0
- repair-config/run-001: expected one result, found 0
- repair-config/run-002: expected one result, found 0
- smoke/run-001: expected one result, found 0
- smoke/run-002: expected one result, found 0
- status-footer/run-001: expected one result, found 0
- status-footer/run-002: expected one result, found 0
- transient-tool/run-001: expected one result, found 0
- transient-tool/run-002: expected one result, found 0

## Repetition statistics

| Case | Recorded scores / planned | Criterion | Mean | Min | Max | SD | Variation |
|---|---:|---|---:|---:|---:|---:|---|
| add-model | 1/2 | registry | 1.000 | 1.000 | 1.000 | N/A | false |
| add-model | 1/2 | verified | 1.000 | 1.000 | 1.000 | N/A | false |
| audit-inconclusive | 1/2 | evidence | 1.000 | 1.000 | 1.000 | N/A | false |
| audit-inconclusive | 1/2 | read_documentation | 1.000 | 1.000 | 1.000 | N/A | false |
| audit-inconclusive | 1/2 | read_implementation | 1.000 | 1.000 | 1.000 | N/A | false |
| audit-inconclusive | 1/2 | verdict | 1.000 | 1.000 | 1.000 | N/A | false |
| audit-match | 1/2 | evidence | 1.000 | 1.000 | 1.000 | N/A | false |
| audit-match | 1/2 | read_documentation | 1.000 | 1.000 | 1.000 | N/A | false |
| audit-match | 1/2 | read_implementation | 1.000 | 1.000 | 1.000 | N/A | false |
| audit-match | 1/2 | verdict | 1.000 | 1.000 | 1.000 | N/A | false |
| audit-mismatch | 1/2 | evidence | 1.000 | 1.000 | 1.000 | N/A | false |
| audit-mismatch | 1/2 | read_documentation | 1.000 | 1.000 | 1.000 | N/A | false |
| audit-mismatch | 1/2 | read_implementation | 1.000 | 1.000 | 1.000 | N/A | false |
| audit-mismatch | 1/2 | verdict | 1.000 | 1.000 | 1.000 | N/A | false |
| batch-task | 0/2 | aggregate | N/A | N/A | N/A | N/A | false |
| batch-task | 0/2 | source_a_preserved | N/A | N/A | N/A | N/A | false |
| batch-task | 0/2 | source_b_preserved | N/A | N/A | N/A | N/A | false |
| batch-task | 0/2 | verification | N/A | N/A | N/A | N/A | false |
| configure-provider | 0/2 | runtime_request | N/A | N/A | N/A | N/A | false |
| configure-provider | 0/2 | verified | N/A | N/A | N/A | N/A | false |
| context-handoff | 0/2 | continuity | N/A | N/A | N/A | N/A | false |
| context-handoff | 0/2 | updated_state | N/A | N/A | N/A | N/A | false |
| custom-stream-provider | 0/2 | runtime_request | N/A | N/A | N/A | N/A | false |
| custom-stream-provider | 0/2 | verified | N/A | N/A | N/A | N/A | false |
| extension-reload | 0/2 | greeting | N/A | N/A | N/A | N/A | false |
| extension-reload | 0/2 | invocation | N/A | N/A | N/A | N/A | false |
| extension-reload | 0/2 | loaded_tool | N/A | N/A | N/A | N/A | false |
| missing-resource | 0/2 | honest_failure | N/A | N/A | N/A | N/A | false |
| missing-resource | 0/2 | single_attempt | N/A | N/A | N/A | N/A | false |
| repair-config | 0/2 | reload_diagnostic | N/A | N/A | N/A | N/A | false |
| repair-config | 0/2 | repaired | N/A | N/A | N/A | N/A | false |
| repair-config | 0/2 | valid_configuration | N/A | N/A | N/A | N/A | false |
| smoke | 0/2 | answer | N/A | N/A | N/A | N/A | false |
| status-footer | 0/2 | bar_correctness | N/A | N/A | N/A | N/A | false |
| status-footer | 0/2 | layout_preservation | N/A | N/A | N/A | N/A | false |
| transient-tool | 0/2 | recovered_answer | N/A | N/A | N/A | N/A | false |
| transient-tool | 0/2 | retried | N/A | N/A | N/A | N/A | false |

## Individual executions

| Task | Status | Duration (s) | Requests | Input tokens | Output tokens | Cached input | Evidence |
|---|---|---:|---:|---:|---:|---:|---|
| add-model/run-001 | scored | 33.58 | 8 | 9811 | 1591 | 4480 | [result](tasks/add-model/run-001/result.json), [trace](tasks/add-model/run-001/trace.jsonl) |
| audit-inconclusive/run-001 | scored | 17.48 | 4 | 3549 | 796 | 1536 | [result](tasks/audit-inconclusive/run-001/result.json), [trace](tasks/audit-inconclusive/run-001/trace.jsonl) |
| audit-match/run-001 | scored | 11.61 | 3 | 2513 | 549 | 1536 | [result](tasks/audit-match/run-001/result.json), [trace](tasks/audit-match/run-001/trace.jsonl) |
| audit-mismatch/run-001 | scored | 13.39 | 3 | 2609 | 669 | 1536 | [result](tasks/audit-mismatch/run-001/result.json), [trace](tasks/audit-mismatch/run-001/trace.jsonl) |
| batch-task/run-001 | environment_error | 7.25 | 3 | N/A | N/A | N/A | [result](tasks/batch-task/run-001/result.json), [trace](tasks/batch-task/run-001/trace.jsonl) |

### add-model/run-001

Final response:

> The probe confirms the addition is successful. Here's the verification:
> 
> **Registry contents (2 entries):**
> 
> | Field | `existing` | `fixture-chat` |
> |---|---|---|
> | `name` | `"existing"` | `"fixture-chat"` ✅ |
> | `display_name` | — | `"Fixture Chat"` ✅ |
> | `reasoning` | — | `true` ✅ |
> | `input` | — | `["text"]` ✅ |
> | `context_window` | `8192` | `32768` ✅ |
> | `max_tokens` | — | `4096` ✅ |
> | `cost` | — | `0` ✅ |
> 
> **Summary:**
> - The original `existing` entry is **preserved unchanged** with `context_window: 8192`.
> - The new `fixture-chat` entry is present with all fields correctly set: displayed as **"Fixture Chat"**, reasoning enabled, text input supported, 32768 context window, 4096 max output tokens, and zero cost.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| registry | 1.000 | registry | {"items":[{"context_window":8192,"name":"existing"},{"context_window":32768,"cost":0,"display_name":"Fixture Chat","input":["text"],"max_tokens":4096,"name":"fixture-chat","reasoning":true}]} |
| verified | 1.000 | verified | 2 |

Tool execution sequence (full results in evidence):

1. `eval_list` {} — 0 ms
2. `eval_read` {"path":"config/models.json"} — 0 ms
3. `eval_read` {"path":"docs/models.md"} — 0 ms
4. `eval_write` {"content":"{\"items\": [{\"name\": \"existing\", \"context_window\": 8192}, {\"name\": \"fixture-chat\", \"display_name\": \"Fixture Chat\", \"reasoning\": true, \"input\": [\"text\"], \"context_window\": 32768, \"max_tokens\": 4096, \"cost\": 0}]}","path":"config/models.json"} — 0 ms
5. `eval_reload` {} — 0 ms
6. `eval_probe` {"input":"","target":"models"} — 0 ms
7. `eval_probe` {"input":"","target":"models"} — 0 ms

### audit-inconclusive/run-001

Final response:

> ## Summary
> 
> **Claim:** `format_name` preserves capitalization (docs/names.md).
> 
> **Implementation:** `src/names.py` defines `format_name` as a thin wrapper that delegates to `normalize` from `external_formatter`. The external formatter's source is not present in the workspace, so the actual transformation applied to the name cannot be inspected.
> 
> **Verdict:** `inconclusive` — the claim cannot be confirmed or refuted from the available code.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| verdict | 1.000 | verdict | 1 |
| read_documentation | 1.000 | read documentation | 1 |
| read_implementation | 1.000 | read implementation | 1 |
| evidence | 1.000 | One explanation cites src/names.py and format_name, and submission is the final tool action. This is a mechanical evidence check, not a semantic proof of explanation quality. | ["Claim (docs/names.md): \"The public format_name function preserves capitalization.\" Implementation (src/names.py): format_name delegates entirely to `normalize` imported from `external_formatter` (`return normalize(name)`). The external formatter's implementation is not included in the workspace (the file itself notes: \"The external formatter implementation is not included.\"), and no other file in the workspace defines `external_formatter`. Because the actual behavior of `normalize` is unavailable, we cannot determine whether capitalization is preserved. Verdict: inconclusive — evidence for the claim is not present in the codebase."] |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/names.md"} — 0 ms
2. `eval_read` {"path":"src/names.py"} — 0 ms
3. `eval_search` {"query":"external_formatter"} — 0 ms
4. `eval_submit` {"explanation":"Claim (docs/names.md): \"The public format_name function preserves capitalization.\" Implementation (src/names.py): format_name delegates entirely to `normalize` imported from `external_formatter` (`return normalize(name)`). The external formatter's implementation is not included in the workspace (the file itself notes: \"The external formatter implementation is not included.\"), and no other file in the workspace defines `external_formatter`. Because the actual behavior of `normalize` is unavailable, we cannot determine whether capitalization is preserved. Verdict: inconclusive — evidence for the claim is not present in the codebase.","verdict":"inconclusive"} — 0 ms

### audit-match/run-001

Final response:

> **Summary:** The documentation in `docs/names.md` claims that `format_name` trims whitespace and lowercases names. The implementation in `src/names.py` confirms this: `format_name` delegates to `normalize`, which calls `value.strip().lower()`. Both claimed behaviors are faithfully implemented. Verdict: **match**.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| verdict | 1.000 | verdict | 1 |
| read_documentation | 1.000 | read documentation | 1 |
| read_implementation | 1.000 | read implementation | 1 |
| evidence | 1.000 | One explanation cites src/names.py and format_name, and submission is the final tool action. This is a mechanical evidence check, not a semantic proof of explanation quality. | ["docs/names.md claims: \"The public format_name function trims whitespace and lowercases names.\" In src/names.py, format_name(name) calls normalize(name), which returns value.strip().lower(). str.strip() trims leading/trailing whitespace and str.lower() lowercases the string. Both explicit user-visible claims (trim whitespace, lowercase) are correctly implemented."] |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/names.md"} — 0 ms
2. `eval_read` {"path":"src/names.py"} — 0 ms
3. `eval_submit` {"explanation":"docs/names.md claims: \"The public format_name function trims whitespace and lowercases names.\" In src/names.py, format_name(name) calls normalize(name), which returns value.strip().lower(). str.strip() trims leading/trailing whitespace and str.lower() lowercases the string. Both explicit user-visible claims (trim whitespace, lowercase) are correctly implemented.","verdict":"match"} — 0 ms

### audit-mismatch/run-001

Final response:

> **Summary:** Documentation mismatch found. The docs claim `format_name` "trims whitespace and preserves capitalization," but the implementation in src/names.py shows it calls `normalize()` which applies `.strip().lower()`. While whitespace trimming is correct, the function lowercases all input rather than preserving capitalization.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| verdict | 1.000 | verdict | 1 |
| read_documentation | 1.000 | read documentation | 1 |
| read_implementation | 1.000 | read implementation | 1 |
| evidence | 1.000 | One explanation cites src/names.py and format_name, and submission is the final tool action. This is a mechanical evidence check, not a semantic proof of explanation quality. | ["The documentation (docs/names.md) claims that the public `format_name` function \"trims whitespace and preserves capitalization.\" In src/names.py, `format_name(name)` delegates to `normalize(name)`, which returns `value.strip().lower()`. While `.strip()` does trim whitespace (matching the first part of the claim), `.lower()` converts the entire string to lowercase, which directly contradicts the claim that capitalization is preserved. The actual behavior is that `format_name` trims whitespace AND lowercases the input, so the \"preserves capitalization\" claim is incorrect."] |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/names.md"} — 0 ms
2. `eval_read` {"path":"src/names.py"} — 0 ms
3. `eval_submit` {"explanation":"The documentation (docs/names.md) claims that the public `format_name` function \"trims whitespace and preserves capitalization.\" In src/names.py, `format_name(name)` delegates to `normalize(name)`, which returns `value.strip().lower()`. While `.strip()` does trim whitespace (matching the first part of the claim), `.lower()` converts the entire string to lowercase, which directly contradicts the claim that capitalization is preserved. The actual behavior is that `format_name` trims whitespace AND lowercases the input, so the \"preserves capitalization\" claim is incorrect.","verdict":"mismatch"} — 0 ms

### batch-task/run-001

Final response:

> 

Diagnostics:

- environment / case_timeout_or_cancelled: context canceled
- environment / cancelled: 请求已取消。

| Criterion | Score | Basis | Observed |
|---|---:|---|---|

Tool execution sequence (full results in evidence):

1. `eval_list` {} — 0 ms
2. `eval_read` {"path":"records/a.json"} — 0 ms
3. `eval_read` {"path":"records/b.json"} — 0 ms
