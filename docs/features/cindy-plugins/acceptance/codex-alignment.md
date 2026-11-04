# Codex 对齐验收

状态：blocked（宿主条件及剩余真实验收）。可独立开发、自动化、构建/安装核对和本轮授权的一次 Core 合图验收已完成；整体目标未完成。开始日期 2026-11-04；基线、差异与执行计划见 [对齐说明](../codex-alignment.md)。历史 Step 5 跳过不构成本次豁免。

## 证据边界

- 当前宿主只读更新检查：运行 0.1.121，目标 0.1.125；未安装更新。
- 实际应用 sourceCommit：`0a7d5a419f5cd3e0fbebc9642dde8388376a68aa`；精确公开 ref 返回 404。未将其它版本源码当作匹配证据。
- 本次开始时仓库干净；开始时已安装开发版/探针是旧包。随后已走正常 Forge 更新并逐项核对；见下文，不能以版本号相同推断内容相同。
- 自动化固定素材、真实模型调用、原生 UI 与实际文件读取必须分别列出；打包不等于安装或验收。

## 验证清单

| 范围 | 当前状态 |
| --- | --- |
| Go 实际契约 → Cindy 快照/manifest/main 漂移检查 | 实际 Go 生成与 --check 通过，setting_reviews 未丢字段 |
| 两项设定板导入/拆分/核对与页面参考传递 | 自动化通过；真实 Host 固定素材的两项裁切/看图/核对/完整合图入库通过；一次真实 Core image.edit 显式传入同一合图成功，原生引擎未验 |
| 初始化 → 三页正文/分镜/授权导入 → Markdown/ZIP/PDF | 独立真实 CLI/SQLite、固定输出与附件传输自动化通过；非真实自主模型创作 |
| revision、重复请求、取消、未知结果重读 | 工具自动化通过；含原键重放、worker 重建后板导入、冲突与未知结果重读；工作区待验 |
| 双任务/项目数据、图片、缓存隔离 | 工具和工作区缓存隔离自动化通过；Host A 对 B 的设定、图片和页面读取均拒绝；可信原生页面身份仍未验 |
| 工作区保存、预览/采用、排序、删除/恢复 | 内部适配器与真实 CLI/REST/SQLite 自动化通过；浏览器标题保存、图片采用通过；原生 Cindy 仍待验 |
| 首次 join、重连、聚焦、关闭、切换、worker 恢复 | 独立会话复用既有同步/校准；关闭与晚到响应隔离通过；浏览器静默 219 秒无业务重读，A/B 切换与真实 WS 重读通过；原生生命周期待验 |
| Web/Codex 实际页面回归 | 当前 470 项前端测试、Web/MCP 构建、2 项 Codex 包测试通过；共享 Web/Codex 源码和产物未变化；原生 Codex 客户端未重新验收 |
| 开发构建、Forge 包、实例/CLI/后台版本 | 正式/开发构建及 27 条目包通过；开发/探针原位更新并与作者树逐项核对；真实后台 CLI SHA256 和协议版本 3 匹配；正式安装未改 |
| 当前原生宿主页面身份与读写授权生命周期 | 未核验，生产桥关闭 |
| 真实模型及实际文件交付 | 本轮显式授权最多 1 次 Core 请求，实际 request=1/poll=0，已导入并核对候选；原生引擎和本轮控制端文件读取未验 |

## 本次命令与结果

- `node cindy-plugins/scripts/verify.mjs codex-alignment`：本轮最终 94/94 通过，完整测试见 [automated.json](codex-alignment-evidence/automated.json)。包含完整合图的精确字节传输、损坏/不完整/Host 拒绝/错误摘要、并发预算、令牌隔离、释放和权限降级，以及探针条件 schema。另含真实工作区保存/预览/采用/排序/删除恢复及 WS 重读；脚本采用沿用业务创建新的 restore 版本，图片采用保留原版本身份。覆盖初始化/故事/章/三页、两项批次裁切、原子失败回滚、冲突、取消、未知结果、双任务/项目、两环境和服务重启。首次清理检查在已确认服务关闭期间遇到 discovery 瞬态错误，已改为等待明确停止，未把该错误当作停止成功；[修复前记录](codex-alignment-evidence/automated-before-shutdown-wait-fix.json) 保留。随后全量并发暴露 status 身份探测争抢生命周期锁，已仅为该身份读取加入有限次重试，不重放业务或启停服务；[修复前记录](codex-alignment-evidence/automated-before-status-contention-fix.json) 保留。
- `go test ./internal/textmcp ./internal/production ./internal/files ./internal/project ./internal/promptcatalog ./cmd/lumi ./cindy-plugins/schema`：6 个业务/CLI 包通过，schema 命令包无测试文件。
- `cd web && node --test --test-reporter=spec`：470/470 通过，包含 Cindy 请求适配、默认只读、能力拒绝、同项目任务缓存隔离、关闭后晚到结果、pagehide/组件清理幂等以及主题转换。`pnpm build`：Web 和 Codex MCP 均成功；共享源码与原 MCP 产物无 diff。`node --test scripts/test-package-codex.mjs`：2/2 通过。前一轮 465 项回归摘要保留于 [regression-20261104.json](codex-alignment-evidence/regression-20261104.json)。
- `node cindy-plugins/scripts/build.mjs --channel production` 和 `--channel development --data-dir /private/tmp/lumi-cindy-alignment-dev-data`：构建通过；仅在测试独立目录启动服务，不停止用户共用服务。
- Host `ghost_forge_pack` 后，`python3 cindy-plugins/scripts/verify-package.py codex-alignment [development]`：正式/开发包逐文件、身份、环境、CLI 执行位/SHA256、build_id 与大小上限通过。包摘要分别见 [正式包](codex-alignment-evidence/package.json)、[开发包](codex-alignment-evidence/package-development.json)；已按摘要保存审阅副本于 build/cindy-reviewed/，不表示安装。

