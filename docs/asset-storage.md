# Lumi 本地资产目录与 Asset Store 规范

## 目的

本规范定义 Lumi 项目内图片、文本附件、音视频、压缩包和生成产物的存储边界。目标是让项目文件夹可直接浏览、整体移动、复制、备份和离线打开，同时保证数据库记录与磁盘文件在进程崩溃、生成失败、重复导入和删除恢复后仍可校验、修复。

本规范适用于所有 Story 导入附件、Premise 设定图、设定资产、Comic Section 图片、缩略图和导出流程。业务模块不得直接读写 `assets/`；其中的写入、读取 URL、替换、删除、校验和清理必须经过统一 Asset Store。漫画导出 ZIP 是明确例外：它只进入项目根 `exports/`，由 Export 领域按固定 7 天保留期管理，不创建 Asset Store File/Object。

本文的目标仓库是 `/Users/qingyang/mine-work/lumi`。该仓库只用于理解旧有 Files/S3 和创作资产行为，不能被本规范的实施修改。

## 当前实现

Goal 03.5 已由 `internal/files` 的单一 service 实现。项目 migration 建立 `upload_stashed`、`file_objects`、`files`、integrity scan/finding、GC plan/entry 与 Asset maintenance task 表；Story 文件导入通过具体 `story_source_items.file_id` 引用正式 File。Echo handler、Story service 与 River worker 不直接写 `assets/` 或操作三张核心表。

打开项目时在锁内补齐受管目录并执行有上限的轻量 reconcile，之后才启动 River。耗时维护通过 `asset_maintenance` queue 执行，数据库副作用并发为 1；UI 的 Assets 页面可上传并幂等 finalize、用 `content_url` 预览、查看扫描摘要和触发安全 reconcile。漫画 ZIP 由同一项目 Runtime 的独立周期清理 worker 管理。REST/SQLite 是事实源，WebSocket 事件只触发刷新。

## 目录布局

```text
{project_root}/
├── README.md
├── STORY.md
├── project.sqlite
├── assets/
│   ├── story/
│   │   └── imports/
│   │       └── story-reference--019....txt
│   ├── premise/
│   │   ├── setting-images/
│   │   │   └── city-at-night--019....webp
│   │   └── assets/
│   │       └── main-character--019....webp
│   └── comic/
│       └── sections/
│           └── chapter-01-section-01--019....webp
├── exports/                    # 漫画 ZIP 的唯一存储位置；固定保留 7 天
└── .lumi/
    ├── cache/                  # 可删除并重建
    ├── thumbnails/             # 可删除并重建
    ├── tmp/                    # 未提交的 .part 文件和工具临时目录
    ├── quarantine/             # 待人工确认的损坏或不可信对象
    └── backups/                # migration 前的一致性数据库备份
```

### 目录语义

| 路径 | 是否事实数据 | 规则 |
|---|---:|---|
| `assets/` | 是 | 按可读 `key_path` 保存的已提交资产文件，用户可以浏览、复制和备份；项目移动时必须保留 |
| `exports/` | 否 | 漫画 ZIP 的唯一存储位置；按 `comic_exports.expires_at` 精确到期并由后台删除，用户自行放入的其他文件不受影响 |
| `.lumi/cache/` | 否 | 运行缓存，不得成为唯一数据来源 |
| `.lumi/thumbnails/` | 否 | 缩略图缓存，可按原对象重建 |
| `.lumi/tmp/` | 否 | 未提交临时文件；只能由当前项目实例使用 |
| `.lumi/quarantine/` | 条件性 | 发现异常后隔离，修复或确认删除前不得作为 active asset 使用 |
| `.lumi/backups/` | 恢复数据 | migration 前数据库备份，按保留策略清理 |

## 核心原则

