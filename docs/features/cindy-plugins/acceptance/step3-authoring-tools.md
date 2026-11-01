# Step 3 验收：文本创作工具与流程

2026-11-01，macOS arm64，Node v25.2.1，Go 1.27.3。**本步完成**：目录、分发器、允许列表、串行创作 Manual、固定文本三页端到端、原生任务文件交付、大结果边界与 Forge 开发包均已验证。插件未安装、未发布；未运行 Cargo/Rust，未修改 `internal/mcp` 旧 MCP。

## 最终检查输出

```text
node cindy-plugins/scripts/build.mjs
  source_commit d6a85e52a894e1d8dd267e01eedd268a36d80256; source_dirty true
  cli_sha256 ce4b5b5f9848bb4ef7776d8b971749a4c49fbb6356a2227e798908d0b85f3303
node cindy-plugins/scripts/verify.mjs step3
  tests 60; pass 60; fail 0; cancelled 0; skipped 0
go test ./internal/promptcatalog ./internal/story ./internal/textmcp ./internal/project ./cmd/lumi ./cindy-plugins/schema
  ok lumi/internal/promptcatalog (cached)
  ok lumi/internal/story (cached)
  ok lumi/internal/textmcp 8.604s
  ok lumi/internal/project (cached)
  ok lumi/cmd/lumi 7.599s
  lumi/cindy-plugins/schema [no test files]
node --test web/src/api/realtime.test.js web/src/realtime/projectRealtimeQueries.test.js
  tests 25; pass 25; fail 0; cancelled 0; skipped 0
ghost_forge_pack({dir:"/Users/qingyang/mine-work/lumi/cindy-plugins/lumi"})
  ok true; lumi-0.3.0.cindy; no installation
python3 cindy-plugins/scripts/verify-package.py step3
  PASS: 18 archive entries, manifest, source bytes, CLI SHA256/mode, size limits; no installation
  package_sha256 bc2a8641cd21581dddab8bb0584a76e0cc36eb81cfc4e28787a4198833d3acfe
git diff --check -- <本步涉及的路径>
  exit 0
```

Node 详细输出及最终 CLI 信息见 [automated.json](step3-evidence/automated.json)；Go/前端检查记录见 [checks.json](step3-evidence/checks.json)。源码存在并行工作，未重置、暂存或提交其他改动，CLI 如实记录 source_dirty。

## 工具与业务验证

manifest 仅暴露 list_tools / call_tool，八个类别中 media、review 明确 not_implemented 且工具为空；13 项允许业务/运行工具不会随上游 MCP 自动扩展。业务 schema 由实际 `internal/textmcp.Specs()` 生成；Cindy 的文字目标限制及返回变化见 [工具契约](../tools.md)。未知工具、非法 action、额外字段、条件必填、类型、UUID、长度和数量边界均拒绝，并给出 schema 与 RULES。

端到端使用 Runtime → 两段式 call_tool → 真实随包 MCP/CLI → Manager 与 SQLite。在隔离临时项目内，无 provider 配置即可完成 draft/ready setup、故事、章节正文、文字角色和三页正文/画面描述。故事、章节、设定和分镜任务经 render_prompt 渲染，固定文字模拟创作结果，再用正常业务工具保存。Markdown 导出逐项核对完整故事、角色、章正文、三页正文与全部画面描述、字节数及页数。临时项目与测试自有服务清理完成。

**这是固定输出工具链验证，不是实际 Agent 自主创作**；后者留到 Step 6。当前 Agent 串行流程见包内 authoring Manual，无多 Agent 前提，不复制 promptcatalog 模板，不在 Lumi 内调用模型或生图 pipeline。

同项目第二任务读取旧 revision 后写入被拒绝，重读确认首任务内容完整保留；模拟已提交写入但回执丢失，重读恢复内容且写入次数始终为 1。跨项目目标及提示词上下文 UUID 被拒绝。项目锁、重复初始化、复制项目、目录身份变化、worker/MCP 取消和恢复的既有回归均通过。

只读任务可查看已运行 CLI 已打开的项目。Node 对所有 action 精确分类，初始化、finalize、select_image、导出均在派发前拒绝；Go 再次授权，固定连接 query_only 防止懒写。测试确认不恢复外部 STORY.md、不打开已关闭项目、拒绝意外 SQL 写入、取消后连接恢复可写。未打开项目须先由可写任务打开，不迁移或修复作品以绕过只读限制。

## 完整结果与错误边界

