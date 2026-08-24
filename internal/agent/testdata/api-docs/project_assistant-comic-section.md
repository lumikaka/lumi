# Comic Section Agent Project API

以下全局已注册操作使用 `request_api` 调用。

## 方法与路径

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}` | 读取漫画 Section（`comic_section.get`）。 |

## 路径参数

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `project_uuid` | string (UUIDv7) | 是 | 当前 Run 绑定项目的公开 UUIDv7。 |
| `chapter_uuid` | string (UUIDv7) | 是 | 目标 Chapter 的公开 UUIDv7。 |
| `section_uuid` | string (UUIDv7) | 是 | 目标 Comic Section 的公开 UUIDv7。 |

## Query 字段

| 操作 | 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}` | 无 | - | - | 不要传 query。 |

## 请求体字段

| 适用方法 | 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}` | 无 | - | - | 不需要请求体。 |
| 全部 | `project_uuid` | string | 禁止 | 项目 UUID 只能出现在 URL Path 中，并且必须等于当前 Run 绑定项目。 |

## 响应字段

| 适用方法 | 字段 | 类型 | 说明 |
| --- | --- | --- | --- |
| 全部 | `success` | boolean | 是否成功。 |
| `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}` | `data` | object | 读取漫画 Section 操作的紧凑响应。 |
| `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}` | `data.uuid` | string | Comic Section 公开 UUIDv7。 |
| `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}` | `data.chapter_uuid` | string | 所属 Chapter 公开 UUIDv7。 |
| `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}` | `data.section_no` | integer | Section 在章节内的序号。 |
| `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}` | `data.title` | string | Section 标题。 |
| `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}` | `data.description_md` | string | Section 描述 Markdown。 |
| `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}` | `data.current_storyboard` | object \| null | 当前 Storyboard 版本及完整 Markdown。 |
| `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}` | `data.revision` | integer | 当前乐观并发版本。 |
| 全部 | `error` | object \| null | 失败时返回公开错误信息；成功响应不包含错误内容。 |

## 权限

| 方法与路径 | 项目权限 | 资源边界 |
| --- | --- | --- |
| `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}` | 可访问当前项目 | 当前项目范围；资源归属由领域服务校验 |

## 调用约束

| route_id | expected_revision | 异步 | 风险 | 需要 request_user_input | 幂等规则 |
| --- | --- | --- | --- | --- | --- |
| `comic_section.get` | 否 | 否 | `low` | 否 | - |

## 错误与调用示例

常见错误：`tool_validation`、`tool_not_allowed`、`not_found`、revision/state conflict。

- `comic_section.get`：`{"method":"GET","response_filter":".data | {uuid,chapter_uuid,section_no,title,description_md,current_storyboard,revision}","url":"/api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}"}`
