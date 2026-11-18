'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const { ProductionStore } = require('../lumi/node/production.cjs');
const { schedule } = require('../lumi/node/production-schedule.cjs');
const uuid = '01989abc-def0-7000-8000-000000000001';
const caps = { operations: ['create', 'get', 'send', 'getRun', 'readMessages'], sourceCallContext: true };
const create = { action: 'create', chapter_uuid: uuid, role: 'chapter', request_key: 'chapter-create', title: 'Chapter' };
async function fixture(t) {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-production-'));
  t.after(() => fs.rm(root, { recursive: true, force: true }));
  return { root, store: new ProductionStore(root, uuid) };
}
test('durable records use CAS, stable receipts, project isolation, read-only and safe paths', async t => {
  const { root, store } = await fixture(t);
  assert.equal((await store.read('index.json')).revision, 0);
  assert.deepEqual(await fs.readdir(root), [], 'read does not create directories');
  const input = { record: 'index.json', expected_revision: 0, request_key: 'write-1', data: { chapters: [uuid] } };
  assert.equal((await store.write(input)).revision, 1);
  assert.equal((await new ProductionStore(root, uuid).write(input)).revision, 1, 'acknowledgement lost: original request is idempotent');
  await assert.rejects(store.write({ ...input, data: { chapters: [] } }), { code: 'request_key_conflict' });
  await assert.rejects(store.write({ ...input, request_key: 'write-2' }), { code: 'production_revision_conflict' });
  await assert.rejects(new ProductionStore(root, uuid, true).write(input), { code: 'read_only_context' });
  await assert.rejects(new ProductionStore(root, uuid.replace(/1$/, '2')).read('index.json'), { code: 'project_identity_mismatch' });
  await assert.rejects(store.read('../project.sqlite'), { code: 'invalid_arguments' });
  await fs.symlink(root, path.join(root, 'user-data/lumi-production/chapters'));
  await assert.rejects(store.read(`chapters/${uuid}/chapter.json`), { code: 'production_path_unsafe' });
});
test('task request survives restart, unknown creation blocks duplicates, original key reconciles', async t => {
  const { root, store } = await fixture(t);
  const plan = await store.task(create, caps, 'real-call');
  const action = plan.host_action;
  assert.equal(action.params.callId, 'real-call');
  assert.equal(action.params.isolatedWorkspace, true);
  assert.equal(action.params.workingDir, undefined);
  assert.equal((await store.task({ ...create, request_key: 'new-key' }, caps)).status, 'pending');
  assert.equal((await store.task(create, caps, 'later-call')).status, 'pending');
  await store.receipt({ request_key: create.request_key, digest: action.digest, error: { code: 'OUTCOME_UNKNOWN' } });
  const recovered = new ProductionStore(root, uuid);
  const resumed = await recovered.task({ action: 'resume_request', request_key: create.request_key });
  assert.deepEqual(resumed.host_action.params, action.params, 'resume preserves original invocation and parameters');
  await recovered.receipt({ ...resumed.host_action, result: { taskId: 'host-task', workingDir: '/host/assigned', revision: 1, permissionMode: 'ask' } });
  const reused = await recovered.task({ ...create, request_key: 'do-not-duplicate' }, caps);
  assert.equal(reused.reused, true);
  assert.equal(reused.operation, 'create');
  assert.equal(reused.host_result.task_id, 'host-task');
  const sending = { action: 'send', chapter_uuid: uuid, role: 'chapter', request_key: 'send-1', text: 'Draft only', expected_revision: 1 };
  const pending = await recovered.task(sending, caps);
  await assert.rejects(recovered.task({ ...sending, request_key: 'send-2' }, caps), { code: 'production_request_pending' });
  await recovered.receipt({ ...pending.host_action, error: { code: 'PERMISSION_DENIED' } });
  assert.equal((await recovered.task({ action: 'resume_request', request_key: 'send-1' })).status, 'failed', 'permission denial is not automatically retried');
  assert.equal((await recovered.task({ action: 'request_status', request_key: 'send-1' })).request.operation, 'send', 'receipt retains operation after recovery; send success cannot be confused with run completion');
});
test('scheduler enforces serial default, two independent scenes, dependency/current versions and repair pause', () => {
  const pages = [1, 2, 3].map(n => ({ uuid: String(n), variant_uuid: 'v' + n, revision: 2, review_status: 'pending' }));
  const scenes = pages.map((p, i) => ({ number: i + 1, pages: [p.uuid], depends_on: i === 2 ? [2] : [], initial_state_verified: true, status: 'waiting' }));
  const chapter = { scenes };
  assert.deepEqual(schedule(chapter, pages).eligible, [1]);
  chapter.scheduling = { mode: 'independent_pair' };
  assert.deepEqual(schedule(chapter, pages).eligible, [], 'parallel without explicit user choice rejected');
  chapter.scheduling.user_choice = '本章独立场景最多两场并行';
  assert.deepEqual(schedule(chapter, pages).eligible, [1, 2]);
  scenes[0].status = 'active'; scenes[1].status = 'active';
  assert.deepEqual(schedule(chapter, pages).eligible, []);
  pages[1].review_status = 'passed'; scenes[1].status = 'done';
  assert.deepEqual(schedule(chapter, pages).eligible, [], 'passed alone lacks verified continuity');
  scenes[1].verification = { checked_at: '2026-11-17T00:00:00Z', handoff: 'Actual continuity checked', pages: [{ ...pages[1] }] };
  assert.deepEqual(schedule(chapter, pages).eligible, [3], 'C may run with A once B has passed and is verified');
  pages[1].revision++;
  assert.deepEqual(schedule(chapter, pages).eligible, [], 'fresh facts invalidate old handoff');
  scenes[1].verification.pages[0].revision++;
  chapter.settings_repair = { status: 'pending', affected_scenes: [1, 3] };
  assert.deepEqual(schedule(chapter, pages).pause, [1]);
  assert.deepEqual(schedule(chapter, pages).eligible, []);
  scenes[0].status = 'paused';
  assert.deepEqual(schedule(chapter, pages).active, [1], 'paused work retains capacity and ownership');
  chapter.settings_repair.status = 'passed';
  assert.deepEqual(schedule(chapter, pages).eligible, [3]);
  scenes[2].pages = ['1'];
  assert.ok(schedule(chapter, pages).blocked.includes('page_ownership_or_order'));
});

