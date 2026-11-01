# Step 3 · 文本创作工具与流程

## 目标

让 Cindy Agent 在不配置 Lumi 模型服务商的情况下，通过插件完成 setup、故事、设定、章节、页面正文和画面描述，并导出一份完整文本作品。

## 前置

完成 [Step 2](step2-project-binding.md)。阅读 [现有工具契约](../codex-plugins/tools.md)，以实际 `internal/textmcp` schema 为准，避免把旧计划当成当前实现。

## 范围

1. 现有工具已超过 Cindy 16 个顶层工具的限额，采用 `list_tools` / `call_tool` 两段式发现。按 `project`、`story`、`settings`、`pages`、`prompts`、`media`、`review`、`export` 分类；未实现类别标注状态，不提供会假成功的占位工具。
2. `list_tools` 无 category 返回概览；指定 category 返回工具名、说明、JSON Schema 和该类必要 RULES。未知工具/错误参数返回正确 schema 与自纠规则，不能要求 Agent 猜调用协议。
3. 主线复用下列业务能力，保持 UUID、revision、不可变正文历史、软删除和 setup 约束：

   | 类别 | 首版工具 |
   | --- | --- |
   | 项目 | `project_init`、`project_overview`、`project_setup` |
   | 故事 | `story_profile`、`chapters`、`book_outline` |
   | 设定 | `settings` |
   | 页面 | `pages` |
   | 提示词 | `prompt_catalog`、`render_prompt` |
   | 导出 | `export_book` |

4. 复用现有 schema 与业务返回，Cindy 适配层单独声明有意变化，例如不暴露任意路径的图片接收工具。`call_tool` 使用明确允许列表，不是任意 MCP method/CLI/HTTP/SQL 代理；MCP App-only 工具不因此自动开放给 Agent。
5. 对 `action` 子操作精确判断读写。只读任务可列举/查看，不得通过 `finalize`、`select_image`、导出落盘、初始化或其他写操作绕过限制；拒绝发生在派发前。
6. Lumi 的 `success:false`、MCP `isError` 与 Host 工具失败保持一致；不把失败信封套在 `ok:true` 中当作成功。工具结果包含必要完整文本，受大小限制时分页/缩小范围，不能截掉作品正文假装完整。
7. `render_prompt` 是业务提示词唯一来源。返回包含目标 UUID、`expected_revision`、任务输入和写回契约；Agent 创作后通过相应工具保存。CLI 不启动已有 LLM/生图 pipeline。
8. 在插件中建立最小 Manual：项目识别 → setup → 故事 → 设定 → 章节/页面 → 审阅 → 导出，附版本冲突、取消和额度耗尽后的恢复。首版以当前 Agent 串行为可运行路径，多 Agent 不作为完成前提。
9. 对导出结果分别报告文件已生成与用户已收到。手机远程访问不能只回 Mac 绝对路径；须走实测可用的 Cindy 文件交付能力，不能声称本地路径在手机可下载。

## 交付

- 工具目录、分发器、允许列表和最小创作 Manual。
- `docs/features/cindy-plugins/tools.md`：Cindy 顶层工具、类别、与现有工具的差异、读写判定、错误及结果边界；共用业务参数链接到既有契约。
- 端到端临时项目验证：固定文本模拟模型输出，至少 3 页，包含正文和画面描述，导出后核对内容。

## 禁止

不在 Manual 复制 promptcatalog 模板，不扩大任意路径/命令能力，不绕过项目锁和 expected_revision，不在 Lumi 内部调用模型服务商，不改旧 MCP，不运行 Cargo/Rust。

## 完成标准

- 从发现目录到无服务商 setup、故事写回、设定和三页文本、导出完整可用。
- 两任务同项目发生版本冲突时保留已有内容，重读后再判断；中断后的未知结果不自动重写。
- 业务写入发出既有 WS 提示，原有 CLI web UI 经 REST 重读；无定时业务轮询。
- 目录/schema/读写授权与实际工具一致，非法 action、未知字段、超限和跨项目 UUID 被拒绝。
- Node 适配测试和实际修改的 Go 测试通过；固定输出测试不冒充真实 Agent 自主创作，后者留到 Step 6。
