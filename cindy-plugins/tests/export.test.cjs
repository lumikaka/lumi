'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const { prepareExport, exportIdentity, DIRECTORY } = require('../lumi/node/export.cjs');
const uuid = '01989abc-def0-7000-8000-000000000001';
async function fixture(t, format = 'markdown') {
  const root = await fs.realpath(await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-export-delivery-')));
  t.after(() => fs.rm(root, { recursive: true, force: true }));
  await fs.mkdir(path.join(root, '.lumi', 'exports'), { recursive: true });
  const bytes = Buffer.from(('完整正文与画面描述\n尾部标记。\n').repeat(12000));
  const ext = { markdown: 'md', zip: 'zip', pdf: 'pdf' }[format];
  const value = { uuid, format, path: path.join(root, '.lumi', 'exports', `text-${uuid}.${ext}`), byte_size: bytes.length, missing_image_count: 3 };
  await fs.writeFile(value.path, bytes);
  return { root, bytes, value, target: path.join(root, DIRECTORY, `text-${uuid}.${ext}`) };
}
test('complete bytes are published atomically in the visible task tree; preparation is not receipt', async t => {
  const { root, bytes, value, target } = await fixture(t);
  const result = await prepareExport(root, value);
  assert.deepEqual(await fs.readFile(target), bytes);
  assert.equal(result.path, path.posix.join(DIRECTORY, `text-${uuid}.md`));
  assert.equal(result.delivery.sha256, crypto.createHash('sha256').update(bytes).digest('hex'));
  assert.equal(result.delivery.byte_size, bytes.length);
  assert.equal(result.file_generated, true);
  assert.equal(result.user_received, false);
  assert.equal(result.delivery.status, 'prepared');
  assert.equal(result.delivery.receipt, null);
  assert.equal(JSON.stringify(result).includes(root), false);
  assert.deepEqual(await fs.readdir(path.dirname(target)), [path.basename(target)]);
});
test('only the exact CLI export path, format and size are accepted', async t => {
  const { root, value, target } = await fixture(t);
  for (const patch of [{ path: '/etc/passwd' }, { path: path.join(root, 'other.md') }, { uuid: '1' }, { format: 'toString' }, { byte_size: -1 }]) {
    assert.throws(() => exportIdentity(root, { ...value, ...patch }), { code: 'protocol_error' });
  }
  await assert.rejects(prepareExport(root, { ...value, byte_size: value.byte_size + 1 }));
  await assert.rejects(fs.stat(target), { code: 'ENOENT' });
});
test('source and destination links cannot broaden the export path', async t => {
  const { root, value } = await fixture(t);
  const outside = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-export-outside-'));
  t.after(() => fs.rm(outside, { recursive: true, force: true }));
  await fs.symlink(outside, path.join(root, DIRECTORY));
  await assert.rejects(prepareExport(root, value));
  assert.deepEqual(await fs.readdir(outside), []);
  await fs.unlink(path.join(root, DIRECTORY));
  const secret = path.join(outside, 'secret');
  await fs.writeFile(secret, 'unrelated bytes');
  await fs.unlink(value.path); await fs.symlink(secret, value.path);
  await assert.rejects(prepareExport(root, { ...value, byte_size: 15 }));
  assert.equal(await fs.readFile(secret, 'utf8'), 'unrelated bytes');
  assert.deepEqual(await fs.readdir(path.join(root, DIRECTORY)), []);
});
test('an existing visible file is never overwritten and temporary files are removed', async t => {
  const { root, value, target } = await fixture(t);
  await fs.mkdir(path.join(root, DIRECTORY));
  await fs.writeFile(target, 'user file');
  await assert.rejects(prepareExport(root, value), { code: 'EEXIST' });
  assert.equal(await fs.readFile(target, 'utf8'), 'user file');
  assert.deepEqual(await fs.readdir(path.dirname(target)), [path.basename(target)]);
});
test('cancelled preparation does not publish an incomplete work', async t => {
  const { root, value, target } = await fixture(t);
  await assert.rejects(prepareExport(root, value, AbortSignal.abort()), { code: 'request_cancelled' });
  await assert.rejects(fs.stat(target), { code: 'ENOENT' });
});
