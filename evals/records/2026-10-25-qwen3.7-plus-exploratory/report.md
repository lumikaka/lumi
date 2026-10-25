# Lumi Agent evaluation report

Model: `qwen3.7-plus` (aliyun_bailian). Planned: 28; recorded: 9. Repetitions: 2.

Protocol: `79a74331ac2a3711e591d2fcbb520ca65436456027be0e2b641ea4a9f82e69f2`

Scores describe each criterion on [0,1]; there is no global acceptance threshold. Agent errors retain observable scores. Environment failures are unscored and excluded from score means. Incomplete cohorts cannot support a complete-run claim. N/A means unavailable, never zero. One repetition cannot establish stability; standard deviation is population SD and is N/A for fewer than two observations. Cost is N/A because this harness has no frozen pricing source.

## Environment / evidence gaps

- add-model/run-002: expected one result, found 0
- audit-inconclusive/run-002: expected one result, found 0
- audit-match/run-002: expected one result, found 0
- audit-mismatch/run-002: expected one result, found 0
- batch-task/run-002: expected one result, found 0
- configure-provider/run-002: expected one result, found 0
- context-handoff/run-002: expected one result, found 0
- custom-stream-provider/run-002: expected one result, found 0
- extension-reload/run-001: environment_error
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
| add-model | 1/2 | registry | 0.000 | 0.000 | 0.000 | N/A | false |
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
| batch-task | 1/2 | aggregate | 1.000 | 1.000 | 1.000 | N/A | false |
| batch-task | 1/2 | source_a_preserved | 1.000 | 1.000 | 1.000 | N/A | false |
| batch-task | 1/2 | source_b_preserved | 1.000 | 1.000 | 1.000 | N/A | false |
| batch-task | 1/2 | verification | 1.000 | 1.000 | 1.000 | N/A | false |
| configure-provider | 1/2 | runtime_request | 1.000 | 1.000 | 1.000 | N/A | false |
| configure-provider | 1/2 | verified | 1.000 | 1.000 | 1.000 | N/A | false |
| context-handoff | 1/2 | continuity | 1.000 | 1.000 | 1.000 | N/A | false |
| context-handoff | 1/2 | updated_state | 1.000 | 1.000 | 1.000 | N/A | false |
| custom-stream-provider | 1/2 | runtime_request | 1.000 | 1.000 | 1.000 | N/A | false |
| custom-stream-provider | 1/2 | verified | 1.000 | 1.000 | 1.000 | N/A | false |
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
| add-model/run-001 | scored | 29.45 | 8 | 9415 | 1395 | 4352 | [result](tasks/add-model/run-001/result.json), [trace](tasks/add-model/run-001/trace.jsonl) |
| audit-inconclusive/run-001 | scored | 19.71 | 4 | 3648 | 918 | 1536 | [result](tasks/audit-inconclusive/run-001/result.json), [trace](tasks/audit-inconclusive/run-001/trace.jsonl) |
| audit-match/run-001 | scored | 13.88 | 3 | 2529 | 702 | 1536 | [result](tasks/audit-match/run-001/result.json), [trace](tasks/audit-match/run-001/trace.jsonl) |
| audit-mismatch/run-001 | scored | 14.21 | 3 | 2563 | 704 | 1536 | [result](tasks/audit-mismatch/run-001/result.json), [trace](tasks/audit-mismatch/run-001/trace.jsonl) |
| batch-task/run-001 | scored | 14.11 | 6 | 4412 | 549 | 2048 | [result](tasks/batch-task/run-001/result.json), [trace](tasks/batch-task/run-001/trace.jsonl) |
| configure-provider/run-001 | scored | 28.16 | 9 | 10194 | 1198 | 4864 | [result](tasks/configure-provider/run-001/result.json), [trace](tasks/configure-provider/run-001/trace.jsonl) |
| context-handoff/run-001 | scored | 22.64 | 9 | 7176 | 942 | 2048 | [result](tasks/context-handoff/run-001/result.json), [trace](tasks/context-handoff/run-001/trace.jsonl) |
| custom-stream-provider/run-001 | scored | 23.21 | 8 | 9495 | 1013 | 4992 | [result](tasks/custom-stream-provider/run-001/result.json), [trace](tasks/custom-stream-provider/run-001/trace.jsonl) |
| extension-reload/run-001 | environment_error | 77.45 | 6 | N/A | N/A | N/A | [result](tasks/extension-reload/run-001/result.json), [trace](tasks/extension-reload/run-001/trace.jsonl) |

