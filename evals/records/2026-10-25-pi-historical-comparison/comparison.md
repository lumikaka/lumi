# Lumi / pi 历史成绩对照

基线：pi-public-history-2026-10-25；模型：qwen3.7-plus；协议：`bd74ae425b8dbb7a739d5c7ae4a44f8ad377b022c91dff59984ca7ff7d70bc0a`。

[Lumi 完整执行报告](<../2026-10-25-pi-reference-qwen3.7-plus/report.md>)。

可直接比较：0；仅近似参考：6；映射已失效：0；无本地对应：2；缺少 pi 基线：0。

pi 数值来自历史记录。本报告不执行 pi；近似参考不计算能力差值。N/A 表示指标缺失。不同任务或不同文档组不合并总分。

| 用例 / 文档组 | pi 历史比例 / n | Lumi 满分比例 / 有效数 / 计划数 | 分类 | 差值（百分点） |
|---|---:|---:|---|---:|
| Create a project prompt template / with_docs | 66.7% / 3 | 66.7% / 3 / 3 | 仅近似参考 | N/A |
| Create a project prompt template / without_docs | 100.0% / 3 | 66.7% / 3 / 3 | 仅近似参考 | N/A |
| Create a user prompt template / with_docs | 100.0% / 3 | 100.0% / 3 / 3 | 仅近似参考 | N/A |
| Create a user prompt template / without_docs | 100.0% / 3 | 33.3% / 3 / 3 | 仅近似参考 | N/A |
| Add model to existing provider / with_docs | 100.0% / 1 | 0.0% / 3 / 3 | 仅近似参考 | N/A |
| Add model to existing provider / without_docs | 0.0% / 1 | 0.0% / 3 / 3 | 仅近似参考 | N/A |
| Documentation evals (reported aggregate) / with_docs | 86.7% / 15 | N/A / 0 / 0 | 本地无对应用例 | N/A |
| Documentation evals (reported aggregate) / without_docs | 13.3% / 15 | N/A / 0 / 0 | 本地无对应用例 | N/A |

Lumi 满分比例：该次运行全部评分项为 1 的次数 / 有效评分次数。它是本对照的描述性指标，不是运行通过门槛。环境故障不计作模型零分，计划数保留。小样本和四舍五入的历史比例不能证明稳定性或统计显著性。

## Create a project prompt template / with_docs

