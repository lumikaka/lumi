# 2026-10-26 共有 Agent 核心基准

模型固定 qwen3.7-plus，每项重复 3 次。当前题集为 8 项 Lumi 回归、其中 4 项真实 pi 核心对照。构建仅在 GitHub 临时 runner 完成。

## 首次尝试：启动适配故障

[运行 40275948766](https://github.com/lumikaka/lumi/actions/runs/40275948766)，源码 `2945231d2a9ce02e5f1cf22ed3f7a9d2ee3cd742`。Lumi 完成 24 次，所有机械评分项均为 1；pi 的 12 次均在模型请求前因 ESM 包解析失败，记为 `environment_error`、不计分，不能据此比较能力。

[原始共有报告](40275948766/shared-comparison.md) · [Lumi 报告](40275948766/lumi/report.md) · [pi 启动错误](40275948766/pi/tasks/smoke/run-001/console.txt)

修复为从已核验源码的编译路径导入 ESM，并增加模型调用前的真实模块加载检查。后续尝试使用新目录，不覆盖这 144 个已封存文件。ZIP 散列、运行版本和归档元数据见 [attempts.json](attempts.json)。
