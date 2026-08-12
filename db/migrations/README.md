# App and project database migrations

`app/` 与 `project/` 是两个完全独立的 golang-migrate 流：

- `app/` 只修改 `{LUMI_DATA_DIR}/lumi.sqlite` 中的应用级数据。
- `project/` 只修改项目根目录的 `project.sqlite`，不得引用全局库 ID。

使用 `lumi_ctl migrate create <app|project> <snake_case_name>` 创建成对 SQL 文件。SQLite migration 由 golang-migrate 自动包裹事务，不要写显式的 `BEGIN` 或 `COMMIT`。

项目 migration 只能通过 `ProjectManager` 的打开流程或 `lumi_ctl migrate project up <absolute_project_root>` 执行。两者都会先验证项目 UUID、获取进程锁，并在需要升级时通过 SQLite backup API 写入 `.lumi/backups/`；失败后用同一 API 恢复。不要通过复制打开中的 `project.sqlite` 创建备份。
