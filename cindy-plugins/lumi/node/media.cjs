'use strict';
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const { RuntimeError, aborted } = require('./errors.cjs');

const MAX_BYTES = 20 * 1024 * 1024, CHUNK_BYTES = 48 * 1024;
const MAX_TRANSFERS = 2, MAX_RESERVED = 40 * 1024 * 1024;
const fail = (code, message) => { throw new RuntimeError(code, message, '保留当前作品；重读目标后用原授权附件重新导入，不自动重新生图。'); };
const mimeFor = bytes => {
  if (bytes.subarray(0, 8).equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]))) return 'image/png';
  if (bytes[0] === 255 && bytes[1] === 216 && bytes[2] === 255) return 'image/jpeg';
  if (bytes.toString('ascii', 0, 4) === 'RIFF' && bytes.toString('ascii', 8, 12) === 'WEBP') return 'image/webp';
  return null;
};

async function prepareRecovery(root, source, input) {
  // A conflict must not leave the only original in a short-lived Host cache.
  // This is a task-visible immutable file, not an Asset Store/index write.
  const bytes = await fs.readFile(source);
  const sha256 = crypto.createHash('sha256').update(bytes).digest('hex');
  const mime = mimeFor(bytes), ext = { 'image/png': 'png', 'image/jpeg': 'jpg', 'image/webp': 'webp' }[mime];
  if (!ext || bytes.length > MAX_BYTES) fail('media_invalid', '恢复图片无效。');
  const dir = path.join(root, 'lumi-media-recovery');
  await fs.mkdir(dir, { mode: 0o700 }).catch(error => { if (error.code !== 'EEXIST') throw error; });
  const stat = await fs.lstat(dir);
  if (!stat.isDirectory() || stat.isSymbolicLink()) fail('media_recovery_unsafe', '恢复目录不是普通目录。');
  const file = path.join(dir, sha256 + '.' + ext);
  let handle;
  try {
    handle = await fs.open(file, fs.constants.O_WRONLY | fs.constants.O_CREAT | fs.constants.O_EXCL | fs.constants.O_NOFOLLOW, 0o600);
    await handle.writeFile(bytes); await handle.sync();
  } catch (error) {
    if (error.code !== 'EEXIST') throw error;
    const saved = await fs.open(file, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW);
    try {
      const existing = await saved.stat();
      if (!existing.isFile() || existing.size !== bytes.length || crypto.createHash('sha256').update(await saved.readFile()).digest('hex') !== sha256) fail('media_recovery_unsafe', '已有恢复文件与原图不一致。');
    } finally { await saved.close(); }
  } finally { await handle?.close(); }
  const after = await fs.lstat(dir);
  if (!after.isDirectory() || after.isSymbolicLink() || after.dev !== stat.dev || after.ino !== stat.ino) fail('binding_changed', '恢复目录在写入期间变化。');
  return { relative_path: 'lumi-media-recovery/' + path.basename(file), sha256, byte_size: bytes.length,
    target_uuid: input.target_uuid, slot: input.slot, file_generated: true, user_received: false,
    instructions: '原图已保留在当前任务文件列表。重读目标，确认后将这个已知文件作为顶层 attachments 再交接；不要自动重生成或覆盖。' };
}

