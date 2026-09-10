---
plan_state: finished
git_commit_message: "feat: show summarized MCP activity history in ChatArea"
---

# 在 ChatArea 中展示 MCP 概括性操作历史

## 目标与边界

MCP 活动显示为 ChatArea 原有列表中的只读 thread，无新增页签或独立入口。保留按段归并的操作历史，不展示逐次调用日志、详细参数、完整响应或后台任务进度；不调用 AI 生成概括，不创建聊天消息、Turn 或 Run。

本方案完整替代之前包含后台任务关联、进度同步及最多 5 条重点操作的方案。

## 会话与历史规则

- 按项目和稳定授权来源聚合，OAuth token 刷新保持归属。
- 连续 30 分钟无 MCP 工具调用，下一次受理新建 thread。仅受理时间续期；后台任务、晚返回结果、确认与界面查看不续期。
- 受理时在项目事务中分配持久递增序号，所有历史按受理顺序，而非返回顺序展示。
- 连续成功读取合并；连续成功修改同一资源且操作类别相同，更新当前段。切换操作或资源时新建段，保留之前历史。
- 未返回调用、等待确认和失败都是合并边界，不能跨过未知结果合并。并发调用只有确认属于相邻成功操作后才合并。
- 晚返回失败在原受理位置独立保留；后续成功、幂等重放或过期的校准快照不能覆盖它。
- 生成调用受理成功只记录“已提交”，不关联、查询或追踪后台任务，不回写生成完成。
- 条目保留等待确认及调用结果；thread 不维护运行状态，数据库兼容值固定 idle，UI 不显示该状态。
- 历史不截断，较多时分页加载；段标识保留首条调用 UUID。

## 实施内容

- `chat_threads` 新增 mcp 类型，允许空模型配置，保留 conversation/workflow 模型约束；MCP 输入和 Trajectory 入口受前后端限制。
- 项目库新增 `mcp_threads` 与 `mcp_thread_activities`，保存时间分组、轻量受理元数据与结果，使用 bigint 主外键和对外 UUIDv7。
- 应用库原 `mcp_calls` 保持执行、幂等、确认与结果事实源；调用 UUID 仅作为跨库相关性标识，不复制凭据及完整请求。
- 未完成调用在重新打开项目及历史读取时校准，不通过重执行业务操作恢复展示。
- 新增 `GET /api/v1/projects/:project_uuid/chat_threads/:thread_uuid/mcp_activity`，使用标准成功信封、items 和 cursor_pagination。先归并再分页；cursor 绑定公开 thread/段 UUID 与 revision，变更后返回 mcp_activity_changed，前端从头重读已加载窗口。
- ChatArea 同一列表展示 MCP 来源和时间，详情为模板概括历史，无输入框、状态徽标或后台进度。新增活动不抢占当前选择。
- 复用 mcp:changed 与 TanStack Query 失效后的 REST 重读；首次 join、重新 join 与聚焦校准，无定时 HTTP 轮询。后台任务事件不刷新历史。
- SQLite 父表约束迁移通过显式 opt-in 重建，保留字段顺序及自增序列，提交前检查全部外键；已有 MCP 历史时拒绝有损降级。

## 验证

已覆盖：30 分钟边界、只按受理续期、并发单一 session 和递增序号、逆序返回、晚失败保留、成功填补边界后的合并、资源和动作切换、归并后分页与旧 cursor 拒绝、确认过期、重启恢复、来源隔离、幂等重放、模型为空的只读线程、无聊天消息、真实 MCP SDK → REST 历史链路，以及迁移数据/外键/自增序列保留。

相关 Go 检查包括 internal/mcpserver、internal/agent、internal/server、internal/httpapi、internal/jobqueue、internal/dbmigrate。前端 395 项测试及生产构建通过，构建保留既有大 chunk 提示。测试生成使用模拟服务；本地未运行 Cargo/Rust 命令。

临时项目浏览器验收已确认：MCP 位于原会话列表，读取合并、成功/失败/成功三段历史按顺序保留，无聊天输入框和 thread 运行状态。

使用说明、架构、项目 MCP PRD 和 Chat Thread PRD 已同步。
