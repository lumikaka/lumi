# Comic API

Comic 的 `section_no` 是包含封面和封底的绝对装订顺序。`page_role` 取 `front_cover`、`body` 或 `back_cover`；`vertical_strip` 项目只使用 `body`。

## `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic`

读取章节的 Comic 汇总状态。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 当前项目内的 active Chapter UUID。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Comic State UUID。 |
| `data.chapter_uuid` | string(UUIDv7) | 所属 Chapter UUID。 |
| `data.status` | string | `empty`、`draft`、`storyboarded` 或 `ready`。 |
| `data.has_premise_assets` | boolean | 项目是否已有可用 Premise Asset。 |
| `data.premise_asset_count` | integer | 可用 Premise Asset 数。 |
| `data.revision` | integer | Comic State 当前 revision。 |
| `data.updated_at` | string(date-time) | 最近更新时间。 |

### request_api 示例

```json
{"method":"GET","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic","response_filter":".data | {status,revision}"}
```

### 接口约束

无额外约束。

## `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections`

按绝对装订顺序列出章节内全部 active Comic Section。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 当前项目内的 active Chapter UUID。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | array<object> | Section 列表；无结果时为空数组。 |
| `data.items[].uuid` | string(UUIDv7) | Section UUID。 |
| `data.items[].chapter_uuid` | string(UUIDv7) | 所属 Chapter UUID。 |
| `data.items[].section_no` | integer | 包含特殊页的绝对装订顺序。 |
| `data.items[].page_role` | string | `front_cover`、`body` 或 `back_cover`。 |
| `data.items[].title` | string | Section 标题。 |
| `data.items[].description_md` | string | Section 描述 Markdown。 |
| `data.items[].current_storyboard` | object 或 null | 当前分镜；非空时含 `uuid:string(UUIDv7)`、`version_no:integer`、`content_md:string`、`source_type:string`、`created_at:string(date-time)`。 |
| `data.items[].revision` | integer | Section 当前 revision。 |

### request_api 示例

```json
{"method":"GET","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-sections","response_filter":".data.items[] | {uuid,section_no,page_role,title,revision}"}
```

### 接口约束

无额外约束。

## `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections`

创建一个 Comic Section，并可同时创建初始 Storyboard。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 当前项目内的 active Chapter UUID。 |
| `title` | body | string，最长 160 字符 | 是 | Section 标题。 |
| `description_md` | body | string，最长 262144 字符 | 否 | 描述 Markdown；默认空字符串。 |
| `storyboard_md` | body | string，最长 262144 字符 | 否 | 完整初始 Storyboard Markdown；默认不创建版本。 |
| `page_role` | body | string | 否 | `front_cover`、`body` 或 `back_cover`；默认 `body`。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 新 Section UUID。 |
| `data.chapter_uuid` | string(UUIDv7) | 所属 Chapter UUID。 |
| `data.section_no` | integer | 归一化后的绝对装订顺序。 |
| `data.page_role` | string | 最终页面角色。 |
| `data.title` | string | 标题。 |
| `data.description_md` | string | 描述 Markdown。 |
| `data.current_storyboard` | object 或 null | 当前分镜；非空时含 `uuid:string(UUIDv7)`、`version_no:integer`、`content_md:string`、`source_type:string`、`created_at:string(date-time)`。 |
| `data.revision` | integer | 新 Section revision。 |

### request_api 示例

```json
{"method":"POST","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-sections","request_body":{"title":"雨夜相遇","page_role":"body"},"response_filter":".data | {uuid,section_no,page_role,revision}"}
```

### 接口约束

普通绘本的空页面序列必须先创建 `body`；已有 active `body` 后才可创建封面或封底，且两种特殊页各最多一个。`vertical_strip` 只允许 `body`。服务端会保持封面在首位、封底在末位。

## `PATCH /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}`

