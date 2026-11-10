# Cindy 创作流程独立验收（2026-11-10）

结论：本轮 Cindy 源码流程及其直接影响验收通过，防漏检查和导出验证的遗漏已补齐。没有发现需要修改共享业务或稳定插件才能解决的问题。

验收对象为 `704b29000738317fb0ff0357a305a9f1407e6a52` 相对父提交 `a875198f5d3c2b6e9f9a15d055c4cdbcdf75ec91` 的改动，以及本次补漏。开始时工作区干净。先读取根 AGENTS.md、`cindy-plugins/README.md` 的本轮交付说明和全部 9 个文件的实际差异；历史三端验收没有计作本次证据。

## 逐项结论

| 检查项 | 本次证据与结论 |
| --- | --- |
| 自动镜头计划与保存前核对 | Manual「设定、章节与页面」明确使用 `moment_count_plan=auto`、省略上限覆盖、保留计划前缀、逐页保存前核对。当前 CLI 实测同章 4 页计划扩展为 6 页时前 4 项一致，首组三项为 1/2/3，用户指定 `[5,6]` 仍生效；完整正文和分镜保存后逐项读回。删除计划保留或省略上限规则现会被门禁拒绝。 |
| 成图核对、续做与交付 | 复用既有 `pages record_image_review`，没有新增审阅工具。临时作品同时形成 missing_image / pending / needs_revision / passed；重连后 pending 图身份不变，看原图再记录；更新观察必须带 review_revision。换图和修改分镜使旧通过结论失效，旧图 UUID 或旧页 revision 的记录被拒绝。缺图时完整导出失败，补齐并核对后单章导出成功，而另章缺图仍阻止整书完整导出。 |
| 用户要求与 Agent 自选方案 | Manual「Setup 与故事」明确由 generation_brief 分别记录，续做回查原始要求；自选页数、额度、节奏及省略步骤不能冒充用户限制。已有连续授权不新增重复确认。原图参考手册同样明确用户要求优先级，未照搬其他宿主确认或生图规则。 |
| 对照与过期检测 | 共享来源 SHA256、自动页数范围、Go Specs 实时比对、既有 schema/发现入口检查均通过。新反例在修复前实测出现 2 个失败测试，复现未拦截的规则遗漏；修复后 7 个门禁测试通过。另在临时副本分别将 Codex、Claude 生成 Skill 的 record_image_review 改为过期名字，原生成脚本的 `--check` 均报对应 differs，且没有修复或改写副本。 |
| Cindy 附件、参考图、冲突和中断 | 浏览器适配测试核对真实 callId、授权附件分块/摘要、拒绝伪造字段和只读写入；完整 PNG 合图交接字节与 prompt/preparation 保持，拒绝/损坏时不冒称已传图。当前 CLI 集成核对 826433 字节原图落盘 SHA256、受控预览、两版历史和选择；导入冲突保留用户最新页及恢复原图，取消清理传输。未知写入结果测试确认只写一次，再读回已提交文本。 |
| Codex、Claude 与公共生成链路 | 对父提交逐文件比较 Git blob 字节：Codex 12 文件、Claude 14 文件、共享 Skill 来源/生成脚本 8 文件全部一致，相关目录没有新增未跟踪文件。`build-skills.mjs --check` 和原有 4 个生成器测试通过，输出继续只覆盖 Codex、Claude。 |
| 共享改动及架构范围 | 原提交全部 9 个路径均属 cindy-plugins；共享 Go/Web、配置和 Makefile 没有本轮变化。本次只补 Cindy 检查脚本、测试和说明，没有为了统一架构改写公共生成器或核心，也没有需要收回的本轮新增共享重构。 |
| 验证是否适度 | 只运行受影响的门禁、发现、附件、参考图、恢复、交付和当前 CLI 临时作品检查。没有要求重跑三端全量 E2E。共享应用 929 个受版本控制的文件与父提交字节相同；没有改 Web 页面或样式，因此没有增加 Web 视觉回归要求。 |

## 实际补漏

