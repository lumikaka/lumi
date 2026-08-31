---
git_commit_message: 'feat(project): 让首页参考图参与 YOLO 设定生成'
plan_state: finished
---

# 首页创建项目参考图接入 YOLO 生成计划

## current_status

### 当前链路

- 首页 Composer 最多可附带 16 个文件。创建会话会把文件上传到 App DB，项目激活后再绑定到 Project DB 的 `project_creation_reference_files`。
- Bootstrap Turn 只向 Agent 上下文注入文件名、MIME、大小和公开 UUID 等元数据，LLM 并不会读取图片像素。
- `CreateYoloInput` 与 YOLO v5 快照只保存故事文本、模型和项目配置，没有参考图计划。
- Premise 设定图任务调用 `imagegen.Request` 时只传文本；因此首页参考图目前不会影响设定图、封面或第一页的实际像素生成。
- Cloudflare 与百炼图片生成适配器已经支持 `imagegen.Request.Images`；Comic 图片任务也已有“Premise 多图合成一张带标签参考板再送入模型”的生产链路。本功能应复用并泛化这套能力。

### 目标结果

用户在首页创建项目时附带的图片，可以被明确标记为人物、场景、道具、画风或自动参考。项目确认并启动 YOLO 后，系统冻结本次引用计划，把图片实际送入 Premise 设定图生成，并将来源文件复用为 Premise Assets，供封面、第一页和后续重生成继续选择。

验收时必须能证明：

1. YOLO 快照记录了哪些参考图被采用、用途是什么。
2. Premise 图片请求包含真实图片字节，而不只是文件名或文字描述。
3. 用户排除的图片只保留在快照审计计划中，不会进入 YOLO Worker 的 included 输入、Premise Assets 或图片请求。
4. 引用图片丢失、损坏或供应商拒绝图片输入时任务明确失败并可重试，不允许退化为纯文本后继续成功。
5. 旧项目、旧 YOLO 快照和不带创建会话的直接 YOLO 保持原行为。

## overview

### 产品语义

首页每张图片新增一份可选的引用计划：

| 字段 | 含义 | 默认值 |
| --- | --- | --- |
| `include_in_yolo` | 是否让该图参与当前项目的视觉生成 | `true` |
| `reference_role` | `auto`、`character`、`scene`、`prop`、`style` | `auto` |
| `title` | 在 Premise 与参考板中的可读名称 | 文件名去扩展名 |
| `instruction` | 用户对保留内容或使用方式的补充说明 | 空 |
| `plan_source` | `system_default`、`agent_proposed`、`user_confirmed` | `system_default` |

角色含义固定为：

- `character`：保留人物/生物的身份、外貌、服装和标志性特征。
- `scene`：参考空间关系、建筑、材质、光照和环境气氛。
- `prop`：参考物件的形态、结构、材质和颜色。
- `style`：只参考线条、色彩、纹理和光照，不复制主体与构图。
- `auto`：作为通用视觉灵感，由设定生成与后续 Premise 选择共同判断。

引用图只直接影响视觉设定与图片生成，不自动改写故事情节。用户若希望图片内容影响故事，仍需在故事描述或 `instruction` 中明确表达。

### 首版流程

1. 用户在首页选择图片；系统给出无阻塞默认计划，用户可按需展开编辑。
2. 创建会话保存文件清单与引用计划；刷新、恢复、重选项目时保持一致。
3. 项目激活时把文件绑定和引用计划一起写入 Project DB，不复制对象存储内容。
4. Project Setup 卡片显示缩略图、角色、标题和是否参与 YOLO。项目仍是草稿时可修改；任何修改都增加 Setup revision。
5. 用户确认创建方案后，Agent 只能通过可信的创建会话上下文启动 YOLO，不能自行提交任意文件 UUID。
6. YOLO v6 在创建时从 Project DB 加载并冻结完整引用计划；被排除的项目也保留在快照审计信息中，但 Worker 只消费 included 项。
7. `premise` 步骤先把 included 文件注册为来源型 Premise Assets，再合成一张确定性的带标签参考板。
8. Premise 设定图任务把参考板作为一个 `imagegen.ImageInput` 发送给现有供应商；之后继续走现有设定拆解、分镜和图片生成链路。
9. 后续 Comic 生成可从 Premise 候选中选择这些来源型 Asset；不新增第二套图片生成系统。

