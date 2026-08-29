# Lumi — PRD 索引

Lumi 的产品需求按 domain 组织。默认以聚合根表族命名；`ai_providers`、`ai_runtime` 和 `exports` 是重要产品概念例外。每个 Feature 是可独立复制的业务能力包；页面、按钮、单个 API、表和 worker 只是 Feature 的实现组成部分，不单独成为 Feature。

## 统一产品术语

技术标识不随产品称呼变化；API、数据库和代码继续使用 `project`、`chapter`、`comic_section`、`storyboard` 与 `image_variant`。面向用户的中文由项目 `picture_book.format` 决定：

| 技术概念 | 普通绘本形式 | 条漫 `vertical_strip` |
|---|---|---|
| `project` | 项目 / 绘本项目 | 项目 / 绘本项目 |
| `chapter` | 绘本 | 章节 |
| `comic_section` | 页面（可选封面、正文页、可选封底） | 画面段落 |
| `storyboard` | 页面脚本 | 分镜脚本 |
| `image_variant` | 页面图片版本 | 页面图片版本 |

除非正在描述技术接口或表结构，产品文案、Agent 回复和 UI 必须使用当前形式对应的称呼，不得把 `comic_section` 本身称为分镜，也不得把 `storyboard` 泛称为页面。

普通绘本形式通过 `comic_sections.page_role=front_cover|body|back_cover` 区分封面、正文页与封底；非空的当前页面序列至少包含一个 `body`，`section_no` 始终表示整本绘本的装订顺序。条漫 `vertical_strip` 只允许 `body`，不引入封面或封底角色，并可暂时没有画面段落。

| Domain | 文档 | 说明 |
|---|---|---|
| 项目 | [`projects/overview.md`](projects/overview.md) | 管理 local-first 项目生命周期、对话式草稿初始化、项目资料、总纲投影和项目 Prompt 配置。 |
| 绘本 / 章节 | [`chapters/overview.md`](chapters/overview.md) | 管理 Chapter 正文版本、导入、回收站和 Story 生成；普通绘本形式称绘本，条漫称章节。 |
| 设定资产 | [`premise_assets/overview.md`](premise_assets/overview.md) | 管理 Premise、候选设定图、资产 Variant 与生命周期。 |
| 页面 / 画面段落 | [`comic_sections/overview.md`](comic_sections/overview.md) | 管理 Section 的封面/正文/封底角色、页面脚本或分镜脚本、页面图片版本和可恢复快照。 |
| 导出 | [`exports/overview.md`](exports/overview.md) | 管理当前漫画 ZIP/PDF 的短期交付、下载和清理。 |
| 对话线程 | [`chat_threads/overview.md`](chat_threads/overview.md) | 管理可恢复项目 Agent 对话、追问、工具和多模态交互。 |
| 工作流 | [`workflows/overview.md`](workflows/overview.md) | 管理可恢复多步业务编排与 YOLO 初始化。 |
| AI Provider | [`ai_providers/overview.md`](ai_providers/overview.md) | 管理全局 Provider 配置、密钥、默认模型和连接验证。 |
| AI 运行时 | [`ai_runtime/overview.md`](ai_runtime/overview.md) | 管理模型解析、冻结执行、恢复任务和 AI 调用审计。 |
| 文件 | [`files/overview.md`](files/overview.md) | 管理项目 File/Object、完整性、维护和受控回收。 |
