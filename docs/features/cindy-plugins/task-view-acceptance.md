# Step 4.5 任务页验收状态

2026-11-02，Step 5 已按用户要求暂时跳过。真实诊断页已打开，页面身份门槛仍未通过，生产四视图未开始；跳过未计为完成或验收通过。以下保留已验证契约与将来恢复条件。

## 宿主与已核对契约

执行宿主仍是 /Applications/Cindy.app，Mac arm64，check_app_update 返回 0.1.121。cindy-source.json 的 sourceCommit 为 0a7d5a419f5cd3e0fbebc9642dde8388376a68aa；精确公开 ref 仍不可取得，沿用随包 Skill 与实际安装包的只读核对，不以公开 tag 或 main 替代。

脚本 `node cindy-plugins/scripts/verify-host-taskview.mjs` 只读取应用二进制包与公开构建元数据，不读取 Host 数据库或聊天。结果和所核对条目的 SHA256 见 [host-contract.json](acceptance/step45-evidence/host-contract.json)。实际 renderer 为任务页传入 `session=<任务标识>` 查询；实际同源协议处理器的公开路由包含 /wake、/app-context、/kv、媒体与模型等。/app-context 委托通用公开上下文，随包契约只声明 region/locale。

代码审计证明所检查的页面路由及上下文形态，不证明所有可能的宿主能力都已枚举。2026-11-02 的真实诊断页另已返回 session 路由、light 主题、wake running、公开上下文键 locale/region，与审计结果相符；当前仍没有找到文档化、可验证的页面任务身份。 2026-11-02 再次 check_app_update 返回 no_installable_update、currentVersion=0.1.121；当前渠道没有可用于本机的更新，未调用安装更新，也未替换宿主应用。

## 页面读取门槛

同一插件页面与逻辑页共用插件 origin/partition 和 BroadcastChannel。页面可以修改 session 查询或广播中的 session 字段；main.js 根据它查出一个已存在绑定，并不能证明请求来自该绑定的任务页。requestId 解决请求匹配，随机 pageId 解决页面实例区分，两者都不是 Host 对页面任务归属的证明。同源广播中交付的 secret 也不能自动解决来源验证。

因此不向当前诊断页开放项目、图片、任意 MCP 或 REST 读取；不把“已经绑定过 A、B 两项目”解释为两个页面可以互相读取。只读并不消除跨项目读错内容的问题。首次 join、重连、聚焦校准和 WS 通知都不能弥补这个身份缺口。

进入生产桥接前，需要实际验证一种 Host 约束的任务页身份机制，例如按实际 WebView 隔离并由宿主验签的页面上下文/消息通道。具体 API 和最低版本必须有正式契约与真实回执；本项目不创建未文档化端点、不关闭 CSP，也不擅自修改 Cindy 应用。

## 最小探针

[product-probe](spike/product-probe/README.md) 是独立作者源码。它静态嵌入未修改的产品媒体 main.js、复用原 Node 与 CLI，使用公开 tasks.create/get 返回的两个独立空目录验证真实媒体落盘。只创建空任务，不发送消息、不启动其他 Agent；不把源码仓库初始化为绘本。该额外任务权限只在临时探针包中，不进入 Lumi 产品包。

诊断 taskView 只报告实际 session 路由、主机注入主题、/wake、/app-context 的公开键，以及同一个页面可以发出其他 session 声明这一通信事实；不读取项目内容。2026-11-02 已真实安装并开页，当前源码任务页发出了 A/B 两个临时任务的 session 声明，回执为 host_attestation_present=false、project_content_read=false。具体结果保存在 [宿主运行记录](acceptance/step6-evidence/host-run-20261102.json) 的 probeReport。它只证明可伪造声明这一事实，没有释放内容，不冒充已执行跨项目数据泄露测试。

两个空任务由 Host 创建，未发送消息、未启动 Agent；A/B 的业务资源隔离已有章节与设定拒绝回执。更新后的任务恢复使用原 task_id 经 tasks.get 归属及标题检查，未重复创建。当前连接已释放，专用 CLI 已按 nonce 停止；项目和探针保留，详见 [Step 6 收尾](acceptance/step6-install.md)。

## 验收矩阵

| 项目 | 结果 |
| --- | --- |
| 源码任务中独立创建两个测试目录，复用产品适配器 | 真实 Host 创建/恢复 A/B 通过；额外上下文来自公开 tasks，非普通产品绑定证明 |
| Forge 结构与随包代码 | 探针已 pack/install/update；审阅与安装重打 21 项内容一致 |
| 页面任务身份与篡改 session 拒绝跨项目 | 未通过前置门槛，业务读取关闭 |
| 真实开页、主题、切换/最大化/关闭 | 实际开页及 light 主题回执通过；其余待验，未以普通浏览器代替 |
| 产品图片取回与页面懒加载 | 待验 |
| WS → main → Query 失效 → REST 重读 | 待验；尚未实现生产页面读桥 |
| 首次/重连/聚焦、持续超过 120 秒、无变化无轮询 | 待验 |
| 手机原生审阅 | 未实现，不宣称支持 |

## Step 5 当前方案

保留 book_outline、chapters/settings/pages list/get、read_image、任务文件导出等对话路径。生产 taskView 维持未开放；不提前交付一个以可伪造 session 为授权凭据的四视图。

身份门槛补齐后，继续复用 web/mcp-app 的展示与 Query 同步，把 Cindy 只读允许列表、受控 JPEG、WS UUID 变更提示和生命周期实现留在独立适配层。页面不直连 loopback，不持有 callId，不写业务状态。worker 持续连接方式需真实验收后确定，不以定时业务查询保活。

## 后续宿主接入条件

Step 5 暂时跳过，将来恢复时仍需先实测[可信页面身份能力](host-task-view-identity-needed.md)。最近一次宿主更新检查无可安装更新；原生桌面重开已由用户跳过。旧诊断退出确认、Codex 真实客户端与共享服务验收已转入[待办](acceptance/todos.md)，其它范围见[剩余验收](acceptance/step6-remaining.md)。历史检查结果保持原有证据范围。
