# Step 6 新增权限更新验收

2026-11-02；仅个人开发实例 `lumi-dev` / `_root__lumi-dev` / namespace=null，正式 Lumi 保留。

## 当前结论

**首次安装拒绝与原生新增权限更新拒绝均通过，各自独立记账。**

- 首次：主人明确确认导入前已卸载开发版，再拒绝首次安装；GHOST_NOT_FOUND与原生列表无开发版符合预期，未证明拒绝更新删除旧包。见 [首次安装拒绝](step6-evidence/permission-manual-rejection-20261102.json)。原基础包随后按既有授权恢复，见 [恢复证据](step6-evidence/permission-manual-rejection-restoration-20261102.json)。
- 更新：保留已安装开发版，直接从文件导入同一已审阅测试包，主人回复“更新已拒绝”。本轮没有安装或恢复旧包；正常root发现与runtime_status成功，原开发build_id `29401ca7cbd2cff3f21d5d2f7c3e2237f31990bfd99ac1e005dfd5faa068c1a8`、CLI SHA256 `d26c4636d6b0116520a3a2baef7cf203676ec6a91d3bc06ea4c58f9aef395c47`均与拒绝前基线一致，测试build `dfb75de6…`未生效。正式Lumi仍是原production构建，PDF/ZIP大小及SHA256、七份旧报告摘要未变，两条连接release=true/shared_retained。见 [拒绝前基线](step6-evidence/permission-native-update-refusal-baseline-20261102.json) 和 [更新拒绝成功证据](step6-evidence/permission-native-update-refusal-success-20261102.json)。

本次结论覆盖原生文件更新的拒绝与旧包继续可调用。原生拒绝点击由主人确认，后置状态由真实只读工具与独立文件核对验证；没有本任务MCP安装取消调用，不填造MUTATION_CANCELLED。没有重读章节或双图，不冒称本轮已做业务作品恢复；没有新生图、网络请求、图片导入、导出或作品写入。

## 测试包与真实回执

[准备记录](step6-evidence/permission-rejection-preflight-20261102.json)记录独立作者副本、21文件指纹、构建、冻结审阅包。相对基础包只修改 `ghost.json` 和派生的 `build-info.json`：新增未使用的 `network.hosts=["lumi-permission-check.invalid"]`，releaseNotes 明示本次应拒绝。CLI、执行代码、Manual与运行配置逐字节不变。测试包不作为产品交付，基础作者目录保持不变。

1. 首次正常 Forge 更新返回 `updated`。主人随后确认“看到了，并点击允许”，所以该结果符合实际允许操作，不是宿主拒绝失效的证据。随即恢复原包并核对21条目/权限位、原构建、正式版诊断和PDF/ZIP摘要。见 [第一次更新及恢复](step6-evidence/permission-rejection-observation-20261102.json)。
2. 再次明确提示点击拒绝后，第二次请求仍返回 `updated`，没有 `MUTATION_CANCELLED`；没有独立界面点击观察，不能推断其审批原因。再次恢复基础包，原构建与CLI摘要、ready、release均核对通过。见 [第二次回执及最终恢复](step6-evidence/permission-rejection-final-restore-20261102.json)。不再自动提交相同测试更新。

## 原生文件拒绝方式与当前处理

本项来自 [Step 6 完成标准第2项](../step6-plugin-install.md#完成标准)，需要真实拒绝新增权限，然后确认旧包仍可调用。准备产物、外层 MCP 拒绝、允许更新或恢复原包均不能代替此结果。

本轮已经按保留旧开发版、直接从文件导入测试包并拒绝更新的顺序完成，不需要重复操作。旧构建持续可调用以拒绝前后runtime_status及本轮没有重装为依据。

此前Agent安装工具的两次updated只作为允许/实际更新历史保留，不计作拒绝结果；先卸载后的首次安装拒绝也单独记账。Step 5仍等待可信任务页身份，四视图和其余Step 6共享/取消矩阵不由本项推断通过。
