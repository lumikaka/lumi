# Step 6 原生停用、恢复与卸载验收

## 最新原生拒绝复核

首次安装拒绝和新增权限更新拒绝均已验证，分别记录。首次是用户先卸载再拒绝安装，GHOST_NOT_FOUND符合预期；恢复原基础包后，用户保留开发版直接导入测试包并明确回复“更新已拒绝”。随后原开发build_id/CLI仍可调用，测试构建未生效，正式版原构建、PDF/ZIP及七份旧报告摘要均未变，连接已释放；本轮没有重装或提交安装工具。见 [首次安装拒绝](step6-evidence/permission-manual-rejection-20261102.json)、[更新前基线](step6-evidence/permission-native-update-refusal-baseline-20261102.json) 与 [原生更新拒绝成功证据](step6-evidence/permission-native-update-refusal-success-20261102.json)。原生点击来自用户确认，未伪造MCP安装取消回执。下文保留先前各阶段已验证结果。

2026-11-02。执行电脑为本次MacBook Pro，范围仅个人开发实例 `lumi-dev` / `_root__lumi-dev` / namespace=null。正式 `lumi` 保留；不修改Cindy安装目录、私有数据库或其它应用插件。

## 当前已验证

1. 主人在Cindy「插件 → Lumi（开发版）」详情页关闭启用开关后，实时Host发现返回 `GHOST_ASLEEP`。正式Lumi仍ready、production、原build_id/CLI摘要且server_running=true。两个已交付PDF/ZIP的字节数与SHA256不变。见 [停用检查点](step6-evidence/plugin-disable-20261102.json)。
2. 主人在同一详情页重新启用后，开发工具恢复发现，runtime_status为ready/development，build_id=`29401ca7cbd2cff3f21d5d2f7c3e2237f31990bfd99ac1e005dfd5faa068c1a8`，CLI SHA256=`d26c4636d6b0116520a3a2baef7cf203676ec6a91d3bc06ea4c58f9aef395c47`。正式Lumi仍保持原构建及ready。见 [启用回执](step6-evidence/plugin-reenable-20261102.json)。
3. 原正常作品任务已完成恢复核对，独立报告确认875字章节、三页完整正文/分镜、UUID/顺序/revision逐项一致；原图仍选中，原图与候选2的受控预览均可读取，最终大纲一致，连接已release。本源码任务核对报告与公开摘要，没有重放业务写入。见 [项目与双图恢复证据](step6-evidence/plugin-project-recovery-20261102.json)。
4. 已核对供恢复安装的作者目录、审阅ZIP和当前ZIP：21条目字节及权限一致，CLI指纹与已安装开发构建一致，见 [恢复安装准备](step6-evidence/lifecycle-reinstall-preflight-20261102.json)。此项仅记录安装前产物校验，后续真实卸载与重装另见下文。

## 当前作品与导出基线

- 项目 `01a199d7-323a-7017-a5da-1d93303f487e`，章节 `01a199e6-89e9-7213-a333-924bffa24af8`，875字正文，chapter.revision=1。
- 第1页 `01a19a9f-e071-7ce8-9e8b-175e4009c456`，revision=3，当前原图variant=`01a19ad5-745c-7ba0-99c8-6b751bb67458`，Asset=`01a19ad5-745d-71c1-b6ff-998d15b3f8d6`。
- 编辑候选2variant=`01a19aeb-076a-7412-8cd5-74466c68394e`，Asset=`01a19aeb-076a-7744-a773-c972e1fd8551`，重装后已正常读取确认保留，没有重新导入。
- 第2/3页revision=0且缺图，三页正文与完整分镜的hash见 [页面证据](step6-evidence/normal-task-pages-save-20261102.json)。
- PDF361740字节、SHA256=`67f061080fff48d722bd2ace6c25f936e55608b5594d3952659d7db1d6da3255`，ZIP2864730字节、SHA256=`51bd06fd9a26996505b25b1ac626b55d71926bd7668493cc01a96eb03541dc54`。路径、完整回执及独立内容检查见 [真实导出证据](step6-evidence/normal-task-reference-export-success-20261102.json)。

## 卸载与恢复流程（含重装后作品与双图重读，已通过）

原任务恢复报告已核对通过、已结束/release、队列为空后，主人已只卸载开发插件；实时Host返回GHOST_NOT_FOUND、正式Lumi仍ready、导出文件保留。随后同包Forge重装返回installed，21条目与审阅包一致；正常作品首次重装读取停在MCP拒绝；主人随后在原任务直接发起新请求，另存复验报告，875字章节、三页正文/分镜与两版图片均恢复读取成功，当前原图选择及revision不变，连接已释放。当前工具目录有Forge安装/更新，没有原生停用/启用/卸载接口；这些开关和卸载必须由主人在同一Cindy详情页操作，不通过文件删除或其它应用代替。

