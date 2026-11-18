# Cindy 对齐实现审查与交付（2026-11-18）

用户最新安排为“后续自己另开任务人工验收，先实现并 review”。本次实现、审查、自动化和打包已完成；完整真机验收由用户后续执行，不将未执行项目记为通过。对照基线为 `5f26922f5bd1532a3b4984e0ca1da8376e58d41f`，历史 Cindy 为 `bb1ca879be47ceef0a1ff43ffab12cbb4fccc75c`。保留当前树后续同步的代码；构建实际 HEAD 为 `33aaae40d7ecf7461f54d44f16481dfb2b9c32f1`。

## 按用户操作的最终实现对照

| 操作 | 实现结果 | 主要实现与验证 |
| --- | --- | --- |
| 在当前对话打开可编辑工作区 | 完成 | ghost.json 的标准 MCP Apps 资源、6 个 review_*、5 个 app-only 工具；main.js 接收每次 Host 注入的任务上下文；web/cindy-app 复用既有页面 |
| 总览、章列表、章/页编辑、设定、阅读、候选和版本采用 | 完成 | MemoryRouter、共享组件与 API；地址栏、前进后退、详情返回路径、操作菜单、重连；workspace 集成及此前独立浏览器记录覆盖 |
| 点选页、章、设定、内容引用及文本候选加入对话 | 完成 | 官方 App SDK → updateModelContext；公开 UUID 和摘要；保存、采用、发送分别触发；483 项前端回归包含无业务写入的选择测试 |
| 多章按序、分镜逐页起草、章节统一设定、逐场生产 | 完成 Cindy 适配 | serial-production Manual、production_records / production_tasks；子任务只产草稿，当前任务唯一串行写回；已有分镜直接复用 |
| 默认串行、明确选择后最多两场独立并行、依赖/返修暂停 | 完成 | production-schedule.cjs 以 Lumi 当前页事实核验；当前版 passed、已核实交接、无活动写者与未决请求才放行后场/后章 |
| 中断恢复与避免重复创建/生成/导入 | 完成 | CAS、原子落盘、稳定请求键、原键续接；导入后重启先读事实；两章四页夹具仍仅四次模拟生成、四次导入 |
| 未采用历史版本矩形局部改图 | 完成链路 | reference.cjs 原尺寸裁图与 SHA；main 原生/Core 交接；receive_media 原签名导入；adopted=false，显式采用才切换；旧 revision 拒绝 |
| 合图、无参考图、设定视觉核对、逐页审阅与续做 | 保留并回归 | 复用共享 Go；Cindy 发现说明、Manual 和媒体入口一致 |
| Markdown / ZIP / PDF / 静态网站 | 保留并回归 | 授权任务文件交付；网站实际目录、发布提示词、章节及阅读站/RSS 顺序均有隔离真实 CLI 断言 |

## 审查发现及修复

1. **章节计划可能漏页后提前完成。** 原调度只核实已列场景，遗漏的章内页不会阻止下一章。现在默认范围为当前章全部活动页，按实际顺序完整覆盖；新增页使旧计划重新阻塞。仅在用户明确限定部分页时允许保存带原要求的 `page_scope`，不能通过省略场景缩小范围。活动/暂停写者和未决请求在计划修复时继续占位。生产单元测试新增首尾遗漏、新增页、非法范围与合法部分范围；真实 CLI 多章夹具验证漏页不能放行后章。
2. **晚到错误可能掩盖已保存的成功回执。** main.js 现在以持久成功回执为准；另一个重复请求晚到 OUTCOME_UNKNOWN 不再把已成功结果包装成失败。main.test.cjs 有失败前可复现、修复后通过的断言。
3. **明确权限拒绝后角色无法恢复创建。** 新增 `retry_create`：只替换确定 `PERMISSION_DENIED`、未取得任务身份的失败创建。用户修复授权并明确要求后，原角色原子认领一个不同的新稳定键；原拒绝保留。未知、取消、键冲突或成功请求均不可替换。Store 测试和 main → Runtime → 实际文件 → mock Host 集成检查了只读/外项目拒绝、刷新页面后不重复创建、原回执保留及新任务复用。
4. **发现说明补充实际 setup 顺序。** landscape/square/portrait 预设只传 mode，custom 才传 width/height；update 成功后再用其返回 revision 执行 finalize，避免按预计 revision 预排后续写入。

审查同时核对任务绑定、每次权限检查、旧卡片 expected_project_uuid、参考裁图内容/尺寸/摘要、原样 preparation、导出回执及未知结果边界。未发现尚未修复的阻塞性实现问题。此结论不等于对所有宿主行为的保证；后续真机项目见独立清单。

