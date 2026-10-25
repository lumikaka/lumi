# Lumi Agent evaluation report

Model: `qwen3.7-plus` (aliyun_bailian). Planned: 28; recorded: 28. Repetitions: 2.

Protocol: `20e6eebd46c54755341de6ab0039655a7722c41273eb2d165b18fedbc9470928`

Scores describe each criterion on [0,1]; there is no global acceptance threshold. Agent errors retain observable scores. Environment failures are unscored and excluded from score means. Incomplete cohorts cannot support a complete-run claim. N/A means unavailable, never zero. One repetition cannot establish stability; standard deviation is population SD and is N/A for fewer than two observations. Cost is N/A because this harness has no frozen pricing source.

## Environment / evidence gaps

- custom-stream-provider/run-002: environment_error
- extension-reload/run-002: environment_error

## Repetition statistics

| Case | Recorded scores / planned | Criterion | Mean | Min | Max | SD | Variation |
|---|---:|---|---:|---:|---:|---:|---|
| add-model | 2/2 | registry | 1.000 | 1.000 | 1.000 | 0.000 | false |
| add-model | 2/2 | verified | 1.000 | 1.000 | 1.000 | 0.000 | false |
| audit-inconclusive | 2/2 | evidence | 1.000 | 1.000 | 1.000 | 0.000 | false |
| audit-inconclusive | 2/2 | read_documentation | 1.000 | 1.000 | 1.000 | 0.000 | false |
| audit-inconclusive | 2/2 | read_implementation | 1.000 | 1.000 | 1.000 | 0.000 | false |
| audit-inconclusive | 2/2 | verdict | 1.000 | 1.000 | 1.000 | 0.000 | false |
| audit-match | 2/2 | evidence | 1.000 | 1.000 | 1.000 | 0.000 | false |
| audit-match | 2/2 | read_documentation | 1.000 | 1.000 | 1.000 | 0.000 | false |
| audit-match | 2/2 | read_implementation | 1.000 | 1.000 | 1.000 | 0.000 | false |
| audit-match | 2/2 | verdict | 1.000 | 1.000 | 1.000 | 0.000 | false |
| audit-mismatch | 2/2 | evidence | 1.000 | 1.000 | 1.000 | 0.000 | false |
| audit-mismatch | 2/2 | read_documentation | 1.000 | 1.000 | 1.000 | 0.000 | false |
| audit-mismatch | 2/2 | read_implementation | 1.000 | 1.000 | 1.000 | 0.000 | false |
| audit-mismatch | 2/2 | verdict | 1.000 | 1.000 | 1.000 | 0.000 | false |
| batch-task | 2/2 | aggregate | 1.000 | 1.000 | 1.000 | 0.000 | false |
| batch-task | 2/2 | source_a_preserved | 1.000 | 1.000 | 1.000 | 0.000 | false |
| batch-task | 2/2 | source_b_preserved | 1.000 | 1.000 | 1.000 | 0.000 | false |
| batch-task | 2/2 | verification | 1.000 | 1.000 | 1.000 | 0.000 | false |
| configure-provider | 2/2 | runtime_request | 1.000 | 1.000 | 1.000 | 0.000 | false |
| configure-provider | 2/2 | verified | 1.000 | 1.000 | 1.000 | 0.000 | false |
| context-handoff | 2/2 | continuity | 1.000 | 1.000 | 1.000 | 0.000 | false |
| context-handoff | 2/2 | updated_state | 1.000 | 1.000 | 1.000 | 0.000 | false |
| custom-stream-provider | 1/2 | runtime_request | 1.000 | 1.000 | 1.000 | N/A | false |
| custom-stream-provider | 1/2 | verified | 1.000 | 1.000 | 1.000 | N/A | false |
| extension-reload | 1/2 | greeting | 1.000 | 1.000 | 1.000 | N/A | false |
| extension-reload | 1/2 | invocation | 1.000 | 1.000 | 1.000 | N/A | false |
| extension-reload | 1/2 | loaded_tool | 1.000 | 1.000 | 1.000 | N/A | false |
| missing-resource | 2/2 | honest_failure | 1.000 | 1.000 | 1.000 | 0.000 | false |
| missing-resource | 2/2 | single_attempt | 1.000 | 1.000 | 1.000 | 0.000 | false |
| repair-config | 2/2 | reload_diagnostic | 1.000 | 1.000 | 1.000 | 0.000 | true |
| repair-config | 2/2 | repaired | 0.500 | 0.000 | 1.000 | 0.500 | true |
| repair-config | 2/2 | valid_configuration | 0.500 | 0.000 | 1.000 | 0.500 | true |
| smoke | 2/2 | answer | 1.000 | 1.000 | 1.000 | 0.000 | false |
| status-footer | 2/2 | bar_correctness | 1.000 | 1.000 | 1.000 | 0.000 | false |
| status-footer | 2/2 | layout_preservation | 1.000 | 1.000 | 1.000 | 0.000 | false |
| transient-tool | 2/2 | recovered_answer | 1.000 | 1.000 | 1.000 | 0.000 | false |
| transient-tool | 2/2 | retried | 1.000 | 1.000 | 1.000 | 0.000 | false |

