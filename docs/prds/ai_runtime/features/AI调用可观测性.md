# AI 运行时 — AI调用可观测性

## overview

该 Feature 为单机项目提供统一、可恢复的 AI 调用审计视图。Story、Project Chat、Premise/漫画 Production 与 Workflow 调用都进入同一项目日志；用户可以按 Provider、模型、scenario、状态、请求类型和关键词组合筛选，并从公开 Task、Thread、Run、Workflow UUID 追踪调用上下文。

新 Chat 文本和图片调用的 scenario 统一为 `project_chat`，对外不返回 Scene。项目级日志包含全部 Chat；Premise scope 只展示 Premise Production 调用。历史 scenario 继续可筛选和阅读，但不会影响新日志归类。

列表只搜索受限长度的输入/输出摘要、模型、scenario、错误码和 Provider request id，不对原始请求/响应 JSON 做无界扫描。详情保留既有安全 payload 阅读器，但不保存或返回请求 header、API Key、Authorization、二进制图片或内部数据库 ID。

文本调用在 Provider 返回 usage 时记录输入、缓存输入和输出 token。输入/输出字符数统一统计 JSON 字符串值中的 Unicode code point；输出 token/s 和字符/s 根据正耗时读取时推导。Provider 未提供的缓存指标、迁移前日志、图片请求和无效耗时显示不可用，不伪造为 0。

## data_model

`llm_logs` 通过内部 bigint 外键关联 Task、Production task、Chat 或 Workflow 上下文；`uuid` 与关联资源 UUIDv7 是唯一外部身份。迁移新增可空 `cached_input_tokens`、`input_characters`、`output_characters`，并为项目 + Provider、项目 + scenario 建立历史索引。速度字段是读取投影，不写入 SQLite。

## api

| 接口 | 方法 | 说明 |
|---|---|---|
| `/api/v1/projects/:project_uuid/llm-logs?page=&per_page=&scope=` | GET | 返回 `{items,pagination,filter_groups}`；可组合 `provider_uuid`、`provider_type`、`model`、`scenario`、`status`、`request_type`、`keyword`，筛选发生在分页前。 |
| `/api/v1/projects/:project_uuid/llm-logs/:log_uuid` | GET | 返回单条日志及安全请求/响应 JSON；关联上下文只含公开 UUIDv7。 |

`filter_groups` 以当前 project/scope 为边界返回 Provider、Provider type、model、scenario、status 和 request type。列表按 `created_at DESC` 与稳定 UUID 次序分页；筛选非法时返回统一 `validation_failed` 错误信封。

## ui

| 页面 / 入口 | 说明 |
|---|---|
| 项目或 Premise 的 LLM Logs 面板 | 提供有 label 的组合筛选、关键词提交和重置；筛选变化回到第一页，窄屏改为单列。 |
| 日志表格与详情 Dialog | 展示 token、缓存 token、输入/输出字符数、token/s、字符/s、耗时和公开诊断关联；图片与缺失指标显示“—”。 |

## others

该能力不估算 credits、订阅费用或 Provider 实际账单。WebSocket 只用于触发查询刷新，REST 与项目 SQLite 仍是事实源。
