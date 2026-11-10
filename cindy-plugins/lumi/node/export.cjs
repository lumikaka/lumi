'use strict';
const fs = require('node:fs/promises');
const path = require('node:path');
const crypto = require('node:crypto');
const { RuntimeError, aborted } = require('./errors.cjs');
const { UUID } = require('./project.cjs');

const DIRECTORY = 'lumi-exports';
function exportIdentity(root, value, kind = 'text') {
  const ext = { markdown: 'md', zip: 'zip', pdf: 'pdf' }[value?.format];
  const filename = `${kind}-${value?.uuid}.${ext}`;
  if ((kind === 'website' && (value?.format !== 'zip' || !/^[a-f0-9]{64}$/.test(value.sha256))) || !UUID.test(value?.uuid) || typeof ext !== 'string' || !Number.isSafeInteger(value.byte_size) || value.byte_size < 0
    || value.path !== path.join(root, '.lumi', 'exports', filename)) {
    throw new RuntimeError('protocol_error', '导出回执不符合当前项目的固定文件位置。', '重新读取当前项目；不接受任意来源文件。');
  }
  return { ext, source: value.path, relative: path.posix.join(DIRECTORY, filename) };
}
async function directoryIdentity(dir) {
  const stat = await fs.lstat(dir);
  if (!stat.isDirectory() || await fs.realpath(dir) !== dir) throw new Error('Unsafe export directory');
  return stat;
}
const same = (a, b) => a.dev === b.dev && a.ino === b.ino;

// Only copies the exact successful CLI export into the bound task's visible
// file tree. Neither paths nor filenames are accepted as Agent arguments.
// Native Cindy file browsing transports these bytes; this is preparation,
// never a claim that a phone or user has acknowledged receipt.
async function prepareExport(root, value, signal, kind = 'text') {
  const { source, relative } = exportIdentity(root, value, kind);
  aborted(signal);
  await directoryIdentity(root);
  await directoryIdentity(path.join(root, '.lumi'));
  await directoryIdentity(path.dirname(source));
  const destDir = path.join(root, DIRECTORY);
  try { await fs.mkdir(destDir, { mode: 0o700 }); }
  catch (error) { if (error.code !== 'EEXIST') throw error; }
  const dir = await directoryIdentity(destDir);
  const target = path.join(root, relative), part = path.join(destDir, `.text-${value.uuid}.${crypto.randomBytes(8).toString('hex')}.part`);
  let input, output, ownedPart;
  try {
    input = await fs.open(source, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW);
    const original = await input.stat();
    if (!original.isFile() || original.size !== value.byte_size) throw new Error('Export size mismatch');
    output = await fs.open(part, fs.constants.O_WRONLY | fs.constants.O_CREAT | fs.constants.O_EXCL | fs.constants.O_NOFOLLOW, 0o600);
    ownedPart = await output.stat();
    const buffer = Buffer.alloc(64 * 1024), hash = crypto.createHash('sha256');
    let bytes = 0;
    for (;;) {
      aborted(signal);
      const { bytesRead } = await input.read(buffer, 0, buffer.length, null);
      if (!bytesRead) break;
      bytes += bytesRead;
      if (bytes > value.byte_size) throw new Error('Export grew during copy');
      hash.update(buffer.subarray(0, bytesRead));
      for (let offset = 0; offset < bytesRead;) {
        const { bytesWritten } = await output.write(buffer, offset, bytesRead - offset, null);
        if (!bytesWritten) throw new Error('Short export write');
        offset += bytesWritten;
      }
    }
    const current = await input.stat();
    if (bytes !== value.byte_size || !same(current, original) || current.size !== original.size
      || current.mtimeMs !== original.mtimeMs || current.ctimeMs !== original.ctimeMs) throw new Error('Export changed during copy');
    await output.sync();
    await output.close(); output = null;
    aborted(signal);
    if (!same(await directoryIdentity(destDir), dir)) throw new Error('Export directory replaced');
    // link is an atomic, no-overwrite publication of the fully written file.
    const digest = hash.digest('hex');
    if (kind === 'website' && digest !== value.sha256) throw new Error('Archive changed before delivery');
    let publishedIdentity = ownedPart;
    try { await fs.link(part, target); }
    catch (error) {
      // A replay of a completed website may already be visible. Never
      // overwrite a user file or accept a receipt on filename alone.
      if (kind !== 'website' || error.code !== 'EEXIST') throw error;
      const existing = await fs.open(target, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW);
      try {
        publishedIdentity = await existing.stat();
        if (!publishedIdentity.isFile() || publishedIdentity.size !== bytes) throw error;
        const existingHash = crypto.createHash('sha256');
        for await (const chunk of existing.createReadStream({autoClose: false, signal})) existingHash.update(chunk);
        if (existingHash.digest('hex') !== digest) throw error;
      } finally { await existing.close(); }
    }
    if (!same(await directoryIdentity(destDir), dir)) throw new Error('Export directory replaced');
    const published = await fs.lstat(target);
    if (!same(published, publishedIdentity) || published.size !== bytes) throw new Error('Published export changed');
    const directory = await fs.open(destDir, 'r');
    try { await directory.sync(); } finally { await directory.close(); }
    return { ...value, path: relative, file_generated: true, user_received: false,
      delivery: { status: 'prepared', channel: 'cindy_task_files', relative_path: relative,
        byte_size: bytes, sha256: digest, receipt: null,
        instructions: '在 Cindy 当前任务的文件列表打开此相对路径；手机通过原生任务文件浏览器读取。prepared 仅表示已准备，收到须以控制端读取回执或用户确认为准，不承诺本地链接可下载。' } };
  } finally {
    await output?.close();
    await input?.close();
    const current = await fs.lstat(part).catch(() => null);
    if (current && ownedPart && same(current, ownedPart)) await fs.unlink(part);
  }
}

module.exports = { prepareExport, exportIdentity, DIRECTORY };
