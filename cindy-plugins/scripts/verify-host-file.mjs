// Read-only acceptance probe of the file service shipped with the executing
// Cindy. This is not packaged or exposed as a Lumi Agent tool.
import { spawn } from 'node:child_process';
import { readFile, realpath, writeFile, stat } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { createInterface } from 'node:readline';
import path from 'node:path';
import assert from 'node:assert/strict';

const [rootArg, relative, recordPath] = process.argv.slice(2);
if (!rootArg || !relative || !recordPath || path.isAbsolute(relative) || relative.split('/').some(p => p === '..')) {
  throw new Error('Usage: node verify-host-file.mjs WORKDIR RELATIVE_FILE EVIDENCE_JSON');
}
const root = await realpath(rootArg);
const resourceRoot = '/Applications/Cindy.app/Contents/Resources';
const service = path.join(resourceRoot, 'remote-file-service/file-service.mjs');
const source = JSON.parse(await readFile(path.join(resourceRoot, 'cindy-source.json'), 'utf8'));
const serviceSHA = createHash('sha256').update(await readFile(service)).digest('hex');
const expected = await readFile(path.join(root, relative));
const child = spawn(process.execPath, [service], { stdio: ['pipe', 'pipe', 'pipe'], shell: false });
const exited = new Promise(resolve => child.once('close', resolve));
const pending = new Map(); let id = 0;
child.stderr.on('data', () => {});
createInterface({ input: child.stdout }).on('line', line => {
  const frame = JSON.parse(line), entry = pending.get(frame.id);
  if (!entry) return;
  pending.delete(frame.id); clearTimeout(entry.timer);
  frame.ok ? entry.resolve(frame.result) : entry.reject(new Error(JSON.stringify(frame.error)));
});
child.once('error', error => { for (const p of pending.values()) p.reject(error); });
child.once('close', code => { for (const p of pending.values()) p.reject(new Error(`Cindy file service exited ${code}`)); });
function call(method, params) {
  return new Promise((resolve, reject) => {
    const request = ++id;
    const timer = setTimeout(() => { pending.delete(request); reject(new Error(`Timed out: ${method}`)); }, 10000);
    pending.set(request, { resolve, reject, timer });
    child.stdin.write(JSON.stringify({ id: request, method, params }) + '\n');
  });
}
try {
  const handshake = await call('handshake', { clientSchemaVersion: 3 });
  assert.equal(handshake.schemaVersion, 3);
  const parent = path.posix.dirname(relative);
  const listed = await call('listDir', { workdir: root, relPath: parent === '.' ? '' : parent, hideMetaFiles: true });
  assert.ok(listed.entries.some(e => e.relPath === relative && e.type === 'file'), 'artifact must be visible without exposing hidden metadata');
  const hash = createHash('sha256'); let bytes = 0, chunks = 0;
  for (;;) {
    const chunk = await call('readFileChunk', { workdir: root, relPath: relative, offset: bytes, length: 64 * 1024 });
    assert.equal(chunk.size, expected.length);
    const content = Buffer.from(chunk.dataBase64, 'base64');
    assert.deepEqual(content, expected.subarray(bytes, bytes + content.length));
    hash.update(content); bytes += content.length; chunks++;
    if (chunk.eof) break;
    assert.ok(content.length > 0, 'read must make progress');
  }
  assert.equal(bytes, expected.length);
  const sha256 = hash.digest('hex');
  assert.equal(sha256, createHash('sha256').update(expected).digest('hex'));
  assert.equal((await stat(path.join(root, relative))).size, bytes);
  const record = { kind: 'actual-shipped-cindy-file-service-not-phone-receipt', date: new Date().toISOString(),
    host_source: source, file_service: { schema_version: handshake.schemaVersion, bundle_version: handshake.bundleVersion, sha256: serviceSHA },
    relative_path: relative, bytes, chunks, sha256, visible: true, full_bytes_verified: true,
    user_received: false, mobile_client_verified: false };
  await writeFile(recordPath, JSON.stringify(record, null, 2) + '\n');
  console.log(JSON.stringify(record, null, 2));
} finally {
  for (const entry of pending.values()) clearTimeout(entry.timer);
  child.stdin.end();
  const timer = setTimeout(() => child.kill('SIGKILL'), 2000);
  await exited; clearTimeout(timer);
}
