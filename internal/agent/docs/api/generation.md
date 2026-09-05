# Generation API

生成接口创建异步 Task；`queued` 只表示任务已创建。任务读取、事件、取消与重试见 [Task API](./task.md)。

## `POST /api/v1/projects/{project_uuid}/story-profile/generations`

根据创作要求异步生成新的 Story Profile。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `prompt` | body | string | 是 | 非空生成要求；最多 262,144 个字符。 |
| `model` | body | string | 否 | 文本模型覆盖值；最多 512 个字符，省略时使用项目 Story Text 模型。 |
| `chapter_count` | body | integer | 否 | 总纲面向的计划章节数，范围 1–20；默认 1。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 新 Task 公开 UUIDv7。 |
| `data.kind` | string | 任务类型，当前为 Story Profile 生成。 |
| `data.resource_uuid` | string(UUIDv7) | 当前项目公开 UUIDv7。 |
| `data.status` | string | 初始任务状态，通常为 `queued`。 |
| `data.error_code` | string，可省略 | 公开错误码；创建时通常省略。 |
| `data.error_message` | string，可省略 | 公开错误信息；创建时通常省略。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/story-profile/generations",
  "request_body": {"prompt": "生成八章悬疑童话的故事总纲", "chapter_count": 8},
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- 接口只创建异步 Task；Story Profile 在任务成功提交结果前不会改变。
- Task 创建使用 Tool Execution 提供的幂等键，重复执行同一 intent 不应创建第二个任务。

## `POST /api/v1/projects/{project_uuid}/story-profile/reconstructions`

从项目中已有的非空章节正文异步重建 Story Profile。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `model` | body | string | 否 | 文本模型覆盖值；最多 512 个字符，省略时使用项目 Story Text 模型。 |

必须提交 JSON Object `request_body`；不覆盖模型时提交 `{}`。

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 新 Task 公开 UUIDv7。 |
| `data.kind` | string | 任务类型，当前为从章节重建 Story Profile。 |
| `data.resource_uuid` | string(UUIDv7) | 当前项目公开 UUIDv7。 |
| `data.status` | string | 初始任务状态，通常为 `queued`。 |
| `data.error_code` | string，可省略 | 公开错误码。 |
| `data.error_message` | string，可省略 | 公开错误信息。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/story-profile/reconstructions",
  "request_body": {},
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- 项目中至少要有一个包含非空当前正文的 active Chapter。
- 任务成功时会覆盖当前 Story Profile；这是危险操作，创建任务前需要确认。
- 接口只创建异步 Task，并使用 Tool Execution 幂等键。

## `POST /api/v1/projects/{project_uuid}/chapter-batches`

异步规划并创建一批连续 Chapter。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `prompt` | body | string | 是 | 章节规划要求；最多 262,144 个字符。 |
| `model` | body | string | 否 | 文本模型覆盖值；最多 512 个字符。 |
| `chapter_count` | body | integer | 否 | 计划创建的章节数，范围 1–20；默认 1。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 新 Task 公开 UUIDv7。 |
| `data.kind` | string | 任务类型，当前为章节批量规划。 |
| `data.resource_uuid` | string(UUIDv7) | 当前项目公开 UUIDv7。 |
| `data.status` | string | 初始任务状态，通常为 `queued`。 |
| `data.error_code` | string，可省略 | 公开错误码。 |
| `data.error_message` | string，可省略 | 公开错误信息。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapter-batches",
  "request_body": {"prompt": "规划第一卷八章，每章推进一个线索", "chapter_count": 8},
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- 任务会基于当前 Story Profile、已有 active Chapter 与下一组可用章节编号规划新章节。
- 任务成功时会创建多个 Chapter；这是危险操作，创建任务前需要确认。
- 接口只创建异步 Task，并使用 Tool Execution 幂等键。
- Chat Agent 调用会等待内联 Workflow 终态；恢复后的 Tool Result 使用 `data.workflow_uuid`、`data.task_uuid`、`data.resource_uuid`、`data.status` 和成功时的 `data.result`（包含 `chapter_uuids`、`project_uuid`）。请求中的 `response_filter` 仍按上面的 Task 创建响应校验。

## `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/generations`