// Buffers disappear on process death. A file exists only while the existing CLI
// imports it. A later worker removes interrupted files of proven-dead workers
// in this exact plugin-instance namespace, never another live process's files.
class MediaTransfers {
  constructor({ namespace, tempDir = os.tmpdir(), idleMs = 90000, totalMs = 300000 } = {}) {
    this.root = path.join(tempDir, 'lumi-cindy-media-' + crypto.createHash('sha256').update(namespace).digest('hex').slice(0, 24));
    this.idleMs = idleMs; this.totalMs = totalMs;
    this.items = new Map(); this.reserved = 0; this.closed = false;
  }
  begin(owner, input, { bytes, sha256, mime }) {
    if (this.closed) fail('runtime_closed', '媒体运行时已释放。');
    if (!Number.isSafeInteger(bytes) || bytes < 12 || bytes > MAX_BYTES
      || !/^[a-f0-9]{64}$/.test(sha256) || !['image/png', 'image/jpeg', 'image/webp'].includes(mime)) fail('media_invalid', '图片长度、摘要或 MIME 无效。');
    if (this.items.size >= MAX_TRANSFERS || this.reserved + bytes > MAX_RESERVED) fail('media_busy', '最多同时接收两张图片，总计 40 MiB。');
    const transfer = crypto.randomUUID();
    const item = { owner, input: structuredClone(input), bytes, sha256, mime, chunks: [], received: 0, created: Date.now(), committing: false };
    this.items.set(transfer, item); this.reserved += bytes; this.touch(transfer, item);
    return { transfer, chunk_bytes: CHUNK_BYTES, max_bytes: MAX_BYTES };
  }
  touch(transfer, item) {
    clearTimeout(item.timer);
    item.timer = setTimeout(() => this.remove(transfer), Math.min(this.idleMs, Math.max(1, item.created + this.totalMs - Date.now())));
    item.timer.unref();
  }
  get(owner, transfer) {
    const item = this.items.get(transfer);
    if (!item || item.owner !== owner || item.committing || Date.now() >= item.created + this.totalMs) fail('media_transfer_invalid', '媒体传输已失效或不属于当前调用。');
    this.touch(transfer, item); return item;
  }
  chunk(owner, { transfer, index, base64 }) {
    const item = this.get(owner, transfer);
    if (!Number.isSafeInteger(index) || index !== item.chunks.length || typeof base64 !== 'string'
      || !base64.length || base64.length > CHUNK_BYTES * 4 / 3 || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(base64)) {
      fail('media_chunk_invalid', '图片块序号或编码无效；不接受重复或乱序块。');
    }
    const bytes = Buffer.from(base64, 'base64');
    if (!bytes.length || bytes.toString('base64') !== base64 || item.received + bytes.length > item.bytes) fail('media_chunk_invalid', '图片块长度超出声明。');
    item.chunks.push(bytes); item.received += bytes.length;
    return { received: item.received, next_index: item.chunks.length };
  }
  remove(transfer) {
    const item = this.items.get(transfer);
    if (!item) return;
    clearTimeout(item.timer); this.items.delete(transfer); this.reserved -= item.bytes;
    item.chunks = [];
  }
  revoke(owner) { for (const [id, item] of this.items) if (item.owner === owner && !item.committing) this.remove(id); }
  abort(owner, transfer) { this.get(owner, transfer); this.remove(transfer); return { aborted: true }; }

  async prepareRoot() {
    await fs.mkdir(this.root, { mode: 0o700, recursive: true });
    const root = await fs.lstat(this.root);
    if (!root.isDirectory() || root.isSymbolicLink() || (root.mode & 0o077) !== 0
      || (process.getuid && root.uid !== process.getuid())) fail('media_staging_unsafe', '媒体暂存目录不安全。');
    for (const entry of await fs.readdir(this.root, { withFileTypes: true })) {
      if (!entry.isDirectory() || !/^import-[A-Za-z0-9]+$/.test(entry.name)) continue;
      const dir = path.join(this.root, entry.name);
      try {
        const marker = path.join(dir, 'owner.json'), stat = await fs.lstat(marker);
        if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 1024) continue;
        const owner = JSON.parse(await fs.readFile(marker, 'utf8'));
        if (owner.version !== 1 || !Number.isSafeInteger(owner.pid) || owner.pid <= 0) continue;
        try { process.kill(owner.pid, 0); }
        catch (error) { if (error.code === 'ESRCH') await fs.rm(dir, { recursive: true, force: true }); }
      } catch { /* Unknown files are not evidence of ownership. */ }
    }
  }
  async commit(owner, transfer, importFile, signal) {
    const item = this.get(owner, transfer);
    if (item.received !== item.bytes) fail('media_incomplete', '图片字节尚未完整接收。');
    item.committing = true; clearTimeout(item.timer);
    let dir;
    try {
      const bytes = Buffer.concat(item.chunks, item.bytes);
      if (crypto.createHash('sha256').update(bytes).digest('hex') !== item.sha256) fail('media_checksum_mismatch', '图片摘要不匹配。');
      if (mimeFor(bytes) !== item.mime) fail('media_format_mismatch', '图片真实格式与 MIME 不一致。');
      aborted(signal);
      await this.prepareRoot();
      dir = await fs.mkdtemp(path.join(this.root, 'import-'));
      await fs.writeFile(path.join(dir, 'owner.json'), JSON.stringify({ version: 1, pid: process.pid }), { mode: 0o600, flag: 'wx' });
      const file = path.join(dir, 'source.' + { 'image/png': 'png', 'image/jpeg': 'jpg', 'image/webp': 'webp' }[item.mime]);
      await fs.writeFile(file, bytes, { mode: 0o600, flag: 'wx' });
      aborted(signal);
      // Go performs full decoding, 40-million-pixel and project/revision checks.
      const result = await importFile(file, item.input);
      return result.success ? { ...result, data: { ...result.data, source_sha256: item.sha256, source_bytes: item.bytes, source_mime: item.mime } } : result;
    } finally {
      this.remove(transfer);
      if (dir) await fs.rm(dir, { recursive: true, force: true });
    }
  }
  close() { this.closed = true; for (const [id, item] of this.items) if (!item.committing) this.remove(id); }
}
module.exports = { MediaTransfers, prepareRecovery, MAX_BYTES, CHUNK_BYTES, MAX_TRANSFERS, MAX_RESERVED, mimeFor };
