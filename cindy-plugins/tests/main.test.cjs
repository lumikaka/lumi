'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const source = fs.readFileSync(path.join(__dirname, '../lumi/main.js'), 'utf8');
const context = { session_id: 'test-session', workdir: '/temporary/中文 project', workdir_is_local: true, workdir_is_read_only: false };
function host(response = { ok: true, result: { success: true, data: {} } }, tasks) {
  const calls = [], replies = [];
  let callback;
  vm.runInNewContext(source, { cindy: {
    onHostMessage(fn) { callback = fn; }, tasks, node: { async request(args) { calls.push(args); return typeof response === 'function' ? response(args) : response; } },
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

test('standard MCP Apps tools carry fresh Host task context; page facts stay in root UI metadata', async () => {
  const project = '01989abc-def0-7000-8000-000000000001';
  const h = host({ ok: true, result: { success: true, data: { title: 'Full private page prose', revision: 3 } } });
  await h.invoke({ type: 'tool-call', callId: 'host-card-call', tool: 'read_app_resource', args: { session_context: context, resource: 'project', expected_project_uuid: project } });
  assert.deepEqual(JSON.parse(JSON.stringify(h.calls[0].params)), { context, name: 'read_app_resource', expected_project_uuid: project, arguments: { resource: 'project' } });
  assert.equal(h.calls[0].method, 'workspace_call');
  assert.equal(h.calls[0].callId, 'host-card-call');
  assert.equal(h.calls[0].cancelWithCall, true);
  const result = h.replies[0].result;
  assert.equal(result._meta['lumi/app_response'].data.title, 'Full private page prose');
  assert.equal(JSON.stringify(result.content).includes('Full private page prose'), false);
  assert.equal(JSON.stringify(result.structuredContent).includes('Full private page prose'), false);
  await h.invoke({ type: 'tool-call', callId: 'forged-card-call', tool: 'read_app_resource', args: { session_context: context, resource: 'project', expected_project_uuid: project, workdir: '/other' } });
  assert.equal(h.calls.length, 1);
  assert.equal(h.replies.at(-1).result.isError, true);
  assert.equal(h.replies.at(-1).result._meta['lumi/app_response'].error.code, 'invalid_arguments');
});

test('text candidate launch keeps full candidates only in root metadata and opens through a declared top-level tool', async () => {
  const candidates = [{ title: 'First', text: 'Complete first candidate prose.' }, { title: 'Second', text: 'Complete second candidate prose.' }];
  const h = host({ ok: true, result: { success: true, data: { candidate_count: 2, request: { name: 'review_candidates', arguments: { kind: 'text' } } }, transport_launch_arguments: { kind: 'text', candidates } } });
  await h.invoke({ type: 'tool-call', callId: 'host-launch', tool: 'review_candidates', args: { session_context: context, kind: 'text', candidates } });
  assert.equal(h.calls[0].method, 'review_candidates');
  assert.equal(h.replies[0].result._meta['lumi/launch_arguments'].candidates[0].text, candidates[0].text);
  assert.equal(JSON.stringify(h.replies[0].result.content).includes(candidates[0].text), false);
  assert.equal(JSON.stringify(h.replies[0].result.structuredContent).includes(candidates[0].text), false);
});

test('a persisted successful task receipt remains authoritative when a duplicate Host reply fails late', async () => {
  const project = '01989abc-def0-7000-8000-000000000001';
  const h = host(({ params }) => ({ ok: true, result: { success: true, data: params.preflight ? { ready: true }
    : params.receipt ? { status: 'succeeded', host_result: { run_id: 'already-accepted' } }
      : { host_action: { operation: 'send', params: {}, request_key: 'send-once', digest: 'digest' } } } }), {
    async capabilities() { return { operations: ['send'] }; },
    async send() { throw Object.assign(new Error('late duplicate reply'), { code: 'OUTCOME_UNKNOWN' }); },
  });
  await h.invoke({ type: 'tool-call', callId: 'reconcile-host-call', tool: 'production_tasks', args: {
    session_context: context, action: 'send', expected_project_uuid: project, chapter_uuid: project,
    role: 'chapter', request_key: 'send-once', expected_revision: 1, text: 'Draft once.',
  } });
  assert.equal(h.calls.at(-1).params.receipt.error.code, 'OUTCOME_UNKNOWN');
  assert.equal(h.replies[0].result.success, true);
  assert.equal(h.replies[0].result.data.host_result.run_id, 'already-accepted');
});

const productionMessage = (action, extra = {}) => ({ type: 'tool-call', callId: 'production-host-call', tool: 'production_tasks', args: {
  session_context: context, expected_project_uuid: '01989abc-def0-7000-8000-000000000001', action, ...extra,
} });
function productionHost(operation, result, receipt) {
  const invoked = [];
  const h = host(({ params }) => ({ ok: true, result: { success: true, data: params.preflight ? { ready: true }
    : params.receipt ? receipt
      : { host_action: { operation, params: {}, ...(receipt ? { request_key: 'send-once', digest: 'digest' } : {}) } } } }), {
    async capabilities() { return { operations: [operation] }; },
    async [operation](args) { invoked.push({ operation, args }); return result; },
  });
  return { ...h, invoked };
}

test('ordinary running snapshots return once without waiting, reading messages or promising a wakeup', async () => {
  for (const status of ['accepted', 'queued', 'running', 'reconciling', 'unknown', 'future-status']) {
    const h = productionHost('getRun', { runId: 'run-a', status });
    await h.invoke(productionMessage('get_run', { request_key: 'send-a' }));
    assert.equal(h.invoked.length, 1);
    const guidance = h.replies[0].result.data.coordination;
    assert.equal(guidance.run_state, status);
    assert.equal(guidance.run_id, 'run-a');
    assert.equal(guidance.request_key, 'send-a');
    assert.equal(guidance.next_action, 'end_turn_until_report_or_explicit_resume');
    assert.equal(guidance.model_polling_allowed, false);
    assert.equal(guidance.wait_supported, false);
    assert.equal(guidance.automatic_completion_notification, false);
    assert.equal(guidance.chapter_completion_verified, false);
  }
});

test('two chapter runs keep independent outcomes and a terminal run still requires Lumi verification', async () => {
  for (const [id, status, next] of [
    ['chapter-a', 'completed', 'read_handoff_and_verify_lumi_once'],
    ['chapter-b', 'running', 'end_turn_until_report_or_explicit_resume'],
    ['chapter-c', 'needs_attention', 'handle_blocker'],
    ['chapter-d', 'failed', 'handle_blocker'],
  ]) {
    const h = productionHost('getRun', { runId: id, status });
    await h.invoke(productionMessage('get_run', { request_key: id + '-send' }));
    const guidance = h.replies[0].result.data.coordination;
    assert.equal(guidance.run_id, id);
    assert.equal(guidance.next_action, next);
    assert.equal(guidance.chapter_completion_verified, false);
    assert.equal(h.invoked.length, 1, 'no follow-up query or automatic dispatch');
  }
});

test('cached accepted send receipts remain accepted after recovery, never completed or re-dispatched', async () => {
  for (const action of ['send', 'resume_request', 'request_status']) {
    const receipt = { operation: 'send', status: 'succeeded', host_result: { run_id: 'saved-run', status: 'completed' } };
    const h = host(({ params }) => ({ ok: true, result: { success: true, data: params.preflight ? { ready: true }
      : action === 'request_status' ? { request: receipt } : receipt } }), {
      async capabilities() { return { operations: ['send'] }; },
      async send() { throw new Error('must not resend accepted input'); },
    });
    const extra = action === 'send' ? { chapter_uuid: '01989abc-def0-7000-8000-000000000001', role: 'chapter', text: 'Draft', expected_revision: 1 } : {};
    await h.invoke(productionMessage(action, { request_key: 'saved-send', ...extra }));
    assert.equal(h.replies[0].result.data.coordination.run_state, 'accepted');
    assert.equal(h.replies[0].result.data.coordination.chapter_completion_verified, false);
  }
});

test('visible completion words and old messages do not establish run completion; cursors stay exact', async () => {
  const h = productionHost('readMessages', { items: [{ id: 'old', role: 'assistant', content: 'Completed 12/12' }], nextCursor: 'opaque:next:42' });
  await h.invoke(productionMessage('read_messages', { chapter_uuid: '01989abc-def0-7000-8000-000000000001', role: 'chapter', after: 'opaque:before:41' }));
  const data = h.replies[0].result.data;
  assert.equal(data.host_result.next_cursor, 'opaque:next:42');
  assert.equal(data.coordination.run_state, 'unobserved');
  assert.equal(data.coordination.next_action, 'follow_saved_wait_strategy');
  assert.equal(data.coordination.chapter_completion_verified, false);
  assert.equal(h.invoked.length, 1);
});

test('adapter capability guidance does not claim native agent capabilities or start background work', async () => {
  const h = host(({ params }) => ({ ok: true, result: { success: true, data: params.preflight ? { ready: true }
    : { host_capabilities: params.capabilities, wait: 'not_supported_by_adapter' } } }), {
    async capabilities() { return { operations: ['create', 'getRun'], sourceCallContext: true }; },
  });
  await h.invoke(productionMessage('capabilities'));
  const data = h.replies[0].result.data;
  assert.equal(data.wait, 'not_supported_by_adapter');
  assert.equal(data.host_capabilities.sourceCallContext, true);
  assert.equal(data.coordination.scope, 'production_tasks_adapter');
  assert.equal(data.coordination.next_action, 'select_task_and_wait_route');
  assert.equal(data.coordination.model_polling_allowed, false);
});

test('unknown and rejected request receipts preserve recovery and never imply a completed run', async () => {
  for (const [status, next] of [['pending', 'preserve_request_and_reconcile_once'], ['unknown', 'preserve_request_and_reconcile_once'], ['failed', 'handle_blocker']]) {
    const h = host({ ok: true, result: { success: true, data: { request: { operation: 'send', status } } } });
    await h.invoke(productionMessage('request_status', { request_key: 'original-send' }));
    assert.equal(h.replies[0].result.data.coordination.next_action, next);
    assert.equal(h.replies[0].result.data.coordination.chapter_completion_verified, false);
    assert.equal(h.calls.length, 1, 'recovery does not automatically retry');
  }
});

test('failed Host reads retain the actual error and cannot trigger automatic retries', async () => {
  let attempts = 0;
  const h = host(({ params }) => ({ ok: true, result: { success: true, data: params.preflight ? { ready: true }
    : { host_action: { operation: 'getRun', params: { runId: 'original-run' } } } } }), {
    async capabilities() { return { operations: ['getRun'] }; },
    async getRun() { attempts++; throw Object.assign(new Error('read unavailable'), { code: 'TASK_UNAVAILABLE' }); },
  });
  await h.invoke(productionMessage('get_run', { request_key: 'original-send' }));
  const result = h.replies[0].result;
  assert.equal(attempts, 1);
  assert.equal(result.success, false);
  assert.equal(result.error.code, 'host_task_failed');
  assert.equal(result.correction.coordination.next_action, 'handle_error_without_polling');
  assert.equal(result.correction.coordination.model_polling_allowed, false);
});