1. 项目数据库保存元数据和相对路径，磁盘保存大二进制；数据库不保存项目外绝对路径。
2. 已提交资产文件不可变。内容变化必须创建新 object 和新的逻辑 asset/variant，不能原地覆盖旧文件。
3. 物理文件直接保存为 `assets/{key_path}`；`key_path` 面向本地项目可读，不采用 S3 bucket、环境前缀或纯 hash 分片目录。
4. SHA-256 用于内容去重和完整性校验，同内容在一个项目内只保存一份；hash 不决定用户看到的目录结构。
5. 逻辑 File/Asset 与物理 Object 分离。数据库表 `files` 保存业务可引用的正式逻辑文件，`file_objects` 保存不可变物理内容；相同内容可以有多个业务含义、原始文件名和引用历史。
6. 所有公开 Asset 使用 UUIDv7；内部表关联只使用自增 `id`。
7. 所有磁盘路径先经过根目录约束和符号链接检查；不得把用户输入直接拼到文件路径。
8. 数据库事务与文件系统发布不能组成真正的原子事务，必须通过 pending 状态和 reconcile 实现可恢复提交。
9. 漫画 ZIP 不做 object 去重，不创建 `files` / `file_objects`；`comic_exports` 中的大小、SHA-256、受控相对路径与到期时间构成其完整性和生命周期事实。

## 路径规则

### Asset key path

已提交文件直接保存在：

```text
assets/{key_path}
```

`key_path` 是相对 `assets/` 的项目内路径，不是 S3 object key，也不包含 `assets/` 前缀。推荐格式为：

```text
{purpose_namespace}/{safe_stem}--{file_uuid}.{canonical_ext}
```

例如：

```text
story/imports/story-reference--019....txt
premise/setting-images/city-at-night--019....webp
premise/assets/main-character--019....webp
comic/sections/chapter-01-section-01--019....webp
```

- `purpose_namespace` 由 Asset Store 根据 purpose 从服务端 allowlist 选择；业务代码和客户端不能传入任意目录。
- `safe_stem` 从 `display_name` 或原始文件名的 stem 清理得到，仅用于本地可读性；为空时使用 kind/purpose 的安全默认名。名称变化不触发已提交文件重命名。
- `file_uuid` 是创建正式逻辑 File/Asset 时预先生成的 UUIDv7，作为文件名后缀避免重名。若相同 hash 的 object 已存在，则复用已有 object 及其 `key_path`，不再创建第二份文件。
- `canonical_ext` 由实际检测后的 MIME 映射得到，不信任上传文件名扩展名。
- SHA-256 使用 64 位小写十六进制，保存在数据库中用于去重与校验，不写入默认文件名。
- 数据库只保存以 `/` 分隔的 `key_path`；运行时以 Asset Store 的 `assets/` 根句柄解析为当前操作系统路径。
- 禁止 `..`、空段、NUL、盘符、UNC、绝对路径和经过 symlink 后逃出 `assets/` 根目录的路径。
- 每段路径都必须清理分隔符、控制字符、系统保留名、尾随点/空格和超长名称，并使用跨平台一致的长度上限。
- `key_path` 按大小写不敏感方式保持唯一；不能依赖文件名大小写表达不同文件。
- `key_path` 一经提交保持稳定。purpose、标题和展示名称的后续变化只更新数据库，不移动文件。
- 用户可以在文件管理器中浏览、打开和复制 `assets/`，但手工覆盖已提交文件会被 reconcile 标记为 corrupt；新增内容仍应通过 Lumi 导入，不能仅靠复制文件建立业务引用。

### 临时路径

- 临时文件使用 `.lumi/tmp/{operation_uuid}.part`，工具目录使用 `.lumi/tmp/{operation_uuid}/`。
- `operation_uuid` 必须是服务端生成的 UUIDv7，不能采用用户文件名。
- 临时文件不得通过媒体路由访问，也不得被写入 active Asset 引用。
- 启动 reconcile 时清理超过保留期限且不对应 active operation 的临时对象。

## 数据模型

### `upload_stashed`

代表“先上传、后由业务动作消费”的本地暂存记录，对外 REST 资源名为 `asset_uploads`。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅供内部关联
- `uuid` — TEXT NOT NULL UNIQUE，UUIDv7；API、业务 finalize 和前端只使用该值
- `project_id` / `actor_id` — 内部 FK，用于项目和本地 actor 边界
- `purpose` / `original_filename` / `metadata_json` — 预期用途和安全展示信息
- `mime_type` / `canonical_ext` / `byte_size` / `sha256` / 媒体尺寸 — 暂存完成并校验后写入
- `state` — `receiving`、`ready`、`consuming`、`consumed`、`failed`、`expired`
- `file_object_id` / `finalized_file_id` — 可空内部 FK，用于崩溃恢复和 finalize 幂等
- `expires_at` / `consumed_at` / 时间字段

