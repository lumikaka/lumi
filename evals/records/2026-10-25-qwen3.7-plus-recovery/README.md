# Provider 故障后的独立补跑

选取主批次的 custom-stream-provider 与 extension-reload 原始 JSON 定义，各执行两次。使用同一 qwen3.7-plus、基础提示词、工具环境、评分规则、请求预算及 300 秒用例时限。完整输入嵌入 protocol.json。

第一次补跑分别遇到超时和 unexpected EOF；第二次均成功。报告保留全部四次尝试，退出码 1 反映仍有环境错误记录，不把成功补跑覆盖到早先记录中。

- [报告](report.md)
- [控制台输出](console.txt)
- [验收汇总](../../verification.md)

生产 Agent 核心、LLM 适配器、用例、环境和评分的源码哈希与主批次相同。记录器只新增了 snake_case 诊断投影、脱敏底层错误详情及更保守的未知运行时故障分类。
