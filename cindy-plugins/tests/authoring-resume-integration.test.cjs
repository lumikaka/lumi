'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { Runtime, runCLI } = require('../lumi/node/runtime.cjs');
const { CHUNK_BYTES } = require('../lumi/node/media.cjs');
const { boardPNG } = require('./fixtures/board.cjs');
const ok = result => { assert.equal(result.success, true, JSON.stringify(result)); return result.data; };
const sha = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const readTextZIP = file => {
  const result = spawnSync('python3', ['-c', `import json, sys, zipfile
with zipfile.ZipFile(sys.argv[1]) as archive:
    print(json.dumps({"markdown": archive.read("book.md").decode(), "book": json.loads(archive.read("book.json")), "names": archive.namelist()}))
`, file], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr || result.error?.message);
  return JSON.parse(result.stdout);
};

// Synthetic images/review observations verify transport and version semantics,
// not model image quality. The real CLI and project live only in this temp tree.
test('Cindy reuses auto strip plans, versioned reviews, resume summaries and scoped exports', { timeout: 120000 }, async t => {
  const base = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-cindy-resume-'));
  const root = path.join(base, 'book'), home = path.join(base, 'home'), dataDir = path.join(base, 'data');
  const executable = path.join(base, 'lumi');
  const env = { HOME: home, PATH: '/usr/bin:/bin', TMPDIR: os.tmpdir() };
  let runtime, nonce;
  const status = () => runCLI(executable, ['status', '--data-dir', dataDir, '--json'], { cwd: base, env });
  t.after(async () => {
    await runtime?.close();
    if (nonce) {
      const current = await status();
      if (current.running) {
        assert.equal(current.nonce, nonce, 'stop only this isolated test service');
        assert.equal((await fetch(current.endpoint + '/api/v1/cli-instance', { method: 'DELETE', headers: { 'If-Match': nonce } })).status, 200);
        let stopped = false;
        for (let i = 0; i < 200; i++) {
          try { stopped = !(await status()).running; }
          catch (error) { if (error.code !== 'instance_unavailable') throw error; }
          if (stopped) break;
          await new Promise(resolve => setTimeout(resolve, 25));
        }
        assert.ok(stopped);
      }
    }
    await fs.rm(base, { recursive: true, force: true });
  });
  for (const dir of [root, home, dataDir]) await fs.mkdir(dir);
  const build = spawnSync('go', ['build', '-o', executable, './cmd/lumi'], { cwd: path.resolve(__dirname, '../..'), encoding: 'utf8', timeout: 60000 });
  assert.equal(build.status, 0, build.stderr || build.error?.message);
  const binary = await fs.readFile(executable);
  const options = { executable, build: { platform: process.platform, arch: process.arch, cli_sha256: sha(binary) }, dataDir, env };
  runtime = new Runtime(options);
  const context = { session_id: 'resume-fixture', workdir: root, workdir_is_local: true, workdir_is_read_only: false };
  const call = (name, args = {}) => runtime.execute('call_tool', { context, arguments: { name, args } });
  ok(await call('project_init', { idea: 'A synthetic strip with complete dialogue.' }));
  nonce = (await status()).nonce;
  let setup = ok(await call('project_setup', { action: 'get' }));
  setup = ok(await call('project_setup', { action: 'update', expected_revision: setup.revision, project_name: 'Strip fixture', overall_style: 'Geometric shapes', picture_book: { format: 'vertical_strip' } }));
  ok(await call('project_setup', { action: 'finalize', expected_revision: setup.revision }));
  const chapter = ok(await call('chapters', { action: 'create', chapter_code: 'vol01.ch01', title: 'Fixture', content: 'Keep all dialogue and actions.', expected_revision: ok(await call('project_overview')).project.revision }));
  const render = variables => call('render_prompt', { prompt_group: 'chapter', prompt_key: 'comic_storyboard', target: 'pages', chapter_uuid: chapter.uuid, variables });
  const planFrom = rendered => JSON.parse(rendered.prompt.match(/\n(\[[123](?:,[123])+\])\n/)[1]);
  const plan = planFrom(ok(await render({ max_section_count: '4', moment_count_plan: 'auto' })));
  assert.deepEqual([...plan.slice(0, 3)].sort(), [1, 2, 3]);
  assert.deepEqual(planFrom(ok(await render({ max_section_count: '6', moment_count_plan: 'auto' }))).slice(0, 4), plan);
  assert.ok(ok(await render({ max_section_count: '2', moment_count_plan: '[5,6]', max_moments_per_section: '6' })).prompt.includes('\n[5,6]\n'), 'explicit user rhythm remains available');
  const pages = [];
  for (const [index, count] of plan.entries()) {
    const description_md = `Complete dialogue ${index}: “Keep every word.”`;
    const storyboard_md = Array.from({ length: count }, (_, n) => `Moment ${n + 1}: complete action and exact caption.`).join('\n');
    const created = ok(await call('pages', { action: 'create', chapter_uuid: chapter.uuid, title: `Page ${index + 1}`, description_md, storyboard_md,
      expected_revision: ok(await call('pages', { action: 'list', chapter_uuid: chapter.uuid })).state.revision }));
    const saved = ok(await call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: created.uuid }));
    assert.equal(saved.description_md, description_md);
    assert.equal(saved.current_storyboard.content_md, storyboard_md);
    pages.push(saved);
  }
  const bytes = boardPNG();
  const attach = async page => {
    const prepared = ok(await call('prepare_page_image', { page_uuid: page.uuid, setting_uuids: [] }));
    const params = { context, invocation: `image-${page.uuid}` };
    const transfer = ok(await runtime.execute('media_begin', { ...params, input: { ...prepared.image_write_back, prompt: prepared.generation_prompt, preparation: prepared.preparation }, bytes: bytes.length, sha256: sha(bytes), mime: 'image/png' })).transfer;
    for (let pos = 0, index = 0; pos < bytes.length; pos += CHUNK_BYTES, index++) {
      ok(await runtime.execute('media_chunk', { ...params, transfer, index, base64: bytes.subarray(pos, pos + CHUNK_BYTES).toString('base64') }));
    }
    ok(await runtime.execute('media_commit', { ...params, transfer }));
  };
  for (const page of pages.slice(1)) await attach(page);
  const summary = page => call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: page.uuid, summary_only: true }).then(ok);
  const review = async (page, verdict, observation, extra = {}) => {
    const current = await summary(page);
    const image = await call('read_image', { asset_uuid: current.asset_uuid });
    assert.ok(image.transport_image, 'Cindy adapter returns a real controlled image for inspection');
    return call('pages', { action: 'record_image_review', chapter_uuid: chapter.uuid, page_uuid: page.uuid,
      variant_uuid: current.variant_uuid, expected_revision: current.revision, verdict, observation, ...extra });
  };
  const defect = ok(await review(pages[2], 'needs_revision', 'Synthetic defect: caption mismatch.'));
  ok(await review(pages[3], 'passed', 'Synthetic inspection: shapes and caption match.'));
  // Simulate a worker reconnect; pending artwork must survive unchanged.
  const pending = await summary(pages[1]);
  await runtime.close();
  runtime = new Runtime(options);
  const outline = ok(await call('book_outline', { chapter_uuid: chapter.uuid, limit: 2 }));
  const rest = ok(await call('book_outline', { chapter_uuid: chapter.uuid, limit: 2, after: outline.cursor_pagination.next_cursor }));
  assert.deepEqual([...outline.items, ...rest.items].map(p => p.review_status), ['missing_image', 'pending', 'needs_revision', 'passed']);
  assert.equal((await summary(pages[1])).variant_uuid, pending.variant_uuid);
  ok(await review(pages[1], 'passed', 'Synthetic inspection after reconnect.'));
  assert.equal((await summary(pages[1])).variant_uuid, pending.variant_uuid, 'review pending without regeneration');
  const priorImage = await summary(pages[1]);
  await attach(pages[1]);
  const replacement = await summary(pages[1]);
  assert.notEqual(replacement.variant_uuid, priorImage.variant_uuid);
  assert.equal(replacement.review_status, 'pending', 'a replacement image cannot inherit the prior approval');
  assert.equal((await call('pages', { action: 'record_image_review', chapter_uuid: chapter.uuid, page_uuid: pages[1].uuid,
    variant_uuid: priorImage.variant_uuid, expected_revision: replacement.revision, verdict: 'passed', observation: 'Review of the old image.' })).success, false);
  ok(await review(pages[1], 'passed', 'Synthetic inspection of the replacement image.'));
  assert.equal((await review(pages[2], 'passed', 'Corrected observation without its revision.')).success, false);
  ok(await review(pages[2], 'passed', 'Corrected synthetic observation.', { expected_review_revision: defect.review_revision }));
  const current = await summary(pages[3]);
  ok(await call('pages', { action: 'update', chapter_uuid: chapter.uuid, page_uuid: pages[3].uuid, expected_revision: current.revision, storyboard_md: 'Revised complete scene.' }));
  assert.equal((await summary(pages[3])).review_status, 'pending', 'page edits invalidate prior approval');
  assert.equal((await call('pages', { action: 'record_image_review', chapter_uuid: chapter.uuid, page_uuid: pages[3].uuid, variant_uuid: current.variant_uuid, expected_revision: current.revision, verdict: 'passed', observation: 'Stale review.' })).success, false);
  const complete = await call('export_book', { format: 'zip', chapter_uuid: chapter.uuid, allow_missing_images: false });
  assert.equal(complete.success, false, 'full illustrated export rejects missing artwork');
  const other = ok(await call('chapters', { action: 'create', chapter_code: 'vol01.ch02', title: 'Excluded chapter', content: 'MUST NOT LEAK', expected_revision: ok(await call('project_overview')).project.revision }));
  ok(await call('pages', { action: 'create', chapter_uuid: other.uuid, title: 'Other page', description_md: 'MUST NOT LEAK', storyboard_md: 'Other scene.', expected_revision: ok(await call('pages', { action: 'list', chapter_uuid: other.uuid })).state.revision }));
  const currentPages = [];
  for (const page of pages) currentPages.push(ok(await call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: page.uuid })));
  const checkText = text => {
    assert.ok(text.includes('Keep all dialogue and actions.'), 'complete chapter prose survives export');
    for (const page of currentPages) {
      assert.ok(text.includes(page.description_md), `full dialogue survives export for ${page.uuid}`);
      assert.ok(text.includes(page.current_storyboard.content_md), `current storyboard survives export for ${page.uuid}`);
    }
    assert.ok(!text.includes('MUST NOT LEAK'), 'chapter export excludes other chapters');
  };
  for (const format of ['markdown', 'zip', 'pdf']) {
    const exported = ok(await call('export_book', { format, chapter_uuid: chapter.uuid, allow_missing_images: true }));
    assert.equal(exported.scope, 'chapter');
    assert.equal(exported.chapter_uuid, chapter.uuid);
    assert.equal(exported.page_count, 4);
    assert.equal(exported.missing_image_count, 1);
    assert.equal(exported.delivery.status, 'prepared');
    assert.equal(exported.user_received, false);
    const content = await fs.readFile(path.join(root, exported.path));
    assert.equal(content.length, exported.byte_size);
    assert.equal(sha(content), exported.delivery.sha256);
    if (format === 'markdown') checkText(content.toString());
    if (format === 'zip') {
      const archive = readTextZIP(path.join(root, exported.path));
      checkText(archive.markdown);
      assert.equal(archive.book.chapter_uuid, chapter.uuid);
      assert.deepEqual(archive.book.chapters.map(item => item.chapter.uuid), [chapter.uuid]);
      assert.equal(archive.names.filter(name => name.startsWith('images/')).length, 3);
    }
    if (format === 'pdf') {
      assert.equal(content.subarray(0, 5).toString(), '%PDF-');
      assert.ok(content.includes(Buffer.from('%%EOF')));
      assert.equal((content.toString('latin1').match(/\/Type\s*\/Page(?:\s|\/)/g) || []).length, 2, 'vertical-strip PDF preserves the existing two-images-per-sheet layout');
    }
  }
  for (const format of ['markdown', 'zip']) {
    const textOnly = ok(await call('export_book', { format, chapter_uuid: other.uuid, allow_missing_images: true }));
    assert.equal(textOnly.page_count, 1);
    assert.equal(textOnly.missing_image_count, 1);
    const file = path.join(root, textOnly.path);
    const text = format === 'markdown' ? await fs.readFile(file, 'utf8') : readTextZIP(file).markdown;
    assert.ok(text.includes('MUST NOT LEAK'));
    assert.ok(text.includes('Other scene.'), 'zero-image delivery retains the storyboard');
  }
  assert.equal((await call('export_book', { format: 'pdf', chapter_uuid: other.uuid, allow_missing_images: true })).success, false, 'zero-image PDF cannot be delivered as a book');
  await attach(pages[0]);
  ok(await review(pages[0], 'passed', 'Synthetic inspection of the formerly missing image.'));
  ok(await review(pages[3], 'passed', 'Synthetic inspection against the revised scene.'));
  for (const page of pages) assert.equal((await summary(page)).review_status, 'passed');
  const finished = ok(await call('export_book', { format: 'zip', chapter_uuid: chapter.uuid, allow_missing_images: false }));
  assert.equal(finished.page_count, 4);
  assert.equal(finished.missing_image_count, 0);
  assert.equal((await call('export_book', { format: 'zip', allow_missing_images: false })).success, false, 'chapter completion does not imply whole-book completion');
  console.log('CINDY WORKFLOW FIXTURE PASS: shared auto plan, explicit rhythm, 4 resume states, versioned reviews, scoped Markdown/ZIP/PDF and zero-image text; no model calls');
});