临时内容固定为 `.lumi/tmp/{upload_uuid}.part`，不保存或接受客户端路径。重复 finalize 已消费的 UUID 返回同一个正式 File；

### `file_objects`

代表项目内不可变的物理二进制对象。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅供内部关联
- `uuid` — TEXT NOT NULL UNIQUE，UUIDv7；仅诊断接口需要时公开，普通业务 API 不必暴露
- `sha256` — TEXT NOT NULL UNIQUE，64 位小写十六进制
- `key_path` — TEXT NOT NULL UNIQUE，相对 `assets/` 的 `/` 分隔路径
- `mime_type` — TEXT NOT NULL，实际检测值
- `canonical_ext` — TEXT NOT NULL
- `byte_size` — INTEGER NOT NULL，非负
- `width` / `height` — INTEGER，可空；像素尺寸
- `duration_ms` — INTEGER，可空；音视频时长
- `state` — TEXT NOT NULL，`pending`、`ready`、`missing`、`corrupt`、`quarantined`
- `created_at` / `verified_at` — 时间字段

约束和索引：

- `sha256` 唯一，`key_path` 按大小写不敏感方式唯一。
- `state, created_at` 索引用于 reconcile。
- ready object 必须有实际文件、匹配的 size/hash 和允许的 MIME。

### `files`

代表业务可引用的正式逻辑文件。产品和 REST 层仍可称其为 Asset。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT
- `uuid` — TEXT NOT NULL UNIQUE，UUIDv7，供 URL、JSON、前端和开放接口使用
- `file_object_id` — INTEGER NOT NULL FK → `file_objects.id`
- `kind` — TEXT NOT NULL，例如 `image`、`text`、`audio`、`video`、`archive`
- `purpose` — TEXT NOT NULL，例如 `premise_setting_image`、`premise_asset`、`comic_section_image`、`story_import`
- `original_filename` — TEXT，可空；只展示，不参与磁盘定位
- `display_name` — TEXT，可空
- `source_type` — TEXT NOT NULL，例如 `imported`、`generated`、`derived`、`exported`
- `source_file_id` — INTEGER FK → `files.id`，可空；裁切、转换等派生来源
- `metadata_json` — TEXT NOT NULL，默认 `{}`，必须满足 `json_valid`
- `actor_id` — INTEGER FK → `actors.id`，可空
- `created_at` / `deleted_at` — 时间字段

业务表必须通过具体 `file_id` 外键引用 `files.id`，例如 `premise_setting_images.file_id`、`premise_asset_variants.file_id` 和 `comic_image_variants.file_id`。不要用弱约束的任意 `resource_type/resource_id` 代替所有业务外键。

Premise setting、Premise variant 与 Comic image variant 使用 `FinalizeUploadWithBind`/`CommitReader`。领域记录与不可变 File 在 Asset Store 的最终 SQLite 事务中一起可见；后台生成的 bind 还会在同一事务中复核持久化取消状态，取消后的 Provider 返回不能移动业务 current 指针。漫画导出不使用这条提交链路，而是采用 `exports/` 专属的流式、同步和原子 rename 协议。

### 对外模型

普通 Asset JSON 至少包含：

```json
{
  "uuid": "019...",
  "kind": "image",
  "purpose": "comic_section_image",
  "original_filename": "section-01.webp",
  "mime_type": "image/webp",
  "byte_size": 123456,
  "width": 1080,
  "height": 3240,
  "content_url": "/media/projects/{project_uuid}/assets/{asset_uuid}/content"
}
```

- 不返回 `id`、`file_object_id`、River job ID、绝对路径或真实项目根路径。
- `key_path` 默认不对前端公开；前端只使用 `content_url`。
- metadata 必须按 purpose 做允许字段投影，不能原样把工具内部信息全部返回。

## 原子写入协议

### 导入或生成新对象

