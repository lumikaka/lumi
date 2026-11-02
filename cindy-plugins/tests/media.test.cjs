'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { MediaTransfers, MAX_BYTES, CHUNK_BYTES } = require('../lumi/node/media.cjs');
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg==', 'base64');
const metadata = (bytes = png) => ({ bytes: bytes.length, sha256: crypto.createHash('sha256').update(bytes).digest('hex'), mime: 'image/png' });
async function store(t, options = {}) {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-cindy-transfers-'));
  const media = new MediaTransfers({ namespace: 'isolated-test', tempDir: dir, ...options });
  t.after(async () => { media.close(); await fs.rm(dir, { recursive: true, force: true }); });
  return media;
}
function upload(media, owner, input = {}, bytes = png, meta = metadata(bytes)) {
  const { transfer } = media.begin(owner, input, meta);
  for (let pos = 0, index = 0; pos < bytes.length; pos += CHUNK_BYTES, index++) media.chunk(owner, { transfer, index, base64: bytes.subarray(pos, pos + CHUNK_BYTES).toString('base64') });
  return transfer;
}

test('controlled staging preserves exact bytes and cleans on success, failure and cancellation', async t => {
  const media = await store(t), owner = {};
  for (const outcome of ['success', 'failure', 'throw']) {
    const id = upload(media, owner, { target_uuid: 'target' });
    const commit = media.commit(owner, id, async (file, input) => {
      assert.deepEqual(await fs.readFile(file), png); assert.equal(input.target_uuid, 'target');
      assert.equal((await fs.stat(file)).mode & 0o777, 0o600);
      if (outcome === 'throw') throw new Error('CLI interrupted');
      return outcome === 'success' ? { success: true, data: { asset: { uuid: 'saved' } } } : { success: false, data: null, error: { code: 'production_conflict' } };
    });
    if (outcome === 'throw') await assert.rejects(commit, /CLI interrupted/);
    else {
      const result = await commit;
      assert.equal(result.success, outcome === 'success');
      if (result.success) assert.equal(result.data.source_sha256, metadata().sha256);
    }
    assert.equal(media.reserved, 0); assert.equal(media.items.size, 0);
    assert.deepEqual(await fs.readdir(media.root), []);
  }
  const id = upload(media, owner);
  await assert.rejects(media.commit(owner, id, () => assert.fail('cancelled before CLI'), AbortSignal.abort()), { code: 'request_cancelled' });
  assert.equal(media.reserved, 0);
});

test('wrong owner, duplicate/out-of-order/noncanonical/oversize chunks and incomplete files cannot import', async t => {
  const media = await store(t), owner = {}, other = {};
  const { transfer } = media.begin(owner, {}, metadata());
  const chunk = { transfer, index: 0, base64: png.toString('base64') };
  assert.throws(() => media.chunk(other, chunk), { code: 'media_transfer_invalid' });
  for (const bad of [{ ...chunk, index: 1 }, { ...chunk, base64: 'a!===' }, { ...chunk, base64: 'A'.repeat(65540) }, { ...chunk, base64: Buffer.alloc(png.length + 1).toString('base64') }]) assert.throws(() => media.chunk(owner, bad), { code: 'media_chunk_invalid' });
  await assert.rejects(media.commit(owner, transfer, () => assert.fail()), { code: 'media_incomplete' });
  media.chunk(owner, chunk);
  assert.throws(() => media.chunk(owner, chunk), { code: 'media_chunk_invalid' });
  assert.throws(() => media.abort(other, transfer), { code: 'media_transfer_invalid' });
  media.abort(owner, transfer); assert.equal(media.reserved, 0);
});

test('declared size, checksum, MIME and reserved budgets are enforced before CLI', async t => {
  const media = await store(t), owner = {};
  assert.throws(() => media.begin(owner, {}, { ...metadata(), bytes: MAX_BYTES + 1 }), { code: 'media_invalid' });
  for (const [change, code] of [[{ sha256: 'a'.repeat(64) }, 'media_checksum_mismatch'], [{ mime: 'image/jpeg' }, 'media_format_mismatch']]) {
    const id = upload(media, owner, {}, png, { ...metadata(), ...change });
    await assert.rejects(media.commit(owner, id, () => assert.fail('invalid bytes reached CLI')), { code });
    assert.equal(media.reserved, 0);
  }
  media.begin(owner, {}, { ...metadata(), bytes: MAX_BYTES });
  media.begin({}, {}, { ...metadata(), bytes: MAX_BYTES });
  assert.throws(() => media.begin({}, {}, metadata()), { code: 'media_busy' });
  media.revoke(owner); assert.equal(media.reserved, MAX_BYTES);
});

test('idle expiry, release and dead-worker cleanup discard uncommitted bytes without touching live workers', async t => {
  const media = await store(t, { idleMs: 15 }), owner = {};
  const id = upload(media, owner);
  await new Promise(resolve => setTimeout(resolve, 30));
  assert.throws(() => media.get(owner, id), { code: 'media_transfer_invalid' });
  assert.equal(media.reserved, 0);
  await media.prepareRoot();
  for (const [name, pid] of [['import-dead', 2147483647], ['import-alive', process.pid]]) {
    const dir = path.join(media.root, name); await fs.mkdir(dir);
    await fs.writeFile(path.join(dir, 'owner.json'), JSON.stringify({ version: 1, pid }));
    await fs.writeFile(path.join(dir, 'source.png'), png);
  }
  await media.prepareRoot();
  assert.deepEqual(await fs.readdir(media.root), ['import-alive']);
  upload(media, owner); media.close(); assert.equal(media.reserved, 0);
  assert.throws(() => media.begin(owner, {}, metadata()), { code: 'runtime_closed' });
});
