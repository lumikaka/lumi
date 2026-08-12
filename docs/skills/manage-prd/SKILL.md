---
name: manage-prd
description: 创建和维护 Lumi 项目的 PRD。用于按根索引、domain 总览、domain 数据模型和粗粒度 feature 文档组织 docs/prds；也用于判断 feature 粒度、调整已有 PRD 结构，或生成可按 feature 复制到其他系统的产品规格。
---

# 管理 PRD

使用 **domain 总览 + feature 垂直切片** 管理当前项目的 PRD。将 Feature 定义为值得复制到另一个系统的业务能力包，而不是页面、API、数据表、按钮、CRUD 动作或 worker。

更新 `docs/prds` 前，先阅读根目录 `AGENTS.md`、已有 PRD、相关源码、GORM model 和 `db/migrations` 中的 `.up.sql`/`.down.sql`。让文档遵守 REST API、统一 JSON 信封、内部 64-bit `id`、外部 UUIDv7 等约束，不要让 PRD 与实现或项目规范冲突。

## 目录结构

```text
docs/prds/
├── overview.md
└── {domain}/
    ├── overview.md
    ├── data_model.md
    └── features/
        └── {feature_title}.md
```

- 根 `overview.md`：列出所有 domain，并提供链接和一句话说明。
- Domain `overview.md`：描述定位、职责边界、核心概念、feature 列表和 domain 关系。
- Domain `data_model.md`：描述完整实体关系、共享表、状态和全局约束。
- `features/{feature_title}.md`：作为主要阅读单元，尽量自包含并便于复制。

编写具体 PRD 文件时，阅读 `references/templates.md`。

## Feature 粒度

使用以下标准：

> Feature 是一个值得复制到另一个系统的业务能力包。

不要按页面、按钮、字段、数据表、接口、CRUD 动作、worker 或配置项切分。只有两个能力能独立复制、理解和采用时才拆开。

判断候选 feature：

1. 另一个系统是否可以只复制它，而不复制同 domain 下的大多数能力？
2. 它是否产生明确业务结果，而非只暴露技术动作？
3. 它是否需要有意义的数据模型、流程、边界或状态设计？
4. 继续拆小是否会迫使读者跨多个文件才能理解完整能力？

如果前三项大多为否，合并进更大的 feature；如果第四项为是，保持为一个 feature。

### 保持已有 Feature 稳定

- 优先尊重已有名称和粒度，不轻易重命名、拆分、合并或删除。
- 新增前先检查能否并入已有 feature。
- 只有用户明确要求，或现有划分明显无法承载内容时，才调整结构。

## 创建或更新 Domain

1. 阅读根 `docs/prds/overview.md` 和目标 domain 的现有文件。
2. 确认定位、职责边界和现有 feature 划分。
3. 更新 `overview.md` 的核心概念、feature 链接和 domain 关系。
4. 更新 `data_model.md` 的共享实体、完整关系和全局约束。
5. 只为业务能力包级内容创建 feature 文档。
6. 核对术语、表、API 和 UI；新增 domain 时同步根索引。

## 创建或更新 Feature

1. 确认该 feature 值得独立复制。
2. 合并仅代表实现任务的候选项。
3. 按需使用以下 H2，不创建空小节：`overview`、`data_model`、`api`、`api_admin`、`ui`、`ui_admin`、`commands`、`jobs`、`others`。
4. 记录复制所需的最小数据模型、流程、API、UI、job、权限、配置和约束。
5. 如果改变共享实体，同步 domain 文档；改变 domain 定位时同步根索引。

## 写作规则

- 使用中文；domain 目录名使用小写 snake_case，feature 文件名以中文为主且不含空格。
- Feature 标题使用 `# {Domain} — {Feature}`，H2 使用规定的小写名称。
- 不创建 `current_status` 或空占位小节。
- 使用反引号包裹含下划线的标识符。
- 只记录 GORM model 和已存在 migration 中的字段与关联，不使用 ORM AutoMigrate 作为依据。
- SQLite 内部主键使用 `INTEGER PRIMARY KEY AUTOINCREMENT`，作为 64-bit rowid；外键和 JOIN 使用内部 `id`。
- URL、JSON 和前端交互只使用 UUIDv7 `uuid`，不得暴露内部 `id`。
- API 使用 `/api/v1` 下的 REST 资源路径、snake_case 字段和 `AGENTS.md` 的统一 JSON 信封。
- SQLite migration 成对使用 `.up.sql`/`.down.sql`；不要写显式 `BEGIN` 或 `COMMIT`，连接级 PRAGMA 放在 `DATABASE_DSN`。
- 描述实现落点时使用 Echo handler、GORM model、golang-migrate、React 页面或组件及相应测试。
