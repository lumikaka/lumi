# Story Profile API

Story Profile 是 SQLite 中的故事总纲事实状态；`story_md` 始终表示完整 `STORY.md` 文档。

## `GET /api/v1/projects/{project_uuid}/story-profile`

读取当前 Story Profile，并校准项目 `STORY.md` 的投影状态。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |

不接收 `query` 或 `request_body`。

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 当前 Story Profile 版本的公开 UUIDv7。 |
| `data.revision` | integer | 当前乐观并发版本。 |
| `data.story_md` | string | 完整 Story Profile Markdown。 |
| `data.projection_state` | string | 文件投影状态：`synced`、`pending` 或 `conflict`。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/story-profile",
  "response_filter": ".data | {uuid,revision,projection_state}"
}
```

### 接口约束

- 读取会对比 SQLite 与项目 `STORY.md`；检测到外部文件变更时 `projection_state` 变为 `conflict`。

## `PUT /api/v1/projects/{project_uuid}/story-profile`

基于最新 revision 创建新的 Story Profile 版本，并同步项目 `STORY.md`。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `story_md` | body | string | 是 | 完整 `STORY.md`，不是补丁；须为非空白 UTF-8 文本，最多 2 MiB。 |
| `expected_revision` | body | integer | 是 | 刚读取到的 `data.revision`。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 保存后的 Story Profile 版本 UUIDv7；内容变化时创建新 UUID。 |
| `data.revision` | integer | 保存后的 revision；提交相同内容时可能保持不变。 |
| `data.story_md` | string | 保存后的完整 Markdown。 |
| `data.projection_state` | string | 保存后的文件投影状态，通常为 `synced`；文件写入失败时可能为 `pending`。 |

### request_api 示例

```json
{
  "method": "PUT",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/story-profile",
  "request_body": {
    "story_md": "# 故事总纲\n\n狐狸在月光邮局替人们寄出未说出口的话。",
    "expected_revision": 3
  },
  "response_filter": ".data | {uuid,revision,projection_state}"
}
```

### 接口约束

- `story_md` 是完整替换值；`expected_revision` 必须匹配当前 revision。
- `projection_state=conflict` 时不得直接覆盖；先选择导入外部文件或从 SQLite 重建文件。
- 内容与当前版本完全相同时接口幂等返回当前版本。

## `GET /api/v1/projects/{project_uuid}/story-profile/versions`

读取 Story Profile 的不可变版本历史。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |

不接收 `query` 或 `request_body`。

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.items` | array<object> | Story Profile 版本列表；无结果时为空数组。 |
| `data.items[].uuid` | string(UUIDv7) | 版本公开 UUIDv7。 |
| `data.items[].revision` | integer | 该版本对应的 revision。 |
| `data.items[].story_md` | string | 该版本的完整 Markdown。 |
| `data.items[].projection_state` | string | 该版本记录的投影状态。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/story-profile/versions",
  "response_filter": ".data.items[] | {uuid,revision,projection_state}"
}
```

### 接口约束

- 历史版本只读；列表不分页。

## `POST /api/v1/projects/{project_uuid}/story-profile/imports`

把项目目录中被外部修改的 `STORY.md` 导入 SQLite，成为新的当前 Story Profile。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `expected_revision` | body | integer | 是 | 当前 Story Profile 最新 revision；范围 0–2,147,483,647。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 导入后新版本的公开 UUIDv7。 |
| `data.revision` | integer | 导入后的 revision。 |
| `data.story_md` | string | 从项目文件读入的完整 Markdown。 |
| `data.projection_state` | string | 导入成功后为 `synced`。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/story-profile/imports",
  "request_body": {"expected_revision": 3},
  "response_filter": ".data | {uuid,revision,projection_state}"
}
```

### 接口约束

- 当前 `projection_state` 必须为 `conflict`，且 `expected_revision` 必须匹配。
- 操作会用项目文件覆盖 SQLite 当前事实状态；这是危险操作，执行前需要确认。
- 导入作为新版本写入，不原地修改历史版本。

## `POST /api/v1/projects/{project_uuid}/story-profile/projection`

用 SQLite 中的当前 Story Profile 重新生成项目 `STORY.md`。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `expected_revision` | body | integer | 是 | 当前 Story Profile 最新 revision；范围 0–2,147,483,647。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 当前 Story Profile 版本公开 UUIDv7。 |
| `data.revision` | integer | 当前 revision；文件投影不创建内容版本。 |
| `data.story_md` | string | 写入项目文件的完整 Markdown。 |
| `data.projection_state` | string | 成功后为 `synced`；写入失败时保持可恢复状态。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/story-profile/projection",
  "request_body": {"expected_revision": 3},
  "response_filter": ".data | {uuid,revision,projection_state}"
}
```

### 接口约束

- `expected_revision` 必须匹配当前 Story Profile revision。
- 操作会覆盖项目 `STORY.md` 文件；这是危险操作，执行前需要确认。
- 成功重放同一内容不会增加 revision，因而对 Story Profile 内容幂等。