## 独立浏览器工作区

[浏览器记录](codex-alignment-evidence/browser-workspace-20261104.json) 的身份由夹具模拟，业务链路使用真实产品适配器、CLI、REST 和 SQLite；不计作 Cindy 原生授权通过。实际操作验证：标题从 a page 1 保存为 Cindy edited title；采用后标记由第 2 稿变为第 1 稿；外部修改后页面显示 a external WS change；切换 B 后显示 b chapter/b page 1 且仍使用第 2 稿。新夹具记录静默 219429 ms、6 次初始业务读、后续只有 WS 等待，未使用业务轮询。

暗色检查发现透明白底压缩为八位色值后标签对比度不正确，已修复 Cindy 独立产物的颜色映射并加入测试。复查标签采用暗色表面、浅色文字；[390 px 暗色](codex-alignment-evidence/cindy-dark-390.webp) 与 [320 px 浅色](codex-alignment-evidence/cindy-light-320.webp) 均无页面水平溢出。页面横向缩略图轨道保留原交互，窄屏结果不代表手机原生支持。

本轮执行时 HEAD 已由其它工作前进到 ed935e1469b3f514432c94792f3482da13b26b0d，仅涉及 Windows 安装器校验；基线仍固定为 b4480d9。build-info 和包证据记录实际构建版本。

## 当前宿主复核记录

`node cindy-plugins/scripts/verify-host-taskview.mjs docs/features/cindy-plugins/acceptance/codex-alignment-evidence/host-contract-20261104.json`：实际应用包路由和 taskView 的 session 查询审计通过，可信页面身份仍未核验；[证据](codex-alignment-evidence/host-contract-20261104.json)。这不是穷尽所有未公开能力的声明。

早先已安装历史诊断探针 open_probe 返回当前任务开页成功，随后 report 的 observations 为空，只说明请求已接受。[早先回执](codex-alignment-evidence/host-probe-20261104.json) 不计作生产工作区或页面身份验收。更新探针后可收到 loaded/light、public_context_keys=[locale,region]、has_preload=false 的非可信页面观察，仍不是 Host 身份证明，也没有释放作品给页面。后续项目和 Core 验收与该阶段分开记账。

## 完整合图及真实 Core 验收

开发包经 Forge 正常更新到个人实例 _root__lumi-dev，探针同样原位更新，没有新增实例或 Agent。最终 build_id 为 c97cb3e4ad9b68aa6df77d392b2260a43cf014e1372cc41777b0e4093e427752；随包 CLI SHA256 为 4142d15db995b5fac7fb8535efd5b4fcd07edb6e8ce5edd6aed9bba31cc4812b。正式审阅包 SHA256 为 528d881918804b4d5cb143756fb059b825eff05a8cefd763bc16064b049a8d96，开发审阅包为 c2e2c0977f843a49054abbe44242f05f75ee51f1b159ef6f48e7ed3f5df728cf。安装时重新打包的 ZIP 摘要可变，逐文件内容必须一致。见 [最终回执](codex-alignment-evidence/media-final-install-20261104.json) 和 [安装文件比对](codex-alignment-evidence/media-installed-files-20261104.json)。

可重复核对命令：`python3 scripts/cindy-acceptance/verify-installed.py '<当前已确认的 Cindy owner>/cindy-brain/_ns/_root'`。本次正式包 build_id/CLI 与本轮首次只读观察一致；没有误将 11 月 2 日某次较早安装记录当作当前正式基线。只比较开发/探针全部条目，正式仅复核构建和 CLI 字节，不扩大为正式 UI 全量验收。

