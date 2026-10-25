# Lumi Agent evaluation report

Model: `qwen3.7-plus` (openai_compatible). Planned: 18; recorded: 18. Repetitions: 3.

Protocol: `07167443b7a19c16f2fe6422e936b4cdb77dcd99ee3ddd1707c480a53ab7ddcd`

Scores describe each criterion on [0,1]; there is no global acceptance threshold. Agent errors retain observable scores. Environment failures are unscored and excluded from score means. Incomplete cohorts cannot support a complete-run claim. N/A means unavailable, never zero. One repetition cannot establish stability; standard deviation is population SD and is N/A for fewer than two observations. Cost is N/A because this harness has no frozen pricing source.

## Environment / evidence gaps

- pi-prompt-template-project-without-docs/run-001: environment_error

## Repetition statistics

| Case | Recorded scores / planned | Criterion | Mean | Min | Max | SD | Variation |
|---|---:|---|---:|---:|---:|---:|---|
| pi-add-model-with-docs | 3/3 | upstream_contract | 0.000 | 0.000 | 0.000 | 0.000 | false |
| pi-add-model-without-docs | 3/3 | upstream_contract | 0.000 | 0.000 | 0.000 | 0.000 | false |
| pi-prompt-template-project-with-docs | 3/3 | upstream_contract | 1.000 | 1.000 | 1.000 | 0.000 | false |
| pi-prompt-template-project-without-docs | 2/3 | upstream_contract | 0.500 | 0.000 | 1.000 | 0.500 | true |
| pi-prompt-template-user-with-docs | 3/3 | upstream_contract | 1.000 | 1.000 | 1.000 | 0.000 | false |
| pi-prompt-template-user-without-docs | 3/3 | upstream_contract | 0.667 | 0.000 | 1.000 | 0.471 | true |

## Individual executions

