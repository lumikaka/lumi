# Project Setup API

仅在项目 `setup_status=draft` 时，本 Contract 用于整理初始化草稿。Setup Draft 不是正式项目事实；finalization 成功后项目才变为 `ready`，且不得创建、选择或切换 Candidate。

## `GET /api/v1/projects/{project_uuid}/project-setup`

读取当前 Setup Draft、字段来源、缺失信息和已定稿绘本规格。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串，可省略 | Setup Draft UUID；兼容 ready 项目无草稿的情况。 |
| `data.project_uuid` | UUIDv7 字符串 | 所属项目 UUID。 |
| `data.setup_status` | 字符串 | 项目初始化状态：`draft` 或 `ready`。 |
| `data.status` | 字符串 | 草稿状态：`draft`、`pending_confirmation`、`finalized` 或 `failed`。 |
| `data.revision` | 整数 | Setup Draft 当前乐观并发版本。 |
| `data.original_input` | 字符串，可省略 | 首页创建会话的原始输入。 |
| `data.draft_values` | 对象 | 当前整理出的草稿字段。 |
| `data.draft_values.project_name` | 字符串，可省略 | 候选项目名称。 |
| `data.draft_values.generation_language` | 字符串，可省略 | `zh-Hans` 或 `en`。 |
| `data.draft_values.overall_style` | 字符串，可省略 | 候选整体画风。 |
| `data.draft_values.generation_brief` | 字符串，可省略 | 基于原始需求、用户补充和明确展示建议整理的生成 Brief。 |
| `data.draft_values.picture_book` | 对象，可省略 | 规范化绘本规格草稿。 |
| `data.draft_values.picture_book.format` | 字符串 | 绘本形式。 |
| `data.draft_values.picture_book.aspect_ratio` | 对象 | 规范化比例，含 `mode`、`width`、`height`。 |
| `data.draft_values.picture_book.large_image_minimal_text` | 布尔值或 `null` | 仅经典绘本适用。 |
| `data.draft_values.picture_book.interaction_mode` | 字符串或 `null` | 仅互动绘本适用。 |
| `data.draft_values.picture_book.comic_layout` | 字符串或 `null` | 仅漫画故事适用。 |
| `data.field_sources` | 字符串映射 | 各字段来源；值为 `system_default`、`agent_proposed` 或 `user_confirmed`。 |
| `data.missing_information` | 字符串数组 | finalization 前仍缺少的字段路径。 |
| `data.final_picture_book` | 对象，可省略 | 已定稿的不可变绘本规格；结构同规范化 `picture_book`。 |
| `data.reference_plan` | 对象 | 首页创建视觉参考的只读事实，含 `items`；新附件由系统按顺序自动用于画面生成，Agent 不修改或逐项询问其用途。 |
| `data.error_code` | 字符串，可省略 | 草稿失败时的安全错误码。 |
| `data.error_message` | 字符串，可省略 | 草稿失败时的安全错误信息。 |
| `data.created_at` | RFC 3339 字符串，可省略 | 草稿创建时间。 |
| `data.updated_at` | RFC 3339 字符串，可省略 | 最近更新时间。 |
| `data.finalized_at` | RFC 3339 字符串，可省略 | 定稿时间。 |

### request_api 示例

```json
{
  "method": "GET",
  "url": "/api/v1/projects/<project_uuid>/project-setup",
  "response_filter": ".data | {uuid,project_uuid,setup_status,status,revision,draft_values,field_sources,missing_information,final_picture_book,reference_plan,updated_at}"
}
```

### 接口约束

- 不传 `query` 或 `request_body`。
- `field_sources=system_default` 的值不得描述为用户选择。
- PATCH 或 finalization 前必须以本接口返回的最新 `revision` 为准。

## `PATCH /api/v1/projects/{project_uuid}/project-setup`

