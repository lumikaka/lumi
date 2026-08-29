---
name: diagnose-chat-thread
description: 定位并排查 Lumi Chat Thread 的本地运行错误。用户输入 `chat_thread_uuid={UUIDv7}`，并询问“为什么报错”“失败原因”“查看日志”“排查对话”或类似问题时使用；也用于只凭 Chat Thread UUID 跨开发/生产项目定位 `project.sqlite`，汇总 Run、Tool/API、事件和 LLM Logs 证据。
---

# 排查 Chat Thread

对本地数据库执行只读诊断。不要修改数据库，不要把内部 bigint `id` 暴露给用户。

## 快速开始

1. 从用户输入提取 `chat_thread_uuid`，保留原始 UUIDv7。
2. 运行本 Skill 目录下的脚本：

   ```bash
   python3 scripts/diagnose_chat_thread.py '<chat_thread_uuid>'
   ```

   从其他工作目录运行时，使用相对于本 `SKILL.md` 的脚本绝对路径。
3. 如果用户同时给出项目目录，用 `--project-root '<absolute_project_root>'` 跳过跨项目定位。
4. 仅当摘要不能解释问题、且确实需要核对模型原始请求时，追加 `--include-payloads`。不要在回答中完整转贴 Prompt、Response、用户内容或可能的敏感信息。

脚本依次读取 `LUMI_DATA_DIR`、`DATABASE_DSN`、`~/.lumi_dev/lumi.sqlite` 和 `~/.lumi/lumi.sqlite` 中的最近项目索引，再以 SQLite `mode=ro` 打开候选项目的 `project.sqlite`。索引未命中时，最后检查默认 `~/Documents/Lumi/*/project.sqlite`。

## 判定顺序

按以下顺序分析脚本 JSON；不要只看 `llm_logs`：

1. 检查 `tools[].api_error`、`tools[].error_code`。Tool Execution 即使标记为 `completed`，其 API 信封仍可能是 `success:false`。
2. 检查 `runs[].error_code`、`runs[].limit_reason`、`runs[].no_progress_streak` 和 `notable_events`。重点识别 `run_budget_exhausted`、重复工具调用、取消和中断。
3. 检查 `model_requests[].status`、`error_code`、`http_status`、`provider_error_code`。只有这里存在失败证据时，才归因于 LLM 或 Provider。
4. 用时间、`run_uuid`、`request_ordinal` 和公开 Tool UUID 串起因果顺序。不要使用内部数据库 ID 作为证据标识。

如果 Lumi 服务正在运行，且已经获得 `project_uuid`，也可以用公开接口交叉验证：

```text
GET /api/v1/projects/{project_uuid}/chat_threads/{thread_uuid}/trajectory?limit=200
```

该接口的 `model_requests` 来自该线程的 `llm_logs`，`tools` 包含 Tool/API 结果。需要某条模型请求的原始详情时，再读取：

```text
GET /api/v1/projects/{project_uuid}/llm-logs/{model_request_uuid}
```

## 回答要求

先给出最可能的直接原因，再简要列出证据：

- 项目名称和公开 `project_uuid`
- `chat_thread_uuid` 与相关 `run_uuid`
- 首个确定失败点及错误码/安全错误消息
- 后续连锁结果，例如重试、无进展预算耗尽或最终化
- 明确说明问题属于 LLM/Provider、Tool/API、任务执行还是运行预算

如果证据不足，说明缺少什么；不要猜测。用户只要求解释时不要修改代码，用户明确要求修复后再定位对应实现并改动。
