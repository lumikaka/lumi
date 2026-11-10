'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync, writeFileSync, mkdirSync, mkdtempSync, rmSync, existsSync, cpSync } = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const { spawnSync } = require('node:child_process');
const catalog = require('../lumi/node/catalog.cjs');
const repo = path.resolve(__dirname, '../..');
const manualPath = 'cindy-plugins/lumi/manual/authoring/MANUAL.md';
const settingsPath = 'cindy-plugins/lumi/manual/authoring/references/settings.md';
const read = name => readFileSync(path.join(repo, name), 'utf8');
const checker = import('../scripts/check-authoring.mjs');

test('Cindy authoring gate accepts the reviewed workflow and runs read-only', async () => {
  const { checkAuthoring } = await checker;
  assert.deepEqual(checkAuthoring(), []);
  const result = spawnSync(process.execPath, ['cindy-plugins/scripts/check-authoring.mjs'], { cwd: repo, encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /CINDY AUTHORING CHECK PASS/);
});

test('gate detects omitted workflow rules, wrong density and foreign host instructions', async () => {
  const { checkAuthoring } = await checker;
  const manual = read(manualPath);
  for (const [id, before, after] of [
    ['user-intent', '不新增重复确认节点', '再次要求确认'],
    ['strip-plan', '"moment_count_plan":"auto"', '"moment_count_plan":"[5,6]"'],
    ['strip-plan', '每页 1～3 镜', '每页 1～6 镜'],
    ['strip-plan', '省略 max_moments_per_section', '总是指定 max_moments_per_section'],
    ['strip-plan', '保留该逐页数组，实际写几页就使用前几项', '自行重新抽取镜头计划'],
    ['strip-plan', '不通过截断对白、删除剧情凑数', '删去对白以满足数量'],
    ['strip-plan', '不自动改写用户已有作品', '自动重排已有作品'],
    ['page-review', 'action="record_image_review"', 'action="update"'],
    ['page-review', 'read_image(asset_uuid)` 实际查看成图', '根据生成回执推定通过'],
    ['page-review', '看不清文字时不得记 passed', '看不清文字时也记 passed'],
    ['page-review', '看图后重读当前版本', '看图后沿用旧版本'],
    ['resume', '| pending | 已有图待核对：', '| pending | 重新生成：'],
    ['resume', '| missing_image | 缺图：', '| missing_image | 跳过：'],
    ['resume', '| needs_revision | 读取完整 observation，只修该页', '| needs_revision | 重画全书'],
    ['resume', '| passed | 跳过', '| passed | 重画'],
    ['export', '单章必须传该章 `chapter_uuid`，整书省略', '总是导出全书'],
    ['export', 'allow_missing_images=false', 'allow_missing_images=true'],
    ['export', '零插图时完成完整文本交付', '必须有图片'],
    ['host-media', 'attachments 必须在 ghost_call 顶层', 'attachments 塞入 args'],
    ['host-delivery', 'user_received 仍为 false', 'user_received 为 true'],
    ['recovery', '结果未知时不自动重写', '结果未知时重新提交'],
    ['host', 'generation_prompt 原样传入', 'generation_prompt 原样传入；使用 referenced_image_paths'],
  ]) {
    assert.ok(manual.includes(before), `invalid mutation fixture: ${before}`);
    const errors = checkAuthoring({ read: name => name === manualPath ? manual.replace(before, after) : read(name) });
    assert.ok(errors.some(error => error.startsWith(`${id}:`)), `${id}: ${errors.join('\n')}`);
  }
});

test('gate checks the linked Cindy setting/reference workflow, including missing files', async () => {
  const { checkAuthoring } = await checker;
  const settings = read(settingsPath);
  for (const before of [
    '用户明确要求优先于原图特征，再优先于代理自选细节',
    '每批核实请求包含原图',
    'verdict 仅 passed / needs_revision，通过才用于正文',
    '取消/任务切换/释放及 worker 重建后，旧传输令牌失效',
  ]) {
    assert.ok(settings.includes(before), `invalid mutation fixture: ${before}`);
    const errors = checkAuthoring({ read: name => name === settingsPath ? settings.replace(before, '') : read(name) });
    assert.ok(errors.some(error => error.startsWith('setting-reference:')), errors.join('\n'));
  }
  assert.ok(checkAuthoring({ read: name => {
    if (name === settingsPath) throw Object.assign(new Error('missing fixture'), { code: 'ENOENT' });
    return read(name);
  } }).some(error => error.startsWith('setting-reference:')));
});

test('unchanged shared generator rejects stale host artifacts in a temporary copy without rewriting them', t => {
  const fixture = mkdtempSync(path.join(os.tmpdir(), 'cindy-skill-drift-'));
  t.after(() => rmSync(fixture, { recursive: true, force: true }));
  for (const name of ['scripts/build-skills.mjs', 'plugin-skills', 'codex-plugins/lumi/skills', 'claude-plugins/lumi/skills']) {
    mkdirSync(path.dirname(path.join(fixture, name)), { recursive: true });
    cpSync(path.join(repo, name), path.join(fixture, name), { recursive: true });
  }
  const check = () => spawnSync(process.execPath, ['scripts/build-skills.mjs', '--check'], { cwd: fixture, encoding: 'utf8' });
  assert.equal(check().status, 0);
  for (const host of ['codex', 'claude']) {
    const artifact = path.join(fixture, `${host}-plugins/lumi/skills/create-picture-book/SKILL.md`);
    const original = readFileSync(artifact, 'utf8');
    const stale = original.replace('record_image_review', 'obsolete_review');
    assert.notEqual(stale, original);
    writeFileSync(artifact, stale);
    const result = check();
    assert.notEqual(result.status, 0);
    assert.ok(result.stderr.includes(`differs ${host}/create-picture-book/SKILL.md`), result.stderr);
    assert.equal(readFileSync(artifact, 'utf8'), stale, 'check must not repair stable artifacts');
    writeFileSync(artifact, original);
  }
  assert.equal(check().status, 0);
});

test('gate detects upstream drift and missing source pins without modifying sources', async () => {
  const { checkAuthoring } = await checker;
  const lockPath = 'cindy-plugins/scripts/authoring-sources.json';
  const lock = JSON.parse(read(lockPath));
  for (const source of Object.keys(lock.sources)) {
    const errors = checkAuthoring({ read: name => read(name) + (name === source ? '\nChanged upstream\n' : '') });
    assert.ok(errors.some(error => error.startsWith(`source drift: ${source};`)), source);
  }
  delete lock.sources['internal/textmcp/prompt.go'];
  assert.ok(checkAuthoring({ read: name => name === lockPath ? JSON.stringify(lock) : read(name) })
    .some(error => error.startsWith('strip-plan: missing source pin')));
  assert.ok(checkAuthoring({ read: name => name === 'internal/production/comic.go'
    ? read(name).replace('const MaxGeneratedComicSections = 24', 'const MaxGeneratedComicSections = 25') : read(name) })
    .some(error => error.startsWith('strip-plan: max_section_count range differs')));
});

test('gate detects loss of existing review capability or its discovery pointer', async () => {
  const { checkAuthoring } = await checker;
  const broken = { ...catalog, specs: structuredClone(catalog.specs) };
  broken.specs.pages.input_schema.properties.action.enum = ['list', 'get'];
  assert.ok(checkAuthoring({ catalog: broken }).some(error => error.startsWith('capability:')));
  assert.ok(checkAuthoring({ catalog: { ...catalog, discover: () => ({ rules: [] }) } })
    .some(error => error.startsWith('discovery:')));
});

test('Cindy build rejects an omitted rule before generating contracts, UI or binaries', t => {
  const fixture = mkdtempSync(path.join(os.tmpdir(), 'cindy-authoring-gate-'));
  t.after(() => rmSync(fixture, { recursive: true, force: true }));
  const sources = Object.keys(JSON.parse(read('cindy-plugins/scripts/authoring-sources.json')).sources);
  for (const name of new Set([...sources, manualPath, settingsPath, 'internal/production/comic.go',
    'cindy-plugins/scripts/authoring-sources.json', 'cindy-plugins/scripts/check-authoring.mjs', 'cindy-plugins/scripts/build.mjs',
    'cindy-plugins/lumi/node/catalog.cjs', 'cindy-plugins/lumi/node/errors.cjs',
    'cindy-plugins/lumi/node/media-contract.cjs', 'cindy-plugins/lumi/node/business-specs.json',
  ])) {
    const target = path.join(fixture, name);
    mkdirSync(path.dirname(target), { recursive: true });
    writeFileSync(target, read(name));
  }
  writeFileSync(path.join(fixture, manualPath), read(manualPath).replace('"moment_count_plan":"auto"', '"moment_count_plan":"[5,6]"'));
  const result = spawnSync(process.execPath, [path.join(fixture, 'cindy-plugins/scripts/build.mjs'), '--channel', 'development'], { cwd: fixture, encoding: 'utf8' });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /CINDY AUTHORING CHECK FAIL/);
  assert.match(result.stderr, /strip-plan:/);
  assert.equal(existsSync(path.join(fixture, 'build')), false);
  assert.equal(existsSync(path.join(fixture, 'cindy-plugins/lumi/bin')), false);
});
