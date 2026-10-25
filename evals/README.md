# Lumi Agent evals

这是独立、手动触发的 Agent 行为评测。它执行 Lumi 当前的 `agent.Service.ExecuteJob`，不会启动应用、访问用户绘本或调用业务工具。真实模型负责理解任务、选择工具、生成参数及最终回答；仅工具和环境使用固定模拟数据。

- [使用与扩展说明](../docs/agent-evals.md)
- [pi 参考版本与逐项适配清单](adaptation.md)
- [参考文件版本清单](reference.json)
- [验收记录及环境故障补跑](verification.md)
- [Agent 表现证据与建议](findings.md)
- [pi 历史成绩基线及本地对照运行](baselines/pi/README.md)
- [GitHub Actions 真实 pi / Lumi 对照](ci/README.md)
- [首批真实运行记录](records/2026-10-25-qwen3.7-plus/report.md)

```sh
make eval-agent-list
make test-evals
make eval-agent EVAL_ARGS='--lumi-config-dir /absolute/path/to/.lumi_dev --repeat 2'
```

默认输出位于忽略的 `evals/results/`；本次验收的审阅记录保存在 `evals/records/`。本地运行命令不会上传结果；任务提示与模拟工具输出会发送给所选模型 Provider。单独手动触发的 GitHub Actions 将结果保存为 14 天的 artifact。评测使用独立命令，不加入普通测试、应用 UI、Hugo 或桌面打包流程。
