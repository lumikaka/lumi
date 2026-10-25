# 协议检查期间的部分运行

该目录保存字段规格修订后的真实部分运行。检查工具 JSON Schema 时发现无参数工具的 required 被序列化为 null；虽然当前模型 Provider 接受它，标准 JSON Schema 应使用数组。主动取消本批次并将 required 修正为 []，添加跨 Provider 协议约束测试后重新完整运行。

本目录只用于追溯开发验证，不作为完整 cohort 验收。既有 protocol、result 和 trace 保持原样，取消及未运行项目在 report 中可见。
