# 文件 — 资产GC与维护任务

## overview

该 Feature 以“先审计、后应用”的方式回收不再被任何结构化引用、历史、快照或受保护任务使用的 Object。GC 不能由业务删除直接触发物理移除：先生成 dry-run 计划，复核快照和引用，再由维护任务应用。

同一套维护任务也承载 reconcile、完整性扫描、缩略图重建和上传清理，并通过进度、事件、取消和终态让页面恢复操作状态。`chat_context_references.file_id`、`image_file_id` 与 `project_creation_reference_files.file_id` 均属于结构化保留来源，即使图片被用户排除、尚未导入 Premise、原业务资源已经永久删除或首页跨库检查点尚未完成也不能回收对应 Object。GC 还必须扫描 production task、Premise step 和 Workflow JSON 中的公开 File UUID，以保留有效参考板及历史快照。

## data_model

`asset_gc_plans` 保存项目、快照 hash、`dry_run|applied|stale` 状态、预计字节数和应用时间；`asset_gc_entries` 保存计划内对象 UUID、hash、受控路径摘要、字节数和引用摘要。`asset_maintenance_runs` 保存 kind、输入快照、进度、尝试、取消、错误和状态，事件表按 run sequence 追加。

## api

| 接口 | 方法 | 说明 |
|---|---|---|
| `/api/v1/projects/:project_uuid/asset-gc-plans` | POST | 创建 GC dry-run 计划。 |
| `/api/v1/projects/:project_uuid/asset-gc-plans/:plan_uuid/applications` | POST | 应用仍有效的计划。 |
| `/api/v1/projects/:project_uuid/asset-maintenance-tasks` | GET / POST | 查询或创建维护任务。 |
| `/api/v1/projects/:project_uuid/asset-maintenance-tasks/:task_uuid` | GET | 读取维护任务。 |
| `/api/v1/projects/:project_uuid/asset-maintenance-tasks/:task_uuid/events` | GET | cursor 读取维护事件。 |
| `/api/v1/projects/:project_uuid/asset-maintenance-tasks/:task_uuid/cancellations` | POST | 请求取消可取消维护任务。 |

## ui

| 页面 / 入口 | 说明 |
|---|---|
| `/projects/:project_uuid/assets` | 先预览 GC 范围和预计字节数，再显式确认应用；同时展示维护任务历史。 |

## others

GC 只处理受控项目根下的对象，不接受任意路径。Upload Cleanup 会在保留期后仅软删除既未被 `chat_context_references` 使用、也未被 `project_creation_reference_files` 绑定的 `project_chatbot_reference` File，随后仍由 GC dry-run/apply 完成物理回收；已成为首页创建绑定或 Chat Reference 的 File 不得进入该路径。Premise 资产永久删除会把 Reference 的业务目标 FK 置空，但保留 UUID、快照和冻结图片；物理回收仍须通过 Chat Reference、首页创建绑定、历史快照和全部业务引用的复检与审计计划。