按 revision 更新 Section 标题、描述或页面角色。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 所属 active Chapter UUID。 |
| `section_uuid` | path | string(UUIDv7) | 是 | 目标 active Section UUID。 |
| `expected_revision` | body | integer，0–2147483647 | 是 | 刚读取到的 Section revision。 |
| `title` | body | string，最长 160 字符 | 否 | 新标题；省略时不变。 |
| `description_md` | body | string，最长 262144 字符 | 否 | 新描述 Markdown；省略时不变。 |
| `page_role` | body | string | 否 | `front_cover`、`body` 或 `back_cover`；省略时不变。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Section UUID。 |
| `data.chapter_uuid` | string(UUIDv7) | 所属 Chapter UUID。 |
| `data.section_no` | integer | 归一化后的绝对装订顺序。 |
| `data.page_role` | string | 最终页面角色。 |
| `data.title` | string | 最终标题。 |
| `data.description_md` | string | 最终描述 Markdown。 |
| `data.current_storyboard` | object 或 null | 当前分镜；非空时含 `uuid:string(UUIDv7)`、`version_no:integer`、`content_md:string`、`source_type:string`、`created_at:string(date-time)`。 |
| `data.revision` | integer | 更新后的 revision。 |

### request_api 示例

```json
{"method":"PATCH","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-sections/<section_uuid>","request_body":{"title":"雨夜重逢","expected_revision":3},"response_filter":".data | {uuid,title,revision}"}
```

### 接口约束

`expected_revision` 必须匹配当前 Section。普通绘本不能把最后一个 active `body` 改为特殊页；特殊页仍须唯一。`vertical_strip` 不能改为特殊页。

## `PUT /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-section-order`

整体重排章节的 active 正文页。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 目标 active Chapter UUID。 |
| `section_uuids` | body | array<string(UUIDv7)>，1–200 项 | 是 | 排序后的完整 active `body` UUID 列表。 |
| `section_uuids[]` | body | string(UUIDv7) | 是 | 当前 Chapter 内互不重复的 active `body` UUID。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | array<object> | 重排后的全部 active Section。 |
| `data.items[].uuid` | string(UUIDv7) | Section UUID。 |
| `data.items[].chapter_uuid` | string(UUIDv7) | 所属 Chapter UUID。 |
| `data.items[].section_no` | integer | 新绝对装订顺序。 |
| `data.items[].page_role` | string | 页面角色。 |
| `data.items[].title` | string | 标题。 |
| `data.items[].description_md` | string | 描述 Markdown。 |
| `data.items[].current_storyboard` | object 或 null | 当前分镜；非空时含 `uuid:string(UUIDv7)`、`version_no:integer`、`content_md:string`、`source_type:string`、`created_at:string(date-time)`。 |
| `data.items[].revision` | integer | Section revision；被重排的正文页会推进 revision。 |

### request_api 示例

```json
{"method":"PUT","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-section-order","request_body":{"section_uuids":["<section_uuid_2>","<section_uuid_1>"]},"response_filter":".data.items[] | {uuid,section_no,page_role,revision}"}
```

### 接口约束

危险操作，执行前需要确认。数组必须且只能覆盖当前全部 active `body`，不得重复；Agent 不应传封面或封底，服务端会固定其首尾位置。

## `DELETE /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}`

软删除一个 active Comic Section。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 所属 active Chapter UUID。 |
| `section_uuid` | path | string(UUIDv7) | 是 | 目标 active Section UUID。 |
| `expected_revision` | body | integer，0–2147483647 | 是 | 刚读取到的 Section revision。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 已删除的 Section UUID。 |
| `data.deleted` | boolean | 是否已成功完成软删除。 |

### request_api 示例

```json
{"method":"DELETE","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-sections/<section_uuid>","request_body":{"expected_revision":3},"response_filter":".data | {uuid,deleted}"}
```

### 接口约束

危险操作，执行前需要确认，且 `expected_revision` 必须匹配。普通绘本不能删除最后一个 active `body`；`vertical_strip` 可删除最后一个画面段落并回到 `empty`。

## `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/images`