真实 CLI 创建三个十万字节章节：整列表返回 result_too_large，limit=1 沿游标重读逐项确认正文与尾部标记完全一致；整提示词超限时拒绝，明确空上下文章节列表后成功取得完整任务。输入超过 MCP 240 KiB 在连接和写入前返回 arguments_too_large 与 schema，不误报 result_unknown；Node 256 KiB 输入、约 1 MiB 单行保护仍保留。业务/MCP 结果界限为 256 KiB，包含 text 与 structuredContent 编码，不截掉正文冒充完整。

对当前安装 Host 的编译纯函数，使用构造文本输入验证 dispatcher → ghost_call → Agent 结果保留 125087 字节及尾标记；失败转为 isError，错误码与 schema/RULES 消息完整。Host 只转发失败 errorCode/message，因此适配层将自纠信息写入 message，错误码使用 `LUMI_<大写业务码>`，原始业务码仍可识别。

手机工具历史记录的显示投影上限为 8 KiB，超限明确附 `[remote content truncated: payload too large]` 且不破坏 UTF-8。它不能作为完整正文或完整调用记录；完整作品通过独立任务文件浏览器读取。证据见 [host-result-boundaries.json](step3-evidence/host-result-boundaries.json)。此审计运行真实编译函数，但模拟 transport、owner scope 及未使用的媒体辅助函数，**不冒充已安装插件 E2E**。

## 文件生成和手机接收

适配层只复制本次成功导出的固定路径文件，流式完整复制到任务可见 `lumi-exports/`，原子发布且不覆盖。返回相对 path、大小与 SHA256，以及 file_generated=true、user_received=false、delivery.status=prepared。没有任意路径参数。测试覆盖大文件全部字节、链接拒绝、覆盖拒绝、取消及清理；交付准备失败时保留原始生成文件，以 export_delivery_failed 和 generated_export 回执报告，不自动重新导出。

用于用户审阅的 [fixed-book.md](step3-evidence/fixed-book.md) 为三页《回家的星星》，3143 字节，SHA256 `d0d800424931601e25f2181ff1256adbe782b777754d5f8197536ed9445ebb54`。后续测试重新生成的 UUID 和时间可能变化，这份已确认文件保持不变。

1. [fixed-book-receipt.json](step3-evidence/fixed-book-receipt.json) 保存生成时 prepared 回执；这是交付准备，不声称收到。
2. 实际随包 Cindy file-service 完成 handshake、可见列表与分块全字节读取，SHA256 与作品一致，见 [host-file-service.json](step3-evidence/host-file-service.json)。该探针只验证原生传输后端，本身不出具手机回执。
3. 用户从 Cindy 手机当前任务文件列表打开该文件，明确回答 **“已在手机完整打开”**，见 [mobile-delivery.json](step3-evidence/mobile-delivery.json)，独立记录 user_received=true。
4. [手机截图](step3-evidence/mobile-file-open.webp) 显示 fixed-book.md 的故事、角色、章节及第 1/2 页正文和画面描述；第三页完整性由用户明确确认及完整文件字节核对支持，不夸大截图的可见范围。

因此本次分别有“文件已生成”和“用户已收到”证据；不是拿 Mac 绝对路径或本地链接冒充手机下载，也不是伪造插件自动接收回调。

## WS/REST 与最终包

Go `TestStdioTextCreationEndToEndWithoutProviders` 验证业务写入发出既有 WS UUID 提示、REST 读取相同正文及不可变历史，且不产生 provider/task/log 流水线。前端 25 项回归验证 rejoin、Query 精确失效、setup/story/premise 提示与 REST 重读，以及业务页面无定时 refetch；首次 join 和窗口聚焦沿用已有校准流程。

Forge 已校验并生成 [lumi-0.3.0.cindy](../../../../cindy-plugins/lumi/lumi-0.3.0.cindy)，21284783 字节。包校验覆盖全部 18 个条目、manifest、目录/schema/Manual/JS 源字节、CLI SHA256 与 755 执行位、包大小边界，排除测试、数据库及 node_modules，见 [package.json](step3-evidence/package.json)。本步不安装或发布插件。

执行宿主 Cindy 0.1.121，sourceCommit `0a7d5a419f5cd3e0fbebc9642dde8388376a68aa`。精确公开源码未取得，依随包 Skill 和已安装应用构建产物验证，不用不匹配 tag/main 替代。当前 macOS arm64 开发包的 minCindyVersion 不是最低正式兼容版本证明，正式发布和真实 Agent 自主创作仍按后续步骤验证。
