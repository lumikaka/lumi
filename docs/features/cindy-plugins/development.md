# Cindy 插件本地开发与双版本隔离

## 状态与实施范围

2026-11-02 已实现构建与运行配置隔离；开发版0.4.0首次安装、同来源同版本刷新及新构建诊断、两身份并存、正常开发作品媒体/候选/PDF/ZIP和旧专用后台切换均已通过。停用/启用、卸载/重装及作品/双图恢复、首次安装和新增权限更新拒绝亦通过，见[生命周期证据](acceptance/step6-lifecycle.md)。正式后台协议修复与授权切换后，普通正式独立作品从已有 draft 完成一章三页，指定开发章节读取被语义拒绝，前后完整数据不变，见[恢复成功证据](acceptance/step6-evidence/normal-task-production-recovery-success-20261102.json)。真实Codex共享与旧诊断退出按用户决定留为[待办](acceptance/todos.md)，历史Step 2/3包不追记为开发通道证明。当前范围见[Step 4–7状态](acceptance/step4-step7-status.md)。

`make cindy-build` 在作者目录生成正式包内容；`make cindy-dev` 从同一源码生成完整开发作者目录 `build/cindy-dev/lumi-dev/`。`make cindy-dev-stop` 读取该目录内受控 runtime-config.json，并显式执行对应 development/data-dir 的 stop，不依赖当前环境变量。构建、停止都不会安装或发布；先协调共享该服务的使用者，再手动执行停止。

```sh
make cindy-build
make cindy-dev
# 需要独立验收服务时：
make cindy-dev CINDY_DEV_DATA_DIR=/private/tmp/lumi-cindy-acceptance-data
# 仅在协调同一 development 服务的全部使用者之后：
make cindy-dev-stop
```

开发 CLI 用 `-X main.defaultEnvironment=development` 编译，worker 再对 mcp/status 显式传 environment/data-dir。自定义 data-dir 固定进入生成包；停止脚本读取同一配置，不能因 make 命令的临时变量变化停错服务。build-info 同时记录 CLI 指纹与全部随包源文件/配置的 build_id；同版本源码变化可以区分，实际安装是否已刷新仍须真实工具回执。

## 终端构建并安装：当前被宿主入口阻塞

```sh
make cindy-dev-install
# 自定义开发数据目录仍通过相同构建参数传入：
make cindy-dev-install CINDY_DEV_DATA_DIR=/private/tmp/lumi-cindy-acceptance-data
```

**这个目标目前不能自动安装，预期返回非零。** 它先调用现有 `make cindy-dev`，生成本次 `build/cindy-dev/lumi-dev/`；构建失败时输出“开发版构建失败，未执行安装”并停止。构建成功后输出“自动安装被阻塞”及缺少的官方入口，仍返回失败，不会打开安装页面、修改安装目录或停止共享后台。`make cindy-dev` 保持只构建且成功时返回零的既有行为。

构建依赖 macOS arm64、Go 1.27.3、Git、make、Node（满足仓库 Vite 8 的要求），以及已安装的 `web/` 依赖。先执行 `pnpm --dir web install --frozen-lockfile`，Go 依赖须已下载或可从当前网络获取。构建器生成契约、构建 Cindy 独立工作区并编译 Go CLI，不运行 Cargo/Rust。修复依赖、构建错误或无效的绝对 `CINDY_DEV_DATA_DIR` 后可重试；构建成功不能解除安装入口阻塞。

### 2026-11-08 入口核查

