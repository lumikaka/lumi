# GitHub Actions 手动评测

入口为 [Agent evals (shared pi and Lumi)](../../.github/workflows/agent-baseline.yml)，仅 `workflow_dispatch` 手动触发。push 和 PR 不会调用付费模型。本机不安装 pi、Docker，不进行 Rust 构建。

## 当前题集

- **共有对照 4 项**：`smoke`、`audit-match`、`audit-mismatch`、`audit-inconclusive`。
- **仅 Lumi 回归 4 项**：`batch-task`、`context-handoff`、`transient-tool`、`missing-resource`。

默认重复三次：Lumi 24 次、pi 12 次。已删除模型注册、Provider、扩展、提示模板和 TUI 模拟任务。旧 `--suite pi-reference` 在模型请求前报错；历史记录不改写。

共有对照测量真实 **Agent 核心**。pi 固定源码 `cfee633b1ba9497b0c12462da97a08b418b5ebf8`，使用 `pi-agent-core.Agent` 和原生 `pi-ai`；Lumi 继续执行 `agent.Service.ExecuteJob`。不是 pi 原始产品题集或两款完整应用的排名。

任务 JSON、数据、固定提示词和工具 schema 由 Go 命令导出为一份规范。pi 通过 JSONL 桥接调用同一 Go 环境和评分代码，不另写工具或判分副本。pi 每次独立进程和 Agent；Lumi 每次独立临时 SQLite。双方模型响应都来自真实服务。

## 配置与运行

1. 在 Actions 选择 **Agent evals (shared pi and Lumi)** → Run workflow。
2. 沿用 Secret `LUMI_EVAL_API_KEY` 和 Variable `LUMI_EVAL_BASE_URL`。地址包含 `/v1`，不含 `/chat/completions`；密钥不放在 workflow input 或代码里。
3. 默认模型 `qwen3.7-plus`、重复 3 次。地址留空使用仓库变量。

```sh
gh workflow run agent-baseline.yml --repo lumikaka/lumi \
  -f model=qwen3.7-plus -f repetitions=3
gh run list --repo lumikaka/lumi --workflow agent-baseline.yml --limit 5
gh run download <RUN_ID> --repo lumikaka/lumi \
  --name agent-evals-<RUN_ID>-1 --dir evals/results/github-<RUN_ID>
```

重复次数 1–10；每次固定模型及地址，超时 300 秒，每轮最多 16 次逻辑模型请求，输出上限 4,096 Token，省略 temperature 和 reasoning 参数。pi 的 315 秒外层超时用于回收失去响应的进程，并将其记为环境故障。整体作业上限 150 分钟，大批次可能先达到上限。

pi 与 Lumi 的原生重试、上下文组装、工具参数验证和收尾策略保留；pi 流式、Lumi 非流式。Provider 底层重试可能产生额外 HTTP 请求。两边依次执行、模型别名可能更新，因此差值只描述本次观察，不是统计显著的因果结论。

## 报告与证据

- `execution.json`：运行链接、Lumi 版本、固定配置和 8 项回归清单。
- `shared-protocol.json`、`shared-spec.json`、`shared-expected-runs.json`：共有任务、数据、提示词、工具、评分、配置、版本及重复计划。
- `shared-comparison.md/json`：13 个分项的有效数、均值、范围、标准差和差值；完整性不足时差值为 N/A。还包含逐次原始结果、耗时、Token 和异常。
- `lumi/`：完整原生报告、计划、result、trace、持久化会话 items。
- `pi/tasks/`：每次 result、控制台和 trace，含请求、原始流片段、最终消息、工具生命周期和评分。硬超时保留已有过程，绝不补造结果。
- `pi-runtime/`：源码锁、依赖锁、构建产物及源码散列、模型目录。仅在 GitHub 临时 runner 安装依赖并构建 chord/telemetry/ai/agent；不用完整桌面或 Docker 构建。
- `status.json`：Lumi 回归完整性；`shared-comparison.json.blocked`：共有对照完整性。
- `sha256-manifest.json`：上传文件散列；上传前检查密钥、`auth.json` 和符号链接。

分项 0–1 表示观察条件的满足程度，没有统一通过门槛。Agent 错误保留可观察评分；环境故障不计入分数、不补零，并使 CI 标记失败。审计的 evidence 分项只机械检查一次末尾提交及文件/函数引用，不保证解释语义正确。

Token 使用服务返回的 `prompt_tokens` / `completion_tokens`（包含已返回的缓存及推理 Token），缺失则 N/A。费用因没有冻结价格表而为 N/A，模型目录中的零价格不表示免费。模型调用会产生实际费用。

artifact 保留 14 天。取消或平台强制终止可能来不及上传，不能视为完整运行。下载后验证：

```sh
OUT=evals/results/github-<RUN_ID> python3 evals/ci/baseline.py verify
OUT=evals/results/github-<RUN_ID> python3 evals/ci/shared.py report
```

重建只重写派生的共有报告；长期归档请先复制到新目录。真实运行记录另存于 `evals/records/`，不把合成测试当作评测证据。

## 扩展及历史记录

先按 [用例扩展文档](../../docs/agent-evals.md) 添加通用 JSON 用例和可观察评分。若加入双方共有组，更新 `SharedCases`、`shared.SLUGS` 及对应清单测试；Go 桥接复用已有工具/评分。任何新增工具、原生 pi API 或配置差异应重新审阅并在报告列明。更新 pi 需显式更改 `pi-runtime-lock.json` 并验证新版本；不自动跟随 main。

[2026-10-25 旧双引擎记录](../records/2026-10-25-github-baseline/README.md) 包含已退役的专有任务，仅供历史复查。`baseline.py` 保留旧报告读取/重建和通用散列校验，`init` / `prepare` 拒绝旧执行入口。旧源码与构建配置见提交 `e90d14df6896f7bee94b0fad701932897026e12a`。

```sh
make test-evals
python3 -m unittest discover -s evals/ci -p '*_test.py'
node --check evals/ci/pi-shared.mjs
make eval-agent-list
```

以上离线检查不调用模型，也不运行 pi。
