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
const rules = [
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
  { id: 'website-export', section: '静态阅读网站', sources: [skill, 'internal/textmcp/schema.go'], required: [
    /export_website/, /不预查状态/, /comic_export_incomplete/, /至少需要一张有效正文图/, /原幂等键/, /delivery.status=prepared/, /公网发布须另行提出/,
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
    || !catalog.allowed.has('pages') || catalog.categories.review.length) {
    problems.push('capability: reuse existing pages record_image_review; do not add a review interface');
  }
  const pageRules = catalog.discover({ category: 'pages' }).rules.join('\n');
  if (!pageRules.includes('authoring/MANUAL.md') || !pageRules.includes('record_image_review')) {
    problems.push('discovery: pages rules must point to authoring/MANUAL.md for record_image_review');
  }
  if (!section(manual, '发现与项目识别').includes('namespace')) problems.push('host: preserve discovered plugin identity and namespace');
  // Host-specific generation/import interfaces must never leak from a shared skill.
  if (/\b(?:referenced_image_paths|num_last_images_to_include|import_image|generate_page_image|review_book|review_chapter|review_page)\b/.test(manual)) {
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