来源：[原始记录](https://github.com/earendil-works/pi/pull/10261)。

- 作者报告的 PR 摘要；没有逐次结果、Token、耗时或费用。
- PR 检索时仍未合并，不能当作当前 main 的成绩。
- 用例环境为适配实现，尚未证明与 pi 等价
- 任务原文、步骤顺序及严格评分字段沿用 PR #10261 分支源码；实际运行提交未报告。
- 使用固定评测提示词，未复现 pi 默认系统提示词。
- Go 虚拟文件和工具接口模拟；工具返回格式、检索行为及边界与 pi 原生工具未证明等价。
- 仅提供场景相关文档快照，未安装 pi 全部文档、示例和运行时。
- 只模拟本场景的模板加载、无参数展开和目录检查；不执行扩展、包资源或完整 pi 资源加载器。
- 来源是作者报告的摘要，未取得逐次原始运行记录
- pi 实际执行提交未报告；不能用 PR head 代替
- pi 模型、提示词配置和执行预算不可用

## Create a project prompt template / without_docs

来源：[原始记录](https://github.com/earendil-works/pi/pull/10261)。

- 作者报告的 PR 摘要；没有逐次结果、Token、耗时或费用。
- PR 检索时仍未合并，不能当作当前 main 的成绩。
- 用例环境为适配实现，尚未证明与 pi 等价
- 任务原文、步骤顺序及严格评分字段沿用 PR #10261 分支源码；实际运行提交未报告。
- 使用固定评测提示词，未复现 pi 默认系统提示词。
- Go 虚拟文件和工具接口模拟；工具返回格式、检索行为及边界与 pi 原生工具未证明等价。
- 仅提供场景相关文档快照，未安装 pi 全部文档、示例和运行时。
- 只模拟本场景的模板加载、无参数展开和目录检查；不执行扩展、包资源或完整 pi 资源加载器。
- 来源是作者报告的摘要，未取得逐次原始运行记录
- pi 实际执行提交未报告；不能用 PR head 代替
- pi 模型、提示词配置和执行预算不可用

## Create a user prompt template / with_docs

来源：[原始记录](https://github.com/earendil-works/pi/pull/10261)。

- 作者报告的 PR 摘要；没有逐次结果、Token、耗时或费用。
- PR 检索时仍未合并，不能当作当前 main 的成绩。
- 用例环境为适配实现，尚未证明与 pi 等价
- 任务原文、步骤顺序及严格评分字段沿用 PR #10261 分支源码；实际运行提交未报告。
- 使用固定评测提示词，未复现 pi 默认系统提示词。
- Go 虚拟文件和工具接口模拟；工具返回格式、检索行为及边界与 pi 原生工具未证明等价。
- 仅提供场景相关文档快照，未安装 pi 全部文档、示例和运行时。
- 只模拟本场景的模板加载、无参数展开和目录检查；不执行扩展、包资源或完整 pi 资源加载器。
- 来源是作者报告的摘要，未取得逐次原始运行记录
- pi 实际执行提交未报告；不能用 PR head 代替
- pi 模型、提示词配置和执行预算不可用

## Create a user prompt template / without_docs

来源：[原始记录](https://github.com/earendil-works/pi/pull/10261)。

- 作者报告的 PR 摘要；没有逐次结果、Token、耗时或费用。
- PR 检索时仍未合并，不能当作当前 main 的成绩。
- 用例环境为适配实现，尚未证明与 pi 等价
- 任务原文、步骤顺序及严格评分字段沿用 PR #10261 分支源码；实际运行提交未报告。
- 使用固定评测提示词，未复现 pi 默认系统提示词。
- Go 虚拟文件和工具接口模拟；工具返回格式、检索行为及边界与 pi 原生工具未证明等价。
- 仅提供场景相关文档快照，未安装 pi 全部文档、示例和运行时。
- 只模拟本场景的模板加载、无参数展开和目录检查；不执行扩展、包资源或完整 pi 资源加载器。
- 来源是作者报告的摘要，未取得逐次原始运行记录
- pi 实际执行提交未报告；不能用 PR head 代替
- pi 模型、提示词配置和执行预算不可用

## Add model to existing provider / with_docs

来源：[原始记录](https://github.com/earendil-works/pi/pull/10261)。

- 作者报告的 PR 摘要；没有逐次结果、Token、耗时或费用。
- PR 检索时仍未合并，不能当作当前 main 的成绩。
- 用例环境为适配实现，尚未证明与 pi 等价
- 任务原文、步骤顺序及严格评分字段沿用 PR #10261 分支源码；实际运行提交未报告。
- 使用固定评测提示词，未复现 pi 默认系统提示词。
- Go 虚拟文件和工具接口模拟；工具返回格式、检索行为及边界与 pi 原生工具未证明等价。
- 仅提供场景相关文档快照，未安装 pi 全部文档、示例和运行时。
- 模型注册表使用隔离的最小内置目录及加法合并；未执行 pi ModelRuntime 的完整配置验证。
- 来源是作者报告的摘要，未取得逐次原始运行记录
- pi 实际执行提交未报告；不能用 PR head 代替
- pi 模型、提示词配置和执行预算不可用

## Add model to existing provider / without_docs

来源：[原始记录](https://github.com/earendil-works/pi/pull/10261)。

- 作者报告的 PR 摘要；没有逐次结果、Token、耗时或费用。
- PR 检索时仍未合并，不能当作当前 main 的成绩。
- 用例环境为适配实现，尚未证明与 pi 等价
- 任务原文、步骤顺序及严格评分字段沿用 PR #10261 分支源码；实际运行提交未报告。
- 使用固定评测提示词，未复现 pi 默认系统提示词。
- Go 虚拟文件和工具接口模拟；工具返回格式、检索行为及边界与 pi 原生工具未证明等价。
- 仅提供场景相关文档快照，未安装 pi 全部文档、示例和运行时。
- 模型注册表使用隔离的最小内置目录及加法合并；未执行 pi ModelRuntime 的完整配置验证。
- 来源是作者报告的摘要，未取得逐次原始运行记录
- pi 实际执行提交未报告；不能用 PR head 代替
- pi 模型、提示词配置和执行预算不可用

## Documentation evals (reported aggregate) / with_docs

来源：[原始记录](https://github.com/earendil-works/pi/pull/9898)。

- 每个变体重复 3 次，作者报告该组总计 15 次；没有逐用例成绩或完整清单。
- 不将总体通过率分配给某个用例，也不与当前 14 例或新 3 个场景的总分相减。
- 本次没有对应的本地用例；整体成绩不拆分给单个用例

## Documentation evals (reported aggregate) / without_docs

来源：[原始记录](https://github.com/earendil-works/pi/pull/9898)。

- 每个变体重复 3 次，作者报告该组总计 15 次；没有逐用例成绩或完整清单。
- 不将总体通过率分配给某个用例，也不与当前 14 例或新 3 个场景的总分相减。
- 本次没有对应的本地用例；整体成绩不拆分给单个用例