1. 服务端预先创建 operation UUID 和正式 File UUID，在 `.lumi/tmp/` 以 exclusive create 打开 `.part` 文件；两阶段手动上传同时创建 `upload_stashed` 记录。
2. 流式写入并执行大小上限；完成后关闭文件，必要时执行 `fsync`。
3. 检测真实 MIME、像素尺寸/时长、解码有效性和安全限制，计算 SHA-256。
4. 在短事务中查询相同 hash 的 ready object：
   - 已存在：复用该 `file_objects.id`，在最终事务中创建新的逻辑 File；最终事务成功后再删除临时文件或交给 reconcile 清理。
   - 不存在：生成可读且唯一的 `key_path`，创建或认领包含该 `key_path` 的 `pending` object 记录，提交事务。
5. 从 pending object 读取并再次校验 `key_path`，创建目标父目录，以同文件系统 hard-link + unlink 实现“不覆盖目标”的原子发布到 `assets/{key_path}`，并 fsync 目标目录；这一发布原语与 no-replace rename 具有相同可见性，但数据库事务仍与它分离。
6. 在短事务中再次校验 pending object，将其设为 `ready`，创建逻辑 File、业务 variant/event，并将对应 `upload_stashed` 标记为 `consumed`；这些数据库变更必须原子提交。
7. 提交成功后发布项目资源变更事件；发布失败不回滚已经持久化的事实，前端可通过 REST 恢复。

### 崩溃窗口

| 崩溃位置 | 可见状态 | Reconcile 行为 |
|---|---|---|
| 临时文件写入中 | 只有 `.part` | 超期后删除 |
| pending row 已提交、rename 前 | pending row，无 final file | 重试提交或删除 pending row |
| rename 后、ready transaction 前 | final file + pending row | 校验 hash 后标记 ready |
| final file 存在、数据库无 row | orphan file | 报告为可导入的本地文件；不得静默建立业务引用、隔离或删除 |
| ready row 存在、文件缺失 | missing | 标记 missing，保留逻辑引用并向 UI 报告修复状态 |

不得在跨网络 Provider 调用期间打开上述数据库事务。Provider 输出先进入临时文件，下载完成后才进入提交协议。

## 读取与媒体服务

### API

- `GET /api/v1/projects/:project_uuid/assets/:asset_uuid` 返回统一 JSON 信封中的 Asset metadata。
- 列表返回 `{items}`，过滤字段按 purpose/kind 明确定义。
- API mutation 只返回目标 Asset，不返回完整项目 workspace。
- API 只接收 UUIDv7，不接受数据库 ID 或磁盘相对路径作为资源标识。

### 二进制内容

- 二进制内容通过非 API 路由 `GET /media/projects/:project_uuid/assets/:asset_uuid/content` 返回，因此不受 JSON 信封约束。
- 每次请求先通过 ProjectManager 和数据库解析 Asset UUID，再读取已验证的 ready object；不得把 URL 路径映射成磁盘路径。
- 设置准确 `Content-Type`、`Content-Length`、`ETag`、`Last-Modified`、`X-Content-Type-Options: nosniff` 和合理缓存头。
- 大型音视频按需支持 HTTP Range；普通图片不需要通过 Go 一次性读入内存。
- `Content-Disposition` 中的展示文件名必须清理 CR/LF、分隔符和控制字符。
- SVG、HTML 等可执行/主动内容默认以 attachment 返回；未经明确的 sanitize 策略不得内联到项目 UI。

## 图片处理与缩略图

- 原图 object 永不因生成缩略图而被修改。
- 缩略图存放于 `.lumi/thumbnails/{source_sha256}/{processor_version}-{profile}.jpg`，profile 必须来自服务端 allowlist，例如 `grid_256`、`detail_1024`。
- 缩略图是缓存，不创建业务 Asset；缓存丢失时按源 object 重建。
- 图片解码前执行输入字节、宽高、总像素和支持格式限制，防止解压炸弹和异常资源消耗。
- 颜色空间、方向和透明背景转换必须确定性；处理器版本进入 thumbnail cache key 或 generation metadata。
- 裁切产生的新业务图片是新的 object + Asset variant，不放入缩略图目录。

## 引用、版本与删除

