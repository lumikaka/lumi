# 在 GitHub Actions 运行 Lumi / pi 对照

[手动工作流](../../.github/workflows/agent-baseline.yml) 在 GitHub 的 Ubuntu 临时机器上运行两边的真实 Agent。本机不安装 pi、不构建 Docker 镜像、不编译 Rust；只需要下载结果包。工作流不会随 push 或 PR 自动调用付费模型，也不会发布镜像。

## 首次配置

1. 将工作流及依赖代码合入仓库默认分支。GitHub 要求 `workflow_dispatch` 工作流先存在于默认分支，之后才能选择其他已包含工作流的分支运行。
2. 在仓库 Settings → Secrets and variables → Actions 中设置 Secret `LUMI_EVAL_API_KEY`，值为模型 API Key。设置 Variable `LUMI_EVAL_BASE_URL`，值为实际使用的 OpenAI-compatible API 地址，包含 `/v1`、不包含 `/chat/completions`。必须与本地评测的服务地址一致，不能只对齐模型名称。
3. Actions → **Agent baseline (Lumi and pi)** → Run workflow，选择模型及重复次数。默认 `qwen3.7-plus`、3 次；地址输入框留空时使用上述仓库变量。

也可使用已登录的 GitHub CLI：

```sh
gh workflow run agent-baseline.yml --repo lumikaka/lumi \
  -f model=qwen3.7-plus -f repetitions=3

gh run list --repo lumikaka/lumi --workflow agent-baseline.yml --limit 5
gh run watch <RUN_ID> --repo lumikaka/lumi
gh run download <RUN_ID> --repo lumikaka/lumi \
  --name agent-baseline-<RUN_ID>-1 --dir evals/results/github-<RUN_ID>
```

重新运行同一个 GitHub run 时，artifact 名最后一位改为 attempt 编号。首次建议保留默认三次，共 **pi 18 次 + Lumi 18 次**。模型调用会产生实际 API 费用，工作流没有价格表，费用显示 N/A；以服务商账单为准。

当前适配面向普通 Chat Completions、文本和工具调用，推理字段与 temperature 均不发送。其他协议、必须指定推理开关的模型，以及超过 262,144 token 的上下文描述应先修改适配配置并记录差异。模型 ID 相同仍可能指向服务商更新过的模型别名。

## 固定的执行条件

| 条件 | 配置 |
|---|---|
| pi 源码 | `cfee633b1ba9497b0c12462da97a08b418b5ebf8`，对应提示模板评测 PR 的固定源码；不冒充已合并的 main |
| 原题 | 添加模型、项目提示模板、用户提示模板；各分有文档和无文档 |
| pi 入口 | 原 `packages/evals/src/cli.ts`，保留原 Docker 隔离、发现/重复/判分/报告逻辑；构建前补执行上游模型目录生成步骤 |
| Lumi 入口 | 当前提交的 `cmd/lumi_evals --suite pi-reference`，真实 Agent 核心及现有虚拟测试环境 |
| 输出上限 | 两边均 4,096 token；pi 仍可根据剩余上下文进一步收缩 |
| 超时 | pi 保留原始每个 test 300 秒；Lumi 每个 case 300 秒；两者计时范围不完全相同 |
| 请求预算 | Lumi 每个 turn 最多 16 次，含生产无进展策略；pi 保留原策略，没有外加统一请求次数上限 |
| 运行顺序 | 先 pi 再 Lumi，各自保留原始顺序和重复调度；网络条件可能随时间变化 |

`prepare` 校验固定提交及关键文件 Git blob。在原 harness 中加入一个评测模型注册调用：在内存注册 `lumi-eval` Provider，使用 pi 原生 `openai-completions` 传输。它不创建 `models.json`、不替换题目中的 `openai` Provider、不加载额外扩展，也不改 Agent、工具和 judge。适配源码、差异及散列写入 `pi-adapter/`；注册的模型价格为 0 仅用于禁用估价，不代表免费。凭据来自运行时的临时 `auth.json` 挂载，不进入构建上下文；运行后删除。

上游不把 `packages/ai/src/providers/data` 提交到 Git。干净检出直接执行其 Dockerfile 的 `build:offline` 会失败，因此构建时在 `npm ci` 后先执行上游 `npm run hydrate:model-data`。这一官方生成器读取实时公共模型目录，不是固定历史目录；实际镜像中的 JSON 和上游散列清单会导出到 `pi-model-catalog/`。这段 Dockerfile 差异也保存为补丁。不得把“固定源码”解释为“所有模型元数据都固定在源码提交时”。