- 执行 Mac 的 `check_app_update` 只读回执与 `/Applications/Cindy.app/Contents/Info.plist` 均为 **0.1.128**；`cindy-source.json` 记录实际构建 commit `742af4c3f3f477eac7dd796bfbac794390548253`。
- 已读取该应用随包的 `Resources/cli/cindy`：只接受零个或一个路径参数，最后执行 `open -a`；没有安装子命令、等待安装事务完成或返回安装身份/指纹的协议。官方 [v0.1.128 的终端启动器源码](https://github.com/makecindy/cindy/blob/9d62b213c960c63daa46996de8a606965f2f2299/apps/desktop/resources/cli/cindy) 也仅实现打开路径。安装 PATH 启动器不能补齐此能力；`open` 成功只证明请求已交给应用。
- 当前宿主的能力发现仅返回插件工具网关；随包 `cindy-plugin-creator` 的 `plugin-packaging.md` §7.2 与实时 Forge schema 提供 **Agent 宿主工具** `ghost_forge_pack` / `ghost_forge_install`，没有声明可供此 Make 目标使用的终端调用协议。不能把工具名当 shell 命令，也不能借私有 IPC/HTTP、宿主数据库或直接覆盖安装目录实现安装。
- 官方 tag `v0.1.128` 指向 `9d62b213c960c63daa46996de8a606965f2f2299`，与本机构建不同；实际构建 commit 的官方 GitHub tree 查询返回 HTTP 422，无法取得对应公开源码。因此本结论限于已核实的随包入口、手册与运行时发现，不宣称所有 Cindy 版本都不存在安装 CLI，也不以 tag 或 main 代替本机构建兼容性证据。

### Cindy 侧还需提供的公开能力

需要由 Cindy 提供并文档化一个**可从终端调用、可等待最终结果**的官方 Forge 打包/安装入口，以及安装后的只读核验入口。当前仓库没有这一入口的已核实调用方式，所以没有预设 CLI 名称、参数或自定义安装器环境变量。接入时必须满足：

1. 接收作者目录 `build/cindy-dev/lumi-dev/`，复用宿主打包校验、正常授权及安装事务；拒绝、超时、打包/安装失败均返回非零和清楚的中文错误，不自动重试被拒操作。
2. 能定位并更新既有 `lumi-dev` 的精确 `instanceId`、namespace 和来源，保留其配置与数据。多候选需明确选择，不能悄悄新装副本、切换来源或更新正式 `lumi`。当前 Forge 缺省是 `namespace:"local"`；已有 root/企业开发实例必须沿用其身份，不能靠缺省值迁移。
3. 最终回执区分 `installed` / `updated`，包含执行电脑、实例、namespace、来源、插件 id/command/version、实际包 SHA256；安装后从宿主管理的实际副本核对 `build-info.json` 的 `build_id` 与 `cli_sha256`、development 配置，必要时重算包内内容摘要。不能只拿仓库构建信息或版本号当作安装证据。身份、内容不匹配或无法核验都不得报告成功；若安装事务已完成而核验失败，应明确“已安装但核验失败”，避免盲目重放。
4. 单独报告后台状态。包更新成功并不代表运行中的 detached Go 后台已切换；无法确认时保留未知/尚未验证，不自动 stop。只有协调共享同一 environment/data-dir 的全部客户端后，才另行手动停止并重连验证新后台。

入口补齐前，继续使用下节的 **Cindy 内官方 Forge 工作流**。这不是终端目标的成功回退：打包、打开页面、人工安装和后台切换分别报告；只有真实安装回执和新构建核验都通过，才能报告开发插件已更新。检查此目标的错误分支可运行 `node --test cindy-plugins/tests/dev-install.test.cjs`，测试使用临时目录和构建替身，不操作 Cindy 安装或后台。

## 一份源码，两个入口

| 项目 | 正式版 | 开发版 |
| --- | --- | --- |
| Cindy 插件 id / command | `lumi` / `lumi` | `lumi-dev` / `lumi-dev` |
| 显示名称 | Lumi | Lumi（开发版） |
| 随包 CLI | `bin/lumi` | `bin/lumi_dev` |
| CLI environment | `production` | `development` |
| 默认应用数据 | `~/.lumi` | `~/.lumi_dev` |
| 安装渠道 | 正式包或市场安装 | 本地 Forge 开发安装，不要求公开发布 |

- 共享作者源码仍位于 `cindy-plugins/lumi/`。main.js、Node worker、Manual、UI 和业务 CLI 各维护一份；构建时生成身份、显示名称、命令及运行配置差异，不手工复制出第二套源码。
- `make cindy-dev` 必须从当前源码构建本机开发 CLI、生成完整开发作者目录 `build/cindy-dev/lumi-dev/`，包含 manifest、Node、Manual、图标、必要静态资源与 CLI；记录源 commit/dirty、channel、架构及 CLI SHA256。生成目录位于仓库构建区，不能是 Cindy 管理的已安装目录。
- 开发 CLI 默认环境为 `development`；worker 仍须对 `mcp`、`status`、`serve` 等实际调用显式传 `--environment development` 与规范化的开发 `--data-dir`，不能只靠改文件名、`APP_ENV` 或 GUI 的 PATH。正式默认值不变。
- 自定义开发 data-dir 只来自受控构建配置，生成到开发包；构建、诊断、停止和验收使用同一值，不把环境、可执行文件或任意 data-dir 开放为 Agent/页面参数。正式与开发的插件配置、KV 和实例记录分别保存，不迁移正式数据到开发实例。
- 使用独立测试绘本目录；应用数据隔离不会复制或隔离同一个 `project.sqlite`。Cindy 开发版与 Codex 开发版默认都使用 `~/.lumi_dev`，可能共享同一个 development server；这只是正式/开发隔离，不是两个宿主之间的隔离。需要独立服务的验收使用专用临时 data-dir。

## 修改源码后，重新打包并安装开发包

完整合图的内部传输和原生固定素材验收见 [Codex 对齐验收](acceptance/codex-alignment.md)。验收探针默认保留历史 data-dir；本次可用 `node docs/features/cindy-plugins/spike/product-probe/build.mjs --data-dir /private/tmp/lumi-cindy-alignment-host-data` 构建隔离服务配置，避免为更新包停止旧共享后台。该参数仅接受明确的 /private/tmp/lumi-cindy-* 目录，不开放给普通模型工具；同一个作品仍受原项目锁约束，不能以切 data-dir 绕过占用。

以下构建入口已经实现，安装动作仍需走实际 Forge 回执。日常本地迭代无需发布 GitHub Release、创建 Tag、递增公开 patch 或提交插件市场。执行电脑是实际运行插件的 Mac；手机可作为控制端，安装回执必须核对该 Mac，不能用控制端版本或路径替代。

1. **修改共享源码并生成开发目录。** 运行 `make cindy-dev`。Go 变化必须重新编译随包 `bin/lumi_dev`；JS、Manual、manifest 或 UI 变化也必须重新生成相应文件。允许构建器复用已验证未变的 CLI，但不得漏复制变更或使用旧指纹。检查输出身份为 `lumi-dev`、环境为 development、data-dir 为本次开发目录，并校验执行位和包体限制。
2. **先打包供审阅。** 从生成的开发作者目录调用 `ghost_forge_pack({dir: <开发作者目录绝对路径>})`，记录 `.cindy` 的真实路径、版本、SHA256、条目清单和 CLI 指纹。当前宿主随包打包契约允许同版本重新打包和本地覆盖安装；产品的同版本刷新仍须实测。不能用市场提审的版本规则要求每次本地开发发 patch。
3. **协调正在运行的开发任务。** 更新前结束或取消本次开发实例的在途操作，释放任务连接，关闭/撤销旧页面的订阅和 focus/wake 唤醒资格。如果 Go CLI 改动需要重启，协调同一 environment/data-dir 的全部使用者，再显式执行 `make cindy-dev-stop`。该命令只能调用对应开发 CLI 的 `stop --environment development --data-dir <同一开发目录>`；不能以默认 `lumi stop`、按 PID 强杀或删锁替代，也不能因是开发目录就认为服务由本插件独占。
4. **安装生成的开发包。** 在已有开发安装委托范围内，从刚审阅且未再编辑的同一作者目录调用 `ghost_forge_install`。它会重新打包并安装此次生成的包，不能把先前的 `.cindy` 路径误作 `dir`。当前宿主首次缺省安装为本地开发副本 `namespace:"local"`；更新已有开发实例时传 `{dir: <同一开发作者目录绝对路径>, target_instance_id: <已确认的开发实例>}`，沿用准确的 `instanceId`、namespace 和来源，尤其不能把既有 root 实例隐式换成 local。出现多个候选时按回执选择开发实例，不能选择正式 `lumi`、改 namespace 或额外装副本来绕过冲突。沿用宿主正常权限确认；拒绝时保留未完成状态，不自动重试。
5. **核对真正安装的结果。** 留存 install 的实际包指纹、清单、版本、来源、`instanceId`、namespace 和执行电脑，与审阅包逐项比较；Forge 重新打包后的 SHA 可能变化，不假定字节恒定或仅元数据变化。开发更新只替换对应开发实例，正式插件的启用状态、配置和数据不变。同版本下用源 commit/dirty 与内容指纹区分构建，不能只看 manifest version。
6. **重新建立连接并验证。** 在独立测试绘本目录重新发现开发工具，调用运行诊断和项目概况，核对实际开发 channel、随包 CLI 构建标识、项目 UUID 与新 server 代次。已有会话未刷新时按当前宿主提示重新打开任务/会话，必要时重启宿主；不承诺热加载。诊断仍不向 Agent/页面暴露私有路径、endpoint、PID 或 nonce；实例代次与路径一致性由测试/运行时内部核验。只有实际调用成功后才报告新开发包可用。

**不能省略 pack/install：** Cindy 运行的是安装后的包内副本。仅修改仓库的 JS/Manual、重新编译仓库的 `lumi_dev`，或只执行 `ghost_forge_pack`，都不会更新已安装实例。不得直接替换 Host 安装目录里的文件，也不得让开发包通过仓库绝对路径偷偷绕过随包 CLI。

**安装与后台升级是两个步骤：** 已运行的 detached CLI 不会因包内二进制更新、worker 退出或插件更新而自动替换。无法安全停止共享开发 server 时可以准备新包，并如实报告当前只完成构建、打包还是安装；若已经安装，必须说明“新包已安装，运行中的 server 尚未切换”，不能把包更新视为新 Go 代码已生效。只改 JS/Manual 且 CLI 未变时，无需为更新包而停止共享 server。

## 安装与发布边界

`make cindy-dev` 只生成开发作者目录，不暗中安装或发布；“构建并安装开发版”的明确委托则包含后续 Forge pack/install，沿用已有授权，不重复询问。普通开发不调用公开/组织发布工具，不替换市场中的正式实例或改变其来源。不要把开发身份或 namespace 当作绕过正式版本提交权限的方式。

实施时按真正执行电脑的当前 `cindy-plugin-creator` 核对 Forge 契约和精确源码/运行证据；本文不是新增宿主兼容性证明。现有来源限制见 [开发依据](references.md)。同版本更新、工具发现与诊断回执见 [Step 6 验收](acceptance/step6-install.md)；双实例并存等未测项保留待验，不把 pack 成功当安装成功。

## 必须补齐的验收

| 场景 | 通过条件 |
| --- | --- |
| 正式与开发同时安装 | id/command、安装实例和默认 data-dir 分开；各自调用正确环境与独立测试作品 |
| 开发首次安装及同版本刷新 | 修改 JS/Manual 后保持版本号，重新构建、pack/install；实际发现/调用得到新内容，CLI 未变时无多余 server 停止 |
| Go 改动与旧后台存活 | 只更新包不会被报告为后台已升级；协调后显式停止目标开发 server，重连验证新构建与新实例代次 |
| 来源与目标实例 | 更新定位正确开发 instanceId/namespace/source；多候选不误选正式实例，不隐式切换来源 |
| 同环境共享服务 | Cindy 与 Codex 开发客户端可共同使用；释放、更新或卸载 Cindy 开发插件不自动停止另一客户端的服务 |
| 正式版不受影响 | 开发构建、安装、刷新、停止和卸载后，正式包指纹/配置/启用状态不变，正式客户端仍可调用 |
| 卸载与恢复 | 只卸载对应开发插件，保留测试作品；先阻止旧页面重连，再核查代理及共享 CLI 状态，不用卸载回执代替进程验证 |

自动化使用临时 HOME、应用数据与作品目录；真实宿主验收记录执行电脑、宿主版本/源码依据、插件身份与来源、两次包指纹、CLI 指纹、调用结果和未验证项。结果分别追加到 Step 1/6 验收记录，不能修改历史证据使其看似已覆盖这些新增要求。
# Codex 对齐开发增量（2026-11-04）

本次进行中的目标与验收见 [对齐基线](codex-alignment.md) 和 [验收记录](acceptance/codex-alignment.md)。历史 Step 5 四视图范围不再是目标上限；可编辑共享工作区的原生宿主接入仍未完成，当前生产页面桥关闭。

独立工作区入口/构建、内部授权适配接口和共享页面装配已实现，使用方式与失败关闭边界见 [作品工作区](workspace.md)。Cindy 构建会生成并携带 workspace.html；该文件不提交，当前不在 manifest 声明 taskView。`node scripts/cindy-acceptance/workspace.mjs` 可启动模拟页面授权、真实 CLI 的独立浏览器夹具。

`node cindy-plugins/scripts/sync-contracts.mjs` 从实际 Go `textmcp.Specs()` 生成业务快照、manifest receive_media 和 main.js 同一附件 schema；`--check` 只检查漂移，失败时不会修复文件。构建先生成再复制开发目录，不会让开发包继承旧快照。`node cindy-plugins/scripts/verify.mjs codex-alignment` 保存本次独立证据，避免覆盖历史 Step 记录。

设定板导入只走 receive_media(slot=setting_board) 的授权附件。普通 setting_boards 仅 list/get；apply_setting_breakdown 明确开放并沿用 Go 原子事务。为保持 Go 既有的路径参与幂等输入语义，授权总览原件先保存到当前任务 lumi-media-recovery/ 的内容摘要路径，重启后同键同参数仍使用同一来源；不改 Codex 默认契约。不会向用户项目写入 Codex 信任或引擎配置。
