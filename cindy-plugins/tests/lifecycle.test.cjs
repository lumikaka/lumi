'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { Runtime } = require('../lumi/node/runtime.cjs');
const { RuntimeError } = require('../lumi/node/errors.cjs');
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
async function until(predicate) {
  for (let i = 0; i < 200; i++) { if (predicate()) return; await pause(5); }
  throw new Error('condition not reached');
}
const deferred = () => { let resolve; const promise = new Promise(r => { resolve = r; }); return { promise, resolve }; };
const identity = { running: true, nonce: 'a'.repeat(64), pid: 1, endpoint: 'http://127.0.0.1:10000', environment: 'production', creation_protocol_version: 1 };
const uuid = '01989abc-def0-7000-8000-000000000001';
const overview = () => ({ success: true, data: { project: { uuid, name: 'test', revision: 0 },
  setup: { project_uuid: uuid, revision: 0, setup_status: 'draft', status: 'draft' } } });

async function fixture(t, { holdInit = false, holdCalls = false } = {}) {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-step1-lifecycle-'));
  const init = deferred(), calls = [], connections = [];
  const runtime = new Runtime({ executable: process.execPath, build: { platform: process.platform, arch: process.arch, cli_sha256: 'test-only' },
    dataDir: root, verifyBinary: false, connectionFactory() {
      const peer = { closed: false, info: {}, async initialize() { if (holdInit) await init.promise; },
        async request() { return {}; }, async close() { this.closed = true; },
        overview(signal) {
          if (!holdCalls) return Promise.resolve(overview());
          return new Promise((resolve, reject) => {
            const cancel = () => reject(new RuntimeError('request_cancelled', 'cancelled'));
            signal.addEventListener('abort', cancel, { once: true });
            calls.push(() => { signal.removeEventListener('abort', cancel); resolve(overview()); });
          });
        },
      };
      connections.push(peer); return peer;
    } });
  runtime.inspect = async () => identity; // deterministic race tests, no server.
  t.after(async () => { init.resolve(); await runtime.close(); await fs.rm(root, { recursive: true }); });
  const context = session => ({ context: { session_id: session, workdir: root, workdir_is_local: true, workdir_is_read_only: false } });
  return { runtime, context, calls, connections, init };
}

test('release revokes before awaits, drains that task and preserves another task on the shared proxy', async t => {
  const f = await fixture(t, { holdCalls: true });
  const first = f.runtime.execute('project_overview', f.context('a'));
  const rejected = assert.rejects(first, { code: 'request_cancelled' });
  const second = f.runtime.execute('project_overview', f.context('b'));
  await until(() => f.calls.length === 2);
  const released = await f.runtime.execute('runtime_release', f.context('a'));
  await rejected;
  assert.equal(released.data.released, true);
  assert.equal(f.runtime.sessions.has('a'), false);
  assert.equal(f.connections.length, 1);
  assert.equal(f.connections[0].closed, false);
  // Resolve both callbacks: the cancelled response must not resurrect its binding.
  for (const complete of f.calls) complete();
  assert.equal((await second).success, true);
  assert.equal(f.runtime.sessions.has('a'), false);
  assert.equal(f.runtime.sessions.has('b'), true);
});

test('one task cancelling during shared initialize does not cancel the other', async t => {
  const f = await fixture(t, { holdInit: true });
  const controller = new AbortController();
  const one = f.runtime.execute('project_overview', f.context('a'), controller.signal);
  const rejected = assert.rejects(one, { code: 'request_cancelled' });
  const two = f.runtime.execute('project_overview', f.context('b'));
  await until(() => [...f.runtime.connections.values()][0]?.active === 2);
  controller.abort(); await rejected;
  f.init.resolve();
  assert.equal((await two).success, true);
  assert.equal(f.connections.length, 1);
});

test('release during directory lookup cannot create a late proxy or restore a binding', async t => {
  const f = await fixture(t);
  const gate = deferred(), entered = deferred();
  const original = f.runtime.directory.bind(f.runtime);
  f.runtime.directory = async root => { entered.resolve(); await gate.promise; return original(root); };
  const call = f.runtime.execute('project_overview', f.context('a'));
  const rejected = assert.rejects(call, { code: 'request_cancelled' });
  await entered.promise;
  const release = f.runtime.execute('runtime_release', f.context('a'));
  assert.equal(f.runtime.sessions.has('a'), false);
  gate.resolve(); await release; await rejected;
  assert.equal(f.connections.length, 0);
});

test('read-only diagnostic avoids CLI status lock writes even when data-dir is inside workdir', async t => {
  const f = await fixture(t);
  f.runtime.inspect = async () => { throw new Error('status must not touch a read-only workdir'); };
  const params = f.context('readonly'); params.context.workdir_is_read_only = true;
  const result = await f.runtime.execute('runtime_status', params);
  assert.equal(result.success, true);
  assert.equal(result.data.server_running, null);
  assert.equal(result.data.server_observation, 'not_checked_read_only');
  assert.equal(result.data.server_compatible, null);
});

test('legacy shared daemon is diagnosed separately from bundled CLI and never receives business operations', async t => {
  const f = await fixture(t);
  f.runtime.inspect = async () => ({ ...identity, creation_protocol_version: undefined });
  const diagnostic = await f.runtime.execute('runtime_status', f.context('legacy'));
  assert.equal(diagnostic.data.runtime, 'upgrade_required');
  assert.equal(diagnostic.data.server_compatible, false);
  assert.equal(diagnostic.data.server_creation_protocol_version, null);
  let dispatched = 0;
  for (const peer of f.connections) {
    peer.overview = peer.call = async () => { dispatched++; return overview(); };
  }
  for (const [method, args] of [['project_init', { idea: 'must not initialize' }], ['project_overview', {}], ['project_setup', { action: 'get' }]]) {
    await assert.rejects(f.runtime.execute(method, { ...f.context('legacy'), arguments: args }), { code: 'cli_upgrade_required' });
  }
  assert.equal(dispatched, 0);
  assert.equal(f.runtime.sessions.get('legacy').projectUUID, undefined);
  assert.equal(f.connections[0].closed, false, 'diagnosis must retain the existing proxy/service');
});

test('graceful shutdown waits for media file cleanup even before an MCP call exists', async t => {
  const f = await fixture(t), gate = deferred(), entered = deferred();
  const staged = path.join(f.context('media').context.workdir, 'uncommitted-image');
  f.runtime.executeMedia = async () => {
    await fs.writeFile(staged, 'temporary bytes'); entered.resolve();
    try { await gate.promise; } finally { await fs.unlink(staged); }
  };
  const call = f.runtime.execute('media_commit', {});
  await entered.promise;
  let finished = false;
  const closing = f.runtime.close().then(() => { finished = true; });
  await pause(10);
  assert.equal(finished, false, 'shutdown must not exit during the staging phase');
  gate.resolve(); await Promise.all([call, closing]);
  assert.equal(await fs.stat(staged).then(() => true, () => false), false);
});