原 pi 评测必须使用容器隔离。CI 构建有文档和无文档两种镜像，共享构建层；宿主仅安装报告读取依赖，不重复安装完整 monorepo。没有跨运行 Docker/npm/Go 缓存。使用标准 `ubuntu-24.04`，并保存构建前后磁盘记录。镜像真实大小和首次构建峰值要以首轮 CI 为准；磁盘不足会保存故障证据，不能将其解释为 Agent 低分。

## 结果包

```text
comparison.md / comparison.json   # 并列成功比例、有效样本数、重复波动、缺失指标、证据路径
execution.json                   # 请求的同一模型/地址、两边提交、运行链接、配置差异
pi-adapter/                      # 固定源码清单、适配补丁、模型描述、宿主依赖锁
pi-model-catalog/                # 从实际镜像导出的内置模型目录及上游散列清单
pi/<原生运行目录>/                # pi protocol、expected-runs、observations、报告、Vitest、完整 sessions
lumi/                            # Lumi protocol、report、逐次 result 和 trace
pi-console.txt / lumi-console.txt # 完整命令输出
*-exit-code.txt                   # 两个入口的退出码
disk-before.txt / disk-after.txt / docker-disk.txt
sha256-manifest.json              # 上传前记录的文件散列；不构成真实性签名
```

结果在 GitHub 保留 **14 天**；需要长期保留时下载或把审核后的记录复制到 `evals/records/`。上传前检查原始密钥、`auth.json` 和符号链接；如果发现凭据，阻止上传，不篡改 trace 或其散列。不要把真实密钥放在任何 workflow input、命令参数或代码中。pi 构建或运行未完整结束时，本轮跳过 Lumi，避免反复排查 pi 时重复付费；已有运行包保持独立，不拼接成一轮。被取消或被平台强制终止的作业可能来不及上传，不能算完整运行。

`comparison.json` 保留每次尝试、分数及耗时/Token 的有效数、均值、最小值、最大值、总体标准差。原生细节可通过其中的相对路径复查。`null`/N/A 表示无法取得；指标总数只汇总标明的有效运行，不冒充整个 cohort 消耗。pi 的 cache token 统计口径可能与 Lumi 的 input token 不同，不直接相加比较总输入。

两边的“成功比例”均是原题严格判分恰好为 1 的次数除以有效评分次数。Lumi 的 `agent_error` 与其评分照实保留，`environment_error` 从比例分母排除。pi 的 `errored`、未评分或缺失记录不自动判为 Agent 失败，也不擅自归因为环境故障；需要根据原始错误和 session 进一步判断，自动报告将其列为证据缺口。低分不会使 CI 失败；非零入口退出、缺失/重复/损坏记录和未评分缺口会使最后的完整性检查失败，但仍上传可用证据。

当前每行标为 `descriptive_adapted`，不计算可归因于 Agent 核心的百分点差值。同一模型排除了一个重要混杂因素；系统提示词、工具输出、文档范围及运行环境仍不同。若要测纯核心差距，需要额外让两边共用工具协议、提示词、数据和预算，那属于另一组实验。

## 验证与扩展

```sh
make test-evals
python3 -m unittest discover -s evals/ci -p '*_test.py'
bash -n evals/ci/run-pi.sh
```

这些检查不调用模型、不运行 pi、不构建镜像。Python 中的合成 pi 数据只用于验证报告管道，不能作为真实评测记录。

下载后先核对完整结果包，随后可离线重建并列报告（只更新 comparison 文件）：

```sh
OUT=evals/results/github-<RUN_ID> python3 evals/ci/baseline.py verify
OUT=evals/results/github-<RUN_ID> python3 evals/ci/baseline.py report
```

更新 pi 时同时审核 `pi-source-lock.json`、原题/评分器、模型注册 API、原生报告 schema、用例映射和 Lumi 的适配合同。新增场景同时修改工作流选中的原文件、`SCENARIOS`、Lumi 参考用例及故障测试；不要仅修改文件散列绕过不兼容检查。

实现和离线检查不等于真实 CI 验收。只有实际 Actions 运行产生完整结果包，才能确认构建、模型连接和对照运行成功；失败后保留旧记录，新建尝试或运行，不能用假数据填补。