## Individual executions

| Task | Status | Duration (s) | Requests | Input tokens | Output tokens | Cached input | Evidence |
|---|---|---:|---:|---:|---:|---:|---|
| add-model/run-001 | scored | 37.63 | 9 | 11737 | 1713 | 6144 | [result](tasks/add-model/run-001/result.json), [trace](tasks/add-model/run-001/trace.jsonl) |
| audit-inconclusive/run-001 | scored | 20.65 | 4 | 3648 | 1003 | 1024 | [result](tasks/audit-inconclusive/run-001/result.json), [trace](tasks/audit-inconclusive/run-001/trace.jsonl) |
| audit-match/run-001 | scored | 14.69 | 3 | 2524 | 731 | 1024 | [result](tasks/audit-match/run-001/result.json), [trace](tasks/audit-match/run-001/trace.jsonl) |
| audit-mismatch/run-001 | scored | 13.40 | 3 | 2544 | 623 | 512 | [result](tasks/audit-mismatch/run-001/result.json), [trace](tasks/audit-mismatch/run-001/trace.jsonl) |
| batch-task/run-001 | scored | 12.20 | 5 | 3474 | 502 | 1536 | [result](tasks/batch-task/run-001/result.json), [trace](tasks/batch-task/run-001/trace.jsonl) |
| configure-provider/run-001 | scored | 26.99 | 8 | 10187 | 1177 | 6144 | [result](tasks/configure-provider/run-001/result.json), [trace](tasks/configure-provider/run-001/trace.jsonl) |
| context-handoff/run-001 | scored | 17.62 | 8 | 5899 | 646 | 1536 | [result](tasks/context-handoff/run-001/result.json), [trace](tasks/context-handoff/run-001/trace.jsonl) |
| custom-stream-provider/run-001 | scored | 24.94 | 8 | 8965 | 1076 | 3712 | [result](tasks/custom-stream-provider/run-001/result.json), [trace](tasks/custom-stream-provider/run-001/trace.jsonl) |
| extension-reload/run-001 | scored | 19.22 | 8 | 8001 | 712 | 3584 | [result](tasks/extension-reload/run-001/result.json), [trace](tasks/extension-reload/run-001/trace.jsonl) |
| missing-resource/run-001 | scored | 4.00 | 2 | 879 | 128 | 0 | [result](tasks/missing-resource/run-001/result.json), [trace](tasks/missing-resource/run-001/trace.jsonl) |
| repair-config/run-001 | agent_error | 7.71 | 3 | 2126 | 297 | 1024 | [result](tasks/repair-config/run-001/result.json), [trace](tasks/repair-config/run-001/trace.jsonl) |
| smoke/run-001 | scored | 2.65 | 1 | 113 | 94 | 0 | [result](tasks/smoke/run-001/result.json), [trace](tasks/smoke/run-001/trace.jsonl) |
| status-footer/run-001 | scored | 37.05 | 8 | 9773 | 1796 | 4864 | [result](tasks/status-footer/run-001/result.json), [trace](tasks/status-footer/run-001/trace.jsonl) |
| transient-tool/run-001 | scored | 5.47 | 3 | 1406 | 175 | 0 | [result](tasks/transient-tool/run-001/result.json), [trace](tasks/transient-tool/run-001/trace.jsonl) |
| transient-tool/run-002 | scored | 5.14 | 3 | 1406 | 156 | 0 | [result](tasks/transient-tool/run-002/result.json), [trace](tasks/transient-tool/run-002/trace.jsonl) |
| status-footer/run-002 | scored | 48.78 | 9 | 11239 | 2407 | 5504 | [result](tasks/status-footer/run-002/result.json), [trace](tasks/status-footer/run-002/trace.jsonl) |
| smoke/run-002 | scored | 2.52 | 1 | 113 | 91 | 0 | [result](tasks/smoke/run-002/result.json), [trace](tasks/smoke/run-002/trace.jsonl) |
| repair-config/run-002 | scored | 13.48 | 6 | 5853 | 480 | 2048 | [result](tasks/repair-config/run-002/result.json), [trace](tasks/repair-config/run-002/trace.jsonl) |
| missing-resource/run-002 | scored | 4.85 | 2 | 879 | 197 | 0 | [result](tasks/missing-resource/run-002/result.json), [trace](tasks/missing-resource/run-002/trace.jsonl) |
| extension-reload/run-002 | environment_error | 300.05 | 5 | N/A | N/A | N/A | [result](tasks/extension-reload/run-002/result.json), [trace](tasks/extension-reload/run-002/trace.jsonl) |
| custom-stream-provider/run-002 | environment_error | 45.87 | 1 | N/A | N/A | N/A | [result](tasks/custom-stream-provider/run-002/result.json), [trace](tasks/custom-stream-provider/run-002/trace.jsonl) |
| context-handoff/run-002 | scored | 18.78 | 9 | 6968 | 678 | 2048 | [result](tasks/context-handoff/run-002/result.json), [trace](tasks/context-handoff/run-002/trace.jsonl) |
| configure-provider/run-002 | scored | 20.77 | 7 | 7816 | 857 | 3712 | [result](tasks/configure-provider/run-002/result.json), [trace](tasks/configure-provider/run-002/trace.jsonl) |
| batch-task/run-002 | scored | 13.04 | 5 | 3474 | 536 | 1024 | [result](tasks/batch-task/run-002/result.json), [trace](tasks/batch-task/run-002/trace.jsonl) |
| audit-mismatch/run-002 | scored | 13.08 | 3 | 2570 | 491 | 1024 | [result](tasks/audit-mismatch/run-002/result.json), [trace](tasks/audit-mismatch/run-002/trace.jsonl) |
| audit-match/run-002 | scored | 11.14 | 3 | 2514 | 445 | 1024 | [result](tasks/audit-match/run-002/result.json), [trace](tasks/audit-match/run-002/trace.jsonl) |
| audit-inconclusive/run-002 | scored | 30.52 | 5 | 4598 | 1324 | 2048 | [result](tasks/audit-inconclusive/run-002/result.json), [trace](tasks/audit-inconclusive/run-002/trace.jsonl) |
| add-model/run-002 | scored | 31.68 | 9 | 11749 | 1408 | 5632 | [result](tasks/add-model/run-002/result.json), [trace](tasks/add-model/run-002/trace.jsonl) |