基于最新 revision 更新一个或多个 Setup Draft 字段。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `expected_revision` | body | 整数，1～2147483647 | 是 | 刚读取到的 Setup Draft revision。 |
| `project_name` | body | 非空字符串，最长 120 字符 | 否 | 候选项目名称。 |
| `generation_language` | body | 枚举字符串 | 否 | `zh-Hans` 或 `en`。 |
| `overall_style` | body | 非空字符串，最长 12000 字符 | 否 | 候选整体画风。 |
| `generation_brief` | body | 非空字符串，最长 4000 字符 | 否 | 自动生成使用的故事 Brief；应在进入确认前整理并持久化。 |
| `picture_book` | body | 对象 | 否 | 绘本规格；提供时必须包含 `format`。 |
| `picture_book.format` | body | 枚举字符串 | 条件必填 | `classic_picture_book`、`wordless_picture_book`、`interactive_picture_book`、`comic_story` 或 `vertical_strip`。 |
| `picture_book.aspect_ratio` | body | 对象 | 否 | 经典、无字和漫画故事可用；省略时默认横向 4:3。 |
| `picture_book.aspect_ratio.mode` | body | 枚举字符串 | 条件必填 | `landscape`、`square`、`portrait` 或 `custom`。 |
| `picture_book.aspect_ratio.width` | body | 整数，1～100 | 条件必填 | `mode=custom` 时必填；预设模式不得传。 |
| `picture_book.aspect_ratio.height` | body | 整数，1～100 | 条件必填 | `mode=custom` 时必填；预设模式不得传。 |
| `picture_book.large_image_minimal_text` | body | 布尔值 | 否 | 仅 `classic_picture_book` 可用，默认 `false`。 |
| `picture_book.interaction_mode` | body | 枚举字符串 | 否 | 仅 `interactive_picture_book` 可用；`find_it`、`make_a_choice`、`guess` 或 `follow_along`，默认 `find_it`。 |
| `picture_book.comic_layout` | body | 枚举字符串 | 否 | 仅 `comic_story` 可用；`four_panel` 或 `page_comic`，默认 `page_comic`。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | Setup Draft UUID。 |
| `data.project_uuid` | UUIDv7 字符串 | 所属项目 UUID。 |
| `data.setup_status` | 字符串 | 仍为 `draft`。 |
| `data.status` | 字符串 | 信息完整时为 `pending_confirmation`，否则为 `draft`；失败时可为 `failed`。 |
| `data.revision` | 整数 | 更新后递增的 revision。 |
| `data.original_input` | 字符串，可省略 | 首页原始输入。 |
| `data.draft_values` | 对象 | 更新并规范化后的 `project_name`、`generation_language`、`overall_style`、`generation_brief` 和可选 `picture_book`。 |
| `data.draft_values.picture_book.aspect_ratio` | 对象 | 返回事实值总含 `mode`、`width`、`height`。 |
| `data.field_sources` | 字符串映射 | 本次提交字段标记为 `agent_proposed`；采用默认值的细分字段标记为 `system_default`。 |
| `data.missing_information` | 字符串数组 | 仍缺少的字段路径。 |
| `data.final_picture_book` | 对象，可省略 | 草稿阶段通常省略。 |
| `data.reference_plan` | 对象 | 系统托管且不随本次字段更新改变的视觉参考事实。 |
| `data.error_code` | 字符串，可省略 | 失败状态的安全错误码。 |
| `data.error_message` | 字符串，可省略 | 失败状态的安全错误信息。 |
| `data.created_at` | RFC 3339 字符串，可省略 | 草稿创建时间。 |
| `data.updated_at` | RFC 3339 字符串 | 更新时间。 |
| `data.finalized_at` | RFC 3339 字符串，可省略 | 草稿阶段省略。 |

### request_api 示例

```json
{
  "method": "PATCH",
  "url": "/api/v1/projects/<project_uuid>/project-setup",
  "request_body": {
    "expected_revision": 1,
    "project_name": "云海灯塔",
    "generation_brief": "一只小火车穿越云海灯塔群，帮助迷路的星星回家。"
  },
  "response_filter": ".data | {uuid,project_uuid,setup_status,status,revision,draft_values,field_sources,missing_information,final_picture_book,reference_plan,updated_at}"
}
```

### 接口约束

