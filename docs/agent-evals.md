# Agent 行为评测

评测使用当前 Lumi Agent 核心、一个固定真实模型、固定提示词和模拟工具。它与业务系统提示词、绘本数据、业务工具和应用队列隔离。当前保留 8 个通用用例，来源、适配与移除原因见 [适配清单](../evals/adaptation.md)。

2026-10-26 已移除依赖 pi 专有产品功能的 6 个默认用例和全部 6 个 `pi-reference` 变体。[pi 历史基线](../evals/baselines/pi/README.md) 仅供旧记录离线复查；新增共有对照组为 smoke 与三类文档审计，双方共享资料、任务、固定提示词、Go 工具实现与评分规则，在 GitHub Actions 分别调用真实 Agent 核心。

## 手动运行

要求能运行本项目 Go 命令，且模型支持工具调用。无需前端、Hugo、Docker 或 Rust。普通 `make test` 不消耗模型额度。

```sh
# 枚举用例，不调用模型
make eval-agent-list

# 只运行离线回归测试
make test-evals

# 使用已有 Lumi 配置；仅复制 Provider 设置到临时库读取，源库只读
make eval-agent EVAL_ARGS='--lumi-config-dir /absolute/path/to/.lumi_dev --repeat 2'

# 可明确选择一个已配置 Provider 及模型
# Provider 可选 aliyun_bailian、cloudflare_ai_gateway、openai_relay
make eval-agent EVAL_ARGS='--lumi-config-dir /absolute/path/to/.lumi_dev --provider aliyun_bailian --model qwen3.7-plus --repeat 2'
```

也可使用专用环境变量，避免依赖已安装 Lumi。密钥只用环境变量传入，不支持命令行密钥参数，不自动读取 `.env`。

```sh
export LUMI_EVAL_PROVIDER=openai_compatible
export LUMI_EVAL_BASE_URL=https://your-provider.example/v1
export LUMI_EVAL_MODEL=your-tool-capable-model
# 用你的密钥管理方式设置 LUMI_EVAL_API_KEY
make eval-agent EVAL_ARGS='--repeat 2 --out evals/results/my-run'
```

`openai_compatible`/`aliyun_bailian` 使用现有 Chat Completions 适配器；`openai_relay`/`cloudflare_ai_gateway` 使用现有 Responses 适配器。不能把 `--lumi-config-dir` 与显式 endpoint/key 混用，以免凭据误投其他端点。模型设置在整次运行开始时读取一次，后续每例和每轮都使用同一配置。

| 参数 | 默认 | 含义 |
|---|---|---|
| `--suite` | `regression` | 当前 8 例；`pi-reference` 已退役，传入会在调用模型前报错 |
| `--repeat` | 1 | 每例重复次数，1–100；偶数轮反向用例顺序 |
| `--filter` | 空 | slug 或 suite 子串，例如 `audit`、`recovery`、`smoke` |
| `--cases` | 内嵌首批用例 | 读取指定目录的平铺 JSON 用例 |
| `--out` | `evals/results/<UTC timestamp>` | 必须是新目录，拒绝覆盖已有评测 |
| `--timeout` | 180 | 每例总时限，含全部 prompt 步骤 |
| `--max-requests` | 16 | 每轮用户输入允许的模型请求数，包含生产运行器保留的最后收尾请求 |
| `--max-tokens` | 4096 | 每次请求输出上限 |
| `--temperature` | 不传 | 由 Provider 默认处理；指定后冻结记录；现有 LLM 适配器对不支持 temperature 的 reasoning 模型可能省略该字段，trace 保存适配器收到的规范化请求；不保证省略的参数实际发送到网络 |
| `--report` | 空 | 从已有证据重建报告，不调用模型 |
| `--compare` | 空 | 读取已有运行目录，对照历史基线；不调用模型、不修改原记录 |
| `--baseline` | `evals/baselines/pi/catalog.json` | `--compare` 使用的历史目录；输出由 `--out` 指定 |

