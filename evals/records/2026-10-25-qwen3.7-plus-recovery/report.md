# Lumi Agent evaluation report

Model: `qwen3.7-plus` (aliyun_bailian). Planned: 4; recorded: 4. Repetitions: 2.

Protocol: `4cb5d223fe7c702968b6888e8cc5e118d77a088c7d6445cbfb9904a3cd879a61`

Scores describe each criterion on [0,1]; there is no global acceptance threshold. Agent errors retain observable scores. Environment failures are unscored and excluded from score means. Incomplete cohorts cannot support a complete-run claim. N/A means unavailable, never zero. One repetition cannot establish stability; standard deviation is population SD and is N/A for fewer than two observations. Cost is N/A because this harness has no frozen pricing source.

## Environment / evidence gaps

- custom-stream-provider/run-001: environment_error
- extension-reload/run-001: environment_error

## Repetition statistics

| Case | Recorded scores / planned | Criterion | Mean | Min | Max | SD | Variation |
|---|---:|---|---:|---:|---:|---:|---|
| custom-stream-provider | 1/2 | runtime_request | 1.000 | 1.000 | 1.000 | N/A | false |
| custom-stream-provider | 1/2 | verified | 1.000 | 1.000 | 1.000 | N/A | false |
| extension-reload | 1/2 | greeting | 1.000 | 1.000 | 1.000 | N/A | false |
| extension-reload | 1/2 | invocation | 1.000 | 1.000 | 1.000 | N/A | false |
| extension-reload | 1/2 | loaded_tool | 1.000 | 1.000 | 1.000 | N/A | false |

## Individual executions

| Task | Status | Duration (s) | Requests | Input tokens | Output tokens | Cached input | Evidence |
|---|---|---:|---:|---:|---:|---:|---|
| custom-stream-provider/run-001 | environment_error | 300.04 | 3 | N/A | N/A | N/A | [result](tasks/custom-stream-provider/run-001/result.json), [trace](tasks/custom-stream-provider/run-001/trace.jsonl) |
| extension-reload/run-001 | environment_error | 19.04 | 1 | N/A | N/A | N/A | [result](tasks/extension-reload/run-001/result.json), [trace](tasks/extension-reload/run-001/trace.jsonl) |
| extension-reload/run-002 | scored | 24.35 | 8 | 8205 | 774 | 3712 | [result](tasks/extension-reload/run-002/result.json), [trace](tasks/extension-reload/run-002/trace.jsonl) |
| custom-stream-provider/run-002 | scored | 29.89 | 8 | 10097 | 1205 | 4992 | [result](tasks/custom-stream-provider/run-002/result.json), [trace](tasks/custom-stream-provider/run-002/trace.jsonl) |

### custom-stream-provider/run-001

Final response:

> 

Diagnostics:

- environment / case_timeout_or_cancelled: context deadline exceeded
- environment / timeout: Provider 请求超时。

| Criterion | Score | Basis | Observed |
|---|---:|---|---|

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/api.md"} — 0 ms
2. `eval_list` {} — 0 ms
3. `eval_search` {"query":"config"} — 0 ms

### extension-reload/run-001

Final response:

> 

Diagnostics:

- environment / llm_network_error: 无法连接 Provider。

| Criterion | Score | Basis | Observed |
|---|---:|---|---|

### extension-reload/run-002

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

### custom-stream-provider/run-002

Final response:

> Here is the response from probing the stream runtime with `"ping"`:
> 
> | Field | Value |
> |---|---|
> | **Model** | `acme-chat` |
> | **Protocol** | `event_stream` |
> | **Text** | `pong` |
> | **Input Tokens** | 4 |
> | **Output Tokens** | 3 |
> | **Valid Request Received** | `true` |
> 
> The stream runtime is functioning correctly — it received the simulated `"ping"` request, processed it through the `event_stream` protocol using the `acme-chat` model, and returned `"pong"` as the assembled text output.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| runtime_request | 1.000 | runtime request | {"input_tokens":4,"model":"acme-chat","output_tokens":3,"protocol":"event_stream","text":"pong","valid_request_received":true} |
| verified | 1.000 | verified | 2 |

Tool execution sequence (full results in evidence):

1. `eval_read` {"path":"docs/api.md"} — 0 ms
2. `eval_list` {} — 0 ms
3. `eval_search` {"query":"config/"} — 0 ms
4. `eval_write` {"content":"{\n  \"endpoint\": \"fixture://acme\",\n  \"protocol\": \"event_stream\",\n  \"credential_env\": \"ACME_KEY\",\n  \"model\": \"acme-chat\",\n  \"text_field\": \"delta.text\",\n  \"done_event\": \"complete\"\n}","path":"config/stream.json"} — 0 ms
5. `eval_reload` {} — 0 ms
6. `eval_probe` {"input":"ping","target":"stream"} — 0 ms
7. `eval_probe` {"input":"ping","target":"stream"} — 0 ms