- 业务 current 指针只指向逻辑 File/Asset 或包含 `file_id` 的 variant，不能直接指向 `key_path`。
- 替换图片创建新 Asset/variant，再在一个数据库事务中切换 current 指针并写 event。
- 普通删除先设置逻辑 File/Asset 或业务资源的 `deleted_at`；回收站中的引用继续阻止物理 GC。
- 永久删除必须先移除或重定向所有具体 `file_id` 业务外键，再删除逻辑 File。
- 一个 object 没有任何 active、history、trash、snapshot 或其他结构化引用，并超过 grace period 后，才允许通用 GC 删除磁盘文件和 object row。
- GC 必须可 dry-run，输出 UUID、hash、`key_path`、引用摘要和预计回收字节数；不得根据文件修改时间直接删除 `assets/` 中的文件。

### Premise 资产永久删除边界

- `premise_assets` 的普通删除只设置 `deleted_at`；永久删除只接受已在回收站且 `expected_revision` 匹配的资产。
- 永久删除前检查引用该资产、variant 或 File UUID 的 queued/running Production task、Workflow，以及绑定该资产的 queued/in-progress/waiting Chat turn。活动使用存在时返回领域冲突，不做部分删除。
- 清空回收站按资产 UUID 稳定扫描：安全项删除，活动项以 `blocked_items` 返回，因此批量结果允许部分成功；所有领域变更仍在一个 SQLite 事务内提交。
- 删除 Premise 领域记录会级联其 tag、event 和 variant。随后逐个 File 检查结构化外键、派生 File、upload、任务输入/输出、Workflow、导出和 Chapter 快照 JSON 引用；仍被任何历史使用时保留，否则仅设置 `files.deleted_at`。
- 单项/批量结果中的 `file_soft_deleted_count` 只表示逻辑 File 进入删除状态，`retained_file_count` 表示仍有历史引用；两者都不代表磁盘 object 已被删除。
- 物理回收继续遵守 grace period、dry-run 和 GC plan；业务永久删除接口不得直接 unlink `assets/{key_path}` 或删除 `file_objects`。

## Reconcile 与维护

项目打开后执行轻量检查，显式维护操作执行全量检查：

1. 清理超期 `.part` 和工具临时目录。
2. 修复或报告 pending object。
3. 验证 ready object 是否存在及 size 是否匹配。
4. 按需重新计算 SHA-256，标记 corrupt。
5. 递归扫描 `assets/` 中没有 object row 的 orphan 文件，报告为可导入的本地文件；未经用户确认不得自动建立业务引用、隔离或删除。
6. 检查逻辑 File、variant 和 current 指针的 `file_id` 外键。
7. 重建缺失缩略图索引。

Goal 03.5 实现有上限的同步轻量 reconcile，并复用 Goal 03 已完成的 River runtime 执行全量 reconcile、完整性扫描、缩略图批量重建、暂存清理和 GC；每个项目只允许一个同类维护任务运行。

Reconcile 不能自动删除用户可能需要恢复的数据。corrupt、hash 冲突、未知格式和无法确认来源的 object 移入 `.lumi/quarantine/`，由 UI 给出导出、重新生成或确认删除选项。

## 导出规则

- 导出使用 Asset metadata 和 snapshot 中的 UUID/顺序读取内容，不遍历目录猜测业务顺序。
- Comic ZIP 只保存到项目根 `exports/`，不允许选择其他长期保存位置，也不写入 `assets/`、`files` 或 `file_objects`。
- 文件名根据 scope、Chapter、画册类型和 snapshot hash 生成安全前缀，并以 Export UUIDv7 结尾；同一快照到期后重建会得到新路径，不会与旧清理任务争用。
- ZIP 直接流式写入任务专属 `{name}.zip.part`，对压缩后字节同时计算大小和 SHA-256；文件 `fsync` 成功后原子 rename 为最终 ZIP，再同步 `exports/` 目录。
- ready、failed、cancelled 从各自终态起固定保留 7 天；重试清空旧 `expires_at`，新的终态重新计算。相同快照只复用 `status=ready AND expires_at>now` 的 ZIP，且复用不续期。
- 下载只通过 `/media/projects/:project_uuid/comic-exports/:export_uuid/content` 按 Export UUID 解析；支持 Range、ETag 和安全文件名。到期边界立即返回 410，记录清理后返回 404。
- 每项目 River Runtime 在启动时及每小时运行 active-state 唯一清理任务，每轮最多处理 1000 项。它只删除数据库登记路径、符合 Lumi UUID 命名的无记录旧 ZIP，以及到期 `.part`；用户放入 `exports/` 的其他文件不得删除。单项文件删除失败时保留 `expired` 记录供下轮重试。
- 保留期本身就是该派生产物的删除宽限期；`exports/` 不参与 Asset object 去重、业务引用或项目核心备份完整性判断。
- 升级前 `output_file_id` 仅为兼容列。普通 Asset 读取也按关联 Export 的精确到期时间拒绝旧 URL；清理器随后先软删除 `purpose=export` File，再用带审计计划与事务内引用复检的 export-only GC 回收物理 object。object 被其他 File 共享时只删除到期逻辑 File。