按 Ctrl-C 取消当前请求，已有记录保留，未执行项目显示缺失。强制杀进程可能只有部分 trace；离线重建仍会报告缺口。重新跑评测应使用新目录，不能把重试覆盖到旧证据中。

```sh
make eval-agent-report run=evals/results/my-run
# 或
 go run ./cmd/lumi_evals --report evals/results/my-run
```

退出码：0 表示已完整记录计划 cohort（允许低分和 Agent 任务失败）；1 表示存在环境故障或缺失/重复/损坏证据；2 表示配置、用例、输出或命令错误。使用 `go run` 时 Go launcher 将被运行程序的非零退出码映射为自身退出码 1，并打印 `exit status N`；需要直接使用上述退出码的脚本可先 `go build -o build/lumi_evals ./cmd/lumi_evals`。

## 隔离与执行边界

`agent.NewIsolatedSession` 每个 `(case, repetition)` 建立独立临时 app/project SQLite 和线程。它不连接已有项目 manager、不导入用户项目、没有 River worker。创建 turn 后同步调用真实 `Service.ExecuteJob`，共用生产上下文重建/压缩、批量工具意图持久化、tool-call 配对、参数修复、Provider 响应重试、预算/无进展处理和最终落库逻辑。

注入仅经独立构造器启用，不提供 HTTP 或应用设置开关。固定 system prompt 保存在该临时会话的 prompt snapshot；业务 prompt、路由文档、图像工具、request_api、工作流和用户交互工具不进入模型工具列表。工具名以 `eval_` 开头；只保留文件读写、列表、搜索、审计提交及记录获取。

同一次运行的模型及参数固定不变。历史记录保存了当时的文档、提示词和用例定义；旧记录可以读取，但不再通过当前执行入口重新运行。

虚拟环境是每例新建的内存 map，`eval_read/write/list/search` 不能访问宿主文件；路径必须为规范相对路径。没有 shell、网络、任意代码执行、配置加载器或读取评分规则的工具。`eval_fetch` 使用独立 fixture，可模拟暂时故障和记录缺失；受测模型仍经 Lumi LLM 客户端真实调用。

执行结束时先导出完整记录，再关闭并移除临时 SQLite。模型凭据不进入 prompt/工具参数/协议文件，现有请求与响应 snapshot 编码器负责脱敏。开发者修改工具环境代码属于受信任操作；此机制不声称可以安全执行任意恶意 Go/JS 或替代操作系统容器。

## 输出与结果解释

```text
<run>/
  protocol.json             # UUIDv7、参考版本、固定模型配置、prompt、完整用例、源文件哈希、协议 digest
  expected-runs.json        # 调用模型前已确定的全部任务
  tasks/<slug>/run-NNN/
    trace.jsonl             # 即时写入并同步的步骤、模型请求/响应、工具事件和结束状态
    result.json             # 分项评分、问题、耗时、usage、turn、持久化 items、工具过程和文件末态
  observations.jsonl        # 每个可归属任务的一行结构化结果
  report.json               # 单次结果及每例/分项重复统计
  report.md                 # 人可读报告及逐次证据链接
```

源文件、二进制及协议 hash 用于区分代码/用例版本；结果中的 trace hash 用于检测记录缺失或变化。它们提供本地审计一致性，不是防篡改签名。JSON 中所有字段为 snake_case，任务按 case_slug/repetition 标识；数据库内部 ID 不导出，持久化实体用公开 UUID。原始模型协议中的工具 call ID 保留供配对。

