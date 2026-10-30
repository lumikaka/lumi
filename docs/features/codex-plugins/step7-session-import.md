# Step 7（可选）· 用 Hook 导入 Codex 会话记录

## 目标

在用户主动开启时，把 Codex 中与该项目相关的会话记录以只读形式保存进项目，便于在桌面版回看创作过程。默认关闭，不影响主线功能。

## 背景

- 方案：`docs/features/codex-plugins/plan.md`（「探索能否用 hook 把聊天记录也存进 project.sqlite，可能可以通过 Lumi CLI 实现」）。
- 调研结论：`docs/features/codex-plugins/spike.md` 中的 hooks 部分（事件、transcript / rollout 路径、信任要求）。
- 项目内现有聊天模型：`chat_turns` / `chat_runs` / `chat_items`，以及 thread 列表与 ChatArea（`docs/chat-area-implementation.md`）。

## 范围

1. `lumi session import`：读取 hook 传入的会话信息，按 rollout 增量导入。
   - 只导入用户消息、助手消息、Lumi 工具调用的摘要；不保存完整工具参数、图片内容或与 Lumi 无关的命令输出。
   - 用 Codex 会话 ID 加偏移量保证幂等，重复触发不产生重复记录。
   - 存为单独的只读 thread 类型，不写入或冒充 `chat_turns` / `chat_runs`。
   - 记录 rollout 原始路径作为引用，不复制完整 rollout 文件。
2. 开关：`lumi session enable|disable` 写入或移除项目级 hook 配置；默认关闭，开启时向用户说明会保存哪些内容。
3. 桌面版 ChatArea 能列出并只读展示这类 thread。
4. 容错：rollout 格式不识别时跳过并记录警告，绝不让 hook 失败影响 Codex 对话。

## 禁止

- 不修改旧 MCP 逻辑。
- 不默认开启。
- 不在 project migration 中破坏现有 chat 表；新增表或字段需向后兼容。
- 不运行任何 Cargo / Rust 命令。

## 完成标准

- 用录制的 rollout fixture 测试：首次导入、增量导入、重复导入幂等、格式未知时降级。
- 关闭开关后不再导入，已导入记录保留。
- 桌面版能展示导入的 thread，且无输入框、无运行状态。
- 新增 `docs/features/codex-plugins/session-import.md`。
