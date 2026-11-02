# 任务页身份：宿主接入条件

2026-11-02。本地接入需求记录，未向外部提交。对应 [Step 5](step5-task-view.md) 的任务隔离前置，不改变四视图目标。

## 当前已知事实

Cindy 0.1.121 的随包taskView契约通过URL查询参数session标识任务路由；sessionContext则由Host为真实Agent工具调用铸造。两者分别用于页面路由和工具执行，现有契约没有提供对页面请求发送者的可信任务身份。

真实诊断页曾在同一个页面中发出A/B两种session声明，Host页面身份凭据未出现，project_content_read=false。见 [真实探针回执](acceptance/step6-evidence/host-run-20261102.json) 及 [任务页验收](task-view-acceptance.md)。这是前置身份门槛未满足的证据，不是已经发生跨项目内容泄露的声明。生产业务读桥保持关闭。

本次只读更新检查仍是0.1.121/no_installable_update，见 [当前运行条件](acceptance/step6-evidence/remaining-readiness-20261102.json)。用户已选择等待宿主支持；无需用户提供Cindy源码，也不在当前安装目录补丁或旁路读取作品。

## 可接入的宿主能力

Host需要让插件逻辑页验证：发出业务读取请求的具体WebView，属于哪个插件实例、哪个任务，以及该页面实例是否仍有效。可采用Host认证的点对点通道或Host校验的页面身份机制；此处不虚构当前不存在的API名称或令牌格式。

身份必须来自Host观察到的发送者，不能采信页面自行提供的session、URL、origin、BroadcastChannel名字或JSON字段。仅把可重放令牌放进同源广播，也不能替代发送者校验。可信工具调用的sessionContext不能由页面借用或伪造。

任务切换、重新开页、停用、卸载及绑定失效后，Host应撤销旧页面的读取/唤醒资格。逻辑页收到可信身份后仍只按当前绑定查询Lumi UUID，不向页面提供callId、任意目录或loopback连接凭据。

## 恢复Step 5前的验收

1. 固定新宿主版本及可核验实现依据，用两个不同任务/作品的真实taskView验证发送者身份。
2. 页面修改session查询、改发另一任务声明、重放旧页面消息时，不返回其它任务内容；保持失败关闭。
3. 关闭/重开、任务切换和插件停用后，旧页面不能重新取得内容；原任务正常页仍可恢复。
4. 身份门槛通过后，再接入整本、单页、候选、设定四视图及受控图片、WS变更提示到Query失效/REST重读。

本文是宿主接口验收需求，四视图尚未实现，不将该文档或诊断页计作Step 5交付。
