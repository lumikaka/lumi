# PRD 模板

编写 PRD 时使用以下模板，没有实际内容的小节直接省略。

## 目录

- [Domain overview](#domain-overviewmd)
- [Domain data model](#domain-data_modelmd)
- [Feature](#feature-featuremd)
- [Feature 拆分建议](#feature-拆分建议)

## Domain `overview.md`

```markdown
# {Domain 名称} — {一句话描述}

## 模块职责

{Domain} 模块负责{核心职责}，用于{主要业务场景}。本节只描述整体定位、边界和存在理由，具体能力见 Feature 列表。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | {本模块负责的能力} |
| 不负责 | {明确排除的能力} |

## 核心概念

### {概念名称}

{用 1-3 句话解释含义和设计意图。}

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `{feature}` | [`features/{feature}.md`](features/{feature}.md) | {解决的问题} |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| {模块名} | {数据依赖、调用关系或边界} |
```

## Domain `data_model.md`

```markdown
# {Domain 名称} — 数据模型

## 实体关系

{使用 ASCII 图展示外键或数据流。}

## 表：{table_name}

{一句话说明用途。}

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，SQLite 64-bit rowid，仅供内部主键、外键和 JOIN
- `uuid` — TEXT NOT NULL UNIQUE，UUIDv7，用于 URL、JSON、前端和开放接口
- `{field}` — {SQLite TYPE}，{说明、默认值、可空性和约束}
- `{fk_field}` — INTEGER FK → `{target_table}.id`，{说明}

**索引：**

- `{field}` — {普通、唯一或联合索引及用途}

**主要相关 Feature：**

- [`{feature}`](features/{feature}.md)

## 枚举：{enum_name}

- `{VALUE}` — {说明}

## 数据生命周期

1. {阶段} → {描述}
```

只记录 GORM model 或 `.up.sql`/`.down.sql` migration 中真实存在的结构。SQLite 的类型亲和性不能代替应用层校验。

## Feature `{feature}.md`

````markdown
# {Domain 名称} — {Feature 名称}

## overview

{业务目标、现状、职责边界、核心流程、依赖和约束。}

## data_model

{独立复制该 feature 所需的实体、字段、索引、枚举和状态流转；区分内部 id 与外部 uuid。}

## api

| 接口 | 方法 | 说明 |
|---|---|---|
| `/api/v1/{resources}` | GET | {列表、筛选、分页和排序} |
| `/api/v1/{resources}/:uuid` | GET | {单对象、可见性和错误码} |
| `/api/v1/{resources}` | POST | {创建、字段与校验} |
| `/api/v1/{resources}/:uuid` | PATCH/PUT/DELETE | {更新或删除、权限与幂等性} |
| `/api/v1/{resources}/:uuid/{action}` | POST | {资源语义动作和副作用} |

所有响应遵守统一 JSON 信封，字段使用 snake_case。

## api_admin

| 接口 | 方法 | 说明 |
|---|---|---|
| `/api/v1/admin/{resources}` | GET | {管理列表} |
| `/api/v1/admin/{resources}/:uuid` | GET | {管理详情与权限} |
| `/api/v1/admin/{resources}` | POST | {管理创建} |
| `/api/v1/admin/{resources}/:uuid` | PATCH/PUT/DELETE | {管理更新或删除} |

## ui

| 页面 / 入口 | 说明 |
|---|---|
| `{route}` | {展示、操作、状态、空状态和错误状态} |

## ui_admin

| 页面 / 入口 | 说明 |
|---|---|
| `{route}` | {管理端列表、详情、编辑、审核或筛选} |

## commands

```bash
{command} [options]
```

| 参数 | 类型 | 说明 |
|---|---|---|
| `--{option}` | {type} | {执行范围、幂等性和输出} |

## jobs

| Job / Worker | 触发条件 | 策略 |
|---|---|---|
| `{JobName}` | {触发条件} | {执行、重试、幂等和失败处理} |

## others

{配置、权限、埋点、migration、兼容性、风险或开放问题。}
````

## Feature 拆分建议

```markdown
## 推荐 Feature

| Feature | 职责边界 | 为什么值得单独复制 |
|---|---|---|
| `{feature}` | {包含与排除} | {业务能力包理由} |

## 合并进 Feature 的小功能

| 小功能 | 合并到 | 原因 |
|---|---|---|
| {按钮/API/页面/任务} | `{feature}` | {不应单独成 feature 的原因} |

## 不建议单独成 Feature

| 候选项 | 原因 |
|---|---|
| {候选项} | {太细、只有技术动作或没有独立业务结果} |
```
