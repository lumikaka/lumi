# pi evals 参考与适配清单

本表描述默认 14 例回归组。后续独立增加了 [pi 历史对照组](baselines/pi/README.md)，其原题来源、文档双组和已知差异单独记录；两组不混用成绩。

参考仓库：[earendil-works/pi packages/evals](https://github.com/earendil-works/pi/tree/38ff99177fba21e9c57a670f4e95adcf407c99c7/packages/evals)。固定提交 **38ff99177fba21e9c57a670f4e95adcf407c99c7**，提交时间 2026-10-24 20:48:11 UTC，研究日期 2026-10-25（Asia/Shanghai）。[reference.json](reference.json) 保存该目录所有文件的 Git blob SHA。源码经 GitHub 读取，未安装或运行 pi。

研究对象包括 README、`src/{cli,docker,harness,plan,report}.ts`、全部七类 `*.eval.ts`、`configured-runtime.ts`、`acme-server.ts`、Docker entrypoint/install-runtime、Vitest 配置及相应测试。实现以 Go 编写，没有复制 pi 的实现代码。

## 机制

| 参考机制与代码位置 | Lumi 适配与落点 | 状态及边界 |
|---|---|---|
| `evals/*.eval.ts` 平铺、suite/case identity、运行前发现 | `internal/agentevals/cases/*.json`，slug/suite，`--list`、`--filter`、`--cases`；校验重复身份与用例描述 | 已实现；JSON 数据定义替代 Vitest |
| `plan.ts` 预先展开完整 case × repetition × model cohort | `Plan` 在调用模型前保存 `protocol.json`、`expected-runs.json`；每次只冻结一个 Provider/model/config | 已实现；没有运行时模型切换 |
| `plan.ts` 重复、交替执行顺序 | `--repeat`；偶数轮反向用例顺序；每个任务重新初始化 | 已实现；无双臂时交替用例顺序减少简单顺序偏差 |
| `harness.ts` 新 session/workspace/home、固定 fixtures、工具 allowlist | `agent.NewIsolatedSession` 新建临时 app/project SQLite、线程和预算；`Environment` 持有独立虚拟文件、活动配置、故障计数器 | 已实现；跨用例及重复不共享状态；同例多轮保留上下文 |
| `harness.ts` 真实 Agent 核心执行 | `CreateTurn → ExecuteJob → buildContext / performChatModelRequest / prepareToolIntentBatch / executeTool / persistToolResult` | 已实现；共用生产循环、工具意图持久化、参数修复、无进展检测及预算收尾，不另写 Agent 循环 |
| system-prompt transform、隐藏控制元数据、凭据隔离 | 固定 `SystemPrompt`；冻结独立 prompt snapshot；模型只见 allowlist、fixture 和任务；评分定义只由宿主读取 | 已实现；API key 只进入真实 Provider 请求，不交给模拟工具；结果脱敏 |
| prompt/reload/prompt 输入序列 | `steps` 支持 prompt 和 reload，`eval_reload` 也可由 Agent 主动调用；模拟 runtime 只在 reload 后读取配置 | 已实现；reload 不重置会话 |
| 结构化结果检查、ToolCallJudge、自定义 judge | JSON 严格相等、最终文本、调用名称/参数/次数、加载后 probe、页脚正确性及 Levenshtein 保留度 | 已实现；分项独立 0–1，无全局门槛；只评可观察事实，不使用模型自评 |
| `configured-runtime.ts` 运行时验证而非只检查源文件 | 对实际加载的虚拟 registry/greeting/provider/footer 执行确定性 probe；校验保留原有 model | 已实现；模拟目标配置不会修改受测模型 |
| `acme-server.ts` 受控 API、流式协议与错误场景 | provider/stream 配置由虚拟 API probe 验证 endpoint、credential reference、protocol、model 和流事件映射 | 已实现为工具环境模拟；`pong` 是模拟服务返回值，不是 Agent 预设回答；受测 Agent 所有推理调用都是真实模型 |
| session JSONL、事件、耗时、usage、异常和结果附件 | 每个任务的 `trace.jsonl` 记录完整规范化模型请求/响应、步骤、工具结果、turn、末态；`result.json` 包含公开 UUID 的持久化 items、工具轨迹、评分、files/active configuration | 已实现；使用现有 LLM 诊断脱敏；无效 HTTP 响应只保留现有适配器提供的有界诊断，不宣称保留原始网络字节 |
| 单个失败后继续执行计划 | 逐任务隔离、recover、超时、错误分类；低分与 Agent 错误不阻断后续；中断后离线重建报告 | 已实现；强制结束进程可能留下部分 trace，未完成计划项明确缺失 |
| `report.ts` 缺失/重复/未评分阻断、未知指标不补零 | 协议与 trace 哈希校验、精确 cohort 校验、缺失/重复/非法分数检测；`null`/N/A；基础设施错误不计入得分均值 | 已实现；不完整组标记 complete=false，显示观测样本数，不伪造完整成绩 |
| 汇总多次表现和 observed flakiness | 每例每分项 n/mean/min/max/population SD、observed_variation；耗时和 Token 统计也保留有效样本数 | 已实现；一次运行 SD 不可用；两次相同不证明稳定性 |
| Markdown/机器可读报告、可重建 | `report.md`、`report.json`、`observations.jsonl`、`--report` | 已实现；不会将低分转换为进程失败 |
| vitest-evals host 与 docs 分离、手动入口 | `cmd/lumi_evals`、Makefile 独立命令；普通 `go test` 仅离线测试 | 已实现；单元测试的脚本模型不作为真实评测证据 |
| Docker 镜像、临时 HOME、降 UID、隐藏 npm/源码、consumer tarball 安装 | 本版不提供 OS 文件工具、shell、网络工具或执行 Agent 生成代码；虚拟文件只存在 map 中，评估宿主与评分规则不可被工具读取 | 原隔离机制的目标已适配；Docker/UID/npm 的具体实现不适用。这里是能力限制与状态隔离，不声称容器级恶意代码隔离 |
| with_docs/without_docs 双臂、精确配对、lift、饱和/负向差异标志 | 不建立双臂，不报告文档增益或 pi 对照成绩 | 不适用：本轮明确无需对照评测；仍保留单模型重复统计与完整 cohort 检查 |
| Pi auth 文件、扩展 JS、真实 TUI、源码/文档安装差异 | 通用 declarative fixture/API，不安装 pi、不调用 pi OAuth、不执行生成代码 | 专有环境不适用；对应行为见下表全部改写 |
| 多模型并行或排行榜 | 每次命令冻结一个模型，串行执行任务 | 不适用：用户要求单次固定同一模型及配置；可另开新输出目录独立运行 |

## 场景

每个 JSON 用例均包含目标、初始条件、可观察结果、步骤和各评分项的依据。原来的七类 suite 均有通用对应，新增恢复场景补足用户要求。

| 参考用例 | 通用用例 slug | 可观察评价 | 状态 |
|---|---|---|---|
| `smoke.eval.ts` | `smoke` | 无工具回答 Paris，真实 usage 在可获取时记录 | 已实现 |
| `documentation-audit.eval.ts` 对所有 pi 文档逐页审计 | `audit-match`、`audit-mismatch`、`audit-inconclusive` | 固定文档/实现/缺失依赖三类；读取资料、单次结构化 verdict、路径/符号证据 | 已实现；pi 全量文档树是专有资料，不搬入；机械证据分不等于语义完备性 |
| `models.docs.eval.ts` 添加模型且保留已有模型 | `add-model` | 已加载 registry 的全部字段、旧 model 保留、probe 调用 | 已实现 |
| `openai-provider.docs.eval.ts` 配置并调用 Provider | `configure-provider` | 加载后的模拟协议、model、credential reference 及请求/响应 | 已实现 |
| `custom-provider.docs.eval.ts` 从 API fixture 配置流式 Provider | `custom-stream-provider` | 模拟 event_stream 的文本字段和结束事件映射，完整 pong 与固定服务 usage | 已实现；服务 fixture usage 与受测模型 usage 分开保存 |
| `extensions.docs.eval.ts` 创建、重载、调用 hello | `extension-reload` | 配置生效、Bob 参数工具调用、真实模型最后回答 Hello, Bob! | 已实现为声明式 greeting runtime |
| `tui.docs.eval.ts` 替换 context bar 并保留其余状态行 | `status-footer` | 42.2%、65%、120% 三 fixture，十格 bar、四舍五入、clamp、其余文字 Levenshtein 最低相似度 | 已实现为模拟 renderer；不测试真实终端/React 绘制 |
| 多步 prompt/reload 模式扩展 | `context-handoff` | 三轮保存、纠正、读取，code 保留、destination 更新 | 已实现 |
| Lumi 补充：依赖工具结果完成任务 | `batch-task` | 读取两个来源、计算、写入、再读取验证、来源文件不变 | 已实现；不强制模型将调用放在同一个 batch |
| Lumi 补充：暂时工具故障 | `transient-tool` | 第一次失败、重试同一工具、第二次返回真实 fixture 值 | 已实现；受控故障属于任务条件，不记环境故障 |
| Lumi 补充：永久缺失 | `missing-resource` | not_found 后诚实回答 UNAVAILABLE，单次尝试 | 已实现 |
| Lumi 补充：配置异常修复 | `repair-config` | 先 reload 收到错误、修复严格 JSON、再次加载、调用 | 已实现 |

参数越界、未知工具、生产 Provider 响应修复、缺失 telemetry、重复/丢失证据、Agent 低分与环境错误区分另有离线回归测试。运行时遇到的 Provider 连续无效输出会原样记录，不通过伪造模型响应在真实评测中强制制造。
