# Lumi Step 0 临时探针

这是可卸载的独立验证插件，不是 Lumi 产品插件。先查询 `list_tools`，再按 category 下钻。

运行时验证使用随包 CLI 和带 owner 标记的 `/private/tmp/lumi-cindy-step0-*`；不从当前用户工作目录初始化作品，不修改其他插件、引擎配置、批准配置，不发布。

通过 `ghost_call({ghost_id:"lumi-step0-probe-20261101",namespace:null,tool:"call_tool",args:{name:"context"}})` 读取宿主上下文（路径只报告是否存在与哈希）；`parallel_projects` 用两个临时项目和两个 data-dir 验证并发；`stop` 仅停止探针实例。这些合成并发不等于两个真实 Cindy 任务验收。

媒体仅通过顶层 `attachments` 显式交接。`receive_media` 从插件自己的公开媒体端点读取原件，并以最多 48 KiB 的 base64 块交给 Node，最多 20 MiB；不读取 Host 缓存路径。`reference` 将同一原件寄存回 Host，返回受管地址，不触发付费生成。

`view` 打开当前任务页面；页面先 wake，再用 BroadcastChannel 请求一次校准，聚焦再校准。无定时业务读取。不声称本页已经实现 TanStack Query 或 Lumi 完整审阅 UI。

`archive_boundary` 仅查询任务能力和尝试读取当前普通任务的边界。探针不默认采集聊天，初版没有 hooks。没有真实 hook 投递或模型调用的项目必须记录未验证。

失败结果包含 schema 与 RULES；宿主拒绝不通过改权限或内部 IPC 绕过。验证结束先 stop，再从 Cindy 插件详情卸载本探针。