### add-model/run-001

Final response:

> The probe confirms the model registry is loaded correctly with both entries:
> 
> ### Registry Contents
> 
> | # | `name` | `display_name` | `reasoning` | `input` | `context_window` | `max_tokens` | `cost` |
> |---|--------|----------------|-------------|---------|-------------------|--------------|--------|
> | 1 | `existing` | *(none)* | *(none)* | *(none)* | 8192 | *(none)* | *(none)* |
> | 2 | `fixture-chat` | **Fixture Chat** | **true** | **["text"]** | **32768** | **4096** | **0** |
> 
> ### Confirmation
> 
> - ✅ **Existing entry preserved** — `existing` with `context_window: 8192` is untouched.
> - ✅ **New entry added** — `fixture-chat` with all required fields:
>   - `display_name`: `"Fixture Chat"`
>   - `reasoning`: `true`
>   - `input`: `["text"]`
>   - `context_window`: `32768`
>   - `max_tokens`: `4096`
>   - `cost`: `0`
> 
> The addition is fully confirmed in the active runtime registry.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| registry | 1.000 | registry | {"items":[{"context_window":8192,"name":"existing"},{"context_window":32768,"cost":0,"display_name":"Fixture Chat","input":["text"],"max_tokens":4096,"name":"fixture-chat","reasoning":true}]} |
| verified | 1.000 | verified | 2 |

Tool execution sequence (full results in evidence):

1. `eval_list` {} — 0 ms
2. `eval_read` {"path":"config/models.json"} — 0 ms
3. `eval_read` {"path":"docs/models.md"} — 0 ms
4. `eval_write` {"content":"{\n  \"items\": [\n    {\"name\": \"existing\", \"context_window\": 8192},\n    {\n      \"name\": \"fixture-chat\",\n      \"display_name\": \"Fixture Chat\",\n      \"reasoning\": true,\n      \"input\": [\"text\"],\n      \"context_window\": 32768,\n      \"max_tokens\": 4096,\n      \"cost\": 0\n    }\n  ]\n}","path":"config/models.json"} — 0 ms
5. `eval_reload` {} — 0 ms
6. `eval_probe` {"input":"","target":"models"} — 0 ms
7. `eval_probe` {"input":"","target":"models"} — 0 ms
8. `eval_read` {"path":"config/models.json"} — 0 ms

### audit-inconclusive/run-001

Final response:

> **Summary:** The documentation claims format_name preserves capitalization, but the implementation delegates entirely to an external `normalize` function from `external_formatter` that is not included in the workspace. Without access to that implementation, the claim cannot be verified, resulting in an inconclusive audit.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| verdict | 1.000 | verdict | 1 |
| read_documentation | 1.000 | read documentation | 1 |
| read_implementation | 1.000 | read implementation | 1 |
| evidence | 1.000 | One explanation cites src/names.py and format_name, and submission is the final tool action. This is a mechanical evidence check, not a semantic proof of explanation quality. | ["The claim in docs/names.md states: \"The public format_name function preserves capitalization.\" In src/names.py, format_name is defined as `def format_name(name): return normalize(name)`, delegating entirely to `normalize` imported from `external_formatter`. The file explicitly notes: \"The external formatter implementation is not included.\" Since the actual behavior of `format_name` depends wholly on the unavailable `external_formatter.normalize` implementation, we cannot verify whether capitalization is preserved. Evidence is unavailable, so the audit is inconclusive."] |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/names.md"} — 0 ms
2. `eval_read` {"path":"src/names.py"} — 0 ms
3. `eval_search` {"query":"external_formatter"} — 0 ms
4. `eval_search` {"query":"normalize"} — 0 ms
5. `eval_submit` {"explanation":"The claim in docs/names.md states: \"The public format_name function preserves capitalization.\" In src/names.py, format_name is defined as `def format_name(name): return normalize(name)`, delegating entirely to `normalize` imported from `external_formatter`. The file explicitly notes: \"The external formatter implementation is not included.\" Since the actual behavior of `format_name` depends wholly on the unavailable `external_formatter.normalize` implementation, we cannot verify whether capitalization is preserved. Evidence is unavailable, so the audit is inconclusive.","verdict":"inconclusive"} — 0 ms

### audit-match/run-001

Final response:

> **Summary:** The documentation in `docs/names.md` states that the public `format_name` function trims whitespace and lowercases names. The implementation in `src/names.py` confirms this: `format_name` calls `normalize`, which applies `.strip().lower()`. The claim is a **match** — the implementation behaves exactly as documented.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| verdict | 1.000 | verdict | 1 |
| read_documentation | 1.000 | read documentation | 1 |
| read_implementation | 1.000 | read implementation | 1 |
| evidence | 1.000 | One explanation cites src/names.py and format_name, and submission is the final tool action. This is a mechanical evidence check, not a semantic proof of explanation quality. | ["docs/names.md claims: \"The public format_name function trims whitespace and lowercases names.\" In src/names.py, format_name(name) delegates to normalize(value), which returns value.strip().lower(). strip() trims leading/trailing whitespace and lower() lowercases the string. All three parts of the claim (public function, trims whitespace, lowercases) are confirmed by the implementation."] |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/names.md"} — 0 ms
2. `eval_read` {"path":"src/names.py"} — 0 ms
3. `eval_submit` {"explanation":"docs/names.md claims: \"The public format_name function trims whitespace and lowercases names.\" In src/names.py, format_name(name) delegates to normalize(value), which returns value.strip().lower(). strip() trims leading/trailing whitespace and lower() lowercases the string. All three parts of the claim (public function, trims whitespace, lowercases) are confirmed by the implementation.","verdict":"match"} — 0 ms

### audit-mismatch/run-001

Final response:

> **Summary:** The docs claim `format_name` "trims whitespace and preserves capitalization," but the implementation actually lowercases the input (via `normalize` → `.strip().lower()`). This is a **mismatch** — the capitalization-preservation claim is false.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| verdict | 1.000 | verdict | 1 |
| read_documentation | 1.000 | read documentation | 1 |
| read_implementation | 1.000 | read implementation | 1 |
| evidence | 1.000 | One explanation cites src/names.py and format_name, and submission is the final tool action. This is a mechanical evidence check, not a semantic proof of explanation quality. | ["The documentation in docs/names.md claims that format_name \"trims whitespace and preserves capitalization.\" However, the implementation in src/names.py shows that format_name calls normalize(), which applies both .strip() and .lower() to the input. While the function does trim whitespace as claimed, it converts all characters to lowercase rather than preserving the original capitalization. The actual behavior is that format_name trims whitespace AND lowercases the entire string."] |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/names.md"} — 0 ms
2. `eval_read` {"path":"src/names.py"} — 0 ms
3. `eval_submit` {"explanation":"The documentation in docs/names.md claims that format_name \"trims whitespace and preserves capitalization.\" However, the implementation in src/names.py shows that format_name calls normalize(), which applies both .strip() and .lower() to the input. While the function does trim whitespace as claimed, it converts all characters to lowercase rather than preserving the original capitalization. The actual behavior is that format_name trims whitespace AND lowercases the entire string.","verdict":"mismatch"} — 0 ms

### batch-task/run-001

Final response:

> The task is complete. Here's a summary:
> 
> - **records/a.json**: `{"amount": 12}`
> - **records/b.json**: `{"amount": 30}`
> - **Sum of amounts**: 12 + 30 = **42**
> - **Count of records**: **2**
> 
> **summary.json** was written with `{"total": 42, "count": 2}` and verified by reading it back. The source files (`records/a.json` and `records/b.json`) were not modified.

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
4. `eval_write` {"content":"{\"total\": 42, \"count\": 2}","path":"summary.json"} — 0 ms
5. `eval_read` {"path":"summary.json"} — 0 ms

### configure-provider/run-001

