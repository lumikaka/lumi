# Lumi / pi remote baseline

Requested model: `qwen3.7-plus`. Repeats per variant: 3.

Descriptive comparison of adapted tasks. A matching model ID does not establish Agent-core equivalence.

| Case | Lumi success / scored (planned) | pi success / scored (planned) |
|---|---:|---:|
| pi-add-model-with-docs | 0/0 (3); N/A | 0/0 (3); N/A |
| pi-add-model-without-docs | 0/0 (3); N/A | 0/0 (3); N/A |
| pi-prompt-template-project-with-docs | 0/0 (3); N/A | 0/0 (3); N/A |
| pi-prompt-template-project-without-docs | 0/0 (3); N/A | 0/0 (3); N/A |
| pi-prompt-template-user-with-docs | 0/0 (3); N/A | 0/0 (3); N/A |
| pi-prompt-template-user-without-docs | 0/0 (3); N/A | 0/0 (3); N/A |

Score, duration and token mean/min/max/stddev, metric availability, per-run statuses and evidence paths are in [comparison.json](comparison.json).

[Lumi native report](lumi/report.md).

## Evidence / infrastructure issues

- pi evidence unavailable or invalid: Expected exactly one pi cohort, found 0.
- pi runner exit status 1; inspect console and native results
- lumi evidence unavailable or invalid: [Errno 2] No such file or directory: '/home/runner/work/_temp/agent-baseline/lumi/report.json'
- lumi runner exit status unavailable

## Interpretation and remaining differences

- Same endpoint and requested model ID; the service may route a mutable model alias to different backend revisions.
- The pinned pi source hydrates its ignored built-in model catalog from live upstream catalogs before building; the actual generated catalog and hashes are frozen in pi-model-catalog/.
- Original pi tasks/judges run against real pi resources; Lumi uses the reviewed virtual filesystem/runtime adaptation.
- System prompts, tool schemas/results, and available documentation differ. This is not an isolated Agent-core experiment.
- pi uses streaming Chat Completions; Lumi uses non-streaming Chat Completions. Both omit temperature and reasoning request fields.
- Both cap output at 4096 tokens. pi keeps its native 300-second test timeout and native retry/loop policy; Lumi has 300 seconds per case and 16 requests per turn, including its no-progress policy.
- pi errored/unscored records have no automatic Agent/environment attribution; inspect native Vitest errors and sessions. They are never silently counted as zero.
- No price snapshot is configured. Cost is unavailable; zero model catalog prices only disable estimation and do not mean free inference.
- Three repeats describe observed variation, not statistical significance. Scores do not gate infrastructure acceptance.
