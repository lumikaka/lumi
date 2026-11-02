# 产品适配器宿主验收探针

这是独立测试插件，不能当作正式 Lumi 身份或普通任务绑定的验收证明。构建脚本把当前开发目录的 main.js 原文嵌入静态函数，复制同一份 Node 模块与 CLI，并记录源适配器 SHA256。测试 wrapper 只使用公开 tasks.create({isolatedWorkspace:true}) / tasks.get 回执建立 a/b 临时目录，不接收模型填写的路径，不发送消息，也不启动其他 Agent。

这样可以在当前源码任务中验证真实 Host 授权图片经过同一产品适配器进入独立 Lumi 项目，而不把源码仓库初始化成绘本。它不替代正常产品 session_context 注入、独立桌面人工观察或手机交付验收。临时 taskView 只记录公开上下文与通信身份边界；没有可验证的页面身份时不读取作品。

构建前先 make cindy-dev，然后执行：

```sh
node docs/features/cindy-plugins/spike/product-probe/build.mjs
node --check build/cindy-probe/lumi-host-acceptance-20261101/main.js
node --test cindy-plugins/tests/host-probe.test.cjs
```

生成作者目录 build/cindy-probe/lumi-host-acceptance-20261101/，由正常 Forge pack/install 验收。新增 agent.tasks 权限只属于这个临时探针，不进入产品 manifest。临时空任务保持可见、缺省权限，不增加 hidden、targets、hooks 或全局访问。

原生 CLI 现在先复制到同目录的新文件，再原子替换目标，保留执行位；不会就地截断仍可能被进程使用的旧可执行文件。测试验证旧打开句柄继续读取旧字节、目标 inode 改变，以及拒绝目标符号链接。本次重新构建后的新 CLI 已正常输出 --help；这不证明上轮 UE 进程的原因，也不代表旧进程已退出。

2026-11-02 已在真实 Host 安装并创建 A/B，任务 ID 见 Step 6 回执。更新后原 create 请求键重放遇到参数冲突，不换键重复创建。使用 create_fixture({fixture,task_id}) 传入前次 Host 返回的任务 ID，内部经 tasks.get 的本插件归属校验与固定标题核对恢复；不接受模型路径。未保存或不能核实任务 ID 时停止，不扫描或接管其它任务。

专用 development data-dir 是 /private/tmp/lumi-cindy-host-acceptance-20261101。清理时先 release_fixture 撤销两任务绑定，再核对专用 CLI 实例身份并停止；不能关闭默认 ~/.lumi_dev、Codex 或正式服务。卸载只移除该精确测试插件，不删除作品、导出或用户任务。
