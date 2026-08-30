# Storyboard API

## `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/storyboard-variants`

按版本号倒序列出 Section 的全部 Storyboard 版本。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 所属 active Chapter UUID。 |
| `section_uuid` | path | string(UUIDv7) | 是 | 目标 active Section UUID。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | array<object> | Storyboard 版本列表；无结果时为空数组。 |
| `data.items[].uuid` | string(UUIDv7) | Storyboard Variant UUID。 |
| `data.items[].version_no` | integer | Section 内递增版本号。 |
| `data.items[].content_md` | string | 该版本的完整 Storyboard Markdown。 |
| `data.items[].source_type` | string | `manual`、`generated` 或 `restore`。 |
| `data.items[].created_at` | string(date-time) | 版本创建时间。 |

### request_api 示例

```json
{"method":"GET","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-sections/<section_uuid>/storyboard-variants","response_filter":".data.items[] | {uuid,version_no,source_type,created_at}"}
```

### 接口约束

无额外约束。需要 Section revision 时读取 `/api/v1/agent-docs/api/comic-section.md`。

## `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/storyboard-variants`

创建一个完整 Storyboard 版本并将其设为当前版本。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 所属 active Chapter UUID。 |
| `section_uuid` | path | string(UUIDv7) | 是 | 目标 active Section UUID。 |
| `content_md` | body | string，1–262144 字符 | 是 | 完整 Storyboard Markdown，不是增量补丁；去除首尾空白后不得为空。 |
| `expected_revision` | body | integer，0–2147483647 | 是 | 刚读取到的 Section revision。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Section UUID。 |
| `data.chapter_uuid` | string(UUIDv7) | 所属 Chapter UUID。 |
| `data.section_no` | integer | 绝对装订顺序。 |
| `data.page_role` | string | `front_cover`、`body` 或 `back_cover`。 |
| `data.title` | string | Section 标题。 |
| `data.description_md` | string | Section 描述 Markdown。 |
| `data.current_storyboard` | object | 新当前版本；含 `uuid:string(UUIDv7)`、`version_no:integer`、`content_md:string`、`source_type:string`、`created_at:string(date-time)`。 |
| `data.revision` | integer | 创建版本后的 Section revision。 |

### request_api 示例

```json
{"method":"POST","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-sections/<section_uuid>/storyboard-variants","request_body":{"content_md":"# 雨夜相遇\n\n完整分镜内容","expected_revision":3},"response_filter":".data | {uuid,current_storyboard:{uuid,version_no},revision}"}
```

### 接口约束

`expected_revision` 必须匹配当前 Section。每次调用创建不可变的新版本，Agent Contract 不接受 `source_type` 字段。

## `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/storyboard-variants/{variant_uuid}/selections`

把历史 Storyboard 内容恢复为新的当前版本。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 所属 active Chapter UUID。 |
| `section_uuid` | path | string(UUIDv7) | 是 | 目标 active Section UUID。 |
| `variant_uuid` | path | string(UUIDv7) | 是 | 目标 Section 内的历史 Storyboard Variant UUID。 |
| `expected_revision` | body | integer，0–2147483647 | 是 | 刚读取到的 Section revision。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Section UUID。 |
| `data.chapter_uuid` | string(UUIDv7) | 所属 Chapter UUID。 |
| `data.section_no` | integer | 绝对装订顺序。 |
| `data.page_role` | string | 页面角色。 |
| `data.title` | string | Section 标题。 |
| `data.description_md` | string | Section 描述 Markdown。 |
| `data.current_storyboard` | object | 复制出的新 `restore` 版本；含 `uuid:string(UUIDv7)`、`version_no:integer`、`content_md:string`、`source_type:string`、`created_at:string(date-time)`。 |
| `data.revision` | integer | 恢复后的 Section revision。 |

### request_api 示例

```json
{"method":"POST","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-sections/<section_uuid>/storyboard-variants/<variant_uuid>/selections","request_body":{"expected_revision":3},"response_filter":".data | {uuid,current_storyboard:{uuid,version_no,source_type},revision}"}
```

### 接口约束

`expected_revision` 必须匹配，且 `variant_uuid` 必须属于目标 Section。选择不会改写历史记录，而是复制其内容创建新的 `restore` 版本。
