'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const { ReferenceTransfers, readReference, CHUNK_BYTES, MAX_BYTES } = require('../lumi/node/reference.cjs');

test('trusted prepared reference reader rejects paths, links, changed metadata and oversize files', async t => {
  const root = await fs.realpath(await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-reference-test-')));
  t.after(() => fs.rm(root, { recursive: true, force: true }));
  const bytes = await fs.readFile(path.resolve(__dirname, '../../docs/features/cindy-plugins/spike/media/core-fox.png'));
  const file = path.join(root, 'lumi-page-premise-1234.png'); await fs.writeFile(file, bytes);
  const info = { mime_type: 'image/png', width: bytes.readUInt32BE(16), height: bytes.readUInt32BE(20) };
  const data = { reference_mode: 'collage', reference_image_path: file, image_info: info };
  assert.deepEqual(await readReference(data, root), bytes);
  for (const modified of [
    { ...data, reference_image_path: path.join(root, 'model.png') },
    { ...data, reference_image_path: path.join(root, '..', path.basename(file)) },
    { ...data, image_info: { ...info, width: info.width + 1 } },
    { ...data, reference_mode: 'none' },
  ]) await assert.rejects(readReference(modified, root), { code: 'reference_unavailable' });
  await fs.rename(file, file + '.original'); await fs.symlink(file + '.original', file);
  await assert.rejects(readReference(data, root), { code: 'reference_unavailable' });
  await fs.unlink(file); const handle = await fs.open(file, 'w'); await handle.truncate(MAX_BYTES + 1); await handle.close();
  await assert.rejects(readReference(data, root), { code: 'reference_unavailable' });
  await assert.rejects(readReference(data, root, AbortSignal.abort()), { code: 'request_cancelled' });
});

test('reference transfers preserve exact bytes and reject impostors, replay, revocation and late completion', async t => {
  const root = await fs.realpath(await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-reference-test-')));
  t.after(() => fs.rm(root, { recursive: true, force: true }));
  const bytes = await fs.readFile(path.resolve(__dirname, '../../docs/features/cindy-plugins/spike/media/core-fox.png'));
  const file = path.join(root, 'lumi-page-premise-9876.png'); await fs.writeFile(file, bytes);
  const data = { reference_mode: 'collage', reference_image_path: file, image_info: { mime_type: 'image/png', width: bytes.readUInt32BE(16), height: bytes.readUInt32BE(20) } };
  const references = new ReferenceTransfers({ tempDir: root }), owner = {};
  t.after(() => references.close());
  const transfer = await references.begin(owner, data, undefined, async () => {});
  const wrong = { transfer: transfer.transfer, index: 0 };
  assert.throws(() => references.chunk({}, wrong), { code: 'reference_transfer_invalid' });
  const chunks = [];
  for (let offset = 0, index = 0; offset < bytes.length; offset += CHUNK_BYTES, index++) {
    chunks.push(Buffer.from(references.chunk(owner, { ...wrong, index }).base64, 'base64'));
    assert.throws(() => references.chunk(owner, { ...wrong, index }), { code: 'reference_chunk_invalid' });
  }
  assert.deepEqual(Buffer.concat(chunks), bytes);
  const second = {};
  await references.begin(second, data, undefined, async () => {});
  await assert.rejects(references.begin({}, data, undefined, async () => {}), { code: 'reference_busy' });
  references.revoke(second);
  references.revoke(owner);
  assert.throws(() => references.chunk(owner, wrong), { code: 'reference_transfer_invalid' });
  await assert.rejects(references.begin(owner, data, undefined, async () => { references.close(); }), { code: 'reference_unavailable' });
  assert.equal(references.items.size, 0);
});