Final response:

> The probe was successful. Here's the response:
> 
> | Field | Value |
> |---|---|
> | **Status** | ✅ Success |
> | **Text** | `pong` |
> | **Model** | `acme-chat` |
> | **Protocol** | `chat_completions` |
> | **Valid Request Received** | `true` |
> | **Input Tokens** | 4 |
> | **Output Tokens** | 3 |
> 
> The Acme provider runtime is loaded and responding correctly — it received the `ping` request and returned `pong` as expected.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| runtime_request | 1.000 | runtime request | {"input_tokens":4,"model":"acme-chat","output_tokens":3,"protocol":"chat_completions","text":"pong","valid_request_received":true} |
| verified | 1.000 | verified | 2 |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/api.md"} — 0 ms
2. `eval_list` {} — 0 ms
3. `eval_search` {"query":"provider.json"} — 0 ms
4. `eval_search` {"query":"credential_env"} — 0 ms
5. `eval_write` {"content":"{\n  \"endpoint\": \"fixture://acme\",\n  \"protocol\": \"chat_completions\",\n  \"credential_env\": \"ACME_KEY\",\n  \"model\": \"acme-chat\"\n}","path":"config/provider.json"} — 0 ms
6. `eval_reload` {} — 0 ms
7. `eval_probe` {"input":"ping","target":"provider"} — 0 ms
8. `eval_probe` {"input":"ping","target":"provider"} — 0 ms

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

### custom-stream-provider/run-001

Final response:

> Here is the response from probing the stream runtime with **"ping"**:
> 
> | Field | Value |
> |---|---|
> | **valid_request_received** | `true` |
> | **model** | `acme-chat` |
> | **protocol** | `event_stream` |
> | **text** | `pong` |
> | **input_tokens** | `4` |
> | **output_tokens** | `3` |
> 
> The stream runtime is functioning correctly. It received the ping request, processed it through the `event_stream` protocol using the `acme-chat` model, and returned the text **"pong"** — assembled from the two delta events (`"po"` + `"ng"`) followed by the `complete` done event.

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

> Hello, Bob!

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| greeting | 1.000 | greeting | "Hello, Bob!" |
| loaded_tool | 1.000 | loaded tool | "Hello, Bob!" |
| invocation | 1.000 | invocation | 2 |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/extensions.md"} — 0 ms
2. `eval_list` {} — 0 ms
3. `eval_write` {"content":"{\"template\":\"Hello, {name}!\"}","path":"config/hello.json"} — 0 ms
4. `eval_reload` {} — 0 ms
5. `eval_probe` {"input":"Bob","target":"hello"} — 0 ms
6. `eval_probe` {"input":"Bob","target":"hello"} — 0 ms

### missing-resource/run-001

Final response:

> UNAVAILABLE

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| honest_failure | 1.000 | honest failure | "UNAVAILABLE" |
| single_attempt | 1.000 | single attempt | 1 |

Tool execution sequence (full results in evidence):

1. `eval_fetch` {"key":"missing"} — 0 ms; error: not_found

### repair-config/run-001

Final response:

> 

Diagnostics:

- agent / agent_turn_budget_exceeded: Agent 已达到本轮安全预算，且未能生成可靠的最终说明。

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| repaired | 0.000 | repaired | "" |
| valid_configuration | 0.000 | valid configuration | "reload_failed" |
| reload_diagnostic | 1.000 | reload diagnostic | 2 |

Tool execution sequence (full results in evidence):

1. `eval_reload` {} — 0 ms
2. `eval_read` {"path":"docs/extensions.md"} — 0 ms
3. `eval_reload` {} — 0 ms
4. `eval_read` {"path":"docs/extensions.md"} — 0 ms

### smoke/run-001

Final response:

> Paris

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| answer | 1.000 | answer | "Paris" |

### status-footer/run-001

Final response:

> All three fixtures verified successfully:
> 
> | Fixture Input | Rounded % | Clamped? | Cells (filled/empty) | Bar Output |
> |---|---|---|---|---|
> | 42.2% | 42% | No | 4 / 6 | `████░░░░░░` |
> | 65% | 65% | No | 7 / 3 | `███████░░░` |
> | 120% | 120% → **100%** | Yes | 10 / 0 | `██████████` |
> 
> **Checks:**
> - ✅ **Ten-cell bar** — all three outputs have exactly 10 cells (█ + ░)
> - ✅ **Rounded percent** — 42.2% rounded to 42%; 65% unchanged; 120% rounded to 120%
> - ✅ **Clamped to 0–100%** — 120% clamped to 100% with a full bar
> - ✅ **Prefix preserved** — `"workspace | model | "` present on all lines
> - ✅ **Suffix preserved** — `" | $1.008"` present on all lines

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| bar_correctness | 1.000 | bar correctness | ["workspace \| model \| ████░░░░░░ 42% \| $1.008","workspace \| model \| ███████░░░ 65% \| $1.008","workspace \| model \| ██████████ 100% \| $1.008"] |
| layout_preservation | 1.000 | Minimum normalized Levenshtein similarity of the surrounding text across all three renders; independent of bar correctness. | ["workspace \| model \| ████░░░░░░ 42% \| $1.008","workspace \| model \| ███████░░░ 65% \| $1.008","workspace \| model \| ██████████ 100% \| $1.008"] |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/footer.md"} — 0 ms
2. `eval_list` {} — 0 ms
3. `eval_read` {"path":"config/footer.json"} — 0 ms
4. `eval_write` {"content":"{\"prefix\": \"workspace | model | \", \"suffix\": \" | $1.008\", \"width\": 10, \"clamp\": true}","path":"config/footer.json"} — 0 ms
5. `eval_reload` {} — 0 ms
6. `eval_probe` {"input":"","target":"footer"} — 0 ms
7. `eval_probe` {"input":"","target":"footer"} — 0 ms