- `scored`：Agent 运行完毕，按实际观察评分；可以是 0 分。
- `agent_error`：任务因预算、工具协议/输出等 Agent 侧问题未正常结束；保留可观察的分项评分与原因。
- `environment_error`：真实 Provider 连接、鉴权、限流、传输失败、总时限/取消，或 harness 初始化、存储、清理、panic 等问题。分数不可用，不把它们当作模型零分。
- 用例故意制造的暂时不可用、not_found是任务条件；是否恢复属于 Agent 表现。Provider 返回可解析诊断但不可用的模型输出归为 Agent 输出问题，传输中断/读取错误归为环境问题。此分类描述观察到的故障层，不断言 Provider 内部根因。
- 分项使用严格相等、参数/次数匹配及文档审计证据检查。没有统一通过线、跨场景总分或发布门禁。文档审计的解释引用是机械检查，不保证所有自然语言论证都正确。
- 重复统计按分项展示有效样本数、均值、最小/最大、总体标准差。分数不同会标记 `observed_variation`。缺失/重复/环境错误造成 `complete=false`，均值仅描述有效观察；一次或两次同分不能证明长期稳定。
- 输入、输出、缓存 Token 来自实际 Provider；任一请求缺失对应用量，该次执行相应总量就是 `null`/N/A，不把部分已知数当成完整总量。没有冻结价格表，成本标记不可用。耗时是本机墙钟时间，含模型等待。
- trace 保留规范化请求、响应、完整工具输入输出；对无法正常解析的 Provider HTTP 内容，仅保存现有适配器的有界诊断及截断标记。不可获取的原始网络内容不伪造补全。

## 扩展用例

在 `internal/agentevals/cases/` 添加一个 JSON 文件，或用 `--cases` 加载自定义目录。参考最小用例 `smoke.json` 和多步用例 `context-handoff.json`。

必填 `slug`（小写字母/数字/连字符且唯一）、`suite`、`goal`、`initial_conditions`、`observable_result`、`steps`、`checks`。`reference` 记录来源；`files` 为可见虚拟文件；`tools` 是 allowlist；`faults` 可指定某工具首次若干调用返回 transient_unavailable。步骤只支持 `{"type":"prompt","content":"..."}`。已移除 reload/probe、模板展开和专有资源评分；旧用例不能通过 `--cases` 重新启用。

每个 check 必须有唯一 `name`、`kind`、可解释的 `basis`：

| kind | 判据 |
|---|---|
| `final_equals` / `final_contains` | 最后一轮最终回答（equals 去首尾空白） |
| `file_json` / `file_equals` | 文件末态严格比较，JSON 允许键序不同但不允许额外字段 |
| `tool_call` | 名称与参数子集匹配；`count` 指定准确次数，省略为至少一次；实际执行中返回错误也计数；参数校验拒绝保留在 items/trace，不算工具执行 |
| `audit_evidence` | 一次审计提交中引用固定通用实现路径与符号，并且是最后一次工具调用，专用于内置审计 fixture |

评分规则在宿主执行，不会加入模型上下文；工具成功宣称或最终文字不能替代文件和工具记录验证。增加新模拟工具需修改 `environment.go` 的 schema、Validate 和 Execute；新评价方式需修改 `Case.Validate` 与 `grade.go`，并加有意义的故障/边界回归测试。不要在工具里读取真实项目或执行模型生成代码。

用脚本模型做 harness 单元测试只验证管道、隔离、评分和故障处理。`lumi_evals` 运行入口固定构造真实 `llm.OpenAICompatibleClient`，不能通过参数替换成预设响应。交付证据必须来自真实命令。

## 首批记录

[GitHub Actions 手动评测](../evals/ci/README.md) 运行 8 个 Lumi 通用用例，并用真实 pi Agent 核心执行其中 4 项共有任务。沿用 `agent-baseline.yml` 的手动入口，已替换旧专有任务执行逻辑。新旧报告分别保存，不混用成绩。

见 [2026-10-25 qwen3.7-plus 报告](../evals/records/2026-10-25-qwen3.7-plus/report.md) 和该目录 `protocol.json`。完整运行的检查结果、环境故障补跑、表现观察及改进建议记录在 [验收记录](../evals/verification.md) 中。后续修复 Agent 能力时应新建独立运行目录，保留本次基线。