### add-model/run-001

Final response:

> The probe confirms the model registry is loaded correctly with both entries:
> 
> | # | `name` | `display_name` | `reasoning` | `input` | `context_window` | `max_tokens` | `cost` |
> |---|--------|----------------|-------------|---------|------------------|--------------|--------|
> | 1 | `existing` | *(not set)* | *(not set)* | *(not set)* | 8192 | *(not set)* | *(not set)* |
> | 2 | `fixture-chat` | **Fixture Chat** | ✅ `true` | `text` | 32768 | 4096 | 0 |
> 
> **Confirmation details:**
> 
> - ✅ **Existing entry preserved** – `existing` with `context_window: 8192` is intact.
> - ✅ **New entry added** – `fixture-chat` appears with all specified fields:
>   - Displayed as **"Fixture Chat"**
>   - Supports **reasoning** (`true`) and **text input**
>   - Context window: **32768**
>   - Max output tokens: **4096**
>   - Cost: **0**
> - ✅ **No configuration errors** – The registry loaded cleanly with no issues.
> 
> The addition is fully confirmed.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| registry | 0.000 | registry | {"items":[{"context_window":8192,"name":"existing"},{"context_window":32768,"cost":0,"display_name":"Fixture Chat","input":"text","max_tokens":4096,"name":"fixture-chat","reasoning":true}]} |
| verified | 1.000 | verified | 2 |

Tool execution sequence (full results in evidence):

1. `eval_list` {} — 0 ms
2. `eval_read` {"path":"config/models.json"} — 0 ms
3. `eval_read` {"path":"docs/models.md"} — 0 ms
4. `eval_write` {"content":"{\"items\": [{\"name\": \"existing\", \"context_window\": 8192}, {\"name\": \"fixture-chat\", \"display_name\": \"Fixture Chat\", \"reasoning\": true, \"input\": \"text\", \"context_window\": 32768, \"max_tokens\": 4096, \"cost\": 0}]}","path":"config/models.json"} — 0 ms
5. `eval_reload` {} — 0 ms
6. `eval_probe` {"input":"","target":"models"} — 0 ms
7. `eval_probe` {"input":"","target":"models"} — 0 ms

### audit-inconclusive/run-001

Final response:

> **Summary:** The documentation claims `format_name` preserves capitalization, but the implementation delegates to an external `normalize` function whose source is unavailable. Without access to that implementation, the claim cannot be verified, resulting in an inconclusive verdict.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| verdict | 1.000 | verdict | 1 |
| read_documentation | 1.000 | read documentation | 1 |
| read_implementation | 1.000 | read implementation | 1 |
| evidence | 1.000 | One explanation cites src/names.py and format_name. This is a mechanical evidence check, not a semantic proof of explanation quality. | ["docs/names.md claims \"The public format_name function preserves capitalization.\" In src/names.py, format_name is defined as `def format_name(name): return normalize(name)`, delegating entirely to `normalize` imported from `external_formatter`. The source file itself notes: \"The external formatter implementation is not included.\" Because the behavior of `normalize` is unknown — its implementation is not available in the workspace — there is no evidence to confirm or refute whether capitalization is preserved. The claim cannot be verified against the implementation."] |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/names.md"} — 0 ms
2. `eval_read` {"path":"src/names.py"} — 0 ms
3. `eval_search` {"query":"normalize"} — 0 ms
4. `eval_search` {"query":"external_formatter"} — 0 ms
5. `eval_submit` {"explanation":"docs/names.md claims \"The public format_name function preserves capitalization.\" In src/names.py, format_name is defined as `def format_name(name): return normalize(name)`, delegating entirely to `normalize` imported from `external_formatter`. The source file itself notes: \"The external formatter implementation is not included.\" Because the behavior of `normalize` is unknown — its implementation is not available in the workspace — there is no evidence to confirm or refute whether capitalization is preserved. The claim cannot be verified against the implementation.","verdict":"inconclusive"} — 0 ms

