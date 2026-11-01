# Step 0 可审阅探针

独立插件 ID：`lumi-step0-probe-20261101`，初版 `0.0.1`，最终 `0.0.5`，个人实例 `_root__lumi-step0-probe-20261101` / namespace=null；已正常卸载，不发布。

- 源码：`probe/ghost.json`、`main.js`、`node/worker.cjs`、`task.*`、`manual/probe/MANUAL.md`。
- 生成物：`probe/bin/lumi`、`probe/probe-config.json` 和 `probe/*.cindy` 被忽略，不将 38 MiB 二进制纳入 Git。
- 权限：按需随包 Node、sessionContext、taskView、media.deposit、agent.tasks（查询边界及创建两个隔离测试任务）。0.0.5 新增两个 hooks 与 turn/session/activity，逻辑页改为 resident；默认不采集，只能显式启用两个已登记测试任务，所有消息 allow，仅存字段及文本哈希。无网络/凭证声明，无外发能力。
- Native Node 具有当前 OS 用户权限；探针自身仅接受固定 a/b 槽位，逐次检查临时目录 owner 和真实路径，不接受模型给出的 root_path。
- 初版安装通过正常 Forge install 确认；只测试和移除此探针，不修改其他插件或批准配置。

## 复现

1. 阅读根 AGENTS.md 与本目录上级 `step0-spike.md`，重新确认宿主身份。不要沿用本文版本推定当前宿主。
2. `python3 docs/features/cindy-plugins/spike/scripts/configure.py` 生成独立临时根；已有配置则拒绝重复创建。
3. `GOCACHE=/private/tmp/lumi-cindy-step0-go-cache go build -trimpath -o docs/features/cindy-plugins/spike/probe/bin/lumi ./cmd/lumi`；不运行 Rust。
4. `node docs/features/cindy-plugins/spike/scripts/local-check.cjs`。若当前 Agent 沙箱禁 loopback bind，按正常执行审批运行同一检查，不改变 Cindy 批准配置。
5. `ghost_forge_pack({dir:<probe 的绝对路径>})`，查看相应版本的 `evidence/pack-*.json`；包中 `bin/lumi` 应为 `100755`。
6. `ghost_forge_install({dir:<同一 probe 目录>})` 走宿主正常确认，然后以真实回执 namespace/instanceId 查询和调用。
7. 结束先调用 `call_tool(name=stop)`，用 status 检查两个实例均为 false，再从 Cindy 插件详情仅卸载本探针。

0.0.4 额外复现：`task_create(a/b)` → `task_send(a/b)` → 在各测试任务保留 Ask 的正常确认 → `task_read(a/b)`；不要重发已接受的请求或改批准配置。`session_probe` 只接受登记任务的 Host sessionContext，验证两者独立项目及共享 a 项目服务。`lifecycle_start` 使用独立 Node 入口；至少等待 130 秒，仅从临时 JSONL/只读进程列表观察，然后调用 `lifecycle_report`，期间不更新插件或调用此入口。

0.0.5 的 hooks 在正常更新确认后，先 `hooks_enable`，测试任务实际完成/中断后 `hooks_report`；没有投递即记未验证，不采集普通任务填充样本。只读证据校验：`python3 docs/features/cindy-plugins/spike/scripts/verify-records.py`，不会发起模型、安装或重跑 Host 测试。

## 运行数据和清理范围

本轮临时根：`/private/tmp/lumi-cindy-step0-20261101-silc5gb5`。项目分别在 `a/project`、`b/project`；data-dir 分别在 `a/data`、`b/data`。所有 CLI 命令显式带该 data-dir、environment 和 cwd，环境变量只保留最小系统 PATH、临时 HOME/TMPDIR。

本轮该临时根、专用 Go cache 和作者目录 probe-config.json 均已删除；复现从 configure.py 重新生成，再 pack/install，不能直接安装保留的历史审阅包运行。两个临时 Cindy 任务已归档，Host 受管生成图保留；五个审阅包与 CLI 是被 Git 忽略的本地生成物。

`worker-evidence.jsonl` 是探针原始日志；仓库证据需去除真实用户路径。源码中的 config 是本轮特有，复现必须重新配置。测试只通过 Lumi 自身 `stop` 关闭标记实例，不按 PID 猜测杀进程，不删除锁文件解锁。构建缓存只属于本次探针，最终清理时单列说明。

本地检查通过不等于 Cindy Host 通过；一次 WS join 不等于持续通知或 UI 重连；合成并发不等于两个真实任务；1px fixture 不等于模型生图。完整结论见上级 `spike.md`。