### 实施阶段

按以下顺序提交，保证每一阶段都能独立迁移和回滚：

1. 数据模型、迁移、读取 DTO 与旧数据默认值。
2. 首页创建 manifest、恢复逻辑与 Project Setup 编辑接口。
3. Agent 合约、确认摘要和 YOLO v6 快照。
4. Premise 来源 Asset 导入、引用板与图片任务接入。
5. 后续 Section 引用选择增强、错误体验、文档与完整回归。

## data_model

### App DB：创建会话来源

新增迁移 `db/migrations/app/20260831000005_add_project_creation_reference_plans.{up,down}.sql`，为 `project_creation_session_references` 增加：

- `reference_role TEXT NOT NULL DEFAULT 'auto'`，CHECK 为五种合法角色。
- `title TEXT NOT NULL DEFAULT ''`。
- `instruction TEXT NOT NULL DEFAULT ''`。
- `include_in_yolo INTEGER NOT NULL DEFAULT 1`。
- `plan_source TEXT NOT NULL DEFAULT 'system_default'`，CHECK 为三种来源。

该迁移通过 SQLite 表重建保留原有 ID、状态、索引和外键；新增字段不能只依赖 `ALTER TABLE` 后遗漏 CHECK。旧行 title 保持空字符串，由读取 DTO 展示文件名派生标题，但不会被旧 YOLO 快照消费。

服务层规范化并验证：

- `title` 为空时使用文件名去扩展名，最多 160 个 Unicode 字符。
- `instruction` 最多 2000 个 Unicode 字符。
- 沿用现有约束，只接受 PNG、JPEG 与 WebP；非图片继续返回 `project_creation_invalid`，不把它们混入创建 reference manifest。
- 创建会话最多 16 张图片（无论 included 与否）；数量验证在写会话与恢复时都执行。
- `plan_source` 不接受客户端直接赋值：全部计划字段等于默认值时由服务端写 `system_default`；只要角色、标题、instruction 或 include 明确偏离默认值，就写 `user_confirmed`。
- 创建请求幂等比较包含以上计划字段，避免同一幂等键悄悄改变语义。

### Project DB：项目事实状态

新增迁移 `db/migrations/project/20260831000034_add_project_creation_reference_plans.{up,down}.sql`，扩展 `project_creation_reference_files`：

- `reference_role`、`title`、`instruction`、`include_in_yolo`、`plan_source`，约束与 App DB 一致。
- `premise_asset_id BIGINT NULL REFERENCES premise_assets(id) ON DELETE SET NULL`。
- `imported_at DATETIME NULL`。
- `updated_at DATETIME NOT NULL`；旧行以 `created_at` 回填。
- 为 `premise_asset_id IS NOT NULL` 建立 partial unique index；一个来源 Asset 只能绑定一个 project reference。

该迁移同样重建表并完整恢复现有 unique/index/foreign-key 约束，down migration 也需要保留原有行和顺序。

项目激活后的 Project DB 是引用计划的事实来源；App DB 仅用于创建恢复与跨库绑定。重复执行绑定时只校验不可变身份：项目、创建会话、位置和文件，不用 App DB 的旧值覆盖用户已在 Setup 中修改的计划。

### Premise Asset 映射

included 引用按下列规则复用同一个 `files.id`，不复制文件对象：

| reference role | premise asset type |
| --- | --- |
| `character` | `character` |
| `scene` | `scene` |
| `prop` | `prop` |
| `style` | `reference` |
| `auto` | `reference` |

- variant 使用现有 `source_type = manual`，增加事件 `asset_created_from_project_reference` 表达真实来源，避免为枚举扩表重建 SQLite 表。
- tags 至少包含 `project-creation-reference` 与 `reference-role-<role>`；summary 合并角色说明和用户 `instruction`。
- 标题冲突时确定性追加 ` · 参考图 <position>`；position 沿用现有 1 到 16 的服务端顺序，重试必须得到同一 Asset。
- 导入与 `project_creation_reference_files.premise_asset_id` 绑定在一个 Project DB 事务内；已绑定时直接复用。
- 删除/归档 Premise Asset 不删除来源文件或创建绑定；重试发现绑定 Asset 已归档时恢复或新建明确版本，并记录事件。

