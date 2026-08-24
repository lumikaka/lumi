# 项目 — 项目Prompt定制与版本恢复

## overview

该 Feature 为内置 Prompt Catalog 保存项目级覆盖，而不修改代码内默认 Prompt。覆盖按 group 和 key 版本化，支持创建、批量更新、恢复历史版本和恢复默认；Story、Premise、漫画与 Agent 调用在任务创建时冻结最终 Prompt，不在重试时重新读取当前覆盖。

## data_model

`project_prompt_versions` 以 `(project_id, prompt_group, prompt_key, version_no)` 唯一保存不可变版本。每条记录含公开 UUIDv7、内容 hash、来源类型及可选 `restored_from_version_id` 内部关联；当前值由 Catalog 默认值与最新项目版本投影得出。

## api

| 接口 | 方法 | 说明 |
|---|---|---|
| `/api/v1/projects/:project_uuid/prompts` | GET | 返回默认值、有效值、占位符和当前版本。 |
| `/api/v1/projects/:project_uuid/prompt-groups/:prompt_group` | PATCH | 原子更新一个 Prompt group 的多个 key。 |
| `/api/v1/projects/:project_uuid/prompt-versions` | GET / POST | 查询历史或创建单个覆盖版本。 |
| `/api/v1/projects/:project_uuid/prompt-versions/:version_uuid/restorations` | POST | 基于历史版本创建新的恢复版本。 |

## ui

| 页面 / 入口 | 说明 |
|---|---|
| `/projects/:project_uuid/overview/prompts` | 按 group 编辑有效 Prompt、查看版本、恢复历史或默认值。 |

## others

默认 Prompt、group、key 和占位符定义位于 `internal/promptcatalog`。保存必须校验合法 group/key 和模板占位符；运行时不接受未冻结的历史重写。
