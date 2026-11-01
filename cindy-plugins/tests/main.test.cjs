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
const message = (name = 'runtime_status', ctx = context, extra = {}) => ({ type: 'tool-call', callId: 'current-host-call', tool: 'call_tool', args: { session_context: ctx, name, args: extra } });

test('browser forwards real callId and only the injected context, with scoped cancellation', async () => {
  const h = host(); await h.invoke(message());
  assert.equal(h.calls[0].callId, 'current-host-call');
  assert.equal(h.calls[0].cancelWithCall, true);
  assert.deepEqual(JSON.parse(JSON.stringify(h.calls[0].params)), { context, arguments: { name: 'runtime_status', args: {} } });
});
test('missing and remote context cannot enter worker', async () => {
  for (const msg of [message('project_overview', null), message('project_overview', { ...context, workdir_is_local: false })]) {
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

test('project arguments are forwarded separately; context, page and KV cannot choose bindings', async () => {
  const h = host();
  await h.invoke(message('project_init', context, { idea: '一只狐狸', name: '星星' }));
  assert.deepEqual(JSON.parse(JSON.stringify(h.calls[0].params)), { context, arguments: { name: 'project_init', args: { idea: '一只狐狸', name: '星星' } } });
  await h.invoke({ type: 'event', name: 'task-closed', session_id: context.session_id });
  await h.invoke({ type: 'panel-message', tool: 'project_init', args: { session_context: context } });
  for (const extra of [{ project_uuid: 'forged' }, { session_id: 'forged' }, { workdir: '/elsewhere' }, { _meta: {} }, { context }]) {
    await h.invoke({ ...message(), args: { session_context: context, name: 'project_init', ...extra } });
    assert.equal(h.replies.at(-1).result.error.code, 'invalid_arguments');
    assert.equal(h.replies.at(-1).result.correction.input_schema.additionalProperties, false);
  }
  assert.equal(h.calls.length, 1);
});

test('missing directory and nonlocal URI fail before entering worker', async () => {
  for (const [ctx, code] of [
    [{ ...context, workdir: '' }, 'workdir_required'],
    [{ ...context, workdir: 'ssh://host/project' }, 'workdir_invalid'],
    [{ ...context, workdir: '/same-local-name', workdir_is_local: false }, 'context_not_local'],
  ]) {
    const h = host();
    for (const tool of ['project_init', 'project_setup', 'project_overview']) {
      await h.invoke(message(tool, ctx));
      assert.equal(h.replies.at(-1).result.error.code, code);
    }
    assert.equal(h.calls.length, 0);
  }
});


test('directory discovery requires no project and failures become Host tool failures with correction', async () => {
  const result = { success: false, data: null, error: { code: 'invalid_arguments', message: '参数无效', details: '需要 revision' }, correction: { input_schema: { type: 'object' }, rules: ['重读'] } };
  const h = host({ ok: true, result });
  await h.invoke({ type: 'tool-call', callId: 'catalog', tool: 'list_tools', args: { category: 'pages' } });
  assert.equal(h.calls[0].method, 'list_tools');
  assert.equal(h.replies[0].ok, false);
  assert.equal(h.replies[0].errorCode, 'LUMI_INVALID_ARGUMENTS');
  assert.match(h.replies[0].message, /Lumi invalid_arguments/);
  assert.match(h.replies[0].message, /schema/);
});

test('reads in readonly context reach the action-aware Node gate; undeclared top-level tools do not', async () => {
  const h = host();
  await h.invoke(message('pages', { ...context, workdir_is_read_only: true }, { action: 'list' }));
  assert.equal(h.calls.length, 1);
  await h.invoke({ type: 'tool-call', callId: 'legacy', tool: 'pages', args: { session_context: context } });
  assert.equal(h.calls.length, 1);
  assert.equal(h.replies.at(-1).ok, false);
});
