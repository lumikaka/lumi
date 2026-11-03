'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { Runtime, runCLI } = require('../lumi/node/runtime.cjs');
const { CHUNK_BYTES, mimeFor } = require('../lumi/node/media.cjs');
const author = path.resolve(__dirname, '../lumi'), executable = path.join(author, 'bin/lumi');
const ok = result => { assert.equal(result.success, true, JSON.stringify(result)); return result.data; };

test('fixture original → bounded transfer → real Asset Store → preview, variants, conflict and PDF/ZIP; ordinary MCP stays compatible', { timeout: 120000 }, async t => {
  const base = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-cindy-media-e2e-'));
  const root = path.join(base, 'book'), other = path.join(base, 'other'), home = path.join(base, 'home'), dataDir = path.join(base, 'data');
  for (const dir of [root, other, home, dataDir]) await fs.mkdir(dir);
  const env = { HOME: home, PATH: '/usr/bin:/bin', TMPDIR: os.tmpdir() };
  const build = JSON.parse(await fs.readFile(path.join(author, 'build-info.json')));
  const runtime = new Runtime({ executable, build, dataDir, env });
  const context = (session = 'author', workdir = root) => ({ session_id: session, workdir, workdir_is_local: true, workdir_is_read_only: false });
  const call = (name, args = {}, ctx = context()) => runtime.execute('call_tool', { context: ctx, arguments: { name, args } });
  const media = (method, input, ctx = context(), invocation = 'authorized-original-call', signal) => runtime.execute(method, { context: ctx, invocation, ...input }, signal);
  const status = () => runCLI(executable, ['status', '--data-dir', dataDir, '--json'], { cwd: base, env });
  let nonce;
  t.after(async () => {
    await runtime.close();
    const current = await status();
    if (current.running) {
      assert.equal(current.nonce, nonce);
      assert.equal((await fetch(current.endpoint + '/api/v1/cli-instance', { method: 'DELETE', headers: { 'If-Match': current.nonce } })).status, 200);
      for (let i = 0; i < 200; i++) {
        try { if (!(await status()).running) break; } catch (error) { if (error.code !== 'instance_unavailable') throw error; }
        await new Promise(resolve => setTimeout(resolve, 25));
      }
      assert.equal((await status()).running, false);
    }
    await fs.rm(base, { recursive: true, force: true });
  });
  ok(await call('project_init', { idea: 'A fox finds a star', name: 'Media fixture', generation_language: 'en' }));
  nonce = (await status()).nonce;
  let setup = ok(await call('project_setup', { action: 'get' }));
  setup = ok(await call('project_setup', { action: 'update', expected_revision: setup.revision, project_name: 'Media fixture', generation_language: 'en', overall_style: 'watercolor', generation_brief: 'A one-page fixture.', picture_book: { format: 'classic_picture_book', aspect_ratio: { mode: 'landscape' }, large_image_minimal_text: false } }));
  ok(await call('project_setup', { action: 'finalize', expected_revision: setup.revision }));
  const project = ok(await call('project_overview'));
  const chapter = ok(await call('chapters', { action: 'create', chapter_code: 'vol01.ch01', title: 'The star', expected_revision: project.project.revision }));
  const state = ok(await call('pages', { action: 'list', chapter_uuid: chapter.uuid }));
  let page = ok(await call('pages', { action: 'create', chapter_uuid: chapter.uuid, title: 'A light', description_md: 'The fox held a warm star.', storyboard_md: 'A fox in a blue scarf holds a star beside a stream.', expected_revision: state.state.revision }));
  const settingState = ok(await call('settings', { action: 'list' }));
  const setting = ok(await call('settings', { action: 'create', asset_type: 'character', title: 'Fox', summary: 'Orange fox, blue scarf.', expected_revision: settingState.premise.revision }));
  const bytes = await fs.readFile(path.resolve(__dirname, '../../docs/features/cindy-plugins/spike/media/core-fox.png'));
  assert.ok(bytes.length > CHUNK_BYTES, 'real multichunk fixture');
  const sha256 = crypto.createHash('sha256').update(bytes).digest('hex');
  const input = () => ({ target_uuid: page.uuid, slot: 'page_image', expected_revision: page.revision, prompt: 'Fixture fox and star', fit_mode: 'keep' });
  const begin = async (args = input(), content = bytes, ctx = context(), invocation) => {
    if (args.slot === 'page_image' && !args.preparation) {
      const prepared = ok(await call('prepare_page_image', { page_uuid: args.target_uuid, setting_uuids: [], additional_direction: args.prompt || '' }, ctx));
      args = { ...args, prompt: prepared.generation_prompt, preparation: prepared.preparation };
    }
    return ok(await media('media_begin', { input: args, bytes: content.length, sha256: crypto.createHash('sha256').update(content).digest('hex'), mime: mimeFor(content) }, ctx, invocation)).transfer;
  };
  const chunks = async (transfer, content = bytes, ctx = context(), invocation) => {
    for (let pos = 0, index = 0; pos < content.length; pos += CHUNK_BYTES, index++) ok(await media('media_chunk', { transfer, index, base64: content.subarray(pos, pos + CHUNK_BYTES).toString('base64') }, ctx, invocation));
  };
  const transfer = await begin();
  await assert.rejects(media('media_chunk', { transfer, index: 0, base64: bytes.subarray(0, CHUNK_BYTES).toString('base64') }, context('other-task')), { code: 'media_transfer_invalid' });
  await assert.rejects(media('media_chunk', { transfer, index: 0, base64: bytes.subarray(0, CHUNK_BYTES).toString('base64') }, context(), 'different-call'), { code: 'media_transfer_invalid' });
  await chunks(transfer);
  const imported = ok(await media('media_commit', { transfer }));
  assert.equal(imported.source_sha256, sha256); assert.equal(imported.source_bytes, bytes.length);
  const stored = await fetch((await status()).endpoint + imported.asset.content_url);
  assert.equal(stored.status, 200);
  assert.equal(crypto.createHash('sha256').update(Buffer.from(await stored.arrayBuffer())).digest('hex'), sha256, 'Asset Store has the exact original');
  assert.equal(JSON.stringify(imported).includes(runtime.media.root), false);
  const asset = imported.asset.uuid;
  page = ok(await call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: page.uuid }));
  assert.equal(page.current_image.asset.uuid, asset);
  const preview = await call('read_image', { asset_uuid: asset });
  assert.equal(preview.data.asset_uuid, asset); assert.ok(preview.data.width <= 1024 && preview.data.height <= 1024);
  assert.ok(Buffer.from(preview.transport_image.data, 'base64').length <= 120 * 1024);
  // The former Codex MCP call still returns ImageContent; no Cindy context required.
  const peer = [...runtime.connections.values()][0].peer;
  const raw = await peer.request('tools/call', { name: 'read_image', arguments: { asset_uuid: asset } });
  assert.equal(raw.content.filter(c => c.type === 'image').length, 1);
  assert.equal(JSON.stringify(raw.structuredContent).includes('image_data'), false);
  // A target edit between begin and commit must preserve the user's latest page.
  const stale = await begin(); await chunks(stale);
  page = ok(await call('pages', { action: 'update', chapter_uuid: chapter.uuid, page_uuid: page.uuid, title: 'User edit', expected_revision: page.revision }));
  const conflict = await media('media_commit', { transfer: stale });
  assert.equal(conflict.error.code, 'production_conflict');
  const recovery = conflict.correction.recovery_file;
  assert.equal(recovery.sha256, sha256); assert.equal(path.isAbsolute(recovery.relative_path), false);
  assert.deepEqual(await fs.readFile(path.join(root, recovery.relative_path)), bytes);
  assert.equal(ok(await call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: page.uuid })).title, 'User edit');
  assert.deepEqual(await fs.readdir(runtime.media.root), []);
  // Cropping produces a derived Asset while preserving the original in history.
  const cropped = await begin({ ...input(), fit_mode: 'crop' }); await chunks(cropped);
  const cropResult = ok(await media('media_commit', { transfer: cropped }));
  assert.equal(cropResult.fit_mode, 'crop'); assert.equal(cropResult.version_no, 2);
  page = ok(await call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: page.uuid }));
  assert.equal(page.current_image.uuid, cropResult.variant_uuid);
  const candidates = await peer.call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: page.uuid, include_versions: true });
  assert.equal(ok(candidates).image_versions.items.length, 2);
  page = ok(await call('pages', { action: 'select_image', chapter_uuid: chapter.uuid, page_uuid: page.uuid, variant_uuid: imported.variant_uuid, expected_revision: page.revision }));
  assert.equal(page.current_image.uuid, imported.variant_uuid);
  const settingTransfer = await begin({ target_uuid: setting.uuid, slot: 'setting_image', expected_revision: setting.revision, fit_mode: 'pad' }); await chunks(settingTransfer);
  assert.equal(ok(await media('media_commit', { transfer: settingTransfer })).fit_mode, 'pad');
  for (const content of [Buffer.from(preview.transport_image.data, 'base64'), await fs.readFile(path.resolve(__dirname, '../../root-site/assets/lumi-preview.webp'))]) {
    const currentSetting = ok(await call('settings', { action: 'get', setting_uuid: setting.uuid }));
    const id = await begin({ target_uuid: setting.uuid, slot: 'setting_image', expected_revision: currentSetting.revision }, content);
    await chunks(id, content);
    assert.equal(ok(await media('media_commit', { transfer: id })).source_mime, mimeFor(content));
  }
  const damaged = bytes.subarray(0, 50), broken = await begin(input(), damaged); await chunks(broken, damaged);
  assert.equal((await media('media_commit', { transfer: broken })).success, false, 'full Go decoder rejects truncated PNG');
  ok(await call('project_init', { idea: 'Different project' }, context('other-project', other)));
  assert.equal((await call('image_target', { target_uuid: page.uuid, slot: 'page_image' }, context('other-project', other))).success, false);
  assert.equal((await call('read_image', { asset_uuid: asset }, context('other-project', other))).success, false);
  await assert.rejects(call('receive_media', input()), { code: 'media_handoff_required' });
  await assert.rejects(media('media_begin', { input: input(), bytes: bytes.length, sha256, mime: 'image/png' }, { ...context('readonly'), workdir_is_read_only: true }), { code: 'read_only_context' });
  const interrupted = await begin();
  await assert.rejects(media('media_chunk', { transfer: interrupted, index: 0, base64: bytes.subarray(0, CHUNK_BYTES).toString('base64') }, context(), undefined, AbortSignal.abort()), { code: 'request_cancelled' });
  assert.equal(runtime.media.items.size, 0);
  for (const format of ['pdf', 'zip']) {
    const file = ok(await call('export_book', { format, allow_missing_images: false }));
    const output = await fs.readFile(path.join(root, file.path));
    assert.equal(output.length, file.byte_size);
    assert.equal(output.subarray(0, format === 'pdf' ? 4 : 2).toString(), format === 'pdf' ? '%PDF' : 'PK');
  }
  console.log(`MEDIA FIXTURE PASS: original ${bytes.length} bytes, SHA256 ${sha256}; bounded preview; 2 variants; PDF/ZIP; isolated CLI cleanup`);
});
