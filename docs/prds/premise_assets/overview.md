# 设定资产 — Premise 生成、版本与生命周期

## 模块职责

设定资产模块负责项目视觉 Premise 的默认画风、输入批次、候选设定图、资产拆分、不可变 variant 和资产回收站。它把可编辑的角色、场景、道具和参考图作为漫画生产可复用的业务资源。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | Premise Profile、source 批次、setting image、资产创建与拆分、tag、variant 选择、资产生命周期。 |
| 不负责 | File/Object 的物理存储和 GC、Chapter 正文、Section 分镜图片、全局 Provider 配置和通用任务状态。 |

## 核心概念

### Source 与资产化

`premise_sources` 是不可变的生成或人工输入批次；setting image 是候选图历史。用户可从候选图创建或拆分出独立 `premise_assets`，资产通过 current variant 选择当前视觉版本。

### 领域删除与文件删除分离

删除 Premise 资产先进入回收站。永久删除只移除领域记录和无引用逻辑 File；Object 的物理回收仍由 `files` 的引用复检、grace period 与 GC 计划决定。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `设定生成与资产化` | [`features/设定生成与资产化.md`](features/设定生成与资产化.md) | 管理 Premise 输入、候选图、资产拆分和版本选择。 |
| `设定资产生命周期` | [`features/设定资产生命周期.md`](features/设定资产生命周期.md) | 提供回收站、永久删除与历史引用保护。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| 文件 | setting image 和 asset variant 通过 `files.id` 引用受控 File。 |
| 漫画 Section | Section Premise 从 active、current variant 且 ready 的资产中选择参考。 |
| AI 运行时 | Premise 任务在创建时冻结文本或图片模型和 Prompt。 |
| Chat Thread | Agent 可在受控业务 API 下创建、更新或引用 Premise 资产。 |
