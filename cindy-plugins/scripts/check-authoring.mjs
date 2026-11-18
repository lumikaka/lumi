// Read-only Cindy workflow gate. Shared skills and Go stay authoritative;
// reviewed source hashes force a new comparison when those inputs change.
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { resolve } from 'node:path';

const repo = fileURLToPath(new URL('../../', import.meta.url));
const require = createRequire(import.meta.url);
const manualPath = 'cindy-plugins/lumi/manual/authoring/MANUAL.md';
const settingsPath = 'cindy-plugins/lumi/manual/authoring/references/settings.md';
const skill = 'plugin-skills/create-picture-book/SKILL.md';
const usage = 'plugin-skills/create-picture-book/references/mcp-usage.md';
const serial = 'plugin-skills/create-picture-book/references/serial-production.md';
const rules = [
  { id: 'serial-production', manual: 'cindy-plugins/lumi/manual/authoring/references/serial-production.md', sources: [serial], required: [
    /project_overview → project_setup get → book_outline/, /user-data\/lumi-production\//, /storyboard\/page-<序号>.json/,
    /source_spans/, /utterances/, /continuity/, /storyboard-commit.json/, /分镜任务数为 0/, /每章仅一个分镜角色/,
    /每章一个设定角色/, /mode=serial/, /mode=independent_pair/, /user_choice/, /最多两场/, /前章实际通过/,
    /review_status=passed/, /variant\/revision/, /checked_at/, /settings_repair/, /paused 也保留名额/,
    /pending_requests/, /只有 status=done/, /不能借旧版 passed 交接后场或下一章/,
    /production_tasks/, /request_status/, /resume_request/, /PERMISSION_DENIED 停止/, /reconciling 不等于完成/,
    /retry_create/, /pending、unknown、取消、请求键冲突或已成功请求均不能通过此入口替换/,
    /page_scope=/, /不能仅靠省略场景缩小范围/, /范围修复不取消已有活动写者或未决请求/,
    /仅描述 production_tasks 适配器/, /原生 thread、subagent、Orca Worker、普通 session 不能互相冒充/,
    /使用宿主 send_to_session 新建模式/, /新建时不传 target_session_id/, /不改用 spawn_agent/,
    /send_to_session 新建会话不自带父子等待或自动完成回报/, /父任务只创建和等待时不接手制作/,
    /禁止 sleep → 查状态 → 汇报 → 再 sleep/, /结束当前轮不等于取消子任务或宣布任务完成/,
    /未经确认的自动唤醒不能承诺/, /run_id\/request_key/, /已处理事件键/, /迟到进度不覆盖终态/,
    /子任务只产草稿，当前 Lumi 任务唯一串行写回/,
    /pending 先看已有图/, /不重复建页\/建章/, /不重复提交/, /不承诺节省 token/,
  ] },
  { id: 'user-intent', section: 'Setup 与故事', sources: [skill, usage], required: [
    /区分用户明确要求与 Agent 自选方案/, /generation_brief/, /原始用户要求/, /不能当作用户限制/,
    /已获得连续执行授权/, /不新增重复确认节点/, /权限拒绝、取消或结果未知/,
  ] },
  { id: 'strip-plan', section: '设定、章节与页面', sources: [skill, usage, 'internal/textmcp/prompt.go'], required: [
    /竖向条漫默认传/, /"moment_count_plan":"auto"/, /max_section_count/, /1–24/, /每页 1～3 镜/,
    /省略 max_moments_per_section/, /保留该逐页数组，实际写几页就使用前几项/,
    /用户明确指定其他节奏时优先采用/, /max_moments_per_section/, /保存前逐页核对镜头数与计划一致/,
    /不通过截断对白、删除剧情凑数/, /不自动改写用户已有作品/,
  ] },
  { id: 'page-review', section: '页面成图核对与续做', sources: [skill, 'internal/textmcp/schema.go', 'internal/production/host_page_review.go'], required: [
    /read_image\(asset_uuid\)` 实际查看成图/, /完整正文、storyboard_md 和用户要求/, /镜头数与计划/,
    /文字、说话人和指定对白/, /不能凭生成成功回执或只看参考合图宣称成图通过/,
    /看不清文字时不得记 passed/, /看图后重读当前版本/,
    /call_tool/, /action="record_image_review"/, /chapter_uuid/, /page_uuid/, /variant_uuid/, /expected_revision/,
    /verdict="passed"/, /"needs_revision"/, /具体 `observation`/, /expected_review_revision=review_revision/,
    /不能把旧图的通过结论复制给新图/,
  ] },
  { id: 'setting-reference', manual: settingsPath, sources: [
    'plugin-skills/create-picture-book/references/setting-references.md',
  ], required: [
    /用户明确要求优先于原图特征，再优先于代理自选细节/, /每批核实请求包含原图/,
    /verdict 仅 passed \/ needs_revision，通过才用于正文/,
    /取消\/任务切换\/释放及 worker 重建后，旧传输令牌失效/,
  ] },
  { id: 'resume', section: '页面成图核对与续做', sources: [skill, 'internal/production/host_page_review.go'], required: [
    /project_overview → project_setup get → book_outline/, /chapter_uuid/, /cursor_pagination/,
    /pages list\(summary_only=true\)/, /\| missing_image \| 缺图：/, /\| pending \| 已有图待核对：/,
    /不能当成缺图重画/, /\| needs_revision \| 读取完整 observation，只修该页/, /\| passed \| 跳过/,
    /不能据此自动重画/, /分别报告范围内已生成页数、核对通过页数/,
  ] },
  { id: 'export', section: '文本审阅与导出', sources: [skill, 'internal/textmcp/schema.go', 'internal/production/text_export.go'], required: [
    /单章必须传该章 `chapter_uuid`，整书省略/, /`scope`、`chapter_uuid`、`page_count`、`missing_image_count`/,
    /完整绘本：.*当前版本核对 passed.*allow_missing_images=false.*missing_image_count=0/,
    /缺图预览：只有用户接受预览或已授权中断时交付进度时才用.*allow_missing_images=true/,
    /PDF 至少需要一张正文图/, /纯文字作品：用户明确要求纯文字时/, /零插图时完成完整文本交付/,
    /全部正文和分镜均保留/, /图片里的文字会进入 PDF/,
  ] },
  { id: 'host-media', section: '图片、参考图与比例', sources: [
    'plugin-skills/create-picture-book/references/page-images.md',
    'plugin-skills/create-picture-book/references/setting-references.md',
  ], required: [
    /setting_reviews/, /reference_delivery.kind=full_collage、status=ready 与 reference_media_url/,
    /generation_prompt 原样传入/, /同一完整 prompt/, /原样传 preparation/,
    /attachments 必须在 ghost_call 顶层/, /call_tool 无法代替附件入口/,
    /production_conflict/, /不能只换 revision 自动覆盖/,
  ] },
  { id: 'local-edit', section: '图片、参考图与比例', sources: [
    'plugin-skills/create-picture-book/references/page-images.md', 'internal/textmcp/schema.go',
    'internal/production/page_image_local_edit.go',
  ], required: [
    /pages get\(include_versions=true\)/, /未采用的历史版本/, /整页左上角/, /selection=\{x,y,width,height\}/,
    /setting_uuids 省略或 \[\]/, /不传 setting_reviews/, /reference_mode=local_edit/,
    /reference_delivery.kind=local_edit_crop/, /原尺寸上下文裁图/, /唯一裁图作为图片输入/,
    /原样传 generation_prompt/, /原签名 preparation/, /fit_mode=keep/,
    /选区外像素保持一致/, /adopted=false/, /用户明确采用后才 `pages select_image`/,
    /已有写入回执/, /不重复导入/, /不重复提交生图/,
  ] },
  { id: 'website-export', section: '静态阅读网站', sources: [skill, 'internal/textmcp/schema.go'], required: [
    /export_website/, /不预查状态/, /comic_export_incomplete/, /至少需要一张有效正文图/, /原幂等键/, /done=true/, /output_directory/, /publish_prompt/, /独立的文本代码块/, /公网发布须另行提出/,
  ] },
  { id: 'host-delivery', section: '文本审阅与导出', sources: ['cindy-plugins/lumi/node/export.cjs'], required: [
    /lumi-exports\//, /Cindy 当前任务的文件列表/, /delivery.status=prepared/, /user_received 仍为 false/,
    /实际读取回执或用户确认/, /export_delivery_failed/, /不为修复交付自动重新导出/,
  ] },
  { id: 'recovery', section: '冲突、中断与额度恢复', sources: [skill], required: [
    /不得只换新 revision 重发旧文本/, /result_unknown/, /结果未知时不自动重写/,
    /重新获取可信任务绑定/, /SUBMISSION_OUTCOME_UNKNOWN/, /不沿用缓存写权限/,
  ] },
];

function section(text, heading) {
  const start = text.indexOf(`\n## ${heading}\n`);
  if (start < 0) return '';
  const end = text.indexOf('\n## ', start + 1);
  return text.slice(start, end < 0 ? undefined : end);
}

export function checkAuthoring({
  read = name => readFileSync(resolve(repo, name), 'utf8'),
  catalog = require('../lumi/node/catalog.cjs'),
} = {}) {
  const problems = [];
  const lock = JSON.parse(read('cindy-plugins/scripts/authoring-sources.json'));
  const manual = read(manualPath);
  const pageLimit = read('internal/production/comic.go').match(/^const MaxGeneratedComicSections = (\d+)$/m)?.[1];
  if (!pageLimit || !section(manual, '设定、章节与页面').includes(`合法范围 1–${pageLimit}`)) {
    problems.push('strip-plan: max_section_count range differs from internal/production/comic.go MaxGeneratedComicSections');
  }
  if (!lock.sources['scripts/build-skills.mjs']) problems.push('source: missing source pin scripts/build-skills.mjs');
  for (const rule of rules) {
    for (const source of rule.sources) if (!lock.sources[source]) problems.push(`${rule.id}: missing source pin ${source}`);
    let content = manual;
    if (rule.manual) {
      try { content = read(rule.manual); }
      catch (error) {
        if (error.code !== 'ENOENT') throw error;
        problems.push(`${rule.id}: missing ${rule.manual}`);
        continue;
      }
    }
    for (const pattern of rule.required) if (!pattern.test(rule.section ? section(content, rule.section) : content)) {
      problems.push(`${rule.id}: ${rule.manual || manualPath}${rule.section ? ` / ${rule.section}` : ''} lacks ${pattern}`);
    }
  }
  for (const [source, expected] of Object.entries(lock.sources)) {
    const actual = createHash('sha256').update(read(source)).digest('hex');
    if (actual !== expected) problems.push(`source drift: ${source}; compare Cindy rules before updating authoring-sources.json`);
  }
  // Verify reuse of the existing business capability, not a Cindy-only schema.
  const snapshot = JSON.parse(read('cindy-plugins/lumi/node/business-specs.json'));
  const pages = snapshot.find(s => s.Name === 'pages')?.Schema;
  if (!pages?.properties.action.enum.includes('record_image_review')
    || JSON.stringify(catalog.specs.pages.input_schema) !== JSON.stringify(pages)
    || !catalog.allowed.has('pages')) {
    problems.push('capability: reuse existing pages record_image_review');
  }
  const pageRules = catalog.discover({ category: 'pages' }).rules.join('\n');
  if (!pageRules.includes('authoring/MANUAL.md') || !pageRules.includes('record_image_review')) {
    problems.push('discovery: pages rules must point to authoring/MANUAL.md for record_image_review');
  }
  // Expose the independent-session route before an Agent chooses a child type.
  // This is host guidance, not a new Lumi tool or a native wait capability.
  const manifest = JSON.parse(read('cindy-plugins/lumi/ghost.json'));
  for (const [surface, content] of [
    ['ghost.json production_tasks', manifest.tools.find(tool => tool.name === 'production_tasks')?.description || ''],
    ['production discovery', catalog.discover({ category: 'production' }).rules.join('\n')],
    [manualPath, manual],
  ]) {
    if (![/create_thread/, /send_to_session 新建模式/, /不传 target_session_id/, /不[^。]*改用 spawn_agent/]
      .every(pattern => pattern.test(content))) {
      problems.push(`session-routing: ${surface} must map independent Cindy sessions to send_to_session, not spawn_agent`);
    }
  }
  if (!section(manual, '发现与项目识别').includes('namespace')) problems.push('host: preserve discovered plugin identity and namespace');
  const localSchema = catalog.specs.prepare_page_image.input_schema;
  if (!localSchema.properties.base_variant_uuid || !localSchema.properties.selection
    || !catalog.specs.receive_media.input_schema.properties.preparation.properties.local_edit) {
    problems.push('local-edit: missing historical selection or signed import schema');
  }
  const mediaRules = catalog.discover({ category: 'media' }).rules.join('\n');
  if (!mediaRules.includes('local_edit_crop') || !mediaRules.includes('adopted=false')) {
    problems.push('local-edit: media discovery must explain crop delivery and explicit adoption');
  }
  // Host-specific generation/import interfaces must never leak from a shared skill.
  if (/\b(?:referenced_image_paths|num_last_images_to_include|import_image|generate_page_image)\b/.test(manual)) {
    problems.push('host: foreign generation/import/review interface in Cindy Manual');
  }
  return problems;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const problems = checkAuthoring();
  if (problems.length) {
    console.error(`CINDY AUTHORING CHECK FAIL\n${problems.join('\n')}`);
    process.exitCode = 1;
  } else console.log(`CINDY AUTHORING CHECK PASS: ${rules.length} workflow rules, reviewed shared sources, existing pages capability and Cindy host boundaries`);
}
