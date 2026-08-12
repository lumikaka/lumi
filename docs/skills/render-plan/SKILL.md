---
name: render-plan
description: 润色和完善 docs/plans/ 下的计划文档，按标准层面结构整理内容。手动调用触发（/render-plan）
disable-model-invocation: true
---

# Render Plan

润色 `docs/plans/*.md` 中的计划文档，使其结构清晰、内容简洁。

## 安全约束

只允许编辑 `docs/plans/*.md` 文件，不得创建或修改其他路径的文件。

## 工作流程

### 1. 确定目标文件

- 如果用户指定了文件名，直接使用
- 如果未指定，询问用户要润色哪个文件

### 2. 按层面结构重写

用以下层面组织文档。如果某个层面在计划中不涉及，直接省略该章节（不要写"无"或"不涉及"）：

```markdown
# {计划标题}

## current_status
当前现状

## overview
模块概述、职责边界、与其他模块的关系。

## data_model
数据结构、实体关系、字段定义。

## api
面向用户的 API

## api_admin
管理端 API

## ui
面向用户的界面/交互

## ui_admin
管理端界面/交互

## commands
同步命令 / CLI — mix task 等。

## jobs
异步任务 / 后台作业 — Oban Worker、触发条件、执行策略。

## others
其他不属于上述层面的内容。

## prds
完成后更新 `docs/prds/*` 下的 prd 文档
```

### 3. 写作原则

1. 计划一定要简洁不啰嗦
2. 尽量不丢失原md中表达的信息
3. 要探索项目现有结构完善计划，不是单纯基于我给的文本润色

### 4. 更新 frontmatter

两条命令顺序执行，避免竞争

```bash
nova-frontmatter replace --path "/plan_state" --value "rendered" --type "string" --create-missing {file}
nova-frontmatter replace --path "/git_commit_message" --value "{git_message}" --type "string" --create-missing {file}
```
> `{file}` — 当前计划文件路径，如 `docs/plans/foo.md`
> `{git_message}` — 根据当前的md文件内容对应的 commit message，注意通常不是 `docs:xxx`
