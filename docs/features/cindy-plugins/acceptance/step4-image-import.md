# Step 4 图片交接验收

2026-11-02 当前结论：**Step 4已验Core路径通过。**真实生成→授权附件→Asset Store→重读预览→真实PDF导出，以及Lumi受控参考图实际参与一次Core编辑均有成功回执；候选选择与ZIP也已通过。PDF仅1页，第2/3页缺图；不扩大为全三页插图、引擎原生/订阅额度或手机接收验收。当前范围见 [汇总](step4-step7-status.md)，以下旧日期指纹和未安装描述保留为当时记录。

## 参考编辑、候选选择及真实导出已通过

原作品任务使用本次Lumi read_image返回的768×576受控JPEG作为Core image.edit的实际image输入，模型openai/gpt-image-2 / openai；只提交1次，立即导入候选2（2437133字节、1448×1086，Asset 01a19aeb-076a-7744-a773-c972e1fd8551）。以最新page.revision执行一次select_image切回原图，最终revision=3、图片版本1；原图和编辑版均保留，正文/分镜未变。执行任务观察奶奶露脸范围已收窄、背景灯已移除，仍不宣称独立OCR或逐像素不变。

真实PDF/ZIP各导出1次，父任务独立完整读取交付副本核对SHA256和大小：PDF361740字节、1页、内嵌1262×947压缩JPEG；ZIP2864730字节、CRC有效，包含book.md/book.json及原图PNG。ZIP章节全文875字及三页正文/分镜的hash与原保存记录一致，图片hash与首次生成导入一致，证明导出使用切回后的原图。两个回执missing_image_count=2，手机接收未确认。见 [完整成功证据](step6-evidence/normal-task-reference-export-success-20261102.json)。

本次独立检查输出：`PASS: actual Core edit reference URL, candidate import/select revisions 1->2->3, PDF/ZIP SHA256, PDF page count, ZIP CRC, all saved page/chapter text hashes and selected original PNG`。既有异常边界自动化与本节真实模型/文件检查分别记账，不重新提交付费生成或导出以充当测试。

## 正常作品任务真实图片复验

用户提供独立《图片生成导入复验报告.md》，原失败报告未覆盖。父任务已核对其中的完整导入回执、受控图片读取、页面当前选择、前后比较和大纲，归一化记录见 [成功证据](step6-evidence/normal-task-first-image-success-20261102.json)。实际 Core request=1、receive_media=1、poll=0，模型 `openai/gpt-image-2` / `openai`；没有参考图片输入或追加生成。

PNG 原件2861885字节、1448×1086、精确4:3，keep导入；source_sha256=`be9e712ebc226eca2ded54b64a1608a170fb8d194345144118ea01cf21998f16`，与Core受管地址摘要一致。Asset `01a19ad5-745d-71c1-b6ff-998d15b3f8d6`、variant `01a19ad5-745c-7ba0-99c8-6b751bb67458` 在导入、读取与当前选择中一致，page.revision 0 → 1、图片version=1；原页面文字与分镜不变。768×576受控JPEG预览已寄存，但未据此调用图片编辑。执行任务报告画面中奶奶露出更多、门内多一盏灯；不把工具链成功当作逐项画面审稿通过。

新开发构建 `29401ca7…` 的提示词三个渲染调用已在正常任务成功执行。第2/3页仍无图，章节revision=1；连接已release、共享服务保留。本次只核对报告和公开任务摘要，没有独立读取原图二进制、重新生成、导入或导出。

## 历史自动化与安装阶段

2026-11-02 续验：开发版已首次安装并同版本更新，真实宿主发现的 worker 子进程代理清理缺陷已修复，最新完整自动化为 74/74，通过记录见 [Step 6](step6-install.md)。真实 Agent 已保存 setup、STORY.md 和三个设定；章节正文写入收到拒绝后未重试，未创建页面或调用付费生图。以下 2026-11-01 的包指纹及“未安装”描述仅属于当时记录，不代表当前状态；真实图片主线仍未通过。

## 代码与边界

- 增加顶层 receive_media，保留 list_tools/call_tool 与明确允许列表；media 类别公开 image_target、read_image 和接收工具说明。
- 授权原件下载、48 KiB 块、两条传输/40 MiB 总预留、长度/摘要/MIME/魔数校验；完整解码与版本提交复用既有 Go 服务。
- 受控 JPEG 通过 Host images 交给当前 Agent，寄存预览后提供 Core 参考 URL；未改共享 Codex MCP ImageContent。
- 冲突后原图保存在任务文件目录，回执附恢复路径和摘要；不覆盖当前版本，不自动重试生成。
- 正式/开发来自同一作者源码，开发目录为 build/cindy-dev/lumi-dev，二进制 lumi_dev、环境 development；诊断包含 channel/environment/build_id/cli_sha256。

## 自动化

完整命令和原始输出保存在 [step4-evidence/automated.json](step4-evidence/automated.json)。测试使用临时 HOME、作品和 data-dir；loopback 需要沙箱外运行许可，初次受限环境的 EPERM 不记为产品失败。测试只关闭核对过 nonce 的本次隔离服务，删除自己的临时作品；不操作用户默认服务。

最终输出：`tests 71 / pass 71 / fail 0 / skipped 0`。包含新增的媒体暂存阶段关闭等待测试，正常 shutdown 会等文件清理完成。

media-integration.test.cjs 使用 Step 0 的已知 PNG 作为固定输入（826433 字节，SHA256 f09a1d3de26dbe010ed12cf9ff6594ea4b064f927c260ca41670e0b055b9652c），通过真实 CLI 导入、HTTP 完整读回并比较 hash；另导入 JPEG/WebP、检查受控预览、keep/crop/pad、两版历史与选择、版本冲突及完整恢复原图、损坏图、跨项目/任务/调用拒绝、取消与 PDF/ZIP 文件头。它不证明本轮真实宿主或模型生成成功。

Go 回归命令与输出见 step4-evidence/go-tests.txt，覆盖共享 production/files/textmcp、CLI 与 schema。没有运行 Cargo/Rust。

最终输出：production、files、textmcp、project、promptcatalog、cmd/lumi 全部 ok；schema 可编译（无测试文件），exit_code=0。

## Forge 与宿主

版本 0.4.0；正式身份 lumi、开发 lumi-dev，macOS arm64。确切包内容、CLI 执行位、SHA256、build_id 与清单由 verify-package.py 校验并记录 package.json / package-development.json；打包不等于安装。

两个包均通过 21 个条目的逐文件核对与体积限制，CLI mode=755。正式包 SHA256 为 `66e72336f0d49c8d140095fa001a043ba35bb4d4187b4d546baf4a1fa9bbda71`，开发包为 `5a49fd173ce44c3d07cc0f050ee7dc5fa4fef5ff5add83a41ac24731feaa629b`。开发构建本轮固定使用专用 `/private/tmp/lumi-cindy-step4-dev-data`，不是默认用户数据目录；后续安装需核对同一配置。

本轮 Host check_app_update 返回 currentVersion=0.1.121；本机 cindy-source.json 仍为 0a7d5a419f5cd3e0fbebc9642dde8388376a68aa，匹配的公开源码查询继续返回 404。依当前随包 Skill 与运行时验收推进；官方 v0.1.121 的历史源码不冒充该构建。minCindyVersion 尚不是正式最低兼容版本证明。

上述2026-11-01阶段尚未安装或发布，真实媒体也未开展；不将合成bridge、固定输出或历史探针改写为产品通过。后续产品安装、真实生成/导入/参考编辑/预览及PDF已通过，见本文开头。任务页持续连接仍属Step 4.5/5的待验范围。
