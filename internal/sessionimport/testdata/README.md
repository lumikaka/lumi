# Rollout fixtures

Recorded on 2026-10-31, macOS, Codex Desktop `26.1027.30824` (22943), bundled runtime `0.166.0-alpha.5.1`.

`desktop-recorded.jsonl` is a **sanitized, reduced recording**, not an invented format or a full transcript. Its session metadata, user-message shape, assistant `final_answer` shape and `task_complete` shape came from the same-project App rollout `rollout-2026-10-31T18-01-56-01a19168-a300-7af1-a0ea-4fe8e03b75b4.jsonl`. The `commentary` shape came from the current App rollout `rollout-2026-10-31T20-13-57-01a191e1-82ac-7602-8313-fc8be7100d07.jsonl`. The final response in the first source matched its last `task_complete.last_agent_message` exactly (19 characters).

All message text is replaced with a fictional fox story; the session UUID is replaced and cwd is `PROJECT_ROOT`, substituted by tests. Account/user identifiers, encrypted content, instructions, git metadata and passthrough metadata are removed. The fixture combines selected records from two sessions for adapter coverage; it does not establish hook timing. No complete real rollout is copied into the repository.

`synthetic-edges.jsonl` is explicitly **synthetic**. It covers direct Lumi calls, namespace-qualified calls, unrelated commands, arguments/output, images, reasoning, injected instructions, interruption, compaction and unknown records. None of its hook or tool behaviors are claimed as live App observations. `DO_NOT_STORE_*` markers must never reach imported history.

Service tests split the recorded fixture, append its final records, replay it, add a partial final line, move/truncate/rewrite the file, and check pagination and project ownership.