新增内部专用方法 `BindProjectCreationReferenceAsset`，只允许绑定当前项目、`project_chatbot_reference` purpose、未删除且已 finalize 的图片。不要放宽 `CreatePremiseAssetFromFile` 对 Chat image_gen 输出的安全约束，也不开放一个可传任意 `file_uuid` 的公开写接口。

### 生成快照版本

YOLO 快照从 v5 升至 v6，新增：

```json
{
  "creation_session_uuid": "uuidv7-or-empty",
  "creation_references": [
    {
      "reference_uuid": "uuidv7",
      "file_uuid": "uuidv7",
      "position": 1,
      "reference_role": "character",
      "title": "小狐狸",
      "instruction": "保留红围巾和耳尖颜色",
      "include_in_yolo": true,
      "plan_source": "user_confirmed"
    }
  ]
}
```

- 快照只含公开 UUIDv7，不含数据库 ID、本地路径、临时 URL 或图片字节。
- 快照中的 `reference_uuid` 统一指 Project DB binding 的 `project_creation_reference_files.uuid`；跨库来源 UUID 仅保留在 binding 内部，不作为 Project API 资源标识。
- 创建时按 `position, reference_uuid` 排序；重试和进程恢复只读快照，不重新读取可变 Setup 计划。
- `runYoloStep` 继续接受 v1-v6。v1-v5 和没有创建会话的 v6 快照走现有纯文本路径。
- Premise setting 的生产任务快照升级到 v3，冻结 included `ReferenceFiles`、角色、标题、instruction、合成器版本和引用提示词版本。
- Production Worker 的版本门禁只新增“v3 + `premise_setting_generation`”组合；其他非 Comic v3 快照仍判为损坏，避免泛化版本校验。

### 引用板文件

在 Files registry 增加 purpose `premise_reference_board`，namespace 为 `premise/reference-boards`：

- 由 included 来源图按位置生成一张 PNG，最多 16 个 tile；每个 tile 显示角色与标题。
- 从 `internal/jobqueue/section_premise.go` 抽出通用合成器，但保持现有 Section Premise v1 像素输出不变。
- 新合成器标记为 `creation_reference_board/v1`；固定画布、间距、字体回退、缩放和 EXIF 方向处理，保证相同输入得到相同输出。
- 合成成功后先把 board File UUID 写进 generation step 的 `output_json`，再调用供应商；重试优先复用该 File。
- 任务完成时用 JSON merge 保留 `reference_board_file_uuid`，不得用最终 setting 字段覆盖整个 `output_json`。
- GC 通过快照/step JSON 中的公开 File UUID 识别引用；需增加保留与清理测试。
- `project_creation_reference_files.file_id` 也必须加入 `premiseVariantFileRetained` 的直接保留条件，确保被排除或尚未导入 Premise 的创建引用不会被 GC 误删。

## api

### 创建会话

扩展现有创建 manifest 中每个 reference 的输入：

```json
{
  "original_filename": "fox.png",
  "mime_type": "image/png",
  "byte_size": 123456,
  "reference_role": "character",
  "title": "小狐狸",
  "instruction": "保留红围巾",
  "include_in_yolo": true
}
```

响应中的 reference 额外返回服务端派生的 `plan_source`。保持现有 REST 路径与统一响应信封；旧客户端不传新字段时使用服务端默认值，不得依赖前端补默认值才能成功，也不得接受客户端伪造来源。

### Project Setup 读取

`GET /api/v1/projects/:project_uuid/project-setup` 的单对象 `data` 增加只读 `reference_plan`：

```json
{
  "reference_plan": {
    "items": [
      {
        "uuid": "reference-uuidv7",
        "file_uuid": "file-uuidv7",
        "position": 1,
        "reference_role": "character",
        "title": "小狐狸",
        "instruction": "保留红围巾",
        "include_in_yolo": true,
        "plan_source": "user_confirmed",
        "premise_asset_uuid": null,
        "thumbnail_url": "/api/v1/..."
      }
    ]
  }
}
```

响应不得包含内部 ID、磁盘路径和永久绕过鉴权的对象 URL。缩略图复用现有 Files 媒体读取入口。

### Project Setup 编辑

新增资源化端点：