test('chapter handoff and dependent scenes wait for owners and unknown requests even with passed old artwork', () => {
  const pages = [1, 2].map(n => ({ uuid: String(n), variant_uuid: 'v' + n, revision: 2, review_status: 'passed' }));
  const scenes = pages.map((p, i) => ({ number: i + 1, pages: [p.uuid], depends_on: i ? [1] : [], initial_state_verified: true,
    status: 'done', verification: { checked_at: '2026-11-18T00:00:00Z', handoff: 'Verified current image', pages: [{ ...p }] } }));
  const chapter = { scenes };
  assert.equal(schedule(chapter, pages).complete, true);
  for (const status of ['active', 'paused', 'waiting']) {
    scenes[0].status = status;
    assert.equal(schedule(chapter, pages).complete, false, `${status} writer cannot be declared complete from old facts`);
  }
  scenes[0].status = 'done';
  scenes[0].pending_requests = ['generation-outcome-unknown'];
  assert.deepEqual(schedule(chapter, pages).active, [1]);
  assert.equal(schedule(chapter, pages).complete, false);
  pages[1].review_status = 'pending'; scenes[1].status = 'waiting';
  chapter.scheduling = { mode: 'independent_pair', user_choice: '独立场景最多两场并行' };
  assert.deepEqual(schedule(chapter, pages).eligible, [], 'dependent scene cannot consume the old approval of an unsettled writer');
  scenes[0].pending_requests = [];
  assert.deepEqual(schedule(chapter, pages).eligible, [2]);
  chapter.pending_requests = ['chapter-write-outcome-unknown'];
  assert.deepEqual(schedule(chapter, pages).eligible, []);
  assert.equal(schedule(chapter, pages).complete, false);
});

