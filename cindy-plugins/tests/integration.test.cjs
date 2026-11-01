'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const { spawn } = require('node:child_process');
const readline = require('node:readline');
const { Runtime, runCLI } = require('../lumi/node/runtime.cjs');
const { MCPConnection } = require('../lumi/node/protocol.cjs');
const author = path.resolve(__dirname, '../lumi');
const executable = path.join(author, 'bin/lumi');
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const ctx = (root, session = 'task-a', readOnly = false) => ({ context: {
  session_id: session, workdir: root, workdir_is_local: true, workdir_is_read_only: readOnly,
} });

async function until(predicate, timeout = 8000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) { if (await predicate()) return; await pause(30); }
  throw new Error('Condition did not become true');
}
function alive(pid) { try { process.kill(pid, 0); return true; } catch (error) { if (error.code === 'ESRCH') return false; throw error; } }
async function snapshot(root) {
  const result = {};
  async function visit(dir) {
    for (const item of await fs.readdir(dir, { withFileTypes: true })) {
      const file = path.join(dir, item.name);
      if (item.isDirectory()) await visit(file);
      else { const stat = await fs.stat(file); result[path.relative(root, file)] = [stat.size, stat.mtimeMs]; }
    }
  }
  await visit(root); return result;
}

test('real bundled CLI: isolation, sharing, cancellation, crashes and stale discovery', { timeout: 120000 }, async t => {
  const base = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-cindy-step1-'));
  const home = path.join(base, 'home'), dataDir = path.join(base, 'data');
  const a = path.join(base, '甲 project with spaces'), b = path.join(base, '乙 project');
  for (const dir of [home, dataDir, a, b]) await fs.mkdir(dir);
  await fs.writeFile(path.join(base, 'owner.json'), JSON.stringify({ purpose: 'lumi-cindy-step1', pid: process.pid }));
  const env = { HOME: home, PATH: '/usr/bin:/bin', TMPDIR: os.tmpdir() };
  const build = JSON.parse(await fs.readFile(path.join(author, 'build-info.json'), 'utf8'));
  const options = { executable, build, dataDir, env };
  const runtimes = new Set(), peers = new Set(), workers = new Set();
  const create = extra => { const r = new Runtime({ ...options, ...extra }); runtimes.add(r); return r; };
  const status = () => runCLI(executable, ['status', '--data-dir', dataDir, '--json'], { cwd: base, env });
  const init = (root, name) => runCLI(executable, ['init', '--name', name, '--data-dir', dataDir, '--json'], { cwd: root, env, timeoutMs: 20000 });
  const cwd = process.cwd();
  let ownedNonce;
  t.after(async () => {
    for (const worker of workers) worker.kill('SIGKILL');
    await Promise.all([...runtimes].map(runtime => runtime.close()));
    await Promise.all([...peers].map(peer => peer.close()));
    // Only this harness's isolated instance may be stopped. Production runtime
    // has no stop route. No default data-dir, cached PID or global stop is used.
    const current = await status();
    if (current.running) {
      assert.equal(current.nonce, ownedNonce);
      const response = await fetch(current.endpoint + '/api/v1/cli-instance', { method: 'DELETE', headers: { 'If-Match': current.nonce } });
      assert.equal(response.status, 200);
      await until(async () => !(await status()).running);
    }
    await fs.rm(base, { recursive: true });
    assert.equal(process.cwd(), cwd);
    console.log('CLEANUP: isolated CLI stopped, proxies closed, temporary projects removed; global cwd unchanged');
  });

  await t.test('stale discovery recovers without stopping unknown services', async () => {
    await fs.writeFile(path.join(dataDir, 'cli-production.json'), JSON.stringify({ endpoint: 'http://127.0.0.1:1', pid: 99999999, environment: 'production', nonce: 'a'.repeat(64) }));
    assert.equal((await status()).running, false);
    await init(a, 'Project A');
    const running = await status(); ownedNonce = running.nonce;
    assert.notEqual(ownedNonce, 'a'.repeat(64));
    await init(b, 'Project B');
  });

  const runtime = create();
  let uuidA, uuidB;
  await t.test('two projects truly overlap and keep cwd, project UUID and names isolated', async () => {
    const [first, second] = await Promise.all([
      runtime.execute('project_overview', ctx(a)), runtime.execute('project_overview', ctx(b, 'task-b')),
    ]);
    assert.equal(first.success, true, JSON.stringify(first));
    assert.equal(second.success, true, JSON.stringify(second));
    assert.equal(first.data.project.name, 'Project A'); assert.equal(second.data.project.name, 'Project B');
    uuidA = first.data.project.uuid; uuidB = second.data.project.uuid;
    assert.notEqual(uuidA, uuidB);
    assert.equal(runtime.connections.size, 2);
    assert.equal((await status()).nonce, ownedNonce);
    assert.equal(process.cwd(), cwd);
  });

  await t.test('same-project two tasks reuse one MCP proxy; releasing one retains the other', async () => {
    const [one, two] = await Promise.all([
      runtime.execute('project_overview', ctx(a)), runtime.execute('project_overview', ctx(a, 'task-a2')),
    ]);
    assert.equal(one.data.project.uuid, two.data.project.uuid);
    assert.equal(runtime.connections.size, 2);
    const record = [...runtime.connections.values()].find(r => r.sessions.has('task-a'));
    const pid = record.peer.child.pid;
    await runtime.execute('runtime_release', ctx(a));
    assert.equal(record.sessions.has('task-a'), false);
    assert.equal(record.sessions.has('task-a2'), true);
    assert.equal((await runtime.execute('project_overview', ctx(a, 'task-a2'))).data.project.uuid, uuidA);
    assert.equal(record.peer.child.pid, pid);
  });

  await t.test('read-only diagnosis makes no project files; overview, remote and forged args rejected', async () => {
    const before = await snapshot(a);
    const result = await runtime.execute('runtime_status', ctx(a, 'readonly', true));
    assert.equal(result.data.read_only, true);
    assert.deepEqual(await snapshot(a), before);
    await assert.rejects(runtime.execute('project_overview', ctx(a, 'readonly', true)), { code: 'read_only_context' });
    await assert.rejects(runtime.execute('runtime_status', { context: { ...ctx(a).context, workdir_is_local: false } }), { code: 'context_not_local' });
    await assert.rejects(runtime.execute('runtime_status', { ...ctx(a), root_path: b }), { code: 'invalid_arguments' });
    await assert.rejects(runtime.execute('runtime_status', {}), { code: 'invalid_arguments' });
    assert.equal(JSON.stringify(result).includes(base), false);
    assert.equal(JSON.stringify(result).includes(ownedNonce), false);
  });

  await t.test('proxy crash invalidates transport and next invocation reconnects', async () => {
    const record = [...runtime.connections.values()].find(r => r.sessions.has('task-a2'));
    const pid = record.peer.child.pid;
    record.peer.child.kill('SIGKILL');
    await record.peer.exited;
    const result = await runtime.execute('project_overview', ctx(a, 'task-a2'));
    assert.equal(result.data.project.uuid, uuidA);
    assert.notEqual([...runtime.connections.values()].find(r => r.sessions.has('task-a2')).peer.child.pid, pid);
    assert.equal((await status()).nonce, ownedNonce);
  });

  await t.test('real MCP cancellation removes only the waiting WS subscription and preserves concurrent reads', async () => {
    const record = [...runtime.connections.values()].find(r => r.sessions.has('task-a2'));
    const peer = record.peer;
    const open = await peer.request('tools/call', { name: 'watch_app_changes', arguments: { action: 'open', project_uuid: uuidA } });
    assert.equal(open.structuredContent.success, true);
    const watch = open.structuredContent.data;
    const controller = new AbortController();
    const waiting = peer.request('tools/call', { name: 'watch_app_changes', arguments: { action: 'next', project_uuid: uuidA,
      subscription_uuid: watch.subscription_uuid, cursor: watch.cursor } }, { signal: controller.signal });
    const rejection = assert.rejects(waiting, { code: 'request_cancelled' });
    const concurrent = runtime.execute('project_overview', ctx(a, 'during-cancel'));
    await pause(50); controller.abort();
    await rejection;
    assert.equal((await concurrent).data.project.uuid, uuidA);
    // A removed subscription proves the Go server received notifications/cancelled;
    // merely rejecting the JS Promise would leave it alive.
    await pause(50);
    const closed = await peer.request('tools/call', { name: 'watch_app_changes', arguments: { action: 'close', project_uuid: uuidA, subscription_uuid: watch.subscription_uuid } });
    assert.equal(closed.structuredContent.error.code, 'app_watch_expired');
    assert.equal((await peer.overview()).data.project.uuid, uuidA);
  });

  await t.test('Codex-style ordinary MCP client shares service and survives all Cindy releases', async () => {
    const codex = new MCPConnection({ executable, args: ['mcp', '--data-dir', dataDir], cwd: a, env });
    peers.add(codex);
    await codex.initialize();
    const [cindyResult, codexResult] = await Promise.all([
      runtime.execute('project_overview', ctx(a, 'cindy-share')), codex.overview(),
    ]);
    assert.equal(cindyResult.data.project.uuid, codexResult.data.project.uuid);
    const before = await status();
    await runtime.close();
    assert.equal((await codex.overview()).data.project.uuid, uuidA);
    assert.equal((await status()).nonce, before.nonce);
  });

  await t.test('worker SIGKILL closes owned proxies via pipe EOF, shared server survives, restart rebinds', async () => {
    const worker = spawn(process.execPath, [path.join(__dirname, 'fixtures/worker.cjs'), dataDir, home], { stdio: ['pipe', 'pipe', 'pipe', 'ipc'] });
    workers.add(worker);
    let proxyPID, reply;
    worker.on('message', msg => { if (msg.kind === 'proxy') proxyPID = msg.pid; });
    worker.stderr.on('data', () => {});
    readline.createInterface({ input: worker.stdout }).on('line', line => { reply = JSON.parse(line); });
    worker.stdin.write(JSON.stringify({ jsonrpc: '2.0', id: '1', method: 'project_overview', params: ctx(b, 'crashing-worker'), cindy: { cancelWithCall: true } }) + '\n');
    await until(() => reply);
    assert.equal(reply.result.data.project.uuid, uuidB);
    assert.ok(proxyPID);
    const exited = new Promise(resolve => worker.once('exit', resolve));
    worker.kill('SIGKILL'); await exited; workers.delete(worker);
    await until(() => !alive(proxyPID));
    assert.equal((await status()).nonce, ownedNonce);
    const restarted = create();
    assert.equal(restarted.sessions.size, 0);
    assert.equal((await restarted.execute('project_overview', ctx(b, 'crashing-worker'))).data.project.uuid, uuidB);
  });

  await t.test('server crash leaves stale discovery; ordinary MCP and restarted Cindy rediscover replacement', async () => {
    const before = await status();
    assert.equal(before.nonce, ownedNonce);
    process.kill(before.pid, 'SIGKILL'); // Exact isolated harness instance only.
    await until(async () => !(await status()).running);
    const restarted = create();
    const result = await restarted.execute('project_overview', ctx(a, 'after-server-crash'));
    const after = await status(); ownedNonce = after.nonce;
    assert.notEqual(after.nonce, before.nonce);
    assert.equal(result.data.project.uuid, uuidA);
    const codex = [...peers][0];
    assert.equal((await codex.overview()).data.project.uuid, uuidA);
  });

  await t.test('idle proxy reclamation and trusted invocation reopening preserve server', async () => {
    const short = create({ idleMs: 50, bindingMs: 100 });
    await short.execute('project_overview', ctx(a, 'idle'));
    const pid = [...short.connections.values()][0].peer.child.pid;
    await until(() => short.connections.size === 0 && !alive(pid));
    await until(() => short.sessions.size === 0);
    assert.equal((await status()).nonce, ownedNonce);
    assert.equal((await short.execute('project_overview', ctx(a, 'idle'))).data.project.uuid, uuidA);
  });

  await t.test('missing CLI and architecture mismatch fail closed', async () => {
    await assert.rejects(create({ executable: path.join(base, 'missing') }).execute('runtime_status', ctx(a)), { code: 'cli_missing' });
    await assert.rejects(create({ build: { ...build, arch: 'unsupported' } }).execute('runtime_status', ctx(a)), { code: 'unsupported_platform' });
  });

  await t.test('symlink rebinding cannot silently switch a live task to another project', async () => {
    const alias = path.join(base, 'alias'); await fs.symlink(a, alias);
    const bound = create();
    await bound.execute('project_overview', ctx(alias, 'rebind'));
    await fs.unlink(alias); await fs.symlink(b, alias);
    await assert.rejects(bound.execute('project_overview', ctx(alias, 'rebind')), { code: 'binding_changed' });
    assert.equal((await bound.execute('project_overview', ctx(alias, 'rebind'))).data.project.uuid, uuidB);
  });
});