`PATCH /api/v1/projects/:project_uuid/project-setup/references/:reference_uuid`

请求：

```json
{
  "expected_revision": 4,
  "reference_role": "style",
  "title": "水彩画风",
  "instruction": "只参考笔触与配色",
  "include_in_yolo": true
}
```

- 字段均可选，但至少提供一项计划变更。
- 只允许 `setup_status=draft` 且引用必须属于当前项目；draft record 为 `draft`、`pending_confirmation` 或 `failed` 时均可修改并恢复到 `draft`，只有 finalized/ready 不可变。
- 比较 `expected_revision`，成功后增加 revision、恢复 draft 状态并清除旧 setup error，再返回完整 `SetupState`，不包 `{ item }`；旧危险操作确认会因 revision 不匹配而自然失效。
- 使用现有统一失败信封；revision 冲突与 ready 锁定返回稳定业务错误码。
- 写入成功后经 `/api/v1/ws` 发送 setup 变更提示；前端失效 TanStack Query 后 REST 重读事实状态，不增加 HTTP 轮询。
- 浏览器直改把该项 `plan_source` 标为 `user_confirmed`；Agent in-process route 通过可信调用上下文标为 `agent_proposed`，客户端不能自行伪造来源。Setup finalization 在同一事务中把最终计划统一标记为 `user_confirmed`。

### Agent 工具边界

扩展内部 `CreateYoloInput`，仅增加 `json:"-"` 的可信 `CreationSessionUUID`，不开放 `file_uuids` 或完整 reference payload：

- `RouteYoloWorkflowCreate` handler 从现有 `tc.BootstrapCreationSessionUUID` 注入 creation session UUID。
- YOLO service 用 session UUID + project ID 在服务端加载引用计划并冻结 v6 快照。
- Agent 尝试引用其他项目文件时没有可表达的工具参数，因此无法越权。
- 直接从项目页手动启动 YOLO 时 session 为空，保持纯文本行为。

### 确认摘要

Agent 的 Project Setup route registry、strict schema、projector、API doc 与确认文案加入“视觉参考”段落，逐项列出标题、角色、是否采用和 instruction。任何引用计划修改都会增加 revision，使旧 finalization fingerprint/确认绑定失效；用户必须基于新 revision 再次确认后才能创建 YOLO。

Agent 只能说明“已收到图片及用途”，不得声称已经看过图片像素。首版不要求 Agent 自动分类图片；只有用户文字与现有角色明显冲突时，才允许合并为一次澄清提问。

## ui

### 首页 Composer

- 只在“新建项目”模式的附件区显示专用 `CreationReferenceEditor`，不要改变通用 Chat `ReferenceStrip` 的语义。
- 图片添加后默认 `include_in_yolo=true`、`reference_role=auto`、标题为文件名；默认值无需阻塞用户创建。
- 每项支持缩略图、包含开关、角色下拉、标题和 instruction；高级字段默认折叠。
- 非图片继续沿用当前“仅支持图片”的选择拒绝与错误提示，不引入第二种创建附件语义。
- sessionStorage 创建 checkpoint 保存完整计划；恢复后以服务端创建会话响应校准。
- 在已有项目中使用首页 Composer 时维持普通 Chat 上传路径，不提交创建引用计划。

文案从“上传灵感”调整为明确承诺边界，例如：“参考图会用于建立人物、场景或画风设定；故事情节仍以你的文字为准。”

### Project Setup 卡片

- 在确认按钮前展示“视觉参考”区域，显示缩略图、角色标签、标题、instruction 和是否参与 YOLO。
- Draft 状态可以原位编辑；保存时携带当前 revision，成功后依赖 Query invalidation + REST 重读。
- Ready/运行中状态只读，并显示“已冻结到本次生成”。
- YOLO premise 步骤失败时显示具体参考图错误和“重试当前步骤”，不建议用户重新创建整个项目。
- 所有 include/role 控件提供 label、键盘操作、focus-visible 和 `aria-pressed`/selected 状态；组合 hover 规则写在基础状态之后，避免选中态吞掉 hover 反馈。

### 国际化与可观察状态

- 补齐简体中文和项目已有其他 locale 的角色、帮助、校验、失败和冻结文案。
- Web 不从本地附件状态推断 YOLO 是否使用了图片；只展示 REST 返回的 Setup/Yolo snapshot 派生状态。
- WS 只传公开 UUIDv7 与变更提示，不发送内部 ID 或整张计划 payload。

