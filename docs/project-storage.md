# Local-first 项目存储与生命周期

## 数据边界

Lumi 使用两个互不关联的 SQLite 范围：

- 应用库 `{LUMI_DATA_DIR}/lumi.sqlite` 保存最近项目索引。`root_path` 是机器本地发现信息，可以失效；应用库不保存任何项目业务数据。读取最近项目列表只访问应用库和内存中的已打开项目注册表，不探测项目文件夹。
- 每个项目的 `{project_root}/project.sqlite` 保存唯一项目记录与项目内实体。项目库不能引用应用库 `id`，也不存在跨库事务。

数据库内部主键与外键使用 `INTEGER PRIMARY KEY AUTOINCREMENT` 和 Go `int64`。API、URL、React 状态与实时消息只使用 UUIDv7。

## 创建

`ProjectManager.Create` 先规范化并解析父目录，再依次用 `os.Mkdir` 原子占用基础目录名、`-2`、`-3`，最多尝试到 `-1000`。普通文件、目录和其他已存在节点都只表示候选已占用；创建流程不做存在性预检，也不会修改、接管或删除既有候选。并发调用依靠文件系统原子创建获得不同路径，失败造成的编号空缺可以保留。

目录后缀只影响本机 `root_path`，项目展示名称、项目库名称和最近项目名称仍使用用户输入的原名。候选耗尽、父目录不可写或其他占用错误不会改变任何已打开项目、最近项目索引及生命周期状态。候选全部占用时返回 HTTP 409 `project_directory_name_exhausted`。

创建成功包括：

1. 写入 `README.md`、初始 `STORY.md`、`project.sqlite` 与目录契约。
2. 获取 `.lumi/project.lock` 的操作系统级独占锁。
3. 执行内嵌 project migrations。
4. 在一个项目库事务中创建唯一 `projects` 记录与默认 `local_user` actor；两者都有 UUIDv7。
5. 在应用库记录最近路径，最后将该 store 加入按项目 UUID 管理的已打开注册表；其他项目不受影响。

只有 `os.Mkdir` 成功占用的候选会注册失败清理。后续任一步失败只会删除本次调用新建的目录，不会删除调用前已存在或其他并发调用已占用的文件和目录。

## 打开与身份

打开已有目录时顺序固定为：路径绝对化与清理 → 解析根目录 symlink → 验证 Goal 01 基础目录与读写权限 → 以只读连接读取项目 UUID/format/schema version → 与最近项目 UUID 比对 → 获取该目录的项目锁 → 幂等补齐 `.lumi/quarantine/` 等后加受管目录 → 迁移 → 打开 `ProjectStore` → 有上限的 Asset reconcile → 启动该项目的 River client。这个流程不停止或关闭其他项目。

最近路径即使存在，也不能代表项目身份。列表中的未打开记录统一标记为 `recent`，表示可以尝试打开但尚未验证文件系统状态。正式打开时，如果该路径现在包含另一个 UUID，Lumi 返回 `project_identity_mismatch` 并保留原索引。重复打开相同 UUID 与规范路径是幂等操作；同 UUID 的并发打开合并为一次锁获取、迁移、open hooks 与 Runtime 启动，不同 UUID 可以并行打开。

## migration 与版本

`format_version` 表示项目目录/数据模型的兼容格式，`schema_version` 记录应用完成的 project migration 版本。若任一版本比当前二进制支持的版本更新，打开流程在写入前返回 `project_format_too_new`。

存在待执行 migration 时，Lumi 通过 SQLite online backup API 创建 `.lumi/backups/project-before-*.sqlite`。若 migration 失败，先关闭 migrator，再通过 SQLite restore API恢复；不会用普通文件复制覆盖打开中的数据库。

## 锁与关闭

Lumi 在一个进程内可以同时打开多个项目，每个项目各自持有 `ProjectStore`、River Runtime 和 `.lumi/project.lock`。lock 文件中的 JSON 仅用于向本机用户展示 PID、主机和项目 UUID；互斥性来自该项目目录上的操作系统文件锁。进程正常关闭会解锁，崩溃后内核也会释放锁，因此遗留 lock 文件可以安全复用，不需要按时间猜测或删除。

关闭某个项目时，注册表先把该 entry 标记为 draining 并拒绝新的请求 lease，等待该项目已有请求退出，再调用它自己的 `StopProject`。Runtime 未成功停止时不得关闭数据库。停止成功后才执行 `PRAGMA wal_checkpoint(TRUNCATE)`、关闭数据库句柄并释放锁；其他项目的请求与任务不经过这个关闭临界区。