1. 只卸载「Lumi（开发版）」对应的上述实例，不选正式Lumi，不删除作品、导出或共享应用数据。随后由Agent用实时发现验证确实不可用，核对正式Lumi仍可诊断、导出文件保留。卸载回执不证明detached后台或全部代理已退出，需要另有实际生命周期证据。
2. 沿用已有开发安装委托，用未再编辑且已审阅的 `build/cindy-dev/lumi-dev/` 走正常Forge安装，保留Host确认卡；不改namespace、不从市场换源、不替换正式包。记录真实installed/updated、instanceId和安装包指纹，不把准备产物当作已恢复。
3. 在原作品任务重新建立正常绑定，读取同一项目、875字正文、三页完整文字、当前原图和编辑候选2；核对上述版本/摘要。只读取既有事实，不初始化、不重放选择、生图、导入或导出。结束释放当前任务连接，共享服务保留。

## 尚未证明的范围

Step 5 及四视图已按用户要求暂时跳过，可信任务页身份门槛仍未通过。旧诊断进程退出确认、Codex 缺客户端及真实共享验收已转入[待办](todos.md)，保留未验证状态。停用/启用、卸载/同包重装以及两次正常作品/双图恢复已通过；首次安装和原生新增权限更新拒绝亦已通过；读取阶段优雅停止与事实恢复已另验通过。正式独立作品及其它取消范围仍未验证。界面显示用户已停用不等于业务调用实际返回相应错误；runtime ready也不等于作品已重开。

## 本次卸载与重装检查点

[卸载证据](step6-evidence/plugin-uninstall-20261102.json)记录GHOST_NOT_FOUND、正式版ready和导出摘要保留。原生只读status在卸载后观察到专用development服务running=true、open_projects=[]；重装后PID=54965及nonce保持一致，没有因插件卸载而停掉共享服务。此结论不等于全部代理已退出。

[重装回执](step6-evidence/plugin-reinstall-20261102.json)为installed/enabled=true、_root__lumi-dev、namespace=null、0.4.0，runtime恢复ready、开发构建未变。[实际安装包比较](step6-evidence/lifecycle-reinstalled-package-20261102.json)核对21条目与原审阅包和作者树一致，并冻结本次重打包ZIP供追溯。向原任务的新恢复读取指令已接受，不重放任何创作修改；原任务已返回《插件重装恢复验证报告.md》：ghost_info原始回执为user rejected MCP tool call、isError=true，无结构化errorCode；未重试、未进入业务读取、未调用runtime_release。任务finished/queued_count=0。见 [本次拒绝证据](step6-evidence/plugin-reinstall-recovery-refusal-20261102.json)。随后主人已在原任务直接发起新请求并另存《插件重装恢复复验报告.md》。本轮正常目录发现、项目重开、875字章节及三页完整正文/分镜比较、两版图片受控预览、最终原图选择与大纲均通过；page.revision保持[3,0,0]，runtime_release返回released=true/shared_retained。父任务独立核对七份旧报告的SHA256，包含首次拒绝报告，均未变化。见 [重装恢复成功证据](step6-evidence/plugin-reinstall-recovery-success-20261102.json)。不把本次成功解释为首次拒绝根因已确定，也不把受控预览读取宣称为重新校验原PNG字节。

## 新增权限更新观察

此前两次Agent更新均返回updated，原包恢复属于历史检查点，不作拒绝证据。随后原生首次安装拒绝与保留旧包的更新拒绝已分别通过；本轮原开发构建和CLI仍可调用、测试构建未生效，正式版及导出/报告摘要不变，连接已释放，没有重装。准备包、历史回执和本轮完整结果见 [新增权限更新记录](step6-permission-update.md)。

## 本轮剩余验收条件与用户跳过项

原正常作品的系统项目锁已实际取得并立即释放，锁检查自身未改变锁文件inode/内容/mtime或完整project.sqlite摘要；恢复读取并释放后再次取得独占锁，见 [项目锁检查](step6-evidence/project-lock-release-20261102.json)。用户明确跳过原生桌面重开，未计为通过；用户当前无可用Codex开发客户端，真实共享项仍未验证。旧诊断PID20644仍为UE，退出未确认。正常任务A在读取基线标记后收到优雅停止请求，目标轮次结束；另行恢复的875字章节/三页完整正文与分镜/双图/大纲均匹配，连接已释放，原共享服务保留且无打开项目。见 [读取阶段停止及恢复证据](step6-evidence/normal-task-read-cancel-recovery-success-20261102.json)，不扩大为在途写入/RPC/Core请求被强制中断。完整记录与宿主前置见 [剩余验收](step6-remaining.md)。