## jobs

### YOLO premise 步骤

在 `runYoloPremise` 中按以下顺序执行：

1. 从 v6 快照解析 included 引用并逐个验证 File 存在、已 finalize、MIME 为支持的图片。
2. 幂等导入/复用来源型 Premise Assets，并写回 reference binding。
3. 通过 `json:"-"` 的内部 task input 创建或恢复 setting generation task；公开 Production API 不接受任意引用文件，任务快照包含冻结引用。
4. 合成或复用 `creation_reference_board/v1`。
5. 用 board 作为唯一新增的 `imagegen.ImageInput` 发起设定图生成。
6. 继续现有 setting commit 与 breakdown。

不能因为提前导入了来源 Asset 而跳过 setting breakdown。调整判断为：

- v6 且存在 included 创建引用时，始终执行本次 setting 对应的 breakdown，用于生成尚缺失的设定 Asset。
- v1-v5、v6 无引用和直接 UI 任务保留当前“已有 Asset 时不重复拆解”的兼容逻辑。

### 图片提示词

在 Premise prompt catalog 增加只在存在引用时追加的 `setting_reference_usage` 片段，并与 production task 一起冻结。提示词包含：

- 每张图的角色、标题和 instruction。
- 对角色的固定解释。
- 明确“参考图片内容，不得复制参考板的网格、标签、留白或排版”。
- setting 输出仍遵循原有无文字、角色转面、场景/道具设定要求。

无引用任务不追加该片段，确保旧路径 prompt 不发生无意义变化。

### Breakdown 冲突保护

当前 breakdown 若命中同名 Asset 会提交并选中新生成 candidate。来源型引用需要保护：

- 若目标 Asset 与 `project_creation_reference_files.premise_asset_id` 关联，保留用户来源 variant 为 current。
- breakdown 记录 `breakdown_matched_project_reference` 事件和 checkpoint，不覆盖原图。
- 在调用 `CommitReader` 持久化 crop 之前完成来源绑定检查；`premiseAssetForBreakdownTask` 同时识别该事件，重试直接返回已匹配 Asset，避免生成孤儿 File。
- 仍允许为其他缺失人物、场景和道具创建 generated Asset。
- retry/restart 根据 reference binding、task UUID 和 checkpoint 去重，不产生重复 Asset、variant 或事件。

### 后续 Comic 引用选择

扩展内部 `PremiseAssetReference` 候选元数据：`asset_type`、`summary`、`reference_role`。Section 选择提示词除标题外展示这些字段，让模型能区分“画风参考”和“故事实体参考”；summary/instruction 在选择 prompt 中按字符上限截断并受总 prompt 上限约束，不能把 200 个 12 KiB summary 全量拼接。

- 仍遵守每张图最多 12 个 Premise 文件的现有限制。
- 来源 Asset 与生成 Asset 使用同一选择、合成和图片输入链路。
- 明确选择列表与自动选择逻辑保持现有优先级；首版不强制每页携带所有 `style` 图，避免挤占页面实体参考名额。
- setting 图和拆解出来的 Assets 已承接全局画风；如后续数据证明风格漂移，再单独设计“全局强制 style board”，不在本计划隐式扩张。

### 错误与重试

增加稳定错误分类并映射到现有 Workflow failure：

- `yolo_reference_unavailable`：来源 File 不存在、未 finalize、无法读取或 MIME 不匹配。
- `yolo_reference_board_failed`：解码或合成引用板失败。
- `image_reference_unsupported`：供应商明确拒绝图片输入能力。

以上错误都不得捕获后转成纯文本请求。已提交的来源 Asset 和 reference board 允许在取消后保留；重试通过绑定与 step output 复用。事件日志记录 reference/file/board 的公开 UUID、角色、顺序、数量与 composer version，不记录内部 ID、绝对路径、图片字节或临时鉴权 URL。

## others

### 兼容与迁移策略