- `check-authoring.mjs` 之前只读取主 Manual，引用的 `authoring/references/settings.md` 丢失或其用户原图、核对前置条件、重启失效规则被删除时仍会通过。现将该引用手册纳入只读检查，报告明确的 `setting-reference` 错误；检查规则由 8 组变为 9 组。
- 追加主 Manual 的计划前缀保留、省略 max_moments_per_section、文字看不清不得 passed、看图后重读当前版本的遗漏反例，避免这些关键步骤消失而仍通过。
- 集成检查原先仅验证 Markdown 含页面正文及导出字节摘要。现直接读取 Markdown 和 ZIP 内完整章正文、每页当前正文/分镜、ZIP 章节 UUID 和图片数量，并检查零插图 Markdown/ZIP 均保留分镜、零插图 PDF 拒绝。
- PDF 验证沿用现有渲染规则：条漫两图并排，4 个内容页缺 1 图时，回执为 page_count=4 / missing_image_count=1，PDF 为 2 个物理页。未将 3 张可用图误作 3 个物理页要求，也未修改共享拼版。

## 本次运行结果

以下为本次最终成功运行的实际输出摘要，不是历史验收回执：

```text
CINDY AUTHORING CHECK PASS: 9 workflow rules, reviewed shared sources, existing pages capability and Cindy host boundaries
CINDY CONTRACT CHECK PASS: actual Go Specs, snapshot, manifest and browser media schema (30 business tools; explicit allowlist unchanged)
Skills match plugin-skills/.
```

```sh
node --test --test-reporter=spec \
  cindy-plugins/tests/authoring-workflow.test.cjs \
  cindy-plugins/tests/catalog.test.cjs \
  cindy-plugins/tests/media-main.test.cjs \
  cindy-plugins/tests/reference-main.test.cjs \
  cindy-plugins/tests/reference.test.cjs \
  cindy-plugins/tests/recovery.test.cjs \
  cindy-plugins/tests/export.test.cjs
# tests 27 / pass 27 / fail 0

node --test --test-reporter=spec scripts/test-build-skills.mjs
# tests 4 / pass 4 / fail 0
```

另外，将当前 Cindy Node 源码、既有媒体集成测试及其固定图片复制到临时树，`go build -o <临时树>/cindy-plugins/lumi/bin/lumi ./cmd/lumi` 构建当前 CLI，写入与此二进制匹配的临时 build-info，然后同时运行该媒体测试和仓库内 `authoring-resume-integration.test.cjs`。后者也自行在临时目录构建当前 CLI。没有借用已安装或作者目录中的旧二进制：

```text
CURRENT SOURCE CLI SHA256=b111cdd2c5b2c7e3ab60682ea2b51bc74a8e8b0c32957d5f7cf374d6f7b975bd
CINDY WORKFLOW FIXTURE PASS: shared auto plan, explicit rhythm, 4 resume states, versioned reviews, scoped Markdown/ZIP/PDF and zero-image text; no model calls
MEDIA FIXTURE PASS: original 826433 bytes, SHA256 f09a1d3de26dbe010ed12cf9ff6594ea4b064f927c260ca41670e0b055b9652c; bounded preview; 2 variants; PDF/ZIP; isolated CLI cleanup
tests 2 / pass 2 / fail 0
CLEANUP PASS: temporary CLI, test tree and isolated project services removed
BASELINE BYTE COMPARISON PASS: stable plugins, shared skill sources/generator and Go/Web unchanged
```

合计 33 个测试通过，另有 3 项只读门禁及基线字节比较通过。`git diff --check` 通过。初次受限沙箱不允许监听 127.0.0.1（EPERM），取得本机回环测试权限后在新临时作品中完成集成检查，未为此放宽或重写运行时恢复策略。

## 边界

所有作品、HOME、data-dir 和编译的 CLI 均位于临时目录；只按本次实例 nonce 关闭自己创建的服务。使用固定几何图片与合成观察，证明的是流程规则、传输、版本和导出语义，不证明真实模型视觉质量或所有 Agent 都会遵循文字规则。

本次没有安装、打包、发布、推送或合并，没有修改真实作品。没有修改 Host API 或声明新的宿主兼容性；已安装插件、原生生图和原生工作区历史待验项没有重新计为通过。本轮未产生必须跨插件或共享核心实施的新需求，因此没有追加此类实现或扩大测试要求。
