# Lumi — PRD 索引

Lumi 的产品需求按 domain 组织。默认以聚合根表族命名；`ai_providers`、`ai_runtime` 和 `exports` 是重要产品概念例外。每个 Feature 是可独立复制的业务能力包；页面、按钮、单个 API、表和 worker 只是 Feature 的实现组成部分，不单独成为 Feature。

| Domain | 文档 | 说明 |
|---|---|---|
| 项目 | [`projects/overview.md`](projects/overview.md) | 管理 local-first 项目生命周期、项目资料、总纲投影和项目 Prompt 配置。 |
| 章节 | [`chapters/overview.md`](chapters/overview.md) | 管理 Chapter 正文版本、导入、回收站和 Story 生成。 |
| 设定资产 | [`premise_assets/overview.md`](premise_assets/overview.md) | 管理 Premise、候选设定图、资产 Variant 与生命周期。 |
| 漫画段落 | [`comic_sections/overview.md`](comic_sections/overview.md) | 管理 Section、分镜、图片和可恢复漫画快照。 |
| 导出 | [`exports/overview.md`](exports/overview.md) | 管理当前漫画 ZIP/PDF 的短期交付、下载和清理。 |
| 对话线程 | [`chat_threads/overview.md`](chat_threads/overview.md) | 管理可恢复项目 Agent 对话、追问、工具和多模态交互。 |
| 工作流 | [`workflows/overview.md`](workflows/overview.md) | 管理可恢复多步业务编排与 YOLO 初始化。 |
| AI Provider | [`ai_providers/overview.md`](ai_providers/overview.md) | 管理全局 Provider 配置、密钥、默认模型和连接验证。 |
| AI 运行时 | [`ai_runtime/overview.md`](ai_runtime/overview.md) | 管理模型解析、冻结执行、恢复任务和 AI 调用审计。 |
| 文件 | [`files/overview.md`](files/overview.md) | 管理项目 File/Object、完整性、维护和受控回收。 |
