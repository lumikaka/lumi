'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const { Runtime, runCLI } = require('../lumi/node/runtime.cjs');
const author = path.resolve(__dirname, '../lumi'), executable = path.join(author, 'bin/lumi');
const fixed = {
  story: '# 回家的星星\n\n小狐狸捡到一颗迷路的星星。它借来灯笼，沿溪寻找最高的山坡，最终帮助星星回到夜空。\n',
  chapter: '小狐狸把星星捧在掌心。溪边的萤火虫为它引路。登上山坡时，它打开灯笼，星星顺着光回到了家。',
  setting: '小狐狸阿栗，橙色短毛，白色尾尖，戴蓝围巾，胆小但愿意帮助别人。',
  pages: [
    ['溪边的微光', '阿栗在溪边捡到一颗发抖的星星。“我找不到家了。”星星说。', '远景：深蓝夜色中的小溪；前景：戴蓝围巾的小狐狸双手捧着金色星星。水面倒映微光，留出上方文字空间。'],
    ['借来的灯笼', '萤火虫送来一盏小灯笼。阿栗举起它，一步一步走向山坡。', '中景侧视：阿栗沿弯曲山路向右上方行走，灯笼照亮脚边石头。萤火虫围绕蓝围巾，远处山顶清晰可见。'],
    ['星星回家', '山顶的风吹开灯笼。星星升入夜空，眨了眨眼。阿栗也笑了。', '仰视全景：阿栗站在山顶，打开的灯笼位于画面左下。金色星星划出弧线飞向群星，温暖光线照亮狐狸的笑脸。'],
  ],
};
const ok = r => { assert.equal(r.success, true, JSON.stringify(r)); return r.data; };