为一个 active Chapter 创建异步正文生成任务。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 目标 active Chapter 公开 UUIDv7。 |
| `prompt_key` | body | string | 是 | `story_chapter` 或 `next_story_chapter`。 |
| `prompt` | body | string | 是 | 非空生成要求；最多 262,144 个字符。 |
| `model` | body | string | 否 | 文本模型覆盖值；最多 512 个字符。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 新 Task 公开 UUIDv7。 |
| `data.kind` | string | 任务类型，当前为章节正文生成。 |
| `data.resource_uuid` | string(UUIDv7) | 目标 Chapter 公开 UUIDv7。 |
| `data.status` | string | 初始任务状态，通常为 `queued`。 |
| `data.error_code` | string，可省略 | 公开错误码。 |
| `data.error_message` | string，可省略 | 公开错误信息。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters/01970000-0000-7000-8000-000000000002/generations",
  "request_body": {"prompt_key": "story_chapter", "prompt": "生成本章完整正文"},
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- Chapter 必须处于 active 状态；同一 Chapter 不能已有进行中的生成任务。
- `story_chapter` 用于普通本章生成；`next_story_chapter` 用于依据上一章继续写作。
- 接口只创建异步 Task，并使用 Tool Execution 幂等键；任务成功后才追加正文版本。

## `POST /api/v1/projects/{project_uuid}/premise-sources/{source_uuid}/setting-generations`

根据一个 Premise Source 创建异步设定图生成任务。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `source_uuid` | path | string(UUIDv7) | 是 | 当前项目 Premise Source 公开 UUIDv7。 |
| `prompt` | body | string | 是 | 非空设定图生成要求。 |
| `model` | body | string | 否 | 图片模型覆盖值；最多 512 个字符，省略时使用项目 Premise 图片模型。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 新 Production Task 公开 UUIDv7。 |
| `data.kind` | string | 任务类型，当前为 Premise 设定图生成。 |
| `data.resource_uuid` | string(UUIDv7) | 目标 Premise Source 公开 UUIDv7。 |
| `data.status` | string | 初始任务状态，通常为 `queued`。 |
| `data.error_code` | string，可省略 | 公开错误码。 |
| `data.error_message` | string，可省略 | 公开错误信息。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/premise-sources/01970000-0000-7000-8000-000000000003/setting-generations",
  "request_body": {"prompt": "生成角色与主要场景的统一设定图"},
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- `source_uuid` 必须属于当前项目；`premise_asset_uuids` 不适用于本接口。
- 接口只创建异步 Task，并使用 Tool Execution 幂等键。

## `POST /api/v1/projects/{project_uuid}/premise-setting-images/{setting_image_uuid}/breakdowns`

把一张 Premise Setting Image 异步拆解为结构化 Premise Asset。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `setting_image_uuid` | path | string(UUIDv7) | 是 | 当前项目 Setting Image 公开 UUIDv7。 |
| `prompt` | body | string | 是 | 非空拆解要求。 |
| `model` | body | string | 否 | 文本/视觉模型覆盖值；最多 512 个字符，省略时使用项目配置。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 新 Production Task 公开 UUIDv7。 |
| `data.kind` | string | 任务类型，当前为 Premise Asset 拆解。 |
| `data.resource_uuid` | string(UUIDv7) | 目标 Setting Image 公开 UUIDv7。 |
| `data.status` | string | 初始任务状态，通常为 `queued`。 |
| `data.error_code` | string，可省略 | 公开错误码。 |
| `data.error_message` | string，可省略 | 公开错误信息。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/premise-setting-images/01970000-0000-7000-8000-000000000004/breakdowns",
  "request_body": {"prompt": "拆出角色、场景和关键道具"},
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- `setting_image_uuid` 必须属于当前项目且可读取；`premise_asset_uuids` 不适用于本接口。
- 接口只创建异步 Task，并使用 Tool Execution 幂等键。

## `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-storyboard-generations`

依据章节正文异步生成完整的正文页 Comic Storyboard 规划。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 目标 active Chapter 公开 UUIDv7。 |
| `prompt` | body | string | 是 | 分镜规划要求；最多 262,144 个字符。 |
| `model` | body | string | 否 | 文本模型覆盖值；最多 512 个字符。 |
| `max_section_count` | body | integer | 否 | 最大正文页 Section 数，范围 1–48；默认 6，只计算 `body`。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 新 Task 公开 UUIDv7。 |
| `data.kind` | string | 任务类型，当前为 Comic Storyboard 生成。 |
| `data.resource_uuid` | string(UUIDv7) | 目标 Chapter 公开 UUIDv7。 |
| `data.status` | string | 初始任务状态，通常为 `queued`。 |
| `data.error_code` | string，可省略 | 公开错误码。 |
| `data.error_message` | string，可省略 | 公开错误信息。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters/01970000-0000-7000-8000-000000000002/comic-storyboard-generations",
  "request_body": {"prompt": "按主要情节点拆分正文页", "max_section_count": 8},
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- Chapter 必须处于 active 状态并有非空当前正文。
- 任务成功时会生成或替换 `body` Section 规划，但不删除已有 `front_cover` 或 `back_cover`；创建任务前需要确认。
- 接口只创建异步 Task，并使用 Tool Execution 幂等键。

## `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-generations`