打开和关闭属于服务端生命周期，不是浏览器标签页共享的“当前项目”状态。每个标签页的当前上下文只由 `/projects/:project_uuid` URL 决定；进入项目路由时前端幂等调用 `PUT /api/v1/open-projects/:project_uuid`，已回收项目会透明重开，打开新项目不会关闭旧项目。服务端不保存“最后聚焦项目”，也不提供 `/api/v1/current-project`。

已打开集合使用资源化 API：`GET /api/v1/open-projects` 返回 `{ items }`，`POST /api/v1/open-projects` 按 `root_path` 打开，`PUT /api/v1/open-projects/:project_uuid` 从最近项目索引幂等打开，`DELETE /api/v1/open-projects/:project_uuid` 只关闭目标项目。项目 Summary 使用 `open` 与 `status: open|recent`，允许多项同时 `open: true`。已知但未打开的项目业务请求返回 HTTP 409 `project_not_open`；HTTP 404 `project_not_found` 只表示最近记录或真实目录不可用。

同一 WebSocket 可以同时加入所有已打开项目的 `project:{project_uuid}` topic。授权 join 时会原子取得该项目的 Presence lease，并在 leave、重复 join 被替换或断线时释放，避免授权与回收之间的竞态。每个项目分别计算空闲时间：最后一个 Presence 和请求 lease 释放后，服务端保留 5 分钟冗余时间；期间重新加入或访问会重置计时。超时后若 Story、Chat、Workflow、Asset Maintenance 或 Production 仍有 queued/running 工作，运行时继续工作，待任务全部结束后重新计满 5 分钟再关闭。`waiting_for_input` 不占用执行资源，不阻止回收；进程退出执行 `CloseAll`。

创建、打开、显式关闭和空闲回收统一在 system topic 广播 `open_project:changed`，payload 只包含公开的 `{ project_uuid, open }`。前端只更新目标 UUID 的生命周期与业务查询，不会触发其他标签页争抢全局状态。

## Story 数据与 `STORY.md`

Story 领域数据只存在项目库中。`chapters` 通过内部 `current_story_id` 指向 `chapter_stories` 的 current 正文；普通编辑总是追加新版本并以 revision 条件切换指针。Prompt 候选和 Story Profile 同样保留 append-only 历史。API、URL 和 React cache key 只使用这些记录的 UUIDv7。

current `project_story_profiles.story_md` 是结构化事实源，项目根目录 `STORY.md` 是可读投影。保存流程先提交 pending Profile，再把内容写入 `.lumi/tmp/`、fsync 并原子 rename；成功后记录导出 revision/hash。若进程在 rename 前退出，只要磁盘文件仍匹配上次导出 hash，打开项目时会安全重试投影。若磁盘 hash 已改变，则记录 `conflict`，不会自动覆盖文件或导入内容。

导入外部 `STORY.md` 会创建 `external_import` Profile 版本；用数据库版本重新生成只修复投影状态，不篡改历史。投影写入失败时数据库版本保持 pending，API 返回 `story_projection_failed`，后续读取仍能展示可恢复状态。

章节 `.txt`/`.md` 导入在单个项目库事务中创建 source、items、chapters 和正文版本。批次 request hash 用于幂等重试；原始文件先通过 Asset Store 提交，`story_source_items.file_id` 使用内部外键引用正式 File。业务失败会软删除已提交的补偿 Asset，不保存源绝对路径，也不允许 Story 代码直接写入 `assets/`。

## 路径选择边界

`ExistingDirectorySelector` 与 `NewProjectParentSelector` 是两个独立授权接口。开发 Web UI 的显式绝对路径只是本地开发适配器；浏览器目录上传不实现这些接口。桌面平台应提供可信的原生目录选择实现。

项目内文件通过 `ProjectStore.ResolvePath` 解析。它拒绝绝对路径、`..`、NUL、根目录越界和路径中的 symlink。后续 Goal 的 handler 应按 URL 中的项目 UUID 从带请求 lease 的 `ProjectStore` 获取数据库与根句柄，不得重新接受任意 filesystem root。

## 移动、重新定位和移除索引

项目被服务端关闭后可以整体移动或复制项目文件夹。切换页面或关闭单个浏览器标签页不等于立即关闭项目；需要文件系统级静默时应退出 Lumi，或由未来的移动、复制与备份能力显式请求受控静默点。重新定位必须读取新位置的项目数据库并验证 UUID，关闭并重开的范围仅限目标 UUID，成功打开后才更新应用库路径。磁盘离线、目录丢失、权限不足和身份不匹配只在打开或重新定位时检查并返回操作错误；最近项目列表保留原记录，不缓存这些文件系统状态。

`DELETE /api/v1/recent-projects/:project_uuid` 仅删除应用库索引。它永远不删除项目文件夹；真实项目删除必须是未来单独设计且显式确认的能力。