test('fixed text through discovery → provider-free setup → story/settings/three pages → export', { timeout: 120000 }, async t => {
  const base = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-cindy-authoring-'));
  const home = path.join(base, 'home'), root = path.join(base, 'book'), other = path.join(base, 'other'), dataDir = path.join(base, 'data');
  for (const dir of [home, root, other, dataDir]) await fs.mkdir(dir);
  const env = { HOME: home, PATH: '/usr/bin:/bin', TMPDIR: os.tmpdir() };
  const build = JSON.parse(await fs.readFile(path.join(author, 'build-info.json'), 'utf8'));
  const runtime = new Runtime({ executable, build, dataDir, env });
  const status = () => runCLI(executable, ['status', '--data-dir', dataDir, '--json'], { cwd: base, env });
  const call = (name, args = {}, session = 'author', directory = root, readOnly = false) => runtime.execute('call_tool', {
    context: { session_id: session, workdir: directory, workdir_is_local: true, workdir_is_read_only: readOnly }, arguments: { name, args },
  });
  let identity;
  t.after(async () => {
    await runtime.close();
    const current = await status();
    if (current.running) {
      assert.equal(current.nonce, identity.nonce, 'only stop the owned isolated test instance');
      assert.equal((await fetch(current.endpoint + '/api/v1/cli-instance', { method: 'DELETE', headers: { 'If-Match': current.nonce } })).status, 200);
      const isStopped = async () => {
        try { return !(await status()).running; }
        catch (error) { if (error.code === 'instance_unavailable') return false; throw error; }
      };
      for (let i = 0; i < 200 && !(await isStopped()); i++) await new Promise(r => setTimeout(r, 25));
      assert.equal(await isStopped(), true);
    }
    await fs.rm(base, { recursive: true, force: true });
  });
  assert.equal(ok(await runtime.execute('list_tools', { arguments: {} })).categories.length, 8);
  assert.ok(ok(await runtime.execute('list_tools', { arguments: { category: 'pages' } })).tools[0].input_schema.allOf);
  let p = ok(await call('project_init', { idea: '狐狸送一颗迷路的星星回家', name: '回家的星星', generation_language: 'zh-Hans' }));
  identity = await status();
  assert.equal((await call('chapters', { action: 'create', chapter_code: 'vol01.ch01', title: '回家', expected_revision: p.project.revision })).error.code, 'project_setup_incomplete');
  let setup = ok(await call('project_setup', { action: 'get' }));
  setup = ok(await call('project_setup', { action: 'update', expected_revision: setup.revision, project_name: '回家的星星', generation_language: 'zh-Hans', overall_style: '温暖水彩', generation_brief: '三页儿童绘本，主题是勇气与互助。', picture_book: { format: 'classic_picture_book', aspect_ratio: { mode: 'landscape' }, large_image_minimal_text: false } }));
  assert.equal(ok(await call('project_setup', { action: 'finalize', expected_revision: setup.revision })).setup_status, 'ready');
  const catalog = ok(await call('prompt_catalog', { prompt_group: 'story', limit: 100 }));
  assert.ok(catalog.items.some(x => x.prompt_key === 'story_profile'));
  const rendered = ok(await call('render_prompt', { prompt_group: 'story', prompt_key: 'story_profile', target: 'story_profile', variables: { chapter_count: '1' } }));
  assert.ok(rendered.prompt.includes('expected_revision'));
  assert.ok(rendered.prompt.includes(rendered.target_uuid));
  assert.equal(rendered.write_back.tool, 'story_profile');
  ok(await call('story_profile', { action: 'write', story_md: fixed.story, expected_revision: rendered.expected_revision }));
  assert.equal(ok(await call('story_profile', { action: 'get' })).story_md, fixed.story);
  p = ok(await call('project_overview'));
  let chapter = ok(await call('chapters', { action: 'create', chapter_code: 'vol01.ch01', title: '回家', expected_revision: p.project.revision }));
  const chapterPrompt = ok(await call('render_prompt', { prompt_group: 'story', prompt_key: 'story_chapter', target: 'chapter', chapter_uuid: chapter.uuid, variables: { chapter_plan_json: '{"title":"回家"}', generated_summaries_json: '[]' } }));
  chapter = ok(await call('chapters', { action: 'write', chapter_uuid: chapter.uuid, content: fixed.chapter, content_format: 'txt', expected_revision: chapterPrompt.expected_revision }));
  assert.equal(ok(await call('chapters', { action: 'get', chapter_uuid: chapter.uuid })).current_story.content, fixed.chapter);
  const settings = ok(await call('settings', { action: 'list' }));
  const settingPrompt = ok(await call('render_prompt', { prompt_group: 'premise', prompt_key: 'text_settings', target: 'premise' }));
  assert.equal(settingPrompt.target_uuid, settings.premise.uuid);
  assert.ok(settingPrompt.prompt.includes(fixed.story.trim()));
  const setting = ok(await call('settings', { action: 'create', asset_type: 'character', title: '阿栗', summary: fixed.setting, tags: ['狐狸', '主角'], expected_revision: settingPrompt.expected_revision }));
  assert.equal(ok(await call('settings', { action: 'get', setting_uuid: setting.uuid })).summary, fixed.setting);
  const pagesPrompt = ok(await call('render_prompt', { prompt_group: 'chapter', prompt_key: 'comic_storyboard', target: 'pages', chapter_uuid: chapter.uuid,
    variables: { moment_count_plan: '[1,1,1]', max_section_count: '3', max_moments_per_section: '1' } }));
  assert.equal(pagesPrompt.write_back.tool, 'pages');
  const pages = [];
  for (const [title, description_md, storyboard_md] of fixed.pages) {
    const state = ok(await call('pages', { action: 'list', chapter_uuid: chapter.uuid }));
    pages.push(ok(await call('pages', { action: 'create', chapter_uuid: chapter.uuid, title, description_md, storyboard_md, expected_revision: state.state.revision })));
  }
  for (const [index, page] of pages.entries()) {
    const read = ok(await call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: page.uuid }));
    assert.equal(read.description_md, fixed.pages[index][1]);
    assert.equal(read.current_storyboard.content_md, fixed.pages[index][2]);
  }
  // Two tasks both read revision N. The stale task must not replace task one's text.
  const before = ok(await call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: pages[0].uuid }, 'second-task'));
  ok(await call('pages', { action: 'update', chapter_uuid: chapter.uuid, page_uuid: before.uuid, title: '溪边的微光（已审阅）', expected_revision: before.revision }));
  const conflict = await call('pages', { action: 'update', chapter_uuid: chapter.uuid, page_uuid: before.uuid, description_md: '不得覆盖的过期草稿', expected_revision: before.revision }, 'second-task');
  assert.equal(conflict.error.code, 'production_conflict');
  assert.equal(ok(await call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: before.uuid })).description_md, fixed.pages[0][1]);
  ok(await call('project_init', { idea: '另一部作品' }, 'other', other));
  const crossed = await call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: before.uuid }, 'other', other);
  assert.equal(crossed.success, false);
  const foreignContext = await call('render_prompt', { prompt_group: 'story', prompt_key: 'story_profile', target: 'story_profile',
    variables: { chapter_count: '1' }, context_chapter_uuids: [chapter.uuid] }, 'other', other);
  assert.equal(foreignContext.success, false, 'foreign context UUID must be rejected, not silently filtered');
  for (const [name, args] of [
    ['project_overview', {}], ['project_setup', { action: 'get' }], ['story_profile', { action: 'get' }],
    ['chapters', { action: 'list' }], ['settings', { action: 'get', setting_uuid: setting.uuid }],
    ['pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: before.uuid }],
    ['book_outline', {}], ['prompt_catalog', { prompt_group: 'story' }],
    ['render_prompt', { prompt_group: 'story', prompt_key: 'story_profile', target: 'story_profile', variables: { chapter_count: '1' } }],
  ]) ok(await call(name, args, 'readonly-reader', root, true));
  for (const [name, args] of [
    ['project_init', { idea: 'no' }], ['project_setup', { action: 'finalize', expected_revision: 0 }],
    ['pages', { action: 'select_image', chapter_uuid: chapter.uuid, page_uuid: before.uuid, variant_uuid: before.uuid, expected_revision: 0 }],
    ['export_book', { format: 'markdown', allow_missing_images: true }],
  ]) await assert.rejects(call(name, args, 'readonly', root, true), { code: 'read_only_context' });
  const exported = ok(await call('export_book', { format: 'markdown', allow_missing_images: true }));
  assert.equal(exported.file_generated, true);
  assert.equal(exported.user_received, false);
  assert.equal(exported.delivery.status, 'prepared');
  assert.equal(path.isAbsolute(exported.path), false);
  const bytes = await fs.readFile(path.join(root, exported.path)), text = bytes.toString('utf8');
  assert.equal(bytes.length, exported.byte_size);
  for (const s of [fixed.story.trim(), fixed.chapter, fixed.setting, ...fixed.pages.flatMap(p => p.slice(1))]) assert.ok(text.includes(s), `export missing ${s}`);
  assert.equal((text.match(/### Page /g) || []).length, 3);
  assert.equal(text.includes('不得覆盖的过期草稿'), false);
  assert.deepEqual(await fs.readdir(home), [], 'no provider configuration or engine guidance');
  if (process.env.LUMI_CINDY_EVIDENCE_DIR) {
    const evidence = path.resolve(process.env.LUMI_CINDY_EVIDENCE_DIR);
    await fs.mkdir(evidence, { recursive: true });
    await fs.writeFile(path.join(evidence, 'fixed-book.md'), bytes);
    await fs.writeFile(path.join(evidence, 'fixed-book-receipt.json'), JSON.stringify(exported, null, 2) + '\n');
  }
  console.log(JSON.stringify({ kind: 'fixed-output-not-autonomous-agent', pages: 3, prose_and_visuals_verified: true, bytes: bytes.length, sha256: crypto.createHash('sha256').update(bytes).digest('hex'), delivery: 'prepared_not_received' }));
  // A delivery-preparation failure must keep the known generated file, report
  // that phase precisely and never replay export to try to repair delivery.
  await fs.rename(path.join(root, 'lumi-exports'), path.join(root, 'saved-exports'));
  await fs.writeFile(path.join(root, 'lumi-exports'), 'blocked directory');
  const filesBefore = await fs.readdir(path.join(root, '.lumi/exports'));
  let generated;
  await assert.rejects(call('export_book', { format: 'markdown', allow_missing_images: true }), error => {
    generated = error.correction?.generated_export;
    return error.code === 'export_delivery_failed' && generated?.file_generated === true && generated.user_received === false;
  });
  assert.equal((await fs.readdir(path.join(root, '.lumi/exports'))).length, filesBefore.length + 1);
  assert.ok((await fs.readFile(path.join(root, generated.project_relative_path), 'utf8')).includes(fixed.pages[2][2]));
  await fs.unlink(path.join(root, 'lumi-exports'));
  await fs.rename(path.join(root, 'saved-exports'), path.join(root, 'lumi-exports'));
  // Exercise real MCP size limits and cursor recovery through the Cindy
  // dispatcher. Every recovered item retains the exact full text and marker.
  const longText = 'x'.repeat(100000) + '\nCOMPLETE-LONG-END';
  const longUUIDs = new Set();
  for (let i = 2; i <= 4; i++) {
    p = ok(await call('project_overview'));
    longUUIDs.add(ok(await call('chapters', { action: 'create', chapter_code: `vol01.ch0${i}`, title: `长章 ${i}`,
      content: longText, expected_revision: p.project.revision })).uuid);
  }
  const tooLarge = await call('chapters', { action: 'list' });
  assert.equal(tooLarge.error.code, 'result_too_large');
  assert.equal(tooLarge.data, null);
  assert.equal(tooLarge.correction.tool.name, 'chapters');
  let after;
  do {
    const page = ok(await call('chapters', { action: 'list', limit: 1, ...(after ? { after } : {}) }));
    for (const item of page.items) if (longUUIDs.has(item.uuid)) {
      assert.equal(item.current_story.content, longText); longUUIDs.delete(item.uuid);
    }
    after = page.cursor_pagination.has_more ? page.cursor_pagination.next_cursor : null;
  } while (after);
  assert.equal(longUUIDs.size, 0);
  const renderArgs = { prompt_group: 'story', prompt_key: 'story_profile', target: 'story_profile', variables: { chapter_count: '1' } };
  assert.equal((await call('render_prompt', renderArgs)).error.code, 'result_too_large');
  const narrowed = ok(await call('render_prompt', { ...renderArgs, context_chapter_uuids: [] }));
  assert.ok(narrowed.prompt.includes(narrowed.target_uuid));
  assert.equal(narrowed.prompt.includes(longText), false);
  console.log(JSON.stringify({ kind: 'real-cli-result-boundary', oversize_failed: true, cursor_full_text_recovered: 3,
    narrow_prompt_recovered: true, known_generation_delivery_failure_preserved: true }));
});