旧探针 /private/tmp/lumi-cindy-host-acceptance-20261101 中发现协议不兼容的共享服务。runtime_status 明确 upgrade_required，project_overview 未派发；未停止、升级或删锁。探针改用 /private/tmp/lumi-cindy-alignment-host-data，仍只恢复 Host 拥有的原 A/B 任务，由 Go 项目锁检查访问。真实后台协议版本 3，实际可执行文件 SHA256 与随包 CLI 一致；[字节及后台核对](codex-alignment-evidence/media-byte-verification-20261104.json)。

B 项目以固定几何素材完成 setup、完整章正文及三页正文/分镜；总览图经 Core import_image（不调用模型）登记，再通过顶层 attachments 导入，原子裁切两项，实际查看总览及裁切预览，记录具体 setting_reviews。完整合图 792×420、12345 字节，SHA256 为 5a2f7c8d47b4b3dd56c3d18880282e1386619bce8fac116f8278595f12ea283d；Go 原文件和 Host 受管 URL 回执一致。A 读取 B 的设定、图片和页面均明确不存在。三页固定图导入和三种无缺图导出见 [固定素材真实 Host 记录](codex-alignment-evidence/media-host-fixed-20261104.json)，不算真实模型生成。

随后收到本轮[最多 1 次 Core 请求的明确授权](codex-alignment-evidence/media-model-authorization-20261104.json)，重新准备第 1 页及查看同一合图，重新取得 openai/gpt-image-2 / openai 的 Guide revision 3。实际 image.edit body 的 image 是上述完整合图受管 URL，prompt 原样使用 generation_prompt，size=1024x768、quality=low。request=1、poll=0，无重提。返回实际 PNG 为 1448×1086、1022629 字节（保持 4:3，不能把请求尺寸当成实际尺寸），source_sha256 与 Core 输出 URL 的 98f17a91…7d971c0 一致。顶层附件导入后 page.revision 1→2，当前第 2 稿，正文/分镜未变；再采用原稿并切回真实稿，revision 2→3→4，历史均保留。已查看受控预览，红蓝几何主体及准确旁白可见；不声明独立 OCR 或模型内部利用证明。见 [实际请求及导入](codex-alignment-evidence/media-model-request-20261104.json)、[真实模型验收](codex-alignment-evidence/media-host-real-20261104.json)。

最后再次导出 Markdown/ZIP/PDF，均 missing_image_count=0；ZIP 752108 字节、PDF 82095 字节。它们包含一页真实模型图和两页固定测试图，不能宣称三页均真实生成。回执仍为 file_generated=true、delivery.status=prepared、user_received=false；本轮未取得控制端下载/手机读取证明。未再生成其它图片，原生引擎通道未验。

## 阻塞与最小解除条件

页面生产读写需要 Host 证明请求发送者所属插件实例、任务、有效页面和当前读写权限，并在切任务/关页/停用/卸载时撤销。当前尚未核验该能力；不采信页面自报 session 或广播。先完成不依赖它的工具、共享 UI、独立测试及构建，随后据真实证据更新此处。

匹配宿主实现源码尚不可取得。需要实际构建对应的可核验公开实现或正式契约及运行时能力证据；本任务不修改或升级 Cindy 应用。

剩余解除条件：

1. 提供实际 Cindy 0.1.121 / sourceCommit=0a7d5a41… 的可核验实现和可信页面任务身份、读写授权、切任务/关页/停用/卸载撤销契约；之后才能连接生产桥并执行原生桌面工作区全操作及生命周期验收。不能用 URL、广播、fixture key 或任务路由字段代替。
2. 原生引擎通道需要真实可用的引擎及其正式参考输入契约、明确的本轮模型预算；本次授权仅覆盖一次 Core 请求且已使用。查看同一临时合图不算原生生图参考已传入。
3. 需在正常 Lumi 开发任务中复验普通 Host 上下文绑定及实际文件列表读取，另有真实 Codex 客户端时执行宿主回归。探针的 Host-owned A/B 上下文不冒充普通任务注入或 Codex 原生客户端。B 的最终 PDF 为 lumi-exports/text-01a1a71e-2fb5-7278-8acc-df83fb60068c.pdf，ZIP 为 lumi-exports/text-01a1a71e-1dee-759d-ba61-00c50d71e35d.zip；在原 B 任务文件列表读取并核对摘要后才记录控制端收到。手机无需也未宣称具备桌面工作区。

最后已释放本次 B 及开发诊断连接，项目、候选和文件保留，所有共享后台保留；未修改宿主应用、发布、推送、合并或执行 Cargo/Rust。Formal/开发安装边界及所有固定素材/真实模型证据持续分列，不能将本状态改为完成。
