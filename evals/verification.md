# Agent evals 验证记录

> 历史记录说明：下文为 2026-10-25 题集的观察。相关 pi 专有模拟任务已退役，不作为当前产品能力结论；新题集与真实对照见 [2026-10-26 记录](records/2026-10-26-shared-agent-benchmark/README.md)。

日期：2026-10-25（Asia/Shanghai）。参考版本及适配范围见 [adaptation.md](adaptation.md)。

## 代码检查

- `go test ./...`：通过，覆盖项目全部 Go 包，包括 Agent、jobqueue、HTTP API、LLM 及新评测模块。
- `go vet ./internal/agentevals ./internal/agent ./cmd/lumi_evals`：通过。
- `git diff --check`：通过。
- `make eval-agent-list`：列出 14 个用例，无模型请求。
- 本轮未运行 Cargo、Rust 编译、检查或测试。

[检查清单及源文件哈希](records/checks/checks.json)；[完整 Go 测试输出](records/checks/go-test.txt)。

离线测试覆盖真实 ExecuteJob 的注入边界、跨重复隔离、多轮持久化上下文、参数校验错误回传与修复、模型响应结构重试、生产预算停止分类、临时工具故障、Provider 网络错误、panic 清理、未知 usage、低分、重复/缺失/改变的证据、评分波动，以及无参数工具的合法 JSON Schema。测试里的脚本模型只验证这些机制，不计为真实 Agent 评测。

## 真实模型执行

已完成首轮 14 个用例的完整真实执行：13 次正常结束、1 次 Agent 任务失败，首轮没有环境故障。主计划共执行 28/28 次，随后对两个受 Provider 故障影响的用例各补跑 2 次。

| 记录 | 计划/已执行 | 正常结束并评分 | Agent 错误并评分 | 环境故障、不可评分 |
|---|---:|---:|---:|---:|
| [主批次](records/2026-10-25-qwen3.7-plus/report.md) | 28/28 | 25 | 1 | 2 |
| [补跑](records/2026-10-25-qwen3.7-plus-recovery/report.md) | 4/4 | 2 | 0 | 2 |

两个受影响用例的最后一次补跑均成功。因此每个用例都有两条可评分的真实观察，共 28 条；另保留 4 条不可评分的环境故障。累计 32 次任务执行、161 次模型请求，以上数字不包含先前开发探索及其他连接的冒烟检查。

两份报告各自仍保留环境缺口和非零退出码，不把补跑成绩覆盖或合并进原 cohort。此处验收的是完整首轮、重复执行、隔离、记录与评分机制，不宣称每个计划 cohort 都无环境故障，也没有设置 Agent 通过门槛。[结构化验收证据](verification.json) 逐例列出可评分观察路径，并校验主批次与补跑的模型配置、基础提示词、用例、模拟环境及评分规则一致。

主批次 26 条可评分观察的已知输入/输出 Token 小计为 130,455 / 19,743；补跑中 2 条可评分观察为 18,302 / 1,979。环境失败执行的完整 Token 用量不可获取，因此这些是已知小计，不是总消费量；成本不可用。任务墙钟耗时小计分别约 803.9 秒、373.3 秒。

`repair-config` 的第一次因重复诊断及不可靠预算收尾失败，第二次成功，报告观察到分项波动。详见 [表现证据和改进建议](findings.md)。四条环境失败包括两次用例超时及两次 Provider 连接中断，最新脱敏诊断保留了 `unexpected EOF`；它们没有被算作 Agent 零分。

主批次结束后仅规范化了诊断字段，并将缺乏明确 Agent 证据的未知运行时错误保守归为环境问题。补跑使用最终记录器；生产 Agent 核心、LLM 适配器、用例、模拟环境和评分代码均与主批次一致，哈希已核验。

两份报告均已用 `--report` 离线重建，`report.json` 与重建前完全一致，protocol/result/trace 文件哈希保持不变。所有 161 条模型请求均使用记录的同一模型、固定基础提示词（预算收尾时追加生产固定指令）及 eval_ 工具；没有请求凭据字段。静态凭据模式检查未发现敏感值。

主批次命令：

```sh
go run ./cmd/lumi_evals \
  --lumi-config-dir /Users/qingyang/.lumi_dev \
  --repeat 2 --timeout 300 \
  --out evals/records/2026-10-25-qwen3.7-plus
```

补跑通过 `--cases` 选取完全相同的 `custom-stream-provider.json` 和 `extension-reload.json`，其余模型及预算参数保持不变；完整用例定义嵌入补跑 protocol，无需临时目录即可复查。

模型来自本机已配置百炼，固定 `qwen3.7-plus`。命令仅读取 Provider 配置，不读取或修改用户项目，凭据不保存在记录中。

## 开发阶段观察

1. [探索记录](records/2026-10-25-qwen3.7-plus-exploratory/README.md)：add-model 的可见文档未说明 input 数组类型，模型使用字符串产生机械零分。此证据不足以归因 Agent，已补全字段类型说明并重新运行。原记录保留，剩余 cohort 缺失和主动取消均明确呈现。
2. [协议检查记录](records/2026-10-25-qwen3.7-plus-schema-preview/README.md)：修正空 required 从 null 到 []，提升标准 JSON Schema 兼容性；增加回归测试。该部分运行不冒充完整验收批次。
3. 预算测试发现生产运行器可将预算收尾存为 completed。评测从 completion_reason/budget_reason 识别并报告 agent_error，保留分项评分，不修改 Agent 的生产预算行为。