### transient-tool/run-001

Final response:

> 42

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| recovered_answer | 1.000 | recovered answer | "42" |
| retried | 1.000 | retried | 2 |

Tool execution sequence (full results in evidence):

1. `eval_fetch` {"key":"order"} — 0 ms; error: transient_unavailable
2. `eval_fetch` {"key":"order"} — 0 ms

### transient-tool/run-002

Final response:

> 42

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| recovered_answer | 1.000 | recovered answer | "42" |
| retried | 1.000 | retried | 2 |

Tool execution sequence (full results in evidence):

1. `eval_fetch` {"key":"order"} — 0 ms; error: transient_unavailable
2. `eval_fetch` {"key":"order"} — 0 ms

### status-footer/run-002

Final response:

> All three fixtures verified successfully:
> 
> | # | Input Value | Bar | Percent | Status |
> |---|-------------|-----|---------|--------|
> | 1 | **42.2%** | `████░░░░░░` (4 of 10 cells) | **42%** | ✅ Rounded |
> | 2 | **65%** | `███████░░░` (7 of 10 cells) | **65%** | ✅ Correct |
> | 3 | **120%** | `██████████` (10 of 10 cells) | **100%** | ✅ Clamped |
> 
> **Details:**
> - **Fixture 1 (42.2%)**: 42.2% × 10 = 4.22 → rounds to **4 cells** (`████░░░░░░`), percentage rounds to **42%**.
> - **Fixture 2 (65%)**: 65% × 10 = 6.5 → rounds to **7 cells** (`███████░░░`), percentage is **65%**.
> - **Fixture 3 (120%)**: 120% exceeds 100% → **clamped** to 100%, fills all **10 cells** (`██████████`), percentage shows **100%**.
> 
> All prefix (`workspace | model | `) and suffix (` | $1.008`) text is preserved unchanged. Everything checks out.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| bar_correctness | 1.000 | bar correctness | ["workspace \| model \| ████░░░░░░ 42% \| $1.008","workspace \| model \| ███████░░░ 65% \| $1.008","workspace \| model \| ██████████ 100% \| $1.008"] |
| layout_preservation | 1.000 | Minimum normalized Levenshtein similarity of the surrounding text across all three renders; independent of bar correctness. | ["workspace \| model \| ████░░░░░░ 42% \| $1.008","workspace \| model \| ███████░░░ 65% \| $1.008","workspace \| model \| ██████████ 100% \| $1.008"] |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/footer.md"} — 0 ms
2. `eval_list` {} — 0 ms
3. `eval_read` {"path":"config/footer.json"} — 0 ms
4. `eval_probe` {"input":"","target":"footer"} — 0 ms
5. `eval_write` {"content":"{\"prefix\": \"workspace | model | \", \"suffix\": \" | $1.008\", \"width\": 10, \"clamp\": true}","path":"config/footer.json"} — 0 ms
6. `eval_reload` {} — 0 ms
7. `eval_read` {"path":"config/footer.json"} — 0 ms
8. `eval_probe` {"input":"","target":"footer"} — 0 ms
9. `eval_probe` {"input":"","target":"footer"} — 0 ms

### smoke/run-002

Final response:

> Paris

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| answer | 1.000 | answer | "Paris" |

### repair-config/run-002

Final response:

> Hello, Ada!

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| repaired | 1.000 | repaired | "Hello, Ada!" |
| valid_configuration | 1.000 | valid configuration | "Hello, Ada!" |
| reload_diagnostic | 1.000 | reload diagnostic | 2 |

Tool execution sequence (full results in evidence):

1. `eval_reload` {} — 0 ms
2. `eval_read` {"path":"config/hello.json"} — 0 ms
3. `eval_read` {"path":"docs/extensions.md"} — 0 ms
4. `eval_write` {"content":"{\"template\":\"Hello, {name}!\"}","path":"config/hello.json"} — 0 ms
5. `eval_reload` {} — 0 ms
6. `eval_probe` {"input":"Ada","target":"hello"} — 0 ms

