# 正式 Lumi 独立作品的真实宿主验收

2026-11-02，首轮初始化留存独立 draft 后被旧后台拒绝，原报告保持不变；[协议修复](step6-shared-protocol-fix.md)后，原任务已从该 draft 完成三页及指定章节隔离检查，见[恢复成功证据](step6-evidence/normal-task-production-recovery-success-20261102.json)。**本项已完成相应范围验收，首次初始化与[恢复指令](normal-task-production-recovery.md)均仅保留历史，无需再次发送或初始化。** 两份真实 CLI 的[自动化回归](step6-evidence/channel-isolation-20261102.json)保持原证据范围。

## 执行位置

在 Cindy 新建普通作品任务，选择一个专用于本次正式版验收的独立空目录，再发送下方指令。当前源码任务仍绑定 Lumi 仓库，不能在仓库初始化作品；原开发作品及其任务也保持原样。当前宿主任务管理接口不允许执行中的调用任务移动目录，因此本项需要用户选择普通作品任务的目录后执行。

## 可复制指令

```text
在本任务当前已选择的独立空目录中，验收正式 Lumi 的独立作品。只使用实时发现的正式插件 ghost_id=lumi、namespace=null，不能切换 lumi-dev 或验收探针。先读随包 project/authoring 手册和 project 类工具 schema；核对当前目录确为这次测试的空目录，若已有项目或冲突文件立即停止，不删除文件或换目录强行初始化。

先 runtime_status，记录实际包 build_id、CLI SHA256、environment=production；若环境不符就停止。读取 project_overview，仅在明确 project_not_initialized 时 project_init，idea="一只纸船把问候送过三座小桥"、name="正式版独立作品验收"、generation_language="zh-Hans"。通过 project_setup 的 get/update/finalize 完成 setup：温暖水彩、横版 classic_picture_book，创作简报为“三页合成验收作品，不调用付费媒体”。各次修改使用刚读取的 expected_revision。

通过实际发现的 story_profile、settings、chapters、pages 工具，保存一段短故事、纸船角色设定、一个章节和三页正文/画面描述。内容必须明确包含“PRODUCTION-ONLY-20261102”标记。只用当前项目返回的 UUID 和 revision，不自造 UUID，不使用文件或 SQL 写入作品。

逐项重读，比较完整故事、设定、章节及三页正文/画面描述与提交文本是否完全一致，记录项目/章节/页 UUID 和 revision。正常开发作品的 project_uuid=01a199d7-323a-7017-a5da-1d93303f487e、chapter_uuid=01a199e6-89e9-7213-a333-924bffa24af8；正式新项目 UUID 必须不同。仅在当前正式作品内调用 chapters.get 读取上述开发章节 UUID，预期拒绝；不尝试写入该 UUID、不打开开发作品目录。随后重读正式作品，确认正文和 revision 未变。

不生图、不导入图片、不导出、不安装或卸载插件、不停止任何服务，不启动其它 Agent。取消、拒绝或结果未知时保存实际回执，先核对事实，不自动重放写入。

结束时 runtime_release，保留作品和共享服务。另存《正式版独立作品验收报告.md》，记录实际插件身份、构建/环境、项目 UUID、全部写入及重读回执、完整文本比较结果、跨项目读取拒绝和释放回执；不要输出凭证、巨大 base64 或内部数据库 id。明确本报告只证明普通 Cindy 正式作品写入与隔离，不证明 Step 5、Codex 共享或旧诊断进程退出。返回报告的完整路径。
```

## 父任务收尾条件

取得报告后核对真实身份、正文比较、跨项目拒绝及 release 回执；再核对已授权原开发作品的既有事实未变，才能将普通 Cindy 双实例独立作品项记为通过。源代码测试构建与已安装构建分别记录；两项延后[待办](todos.md)和 Step 5 跳过说明保持原状态。
