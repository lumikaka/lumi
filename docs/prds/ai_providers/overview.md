# AI Provider — 全局凭据、默认模型与连接状态

## 模块职责

AI Provider 模块负责应用级 Provider 的安全配置、激活、默认模型和连接验证。它向项目提供公开 Provider UUIDv7、ready/active 状态和文本/图片模型选项，但不承担项目内场景覆盖、任务冻结或调用日志。

## 职责边界

| 范围 | 说明 |
|---|---|
| 负责 | Provider 身份、Account/Workspace 配置、加密 API Key、默认文本/图片模型、连接检查、验证失效和 active Provider。 |
| 不负责 | 项目级模型覆盖、Prompt、实际模型调用、任务、调用审计和计费。 |

## 核心概念

### Provider 投影

产品资源名为 Provider，目录名保留 `ai_providers` 这一重要业务概念；当前持久化事实表是应用库 `site_settings`。早期 `ai_providers` migration 已在后续迁移中删除，不能作为当前数据模型。

### ready 状态

Provider 同时满足配置完整、密钥存在且连接验证与当前配置指纹匹配时才 ready。只有 ready Provider 可以激活或参与项目模型解析。

## Feature 列表

| Feature | 文档 | 说明 |
|---|---|---|
| `AIProvider安全配置与连接验证` | [`features/AIProvider安全配置与连接验证.md`](features/AIProvider安全配置与连接验证.md) | 安全保存配置、验证连接并选择全局默认 Provider。 |

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| AI 运行时 | 读取 Provider ready/active 状态和默认模型，项目库只保存 Provider UUIDv7。 |
| 项目 | 新建和执行项目任务前可检查文本或图片模型可用性。 |
| 实时通信 | 全局设置变化发布 `site_settings:updated`，客户端据此重读 Provider 事实状态。 |
