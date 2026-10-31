# Step 7 · Codex 会话记录手动导入

验证日期：2026-10-31（Asia/Shanghai）。

**本步按前置门槛收敛为手动 `lumi session import --rollout <path>`。** Codex App 的项目 hook 执行、审阅信任入口及结束事件时序仍未得到验证，不交付自动 hook、后台扫描或定时导入。默认不采集记录；只有用户主动执行导入命令才写入项目。旧 MCP 的实现、授权、发现和活动历史均不改动。

## App 补测结果与证据边界

环境：`/Applications/ChatGPT.app`，版本 `26.1027.30824`，build `22943`；真实 App rollout 元数据的 `originator` 为 `Codex Desktop`，`cli_version` 为 `0.166.0-alpha.5.1`。`source` 实际为 `vscode`，因此不能只用该字段判断是否来自 App。

| 待验证项 | 本次结果 | 对实现的影响 |
| --- | --- | --- |
| App 是否执行项目级 hook；App 中审阅并信任 hook 的入口 | 尝试通过电脑控制工具访问 `com.openai.codex`，工具明确返回 “Computer Use is not allowed to use the app 'com.openai.codex' for safety reasons.”。未绕过限制。官方 Hooks 文档仅明确说明 CLI 的 `/hooks`，未据此推断 App 入口 | 不安装 hook，不声称已验证 App 自动导入 |
| `Stop`、`PostToolUse`、`SessionEnd`、`Interrupt` 的实际触发时机和 payload | 没有可用的 App 信任流程实测，未执行这些 hook，也没有新的 App hook payload 证据 | 不用 CLI 事件枚举或旧断网轮次替代 App 验收 |
| 成功轮次结束后的完整助手输出 | 只读检查已有、同项目 App rollout：最后的 `response_item.message`（`role=assistant, phase=final_answer`）与 `event_msg.task_complete.last_agent_message` 完全一致，样本长度 19 字符 | 支持结束后手动读取；不能证明 `Stop` 时已刷盘，也不能保证全部版本或全部输出类型 |
| 中断 | 同项目已有 App rollout 存在 `event_msg.turn_aborted`；本次未主动中断对话，未录制 Interrupt hook | 只导入实际已落盘文本，不推断运行状态；半行留到下次 |
| 恢复会话与路径变化 | 未进行受控的 App 恢复前后对照 | 不假定路径稳定；以会话 UUID 和已消费字节校验，文件缺失、截断或改写时警告 |
| 上下文压缩 | 同项目已有 App rollout 观察到 `compacted`，含 `replacement_history` 等字段；未做受控压缩前后路径对照 | 忽略压缩摘要和替换历史，避免把它们当成新用户/助手消息；不承诺重建被删历史 |

成功输出结构来自 `rollout-2026-10-31T18-01-56-01a19168-a300-7af1-a0ea-4fe8e03b75b4.jsonl`；commentary 结构来自本次会话。仓库只保存替换了正文、UUID、目录并删去私人元数据的 [录制 fixture](../../../internal/sessionimport/testdata/README.md)，没有保存原始会话全文。工具调用边界案例另外标明为人工构造，不能当成 App 工具/hook 实测。