- 数据库新增字段均有默认值；旧创建引用迁移后表现为 `auto + included`，但只有新建的 YOLO v6 才会消费它们。
- 不回填、重放或改变已存在的 YOLO v1-v5。
- 不更改 Workflow kind 和六步结构，`premise` 进度含义保持兼容。
- 不改变无创建会话的手动 YOLO、普通 Chat 文件引用和现有 Section Premise 合成输出。
- 新 Files purpose 必须纳入 registry、元数据白名单、清理、导出/备份和测试夹具。
- 如供应商适配器宣称成功但没有消费 image input，应视为适配器缺陷并失败，不提供静默 feature flag 回退。

### 后端测试

- App/Project migration up/down、CHECK 约束和旧行默认值。
- 创建 manifest 校验、幂等冲突、checkpoint/recovery 和非图片拒绝。
- 跨库绑定只复制初始计划，重复绑定不覆盖 Project DB 编辑。
- Setup reference PATCH：归属校验、revision 冲突、ready 锁定、统一 JSON 信封、仅公开 UUID。
- Agent schema/guide/projector：确认摘要包含计划，Agent 无法注入任意 file UUID。
- YOLO v6 快照冻结、included/excluded、直接 UI 无引用，以及 v1-v5 恢复测试。
- 来源 Premise Asset：共享同一 file ID、角色映射、标题碰撞、归档恢复、重试不重复。
- 有来源 Asset 时仍运行 setting breakdown；同名命中不覆盖来源 current variant。
- 引用板：顺序、标签、中文字体回退、EXIF、透明图、最大 16 张、确定性、损坏图片失败。
- setting Worker 确认只追加一个 board image input；无引用时保持零 image input。
- 两个图片供应商的请求序列化测试，确认 image bytes/URL 实际进入请求。
- 取消、重试、服务重启后不重复 Asset、任务或 board，并验证 GC 不误删。
- Section 选择提示词包含 asset type、summary 和 reference role。

### 前端测试

- 新项目 manifest 与 sessionStorage checkpoint 保存完整引用计划；刷新恢复不丢字段。
- 默认值、角色切换、排除、标题/instruction 校验和非图片拒绝状态。
- 已选项目的普通 Chat Composer 行为不受影响。
- Setup 卡片携带 revision 编辑，成功后 invalidation，冲突后展示服务端事实状态。
- Ready 状态只读、失败重试入口、键盘与可访问性状态。
- 简体中文与其他 locale 文案键完整，现有首页创建流程快照/交互测试更新。

### 验证命令

实施完成后运行：

```bash
gofmt -w <changed-go-files>
go test ./internal/projectcreation ./internal/project ./internal/agent ./internal/httpapi ./internal/production ./internal/jobqueue ./internal/files
go test ./...
pnpm --dir web test
pnpm --dir web build
```

本计划不运行 Cargo 或任何 Rust 编译、检查与测试命令。

### 发布与观测

- 先上线向后兼容的 DB/读接口，再上线写入新 manifest 的 Web，最后启用 YOLO v6 消费。
- 统计 `included_reference_count`、role 分布、board compose latency、供应商引用失败率、Premise retry 率和来源 Asset 命中率；指标不得带文件名或 instruction。
- 事件审计至少覆盖 `creation_references_frozen`、`asset_created_from_project_reference`、`creation_references_composed`、`breakdown_matched_project_reference`。
- 人工 smoke case：同一故事分别用“无参考图”“人物图”“画风图”“人物+场景+画风”生成，核对快照、请求日志和最终视觉差异。

### 非目标

- 不让 Chat Agent 自动看图、识别图片或自动决定角色。
- 不因为图片像素自动改变故事情节。
- 不新增图片供应商、Workflow kind、轮询机制或第二套 Premise 系统。
- 不复制来源文件对象，不在 API/WS 暴露内部 ID。
- 不实现全局强制 style board；该能力依据首版观测另行规划。

## prds

实施完成后同步：

- `docs/prds/projects/`：项目创建会话、Project Setup reference plan、revision/fingerprint 规则。
- `docs/prds/workflows/`：YOLO v6 快照、premise 引用处理、失败与重试语义。
- `docs/prds/premise_assets/`：来源型 Asset、角色映射、breakdown 冲突保护与 Section 候选元数据。
- `docs/prds/files/`：共享 file ownership、`premise_reference_board` purpose、GC 保留规则。
- `site/` 对应用户文档：参考图角色、只影响视觉不自动改写故事、失败重试说明。