把当前项目尚未消费的 ready upload 导入为新图片版本并设为当前图片。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 所属 active Chapter UUID。 |
| `section_uuid` | path | string(UUIDv7) | 是 | 目标 active Section UUID。 |
| `upload_uuid` | body | string(UUIDv7) | 是 | 当前项目、purpose 为 `comic_section_image` 的 ready upload UUID。 |
| `expected_revision` | body | integer，0–2147483647 | 是 | 刚读取到的 Section revision。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Section UUID。 |
| `data.chapter_uuid` | string(UUIDv7) | 所属 Chapter UUID。 |
| `data.section_no` | integer | 绝对装订顺序。 |
| `data.page_role` | string | 页面角色。 |
| `data.title` | string | 标题。 |
| `data.description_md` | string | 描述 Markdown。 |
| `data.current_storyboard` | object 或 null | 当前分镜；非空时含 `uuid:string(UUIDv7)`、`version_no:integer`、`content_md:string`、`source_type:string`、`created_at:string(date-time)`。 |
| `data.revision` | integer | 导入后的 revision。 |

### request_api 示例

```json
{"method":"POST","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-sections/<section_uuid>/images","request_body":{"upload_uuid":"<upload_uuid>","expected_revision":3},"response_filter":".data | {uuid,page_role,revision}"}
```

### 接口约束

`expected_revision` 必须匹配；upload 只能成功消费一次，且必须属于当前项目并处于 ready 状态。

## `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-variants`

按版本倒序列出 Section 的图片版本。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 所属 active Chapter UUID。 |
| `section_uuid` | path | string(UUIDv7) | 是 | 目标 active Section UUID。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | array<object> | 图片版本列表；无结果时为空数组。 |
| `data.items[].uuid` | string(UUIDv7) | Variant UUID。 |
| `data.items[].version_no` | integer | Section 内递增版本号。 |
| `data.items[].source_type` | string | 来源类型，例如 `manual` 或 `generated`。 |
| `data.items[].generation_uuid` | string(UUIDv7)，可缺省 | 生成版本关联的 Generation UUID；手工导入时缺省。 |
| `data.items[].asset` | object | 图片 Asset；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.items[].created_at` | string(date-time) | 版本创建时间。 |

### request_api 示例

```json
{"method":"GET","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-sections/<section_uuid>/image-variants","response_filter":".data.items[] | {uuid,version_no,source_type,asset:{uuid,mime_type,width,height,status}}"}
```

### 接口约束

无额外约束。

## `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-variants/{variant_uuid}/selections`

把指定历史图片版本设为 Section 的当前图片。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 所属 active Chapter UUID。 |
| `section_uuid` | path | string(UUIDv7) | 是 | 目标 active Section UUID。 |
| `variant_uuid` | path | string(UUIDv7) | 是 | 目标 Section 内的图片 Variant UUID。 |
| `expected_revision` | body | integer，0–2147483647 | 是 | 刚读取到的 Section revision。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Section UUID。 |
| `data.chapter_uuid` | string(UUIDv7) | 所属 Chapter UUID。 |
| `data.section_no` | integer | 绝对装订顺序。 |
| `data.page_role` | string | 页面角色。 |
| `data.title` | string | 标题。 |
| `data.description_md` | string | 描述 Markdown。 |
| `data.current_storyboard` | object 或 null | 当前分镜；非空时含 `uuid:string(UUIDv7)`、`version_no:integer`、`content_md:string`、`source_type:string`、`created_at:string(date-time)`。 |
| `data.revision` | integer | 选择后的 revision。 |

### request_api 示例

```json
{"method":"POST","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-sections/<section_uuid>/image-variants/<variant_uuid>/selections","request_body":{"expected_revision":3},"response_filter":".data | {uuid,revision}"}
```

### 接口约束

`expected_revision` 必须匹配；`variant_uuid` 必须属于目标 Section。

单项读取见 `/api/v1/agent-docs/api/comic-section.md`；Storyboard 版本见 `/api/v1/agent-docs/api/storyboard.md`；图片生成任务见 `/api/v1/agent-docs/api/generation.md`。