| Task | Status | Duration (s) | Requests | Input tokens | Output tokens | Cached input | Evidence |
|---|---|---:|---:|---:|---:|---:|---|
| pi-add-model-with-docs/run-001 | scored | 174.45 | 14 | 65109 | 9600 | 53632 | [result](tasks/pi-add-model-with-docs/run-001/result.json), [trace](tasks/pi-add-model-with-docs/run-001/trace.jsonl) |
| pi-add-model-without-docs/run-001 | scored | 108.81 | 12 | 19107 | 6043 | 13440 | [result](tasks/pi-add-model-without-docs/run-001/result.json), [trace](tasks/pi-add-model-without-docs/run-001/trace.jsonl) |
| pi-prompt-template-project-with-docs/run-001 | scored | 32.39 | 7 | 15358 | 1537 | 7808 | [result](tasks/pi-prompt-template-project-with-docs/run-001/result.json), [trace](tasks/pi-prompt-template-project-with-docs/run-001/trace.jsonl) |
| pi-prompt-template-project-without-docs/run-001 | environment_error | 32.31 | 7 | 8516 | 1602 | 4992 | [result](tasks/pi-prompt-template-project-without-docs/run-001/result.json), [trace](tasks/pi-prompt-template-project-without-docs/run-001/trace.jsonl) |
| pi-prompt-template-user-with-docs/run-001 | scored | 20.45 | 6 | 13391 | 846 | 6144 | [result](tasks/pi-prompt-template-user-with-docs/run-001/result.json), [trace](tasks/pi-prompt-template-user-with-docs/run-001/trace.jsonl) |
| pi-prompt-template-user-without-docs/run-001 | scored | 21.51 | 5 | 5360 | 976 | 1536 | [result](tasks/pi-prompt-template-user-without-docs/run-001/result.json), [trace](tasks/pi-prompt-template-user-without-docs/run-001/trace.jsonl) |
| pi-prompt-template-user-without-docs/run-002 | scored | 25.04 | 6 | 6924 | 1116 | 2688 | [result](tasks/pi-prompt-template-user-without-docs/run-002/result.json), [trace](tasks/pi-prompt-template-user-without-docs/run-002/trace.jsonl) |
| pi-prompt-template-user-with-docs/run-002 | scored | 18.18 | 5 | 7530 | 742 | 3200 | [result](tasks/pi-prompt-template-user-with-docs/run-002/result.json), [trace](tasks/pi-prompt-template-user-with-docs/run-002/trace.jsonl) |
| pi-prompt-template-project-without-docs/run-002 | agent_error | 14.36 | 4 | 3436 | 587 | 512 | [result](tasks/pi-prompt-template-project-without-docs/run-002/result.json), [trace](tasks/pi-prompt-template-project-without-docs/run-002/trace.jsonl) |
| pi-prompt-template-project-with-docs/run-002 | scored | 20.68 | 6 | 13288 | 891 | 6144 | [result](tasks/pi-prompt-template-project-with-docs/run-002/result.json), [trace](tasks/pi-prompt-template-project-with-docs/run-002/trace.jsonl) |
| pi-add-model-without-docs/run-002 | agent_error | 129.38 | 16 | 25994 | 6977 | 16768 | [result](tasks/pi-add-model-without-docs/run-002/result.json), [trace](tasks/pi-add-model-without-docs/run-002/trace.jsonl) |
| pi-add-model-with-docs/run-002 | scored | 149.11 | 15 | 69701 | 8101 | 58624 | [result](tasks/pi-add-model-with-docs/run-002/result.json), [trace](tasks/pi-add-model-with-docs/run-002/trace.jsonl) |
| pi-add-model-with-docs/run-003 | agent_error | 106.25 | 11 | 47783 | 5649 | 33664 | [result](tasks/pi-add-model-with-docs/run-003/result.json), [trace](tasks/pi-add-model-with-docs/run-003/trace.jsonl) |
| pi-add-model-without-docs/run-003 | scored | 86.21 | 13 | 20078 | 4572 | 14080 | [result](tasks/pi-add-model-without-docs/run-003/result.json), [trace](tasks/pi-add-model-without-docs/run-003/trace.jsonl) |
| pi-prompt-template-project-with-docs/run-003 | scored | 22.53 | 6 | 13372 | 901 | 6144 | [result](tasks/pi-prompt-template-project-with-docs/run-003/result.json), [trace](tasks/pi-prompt-template-project-with-docs/run-003/trace.jsonl) |
| pi-prompt-template-project-without-docs/run-003 | scored | 31.32 | 7 | 8301 | 1473 | 4352 | [result](tasks/pi-prompt-template-project-without-docs/run-003/result.json), [trace](tasks/pi-prompt-template-project-without-docs/run-003/trace.jsonl) |
| pi-prompt-template-user-with-docs/run-003 | scored | 20.24 | 6 | 9524 | 789 | 4864 | [result](tasks/pi-prompt-template-user-with-docs/run-003/result.json), [trace](tasks/pi-prompt-template-user-with-docs/run-003/trace.jsonl) |
| pi-prompt-template-user-without-docs/run-003 | scored | 20.77 | 6 | 6525 | 831 | 2048 | [result](tasks/pi-prompt-template-user-without-docs/run-003/result.json), [trace](tasks/pi-prompt-template-user-without-docs/run-003/trace.jsonl) |

### pi-add-model-with-docs/run-001

Final response:

> I've created the user-level model configuration at `/eval/home/.pi/agent/models.json` with the following setup:
> 
> - **Model ID**: `openai/fixture-chat` (provider `openai`, model `fixture-chat`)
> - **Display name**: "Fixture Chat"
> - **Input**: text only
> - **Reasoning**: supported (`true`)
> - **Context window**: 32,768 tokens
> - **Max output**: 4,096 tokens
> - **Cost**: 0 for both input and output (no usage cost)
> 
> This is configured at the user level (agent directory) rather than as project-local configuration, so it will be available across all projects.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"result":{"existingModelsPreserved":true,"model":{"contextWindow":32768,"cost":{"input":0,"output":0},"id":"fixture-chat","input":["text"],"maxTokens":0,"name":"Fixture Chat","provider":"openai","reasoning":true}}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/runtime/docs"} — 0 ms
2. `ls` {"path":"/eval/home/.pi"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/models.md"} — 0 ms
4. `read` {"path":"/eval/runtime/docs/configuration.md"} — 0 ms
5. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
6. `find` {"path":"/eval/home/.pi","pattern":"**/models.json"} — 0 ms
7. `find` {"path":"/eval/runtime","pattern":"**/*.json"} — 0 ms
8. `grep` {"path":"/eval/runtime/docs","pattern":"contextWindow|maxOutput|reasoning|displayName|costPer"} — 0 ms
9. `grep` {"ignoreCase":true,"path":"/eval/runtime/docs","pattern":"context|maxOutput|reasoning|cost|display|fixture"} — 1 ms
10. `ls` {"path":"/eval/runtime/docs"} — 0 ms
11. `find` {"path":"/eval/runtime","pattern":"**/*catalog*"} — 0 ms
12. `find` {"path":"/eval/runtime","pattern":"**/*model*"} — 0 ms
13. `find` {"path":"/eval/runtime","pattern":"**/*.ts"} — 0 ms
14. `find` {"path":"/eval/runtime","pattern":"**/*.js"} — 0 ms
15. `ls` {"path":"/eval/runtime"} — 0 ms
16. `write` {"content":"{\n  \"providers\": {\n    \"openai\": {\n      \"models\": [\n        {\n          \"id\": \"fixture-chat\",\n          \"name\": \"Fixture Chat\",\n          \"input\": [\"text\"],\n          \"reasoning\": true,\n          \"contextWindow\": 32768,\n          \"maxOutput\": 4096,\n          \"cost\": {\n            \"input\": 0,\n            \"output\": 0\n          }\n        }\n      ]\n    }\n  }\n}\n","path":"/eval/home/.pi/agent/models.json"} — 0 ms
17. `read` {"path":"/eval/home/.pi/agent/models.json"} — 0 ms
18. `grep` {"path":"/eval/workspace","pattern":"contextWindow|maxOutput|reasoning"} — 0 ms
19. `find` {"path":"/eval/workspace","pattern":"**/*.json"} — 0 ms
20. `ls` {"path":"/eval/workspace"} — 0 ms

### pi-add-model-without-docs/run-001

Final response:

> I've created the global (user-level) Pi configuration at `/eval/home/.pi/config.json` with the new `openai/fixture-chat` model. Here's a summary of what was configured:
> 
> | Property | Value |
> |---|---|
> | **Model ID** | `openai/fixture-chat` |
> | **Display Name** | Fixture Chat |
> | **Accepts Text** | Yes (no attachments/images) |
> | **Reasoning** | Supported |
> | **Context Window** | 32,768 tokens |
> | **Max Output** | 4,096 tokens |
> | **Cost** | $0 input / $0 output |
> 
> This is placed in the user-level config (`~/.pi/config.json`) rather than any project-local configuration, as requested.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"result":{"error":"Model openai/fixture-chat is unavailable after reload."}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/workspace"} — 0 ms
2. `ls` {"path":"/eval/home/.pi"} — 0 ms
3. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
4. `find` {"path":"/eval/home/.pi","pattern":"**/*.json"} — 0 ms
5. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
6. `find` {"path":"/eval/home/.pi","pattern":"**/*"} — 0 ms
7. `find` {"path":"/eval/workspace","pattern":"**/*"} — 0 ms
8. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
9. `find` {"path":"/eval/home/.pi","pattern":"**/models/**"} — 0 ms
10. `find` {"path":"/eval/home/.pi","pattern":"**/*.*"} — 0 ms
11. `read` {"path":"/eval/home/.pi/config.json"} — 0 ms; error: not_found
12. `find` {"path":"/eval/home/.pi","pattern":"**/*"} — 0 ms
13. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
14. `grep` {"limit":50,"path":"/eval/home/.pi","pattern":"model"} — 0 ms
15. `grep` {"limit":50,"path":"/eval/workspace","pattern":"model"} — 0 ms
16. `ls` {"path":"/eval/home/.pi"} — 0 ms
17. `write` {"content":"{\n  \"models\": {\n    \"openai/fixture-chat\": {\n      \"name\": \"Fixture Chat\",\n      \"attachment\": false,\n      \"reasoning\": true,\n      \"contextWindow\": 32768,\n      \"maxOutputTokens\": 4096,\n      \"cost\": {\n        \"input\": 0,\n        \"output\": 0\n      }\n    }\n  }\n}\n","path":"/eval/home/.pi/config.json"} — 0 ms
18. `read` {"path":"/eval/home/.pi/config.json"} — 0 ms

### pi-prompt-template-project-with-docs/run-001

Final response:

> I don't have access to tools that can retrieve the current time or timezone information. The available tools in this environment are limited to file operations (read, write, edit, find, grep, ls), so I cannot determine the current time or timezone for you.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"project"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `ls` {"path":"/eval/runtime/docs"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/prompt-templates.md"} — 0 ms
4. `read` {"path":"/eval/runtime/docs/configuration.md"} — 0 ms
5. `ls` {"path":"/eval/workspace"} — 0 ms
6. `write` {"content":"---\ndescription: Tell me the current time and timezone\n---\nTell me the current time and timezone.\n","path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms
7. `read` {"path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms

### pi-prompt-template-project-without-docs/run-001

Final response:

> 

Diagnostics:

- environment / runtime_error: agent_validation_failed

| Criterion | Score | Basis | Observed |
|---|---:|---|---|

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `ls` {"path":"/eval/workspace"} — 0 ms
3. `write` {"content":"Tell me the current time and timezone.\n","path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms
4. `find` {"path":"/eval/workspace","pattern":".pi/**/*"} — 0 ms
5. `find` {"path":"/eval/home/.pi","pattern":"**/*"} — 0 ms
6. `write` {"content":"Tell me the current time and timezone.\n","path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms
7. `read` {"path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms
8. `write` {"content":"","path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms

### pi-prompt-template-user-with-docs/run-001

Final response:

> I don't have access to tools that can retrieve the current time and timezone. The available tools in this environment are limited to file operations (reading, writing, searching files) and don't include system time or shell execution capabilities.
> 
> To get the current time and timezone, you would need to:
> - Run a command like `date` in your terminal
> - Check your system clock
> - Use a different tool or environment that has access to system time information

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"user"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/runtime/docs"} — 0 ms
2. `ls` {"path":"/eval/home/.pi"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/prompt-templates.md"} — 0 ms
4. `read` {"path":"/eval/runtime/docs/configuration.md"} — 0 ms
5. `write` {"content":"---\ndescription: Tell me the current time and timezone\n---\nTell me the current time and timezone.\n","path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms
6. `read` {"path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms

### pi-prompt-template-user-without-docs/run-001

Final response:

> I don't have access to any tools that can retrieve the current system time or timezone. My available tools are limited to file operations (read, write, edit, find, grep, ls) within the workspace, and none of them can query the system clock or timezone settings.
> 
> To get the current time and timezone, you could:
> 
> 1. **Use your system terminal:**
>    - On Linux/macOS: `date`
>    - On Windows: `time /t` or `Get-Date` in PowerShell
> 
> 2. **Use an online service** or check the clock on your device.
> 
> Is there something else I can help you with using the file tools I have available?

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"user"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `write` {"content":"Tell me the current time and timezone.","path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms
3. `read` {"path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms

### pi-prompt-template-user-without-docs/run-002

Final response:

> I don't have access to a real-time clock, so I can't tell you the exact current time. However, I can tell you that my system timezone is typically UTC.
> 
> To get the current time on your system, you could run:
> - **Linux/macOS:** `date` in your terminal
> - **Windows:** `time /t` in Command Prompt
> 
> Would you like me to help you with anything else related to your Pi setup?

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"user"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `ls` {"path":"/eval/home/.pi"} — 0 ms
3. `ls` {"path":"/eval/home/.pi"} — 0 ms
4. `find` {"path":"/eval/home/.pi","pattern":"**/*"} — 0 ms
5. `write` {"content":"Tell me the current time and timezone.","path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms
6. `read` {"path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms

### pi-prompt-template-user-with-docs/run-002

Final response:

> I don't have access to any tools that can retrieve the current time or timezone information. There's no system clock, shell execution, or network tool available in this environment that would let me look that up.
> 
> If you'd like the `/current-time` template to work as intended, you'd need to invoke it in an actual Pi session where the agent has access to system time. In this workspace, I'm limited to the file and search tools provided, none of which can query the current time.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"user"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/runtime/docs"} — 0 ms
2. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/prompt-templates.md"} — 0 ms
4. `write` {"content":"---\ndescription: Ask for the current time and timezone\n---\nTell me the current time and timezone.\n","path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms

### pi-prompt-template-project-without-docs/run-002

Final response:

> 

Diagnostics:

- agent / agent_turn_budget_exceeded: Agent 已达到本轮安全预算，且未能生成可靠的最终说明。

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":false,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":null} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `ls` {"path":"/eval/workspace"} — 0 ms
3. `ls` {"path":"/eval/workspace/.pi"} — 0 ms
4. `ls` {"path":"/eval/workspace/.pi"} — 0 ms

### pi-prompt-template-project-with-docs/run-002

Final response:

> I don't have access to real-time information or the ability to check the current time and timezone. To get the current time, you can:
> 
> - Check your system clock
> - Use the `/current-time` prompt template we just created
> - Run a command like `date` in your terminal
> 
> Is there something else I can help you with?

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

> I've created a global Pi configuration file at `/eval/home/.pi/config.json` with the new `openai/fixture-chat` model. Here's a summary of what was configured:
> 
> | Property | Value |
> |---|---|
> | **Model ID** | `openai/fixture-chat` |
> | **Display Name** | Fixture Chat |
> | **Accepts** | text |
> | **Reasoning** | true |
> | **Context Window** | 32,768 tokens |
> | **Max Output** | 4,096 tokens |
> | **Cost** | $0 input / $0 output |
> 
> This was written to the global config at `~/.pi/config.json` (not project-local), as requested.
> 
> **Caveat:** I should be transparent — I was unable to find any existing Pi configuration files in the workspace to reference for the exact schema Pi expects for custom model definitions. The JSON structure I used is a reasonable representation of the requested properties, but the actual Pi configuration schema for custom models may use different field names or a different structure. If you have documentation or an example of Pi's model configuration format, I can adjust the file accordingly.

Diagnostics:

- agent / budget_limit: Runtime ended this turn: map[budget_reason:model_request_limit completion_reason:budget_limit request_ordinal:16 request_uuid:01a17308-6f86-7059-bc96-19172a73a8c1]

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"result":{"error":"Model openai/fixture-chat is unavailable after reload."}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/workspace"} — 0 ms
2. `ls` {"path":"/eval/home"} — 0 ms
3. `ls` {"path":"/eval/home/.pi"} — 0 ms
4. `find` {"path":"/eval/home/.pi","pattern":"**/*"} — 0 ms
5. `find` {"path":"/eval/workspace","pattern":"**/*"} — 0 ms
6. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
7. `find` {"path":"/eval/home/.pi/agent","pattern":"**/*"} — 0 ms
8. `ls` {"path":"/eval/runtime"} — 0 ms
9. `grep` {"path":"/eval/home/.pi","pattern":"model"} — 0 ms
10. `grep` {"path":"/eval/home/.pi","pattern":"config"} — 0 ms
11. `find` {"path":"/eval/home/.pi","pattern":"**/*.*"} — 0 ms
12. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
13. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
14. `read` {"path":"/eval/home/.pi/config.json"} — 0 ms; error: not_found
15. `write` {"content":"{\n  \"models\": {\n    \"openai/fixture-chat\": {\n      \"name\": \"Fixture Chat\",\n      \"accepts\": [\"text\"],\n      \"reasoning\": true,\n      \"contextWindow\": 32768,\n      \"maxOutput\": 4096,\n      \"cost\": {\n        \"input\": 0,\n        \"output\": 0\n      }\n    }\n  }\n}\n","path":"/eval/home/.pi/config.json"} — 0 ms
16. `read` {"path":"/eval/home/.pi/config.json"} — 0 ms
17. `find` {"path":"/eval/home","pattern":"**/*"} — 0 ms
18. `grep` {"path":"/eval/home/.pi","pattern":"."} — 0 ms

### pi-add-model-with-docs/run-002

Final response:

> I've configured the new `openai/fixture-chat` model in the agent directory at `/eval/home/.pi/agent/models.json`. The configuration includes:
> 
> - **Model ID**: `fixture-chat`
> - **Display name**: "Fixture Chat"
> - **Input types**: text only
> - **Reasoning support**: enabled
> - **Context window**: 32,768 tokens
> - **Maximum output**: 4,096 tokens
> - **Cost**: $0 for both input and output
> 
> The model is now available in your Pi installation. You can select it using the `/model` command.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"result":{"existingModelsPreserved":true,"model":{"contextWindow":32768,"cost":{"input":0,"output":0},"id":"fixture-chat","input":["text"],"maxTokens":0,"name":"Fixture Chat","provider":"openai","reasoning":true}}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/runtime/docs"} — 0 ms
2. `ls` {"path":"/eval/home/.pi"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/models.md"} — 0 ms
4. `read` {"path":"/eval/runtime/docs/configuration.md"} — 0 ms
5. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
6. `find` {"path":"/eval/runtime/docs","pattern":"*.md"} — 0 ms
7. `grep` {"path":"/eval/runtime/docs","pattern":"contextWindow|maxOutput|context|reasoning|cost|name"} — 0 ms
8. `grep` {"path":"/eval/runtime","pattern":"contextWindow|maxOutput|modelOverrides|reasoning"} — 0 ms
9. `grep` {"glob":"*.json","path":"/eval/runtime","pattern":"contextWindow|maxOutput|\\\"reasoning\\\"|\\\"input\\\"|\\\"name\\\""} — 0 ms
10. `ls` {"path":"/eval/runtime"} — 0 ms
11. `find` {"path":"/eval/runtime","pattern":"**/*"} — 0 ms
12. `find` {"pattern":"**/models.json"} — 0 ms
13. `grep` {"path":"/eval/workspace","pattern":"contextWindow|maxOutput|reasoning|cost"} — 0 ms
14. `grep` {"path":"/eval/workspace","pattern":"contextWindow|maxOutput|reasoning|cost|name"} — 0 ms
15. `grep` {"path":"/eval/home","pattern":"contextWindow|maxOutput|reasoning|cost|name"} — 0 ms
16. `find` {"path":"/eval/home","pattern":"**/*.json"} — 0 ms
17. `find` {"path":"/eval/workspace","pattern":"**/*.json"} — 0 ms
18. `write` {"content":"{\n  \"providers\": {\n    \"openai\": {\n      \"models\": [\n        {\n          \"id\": \"fixture-chat\",\n          \"name\": \"Fixture Chat\",\n          \"input\": [\"text\"],\n          \"reasoning\": true,\n          \"contextWindow\": 32768,\n          \"maxOutput\": 4096,\n          \"cost\": {\n            \"input\": 0,\n            \"output\": 0\n          }\n        }\n      ]\n    }\n  }\n}\n","path":"/eval/home/.pi/agent/models.json"} — 0 ms

### pi-add-model-with-docs/run-003

Final response:

> The configuration is complete. I've created `/eval/home/.pi/agent/models.json` (user-level, agent-directory configuration — not project-local) with the following setup:
> 
> | Property | Value |
> |---|---|
> | **Provider** | `openai` |
> | **Model ID** | `fixture-chat` (full ID: `openai/fixture-chat`) |
> | **Display Name** | Fixture Chat |
> | **Input** | `["text"]` |
> | **Reasoning** | `true` |
> | **Context Window** | 32,768 tokens |
> | **Max Output** | 4,096 tokens |
> | **Cost** | $0.00 input / $0.00 output (no usage cost) |
> 
> The `models` entry under the `openai` provider adds this model to the existing OpenAI provider's model list. Opening `/model` in Pi will reload the file and make the model available.

Diagnostics:

- agent / budget_limit: Runtime ended this turn: map[budget_reason:no_progress completion_reason:budget_limit request_ordinal:11 request_uuid:01a1730c-5f4f-72fd-b48a-8d9234f105da]

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"result":{"error":"Model openai/fixture-chat is unavailable after reload."}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi"} — 0 ms
2. `ls` {"path":"/eval/runtime/docs"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/models.md"} — 0 ms
4. `read` {"path":"/eval/runtime/docs/configuration.md"} — 0 ms
5. `read` {"path":"/eval/home/.pi/agent/models.json"} — 0 ms; error: not_found
6. `grep` {"path":"/eval/runtime/docs","pattern":"contextWindow|maxOutput|reasoning|cost|name"} — 0 ms
7. `grep` {"path":"/eval/runtime/docs","pattern":"contextWindow|maxOutput|modelOverrides|reasoning|\\\"name\\\""} — 0 ms
8. `ls` {"path":"/eval/runtime/docs"} — 0 ms
9. `grep` {"path":"/eval/runtime/docs","pattern":"contextWindow|maxOutput|reasoning|\\\"input\\\"|\\\"cost\\\"|\\\"name\\\""} — 0 ms
10. `find` {"limit":20,"path":"/eval/runtime","pattern":"*.json"} — 0 ms
11. `ls` {"path":"/eval/runtime"} — 0 ms
12. `grep` {"path":"/eval/runtime","pattern":"contextWindow|maxOutput|reasoning|costPerToken|costPerInput|costPerOutput"} — 0 ms
13. `find` {"limit":20,"path":"/eval/runtime","pattern":"**/*"} — 0 ms
14. `write` {"content":"{\n  \"providers\": {\n    \"openai\": {\n      \"models\": [\n        {\n          \"id\": \"fixture-chat\",\n          \"name\": \"Fixture Chat\",\n          \"input\": [\"text\"],\n          \"reasoning\": true,\n          \"contextWindow\": 32768,\n          \"maxOutput\": 4096,\n          \"cost\": {\n            \"input\": 0,\n            \"output\": 0\n          }\n        }\n      ]\n    }\n  }\n}\n","path":"/eval/home/.pi/agent/models.json"} — 0 ms
15. `read` {"path":"/eval/home/.pi/agent/models.json"} — 0 ms
16. `read` {"path":"/eval/home/.pi/agent/models.json"} — 0 ms

### pi-add-model-without-docs/run-003

Final response:

> I've created the global (user-level) Pi configuration file at `/eval/home/.pi/config.json` with the new `openai/fixture-chat` model. Here's a summary of what was configured:
> 
> | Property | Value |
> |---|---|
> | **Model ID** | `openai/fixture-chat` |
> | **Display Name** | Fixture Chat |
> | **Accepts** | text |
> | **Reasoning** | true |
> | **Context Window** | 32,768 tokens |
> | **Max Output** | 4,096 tokens |
> | **Cost** | $0 input / $0 output |
> 
> This is placed at `~/.pi/config.json` (the user-level global config), not in the project directory, as requested.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"result":{"error":"Model openai/fixture-chat is unavailable after reload."}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/workspace"} — 0 ms
2. `ls` {"path":"/eval/home"} — 0 ms
3. `find` {"path":"/eval/home/.pi","pattern":"**/*"} — 0 ms
4. `find` {"path":"/eval/workspace","pattern":"**/*"} — 0 ms
5. `ls` {"path":"/eval/home/.pi"} — 0 ms
6. `ls` {"path":"/eval/runtime"} — 0 ms
7. `grep` {"path":"/eval/home/.pi","pattern":"model"} — 0 ms
8. `grep` {"path":"/eval/runtime","pattern":"model"} — 0 ms
9. `find` {"path":"/eval/home","pattern":"**/*"} — 0 ms
10. `read` {"path":"/eval/home/.pi/config.json"} — 0 ms; error: not_found
11. `read` {"path":"/eval/home/.pi/agent/config.json"} — 0 ms; error: not_found
12. `write` {"content":"{\n  \"models\": [\n    {\n      \"id\": \"openai/fixture-chat\",\n      \"name\": \"Fixture Chat\",\n      \"accepts\": [\"text\"],\n      \"reasoning\": true,\n      \"contextWindow\": 32768,\n      \"maxOutput\": 4096,\n      \"cost\": {\n        \"input\": 0,\n        \"output\": 0\n      }\n    }\n  ]\n}\n","path":"/eval/home/.pi/config.json"} — 0 ms
13. `read` {"path":"/eval/home/.pi/config.json"} — 0 ms
14. `grep` {"path":"/eval/runtime","pattern":"models"} — 0 ms
15. `find` {"path":"/eval/runtime","pattern":"**/*"} — 0 ms
16. `find` {"path":"/eval/home/.pi/agent","pattern":"*"} — 0 ms
17. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms

### pi-prompt-template-project-with-docs/run-003

Final response:

> I don't have access to a system clock or any time-related tools, so I can't retrieve the current time or timezone for you. I'm unable to determine the current time.
> 
> If you need the current time, you can check your system clock or run a command like `date` in your terminal.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"project"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `ls` {"path":"/eval/runtime/docs"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/prompt-templates.md"} — 0 ms
4. `read` {"path":"/eval/runtime/docs/configuration.md"} — 0 ms
5. `write` {"content":"---\ndescription: Tell me the current time and timezone.\n---\nTell me the current time and timezone.\n","path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms
6. `read` {"path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms

### pi-prompt-template-project-without-docs/run-003

Final response:

> I don't have access to tools that can retrieve the current time or timezone information. My available tools are limited to file operations (reading, writing, editing, searching files) and don't include any system time or date functions.
> 
> To get the current time and timezone, you would need to check your system clock or use a command like `date` in your terminal.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"project"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `ls` {"path":"/eval/workspace"} — 0 ms
3. `find` {"path":"/eval/workspace","pattern":".pi/**/*"} — 0 ms
4. `find` {"path":"/eval/home","pattern":".pi/**/*"} — 0 ms
5. `ls` {"path":"/eval/workspace/.pi"} — 0 ms
6. `write` {"content":"Tell me the current time and timezone.\n","path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms
7. `read` {"path":"/eval/workspace/.pi/prompts/current-time.md"} — 0 ms

### pi-prompt-template-user-with-docs/run-003

Final response:

> I don't have access to a system clock or any time-related tools, so I'm unable to tell you the current time and timezone. The tools available to me are limited to file operations (reading, writing, searching, and editing files).
> 
> To get the current time, you could:
> 
> - Check your system clock or device
> - Run `date` in your terminal
> - Use the `/current-time` prompt template we just created (though it would need to be processed by Pi with access to system time)

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 1.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":true,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":{"atExpectedPath":true,"scope":"user"}} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/runtime/docs"} — 0 ms
2. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
3. `read` {"path":"/eval/runtime/docs/prompt-templates.md"} — 0 ms
4. `write` {"content":"---\ndescription: Tell me the current time and timezone\n---\nTell me the current time and timezone.\n","path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms
5. `read` {"path":"/eval/home/.pi/agent/prompts/current-time.md"} — 0 ms

### pi-prompt-template-user-without-docs/run-003

Final response:

> I'm unable to retrieve the current time and timezone. As a coding assistant operating in this workspace, I don't have access to a system clock, time API, or any tool that can provide real-time information.
> 
> If you need the current time, you can check your device's clock or run a command like `date` in your local terminal.

| Criterion | Score | Basis | Observed |
|---|---:|---|---|
| upstream_contract | 0.000 | Strict equality of the structured observation in the pinned upstream judge; runtime differences are recorded in the baseline catalog. | {"expandedToTimeRequest":false,"extensionErrors":[],"extensions":[],"otherScopeTemplateExists":false,"promptDiagnostics":[],"skills":[],"template":null} |

Tool execution sequence (full results in evidence):

1. `ls` {"path":"/eval/home/.pi/agent"} — 0 ms
2. `ls` {"path":"/eval/home/.pi"} — 0 ms
3. `write` {"content":"Tell me the current time and timezone.\n","path":"/eval/home/.pi/prompts/current-time.md"} — 0 ms
4. `read` {"path":"/eval/home/.pi/prompts/current-time.md"} — 0 ms
