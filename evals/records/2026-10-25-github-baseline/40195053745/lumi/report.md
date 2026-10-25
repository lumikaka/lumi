# Lumi Agent evaluation report

Model: `qwen3.7-plus` (openai_compatible). Planned: 18; recorded: 18. Repetitions: 3.

Protocol: `32ac27da1040ddedc6fa18835f571983fe4342220e12e603f06b546186009419`

Scores describe each criterion on [0,1]; there is no global acceptance threshold. Agent errors retain observable scores. Environment failures are unscored and excluded from score means. Incomplete cohorts cannot support a complete-run claim. N/A means unavailable, never zero. One repetition cannot establish stability; standard deviation is population SD and is N/A for fewer than two observations. Cost is N/A because this harness has no frozen pricing source.

## Repetition statistics

| Case | Recorded scores / planned | Criterion | Mean | Min | Max | SD | Variation |
|---|---:|---|---:|---:|---:|---:|---|
| pi-add-model-with-docs | 3/3 | upstream_contract | 0.000 | 0.000 | 0.000 | 0.000 | false |
| pi-add-model-without-docs | 3/3 | upstream_contract | 0.000 | 0.000 | 0.000 | 0.000 | false |
| pi-prompt-template-project-with-docs | 3/3 | upstream_contract | 1.000 | 1.000 | 1.000 | 0.000 | false |
| pi-prompt-template-project-without-docs | 3/3 | upstream_contract | 1.000 | 1.000 | 1.000 | 0.000 | false |
| pi-prompt-template-user-with-docs | 3/3 | upstream_contract | 1.000 | 1.000 | 1.000 | 0.000 | false |
| pi-prompt-template-user-without-docs | 3/3 | upstream_contract | 0.333 | 0.000 | 1.000 | 0.471 | true |

## Individual executions

