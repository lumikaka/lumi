'use strict';
const fs = require('node:fs/promises');
const path = require('node:path');
const crypto = require('node:crypto');
const { RuntimeError, aborted } = require('./errors.cjs');

const CHUNK_BYTES = 48 * 1024, MAX_BYTES = 20 * 1024 * 1024;
const fail = () => { throw new RuntimeError('reference_unavailable', '完整参考合图无法交接。', '未提交生图；重新 prepare_page_image，不要用缩略图或单项图片替代合图。'); };

// Only a successful Go prepare_page_image result reaches this reader. Never
// accept a model path, use a generic file reader, or expose these tokens to it.
async function readReference(data, tempDir, signal) {
  aborted(signal);
  const file = data?.reference_image_path, info = data?.image_info;
  if (data?.reference_mode !== 'collage' || typeof file !== 'string' || !path.isAbsolute(file)
    || !/^lumi-page-premise-[A-Za-z0-9]+\.png$/.test(path.basename(file))
    || !info || info.mime_type !== 'image/png' || !Number.isSafeInteger(info.width) || info.width < 1
    || !Number.isSafeInteger(info.height) || info.height < 1) fail();
  let handle;
  try {
    if (path.dirname(file) !== await fs.realpath(tempDir)) fail();
    handle = await fs.open(file, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW);
    const before = await handle.stat();
    if (!before.isFile() || before.size < 33 || before.size > MAX_BYTES
      || (process.getuid && before.uid !== process.getuid())) fail();
    // A bounded read also rejects a file that grows after stat without loading
    // arbitrarily many bytes into the worker.
    const bytes = Buffer.alloc(before.size); let offset = 0;
    while (offset < bytes.length) {
      aborted(signal);
      const read = await handle.read(bytes, offset, bytes.length - offset, offset);
      if (!read.bytesRead) fail();
      offset += read.bytesRead;
    }
    const after = await handle.stat();
    if (after.size !== before.size || after.mtimeMs !== before.mtimeMs || after.ctimeMs !== before.ctimeMs
      || !bytes.subarray(0, 8).equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]))
      || bytes.toString('ascii', 12, 16) !== 'IHDR' || bytes.readUInt32BE(8) !== 13
      || bytes.readUInt32BE(16) !== info.width || bytes.readUInt32BE(20) !== info.height) fail();
    aborted(signal);
    return bytes;
  } catch (error) {
    if (error instanceof RuntimeError) throw error;
    fail();
  } finally { await handle?.close(); }
}

class ReferenceTransfers {
  constructor({ tempDir, idleMs = 90000 } = {}) {
    this.tempDir = tempDir; this.idleMs = idleMs; this.items = new Map(); this.starting = 0; this.closed = false;
  }
  async begin(owner, data, signal, validate) {
    if (this.closed || this.items.size + this.starting >= 2) throw new RuntimeError('reference_busy', '参考合图交接并发已满。', '等待当前交接完成后重新准备；未提交生图。');
    this.starting++;
    try {
      const bytes = await readReference(data, this.tempDir, signal);
      await validate(); aborted(signal);
      if (this.closed) fail();
      const transfer = crypto.randomUUID(), sha256 = crypto.createHash('sha256').update(bytes).digest('hex');
      const item = { owner, bytes, next: 0 };
      this.items.set(transfer, item);
      item.timer = setTimeout(() => this.remove(transfer), this.idleMs); item.timer.unref();
      return { transfer, bytes: bytes.length, sha256, mime_type: 'image/png', width: data.image_info.width, height: data.image_info.height, chunk_bytes: CHUNK_BYTES };
    } finally { this.starting--; }
  }
  get(owner, transfer) {
    const item = this.items.get(transfer);
    if (!item || item.owner !== owner) throw new RuntimeError('reference_transfer_invalid', '参考交接已失效或不属于当前任务调用。', '重新准备页面参考；不接受其它调用的令牌。');
    return item;
  }
  chunk(owner, { transfer, index }) {
    const item = this.get(owner, transfer);
    if (!Number.isSafeInteger(index) || index !== item.next || index * CHUNK_BYTES >= item.bytes.length) throw new RuntimeError('reference_chunk_invalid', '参考图片块重复、乱序或越界。');
    item.next++;
    return { index, base64: item.bytes.subarray(index * CHUNK_BYTES, (index + 1) * CHUNK_BYTES).toString('base64') };
  }
  remove(transfer) { const item = this.items.get(transfer); if (item) { clearTimeout(item.timer); this.items.delete(transfer); } }
  release(owner, transfer) { this.get(owner, transfer); this.remove(transfer); return { released: true }; }
  revoke(owner) { for (const [id, item] of this.items) if (item.owner === owner) this.remove(id); }
  close() { this.closed = true; for (const id of this.items.keys()) this.remove(id); }
}
module.exports = { ReferenceTransfers, readReference, CHUNK_BYTES, MAX_BYTES };
