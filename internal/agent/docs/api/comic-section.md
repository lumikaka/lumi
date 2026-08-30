# Comic Section API

## `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}`

读取单个 active 漫画页面或条漫画面段落及其当前 Storyboard。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 所属 active Chapter UUID。 |
| `section_uuid` | path | string(UUIDv7) | 是 | 目标 active Section UUID。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Section UUID。 |
| `data.chapter_uuid` | string(UUIDv7) | 所属 Chapter UUID。 |
| `data.section_no` | integer | 包含封面和封底的绝对装订顺序。 |
| `data.page_role` | string | `front_cover`、`body` 或 `back_cover`。 |
| `data.title` | string | Section 标题。 |
| `data.description_md` | string | Section 描述 Markdown。 |
| `data.current_storyboard` | object 或 null | 当前分镜；非空时含 `uuid:string(UUIDv7)`、`version_no:integer`、`content_md:string`、`source_type:string`、`created_at:string(date-time)`。 |
| `data.revision` | integer | 当前 revision，供后续更新、删除、导入或选择版本使用。 |

### request_api 示例

```json
{"method":"GET","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-sections/<section_uuid>","response_filter":".data | {uuid,page_role,title,current_storyboard:{uuid,version_no,content_md},revision}"}
```

### 接口约束

无额外约束。集合与图片操作见 `/api/v1/agent-docs/api/comic.md`，Storyboard 版本操作见 `/api/v1/agent-docs/api/storyboard.md`。
