---
name: run-plan
description: 落地 docs/plans/ 下的计划文档到当前项目。传入计划文件路径作为参数即开始执行；不传参数则列出所有可用计划供用户选择，手动触发。
disable-model-invocation: true
---

# Run Plan

将 `docs/plans/*.md` 中描述的功能计划逐步落地到代码库。

## 工作流程

### 1. 确定目标计划

- 如果用户传入了计划文件路径（如 `docs/plans/添加api-doc入口.md`），直接使用
- 如果未传入，列出 `docs/plans/` 下所有 `.md` 文件，让用户选择

### 2. 运行计划

落地 {{计划文件}}（如`docs/plans/添加api-doc入口.md`) 的功能到当前项目


### 3. 更新 frontmatter

```bash
nova-frontmatter replace --path "/plan_state" --value "finished" --type "string" --create-missing {file}
```
> `{file}` — 当前计划文件路径，如 `docs/plans/foo.md`

### 4. 提交代码

```bash
nova-frontmatter get --path "/git_commit_message" --output text docs/plans/009-subject-facets.md
```

获取到 `{git_message}` ， 直接使用 `git commit -av` 提交
