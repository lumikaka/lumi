# Comic Snapshot API

## `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-snapshots`

按版本号倒序列出章节的 Comic Snapshot 摘要。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 当前项目内的 active Chapter UUID。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | array<object> | Snapshot 摘要列表；无结果时为空数组。 |
| `data.items[].uuid` | string(UUIDv7) | Snapshot UUID。 |
| `data.items[].version_no` | integer | Chapter 内递增版本号。 |
| `data.items[].reason` | string | 生成快照的变更原因。 |
| `data.items[].source` | string | 快照来源。 |
| `data.items[].section_count` | integer | 快照内 Section 数。 |
| `data.items[].created_at` | string(date-time) | 创建时间。 |

### request_api 示例

```json
{"method":"GET","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-snapshots","response_filter":".data.items[] | {uuid,version_no,reason,section_count,created_at}"}
```

### 接口约束

无额外约束。

## `GET /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-snapshots/{snapshot_uuid}`

读取一个 Comic Snapshot 的章节和页面冻结内容。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 当前项目内的 active Chapter UUID。 |
| `snapshot_uuid` | path | string(UUIDv7) | 是 | 该 Chapter 的 Snapshot UUID。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | Snapshot UUID。 |
| `data.version_no` | integer | Chapter 内递增版本号。 |
| `data.reason` | string | 生成快照的变更原因。 |
| `data.source` | string | 快照来源。 |
| `data.section_count` | integer | 冻结的 Section 数。 |
| `data.schema_version` | integer | Snapshot payload schema 版本。 |
| `data.chapter` | object | 冻结的 Chapter 摘要，含 `uuid:string(UUIDv7)`、`chapter_code:string`、`title:string`。 |
| `data.sections` | array<object> | 冻结页面；每项含 `uuid:string(UUIDv7)`、`section_no:integer`、`page_role:string`、`title:string`、`storyboard_md:string`、`current_image:object`、`premise_reference:object`。 |
| `data.sections[].current_image` | object | 当前图片快照，含 `status:string`，以及可能缺省的 `variant_uuid:string(UUIDv7)`、`asset_uuid:string(UUIDv7)`、`asset:object`。 |
| `data.sections[].premise_reference` | object | Premise 引用快照，字段同 `current_image`。 |
| `data.sections[].current_image.asset` | object，可缺省 | 冻结的公开 Asset；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.sections[].premise_reference.asset` | object，可缺省 | 冻结的公开 Asset；完整字段、类型与可省略性遵循 Overview 的 `public_asset_v1`。 |
| `data.created_at` | string(date-time) | Snapshot 创建时间。 |

### request_api 示例

```json
{"method":"GET","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-snapshots/<snapshot_uuid>","response_filter":".data | {uuid,version_no,chapter:{uuid,title},sections:{uuid,section_no,page_role,title}}"}
```

### 接口约束

无额外约束。

## `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-snapshots/{snapshot_uuid}/restorations`

用 Snapshot 整体替换章节当前 Comic 状态。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目 UUID。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 当前项目内的 active Chapter UUID。 |
| `snapshot_uuid` | path | string(UUIDv7) | 是 | 该 Chapter 的 Snapshot UUID。 |
| `request_body` | body | object | 是 | 必须是空对象 `{}`，不接受额外字段。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | array<object> | 恢复后的全部 active Section；可为空数组。 |
| `data.items[].uuid` | string(UUIDv7) | Section UUID。 |
| `data.items[].chapter_uuid` | string(UUIDv7) | 所属 Chapter UUID。 |
| `data.items[].section_no` | integer | 恢复后的绝对装订顺序。 |
| `data.items[].page_role` | string | `front_cover`、`body` 或 `back_cover`。 |
| `data.items[].title` | string | Section 标题。 |
| `data.items[].description_md` | string | Section 描述 Markdown。 |
| `data.items[].current_storyboard` | object 或 null | 当前分镜；非空时含 `uuid:string(UUIDv7)`、`version_no:integer`、`content_md:string`、`source_type:string`、`created_at:string(date-time)`。 |
| `data.items[].revision` | integer | 恢复后的 Section revision。 |

### request_api 示例

```json
{"method":"POST","url":"/api/v1/projects/<project_uuid>/chapters/<chapter_uuid>/comic-snapshots/<snapshot_uuid>/restorations","request_body":{},"response_filter":".data.items[] | {uuid,section_no,page_role,title,revision}"}
```

### 接口约束

危险操作，执行前需要确认。普通绘本的目标快照必须保留至少一个 active `body`，不能恢复空快照或只有特殊页的快照；`vertical_strip` 可恢复为空，但其中存在的页面必须全是 `body`。
