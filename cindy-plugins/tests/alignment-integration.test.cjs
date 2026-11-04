'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { Runtime, runCLI } = require('../lumi/node/runtime.cjs');
const { CHUNK_BYTES } = require('../lumi/node/media.cjs');
const { WorkspaceGateway } = require('../lumi/node/workspace.cjs');
const author = path.resolve(__dirname, '../lumi');
const executable = path.join(author, 'bin/lumi');
const ok = result => { assert.equal(result.success, true, JSON.stringify(result)); return result.data; };

// Fixed geometric artwork, not a model/recognition test: red rectangle on the
// left, blue rectangle on the right, with white margins for deterministic crops.
const { boardPNG } = require('./fixtures/board.cjs');

test('Cindy board attachment → atomic two-setting crops → reviewed references → three pages and Markdown/ZIP/PDF', { timeout: 120000 }, async t => {
  const base = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-cindy-alignment-'));
  const root = path.join(base, 'book'), other = path.join(base, 'other'), home = path.join(base, 'home'), dataDir = path.join(base, 'data');
  for (const dir of [root, other, home, dataDir]) await fs.mkdir(dir);
  const env = { HOME: home, PATH: '/usr/bin:/bin', TMPDIR: os.tmpdir() };
  const build = JSON.parse(await fs.readFile(path.join(author, 'build-info.json')));
  let runtime = new Runtime({ executable, build, dataDir, env });
  const context = (session = 'book-task', workdir = root) => ({ session_id: session, workdir, workdir_is_local: true, workdir_is_read_only: false });
  const call = (name, args = {}, ctx = context()) => runtime.execute('call_tool', { context: ctx, arguments: { name, args } });
  const status = () => runCLI(executable, ['status', '--data-dir', dataDir, '--json'], { cwd: base, env });
  let nonce;
  t.after(async () => {
    await runtime.close();
    const current = await status();
    if (current.running) {
      assert.equal(current.nonce, nonce, 'never stop another instance');
      assert.equal((await fetch(current.endpoint + '/api/v1/cli-instance', { method: 'DELETE', headers: { 'If-Match': nonce } })).status, 200);
      for (let i = 0; i < 200; i++) {
        try { if (!(await status()).running) break; }
        catch (error) { if (error.code !== 'instance_unavailable') throw error; }
        await new Promise(resolve => setTimeout(resolve, 25));
      }
      assert.equal((await status()).running, false);
    }
    await fs.rm(base, { recursive: true, force: true });
  });
  ok(await call('project_init', { idea: 'A red character carries a blue lantern.', name: 'Alignment fixture', generation_language: 'en' }));
  nonce = (await status()).nonce;
  let setup = ok(await call('project_setup', { action: 'get' }));
  setup = ok(await call('project_setup', { action: 'update', expected_revision: setup.revision, project_name: 'Alignment fixture', generation_language: 'en', overall_style: 'Flat red and blue geometric shapes, white margins.', generation_brief: 'Synthetic fixed-material automation, three pages.', picture_book: { format: 'classic_picture_book', aspect_ratio: { mode: 'landscape' }, large_image_minimal_text: false } }));
  ok(await call('project_setup', { action: 'finalize', expected_revision: setup.revision }));
  const profile = ok(await call('story_profile', { action: 'get' }));
  ok(await call('story_profile', { action: 'write', expected_revision: profile.revision, story_md: 'A red traveler finds a blue light, carries it home and shares it.' }));
  const project = ok(await call('project_overview'));
  let chapter = ok(await call('chapters', { action: 'create', chapter_code: 'vol01.ch01', title: 'The blue light', expected_revision: project.project.revision }));
  chapter = ok(await call('chapters', { action: 'write', chapter_uuid: chapter.uuid, expected_revision: chapter.revision, content: 'The traveler found a light. They carried it home. Together they lit the window.' }));
  const pages = [];
  for (const [index, text] of ['The traveler found a light.', 'They carried it home.', 'Together they lit the window.'].entries()) {
    const state = ok(await call('pages', { action: 'list', chapter_uuid: chapter.uuid }));
    const created = ok(await call('pages', { action: 'create', chapter_uuid: chapter.uuid, title: `Page ${index + 1}`, description_md: text, storyboard_md: `A red rectangle holds a blue lantern. White margins. Exact caption: “${text}”`, expected_revision: state.state.revision }));
    pages.push(ok(await call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: created.uuid })));
  }
  const rendered = ok(await call('render_prompt', { prompt_group: 'premise', prompt_key: 'setting_image', target: 'setting_board', variables: { input_text: 'Red traveler and blue lantern' }, context_chapter_uuids: [], context_setting_uuids: [] }));
  assert.equal(rendered.write_back.tool, 'receive_media');
  assert.equal(rendered.write_back.slot, 'setting_board');
  assert.equal(rendered.write_back.required_from_host.includes('path'), false);
  assert.ok(rendered.prompt.includes('"tool": "receive_media"'));
  const bytes = boardPNG();
  let invocation = 0;
  const receive = async (input, ctx = context()) => {
    const params = { context: ctx, invocation: `attachment-${++invocation}` };
    const transfer = ok(await runtime.execute('media_begin', { ...params, input, bytes: bytes.length, sha256: crypto.createHash('sha256').update(bytes).digest('hex'), mime: 'image/png' })).transfer;
    for (let pos = 0, index = 0; pos < bytes.length; pos += CHUNK_BYTES, index++) ok(await runtime.execute('media_chunk', { ...params, transfer, index, base64: bytes.subarray(pos, pos + CHUNK_BYTES).toString('base64') }));
    return runtime.execute('media_commit', { ...params, transfer });
  };
  const boardInput = { slot: 'setting_board', source_text: rendered.write_back.source_text, style_snapshot: rendered.write_back.style_snapshot, prompt: rendered.generation_prompt, expected_revision: rendered.write_back.expected_revision, idempotency_key: 'board-a' };
  const board = ok(await receive(boardInput));
  assert.equal(ok(await receive(boardInput)).uuid, board.uuid, 'replay uses stable original path across transfers');
  await runtime.close();
  runtime = new Runtime({ executable, build, dataDir, env });
  assert.equal(ok(await receive(boardInput)).uuid, board.uuid, 'worker recovery retains the original idempotent import identity');
  assert.equal((await receive({ ...boardInput, source_text: 'Different request with the same key' })).success, false);
  assert.equal(ok(await call('setting_boards', { action: 'list' })).items.length, 1);
  const source = ok(await call('setting_boards', { action: 'get', board_uuid: board.uuid }));
  assert.ok((await call('read_image', { asset_uuid: source.setting_image.asset.uuid })).transport_image);
  const analysis = ok(await call('render_prompt', { prompt_group: 'premise', prompt_key: 'asset_breakdown', target: 'setting_breakdown', board_uuid: board.uuid }));
  const split = { board_uuid: board.uuid, expected_revision: analysis.write_back.expected_revision, expected_premise_revision: analysis.write_back.expected_premise_revision, idempotency_key: 'split-a', assets: [
    { type: 'character', title: 'Traveler', summary: 'A flat red rectangle.', tags: ['red'], confidence: 1, crop_box: { x: 0, y: 0, width: 0.5, height: 1 } },
    { type: 'prop', title: 'Lantern', summary: 'A flat blue rectangle.', tags: ['blue'], confidence: 1, crop_box: { x: 0.5, y: 0, width: 0.5, height: 1 } },
  ] };
  const overlapping = structuredClone(split); overlapping.idempotency_key = 'invalid-overlap'; overlapping.assets[1].crop_box.x = 0.25;
  assert.equal((await call('apply_setting_breakdown', overlapping)).success, false);
  assert.equal(ok(await call('settings', { action: 'list' })).items.length, 0, 'failed batch has no partial settings');
  const applied = ok(await call('apply_setting_breakdown', split));
  assert.equal(applied.items.length, 2);
  assert.equal(ok(await call('apply_setting_breakdown', split)).uuid, applied.uuid);
  assert.equal(ok(await call('settings', { action: 'list' })).items.length, 2);
  const selection = ok(await call('settings', { action: 'list' })).items.map(item => item.uuid);
  let firstPreparation;
  for (const [index, page] of pages.entries()) {
    const readiness = ok(await call('image_target', { target_uuid: page.uuid, slot: 'page_image', setting_uuids: selection })).setting_readiness;
    assert.equal(readiness.status, 'review_required');
    assert.equal((await call('prepare_page_image', { page_uuid: page.uuid, setting_uuids: selection })).error.code, 'setting_image_review_required');
    const reviews = readiness.items.map(item => ({ setting_uuid: item.setting_uuid, variant_uuid: item.variant_uuid, expected_revision: item.expected_revision, style_digest: readiness.style_digest, verdict: 'passed', reference_basis: 'AUTOMATED MOCK: deterministic red/blue geometric fixture.', comparison: 'MOCK assessment: flat rectangle, intact margins, fixed geometry; not real model visual acceptance.' }));
    const referenceInput = { page_uuid: page.uuid, setting_uuids: selection, setting_reviews: reviews };
    const params = { context: context(), invocation: `page-reference-${index}`, input: referenceInput };
    const referenceResult = await runtime.execute('reference_prepare', params);
    const prepared = ok(referenceResult), transfer = referenceResult.transport_reference;
    assert.equal(prepared.reference_mode, 'collage');
    assert.equal(prepared.sources.length, 2);
    assert.deepEqual(prepared.preparation.setting_reviews, reviews);
    const original = await fs.readFile(prepared.reference_image_path), chunks = [];
    assert.ok(original.length > 0);
    assert.equal(transfer.sha256, crypto.createHash('sha256').update(original).digest('hex'));
    await assert.rejects(runtime.execute('reference_chunk', { context: context('another-task'), invocation: params.invocation, transfer: transfer.transfer, index: 0 }), { code: 'reference_transfer_invalid' });
    await assert.rejects(runtime.execute('reference_chunk', { context: context(), invocation: 'different-host-call', transfer: transfer.transfer, index: 0 }), { code: 'reference_transfer_invalid' });
    for (let offset = 0, chunkIndex = 0; offset < original.length; offset += transfer.chunk_bytes, chunkIndex++) {
      const chunk = ok(await runtime.execute('reference_chunk', { context: context(), invocation: params.invocation, transfer: transfer.transfer, index: chunkIndex }));
      chunks.push(Buffer.from(chunk.base64, 'base64'));
    }
    assert.deepEqual(Buffer.concat(chunks), original, 'Core transport uses the exact two-setting Go collage, never previews');
    ok(await runtime.execute('reference_release', { context: context(), invocation: params.invocation, transfer: transfer.transfer }));
    await assert.rejects(runtime.execute('reference_chunk', { context: context(), invocation: params.invocation, transfer: transfer.transfer, index: 0 }), { code: 'reference_transfer_invalid' });
    firstPreparation ||= prepared;
    ok(await receive({ ...prepared.image_write_back, prompt: prepared.generation_prompt, preparation: prepared.preparation, fit_mode: 'keep' }));
    const saved = ok(await call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: page.uuid, include_versions: true }));
    assert.ok(saved.current_image.asset.uuid);
    assert.equal(saved.description_md, page.description_md);
    assert.equal(saved.image_versions.items.length, 1);
    pages[index] = saved;
  }
  // An already used preparation is stale, and does not create another version.
  assert.equal((await receive({ ...firstPreparation.image_write_back, prompt: firstPreparation.generation_prompt, preparation: firstPreparation.preparation })).success, false);
  assert.equal(ok(await call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: pages[0].uuid, include_versions: true })).image_versions.items.length, 1);
  for (const downgrade of [false, true]) {
    const params = { context: context(), invocation: `revoke-reference-${downgrade}`, input: { page_uuid: pages[0].uuid, setting_uuids: selection, setting_reviews: firstPreparation.preparation.setting_reviews } };
    const reference = await runtime.execute('reference_prepare', params); ok(reference);
    const chunk = { context: context(), invocation: params.invocation, transfer: reference.transport_reference.transfer, index: 0 };
    if (downgrade) await assert.rejects(runtime.execute('reference_chunk', { ...chunk, context: { ...context(), workdir_is_read_only: true } }), { code: 'binding_changed' });
    else ok(await call('runtime_release'));
    assert.equal(runtime.references.items.size, 0, 'release/permission change revokes prepared references');
    await assert.rejects(runtime.execute('reference_chunk', chunk), { code: 'reference_transfer_invalid' });
  }
  const lifecycle = new AbortController(), sender = {};
  const grant = { context: context(), project_uuid: project.project.uuid, signal: lifecycle.signal };
  const gateway = new WorkspaceGateway({ runtime, sender, authority: { resolve: value => value === sender ? grant : null } });
  t.after(() => gateway.close());
  const app = async (name, args) => ok(await gateway.call(name, args));
  const resource = `chapters/${chapter.uuid}/comic-sections/${pages[0].uuid}`;
  const current = await app('read_app_resource', { resource });
  const saved = await app('write_app_resource', { resource, method: 'PATCH', body: { title: 'Workspace saved title', expected_revision: current.revision } });
  assert.equal(saved.title, 'Workspace saved title');
  assert.equal((await gateway.call('write_app_resource', { resource, method: 'PATCH', body: { title: 'Stale overwrite', expected_revision: current.revision } })).success, false);
  const scripts = await app('read_app_resource', { resource: `${resource}/storyboard-variants` });
  assert.ok(scripts.items.length);
  const beforePreview = await app('read_app_resource', { resource });
  await app('read_app_resource', { resource: `${resource}/image-variants` });
  assert.equal((await app('read_app_resource', { resource })).revision, beforePreview.revision, 'preview reads never adopt a version');
  const scripted = await app('write_app_resource', { resource: `${resource}/storyboard-variants`, method: 'POST', body: { content_md: 'A new complete workspace script.', source_type: 'manual', expected_revision: beforePreview.revision } });
  const adopted = await app('write_app_resource', { resource: `${resource}/storyboard-variants/${scripts.items[0].uuid}/selections`, method: 'POST', body: { expected_revision: scripted.revision } });
  assert.equal(adopted.current_storyboard.content_md, scripts.items[0].content_md);
  assert.equal(adopted.current_storyboard.source_type, 'restore');
  assert.notEqual(adopted.current_storyboard.uuid, scripts.items[0].uuid, 'script adoption preserves immutable history with a new restoration version');
  const images = await app('read_app_resource', { resource: `${resource}/image-variants` });
  const imageAdoption = await app('write_app_resource', { resource: `${resource}/image-variants/${images.items[0].uuid}/selections`, method: 'POST', body: { expected_revision: adopted.revision } });
  assert.equal(imageAdoption.current_image.uuid, images.items[0].uuid);
  await app('write_app_resource', { resource: `chapters/${chapter.uuid}/comic-section-order`, method: 'PUT', body: { section_uuids: pages.map(page => page.uuid).reverse() } });
  assert.deepEqual((await app('read_app_resource', { resource: `chapters/${chapter.uuid}/comic-sections` })).items.map(page => page.uuid), pages.map(page => page.uuid).reverse());
  await app('write_app_resource', { resource: `chapters/${chapter.uuid}/comic-section-order`, method: 'PUT', body: { section_uuids: pages.map(page => page.uuid) } });
  const beforeDelete = await app('read_app_resource', { resource });
  await app('write_app_resource', { resource, method: 'DELETE', query: { expected_revision: String(beforeDelete.revision) } });
  const deleted = (await app('read_app_resource', { resource: `chapters/${chapter.uuid}/comic-sections`, query: { state: 'trashed' } })).items.find(item => item.uuid === pages[0].uuid);
  await app('write_app_resource', { resource: `${resource}/restorations`, method: 'POST', body: { expected_revision: deleted.revision } });
  const subscription = await app('watch_app_changes', { action: 'open' });
  const next = gateway.call('watch_app_changes', { action: 'next', subscription_uuid: subscription.subscription_uuid, cursor: subscription.cursor });
  const latest = await app('read_app_resource', { resource });
  await app('write_app_resource', { resource, method: 'PATCH', body: { title: 'WS updated title', expected_revision: latest.revision } });
  const hint = ok(await next);
  assert.equal(hint.state, 'changed');
  assert.deepEqual(Object.keys(hint).sort(), ['cursor', 'event', 'project_uuid', 'state', 'subscription_uuid']);
  assert.equal((await app('read_app_resource', { resource })).title, 'WS updated title', 'hint triggers REST fact reread');
  await app('watch_app_changes', { action: 'close', subscription_uuid: subscription.subscription_uuid });
  ok(await call('project_init', { idea: 'Isolated second project.' }, context('other-task', other)));
  for (const [name, args] of [
    ['setting_boards', { action: 'get', board_uuid: board.uuid }],
    ['read_image', { asset_uuid: source.setting_image.asset.uuid }],
    ['settings', { action: 'get', setting_uuid: selection[0] }],
  ]) assert.equal((await call(name, args, context('other-task', other))).success, false);
  await assert.rejects(receive(boardInput, { ...context(), workdir_is_read_only: true }), { code: 'read_only_context' });
  for (const format of ['markdown', 'zip', 'pdf']) {
    const receipt = ok(await call('export_book', { format, allow_missing_images: false }));
    const content = await fs.readFile(path.join(root, receipt.path));
    assert.equal(content.length, receipt.byte_size);
    assert.equal(receipt.user_received, false);
    if (format === 'markdown') for (const page of pages) assert.ok(content.toString().includes(page.description_md));
    else assert.equal(content.subarray(0, format === 'pdf' ? 4 : 2).toString(), format === 'pdf' ? '%PDF' : 'PK');
  }
  console.log('ALIGNMENT FIXTURE PASS: 1 resumable board, 2 atomic crops, signed mock reviews, 3 complete pages, Markdown/ZIP/PDF, cross-project refusal; no model calls');
});