为一个 active Comic Section 创建异步图片生成任务。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 所属 active Chapter 公开 UUIDv7。 |
| `section_uuid` | path | string(UUIDv7) | 是 | 目标 active Section 公开 UUIDv7。 |
| `prompt` | body | string | 是 | 非空图片生成要求。 |
| `model` | body | string | 否 | 图片模型覆盖值；最多 512 个字符。 |
| `premise_asset_uuids` | body | array<string(UUIDv7)> | 否 | 0–12 个不重复、active 且已有 current variant 的 Premise Asset；只传与画面直接相关的项。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | string(UUIDv7) | 新 Production Task 公开 UUIDv7。 |
| `data.kind` | string | 任务类型，当前为 Comic 图片生成。 |
| `data.resource_uuid` | string(UUIDv7) | 目标 Section 公开 UUIDv7。 |
| `data.status` | string | 初始任务状态，通常为 `queued`。 |
| `data.error_code` | string，可省略 | 公开错误码。 |
| `data.error_message` | string，可省略 | 公开错误信息。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters/01970000-0000-7000-8000-000000000002/comic-sections/01970000-0000-7000-8000-000000000005/image-generations",
  "request_body": {
    "prompt": "生成该页漫画图，保留银色边框",
    "premise_asset_uuids": ["01970000-0000-7000-8000-000000000006"]
  },
  "response_filter": ".data | {uuid,kind,resource_uuid,status,error_code,error_message}"
}
```

### 接口约束

- Section 必须属于路径中的 Chapter、处于 active 状态并有当前 Storyboard；同一 Section 不能已有活动图片任务。
- `premise_asset_uuids` 必须互不重复，所有项须属于当前项目且有可用图片版本。
- 此接口用于单个 Section；生成多个 Section 时使用批量接口。
- 接口只创建异步 Task，并使用 Tool Execution 幂等键。

## `POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/comic-image-generation-batches`

一次预检并原子创建一个 Chapter 内多个 Section 的图片生成任务，以及聚合这些任务的批次 Workflow。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | string(UUIDv7) | 是 | 当前项目公开 UUIDv7。 |
| `chapter_uuid` | path | string(UUIDv7) | 是 | 所属 active Chapter 公开 UUIDv7。 |
| `section_uuids` | body | array<string(UUIDv7)> | 是 | 1–48 个有效且不重复的 Section UUIDv7；数组顺序即请求页序。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.workflow_uuid` | string(UUIDv7) | 聚合整批任务的 Workflow 公开 UUIDv7。 |
| `data.chapter_uuid` | string(UUIDv7) | 所属 Chapter 公开 UUIDv7。 |
| `data.requested_count` | integer | 请求的 Section 数量。 |
| `data.accepted_count` | integer | 原子预检后成功创建或幂等命中的 Task 数量。 |
| `data.tasks` | array<object> | 与输入顺序对应的 Production Task。 |
| `data.tasks[].uuid` | string(UUIDv7) | Task 公开 UUIDv7。 |
| `data.tasks[].kind` | string | 任务类型，当前为 Comic 图片生成。 |
| `data.tasks[].resource_uuid` | string(UUIDv7) | 对应 Section 公开 UUIDv7。 |
| `data.tasks[].status` | string | 任务状态，通常为 `queued`。 |
| `data.tasks[].error_code` | string，可省略 | 公开错误码。 |
| `data.tasks[].error_message` | string，可省略 | 公开错误信息。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/01970000-0000-7000-8000-000000000001/chapters/01970000-0000-7000-8000-000000000002/comic-image-generation-batches",
  "request_body": {
    "section_uuids": [
      "01970000-0000-7000-8000-000000000005",
      "01970000-0000-7000-8000-000000000007"
    ]
  },
  "response_filter": ".data | {workflow_uuid,chapter_uuid,requested_count,accepted_count,tasks:{uuid,kind,resource_uuid,status,error_code,error_message}}"
}
```

### 接口约束

- 所有 Section 必须属于路径中的 active Chapter、处于 active 状态、有当前 Storyboard，且没有活动图片任务；任一项失败则整批不创建任务。
- 服务端为每个 Section 固化当前 Storyboard、项目画风、设定引用和项目图片模型；本接口不接收 prompt、model 或 Premise 引用。
- 输入 UUID 不得重复；创建使用批次幂等键，重放同一 intent 不得复制任务。
- Chat Agent 调用会以内联 Workflow 等待整批终态；直接 UI 调用创建独立 Workflow Thread，父 Workflow Step 调用不额外展示。
- 任一子任务失败不会停止其他任务；全部子任务终止后，批次 Workflow 才按 `failed > interrupted > cancelled > completed` 收敛。
- `accepted_count` 不表示图片已生成完成；通过 `workflow_uuid` 在 ChatArea 查看总进度和逐 Section 状态。
