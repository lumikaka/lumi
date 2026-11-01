'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const { PassThrough } = require('node:stream');
const { spawn } = require('node:child_process');
const path = require('node:path');
const { createWorker } = require('../lumi/node/worker-transport.cjs');
const { RuntimeError } = require('../lumi/node/errors.cjs');
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
async function until(predicate) {
  for (let i = 0; i < 200; i++) { if (predicate()) return; await pause(5); }
  throw new Error('no response');
}
function harness(t, runtime, options = {}) {
  const input = new PassThrough(), output = new PassThrough(), results = [], children = [];
  output.on('data', bytes => { for (const line of bytes.toString().trim().split('\n')) results.push(JSON.parse(line)); });
  const worker = createWorker({ input, output, runtime, spawnLifetime: options.noLifetime ? undefined : () => {
    const child = new EventEmitter();
    child.kill = () => { child.killed = true; child.emit('exit'); };
    children.push(child); return Promise.resolve(child);
  } });
  t.after(() => worker.close());
  return { results, children, input, send(id, method = 'runtime_status', params = {}, cindy = { cancelWithCall: true }) {
    input.write(JSON.stringify({ jsonrpc: '2.0', id, method, params, cindy }) + '\n');
  } };
}
test('Host sentinel exit cancels exactly its call while other calls finish', async t => {
  const completed = [], waiting = [];
  const runtime = { close: async () => {}, execute(method, params, signal) {
    return new Promise((resolve, reject) => {
      signal.addEventListener('abort', () => reject(new RuntimeError('request_cancelled', 'cancelled')), { once: true });
      waiting.push(() => { completed.push(params.task); resolve({ success: true, data: params.task }); });
    });
  } };
  const h = harness(t, runtime);
  h.send('1', 'runtime_status', { task: 'a' }); h.send('2', 'runtime_status', { task: 'b' });
  await until(() => waiting.length === 2);
  h.children[0].emit('exit'); waiting[1]();
  await until(() => h.results.length === 2);
  assert.equal(h.results.find(r => r.id === '1').result.error.code, 'request_cancelled');
  assert.equal(h.results.find(r => r.id === '2').result.data, 'b');
  assert.deepEqual(completed, ['b']);
  assert.equal(h.children.every(child => child.killed), true);
});
test('missing call binding fails before dispatch; malformed JSON can recover', async t => {
  let executions = 0;
  const h = harness(t, { close: async () => {}, async execute() { executions++; return { success: true, data: null }; } });
  h.input.write('bad\n'); h.send('1', 'runtime_status', {}, {});
  await until(() => h.results.length === 2);
  assert.equal(h.results[0].error.code, -32700);
  assert.equal(h.results[1].result.error.code, 'cancellation_unavailable');
  assert.equal(executions, 0);
  h.send('2'); await until(() => h.results.length === 3);
  assert.equal(h.results[2].result.success, true);
});
test('limits reject oversize params and fifth pending call without spawning extra children', async t => {
  const runtime = { close: async () => {}, execute(method, params, signal) {
    return new Promise((resolve, reject) => signal.addEventListener('abort', () => reject(new RuntimeError('request_cancelled', 'cancelled')), { once: true }));
  } };
  const h = harness(t, runtime);
  h.send('large', 'runtime_status', { x: 'x'.repeat(260 * 1024) });
  for (let i = 0; i < 5; i++) h.send(String(i));
  await until(() => h.results.length === 2);
  assert.equal(h.results[0].result.error.code, 'arguments_too_large');
  assert.equal(h.results[1].result.error.code, 'runtime_busy');
  assert.equal(h.children.length, 4);
  for (const child of h.children) child.emit('exit');
  await until(() => h.results.length === 6);
});

test('Host-style require(entry) starts worker even though require.main is bootstrap', async t => {
  const script = `
    const { EventEmitter } = require('node:events');
    globalThis.__CINDY_NODE__ = { spawnEntry() {
      const child = new EventEmitter(); child.kill = () => child.emit('exit');
      return Promise.resolve(child);
    } };
    require(process.argv[1]);
  `;
  const child = spawn(process.execPath, ['-e', script, path.resolve(__dirname, '../lumi/node/worker.cjs')], { stdio: ['pipe', 'pipe', 'pipe'] });
  t.after(() => child.kill());
  const result = new Promise((resolve, reject) => {
    child.stdout.once('data', bytes => resolve(JSON.parse(bytes.toString())));
    child.once('exit', code => reject(new Error(`worker exited before response: ${code}`)));
    child.stderr.once('data', bytes => reject(new Error(bytes.toString())));
  });
  // Release without prior binding touches no user's data-dir or project.
  child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id: '1', method: 'runtime_release',
    params: { context: { session_id: 'load-only-test', workdir: __dirname, workdir_is_local: true, workdir_is_read_only: true } },
    cindy: { cancelWithCall: true } }) + '\n');
  assert.equal((await result).result.data.released, true);
  child.stdin.end();
});