test('serial dependency declarations and malformed nested records fail closed without losing records', () => {
  const page = { uuid: 'p1', variant_uuid: 'v1', revision: 1, review_status: 'passed' };
  const scene = { number: 1, pages: ['p1'], status: 'done', depends_on: [], initial_state_verified: true };
  const chapter = { scenes: [scene] };
  for (const verification of [{ pages: 'bad' }, { checked_at: '2026-11-18T00:00:00Z', handoff: 'x', pages: [null] }, { checked_at: 'bad-date', handoff: 'x', pages: [] }]) {
    scene.verification = verification;
    assert.equal(schedule(chapter, [page]).complete, false);
  }
  scene.depends_on = [2];
  assert.ok(schedule(chapter, [page]).blocked.includes('invalid_scene_dependency'));
  scene.depends_on = [];
  for (const repair of [{ status: 'pending', affected_scenes: '1' }, { status: 'passed', affected_scenes: [999] }]) {
    chapter.settings_repair = repair;
    assert.deepEqual(schedule(chapter, [page]).blocked, ['invalid_settings_repair']);
  }
});

test('chapter completion cannot omit existing pages or ignore pages added after the scene plan', () => {
  const pages = [1, 2, 3].map(n => ({ uuid: String(n), variant_uuid: 'v' + n, revision: 2, review_status: 'passed' }));
  const scene = page => ({ number: Number(page.uuid), pages: [page.uuid], depends_on: [], initial_state_verified: true,
    status: 'done', verification: { checked_at: '2026-11-18T00:00:00Z', handoff: 'Current page inspected', pages: [{ ...page }] } });
  for (const selected of [[pages[1]], pages.slice(1), pages.slice(0, 2)]) {
    const result = schedule({ scenes: selected.map(scene) }, pages);
    assert.equal(result.complete, false, 'unassigned chapter pages cannot be silently skipped');
    assert.deepEqual(result.eligible, []);
    assert.ok(result.blocked.includes('page_scope_incomplete'));
  }
  const chapter = { scenes: pages.map(scene) };
  assert.equal(schedule(chapter, pages).complete, true);
  const added = { uuid: '4', revision: 0, review_status: 'missing_image' };
  assert.equal(schedule(chapter, [...pages, added]).complete, false, 'freshly added page invalidates an old whole-chapter handoff');
  chapter.scenes[0].status = 'active';
  const active = schedule(chapter, [...pages, added]);
  assert.deepEqual(active.active, [1], 'scope repair must retain active writer ownership');
  assert.equal(active.complete, false);
});

