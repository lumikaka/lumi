# Step 6 · 完整 Manual、插件打包与安装验收

## 目标

交付可以在已验收 macOS Cindy 上安装的 `.cindy` 包，让用户通过对话完成完整绘本流程；说明真实的安装、升级、卸载与模型使用条件。

## 前置

Step 1–5 已完成或明确列出仍阻塞的验收项。阅读当前宿主开发 Skill 的 manifest、Manual、packaging 与 compatibility 文档。源码在作者目录，不能从 Host 已安装目录直接修改或打包。

执行前核对 [development.md](development.md) 的正式/开发产物是否已实现；当前独立开发通道仍为待补齐要求，不能把现有 id=lumi 的 Step 2 包当作 lumi-dev 验收。

## 包与能力声明

1. `ghost.json` 使用 v3，声明真实工具、command、whenToUse、Manual、Node 和 taskView。设置 `minCindyVersion` 为已确认支持所需能力且整包验收通过的正式版本；不得由当前 main 或单一方法存在推断低版本兼容。
2. 两段式工具说明准确列出可写项目/素材/导出等副作用。`whenToUse` 只写召回场景；复杂流程只放 Manual；基础包不声明可选聊天 hook、任意任务历史或无关网络能力。
3. Node 依赖制作阶段打包。Go CLI 与静态资源随包交付，打包前设执行位；验证 `.cindy` ≤128 MiB、解压后 ≤256 MiB、文件数 ≤2048 等当前限制，以执行宿主契约为准。不要到首次使用时下载安装器或运行包管理器。
4. 对已宣布支持的每个架构制作并验证适合的包。若市场分发不支持所需架构选择，先验证包含多架构二进制的包及限制；不能假设存在未文档化的市场平台筛选字段。
5. CLI 版本与插件协议保持兼容。新插件遇到旧共享 CLI 时先检测；不静默停止别的任务服务。更新过程保留用户项目、模型偏好与身份，仍需重查任务绑定。
6. 正式包 id/command 为 `lumi`，开发包为 `lumi-dev`，均由共享源码生成。分别核对随包 `bin/lumi` / `bin/lumi_dev`、production / development 与默认 `~/.lumi` / `~/.lumi_dev`；开发包不得引用仓库外置二进制或依赖首次运行下载。

## 创作 Manual

覆盖首次创建/已有项目、完整创作顺序、审阅节点、`render_prompt` 与版本保护、参考图和比例、媒体立即导入、候选选择、导出、额度不足/取消后恢复、桌面项目锁切换。

明确当前 Agent 是执行者；串行路径始终可用。若将来增加子 Agent，必须有适用引擎的实测、明确任务和写入归属，不能把多 Agent 变为首版安装前提。不得将后台 `agent.errand` 或 `tasks` 当作免费/同订阅的通用推理入口。

Manual 使用 `manual.items` 下的普通 `MANUAL.md` 与引用 Markdown，按公开契约打包；不向用户 `~/.codex`、Claude 配置或全局 skills 目录安装一份重复流程。

## 安装、升级与卸载

- 先 `ghost_forge_pack` 得到可审阅的确切包。用户明确要求安装/更新或执行包含安装的本步 Goal 时，用 `ghost_forge_install`；沿用 Host 的首次安装/新增权限确认，不能直接改安装目录。
- 记录实际插件 instanceId、namespace、来源、版本与执行电脑，不因为登录了组织就另装一份组织插件。
- 普通用户入口为 Cindy 插件市场或本地 `.cindy` 导入；市场尚未发布时只提供本地包安装方式，不能生成不存在的市场链接。
- “一句提示词安装”须说明真实必要条件与批准，不承诺零批准/热加载。只有当前会话实际发现并调用工具成功后才说可用。
- 卸载停止本插件代理/订阅，保留用户项目和导出文件；不得顺手卸载 Codex 插件、停止无关服务或删除共享 Lumi 数据。
- 当前步骤不包含公开/组织发布。发布脚本和 CI 准备可以交付；上传、提审与发布须有独立明确委托，按真实渠道流程执行。