- `expected_revision` 之外，至少提交 `project_name`、`generation_language`、`overall_style`、`generation_brief`、`picture_book` 之一；冲突后重新 GET，不得盲重试。
- `classic_picture_book` 可传比例和 `large_image_minimal_text`，不得传互动或漫画字段。
- `wordless_picture_book` 只可传比例，不得传其他形式专属字段。
- `interactive_picture_book` 不得传比例、少字或漫画字段。
- `comic_story` 可传比例和 `comic_layout`，不得传少字或互动字段。
- `vertical_strip` 只传 `format`；固定比例为 1:3，不得传比例或其他形式专属字段。
- 比例预设 `landscape`、`square`、`portrait` 分别规范化为 4:3、1:1、3:4；`custom` 的宽高须同时提供、比例位于 1:3 到 3:1，并由服务端约分。
- GET 返回的比例含规范化 `width`/`height`；PATCH 预设模式时不得将这两个输出字段原样回传。
- 合法 `picture_book` 值示例：经典默认 4:3 为 `{"format":"classic_picture_book","large_image_minimal_text":false}`；经典自定义 3:2 为 `{"format":"classic_picture_book","aspect_ratio":{"mode":"custom","width":3,"height":2}}`；无字横向为 `{"format":"wordless_picture_book","aspect_ratio":{"mode":"landscape"}}`；条漫为 `{"format":"vertical_strip"}`。
- 不确定的信息应向用户询问，不得臆造具体人物、剧情或风格偏好。

## `POST /api/v1/projects/{project_uuid}/project-setup-finalizations`

确认完整 Setup Draft，将不可变绘本规格写入正式项目并切换为 `ready`。

### 请求字段

| 字段 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `project_uuid` | path | UUIDv7 字符串 | 是 | 当前项目公开 UUID。 |
| `expected_revision` | body | 整数，1～2147483647 | 是 | 已向用户展示且刚读取到的完整 Setup Draft revision。 |

### 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.uuid` | UUIDv7 字符串 | 已定稿 Setup Draft UUID。 |
| `data.project_uuid` | UUIDv7 字符串 | 所属项目 UUID。 |
| `data.setup_status` | 字符串 | 成功后为 `ready`。 |
| `data.status` | 字符串 | 成功后为 `finalized`。 |
| `data.revision` | 整数 | 草稿最终 revision。 |
| `data.original_input` | 字符串，可省略 | 首页原始输入。 |
| `data.draft_values` | 对象 | 定稿时使用的完整草稿值。 |
| `data.field_sources` | 字符串映射 | 已确认字段统一为 `user_confirmed`。 |
| `data.missing_information` | 字符串数组 | 成功后固定为空数组。 |
| `data.final_picture_book` | 对象 | 正式不可变绘本规格；含 `format`、规范化 `aspect_ratio` 和适用的形式专属字段。 |
| `data.reference_plan` | 对象 | 已冻结的系统托管视觉参考事实；历史来源和值保持不变。 |
| `data.error_code` | 字符串，可省略 | 成功时省略。 |
| `data.error_message` | 字符串，可省略 | 成功时省略。 |
| `data.created_at` | RFC 3339 字符串，可省略 | 草稿创建时间。 |
| `data.updated_at` | RFC 3339 字符串 | 定稿更新时间。 |
| `data.finalized_at` | RFC 3339 字符串 | 定稿时间。 |

### request_api 示例

```json
{
  "method": "POST",
  "url": "/api/v1/projects/<project_uuid>/project-setup-finalizations",
  "request_body": {
    "expected_revision": 5
  },
  "response_filter": ".data | {uuid,project_uuid,setup_status,status,revision,draft_values,field_sources,missing_information,final_picture_book,reference_plan,updated_at}"
}
```

### 接口约束

- 危险操作；先清楚展示全部待确认设置、默认来源和缺失信息。若带图，只展示视觉参考数量并说明会自动用于画面生成，不逐项展示或询问计划字段；随后提交一次完整请求，确认由运行时生成。
- 仅 `setup_status=draft`、草稿信息完整且 revision 匹配时可定稿；成功后正式绘本规格不可修改。
- 同一已成功 revision 的自动重放幂等返回既有结果；不同 revision 会冲突。
