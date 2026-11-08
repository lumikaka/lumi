# Lumi Cindy 本地包安装与恢复

2026-11-02：当前为macOS arm64本地验收包0.4.0，未发布。两身份安装、开发版同版本更新、媒体/候选/PDF/ZIP、停用/启用、卸载/重装保留作品和安装/更新拒绝均通过。正式共享后台协议修复及授权切换后，普通正式作品也已从原 draft 完成一章三页文字，指定开发章节读取被语义拒绝，前后全文与 revision 不变；插件回归81/81。开发作品的PDF仅1页、ZIP含三页全文，另两页缺图；不宣称完整三页插图或手机实际收到。Step 5暂时跳过，旧诊断退出及Codex共享保留[待办](acceptance/todos.md)，桌面重开跳过。实际范围和指纹见[安装验收](acceptance/step6-install.md)与[当前状态](acceptance/step4-step7-status.md)。

## 身份、电脑与模型

正式包 lumi，开发包 lumi-dev。两者共享源码，但 command、包内 CLI、environment 与默认应用数据分别为 lumi / bin/lumi / production / ~/.lumi 和 lumi-dev / bin/lumi_dev / development / ~/.lumi_dev。本轮开发验收包使用专用临时 data-dir，见确切 build-info 和 node/runtime-config.json；不要把它当作日常正式数据配置。

执行电脑是安装插件的 Mac。手机可以控制该 Mac 任务，但本地项目、CLI 和文件都仍在 Mac；桌面 taskView 不等于手机审阅。项目位于 Cindy 当前任务选择的标准 Lumi 目录，包含 project.sqlite、assets、.lumi 和 STORY.md，不放进插件卸载即删除的私有目录。

文本和图像创作由当前 Cindy Agent 及已可用媒体能力执行，不在 Lumi 再配置模型服务商。Core 与引擎原生路径使用各自实际支持的模型和权限；不能根据聊天模型名称推断额度来源或承诺订阅生图。当前产品包已验证Lumi受控预览实际进入Core编辑；引擎原生路径、订阅额度和其它媒体能力不由此推断。

## 构建、审阅与安装

在作者仓库执行 make cindy-build 或 make cindy-dev，后者生成 build/cindy-dev/lumi-dev/。通过 Cindy Forge pack 打包，再运行 verify-package.py 对应步骤和 channel。脚本逐项校验作者内容、CLI 指纹和执行位、身份、environment 与大小，并把确切审阅 ZIP 固定到 build/cindy-reviewed/<id>/<sha256>.cindy，避免后续 install 重打覆盖审阅原件。

`make cindy-dev-install` 目前在开发构建之后**报阻塞并返回非零，不会执行安装**。2026-11-08 核查的 Cindy 0.1.128 随包终端启动器只打开路径，未取得公开可等待并核验的终端安装入口；Forge 安装仍是宿主工具调用。依赖、失败处理、源码核查限制和所缺宿主能力见[开发说明](development.md#终端构建并安装当前被宿主入口阻塞)。不要把构建成功、包已生成或打开安装页当成安装成功。

普通用户使用 Cindy 的本地 .cindy 导入流程；没有市场链接。Agent 只有在收到明确安装委托时才调用 ghost_forge_install，首次安装或新增权限沿用 Host 原生确认。首次安装被拒时保持未安装，更新被拒时保留旧包继续可用；必须有新的用户安装指令才再尝试被拒包；不换插件身份、来源或 namespace 绕过拒绝。

Forge install 接收作者目录并重新打包，不接收先前 ZIP 作为 dir。安装后核对真实 instanceId、namespace、来源和版本，比较审阅包与实际包内容，再重新 ghost_info 发现工具并执行 runtime_status / project_overview。回执若没有这些身份或实际包指纹，就保留未验证项。登录组织不等于授权安装组织副本。

## 更新与旧后台

共享后台必须实际支持 `creation_protocol_version=1` 的项目绑定和只读约束。新包的 `runtime_status` 将包指纹与 `server_compatible` 分开报告：`false` / `upgrade_required` 表示旧后台仍需协调升级；`null` 表示未确认。此时即使 MCP 握手成功，也不继续初始化或 setup，更不通过去掉项目绑定参数绕过。正式首轮验收曾触发这一问题，修复和保留 draft 的恢复方式见[真实记录](acceptance/step6-shared-protocol-fix.md)。

修改 Go、JS、Manual 或 UI 后重新生成、pack、校验和 install；只改仓库代码或只编译/打包不会更新安装副本。本地同来源同版本刷新已在此执行宿主通过，实际效果仍要用构建指纹和调用结果验证；不要求为了日常开发发布 patch。

包更新不会替换已经运行的 detached CLI。先释放当前任务连接、关闭旧页面并协调同一 environment/data-dir 的所有使用者；只有需要切换 Go 后台时才显式 make cindy-dev-stop，再通过新工具连接。该命令读取生成的同一开发配置，不自动停止正式版、Codex 或其他共享客户端。无法安全切换时如实报告新包与旧后台状态。

## 权限与卸载

基础产品包声明 Node、childSpawn、sessionContext 和媒体预览寄存，不旁听聊天，也没有任务历史权限。Node 是可信本机程序，具备账号级本机权限；它不接受模型指定的任意命令、目录、SQL 或外部二进制。临时验收探针额外声明 tasks，仅用于 Host 生成的独立空目录；这项权限不进入产品包。

停用/卸载只作用于回执中的精确实例，保留项目与导出。回收逻辑先撤销绑定和页面唤醒资格，再核对代理/共享 CLI 归属；不能由卸载回执推断 detached server 已退出，不能全局 stop 或删除共享数据。当前开发实例已通过原生停用/启用、卸载/同包重装、正式版持续可用及作品/双图保留验收，见 [生命周期记录](acceptance/step6-lifecycle.md)；旧诊断进程退出与真实Codex共享按用户决定留为[待办](acceptance/todos.md)。

## 故障与可用替代路径

- GHOST_NOT_FOUND：尚未安装或当前不可用，不能宣称工具已生效。安装被拒绝时先保留回执，不自动重试。
- project_locked：由持锁的桌面或 CLI 释放项目后重开；不删锁、不强杀其他实例。
- read_only_context / context_not_local：回到执行 Mac 的可写本地项目任务；不能把 SSH 同名路径当本机项目。
- production_conflict：重读事实状态再判断。图片原件若已接收，在 correction.recovery_file 指向的任务目录保留，不自动改 revision 覆盖。
- result_unknown：先核对当前文本、Asset 和版本；不重放写入或再次付费生图。
- cli_upgrade_required：新包与运行中的共享后台协议不兼容；先协调同一 environment/data-dir 的所有使用者，获准后正常切换该服务。已有 draft 从概况/setup 重读继续，不重新初始化。更新安装包本身不会切换旧后台。
- taskView：当前页面身份门槛尚未通过，生产审阅页未开放。仍可用业务 list/get、read_image 和 PDF/ZIP 导出完成文本/图片审阅，不能把文本路径称为四视图 UI 已完成。
- 文件交付：prepared 仅表示已准备到任务 lumi-exports/；手机实际收到需要读取回执或用户确认，不能把 Mac 路径当下载链接。

最低正式兼容版本尚待证据：当前安装应用是 0.1.121，但其 sourceCommit 不匹配同名公开 tag，精确公开 ref 返回 404。manifest 的现有字段不等于已证明所有正式 0.1.121 构建兼容。不在本地运行 Cargo/Rust，不下载或替换 Cindy 应用来绕过安装/兼容问题。