### audit-match/run-001

Final response:

> **Summary:** The documentation claim that format_name trims whitespace and lowercases names matches the implementation. The function delegates to normalize(), which applies strip().lower() - exactly as documented.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| verdict | 1.000 | verdict | 1 |
| read_documentation | 1.000 | read documentation | 1 |
| read_implementation | 1.000 | read implementation | 1 |
| evidence | 1.000 | One explanation cites src/names.py and format_name. This is a mechanical evidence check, not a semantic proof of explanation quality. | ["docs/names.md claims: \"The public format_name function trims whitespace and lowercases names.\" In src/names.py, format_name(name) delegates to normalize(name), which returns value.strip().lower(). strip() trims leading/trailing whitespace and lower() lowercases the string. The function is public (no underscore prefix). All explicit user-visible claims in the documentation are accurately reflected by the implementation."] |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/names.md"} — 0 ms
2. `eval_read` {"path":"src/names.py"} — 0 ms
3. `eval_submit` {"explanation":"docs/names.md claims: \"The public format_name function trims whitespace and lowercases names.\" In src/names.py, format_name(name) delegates to normalize(name), which returns value.strip().lower(). strip() trims leading/trailing whitespace and lower() lowercases the string. The function is public (no underscore prefix). All explicit user-visible claims in the documentation are accurately reflected by the implementation.","verdict":"match"} — 0 ms

### audit-mismatch/run-001

Final response:

> **Summary:** The documentation in `docs/names.md` claims `format_name` "trims whitespace and preserves capitalization." However, the implementation in `src/names.py` shows `format_name` calls `normalize()`, which applies `.strip().lower()`. While whitespace trimming is correct, `.lower()` destroys original capitalization instead of preserving it. Verdict: **mismatch**.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| verdict | 1.000 | verdict | 1 |
| read_documentation | 1.000 | read documentation | 1 |
| read_implementation | 1.000 | read implementation | 1 |
| evidence | 1.000 | One explanation cites src/names.py and format_name. This is a mechanical evidence check, not a semantic proof of explanation quality. | ["docs/names.md claims that format_name \"trims whitespace and preserves capitalization.\" In src/names.py, format_name delegates to normalize(value), which returns value.strip().lower(). The .strip() call correctly trims whitespace, matching the first part of the claim. However, the .lower() call converts all characters to lowercase, which contradicts the claim that capitalization is preserved. The actual behavior is that format_name lowercases the input rather than preserving its original capitalization."] |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/names.md"} — 0 ms
2. `eval_read` {"path":"src/names.py"} — 0 ms
3. `eval_submit` {"explanation":"docs/names.md claims that format_name \"trims whitespace and preserves capitalization.\" In src/names.py, format_name delegates to normalize(value), which returns value.strip().lower(). The .strip() call correctly trims whitespace, matching the first part of the claim. However, the .lower() call converts all characters to lowercase, which contradicts the claim that capitalization is preserved. The actual behavior is that format_name lowercases the input rather than preserving its original capitalization.","verdict":"mismatch"} — 0 ms

### batch-task/run-001

Final response:

> All done. Here's a summary of what was accomplished:
> 
> - **Record A** (`records/a.json`): amount = 12
> - **Record B** (`records/b.json`): amount = 30
> - **Sum**: 12 + 30 = **42**
> - **Count**: 2 records
> 
> **`summary.json`** was written and verified with the contents:
> ```json
> {"total": 42, "count": 2}
> ```
> 
> The source files (`records/a.json` and `records/b.json`) remain unchanged.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| aggregate | 1.000 | aggregate | {"count":2,"total":42} |
| source_a_preserved | 1.000 | source a preserved | "{\"amount\":12}" |
| source_b_preserved | 1.000 | source b preserved | "{\"amount\":30}" |
| verification | 1.000 | verification | 1 |

Tool execution sequence (full results in evidence):

1. `eval_list` {} — 0 ms
2. `eval_read` {"path":"records/a.json"} — 0 ms
3. `eval_read` {"path":"records/b.json"} — 0 ms
4. `eval_write` {"content":"{\"total\":42,\"count\":2}","path":"summary.json"} — 0 ms
5. `eval_read` {"path":"summary.json"} — 0 ms
6. `eval_read` {"path":"records/a.json"} — 0 ms
7. `eval_read` {"path":"records/b.json"} — 0 ms