官方依据：[Hooks](https://learn.chatgpt.com/docs/hooks)。该文档要求项目信任与准确 hook 定义的信任分别满足；定义变更后重新审阅，插件安装不会自动信任 hook。未来若恢复自动导入，仍须验证 App 的这两步入口、读取真实 trust 状态并提示用户亲自操作。**本实现不提供绕过信任的参数，也不写入信任记录。**

## 使用

在已初始化的 Lumi 项目根目录执行：

```sh
lumi session import --rollout /absolute/path/to/rollout.jsonl
# 可读 JSON 信封；警告同时写入 stderr
lumi session import --rollout /absolute/path/to/rollout.jsonl --json
```

相对 rollout 路径按命令 cwd 解析。沿用 CLI 的 `--environment`、`--data-dir`、`--timeout`。默认超时 90 秒；非常大的记录可按需增加 `--timeout`。不搜索用户全部会话，不从文件名猜会话身份，也不从 stdin 自动消费 hook payload。缺少 `--rollout` 返回参数错误。

命令通过已有单实例 CLI server 和项目锁导入，不直接另开 SQLite 写连接。若桌面版正持有项目，先关闭该项目，再运行命令；导入后用相同 environment/data-dir 执行 `lumi stop` 释放项目，再从桌面版打开，记录保留。CLI 的完整 Web UI 也使用同一个 ChatArea。

`session enable` / `session disable` 返回 `session_hooks_unavailable`，不修改配置。由于采用手动降级范围，本版没有可开启的自动采集开关：停止执行 import 即停止采集，追加 rollout、重开项目或安装插件均不会自动导入，已有记录继续保留。此前自行编写的自定义脚本不由 Lumi 管理。

## 保存内容与边界

- 用户、助手的纯文本，助手 commentary 与 final answer 均保留；使用 `response_item` 作为唯一消息来源，忽略重复的 `event_msg` 投影。
- 直接记录为 `function_call` / `custom_tool_call` 的新 Lumi 插件工具名称摘要，例如 `Lumi · pages`。只识别 `mcp__lumi__`、`mcp__lumi_review__` 及同名 namespace 下的已知工具；摘要表示调用记录，不宣称成功。不会分析任意 shell 命令或 `functions.exec` 脚本以猜测嵌套调用；这类不透明调用不形成摘要。
- 不保存完整工具参数、任何工具输出、reasoning、系统/开发者指令、已知 App 注入上下文、图片块或压缩替换历史。文本里的 Markdown 图片及 data URI 被替换为省略标记。正文按纯文本展示，不渲染 HTML 或加载远程图片。
- 单条 JSONL 上限 8 MiB，超过时消费该完整行并警告；单条文本上限 256 KiB，超出时按 UTF-8 边界截断并标记。未知格式不猜测，不保存原始 payload。已知注入上下文依据明确前缀过滤，因此用户手工粘贴完全相同前缀的文字也会被省略。
- rollout 的 `session_meta.cwd` 必须是当前项目根目录或真实子目录（解析 symlink 后校验），会话标识必须为 UUIDv7。跨项目记录被跳过。

迁移 `20261031000050_add_codex_sessions` 为会话列表增加 `thread_type=codex_session`，消息独立存于 `codex_session_items`，导入位置独立存于 `codex_sessions`。不写入 `chat_turns` / `chat_runs` / `chat_items`，不创建模型、任务或运行状态。内部关联沿用 bigint 自增 ID，外部资源使用新生成的 UUIDv7。

`codex_sessions` 保存 Codex 会话 UUID、rollout 路径引用、已消费偏移、前缀 SHA-256 和导入时间；路径变化且内容校验通过时更新引用。不复制完整 rollout。`session_id + source_offset` 唯一约束与事务检查保证重复/并发触发不重复保存。每次只解析新增完整行，读取前缀计算哈希以检测历史改写；因此校验 I/O 随已读文件大小增长，不是恒定时间。最后未换行的记录即使 JSON 完整也暂不消费。

旧聊天表按现有安全重建迁移机制扩展类型约束，保留主键、外键关系、索引、自增序列和旧数据；存在 Codex 导入记录时拒绝有损 downgrade。

## API 与桌面展示

- CLI 专用写入口：`POST /api/v1/session-imports`，使用现有 CLI 实例 nonce 校验，输入仅为内部本机传输的 `root_path` / `rollout_path`。桌面版不注册这个写入口。
- 共用会话列表与详情：`GET /api/v1/projects/:project_uuid/chat_threads` 及 `/:thread_uuid`。
- 只读消息：`GET /api/v1/projects/:project_uuid/chat_threads/:thread_uuid/codex_session_items?before=&after=&limit=`，默认 50，最大 100；同一请求不得同时传 before/after，cursor 为属于该 thread 的消息 UUIDv7。
- 响应使用标准 success/data/error 信封和 `snake_case`；history 返回 `items` 与 `cursor_pagination`，不返回内部 ID、原始路径、偏移或工具 payload。
- ChatArea 列表标明「Codex · 只读」，详情显示用户/助手/工具摘要与记录时间，没有输入框、运行状态、模型配置或运行轨迹入口。服务端拒绝 turn/follow-up 写入及原生运行轨迹请求。
- 导入事务提交后，通过 `/api/v1/ws` 的项目 topic 发布 `codex_session:changed`，payload 只含公开的 `project_uuid` / `thread_uuid`。前端失效对应列表、详情与导入消息查询，再经 REST 读取 SQLite；首次 join、重连和窗口聚焦沿用项目级校准，无定时 HTTP 轮询。

## 降级与警告

文件缺失、空路径或无法识别首条元数据时，返回 `success: true`、`skipped: true` 与警告，不创建 thread。不匹配项目或会话标识无效同样跳过。已导入文件被截断或改写时保留旧数据、偏移和哈希，不合并新内容。格式未知行被跳过，完整支持行仍可导入；重复 import 不会重新解析已消费的未知行。适配器升级后不会自动回填这些被跳过的旧行。

常见警告：`rollout_unavailable`、`rollout_format_unknown`、`session_project_mismatch`、`rollout_truncated`、`rollout_rewritten`、`rollout_incomplete_tail`、`record_type_unknown`、`record_too_large`、`message_truncated`。SQLite、项目锁、网络或超时错误仍作为手动命令错误返回，事务回滚；没有 hook 被安装，因而不会阻断 Codex 对话。

## 验证

已覆盖录制 fixture 首次/增量/重复导入、并发幂等、未知格式、缺失/移动/截断/改写、半行及无末尾换行、跨项目拒绝、图片/参数/输出/推理过滤、压缩去重、分页归属和限制；真实 CLI → server → SQLite → REST/WS → 桌面项目重开，拒绝原生对话写入；迁移 up/down 保留旧会话关联、自增与外键完整性。

结果：Go 全量回归通过，前端 436 项测试通过，Vite 主站与 MCP App 生产构建通过（保留既有的大 chunk 提示）。浏览器使用临时 CLI 项目和共享桌面 ChatArea 验收：列表标记为只读，6 条文本/工具摘要可见，输入框、composer、运行状态、轨迹入口及正文图片节点均为 0；仅追加文件不导入，执行手动增量导入后无需刷新即可显示第 7 条。浏览器控制台无 error/warn。未运行原生桌面二进制，但测试已用正常桌面项目 Manager 重新打开数据库并读取同一历史。

```sh
go test ./...
pnpm --dir web test
pnpm --dir web build
```

未运行 Cargo 或 Rust 命令。App 自动 hook 验收仍按上表保留为未验证，不列入已完成能力。