### missing-resource/run-002

Final response:

> UNAVAILABLE

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| honest_failure | 1.000 | honest failure | "UNAVAILABLE" |
| single_attempt | 1.000 | single attempt | 1 |

Tool execution sequence (full results in evidence):

1. `eval_fetch` {"key":"missing"} — 0 ms; error: not_found

### extension-reload/run-002

Final response:

> 

Diagnostics:

- environment / case_timeout_or_cancelled: context deadline exceeded
- environment / timeout: Provider 请求超时。

| Criterion | Score | Basis | Observed |
|---|---:|---|---|

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/extensions.md"} — 0 ms
2. `eval_write` {"content":"{\"template\":\"Hello, {name}!\"}","path":"config/hello.json"} — 0 ms
3. `eval_reload` {} — 0 ms
4. `eval_probe` {"input":"Bob","target":"hello"} — 0 ms

### custom-stream-provider/run-002

Final response:

> 

Diagnostics:

- environment / llm_network_error: 无法连接 Provider。
- environment / llm_network_error: 无法连接 Provider。

| Criterion | Score | Basis | Observed |
|---|---:|---|---|

### context-handoff/run-002

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

### configure-provider/run-002

Final response:

> The probe was successful. Here's the response:
> 
> | Field | Value |
> |---|---|
> | **text** | `"pong"` |
> | **valid_request_received** | `true` |
> | **model** | `acme-chat` |
> | **protocol** | `chat_completions` |
> | **input_tokens** | `4` |
> | **output_tokens** | `3` |
> 
> The provider runtime is loaded and responding correctly — the simulated request was received and returned `"pong"` as expected.

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

### batch-task/run-002

Final response:

> The task is complete. Here's a summary of what was done:
> 
> 1. **Read both records:**
>    - `records/a.json`: `{"amount": 12}`
>    - `records/b.json`: `{"amount": 30}`
> 
> 2. **Computed the summary:**
>    - **Total**: 12 + 30 = **42**
>    - **Count**: **2** records
> 
> 3. **Wrote `summary.json`:**
>    ```json
>    {"total": 42, "count": 2}
>    ```
> 
> 4. **Verified `summary.json`** by reading it back — contents confirmed as `{"total": 42, "count": 2}`. ✓
> 
> The source files (`records/a.json` and `records/b.json`) were not modified.

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
4. `eval_write` {"content":"{\"total\": 42, \"count\": 2}","path":"summary.json"} — 0 ms
5. `eval_read` {"path":"summary.json"} — 0 ms

### audit-mismatch/run-002

Final response:

> **Summary:** The docs claim `format_name` "trims whitespace and preserves capitalization," but the implementation in `src/names.py` calls `normalize()`, which applies `.strip().lower()`. While whitespace trimming is correct, `.lower()` destroys capitalization rather than preserving it — a clear mismatch.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| verdict | 1.000 | verdict | 1 |
| read_documentation | 1.000 | read documentation | 1 |
| read_implementation | 1.000 | read implementation | 1 |
| evidence | 1.000 | One explanation cites src/names.py and format_name, and submission is the final tool action. This is a mechanical evidence check, not a semantic proof of explanation quality. | ["The documentation in docs/names.md claims: \"The public format_name function trims whitespace and preserves capitalization.\" In src/names.py, format_name calls normalize(name), and normalize is defined as `value.strip().lower()`. The `.strip()` call does trim whitespace, matching the first part of the claim. However, `.lower()` converts the entire string to lowercase, which contradicts the claim that capitalization is \"preserved.\" The actual behavior is that all characters are lowercased, not preserved."] |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/names.md"} — 0 ms
2. `eval_read` {"path":"src/names.py"} — 0 ms
3. `eval_submit` {"explanation":"The documentation in docs/names.md claims: \"The public format_name function trims whitespace and preserves capitalization.\" In src/names.py, format_name calls normalize(name), and normalize is defined as `value.strip().lower()`. The `.strip()` call does trim whitespace, matching the first part of the claim. However, `.lower()` converts the entire string to lowercase, which contradicts the claim that capitalization is \"preserved.\" The actual behavior is that all characters are lowercased, not preserved.","verdict":"mismatch"} — 0 ms

### audit-match/run-002

Final response:

> **Summary:** The documentation claim that `format_name` trims whitespace and lowercases names is **matched** by the implementation. The function calls `normalize()`, which applies `.strip().lower()` — exactly as documented.
> 
> Verdict: **match**

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| verdict | 1.000 | verdict | 1 |
| read_documentation | 1.000 | read documentation | 1 |
| read_implementation | 1.000 | read implementation | 1 |
| evidence | 1.000 | One explanation cites src/names.py and format_name, and submission is the final tool action. This is a mechanical evidence check, not a semantic proof of explanation quality. | ["The documentation in docs/names.md claims: \"The public format_name function trims whitespace and lowercases names.\" In src/names.py, `format_name(name)` delegates to `normalize(value)`, which returns `value.strip().lower()`. `strip()` trims leading/trailing whitespace and `lower()` lowercases the string. The implementation exactly matches the documented behavior."] |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/names.md"} — 0 ms
2. `eval_read` {"path":"src/names.py"} — 0 ms
3. `eval_submit` {"explanation":"The documentation in docs/names.md claims: \"The public format_name function trims whitespace and lowercases names.\" In src/names.py, `format_name(name)` delegates to `normalize(value)`, which returns `value.strip().lower()`. `strip()` trims leading/trailing whitespace and `lower()` lowercases the string. The implementation exactly matches the documented behavior.","verdict":"match"} — 0 ms

### audit-inconclusive/run-002

Final response:

> **Summary:** The audit found that docs/names.md claims format_name preserves capitalization, but src/names.py shows format_name delegates to an external `normalize` function from `external_formatter`. Since that external module is not available in the workspace, there's insufficient evidence to verify whether the claim holds, resulting in an inconclusive verdict.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| verdict | 1.000 | verdict | 1 |
| read_documentation | 1.000 | read documentation | 1 |
| read_implementation | 1.000 | read implementation | 1 |
| evidence | 1.000 | One explanation cites src/names.py and format_name, and submission is the final tool action. This is a mechanical evidence check, not a semantic proof of explanation quality. | ["The docs/names.md claim states that \"The public format_name function preserves capitalization.\" In src/names.py, format_name is defined as `return normalize(name)`, delegating entirely to `normalize` imported from `external_formatter`. No implementation of `external_formatter` or `normalize` exists in the workspace (only docs/names.md and src/names.py are present). Because the actual transformation logic is in an unavailable external module, there is no evidence to confirm or refute whether capitalization is preserved."] |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/names.md"} — 0 ms
2. `eval_read` {"path":"src/names.py"} — 0 ms
3. `eval_list` {} — 0 ms
4. `eval_search` {"query":"external_formatter"} — 0 ms
5. `eval_search` {"query":"normalize"} — 0 ms
6. `eval_submit` {"explanation":"The docs/names.md claim states that \"The public format_name function preserves capitalization.\" In src/names.py, format_name is defined as `return normalize(name)`, delegating entirely to `normalize` imported from `external_formatter`. No implementation of `external_formatter` or `normalize` exists in the workspace (only docs/names.md and src/names.py are present). Because the actual transformation logic is in an unavailable external module, there is no evidence to confirm or refute whether capitalization is preserved.","verdict":"inconclusive"} — 0 ms

### add-model/run-002

Final response:

> The probe confirms the model registry is correctly loaded with both entries:
> 
> | # | name | display_name | reasoning | input | context_window | max_tokens | cost |
> |---|------|-------------|-----------|-------|---------------|------------|------|
> | 1 | `existing` | *(none)* | — | — | 8192 | — | — |
> | 2 | `fixture-chat` | **Fixture Chat** | ✅ `true` | `["text"]` | 32768 | 4096 | 0 |
> 
> **Confirmation:**
> - ✅ The pre-existing `existing` entry is preserved exactly as it was.
> - ✅ The new `fixture-chat` entry was added with all specified fields:
>   - **display_name**: `"Fixture Chat"`
>   - **reasoning**: `true` (supports reasoning)
>   - **input**: `["text"]` (supports text input)
>   - **context_window**: `32768`
>   - **max_tokens**: `4096`
>   - **cost**: `0`
> 
> The addition is verified and active.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| registry | 1.000 | registry | {"items":[{"context_window":8192,"name":"existing"},{"context_window":32768,"cost":0,"display_name":"Fixture Chat","input":["text"],"max_tokens":4096,"name":"fixture-chat","reasoning":true}]} |
| verified | 1.000 | verified | 2 |

Tool execution sequence (full results in evidence):

1. `eval_list` {} — 0 ms
2. `eval_read` {"path":"docs/models.md"} — 0 ms
3. `eval_read` {"path":"config/models.json"} — 0 ms
4. `eval_read` {"path":"docs/models.md"} — 0 ms
5. `eval_write` {"content":"{\"items\": [{\"name\": \"existing\", \"context_window\": 8192}, {\"name\": \"fixture-chat\", \"display_name\": \"Fixture Chat\", \"reasoning\": true, \"input\": [\"text\"], \"context_window\": 32768, \"max_tokens\": 4096, \"cost\": 0}]}","path":"config/models.json"} — 0 ms
6. `eval_reload` {} — 0 ms
7. `eval_probe` {"input":"","target":"models"} — 0 ms
8. `eval_probe` {"input":"","target":"models"} — 0 ms
