'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const source = fs.readFileSync(path.join(__dirname, '../lumi/main.js'), 'utf8');
const context = { session_id: 'test-session', workdir: '/temporary/中文 project', workdir_is_local: true, workdir_is_read_only: false };
function host(response = { ok: true, result: { success: true, data: {} } }) {
  const calls = [], replies = [];
  let callback;
  vm.runInNewContext(source, { cindy: {
    onHostMessage(fn) { callback = fn; }, node: { async request(args) { calls.push(args); return response; } },
    async send(reply) { replies.push(reply); },
  } });
  return { calls, replies, invoke: msg => callback(msg) };
}
const message = (tool = 'runtime_status', ctx = context, extra = {}) => ({ type: 'tool-call', callId: 'current-host-call', tool, args: { session_context: ctx, ...extra } });

test('browser forwards real callId and only the injected context, with scoped cancellation', async () => {
  const h = host(); await h.invoke(message());
  assert.equal(h.calls[0].callId, 'current-host-call');
  assert.equal(h.calls[0].cancelWithCall, true);
  assert.deepEqual(JSON.parse(JSON.stringify(h.calls[0].params)), { context });
});
test('missing, remote, read-only and nested forged arguments cannot open a project', async () => {
  for (const msg of [message('project_overview', null), message('project_overview', { ...context, workdir_is_local: false }), message('project_overview', { ...context, workdir_is_read_only: true }), message('project_overview', context, { args: { session_context: context }, root_path: '/elsewhere' })]) {
    const h = host(); await h.invoke(msg);
    assert.equal(h.calls.length, 0);
    assert.equal(h.replies[0].result.success, false);
  }
});
test('only explicit tools may wake worker; host failure stays failure', async () => {
  const h = host({ ok: false, message: '/private/error', code: 'TIMEOUT' });
  await h.invoke({ type: 'event', name: 'focus' });
  assert.equal(h.calls.length, 0);
  await h.invoke(message());
  assert.equal(h.replies[0].result.success, false);
  assert.equal(JSON.stringify(h.replies).includes('/private/error'), false);
});