## 开发包重新打包与安装

日常开发不要求发布 patch；完整流程和状态以 [development.md](development.md) 为准：

1. 修改共享源码后执行待实现的 `make cindy-dev`，生成完整开发作者目录 `build/cindy-dev/lumi-dev/`。Go 改动重新编译随包 CLI；JS、Manual、manifest 和 UI 改动也必须进入新目录。
2. 对该目录执行 Forge pack，保存审阅包、版本、条目清单及包/CLI 指纹；同版本本地刷新须按照执行宿主当前契约验证。
3. 释放正在使用开发实例的任务及旧页面；Go 变化需要重启时，先协调同一开发 environment/data-dir 的使用者，再显式执行 `make cindy-dev-stop`。不得自动停止正式版、Codex 或其他任务使用的服务。
4. 有开发安装委托时，对同一未再编辑的作者目录执行 Forge install；记录重新打包的实际包和回执，定位正确 `lumi-dev` instanceId/namespace/source，必要时按回执传 `target_instance_id`。不把正式市场 `lumi` 换源为开发安装，也不擅自新建其他 namespace 副本。
5. 比较审阅与实际安装内容，重新发现开发工具并调用诊断/项目概况；核对 channel、构建标识、项目和后台代次。旧会话未刷新时按宿主提示重新打开任务/会话，不宣称热加载。手机控制时，回执仍须指向实际执行的 Mac。

仅重新编译仓库 CLI、修改源码文件或 pack 都不会更新 Cindy 的安装副本，必须 install；仅安装新包也不会替换已经运行的 detached CLI。后台未切换时明确报告剩余步骤。仅改 JS/Manual 且 CLI 未变，不为刷新包而停止共享 server。

## 交付

- 版本化 `.cindy` 包、构建脚本、完整 Manual，必要的 Go CLI CI 产出；原生签名/公证只按发布流程在 CI 完成。
- `docs/features/cindy-plugins/install.md`：安装/更新/卸载、版本/架构、模型条件、项目位置、恢复和故障排查。
- `development.md` 与作者 README：记录已经实现的开发命令、生成目录、同版本 pack/install、明确的后台停止/重连步骤；不能提前把计划命令写成已可用。
- README 的简短入口；官网 `site/` 文档可增加链接，但不把 Hugo 接入应用打包链路。
- `docs/features/cindy-plugins/acceptance/step6-install.md`：自动化、真实安装、真实 Agent/媒体与未通过项分别记录。

## 禁止

不从已安装插件目录开发或打包，不在首次使用时下载可执行代码，不擅自改安装来源/namespace，不把安装委托扩展为市场发布，不删除用户项目或无关插件，不在本地运行 Cargo/Rust。

## 完成标准

1. Forge 验证包结构、依赖、执行位、manifest、Manual；干净测试环境不要求另装 Node/Go/Lumi 桌面版即可启动。
2. 首次安装、同来源更新、拒绝新增权限、停用/启用、卸载保留项目均符合回执；只修改本插件。
3. 真实 Cindy 从空目录自主完成 setup、故事、设定、至少 3 页、真实生图、导入、四视图审阅、一次修改/候选选择、PDF/ZIP 导出。mock 数据不能替代真实生图项。
4. 中途取消后从事实状态恢复；两个任务/项目不串写；释放 CLI 项目锁后通过桌面版重开核对正文、素材和历史。
5. 检查相关 Node/Go/前端改动，原 Codex 链路保留；没有本地 Cargo/Rust。若手机交付未经验证，明确限制，不用 Mac 文件路径冒充可下载附件。
6. 逐项完成 development.md 验收矩阵：两版并存、开发首次安装及同版本刷新、Go 旧后台切换、准确安装来源/实例、共享开发 server 归属、开发卸载与正式版持续可用。记录真实回执与指纹，不沿用 Step 0 的递增版本更新作为同版本刷新证明。
