'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const vm = require('node:vm');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { Runtime, runCLI } = require('../lumi/node/runtime.cjs');
const { envelope } = require('../lumi/node/errors.cjs');
const { boardPNG } = require('./fixtures/board.cjs');
const ok = result => { assert.equal(result.success, true, JSON.stringify(result)); return result.data; };
const sha = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
// Real CLI/SQLite/media/files and production main.js; ordinary Cindy tasks and
// generation are deterministic mocks. This is not native Host/model evidence.
test('multi-chapter scene gates, Host request reconciliation and import interruption recover without duplicates', { timeout: 120000 }, async t => {
  const base = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-cindy-production-'));
  const root = path.join(base, 'book'), home = path.join(base, 'home'), dataDir = path.join(base, 'data'), executable = path.join(base, 'lumi');
  for (const dir of [root, home, dataDir]) await fs.mkdir(dir);
  const env = { HOME: home, PATH: '/usr/bin:/bin', TMPDIR: os.tmpdir() };
  const status = () => runCLI(executable, ['status', '--data-dir', dataDir, '--json'], { cwd: base, env });
  let runtime, nonce;
  t.after(async () => {
    await runtime?.close();
    if (nonce) {
      const current = await status();
      if (current.running) {
        assert.equal(current.nonce, nonce, 'only stop the isolated test instance');
        assert.equal((await fetch(current.endpoint + '/api/v1/cli-instance', { method: 'DELETE', headers: { 'If-Match': nonce } })).status, 200);
        // Shutdown can briefly retain discovery while its identity lock closes.
        // Keep waiting only for that known transient, then require stopped facts.
        for (let i = 0; i < 200; i++) {
          try { if (!(await status()).running) break; }
          catch (error) { if (error.code !== 'instance_unavailable') throw error; }
          await new Promise(resolve => setTimeout(resolve, 25));
        }
        assert.equal((await status()).running, false);
      }
    }
    await fs.rm(base, { recursive: true, force: true });
  });
  const built = spawnSync('go', ['build', '-o', executable, './cmd/lumi'], { encoding: 'utf8', timeout: 60000 });
  assert.equal(built.status, 0, built.stderr);
  const options = { executable, build: { platform: process.platform, arch: process.arch, cli_sha256: sha(await fs.readFile(executable)) }, dataDir, env };
  runtime = new Runtime(options);
  const context = { session_id: 'production-fixture', workdir: root, workdir_is_local: true, workdir_is_read_only: false };
  const call = async (name, args = {}) => ok(await runtime.execute('call_tool', { context, arguments: { name, args } }));
  await call('project_init', { idea: 'Two chapters, independent scenes A/B and dependent C.' });
  nonce = (await status()).nonce;
  const projectUUID = (await call('project_overview')).project.uuid;
  let setup = await call('project_setup', { action: 'get' });
  setup = await call('project_setup', { action: 'update', expected_revision: setup.revision, project_name: 'Production fixture', overall_style: 'Synthetic geometry', picture_book: { format: 'classic_picture_book' } });
  await call('project_setup', { action: 'finalize', expected_revision: setup.revision });
  const chapters = [];
  // Create out of lexical order to check the website follows Lumi chapter order.
  for (const [code, title] of [['vol01.ch02', 'Second'], ['vol01.ch01', 'First']]) {
    chapters.push(await call('chapters', { action: 'create', chapter_code: code, title, content: 'Complete chapter prose with exact dialogue.', expected_revision: (await call('project_overview')).project.revision }));
  }
  chapters.reverse();
  const ledger = (name, args) => runtime.execute('production_call', { context, name, arguments: { expected_project_uuid: projectUUID, ...args } }).then(ok);
  let serial = 0;
  const write = async (record, data) => {
    const old = await ledger('production_records', { action: 'read', record });
    return ledger('production_records', { action: 'write', record, data, expected_revision: old.revision, request_key: `record-${++serial}` });
  };
  const chapterPath = chapter => `chapters/${chapter.uuid}/chapter.json`;
  await write('index.json', { project_uuid: projectUUID, chapters: chapters.map(c => c.uuid), user_scope: 'two chapters', review_agreement: 'synthetic automatic fixture only' });
  const plans = [];
  for (const [n, chapter] of chapters.entries()) {
    const pages = [];
    for (let i = 1; i <= (n === 0 ? 3 : 1); i++) {
      const draft = { page_number: i, source_spans: [{ start: 0, end: 10, quote: 'Complete' }], title: `Scene ${i}`, page_role: 'body', description_md: `Exact dialogue ${n}-${i}`, utterances: [{ kind: 'narration', speaker: 'narrator', text: `Exact dialogue ${n}-${i}` }], storyboard_md: 'Complete scene action and caption', continuity: { in: 'Start', out: 'Verified endpoint' } };
      await write(`chapters/${chapter.uuid}/storyboard/page-${i}.json`, draft);
      pages.push(await call('pages', { action: 'create', chapter_uuid: chapter.uuid, title: draft.title, description_md: draft.description_md, storyboard_md: draft.storyboard_md, expected_revision: (await call('pages', { action: 'list', chapter_uuid: chapter.uuid })).state.revision }));
    }
    await write(`chapters/${chapter.uuid}/storyboard/manifest.json`, { input_digest: sha(Buffer.from(chapter.content || 'fixture')), status: 'drafted', pages: pages.map((p, i) => i + 1) });
    await write(`chapters/${chapter.uuid}/storyboard-commit.json`, { pages: pages.map(p => ({ page_uuid: p.uuid, revision: p.revision })) });
    const plan = { scheduling: { mode: 'serial' }, scenes: pages.map((p, i) => ({ number: i + 1, pages: [p.uuid], depends_on: i === 2 ? [2] : [], initial_state_verified: true, status: 'waiting' })) };
    await write(chapterPath(chapter), plan); plans.push({ chapter, pages, plan });
  }
  const decision = chapter => ledger('production_records', { action: 'schedule', record: chapterPath(chapter) });
  assert.deepEqual((await decision(chapters[0])).eligible, [1]);
  assert.deepEqual((await decision(chapters[1])).blocked, ['previous_chapter_unverified']);
  const first = plans[0]; first.plan.scheduling = { mode: 'independent_pair', user_choice: '本章独立场景最多两场并行' };
  await write(chapterPath(chapters[0]), first.plan);
  assert.deepEqual((await decision(chapters[0])).eligible, [1, 2]);
  // Use actual browser entry and a mocked standard SDK. The mock itself also
  // refuses duplicates by durable requestKey, like the documented Host API.
  const hostTasks = new Map(), hostRuns = new Map();
  let creates = 0, sends = 0, uncertain = true, denyNextCreate = false, callback, reply;
  const taskAPI = {
    async capabilities() { return { operations: ['create', 'get', 'send', 'getRun', 'readMessages'], sourceCallContext: true }; },
    async create(params) {
      const saved = JSON.parse(await fs.readFile(path.join(root, 'user-data/lumi-production/tasks.json'), 'utf8'));
      assert.ok(Object.values(saved.data.requests).some(r => r.params.requestKey === params.requestKey && r.status === 'pending'), 'request exists before SDK call');
      if (denyNextCreate) { denyNextCreate = false; throw Object.assign(new Error('permission rejected before creation'), { code: 'PERMISSION_DENIED' }); }
      if (!hostTasks.has(params.requestKey)) hostTasks.set(params.requestKey, { taskId: 'host-task-' + ++creates, workingDir: path.join(base, 'host-' + creates), revision: 1, permissionMode: 'ask' });
      return hostTasks.get(params.requestKey);
    },
    async get({ taskId }) { return [...hostTasks.values()].find(t => t.taskId === taskId); },
    async send(params) {
      if (!hostRuns.has(params.requestKey)) hostRuns.set(params.requestKey, { runId: 'run-' + ++sends, status: 'accepted' });
      if (uncertain) { uncertain = false; throw Object.assign(new Error('receipt lost'), { code: 'OUTCOME_UNKNOWN' }); }
      return hostRuns.get(params.requestKey);
    },
    async getRun({ runId }) { return { runId, status: 'reconciling' }; },
    async readMessages({ after }) {
      assert.equal(after, 'opaque:cursor');
      return { items: [
        { id: 'input', role: 'user', content: 'Draft only.', createdAt: 1 },
        { id: 'private', role: 'thinking', content: 'Internal fixture marker.' },
        { id: 'tool', role: 'tool_use', content: { command: 'private fixture command' } },
        { id: 'draft', role: 'assistant', content: 'Keep OriginalCase: storyboard/page-1.json', createdAt: 2, internalTrace: 'omit' },
      ], nextCursor: 'opaque:next' };
    },
  };
  const source = await fs.readFile(path.join(__dirname, '../lumi/main.js'), 'utf8');
  const browser = () => vm.runInNewContext(source, { cindy: { tasks: taskAPI, onHostMessage(fn) { callback = fn; },
    node: { async request({ method, params }) { try { return { ok: true, result: await runtime.execute(method, params) }; } catch (e) { return { ok: true, result: envelope(e) }; } } },
    async send(result) { reply = result; },
  } });
  browser();
  const taskCall = async args => {
    await callback({ type: 'tool-call', callId: 'actual-host-call', tool: 'production_tasks', args: { session_context: context, expected_project_uuid: projectUUID, ...args } });
    return ok(reply.result);
  };
  const role = { chapter_uuid: chapters[0].uuid, role: 'storyboard' };
  const task = await taskCall({ ...role, action: 'create', title: 'Draft chapter only', request_key: 'create-storyboard' });
  assert.equal(task.host_result.permission_mode, 'ask');
  assert.equal((await taskCall({ ...role, action: 'create', title: 'Reuse', request_key: 'new-create-key' })).reused, true);
  assert.equal(creates, 1);
  const sendArgs = { ...role, action: 'send', request_key: 'send-draft', expected_revision: task.host_result.revision, text: 'Write per-page draft files only; do not mutate Lumi.' };
  await callback({ type: 'tool-call', callId: 'actual-host-call', tool: 'production_tasks', args: { session_context: context, expected_project_uuid: projectUUID, ...sendArgs } });
  assert.equal(reply.ok, false);
  assert.equal(reply.result.error.code, 'production_result_unknown');
  assert.equal((await taskCall({ action: 'request_status', request_key: 'send-draft' })).request.status, 'unknown');
  assert.equal(sends, 1);
  assert.equal((await taskCall(sendArgs)).status, 'unknown', 'ordinary replay only reads receipt');
  assert.equal(sends, 1);
  browser(); // Fresh logic-page instance, same persisted request.
  assert.equal((await taskCall({ action: 'resume_request', request_key: 'send-draft' })).host_result.run_id, 'run-1');
  assert.equal(sends, 1, 'Host idempotency prevents a second execution');
  assert.equal((await taskCall({ action: 'get_run', request_key: 'send-draft' })).host_result.status, 'reconciling');
  const handoff = (await taskCall({ ...role, action: 'read_messages', after: 'opaque:cursor' })).host_result;
  assert.equal(handoff.next_cursor, 'opaque:next');
  assert.deepEqual(JSON.parse(JSON.stringify(handoff.items)), [
    { id: 'input', role: 'user', content: 'Draft only.', created_at: 1 },
    { id: 'draft', role: 'assistant', content: 'Keep OriginalCase: storyboard/page-1.json', created_at: 2 },
  ], 'only visible handoff text crosses the role boundary, with exact cursor and file identity');
  for (const [ctx, project, code] of [
    [{ ...context, workdir_is_read_only: true }, projectUUID, 'read_only_context'],
    [context, '01989abc-def0-7000-8000-000000000001', 'project_identity_mismatch'],
  ]) {
    await callback({ type: 'tool-call', callId: 'wrong-context', tool: 'production_tasks', args: { session_context: ctx, expected_project_uuid: project, ...role, action: 'create', title: 'Rejected', request_key: 'rejected-create' } });
    assert.equal(reply.ok, false); assert.equal(reply.result.error.code, code);
    assert.equal(creates, 1);
  }
  // A known rejection stays recorded. Restored permission alone never causes
  // dispatch; an explicit retry claims the same role once under a new key.
  const deniedRole = { chapter_uuid: chapters[0].uuid, role: 'settings' };
  denyNextCreate = true;
  await callback({ type: 'tool-call', callId: 'permission-rejected', tool: 'production_tasks', args: {
    session_context: context, expected_project_uuid: projectUUID, ...deniedRole,
    action: 'create', title: 'Chapter settings draft', request_key: 'create-settings-denied',
  } });
  assert.equal(reply.result.error.code, 'host_task_rejected');
  assert.equal((await taskCall({ ...deniedRole, action: 'create', title: 'Do not auto retry', request_key: 'different-key' })).status, 'failed');
  assert.equal(creates, 1);
  const retry = { action: 'retry_create', request_key: 'create-settings-denied', new_request_key: 'create-settings-restored' };
  for (const [ctx, project, code] of [
    [{ ...context, workdir_is_read_only: true }, projectUUID, 'read_only_context'],
    [context, '01989abc-def0-7000-8000-000000000001', 'project_identity_mismatch'],
  ]) {
    await callback({ type: 'tool-call', callId: 'retry-wrong-context', tool: 'production_tasks', args: {
      session_context: ctx, expected_project_uuid: project, ...retry,
    } });
    assert.equal(reply.result.error.code, code);
    assert.equal(creates, 1, 'retry cannot bypass current binding or write permission');
  }
  const restored = await taskCall(retry);
  assert.equal(restored.previous_request_key, retry.request_key);
  assert.equal(restored.status, 'succeeded');
  assert.equal(creates, 2);
  browser();
  assert.equal((await taskCall(retry)).host_result.task_id, restored.host_result.task_id);
  assert.equal((await taskCall({ ...deniedRole, action: 'get' })).host_result.task_id, restored.host_result.task_id);
  assert.equal((await taskCall({ action: 'request_status', request_key: retry.request_key })).request.status, 'failed');
  assert.equal(creates, 2, 'original rejection and successful retry survive a fresh logic page without another create');
  const image = boardPNG({ width: 120, height: 160 });
  let generations = 0, imports = 0;
  const summary = (chapter, page) => call('pages', { action: 'get', chapter_uuid: chapter.uuid, page_uuid: page.uuid, summary_only: true });
  const attach = async (chapter, page) => {
    const entry = plans.find(entry => entry.chapter.uuid === chapter.uuid), sceneIndex = entry.pages.indexOf(page);
    const requestKey = `image-${page.uuid}`;
    const prepared = await call('prepare_page_image', { page_uuid: page.uuid, setting_uuids: [] });
    entry.plan.scenes[sceneIndex].pending_requests = [requestKey];
    entry.plan.scenes[sceneIndex].status = 'active';
    await write(chapterPath(chapter), entry.plan);
    await write(`chapters/${chapter.uuid}/scene-${sceneIndex + 1}.json`, { pages: [page.uuid], pending: { stage: 'generation', prompt: prepared.generation_prompt, preparation: prepared.preparation, request_key: requestKey } });
    generations++;
    const params = { context, invocation: 'image-' + page.uuid };
    const transfer = ok(await runtime.execute('media_begin', { ...params, input: { ...prepared.image_write_back, prompt: prepared.generation_prompt, preparation: prepared.preparation }, bytes: image.length, sha256: sha(image), mime: 'image/png' })).transfer;
    ok(await runtime.execute('media_chunk', { ...params, transfer, index: 0, base64: image.toString('base64') }));
    ok(await runtime.execute('media_commit', { ...params, transfer })); imports++;
  };
  first.plan.scenes[0].status = 'active'; first.plan.scenes[1].status = 'active';
  await write(chapterPath(first.chapter), first.plan);
  await attach(first.chapter, first.pages[0]);
  const before = await summary(first.chapter, first.pages[0]);
  assert.equal(before.review_status, 'pending');
  // Interrupt after import but before recording its receipt. Read current facts
  // to reconcile, rather than replaying generation/import or creating chapters.
  await runtime.close(); runtime = new Runtime(options); browser();
  await call('project_overview'); await call('project_setup', { action: 'get' }); await call('book_outline');
  const resumed = await summary(first.chapter, first.pages[0]);
  assert.equal(resumed.variant_uuid, before.variant_uuid);
  assert.equal(resumed.review_status, 'pending');
  assert.equal((await call('chapters', { action: 'list' })).items.length, 2);
  assert.equal((await call('pages', { action: 'list', chapter_uuid: first.chapter.uuid })).items.length, 3);
  const interruptedRecord = await ledger('production_records', { action: 'read', record: `chapters/${first.chapter.uuid}/scene-1.json` });
  assert.equal(interruptedRecord.data.pending.request_key, `image-${first.pages[0].uuid}`);
  const inspect = async (entry, sceneIndex) => {
    const page = entry.pages[sceneIndex], current = await summary(entry.chapter, page);
    const preview = await runtime.execute('call_tool', { context, arguments: { name: 'read_image', args: { asset_uuid: current.asset_uuid } } });
    assert.ok(preview.transport_image);
    await call('pages', { action: 'record_image_review', chapter_uuid: entry.chapter.uuid, page_uuid: page.uuid, variant_uuid: current.variant_uuid, expected_revision: current.revision, verdict: 'passed', observation: 'Synthetic image transport check, not human visual approval.' });
    const fact = await summary(entry.chapter, page);
    const record = `chapters/${entry.chapter.uuid}/scene-${sceneIndex + 1}.json`;
    const saved = await ledger('production_records', { action: 'read', record });
    await write(record, { ...saved.data, pending: null, receipts: [{ request_key: saved.data.pending.request_key,
      page_uuid: page.uuid, variant_uuid: fact.variant_uuid, asset_uuid: fact.asset_uuid, revision: fact.revision, review_status: fact.review_status, reconciled_from_current_facts: true }] });
    Object.assign(entry.plan.scenes[sceneIndex], { status: 'done', pending_requests: [], verification: { checked_at: new Date().toISOString(), handoff: 'Synthetic continuity checkpoint', pages: [{ uuid: fact.uuid, variant_uuid: fact.variant_uuid, revision: fact.revision }] } });
    await write(chapterPath(entry.chapter), entry.plan);
  };
  await inspect(first, 0);
  assert.equal(generations, 1); assert.equal(imports, 1, 'resumed pending image is inspected without repeating work');
  assert.deepEqual((await decision(first.chapter)).eligible, [], 'dependent C waits for B');
  await attach(first.chapter, first.pages[1]); await inspect(first, 1);
  assert.deepEqual((await decision(first.chapter)).eligible, [3]);
  first.plan.settings_repair = { status: 'pending', affected_scenes: [3] }; await write(chapterPath(first.chapter), first.plan);
  assert.deepEqual((await decision(first.chapter)).eligible, []);
  first.plan.settings_repair.status = 'passed'; await write(chapterPath(first.chapter), first.plan);
  await attach(first.chapter, first.pages[2]); await inspect(first, 2);
  assert.equal((await decision(first.chapter)).complete, true);
  const fullPlan = first.plan.scenes;
  for (const omitted of [fullPlan.slice(1), fullPlan.slice(0, -1)]) {
    first.plan.scenes = omitted;
    await write(chapterPath(first.chapter), first.plan);
    const incomplete = await decision(first.chapter);
    assert.equal(incomplete.complete, false, 'passed subset does not complete the whole chapter');
    assert.ok(incomplete.blocked.includes('page_scope_incomplete'));
    assert.deepEqual((await decision(chapters[1])).blocked, ['previous_chapter_unverified']);
  }
  first.plan.scenes = fullPlan;
  await write(chapterPath(first.chapter), first.plan);
  for (const status of ['active', 'paused']) {
    first.plan.scenes[1].status = status;
    await write(chapterPath(first.chapter), first.plan);
    assert.equal((await decision(first.chapter)).complete, false);
    assert.deepEqual((await decision(chapters[1])).blocked, ['previous_chapter_unverified']);
  }
  first.plan.scenes[1].status = 'done';
  first.plan.scenes[1].pending_requests = ['late-generation-outcome-unknown'];
  await write(chapterPath(first.chapter), first.plan);
  assert.equal((await decision(first.chapter)).complete, false);
  assert.deepEqual((await decision(chapters[1])).blocked, ['previous_chapter_unverified']);
  first.plan.scenes[1].pending_requests = [];
  await write(chapterPath(first.chapter), first.plan);
  assert.deepEqual((await decision(chapters[1])).eligible, [1]);
  await attach(chapters[1], plans[1].pages[0]); await inspect(plans[1], 0);
  assert.equal((await decision(chapters[1])).complete, true);
  assert.equal(generations, 4); assert.equal(imports, 4);
  for (const entry of plans) for (const [index, page] of entry.pages.entries()) {
    const record = await ledger('production_records', { action: 'read', record: `chapters/${entry.chapter.uuid}/scene-${index + 1}.json` });
    assert.equal(record.data.pending, null);
    assert.equal(record.data.receipts[0].page_uuid, page.uuid);
    assert.equal(record.data.receipts[0].review_status, 'passed');
  }
  const exported = await call('export_website');
  assert.equal(exported.done, true);
  assert.equal(exported.output_directory, path.join(await fs.realpath(root), 'website'));
  assert.ok(exported.publish_prompt.includes('index.html'));
  const site = JSON.parse(await fs.readFile(path.join(exported.output_directory, 'data/site.json'), 'utf8'));
  const facts = await call('chapters', { action: 'list' });
  assert.deepEqual(facts.items.map(c => c.chapter_code), ['vol01.ch01', 'vol01.ch02']);
  assert.deepEqual(site.chapters.map(c => c.uuid), facts.items.map(c => c.uuid), 'reader follows actual Lumi chapter order');
  const checked = spawnSync('python3', [path.join(__dirname, 'fixtures/check-website.py'), exported.output_directory, '0', exported.generation_uuid], { encoding: 'utf8' });
  assert.equal(checked.status, 0, checked.stderr);
  t.diagnostic(JSON.stringify({ chapters: chapters.length, pages: 4, generations, imports, host_creates: creates, host_sends: sends, website_chapters: site.chapters.map(c => c.uuid), native_host: false, real_model: false }));
});