### configure-provider/run-001

Final response:

> Here is the probe response from the loaded provider runtime:
> 
> | Field | Value |
> |---|---|
> | **text** | `pong` |
> | **model** | `acme-chat` |
> | **protocol** | `chat_completions` |
> | **valid_request_received** | `true` |
> | **input_tokens** | 4 |
> | **output_tokens** | 3 |
> 
> The provider runtime is healthy — it received the `ping` request and responded with `pong`, confirming the Acme provider is correctly configured and operational.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| runtime_request | 1.000 | runtime request | {"input_tokens":4,"model":"acme-chat","output_tokens":3,"protocol":"chat_completions","text":"pong","valid_request_received":true} |
| verified | 1.000 | verified | 2 |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/api.md"} — 0 ms
2. `eval_list` {} — 0 ms
3. `eval_write` {"content":"{\n  \"endpoint\": \"fixture://acme\",\n  \"protocol\": \"chat_completions\",\n  \"credential_env\": \"ACME_KEY\",\n  \"model\": \"acme-chat\"\n}","path":"config/provider.json"} — 0 ms
4. `eval_reload` {} — 0 ms
5. `eval_probe` {"input":"ping","target":"provider"} — 0 ms
6. `eval_probe` {"input":"ping","target":"provider"} — 0 ms

### context-handoff/run-001

Final response:

> ALPHA-17:south

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| continuity | 1.000 | continuity | "ALPHA-17:south" |
| updated_state | 1.000 | updated state | {"code":"ALPHA-17","destination":"south"} |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"notes.json"} — 0 ms; error: not_found
2. `eval_write` {"content":"{\n  \"code\": \"ALPHA-17\",\n  \"destination\": \"north\"\n}","path":"notes.json"} — 0 ms
3. `eval_read` {"path":"notes.json"} — 0 ms
4. `eval_write` {"content":"{\n  \"code\": \"ALPHA-17\",\n  \"destination\": \"south\"\n}","path":"notes.json"} — 0 ms
5. `eval_read` {"path":"notes.json"} — 0 ms
6. `eval_read` {"path":"notes.json"} — 0 ms

### custom-stream-provider/run-001

Final response:

> The stream runtime probe succeeded. Here's the response:
> 
> | Field | Value |
> |---|---|
> | **text** | `pong` |
> | **model** | `acme-chat` |
> | **protocol** | `event_stream` |
> | **valid_request_received** | `true` |
> | **input_tokens** | `4` |
> | **output_tokens** | `3` |
> 
> The runtime correctly received the `ping` input, processed it through the `event_stream` protocol using the `acme-chat` model, and returned `pong` — confirming everything is working as expected.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| runtime_request | 1.000 | runtime request | {"input_tokens":4,"model":"acme-chat","output_tokens":3,"protocol":"event_stream","text":"pong","valid_request_received":true} |
| verified | 1.000 | verified | 2 |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/api.md"} — 0 ms
2. `eval_list` {} — 0 ms
3. `eval_write` {"content":"{\n  \"endpoint\": \"fixture://acme\",\n  \"protocol\": \"event_stream\",\n  \"credential_env\": \"ACME_KEY\",\n  \"model\": \"acme-chat\",\n  \"text_field\": \"delta.text\",\n  \"done_event\": \"complete\"\n}","path":"config/stream.json"} — 0 ms
4. `eval_reload` {} — 0 ms
5. `eval_probe` {"input":"ping","target":"stream"} — 0 ms
6. `eval_probe` {"input":"ping","target":"stream"} — 0 ms

### extension-reload/run-001

Final response:

> 

Diagnostics:

- environment / case_timeout_or_cancelled: context canceled
- environment / cancelled: 请求已取消。

| Criterion | Score | Basis | Observed |
|---|---:|---|---|

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/extensions.md"} — 0 ms
2. `eval_write` {"content":"{\"template\":\"Hello, {name}!\"}","path":"config/hello.json"} — 0 ms
3. `eval_reload` {} — 0 ms
4. `eval_probe` {"input":"Bob","target":"hello"} — 0 ms