## 最终验证

| 检查 | 结果 |
| --- | --- |
| Node 全量 | 127/127，失败 0、跳过 0 |
| React/Web 前端 | 483/483，失败 0、跳过 0 |
| Codex 包兼容 | 2/2 通过 |
| Go 业务和 CLI | 6 个包通过；schema 命令包无测试 |
| 三项同源门禁 | authoring 12 组；31 业务/6 开页/5 app-only/2 production 契约；Skills 无漂移 |
| Web、Codex MCP、Cindy 页面及 Go CLI 构建 | 通过；Web 保留原有大 chunk 提示 |
| Forge 与包内容 | 正式、标准开发、可选隔离开发各 32 条目；16 项工具；执行位 755、CLI 摘要、资源绑定、SDK、Manual、逐文件字节核对通过 |

以上为交付时的验证结论；原始日志与回执仅作本地构建产物，放在被 Git 忽略的 `build/cindy-acceptance/`，不作为源码文档依赖。可按文末命令重新核对。

当前修改相对基线没有改变 `web/src/`、`web/mcp-app/`、`internal/`、`cmd/` 或 `codex-plugins/`。因此本次审查修复没有共享桌面 Web 页面增量；前端全量及 Web/MCP 构建重新通过。未在本机运行 Cargo/Rust。

## 包与实际安装的区别

| 包 | 作者目录与产物 | SHA256 |
| --- | --- | --- |
| 标准开发，保留现有后台配置 | `build/cindy-dev/lumi-dev/lumi-dev-0.4.0.cindy` | `e73942cb17ee85a7966438776f1af96d3421c921bcb7ab9dddb935b41648e9f4` |
| 正式 | `cindy-plugins/lumi/lumi-0.4.0.cindy` | `22491fff058c2903b262e3462537e4e257cd23e7be990c23e551865470a3f843` |
| 可选专用验收后台，仅供用户后续明确选择 | `build/cindy-native-acceptance/lumi-dev/lumi-dev-0.4.0.cindy` | `c72786f1dcb7d9bec124937303d3ee7d6a78bce261376ee1d42d215d1efe5992` |

标准开发 build_id 为 `5e688bf3082e45875c43811d3f8ea05fcb5eafaa1cc603c37be56ed932275871`。本次只制作并核对包，未安装、改权限/模型或切换后台；已有 local:lumi-dev 仍是前次原生验收构建 `7559a90e…`，不是这次审查包。正式实例未修改。可选包 data_dir 仍是 `/private/tmp/lumi-cindy-native-acceptance-20261118-data`，没有安装或启动它。历史包按摘要保存在本地 `build/cindy-reviewed/`。

## 剩余宿主差异与人工验收

- Cindy 普通子任务目录由 Host 分配，不能指定同项目目录；子任务产草稿，当前 Lumi 任务核对后唯一串行提交。无 wait_threads，使用 getRun 和原样增量消息游标有界读取；不承诺等同的上下文/token 节省。
- 当前保留的共享后台原生回执为协议 9，历史裁图需要协议 11；旧后台在准备阶段返回 `LUMI_CLI_UPGRADE_REQUIRED`，未提交生图、候选导入或采用。新包包含支持该功能的 CLI，但不会自动停止或替换已有后台。后续需用户选择协调升级该后台，或选用上述独立配置；此前仅做其他独立验收。
- UI 保存及点选后真实下一轮识别已在前次构建取得用户协助的原生证据；完整权限/卡片生命周期、多章真实制作和实际模型历史改图仍交用户在新包上人工验收。[逐步清单及可复制任务](manual-acceptance.md)。

## 可复核命令

在仓库根目录执行（真实 CLI 测试需要本机回环网络可用）：

```sh
node --test --test-reporter=spec cindy-plugins/tests/*.test.cjs
go test ./internal/textmcp ./internal/production ./internal/files ./internal/project ./internal/promptcatalog ./cmd/lumi ./cindy-plugins/schema
node cindy-plugins/scripts/check-authoring.mjs
node cindy-plugins/scripts/sync-contracts.mjs --check
node scripts/build-skills.mjs --check
python3 cindy-plugins/scripts/verify-package.py codex-alignment production
python3 cindy-plugins/scripts/verify-package.py codex-alignment development
```

在 web 目录执行 `node --test --test-reporter=spec`。构建入口为 `node cindy-plugins/scripts/build.mjs --channel production` / `development`，随后通过 Cindy Forge pack 打包，不以构建成功冒充已安装。