| Task | Status | Duration (s) | Requests | Input tokens | Output tokens | Cached input | Evidence |
|---|---|---:|---:|---:|---:|---:|---|
| pi-add-model-with-docs/run-001 | scored | 114.75 | 14 | 63788 | 5947 | 51968 | [result](tasks/pi-add-model-with-docs/run-001/result.json), [trace](tasks/pi-add-model-with-docs/run-001/trace.jsonl) |
| pi-add-model-without-docs/run-001 | agent_error | 137.88 | 16 | 27936 | 7471 | 19200 | [result](tasks/pi-add-model-without-docs/run-001/result.json), [trace](tasks/pi-add-model-without-docs/run-001/trace.jsonl) |
| pi-prompt-template-project-with-docs/run-001 | scored | 19.64 | 6 | 11597 | 796 | 3840 | [result](tasks/pi-prompt-template-project-with-docs/run-001/result.json), [trace](tasks/pi-prompt-template-project-with-docs/run-001/trace.jsonl) |
| pi-prompt-template-project-without-docs/run-001 | scored | 28.91 | 7 | 8128 | 1311 | 4352 | [result](tasks/pi-prompt-template-project-without-docs/run-001/result.json), [trace](tasks/pi-prompt-template-project-without-docs/run-001/trace.jsonl) |
| pi-prompt-template-user-with-docs/run-001 | scored | 26.87 | 6 | 13377 | 1217 | 6144 | [result](tasks/pi-prompt-template-user-with-docs/run-001/result.json), [trace](tasks/pi-prompt-template-user-with-docs/run-001/trace.jsonl) |
| pi-prompt-template-user-without-docs/run-001 | scored | 29.57 | 6 | 6803 | 1432 | 3200 | [result](tasks/pi-prompt-template-user-without-docs/run-001/result.json), [trace](tasks/pi-prompt-template-user-without-docs/run-001/trace.jsonl) |
| pi-prompt-template-user-without-docs/run-002 | scored | 27.24 | 6 | 6991 | 1238 | 3200 | [result](tasks/pi-prompt-template-user-without-docs/run-002/result.json), [trace](tasks/pi-prompt-template-user-without-docs/run-002/trace.jsonl) |
| pi-prompt-template-user-with-docs/run-002 | scored | 21.12 | 5 | 7547 | 1008 | 3200 | [result](tasks/pi-prompt-template-user-with-docs/run-002/result.json), [trace](tasks/pi-prompt-template-user-with-docs/run-002/trace.jsonl) |
| pi-prompt-template-project-without-docs/run-002 | scored | 23.41 | 6 | 7153 | 1045 | 3840 | [result](tasks/pi-prompt-template-project-without-docs/run-002/result.json), [trace](tasks/pi-prompt-template-project-without-docs/run-002/trace.jsonl) |
| pi-prompt-template-project-with-docs/run-002 | scored | 28.29 | 7 | 15107 | 1259 | 7808 | [result](tasks/pi-prompt-template-project-with-docs/run-002/result.json), [trace](tasks/pi-prompt-template-project-with-docs/run-002/trace.jsonl) |
| pi-add-model-without-docs/run-002 | agent_error | 175.08 | 16 | 27859 | 9924 | 19072 | [result](tasks/pi-add-model-without-docs/run-002/result.json), [trace](tasks/pi-add-model-without-docs/run-002/trace.jsonl) |
| pi-add-model-with-docs/run-002 | scored | 114.89 | 14 | 68909 | 5858 | 56576 | [result](tasks/pi-add-model-with-docs/run-002/result.json), [trace](tasks/pi-add-model-with-docs/run-002/trace.jsonl) |
| pi-add-model-with-docs/run-003 | agent_error | 176.89 | 16 | 81466 | 9787 | 67328 | [result](tasks/pi-add-model-with-docs/run-003/result.json), [trace](tasks/pi-add-model-with-docs/run-003/trace.jsonl) |
| pi-add-model-without-docs/run-003 | scored | 65.98 | 10 | 14336 | 3299 | 8960 | [result](tasks/pi-add-model-without-docs/run-003/result.json), [trace](tasks/pi-add-model-without-docs/run-003/trace.jsonl) |
| pi-prompt-template-project-with-docs/run-003 | scored | 25.77 | 5 | 10358 | 1215 | 3840 | [result](tasks/pi-prompt-template-project-with-docs/run-003/result.json), [trace](tasks/pi-prompt-template-project-with-docs/run-003/trace.jsonl) |
| pi-prompt-template-project-without-docs/run-003 | scored | 18.80 | 5 | 5461 | 822 | 1536 | [result](tasks/pi-prompt-template-project-without-docs/run-003/result.json), [trace](tasks/pi-prompt-template-project-without-docs/run-003/trace.jsonl) |
| pi-prompt-template-user-with-docs/run-003 | scored | 21.14 | 6 | 13314 | 886 | 6144 | [result](tasks/pi-prompt-template-user-with-docs/run-003/result.json), [trace](tasks/pi-prompt-template-user-with-docs/run-003/trace.jsonl) |
| pi-prompt-template-user-without-docs/run-003 | scored | 23.64 | 6 | 6894 | 1063 | 2688 | [result](tasks/pi-prompt-template-user-without-docs/run-003/result.json), [trace](tasks/pi-prompt-template-user-without-docs/run-003/trace.jsonl) |

### pi-add-model-with-docs/run-001

Final response:

> The configuration has been written to `/eval/home/.pi/agent/models.json`. Here's a summary of what was configured:
> 
> - **Provider**: `openai`
> - **Model ID**: `fixture-chat` (referenced as `openai/fixture-chat`)
> - **Display name**: "Fixture Chat"
> - **Input**: text only
> - **Reasoning**: supported (`true`)
> - **Context window**: 32,768 tokens
> - **Max output**: 4,096 tokens
> - **Cost**: $0 input / $0 output (no usage cost)
> 
> This is a user-level configuration in the agent directory (`~/.pi/agent/models.json`), not a project-local configuration. The `models` entry under the `openai` provider adds this model to the catalog.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"result":{"existingModelsPreserved":true,"model":{"contextWindow":32768,"cost":{"input":0,"output":0},"id":"fixture-chat","input":["text"],"maxTokens":0,"name":"Fixture Chat","provider":"openai","reasoning":true}}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/runtime/docs"} — 0 ms
2. `ls` {"path":"/eval/home/.pi"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/models.md"} — 0 ms
4. `read` {"path":"/eval/runtime/docs/configuration.md"} — 0 ms
5. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
6. `find` {"path":"/eval/runtime","pattern":"**/models.json"} — 0 ms
7. `grep` {"glob":"*.md","path":"/eval/runtime/docs","pattern":"contextWindow|maxOutput|reasoning|cost|name"} — 0 ms
8. `grep` {"path":"/eval/runtime","pattern":"contextWindow|maxOutput|modelOverrides|reasoning"} — 0 ms
9. `find` {"limit":20,"path":"/eval/runtime","pattern":"**/*.json"} — 0 ms
10. `grep` {"path":"/eval/runtime","pattern":"contextWindow|maxOutput|\\\"cost\\\"|\\\"name\\\"|\\\"reasoning\\\"|\\\"input\\\""} — 0 ms
11. `ls` {"path":"/eval/runtime"} — 0 ms
12. `find` {"path":"/eval/runtime","pattern":"**/*"} — 0 ms
13. `grep` {"path":"/eval/runtime","pattern":"contextWindow|maxOutput|\\\"cost\\\"|\\\"name\\\"|reasoning|\\\"input\\\"|context|output"} — 1 ms
14. `write` {"content":"{\n  \"providers\": {\n    \"openai\": {\n      \"models\": [\n        {\n          \"id\": \"fixture-chat\",\n          \"name\": \"Fixture Chat\",\n          \"input\": [\"text\"],\n          \"reasoning\": true,\n          \"contextWindow\": 32768,\n          \"maxOutput\": 4096,\n          \"cost\": {\n            \"input\": 0,\n            \"output\": 0\n          }\n        }\n      ]\n    }\n  }\n}\n","path":"/eval/home/.pi/agent/models.json"} — 0 ms

### pi-add-model-without-docs/run-001

Final response:

> 

Diagnostics:

- agent / agent_turn_budget_exceeded: Agent 已达到本轮安全预算，且未能生成可靠的最终说明。

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"result":{"error":"Model openai/fixture-chat is unavailable after reload."}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/workspace"} — 0 ms
2. `ls` {"path":"/eval/home/.pi"} — 0 ms
3. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
4. `find` {"path":"/eval/home/.pi","pattern":"**/*.json"} — 0 ms
5. `ls` {"path":"/eval/home/.pi"} — 0 ms
6. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
7. `find` {"path":"/eval/home","pattern":"**/*"} — 0 ms
8. `write` {"content":"{\n  \"models\": [\n    {\n      \"id\": \"openai/fixture-chat\",\n      \"name\": \"Fixture Chat\",\n      \"input\": [\"text\"],\n      \"reasoning\": true,\n      \"contextWindow\": 32768,\n      \"maxOutput\": 4096,\n      \"cost\": {\n        \"input\": 0,\n        \"output\": 0\n      }\n    }\n  ]\n}\n","path":"/eval/home/.pi/agent/models.json"} — 0 ms
9. `grep` {"path":"/eval/home","pattern":"models"} — 0 ms
10. `ls` {"path":"/eval/workspace"} — 0 ms
11. `find` {"path":"/eval/home","pattern":"**/*.*"} — 0 ms
12. `ls` {"path":"/eval/home"} — 0 ms
13. `ls` {"path":"/eval/home/.pi"} — 0 ms
14. `write` {"content":"{\n  \"models\": [\n    {\n      \"id\": \"openai/fixture-chat\",\n      \"name\": \"Fixture Chat\",\n      \"input\": [\"text\"],\n      \"reasoning\": true,\n      \"contextWindow\": 32768,\n      \"maxOutput\": 4096,\n      \"cost\": {\n        \"input\": 0,\n        \"output\": 0\n      }\n    }\n  ]\n}\n","path":"/eval/home/.pi/config.json"} — 0 ms
15. `write` {"content":"{\n  \"models\": [\n    {\n      \"id\": \"openai/fixture-chat\",\n      \"name\": \"Fixture Chat\",\n      \"input\": [\"text\"],\n      \"reasoning\": true,\n      \"contextWindow\": 32768,\n      \"maxOutput\": 4096,\n      \"cost\": {\n        \"input\": 0,\n        \"output\": 0\n      }\n    }\n  ]\n}\n","path":"/eval/home/.pi/models.json"} — 0 ms
16. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
17. `ls` {"path":"/eval/home/.pi"} — 0 ms
18. `write` {"content":"","path":"/eval/home/.pi/config.json"} — 0 ms
19. `write` {"content":"","path":"/eval/home/.pi/models.json"} — 0 ms
20. `write` {"content":"{}","path":"/eval/home/.pi/config.json"} — 0 ms
21. `write` {"content":"{}","path":"/eval/home/.pi/models.json"} — 0 ms
22. `read` {"path":"/eval/home/.pi/agent/models.json"} — 0 ms

### pi-prompt-template-project-with-docs/run-001

Final response:

> I don't have access to tools that can retrieve the current time or timezone information. The tools available to me are limited to file operations (reading, writing, searching files) and don't include any system clock or time-related functions.
> 
> To get the current time and timezone, you could:
> - Check your system clock directly
> - Use a terminal command like `date` (on Unix/Linux/macOS) or `Get-Date` (on Windows PowerShell)
> - Ask a different assistant with access to time-related tools

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"project"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `ls` {"path":"/eval/workspace"} — 0 ms
3. `ls` {"path":"/eval/runtime/docs"} — 0 ms
4. `read` {"path":"/eval/runtime/docs/prompt-templates.md"} — 0 ms
5. `read` {"path":"/eval/runtime/docs/configuration.md"} — 0 ms
6. `write` {"content":"Tell me the current time and timezone.\n","path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms

### pi-prompt-template-project-without-docs/run-001

Final response:

> I don't have access to real-time information or the ability to retrieve the current time and timezone. My available tools are limited to file operations (read, write, edit, find, grep, ls) and don't include any system time functions.
> 
> To get the current time and timezone, you would need to:
> - Check your system clock
> - Use a command like `date` in your terminal
> - Or ask a different AI assistant that has access to real-time data

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"project"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `ls` {"path":"/eval/workspace"} — 0 ms
3. `ls` {"path":"/eval/workspace/.pi"} — 0 ms
4. `ls` {"path":"/eval/workspace/.pi"} — 0 ms
5. `find` {"pattern":".pi/**/*"} — 0 ms
6. `write` {"content":"Tell me the current time and timezone.","path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms
7. `read` {"path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms

### pi-prompt-template-user-with-docs/run-001

Final response:

> I don't have access to a real-time clock or system time, so I can't tell you the current time or timezone. You can check your system clock directly, or if you have a tool or extension that provides time information, I'd be happy to help you use it.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"user"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/runtime/docs"} — 0 ms
2. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/prompt-templates.md"} — 0 ms
4. `read` {"path":"/eval/runtime/docs/configuration.md"} — 0 ms
5. `write` {"content":"---\ndescription: Tell me the current time and timezone\n---\nTell me the current time and timezone.\n","path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms
6. `read` {"path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms

### pi-prompt-template-user-without-docs/run-001

Final response:

> I don't have access to a clock or any tool that can determine the current time or timezone. My environment doesn't provide shell execution, network access, or any time-related API, so I'm unable to retrieve this information for you.
> 
> If you're running Pi in a terminal or environment that supports it, you could check your system time directly (e.g., by running `date` in your shell).

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"user"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi"} — 0 ms
2. `find` {"path":"/eval/home/.pi","pattern":"**/*"} — 0 ms
3. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
4. `write` {"content":"Tell me the current time and timezone.\n","path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms
5. `read` {"path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms

### pi-prompt-template-user-without-docs/run-002

Final response:

> I don't have access to a real-time clock or system time function in this environment, so I'm unable to tell you the current time and timezone. 
> 
> The `/current-time` prompt template is working correctly — it's just that I lack the capability to retrieve live system time. If this workspace had a shell or time-related tool available, the prompt could be fulfilled.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":false,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":null} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `find` {"path":"/eval/home/.pi/agent","pattern":"*"} — 0 ms
3. `ls` {"path":"/eval/home/.pi"} — 0 ms
4. `find` {"path":"/eval/home/.pi","pattern":"**/*"} — 0 ms
5. `write` {"content":"Tell me the current time and timezone.","path":"/eval/home/.pi/prompts/current-time.md"} — 0 ms
6. `read` {"path":"/eval/home/.pi/prompts/current-time.md"} — 0 ms

### pi-prompt-template-user-with-docs/run-002

Final response:

> I don't have access to tools that can retrieve the current time or timezone information. The tools available to me are limited to file operations (reading, writing, editing, searching files) and don't include any way to query system time or execute shell commands.
> 
> If you need the current time, you could:
> - Check your system clock directly
> - Use a terminal command like `date` (on Unix-like systems) or PowerShell's `Get-Date` (on Windows)
> - Use an online time service
> 
> The `/current-time` prompt template we created is ready to use in Pi sessions, but I can't actually execute it to get the time for you right now.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"user"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/runtime/docs"} — 0 ms
2. `ls` {"path":"/eval/home/.pi"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/prompt-templates.md"} — 0 ms
4. `write` {"content":"Tell me the current time and timezone.\n","path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms

### pi-prompt-template-project-without-docs/run-002

Final response:

> I don't have access to any tools that can retrieve the current time or timezone information. The tools available to me in this workspace are limited to file operations (reading, writing, editing, searching, and listing files). I cannot query the system clock or timezone settings.
> 
> To get the current time and timezone, you would need to check your local system clock or run a command like `date` in your terminal.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"project"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `ls` {"path":"/eval/workspace"} — 0 ms
3. `find` {"path":"/eval/workspace","pattern":".pi/**/*"} — 0 ms
4. `find` {"path":"/eval/workspace","pattern":"**/.pi/**/*"} — 0 ms
5. `find` {"path":"/eval/home/.pi","pattern":"**/*"} — 0 ms
6. `write` {"content":"Tell me the current time and timezone.\n","path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms
7. `read` {"path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms

### pi-prompt-template-project-with-docs/run-002

Final response:

> I don't have access to a tool that can retrieve the current time or timezone. The available tools in this environment are limited to file system operations (reading, writing, editing, searching files) and don't include any system time or date functions.
> 
> To get the current time and timezone, you would need to check your system clock or use a command like `date` in a terminal outside of this environment.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"project"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `ls` {"path":"/eval/runtime/docs"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/prompt-templates.md"} — 0 ms
4. `read` {"path":"/eval/runtime/docs/configuration.md"} — 0 ms
5. `write` {"content":"Tell me the current time and timezone.\n","path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms
6. `read` {"path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms

### pi-add-model-without-docs/run-002

Final response:

> I've configured the new `openai/fixture-chat` model in your Pi installation's global configuration (not project-local). The configuration includes:
> 
> - **Model ID**: `openai/fixture-chat`
> - **Display Name**: "Fixture Chat"
> - **Input Type**: text
> - **Reasoning Support**: enabled
> - **Context Window**: 32,768 tokens
> - **Maximum Output**: 4,096 tokens
> - **Cost**: $0 (no usage cost)
> 
> The configuration has been added to `/eval/home/.pi/config.json` and also as a model definition file at `/eval/home/.pi/agent/models/openai/fixture-chat.json`.

Diagnostics:

- agent / budget_limit: Runtime ended this turn: map[budget_reason:model_request_limit completion_reason:budget_limit request_ordinal:16 request_uuid:01a1733b-e071-7638-84b7-021973b00d74]

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"result":{"error":"Model openai/fixture-chat is unavailable after reload."}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi"} — 0 ms
2. `ls` {"path":"/eval/workspace"} — 0 ms
3. `find` {"pattern":"**/*pi*"} — 0 ms
4. `find` {"pattern":"**/*.json"} — 0 ms
5. `find` {"pattern":"**/config*"} — 0 ms
6. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
7. `ls` {"path":"/eval/home/.pi"} — 0 ms
8. `grep` {"path":"/eval/home/.pi","pattern":"model"} — 0 ms
9. `find` {"path":"/eval/home","pattern":"**/*"} — 0 ms
10. `write` {"content":"{\n  \"models\": {\n    \"openai/fixture-chat\": {\n      \"name\": \"Fixture Chat\",\n      \"accepts\": \"text\",\n      \"reasoning\": true,\n      \"contextWindow\": 32768,\n      \"maxOutput\": 4096,\n      \"cost\": {\n        \"input\": 0,\n        \"output\": 0\n      }\n    }\n  }\n}\n","path":"/eval/home/.pi/config.json"} — 0 ms
11. `grep` {"pattern":"fixture"} — 0 ms
12. `grep` {"pattern":"openai"} — 0 ms
13. `grep` {"pattern":"model"} — 0 ms
14. `find` {"path":"/eval/home/.pi","pattern":"**/*"} — 0 ms
15. `find` {"path":"/eval/workspace","pattern":"**/*"} — 0 ms
16. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
17. `read` {"path":"/eval/home/.pi/config.json"} — 0 ms
18. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
19. `write` {"content":"{\n  \"name\": \"Fixture Chat\",\n  \"accepts\": \"text\",\n  \"reasoning\": true,\n  \"contextWindow\": 32768,\n  \"maxOutput\": 4096,\n  \"cost\": {\n    \"input\": 0,\n    \"output\": 0\n  }\n}\n","path":"/eval/home/.pi/agent/models/openai/fixture-chat.json"} — 0 ms
20. `find` {"path":"/eval/home/.pi/agent","pattern":"**/*"} — 0 ms
21. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
22. `read` {"path":"/eval/home/.pi/config.json"} — 0 ms
23. `read` {"path":"/eval/home/.pi/agent/models/openai/fixture-chat.json"} — 0 ms

### pi-add-model-with-docs/run-002

Final response:

> The configuration has been created at `/eval/home/.pi/agent/models.json` (the agent directory, not project-local). Here's what was configured:
> 
> - **Model ID**: `openai/fixture-chat` (provider `openai`, model id `fixture-chat`)
> - **Display name**: "Fixture Chat"
> - **Input modalities**: `["text"]` (text only)
> - **Reasoning**: `true` (supports reasoning/thinking)
> - **Context window**: 32,768 tokens
> - **Max output**: 4,096 tokens
> - **Cost**: 0 for both input and output (no usage cost)
> 
> The model is added under the `openai` provider's `models` array, which per the documentation adds or replaces a model with that ID on the provider. Opening `/model` will reload the file and make the new model available.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"result":{"existingModelsPreserved":true,"model":{"contextWindow":32768,"cost":{"input":0,"output":0},"id":"fixture-chat","input":["text"],"maxTokens":0,"name":"Fixture Chat","provider":"openai","reasoning":true}}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/runtime/docs"} — 0 ms
2. `ls` {"path":"/eval/home/.pi"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/models.md"} — 0 ms
4. `read` {"path":"/eval/runtime/docs/configuration.md"} — 0 ms
5. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
6. `grep` {"limit":5,"path":"/eval/runtime","pattern":"contextWindow|maxOutput|contextTokens|maxTokens|displayName"} — 0 ms
7. `grep` {"limit":5,"path":"/eval/runtime","pattern":"modelOverrides|models\\.json"} — 0 ms
8. `ls` {"path":"/eval/runtime/docs"} — 0 ms
9. `grep` {"limit":10,"path":"/eval/runtime","pattern":"reasoning|thinking|supportsThinking"} — 0 ms
10. `grep` {"limit":10,"path":"/eval/runtime","pattern":"contextWindow|context_window|contextLimit|\\\"context\\\""} — 0 ms
11. `grep` {"limit":20,"path":"/eval/runtime","pattern":"cost|price|pricing|inputCost|outputCost"} — 0 ms
12. `grep` {"limit":20,"path":"/eval/runtime","pattern":"\\\"name\\\"|displayName|\\\"label\\\""} — 0 ms
13. `grep` {"limit":20,"path":"/eval/runtime","pattern":"\\\"input\\\"|\\\"reasoning\\\"|\\\"name\\\"|\\\"id\\\""} — 0 ms
14. `grep` {"limit":30,"path":"/eval/runtime","pattern":"contextWindow|context_window|maxOutput|outputLimit|maxTokens|contextLimit"} — 0 ms
15. `grep` {"limit":30,"path":"/eval/runtime","pattern":"reasoning|supportsReasoning|thinking"} — 0 ms
16. `find` {"path":"/eval/runtime","pattern":"*.ts"} — 0 ms
17. `find` {"path":"/eval/runtime","pattern":"*.json"} — 0 ms
18. `grep` {"limit":30,"path":"/eval/runtime","pattern":"modelOverrides|\\\"models\\\""} — 0 ms
19. `write` {"content":"{\n  \"providers\": {\n    \"openai\": {\n      \"models\": [\n        {\n          \"id\": \"fixture-chat\",\n          \"name\": \"Fixture Chat\",\n          \"input\": [\"text\"],\n          \"reasoning\": true,\n          \"contextWindow\": 32768,\n          \"maxOutput\": 4096,\n          \"cost\": {\n            \"input\": 0,\n            \"output\": 0\n          }\n        }\n      ]\n    }\n  }\n}\n","path":"/eval/home/.pi/agent/models.json"} — 0 ms
20. `read` {"path":"/eval/home/.pi/agent/models.json"} — 0 ms

### pi-add-model-with-docs/run-003

Final response:

> I've configured the new `openai/fixture-chat` model in the user-level `models.json` at `/eval/home/.pi/agent/models.json`. Here's what was set:
> 
> - **Model ID**: `openai/fixture-chat`
> - **Display name**: "Fixture Chat"
> - **Input**: text only (`["text"]`)
> - **Reasoning**: supported (`true`)
> - **Context window**: 32,768 tokens
> - **Max output**: 4,096 tokens
> - **Cost**: no cost (omitted, which defaults to no cost per the documentation)
> 
> The configuration is in the agent directory (`~/.pi/agent/models.json`), not project-local, as requested. The `openai` provider's `models` array adds this model to Pi's catalog. Per the docs, models without an explicit catalog price report tokens at no cost.

Diagnostics:

- agent / budget_limit: Runtime ended this turn: map[budget_reason:model_request_limit completion_reason:budget_limit request_ordinal:16 request_uuid:01a17340-2813-78ac-8311-07bc34d43433]

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"result":{"error":"Model openai/fixture-chat is unavailable after reload."}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/runtime/docs"} — 0 ms
2. `ls` {"path":"/eval/home/.pi"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/models.md"} — 0 ms
4. `read` {"path":"/eval/runtime/docs/configuration.md"} — 0 ms
5. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
6. `grep` {"path":"/eval/runtime/docs","pattern":"reasoning|contextWindow|maxOutput|cost"} — 0 ms
7. `find` {"path":"/eval/runtime","pattern":"*.json"} — 0 ms
8. `grep` {"path":"/eval/runtime","pattern":"contextWindow|maxOutput|reasoning|modelOverrides"} — 1 ms
9. `grep` {"path":"/eval/runtime","pattern":"models\\.json"} — 0 ms
10. `grep` {"path":"/eval/runtime","pattern":"\\\"name\\\"|\\\"input\\\"|\\\"reasoning\\\"|\\\"context\\\"|\\\"maxOutput\\\"|\\\"cost\\\"|\\\"noCost\\\"|\\\"free\\\""} — 0 ms
11. `find` {"path":"/eval/runtime","pattern":"*model*"} — 0 ms
12. `find` {"path":"/eval/runtime","pattern":"*catalog*"} — 0 ms
13. `ls` {"path":"/eval/runtime"} — 0 ms
14. `grep` {"path":"/eval/runtime","pattern":"contextWindow|maxOutput|reasoning|cost"} — 0 ms
15. `find` {"path":"/eval/runtime","pattern":"*"} — 0 ms
16. `ls` {"path":"/eval/workspace"} — 0 ms
17. `grep` {"path":"/eval/workspace","pattern":"contextWindow|maxOutput|reasoning|name"} — 0 ms
18. `grep` {"path":"/eval/runtime","pattern":"context|window|token|output|reason|cost|price|name|display"} — 1 ms
19. `find` {"path":"/eval/runtime","pattern":"**/*"} — 0 ms
20. `grep` {"path":"/eval/runtime","pattern":"contextWindow|maxOutput|reasoning|\\\"name\\\"|\\\"displayName\\\"|\\\"cost\\\"|\\\"pricing\\\"|\\\"noCost\\\"|\\\"free\\\"|\\\"input\\\"|\\\"supports\\\""} — 0 ms
21. `write` {"content":"{\n  \"providers\": {\n    \"openai\": {\n      \"models\": [\n        {\n          \"id\": \"fixture-chat\",\n          \"name\": \"Fixture Chat\",\n          \"input\": [\"text\"],\n          \"reasoning\": true,\n          \"contextWindow\": 32768,\n          \"maxOutput\": 4096\n        }\n      ]\n    }\n  }\n}\n","path":"/eval/home/.pi/agent/models.json"} — 0 ms
22. `read` {"path":"/eval/home/.pi/agent/models.json"} — 0 ms

### pi-add-model-without-docs/run-003

Final response:

> I've successfully configured the Pi installation with the new `openai/fixture-chat` model. The configuration has been added to `/eval/home/.pi/config/models.json` with the following specifications:
> 
> - **Model ID**: `openai/fixture-chat`
> - **Display Name**: "Fixture Chat"
> - **Input Types**: text
> - **Reasoning Support**: enabled
> - **Context Window**: 32,768 tokens
> - **Maximum Output**: 4,096 tokens
> - **Usage Cost**: zero (no cost for input, output, cache read, or cache write)
> 
> The configuration is stored in the global Pi configuration directory (`/eval/home/.pi/config/`) rather than as project-local configuration, as requested.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"result":{"error":"Model openai/fixture-chat is unavailable after reload."}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/workspace"} — 0 ms
2. `ls` {"path":"/eval/home/.pi"} — 0 ms
3. `ls` {"path":"/eval/workspace"} — 0 ms
4. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
5. `grep` {"path":"/eval/home/.pi","pattern":"model"} — 0 ms
6. `grep` {"path":"/eval/workspace","pattern":"model"} — 0 ms
7. `find` {"path":"/eval/home/.pi","pattern":"**/*"} — 0 ms
8. `write` {"content":"{\n  \"openai/fixture-chat\": {\n    \"name\": \"Fixture Chat\",\n    \"input\": [\"text\"],\n    \"reasoning\": true,\n    \"contextWindow\": 32768,\n    \"maxOutput\": 4096,\n    \"cost\": {\n      \"input\": 0,\n      \"output\": 0,\n      \"cacheRead\": 0,\n      \"cacheWrite\": 0\n    }\n  }\n}\n","path":"/eval/home/.pi/config/models.json"} — 0 ms
9. `read` {"path":"/eval/home/.pi/config/models.json"} — 0 ms
10. `read` {"path":"/eval/home/.pi/config.json"} — 0 ms; error: not_found

### pi-prompt-template-project-with-docs/run-003

Final response:

> I don't have access to a real-time clock or your local timezone settings in this environment. Because I don't have shell execution or network access to fetch live time data, I'm unable to tell you the current time and timezone.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"project"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `ls` {"path":"/eval/runtime/docs"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/prompt-templates.md"} — 0 ms
4. `read` {"path":"/eval/runtime/docs/configuration.md"} — 0 ms
5. `write` {"content":"Tell me the current time and timezone.\n","path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms

### pi-prompt-template-project-without-docs/run-003

Final response:

> I don't have access to a tool that can retrieve the current system time or timezone information. The available tools in this workspace are limited to file operations (read, write, edit, find, grep, ls) and don't include any way to query the system clock or environment.
> 
> If you need the current time, you could check your local system clock or use a service like `date` in a terminal environment outside of this workspace.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"project"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `ls` {"path":"/eval/workspace"} — 0 ms
3. `write` {"content":"Tell me the current time and timezone.","path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms
4. `read` {"path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms

### pi-prompt-template-user-with-docs/run-003

Final response:

> I'm sorry, but I don't have access to any tools that can retrieve the current time or timezone information. My available tools are limited to file operations (reading, writing, editing, searching files) and directory listing. I don't have shell execution, network access, or any time-related capabilities.
> 
> To get the current time and timezone, you would need to check your system clock or run a command like `date` in your terminal directly.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"user"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `ls` {"path":"/eval/runtime/docs"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/prompt-templates.md"} — 0 ms
4. `read` {"path":"/eval/runtime/docs/configuration.md"} — 0 ms
5. `write` {"content":"Tell me the current time and timezone.","path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms
6. `read` {"path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms

### pi-prompt-template-user-without-docs/run-003

Final response:

> I don't have access to a clock or system time through my available tools, so I'm unable to tell you the current time and timezone. 
> 
> If you need this functionality, you could check your system clock directly, or if there's a way to extend the toolset with a time-related tool, the `/current-time` prompt would then be able to use it.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":false,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":null} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi"} — 0 ms
2. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
3. `write` {"content":"Tell me the current time and timezone.","path":"/eval/home/.pi/prompts/current-time.md"} — 0 ms
4. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
5. `ls` {"path":"/eval/home/.pi"} — 0 ms
6. `read` {"path":"/eval/home/.pi/prompts/current-time.md"} — 0 ms
