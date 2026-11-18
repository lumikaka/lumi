'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { Runtime, runCLI } = require('../lumi/node/runtime.cjs');
const { CHUNK_BYTES } = require('../lumi/node/media.cjs');
const { boardPNG, pngPixels } = require('./fixtures/board.cjs');
const author = path.resolve(__dirname, '../lumi'), executable = path.join(author, 'bin/lumi');
const ok = result => { assert.equal(result.success, true, JSON.stringify(result)); return result.data; };
const hash = bytes => crypto.createHash('sha256').update(bytes).digest('hex');

test('Cindy historical rectangle crop → exact attachment → unadopted candidate → worker resume → explicit adoption', { timeout: 120000 }, async t => {
  const base = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-cindy-local-edit-'));
  const root = path.join(base, 'book'), home = path.join(base, 'home'), dataDir = path.join(base, 'data');
  for (const dir of [root, home, dataDir]) await fs.mkdir(dir);
  const env = { HOME: home, PATH: '/usr/bin:/bin', TMPDIR: os.tmpdir() };
  const build = JSON.parse(await fs.readFile(path.join(author, 'build-info.json')));
  let runtime = new Runtime({ executable, build, dataDir, env });
  const context = { session_id: 'local-edit-fixture', workdir: root, workdir_is_local: true, workdir_is_read_only: false };
  const call = (name, args = {}) => runtime.execute('call_tool', { context, arguments: { name, args } });
  const status = () => runCLI(executable, ['status', '--data-dir', dataDir, '--json'], { cwd: base, env });
  let nonce;
  t.after(async () => {
    await runtime.close();
    const current = await status();
    if (current.running) {
      assert.equal(current.nonce, nonce, 'cleanup only the isolated fixture service');
      assert.equal((await fetch(current.endpoint + '/api/v1/cli-instance', { method: 'DELETE', headers: { 'If-Match': nonce } })).status, 200);
      for (let i = 0; i < 200; i++) {
        try { if (!(await status()).running) break; } catch (error) { if (error.code !== 'instance_unavailable') throw error; }
        await new Promise(resolve => setTimeout(resolve, 25));
      }
      assert.equal((await status()).running, false);
    }
    await fs.rm(base, { recursive: true, force: true });
  });
  const receive = async (input, bytes, invocation) => {
    const media = (method, extra) => runtime.execute(method, { context, invocation, ...extra });
    const start = ok(await media('media_begin', { input, bytes: bytes.length, sha256: hash(bytes), mime: 'image/png' }));
    for (let at = 0, index = 0; at < bytes.length; at += CHUNK_BYTES, index++) {
      ok(await media('media_chunk', { transfer: start.transfer, index, base64: bytes.subarray(at, at + CHUNK_BYTES).toString('base64') }));
    }
    return media('media_commit', { transfer: start.transfer });
  };
  ok(await call('project_init', { idea: 'Isolated historical local edit fixture', generation_language: 'en' }));
  nonce = (await status()).nonce;
  let setup = ok(await call('project_setup', { action: 'get' }));
  setup = ok(await call('project_setup', { action: 'update', expected_revision: setup.revision, project_name: 'Local edit fixture', generation_language: 'en', overall_style: 'Flat geometric illustration', generation_brief: 'One page for automated protocol acceptance.', picture_book: { format: 'classic_picture_book', aspect_ratio: { mode: 'landscape' }, large_image_minimal_text: false } }));
  ok(await call('project_setup', { action: 'finalize', expected_revision: setup.revision }));
  const project = ok(await call('project_overview'));
  const chapter = ok(await call('chapters', { action: 'create', chapter_code: 'vol01.ch01', title: 'Shapes', expected_revision: project.project.revision }));
  const list = ok(await call('pages', { action: 'list', chapter_uuid: chapter.uuid }));
  const page = ok(await call('pages', { action: 'create', chapter_uuid: chapter.uuid, title: 'Rectangle identity', description_md: 'A red and blue pair.', storyboard_md: 'Red on the left; blue on the right.', expected_revision: list.state.revision }));
  const get = () => call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: page.uuid, include_versions: true }).then(ok);
  const input = prepared => ({ ...prepared.image_write_back, prompt: prepared.generation_prompt, preparation: prepared.preparation, fit_mode: 'keep' });
  const variants = [];
  for (let i = 0; i < 2; i++) {
    const prepared = ok(await call('prepare_page_image', { page_uuid: page.uuid, setting_uuids: [] }));
    assert.equal(prepared.reference_mode, 'none');
    variants.push(ok(await receive(input(prepared), boardPNG(), `whole-${i}`)));
  }
  assert.equal((await get()).current_image.uuid, variants[1].variant_uuid);
  const preparation = await runtime.execute('reference_prepare', { context, invocation: 'crop-read', input: {
    page_uuid: page.uuid, base_variant_uuid: variants[0].variant_uuid, selection: { x: .2, y: .2, width: .2, height: .3 }, additional_direction: 'Replace only the selected red area with green.',
  } });
  const prepared = ok(preparation), transport = preparation.transport_reference;
  assert.equal(prepared.reference_mode, 'local_edit');
  assert.equal(transport.kind, 'local_edit_crop');
  assert.equal(prepared.preparation.local_edit.base_version_no, 1);
  const cropBytes = [];
  for (let at = 0, index = 0; at < transport.bytes; at += CHUNK_BYTES, index++) {
    const chunk = ok(await runtime.execute('reference_chunk', { context, invocation: 'crop-read', transfer: transport.transfer, index }));
    cropBytes.push(Buffer.from(chunk.base64, 'base64'));
  }
  assert.equal(hash(Buffer.concat(cropBytes)), prepared.reference_image_sha256);
  const original = pngPixels(boardPNG()), crop = pngPixels(Buffer.concat(cropBytes));
  const edit = prepared.preparation.local_edit;
  for (let y = 0; y < crop.height; y++) for (let x = 0; x < crop.width; x++) {
    assert.deepEqual(crop.at(x, y), original.at(edit.crop_pixels.x + x, edit.crop_pixels.y + y), 'full context crop is at original resolution');
  }
  ok(await runtime.execute('reference_release', { context, invocation: 'crop-read', transfer: transport.transfer }));
  const patch = boardPNG({ width: crop.width, height: crop.height, color: [20, 220, 40] });
  const candidate = ok(await receive(input(prepared), patch, 'local-import'));
  assert.equal(candidate.adopted, false);
  assert.equal(candidate.version_no, 3);
  assert.equal((await get()).current_image.uuid, variants[1].variant_uuid);
  const stored = await fetch((await status()).endpoint + candidate.asset.content_url);
  assert.equal(stored.status, 200);
  const output = pngPixels(Buffer.from(await stored.arrayBuffer()));
  let changedInside = 0;
  const s = edit.selection_pixels;
  for (let y = 0; y < output.height; y++) for (let x = 0; x < output.width; x++) {
    const inside = x >= s.x && x < s.x + s.width && y >= s.y && y < s.y + s.height;
    if (inside) changedInside += JSON.stringify(output.at(x, y)) !== JSON.stringify(original.at(x, y));
    else assert.deepEqual(output.at(x, y), original.at(x, y), `outside pixel ${x},${y}`);
  }
  assert.ok(changedInside > 0, 'the synthetic edit actually changed selected pixels');
  // Simulate a lost import reply: reconstruct the worker, then consult SQLite
  // facts before taking any further action. No generation/import is replayed.
  await runtime.close();
  runtime = new Runtime({ executable, build, dataDir, env });
  const resumed = await get();
  assert.equal(resumed.current_image.uuid, variants[1].variant_uuid);
  assert.equal(resumed.image_versions.items.length, 3);
  assert.equal(resumed.image_versions.items.filter(v => v.uuid === candidate.variant_uuid).length, 1);
  assert.equal((await receive(input(prepared), patch, 'stale-replay')).error.code, 'production_conflict');
  assert.equal((await get()).image_versions.items.length, 3, 'stale signed import cannot duplicate the candidate');
  const adopted = ok(await call('pages', { action: 'select_image', chapter_uuid: chapter.uuid, page_uuid: page.uuid, variant_uuid: candidate.variant_uuid, expected_revision: resumed.revision }));
  assert.equal(adopted.current_image.uuid, candidate.variant_uuid);
  console.log(`LOCAL EDIT FIXTURE PASS: historical v1, unadopted v3, ${changedInside} changed inside, 0 changed outside, worker resume without duplicate import, explicit adoption; synthetic image, no model calls`);
});
