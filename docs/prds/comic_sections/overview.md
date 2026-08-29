# 页面 / 画面段落 — 页面角色、脚本、图片版本与快照

## 模块职责

本模块负责 Chapter 的漫画状态、有序 Section、页面角色、Storyboard 与 Image Variant、Section Premise 参考、生成冲突和快照恢复。它以 Chapter 为父资源，但拥有当前漫画产物的可编辑状态和历史。普通绘本形式使用“页面 / 页面脚本”，条漫 `vertical_strip` 使用“画面段落 / 分镜脚本”；Image Variant 统一称“页面图片版本”。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | Chapter comic state、Section 页面角色与排序、脚本、图片、Variant 选择、Section Premise、快照与恢复。 |
| 不负责 | Chapter 正文、Premise 资产本身、File/Object 存储、模型选择和最终 ZIP/PDF 导出保留。 |

## 核心概念

### 形式化称呼

技术实体始终为 `comic_section`、`storyboard` 与 `image_variant`。产品层按项目形式选择“页面 / 页面脚本”或“画面段落 / 分镜脚本”；两种形式都把 `image_variant` 称为“页面图片版本”。Section 是承载脚本和图片版本的画面单元，不能与 Storyboard 混称。

### 页面角色与装订顺序

普通绘本的 Section 可为 `front_cover`、`body` 或 `back_cover`。封面和封底均可选且每本最多一个，正文页至少一个且可有多个；全空 comic state 必须先创建 `body`，创建特殊页、删除正文页或转换角色都不能使普通绘本回到 0 个正文页。`section_no` 是整本绘本的绝对装订位置，服务端固定封面在首位、封底在末位，正文页只在两者之间排序。条漫 `vertical_strip` 只允许 `body`，并可删除最后一个画面段落回到 empty。

### 当前漫画状态

每个 Chapter 恰有一个 `chapter_comic_states`。active Section 的 current storyboard 和 current image 指向不可变 Variant；编辑、生成和恢复创建新历史或切换指针，不覆盖既有内容。`storyboarded` 要求至少一个正文页，且所有实际存在的页面都有脚本；`ready` 同样要求至少一个正文页，且所有实际存在的封面、正文页和封底都有 current image。

### 快照恢复

快照是 Chapter 漫画状态的不可变记录，含有序 Section、`page_role`、脚本、当前图片和 Premise 引用。恢复前阻止活动生成；普通绘本拒绝恢复 empty 或只含特殊页的快照，条漫仍可恢复 empty。详情以媒体可用状态而非内部路径或 hash 暴露历史。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `漫画分镜与图片生产` | [`features/漫画分镜与图片生产.md`](features/漫画分镜与图片生产.md) | 编辑 Section 角色与顺序，生产脚本/图片并选择当前 Variant。 |
| `漫画快照与恢复` | [`features/漫画快照与恢复.md`](features/漫画快照与恢复.md) | 预览含页面角色的不可变快照，并安全恢复 Chapter 漫画状态。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| 章节 | Chapter 是 comic state 的父资源；Chapter 删除会按外键清理自身漫画记录。 |
| 设定资产 | Section Premise 仅选择 active、current variant 和 ready File 的设定资产。 |
| 文件 | 图片 Variant 与 Premise 参考通过受控 File 投影读取。 |
| 工作流 / AI 运行时 | 分镜和图片生成可使用冻结任务或 Workflow；业务结果由本模块提交。 |
| 导出 | Export 按页面角色与装订顺序读取当前 ready 图片并冻结快照，不拥有 Section 编辑状态。 |
| 对话线程 | `comic_section` Reference 冻结 `page_role` 和 `body_page_no`，供 ChatArea 与 Agent 区分封面、正文页和封底。 |
