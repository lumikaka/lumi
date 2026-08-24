# 项目 — 故事总纲版本与STORY投影

## overview

该 Feature 把项目总纲保存为 `project_story_profiles` 的追加式版本，并把 current `story_md` 安全投影为项目根 `STORY.md`。数据库记录是事实源；磁盘文件是可读投影，因此外部编辑、原子写入中断和投影失败都不会覆写已知历史。

用户可手动编辑、导入外部 `STORY.md`、从数据库重新生成投影，或由 AI 根据章节重建总纲。投影发现磁盘 hash 与已知导出 hash 不一致时进入 `conflict`，必须由用户选择后续操作。

## data_model

每个版本保存公开 `uuid`、`version_no`、`revision`、`story_md`、内容 hash、来源和投影状态。`is_current` 在同一项目中唯一；历史内容不可更新。`exported_revision`、`exported_hash` 和 `observed_file_hash` 记录最后成功投影及外部修改观察结果。

## api

| 接口 | 方法 | 说明 |
|---|---|---|
| `/api/v1/projects/:project_uuid/story-profile` | GET / PUT | 读取或写入当前总纲。 |
| `/api/v1/projects/:project_uuid/story-profile/versions` | GET | 返回总纲版本历史。 |
| `/api/v1/projects/:project_uuid/story-profile/imports` | POST | 导入外部 `STORY.md` 并创建新版本。 |
| `/api/v1/projects/:project_uuid/story-profile/projection` | POST | 由当前数据库版本重新生成可读投影。 |
| `/api/v1/projects/:project_uuid/story-profile/generations` | POST | 创建总纲生成任务。 |
| `/api/v1/projects/:project_uuid/story-profile/reconstructions` | POST | 根据章节重建总纲任务。 |

## ui

| 页面 / 入口 | 说明 |
|---|---|
| `/projects/:project_uuid/overview/profile` | 编辑当前总纲、查看版本与投影状态，并触发导入、重建或重新投影。 |

## jobs

| Job / Worker | 触发条件 | 策略 |
|---|---|---|
| `story_profile_generation` / `story_profile_from_chapters` | 用户请求生成或重建总纲 | 冻结 Prompt、模型和输入；结果以新 Profile 版本提交，不覆盖并发人工编辑。 |

## others

投影写入先在受控临时路径写入并同步，再原子 rename。任何 API 与实时 payload 都不得返回内部 ID、绝对路径或文件系统密钥。