## 项目复制、备份与 migration

- 只有关闭项目或通过 Lumi 的“复制/备份项目”流程，才能保证 `project.sqlite`、WAL 和 assets 形成一致快照。
- 复制项目时先停止对应 River client 和新资产写入，等待进行中的提交结束，使用 SQLite backup API 创建数据库快照，再复制所有 ready objects。
- migration 只改变数据库时，备份 `project.sqlite`；涉及资产格式重写时，先创建可恢复 manifest，并采用 copy-on-write，不能原地批量覆盖 object。
- 打开复制后的项目时运行 integrity check 和轻量 reconcile，不因项目根绝对路径变化修改 Asset 记录。

## 安全边界

- Asset Store 的根目录句柄由 ProjectStore 创建，业务层不能传入任意 filesystem root。
- 所有操作在最终 open/rename 前再次验证 resolved path 位于项目根内，并拒绝 symlink 逃逸。
- 文件类型以内容检测为准，扩展名和浏览器 `Content-Type` 只作提示。
- 对上传字节、像素、解码时间、压缩展开大小、文件数量和批量总量设置限制。
- 外部工具只能访问本次 operation 的临时目录和显式输入文件，不能获得整个项目根写权限。
- 应用日志只记录 Asset UUID、hash 前缀和安全错误码；默认不记录项目根路径、Prompt 全文或文件内容。
- `/media` 只监听本地服务并校验 Origin/项目状态，不提供任意路径下载能力。

## WebSocket 事件

Asset 实时消息只通过 `/api/v1/ws`，使用 `topic/event/payload/ref/join_ref` 信封。

建议事件：

- `asset/created`
- `asset/updated`
- `asset/trashed`
- `asset/restored`
- `asset/missing`
- `asset/reconciled`
- `thumbnail/ready`

payload 只包含 project UUID、asset UUID、upload UUID、task UUID、状态和必要摘要，不包含 `id`、`file_object_id`、River job ID、相对路径或绝对路径。事件是刷新提示，REST/SQLite 才是事实源。

## 测试要求

- 使用真实临时目录和真实 SQLite 文件测试，不只 mock filesystem。
- 覆盖 `key_path` 生成、跨平台重名、路径清理、hash 去重、同 hash 并发导入、MIME/扩展名不一致、超限、无效图片和 symlink 逃逸。
- 在每个原子写入步骤注入崩溃，验证 reconcile 最终得到 ready、missing、quarantined 或安全删除结果。
- 覆盖软删除、恢复、永久删除、多引用、snapshot 引用和 GC grace period。
- 覆盖项目移动、项目复制、WAL checkpoint、migration 备份和缺失 object。
- 覆盖 `/media` 的 ETag、Range、缓存、nosniff、文件名清理和 UUID-only 路由。
- 覆盖 WebSocket payload 不含内部 ID 和磁盘路径。

## 明确禁止

- 禁止业务代码直接 `os.WriteFile` 到 `assets/`。
- 禁止数据库保存项目外绝对路径作为 Asset 的长期来源。
- 禁止覆盖已提交 object。
- 禁止把未经清理的原始文件名或外部 S3 object key 直接作为 `key_path`；原始名称只能参与生成安全的 `safe_stem`。
- 禁止把 `assets/` 直接暴露为 Echo static root。
- 禁止仅依据文件不存在就级联删除业务记录。
- 禁止仅依据数据库引用为零立即删除文件；必须经过 grace period 和可审计 GC。
- 禁止 API、WebSocket 或前端状态暴露内部 `id`、River job ID 或物理路径。
