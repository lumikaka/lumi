'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const os = require('node:os');
const sourceDir = path.resolve(__dirname, '../../docs/features/cindy-plugins/spike/product-probe');
const product = fs.readFileSync(path.resolve(__dirname, '../lumi/main.js'), 'utf8');
const source = fs.readFileSync(path.join(sourceDir, 'main-prefix.js'), 'utf8') + product + fs.readFileSync(path.join(sourceDir, 'main-suffix.js'), 'utf8');
const caller = { session_id: 'parent', workdir: '/source/repo', workdir_is_local: true, workdir_is_read_only: false };
function fixture() {
  let handler, changed = false;
  const requests = [], replies = [], creates = [], tasks = new Map();
  vm.runInNewContext(source, { BroadcastChannel: class { postMessage() {} }, cindy: {
    onHostMessage(fn) { handler = fn; }, send: async value => { replies.push(value); },
    tasks: {
      capabilities: async () => ({ sourceCallContext: true }),
      create: async options => {
        creates.push(options);
        const item = { taskId: 'host-task-' + creates.length, title: options.title, workingDir: '/host-owned/fixture-' + creates.length, permissionMode: 'ask' };
        tasks.set(item.taskId, item); return item;
      },
      get: async ({ taskId }) => ({ ...tasks.get(taskId), ...(changed ? { workingDir: '/different/root' } : {}) }),
      send: () => assert.fail('harness must not dispatch another Agent'),
    },
    node: { request: async args => { requests.push(args); return { ok: true, result: { success: true, data: {} } }; } },
  } });
  return { requests, replies, creates, changeDirectory: () => { changed = true; },
    call: (tool, args = {}) => handler({ type: 'tool-call', tool, callId: 'actual-call', args: { ...args, session_context: caller } }) };
}
test('Host probe only derives fixture directories from owned Host receipts and never starts an Agent', async () => {
  const h = fixture();
  await h.call('run', { fixture: 'a', name: 'project_init', args: { idea: 'x' } });
  assert.equal(h.requests.length, 0);
  await h.call('create_fixture', { fixture: 'a' });
  assert.equal(h.creates[0].isolatedWorkspace, true); assert.equal(h.creates[0].callId, 'actual-call');
  assert.equal('permissionMode' in h.creates[0], false);
  assert.equal(h.replies.at(-1).result.agent_started, false);
  await h.call('create_fixture', { fixture: 'a', task_id: 'host-task-1' });
  assert.equal(h.creates.length, 1, 'restore a Host-owned receipt without a second create');
  assert.equal(h.replies.at(-1).result.context_source, 'host_tasks_get_previous_receipt');
  await h.call('create_fixture', { fixture: 'b', task_id: 'host-task-1' });
  assert.equal(h.replies.at(-1).ok, false, 'receipt cannot rename fixture A to B');
  await h.call('run', { fixture: 'a', name: 'project_overview', args: {}, workdir: '/model-chosen' });
  assert.equal(h.requests[0].params.context.workdir, '/host-owned/fixture-1');
  assert.equal(h.requests[0].params.context.session_id, 'host-task-1');
  assert.equal(h.requests[0].callId, 'actual-call'); assert.equal(h.requests[0].cancelWithCall, true);
  assert.equal(JSON.stringify(h.replies).includes('/host-owned/'), false);
  h.changeDirectory();
  await h.call('run', { fixture: 'a', name: 'project_init', args: { idea: 'must not write' } });
  assert.equal(h.requests.length, 1); assert.equal(h.replies.at(-1).ok, false);
});

test('probe rebuild replaces native executables without changing bytes held by an old process', async t => {
  const { copyNative } = await import('../../docs/features/cindy-plugins/spike/product-probe/copy-native.mjs');
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'lumi-probe-native-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const source = path.join(root, 'new-cli'), target = path.join(root, 'bin', 'lumi_dev');
  fs.mkdirSync(path.dirname(target));
  fs.writeFileSync(source, 'new executable', { mode: 0o755 });
  fs.writeFileSync(target, 'old executable', { mode: 0o755 });
  const handle = fs.openSync(target, 'r'), oldInode = fs.fstatSync(handle).ino;
  try {
    copyNative(source, target);
    assert.equal(fs.readFileSync(handle, 'utf8'), 'old executable');
    assert.equal(fs.readFileSync(target, 'utf8'), 'new executable');
    assert.notEqual(fs.statSync(target).ino, oldInode);
    assert.equal(fs.statSync(target).mode & 0o777, 0o755);
    assert.deepEqual(fs.readdirSync(path.dirname(target)), ['lumi_dev']);
  } finally { fs.closeSync(handle); }
});

test('probe native copy rejects destination links and preserves their referenced files', async t => {
  const { copyNative } = await import('../../docs/features/cindy-plugins/spike/product-probe/copy-native.mjs');
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'lumi-probe-link-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const source = path.join(root, 'new-cli'), untouched = path.join(root, 'untouched');
  fs.writeFileSync(source, 'new executable'); fs.writeFileSync(untouched, 'keep');
  fs.symlinkSync(untouched, path.join(root, 'linked-cli'));
  assert.throws(() => copyNative(source, path.join(root, 'linked-cli')));
  fs.symlinkSync(root, path.join(root, 'linked-bin'));
  assert.throws(() => copyNative(source, path.join(root, 'linked-bin', 'new-file')));
  assert.equal(fs.readFileSync(untouched, 'utf8'), 'keep');
  assert.equal(fs.existsSync(path.join(root, 'new-file')), false);
});