test('explicit user page scope preserves partial work while rejecting omitted, foreign and reordered targets', () => {
  const pages = [1, 2, 3].map(n => ({ uuid: String(n), variant_uuid: 'v' + n, revision: 2, review_status: 'passed' }));
  const chapter = { page_scope: { page_uuids: ['2'], user_request: '只处理本章第2页' }, scenes: [
    { number: 1, pages: ['2'], depends_on: [], initial_state_verified: true, status: 'done',
      verification: { checked_at: '2026-11-18T00:00:00Z', handoff: 'Scoped page inspected', pages: [{ ...pages[1] }] } },
  ] };
  assert.equal(schedule(chapter, pages).complete, true, 'unrequested pages stay outside an explicit user scope');
  for (const page_scope of [null, {}, { page_uuids: ['2'] }, { page_uuids: [], user_request: 'x' },
    { page_uuids: ['2', '2'], user_request: 'x' }, { page_uuids: ['foreign'], user_request: 'x' },
    { page_uuids: ['3', '2'], user_request: 'x' }]) {
    const result = schedule({ ...chapter, page_scope }, pages);
    assert.equal(result.complete, false);
    assert.ok(result.blocked.includes('invalid_page_scope'));
  }
  chapter.page_scope.page_uuids = ['1', '2'];
  assert.ok(schedule(chapter, pages).blocked.includes('page_scope_incomplete'), 'every explicitly requested page must have an owner');
  chapter.page_scope = { page_uuids: ['1', '3'], user_request: '只制作第1和第3页' };
  pages[0].review_status = pages[2].review_status = 'pending';
  chapter.scenes = [1, 3].map((number, i) => ({ number: i + 1, pages: [String(number)], depends_on: [], status: 'waiting', initial_state_verified: true }));
  assert.deepEqual(schedule(chapter, pages).eligible, [1], 'separate selected page ranges retain serial order');
  chapter.scenes = [{ ...chapter.scenes[0], pages: ['1', '3'] }];
  assert.ok(schedule(chapter, pages).blocked.includes('page_ownership_or_order'), 'one scene must remain a continuous page range');
});

test('an explicitly retried denied create preserves its old receipt and claims one replacement request', async t => {
  const { root, store } = await fixture(t);
  const original = (await store.task(create, caps, 'original-call')).host_action;
  await store.receipt({ ...original, error: { code: 'PERMISSION_DENIED' } });
  assert.equal((await store.task({ ...create, request_key: 'ordinary-new-key' }, caps)).status, 'failed', 'ordinary create never retries a denial');
  const recovered = new ProductionStore(root, uuid);
  const retry = { action: 'retry_create', request_key: create.request_key, new_request_key: 'after-user-restored-access' };
  await assert.rejects(recovered.task({ ...retry, new_request_key: retry.request_key }, caps, 'fresh-call'), { code: 'invalid_arguments' });
  await assert.rejects(new ProductionStore(root, uuid, true).task(retry, caps, 'fresh-call'), { code: 'read_only_context' });
  const action = (await recovered.task(retry, caps, 'fresh-call')).host_action;
  assert.equal(action.operation, 'create');
  assert.equal(action.params.title, original.params.title);
  assert.equal(action.params.isolatedWorkspace, true);
  assert.equal(action.params.callId, 'fresh-call');
  assert.notEqual(action.params.requestKey, original.params.requestKey);
  assert.equal((await recovered.task(retry, caps, 'another-call')).status, 'pending', 'same replacement key does not dispatch twice');
  await assert.rejects(recovered.task({ ...retry, new_request_key: 'second-replacement' }, caps, 'another-call'), { code: 'production_retry_forbidden' });
  await recovered.receipt({ ...action, result: { taskId: 'new-host-task', workingDir: '/host/new-directory', revision: 1, permissionMode: 'ask' } });
  assert.equal((await recovered.task({ ...create, request_key: 'reuse-after-retry' }, caps)).host_result.task_id, 'new-host-task');
  assert.equal((await recovered.task({ action: 'request_status', request_key: create.request_key })).request.status, 'failed', 'original denied receipt stays immutable');
});

test('replacement creation cannot clear an unknown, conflicting or successful Host request', async t => {
  for (const code of ['OUTCOME_UNKNOWN', 'REQUEST_KEY_CONFLICT', 'INVALID_PARAMS', null]) {
    const { store } = await fixture(t);
    const action = (await store.task(create, caps, 'original-call')).host_action;
    await store.receipt({ ...action, ...(code ? { error: { code } } : { result: { taskId: 'accepted-task', workingDir: '/host/accepted', revision: 1 } }) });
    await assert.rejects(store.task({ action: 'retry_create', request_key: create.request_key, new_request_key: 'new-key' }, caps, 'fresh-call'), { code: 'production_retry_forbidden' });
    assert.equal((await store.read('tasks.json')).data.slots[uuid + ':chapter'].create_key, create.request_key);
  }
});
