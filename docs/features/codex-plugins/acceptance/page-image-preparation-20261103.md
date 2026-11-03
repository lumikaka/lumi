# 页面参考合图与宿主导入，2026-11-03

已实施 Go 服务、MCP、插件指令和开发打包，保留当前工作区已有修改。本次没有修改 Web 的 JSX、样式、路由或默认交互，没有运行 Cargo/Rust，没有手工修改或刷新插件缓存。

## 公共实现与调用顺序

原 jobqueue 的合图算法移至 `internal/production/section_premise_composer.go`；Asset Store 源图读取下沉到 `internal/production/section_premise.go`。Web 保留原持久化/恢复逻辑，宿主准备每次调用公共合成器现拼，合图只写系统临时文件，不创建项目资产或合图缓存。

`SectionImageRules`、`EffectiveImageStyle` 和 `RenderSectionImagePrompt` 统一有效模板、角色基础规则、项目画风及旧数据回退、语言指令、参考标签、保存的分镜及用户补充要求。旧持久化快照兼容分支保持原样。

插件执行：选择本页设定 → `prepare_page_image` → 宿主读返回的临时绝对路径 → imagegen 仅 `referenced_image_paths=[同一路径]` → `import_image` 原样带回完整提示词和 preparation。明确 `[]` 返回 none 模式、空路径和空清单，宿主不传参考图。缺图或无效引用直接失败。

来源清单按输入 UUID 顺序给出编号/中文名称、设定 revision、采用图片 variant_uuid/version_no/file_uuid 与实际源字节 SHA-256。准备摘要覆盖页面、设定及当前图片内容/版本、顺序标签、有效规则和画风/语言、项目规格、完整提示词、补充要求及合成器输入版本，临时路径不参与身份。

preparation 使用服务进程 HMAC 签名，绑定项目、页面、revision、选择、补充要求及摘要；不保存准备缓存。服务重启需重新准备。导入预检和最终 Asset 绑定事务都重读事实并重算，旧提示词/摘要、仅换 revision/摘要或修改凭据都会拒绝，不创建页面图片版本。

## 验证结果

- 重构前后本机 1、4、12 项的英文/中文标签 PNG，共六组文件逐字节一致。英文固定字体的 PNG SHA-256 基线写入回归测试；中文沿用系统字体，当前机器实际渲染检查通过。
- 完整提示词的 90 组重构前黄金摘要通过：中英文、正文/封面/封底、快照版本 1–5、0/1/4 个参考，包含自定义语言指令和用户补充要求。有效 project_overall_style 与 Web 保持一致。
- 覆盖单项、多项、12 项上限及 13 项拒绝、中文标签、缺图/源文件缺失、重复/无效/跨项目引用、真实无参考流程、提示词超限与一致性。
- prepare 后切换所选设定当前版本，使用原提示词和 preparation 导入被拒绝，页面图片版本数为零；替换摘要无法绕过签名。页面、画风、语言和规则变化也拒绝。
- 在图片上传完成、最终绑定前切换设定版本、项目画风、语言指令或页面内容，事务内校验拒绝，未写入图片版本。
- MCP 返回真实可读绝对路径和结构化数据，只有 text content；不含合图 Base64、data URI 或 image content，也不泄露内部资产路径/数据库 ID。
- `internal/production`、`internal/textmcp`、`cmd/lumi` 全包 Go 测试通过；jobqueue 全包和最终相关 Web 生图回归通过。前端 454 项测试通过，开发打包 4 项测试通过，Cindy 81 项回归通过，技能验证通过。
- `make codex-dev-build` 通过，生成最终 Go CLI 与开发市场源。服务协议升级到 3，旧后台会要求升级，防止新 relay 派发到不支持准备摘要的后台。Cindy 共享附件入口同步 preparation 透传和 schema，保持同一服务契约。

## 实际夹具与 Web 检查

使用 `scripts/codex-acceptance/page_image_fixture.py` 创建隔离测试项目，四项中文设定均以仓库图标作为明确的测试替身。真实 stdio → CLI → SQLite 返回临时 PNG 路径，agent 通过宿主 `view_image` 实际查看该路径，确认 3 列布局、完整主体、中文编号名称和留白。随后 imagegen 替身只接收该路径，实际 MCP 使用同一完整提示词、摘要与签名导入成功，返回版本 1；`same_prompt=true`、`reference_count=1`。

浏览器打开 CLI 嵌入的正常 Web 单页，确认既有导航、页面列表、编辑区、生成按钮、聊天区和样式正常；导入后未刷新页面即由 WS 提示刷新为 1 稿。点击当前图的大图弹层、画稿历史菜单及项目主页 → 设定卡片导航通过，中文名称和当前版本正常，浏览器错误日志为空。Web 生图 provider 使用 recording 替身验证，未触发真实模型调用。

**真实宿主 imagegen 未调用，未消耗真实生图额度。** 本轮验证了真实路径交付、agent 实际读图、宿主调用参数替身和真实导入闭环；不声称已验证真实 imagegen 的图片接收与生成效果。未构建或启动 Tauri/Rust；Web 检查使用相同嵌入前端的 CLI 页面。

复跑方法见 [夹具说明](../../../../scripts/codex-acceptance/README.md)。原始 manifest、PNG、请求和构建日志保留在系统临时目录，不作为项目素材或源码提交。
