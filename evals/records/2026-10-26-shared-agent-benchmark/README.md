# 2026-10-26 共有 Agent 核心基准

本批模型固定 **qwen3.7-plus**，每项重复 **3 次**。Lumi 运行 8 项通用回归，其中 smoke 和三类文档审计同时交给真实 pi Agent 核心执行。构建和模型调用均在 GitHub 临时 runner 完成。

## 完整运行与观察

[运行 40276748826](https://github.com/lumikaka/lumi/actions/runs/40276748826) 成功；源码 `51c1f80be906d7031b92bc849b7a124c19a27422`，pi 固定 `cfee633b1ba9497b0c12462da97a08b418b5ebf8`。

[共有对照报告](40276748826/shared-comparison.md) · [结构化对照](40276748826/shared-comparison.json) · [8 项 Lumi 报告](40276748826/lumi/report.md) · [完整计划与配置](40276748826/shared-protocol.json)

| 范围 | 计划 / 记录 | 所有分项为 1 的次数 | 环境故障 |
|---|---:|---:|---:|
| Lumi 全部 8 项 | 24 / 24 | 23 | 0 |
| Lumi 共有 4 项 | 12 / 12 | 11 | 0 |
| pi 共有 4 项 | 12 / 12 | 12 | 0 |

以上是描述性计数，没有统一通过门槛。13 个共有分项中，仅 `audit-match / evidence` 有观察差异：Lumi 均值 2/3，pi 均值 1；其余 12 项双方均为 1。这是小样本结果，不能推断完整产品或总体 Agent 能力排名。

### 具体能力问题

Lumi 的 [audit-match/run-001 结果](40276748826/lumi/tasks/audit-match/run-001/result.json) 与 [完整过程](40276748826/lumi/tasks/audit-match/run-001/trace.jsonl) 显示：它读过要求的两个文件，给出正确的 match 结论，也准确解释 `format_name → normalize → strip().lower()`；但提交的 explanation **没有写出 `src/names.py` 路径**，未满足任务要求。该分项原样记为 0，后两次为 1。它不是审计逻辑错误，也不是环境故障。

建议后续独立验证提交前对输出约束的复核，检查用户要求的文件/函数引用是否齐全。这次保留真实失分，没有改 Agent 核心、改判据或覆盖重跑来提高成绩。审计 evidence 仍只是机械引用检查，不能单凭它证明解释语义正确。

### 请求与资源指标

共有任务 Lumi 33 次模型请求，pi 35 次。输入/输出 Token 分别为 Lumi **26,171 / 5,685**、pi **32,833 / 6,492**。pi 从原始流 usage 取 prompt_tokens/completion_tokens，与 Lumi 使用的字段一致；费用因无冻结价格表而不可用。

共有任务执行耗时合计 Lumi **132.579 秒**、pi **156.508 秒**；每个用例的范围和标准差见报告。执行耗时不含构建；pi 也不含 Node 模块导入时间。两边依次执行，保留不同的上下文、重试、收尾以及流式/非流式实现，不能据此断言固定速度优势。

## 验证与归档

两次尝试各有 144 个封存文件，均通过逐文件 SHA-256 验证。第二次的所有 68 个共有任务模型请求已逐一核验模型、max_tokens、temperature/reasoning 参数省略策略；24 次共有执行的首个提示词和工具定义与共用规范一致。

原始报告只在临时副本重建：任务观察、分数和完整性结果相同，共有 Markdown 逐字节相同；Linux 与本机标准差计算只有小于 1e-12 的浮点末位差异，已逐项记录，不改写原始文件。见 [validation.json](validation.json)。全部归档约 7.3 MiB，无本地 pi 依赖、Docker 镜像或 Rust 构建。

ZIP 散列、运行版本和 artifact 标识见 [attempts.json](attempts.json)。下列校验不调用模型：

```sh
OUT=evals/records/2026-10-26-shared-agent-benchmark/40276748826 python3 evals/ci/baseline.py verify
```

## 首次尝试：启动适配故障

[运行 40275948766](https://github.com/lumikaka/lumi/actions/runs/40275948766)，源码 `2945231d2a9ce02e5f1cf22ed3f7a9d2ee3cd742`。Lumi 完成 24 次，所有机械评分项均为 1；pi 的 12 次均在模型请求前因 ESM 包解析失败，记为 `environment_error`、不计分，不能据此比较能力。

[原始共有报告](40275948766/shared-comparison.md) · [Lumi 报告](40275948766/lumi/report.md) · [pi 启动错误](40275948766/pi/tasks/smoke/run-001/console.txt)

修复为从已核验源码的编译路径导入 ESM，并增加付费调用前的真实模块加载检查。完整运行是新建的独立 cohort；未将首轮 Lumi 满分拼接到第二轮 pi 结果，也未删除首轮环境错误。
