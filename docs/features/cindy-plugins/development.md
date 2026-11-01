# Cindy 插件本地开发与双版本隔离

## 状态与实施范围

2026-11-01 补充的开发要求，**尚未实现或完成安装验收**。当前 `cindy-plugins/lumi/` 的 Step 2 包虽然称为“开发包”，插件身份仍为 `lumi`，worker 固定使用 `production + ~/.lumi`；自动化中的临时 HOME/data-dir 只隔离测试，不代表已有可日常使用的独立开发版。

本文的 `make cindy-dev`、`make cindy-dev-stop` 和开发产物目录均为待实现契约，不是当前可执行入口。现有构建命令见[作者 README](../../../cindy-plugins/README.md)。[Step 1](step1-runtime-cli.md) 补齐构建与运行隔离，[Step 6](step6-plugin-install.md) 完成实际安装、同版本更新和并存验收；历史验收记录保持原样。

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

本流程用于开发入口实现之后。日常本地迭代无需发布 GitHub Release、创建 Tag、递增公开 patch 或提交插件市场。执行电脑是实际运行插件的 Mac；手机可作为控制端，安装回执必须核对该 Mac，不能用控制端版本或路径替代。

1. **修改共享源码并生成开发目录。** 运行规划的 `make cindy-dev`。Go 变化必须重新编译随包 `bin/lumi_dev`；JS、Manual、manifest 或 UI 变化也必须重新生成相应文件。允许构建器复用已验证未变的 CLI，但不得漏复制变更或使用旧指纹。检查输出身份为 `lumi-dev`、环境为 development、data-dir 为本次开发目录，并校验执行位和包体限制。
2. **先打包供审阅。** 从生成的开发作者目录调用 `ghost_forge_pack({dir: <开发作者目录绝对路径>})`，记录 `.cindy` 的真实路径、版本、SHA256、条目清单和 CLI 指纹。当前宿主随包打包契约允许同版本重新打包和本地覆盖安装；产品的同版本刷新仍须实测。不能用市场提审的版本规则要求每次本地开发发 patch。
3. **协调正在运行的开发任务。** 更新前结束或取消本次开发实例的在途操作，释放任务连接，关闭/撤销旧页面的订阅和 focus/wake 唤醒资格。如果 Go CLI 改动需要重启，协调同一 environment/data-dir 的全部使用者，再显式执行规划的 `make cindy-dev-stop`。该命令只能调用对应开发 CLI 的 `stop --environment development --data-dir <同一开发目录>`；不能以默认 `lumi stop`、按 PID 强杀或删锁替代，也不能因是开发目录就认为服务由本插件独占。
4. **安装生成的开发包。** 在已有开发安装委托范围内，从刚审阅且未再编辑的同一作者目录调用 `ghost_forge_install({dir: <同一开发作者目录绝对路径>})`。它会重新打包并安装此次生成的包，不能把先前的 `.cindy` 路径误作 `dir`。首次安装默认个人范围；更新沿用准确的开发 `instanceId`、namespace 和来源。出现多个候选时按回执选择开发实例并传 `target_instance_id`，不能选择正式 `lumi`、改 namespace 或额外装副本来绕过冲突。沿用宿主正常权限确认；拒绝时保留未完成状态，不自动重试。
5. **核对真正安装的结果。** 留存 install 的实际包指纹、清单、版本、来源、`instanceId`、namespace 和执行电脑，与审阅包逐项比较；Forge 重新打包后的 SHA 可能变化，不假定字节恒定或仅元数据变化。开发更新只替换对应开发实例，正式插件的启用状态、配置和数据不变。同版本下用源 commit/dirty 与内容指纹区分构建，不能只看 manifest version。
6. **重新建立连接并验证。** 在独立测试绘本目录重新发现开发工具，调用运行诊断和项目概况，核对实际开发 channel、随包 CLI 构建标识、项目 UUID 与新 server 代次。已有会话未刷新时按当前宿主提示重新打开任务/会话，必要时重启宿主；不承诺热加载。诊断仍不向 Agent/页面暴露私有路径、endpoint、PID 或 nonce；实例代次与路径一致性由测试/运行时内部核验。只有实际调用成功后才报告新开发包可用。

**不能省略 pack/install：** Cindy 运行的是安装后的包内副本。仅修改仓库的 JS/Manual、重新编译仓库的 `lumi_dev`，或只执行 `ghost_forge_pack`，都不会更新已安装实例。不得直接替换 Host 安装目录里的文件，也不得让开发包通过仓库绝对路径偷偷绕过随包 CLI。

**安装与后台升级是两个步骤：** 已运行的 detached CLI 不会因包内二进制更新、worker 退出或插件更新而自动替换。无法安全停止共享开发 server 时可以准备新包，并如实报告当前只完成构建、打包还是安装；若已经安装，必须说明“新包已安装，运行中的 server 尚未切换”，不能把包更新视为新 Go 代码已生效。只改 JS/Manual 且 CLI 未变时，无需为更新包而停止共享 server。

## 安装与发布边界

`make cindy-dev` 只生成开发作者目录，不暗中安装或发布；“构建并安装开发版”的明确委托则包含后续 Forge pack/install，沿用已有授权，不重复询问。普通开发不调用公开/组织发布工具，不替换市场中的正式实例或改变其来源。不要把开发身份或 namespace 当作绕过正式版本提交权限的方式。

实施时按真正执行电脑的当前 `cindy-plugin-creator` 核对 Forge 契约和精确源码/运行证据；本文不是新增宿主兼容性证明。现有来源限制见 [开发依据](references.md)。尚未取得同版本更新、双实例并存或新工具发现的真实回执时保留待验，不把 pack 成功当安装成功。

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
