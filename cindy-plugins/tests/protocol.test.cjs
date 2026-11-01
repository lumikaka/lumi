'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const { MCPConnection } = require('../lumi/node/protocol.cjs');
const fixture = path.join(__dirname, 'fixtures/mcp.cjs');
function peer(t, mode = 'normal') {
  const value = new MCPConnection({ executable: process.execPath, args: [fixture, mode], cwd: __dirname, env: process.env, timeoutMs: 2000 });
  t.after(() => value.close());
  return value;
}

test('MCP initialize, reverse request rejection, and full business failure envelope', async t => {
  const value = peer(t, 'reverse');
  assert.equal((await value.initialize()).server_name, 'lumi-text');
  assert.equal((await value.overview()).success, true);
  const failed = peer(t, 'business-error');
  await failed.initialize();
  assert.deepEqual(await failed.overview(), { success: false, data: null, error: { code: 'project_locked', message: 'locked', details: 'release owner' } });
});

test('cancel one request, preserve a concurrent request and ignore late completion', async t => {
  const value = peer(t, 'slow');
  await value.initialize();
  const controller = new AbortController();
  const first = value.overview(controller.signal);
  const second = value.overview();
  controller.abort();
  await assert.rejects(first, { code: 'request_cancelled' });
  assert.equal((await second).success, true);
  assert.deepEqual(await value.request('ping'), {});
});

test('write timeout reports unknown result and does not replay', async t => {
  const value = peer(t, 'slow');
  await value.initialize();
  const before = value.nextID;
  await assert.rejects(value.request('tools/call', { name: 'test_write' }, { timeoutMs: 10, mutation: true }), { code: 'result_unknown' });
  assert.equal(value.nextID, before + 1);
});

for (const [mode, code] of [['malformed', 'protocol_error'], ['oversize', 'result_too_large'], ['wrongid', 'protocol_error'], ['crash', 'cli_exited'], ['invalid-envelope', 'protocol_error'], ['rpc-error', 'mcp_error']]) {
  test(`reject ${mode} without converting it to business success`, async t => {
    const value = peer(t, mode);
    await value.initialize();
    await assert.rejects(value.overview(), { code });
  });
}
test('incompatible MCP and oversize params fail before a business call', async t => {
  await assert.rejects(peer(t, 'version').initialize(), { code: 'cli_incompatible' });
  const value = peer(t);
  await value.initialize();
  await assert.rejects(value.request('tools/call', { input: 'x'.repeat(250 * 1024) }), { code: 'arguments_too_large' });
  assert.equal((await value.overview()).success, true);
});
