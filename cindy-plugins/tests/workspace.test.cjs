'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { WorkspaceGateway } = require('../lumi/node/workspace.cjs');
const { Runtime } = require('../lumi/node/runtime.cjs');
const { RuntimeError } = require('../lumi/node/errors.cjs');
const uuid = '01989abc-def0-7000-8000-000000000001';

test('startup identity lock contention has a bounded read retry without business or service operations', async () => {
  const commands = [];
  const runtime = new Runtime({ executable: '/bundle/bin/lumi', statusRunner: async (_binary, args) => {
    commands.push(args); if (commands.length < 3) throw new RuntimeError('instance_unavailable', 'Transient status lock');
    return { running: false, environment: 'production' };
  } });
  assert.equal((await runtime.inspect('/isolated/data')).running, false);
  assert.equal(commands.length, 3);
  assert.ok(commands.every(args => args[0] === 'status'));
  let attempts = 0;
  const unavailable = new Runtime({ executable: '/bundle/bin/lumi', statusRunner: async () => { attempts++; throw new RuntimeError('instance_unavailable', 'Still unavailable'); } });
  await assert.rejects(unavailable.inspect('/isolated/data'), { code: 'instance_unavailable' });
  assert.equal(attempts, 4);
  await runtime.close(); await unavailable.close();
});

test('production has no page authority; self-reported session/query/broadcast cannot authorize an RPC', async () => {
  let called = false;
  const runtime = { workspace() { called = true; } };
  const gateway = new WorkspaceGateway({ runtime, sender: { session: 'forged', project_uuid: uuid, can_write: true } });
  await assert.rejects(gateway.call('read_app_resource', { resource: 'project' }), { code: 'workspace_host_identity_unavailable' });
  assert.equal(called, false);
  for (const method of ['workspace', 'read_app_resource', 'write_app_resource', 'watch_app_changes']) {
    const real = new Runtime();
    await assert.rejects(real.execute(method, { context: {}, arguments: {} }), { code: 'method_not_found' });
    await real.close();
  }
});
test('Host-observed sender identity, permission change and page revocation are checked for every result', async () => {
  const controller = new AbortController(), sender = {}, impostor = {};
  let finish, calls = 0;
  let grant = { project_uuid: uuid, context: { session_id: 'trusted-host-task', workdir_is_read_only: false }, signal: controller.signal };
  const authority = { resolve: value => value === sender ? grant : null };
  const runtime = { workspace: async (...args) => { calls++; assert.equal(args[2], grant.context); return new Promise(resolve => { finish = resolve }); } };
  const fake = new WorkspaceGateway({ runtime, authority, sender: impostor });
  await assert.rejects(fake.call('read_app_resource', { resource: 'project' }), { code: 'workspace_revoked' });
  assert.equal(calls, 0);
  const real = new WorkspaceGateway({ runtime, authority, sender });
  const pending = real.call('write_app_resource', { method: 'PATCH', resource: 'project', body: {} });
  await new Promise(resolve => setImmediate(resolve));
  grant = { ...grant, context: { ...grant.context, workdir_is_read_only: true } };
  finish({ success: true, data: {} });
  await assert.rejects(pending, { code: 'result_unknown' });
  controller.abort();
  await assert.rejects(real.call('read_app_resource', { resource: 'project' }), { code: 'workspace_revoked' });
  await real.close();
});
