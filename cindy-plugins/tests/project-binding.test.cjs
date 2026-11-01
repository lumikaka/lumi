'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const { Runtime, runCLI } = require('../lumi/node/runtime.cjs');
const { MCPConnection } = require('../lumi/node/protocol.cjs');
const { publicFailure, publicProject } = require('../lumi/node/project.cjs');
const author = path.resolve(__dirname, '../lumi');
const executable = path.join(author, 'bin/lumi');
const uuidPattern = /^[a-f0-9]{8}-[a-f0-9]{4}-7[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/;
const ctx = (workdir, session_id, args) => ({ context: { session_id, workdir, workdir_is_local: true, workdir_is_read_only: false },
  ...(args ? { arguments: args } : {}) });
const ok = result => { assert.equal(result.success, true, JSON.stringify(result)); return result.data; };

test('Cindy project binding through the real bundled CLI', { timeout: 120000 }, async t => {
  const base = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-cindy-binding-'));
  const home = path.join(base, 'home'), dataDir = path.join(base, 'data');
  const a = path.join(base, '甲 project'), b = path.join(base, '乙 project'), c = path.join(base, '双任务');
  for (const dir of [home, dataDir, a, b, c]) await fs.mkdir(dir);
  const env = { HOME: home, PATH: '/usr/bin:/bin', TMPDIR: os.tmpdir() };
  const build = JSON.parse(await fs.readFile(path.join(author, 'build-info.json'), 'utf8'));
  const options = { executable, build, dataDir, env };
  const runtimes = new Set(), peers = new Set();
  const create = () => { const value = new Runtime(options); runtimes.add(value); return value; };
  const status = () => runCLI(executable, ['status', '--data-dir', dataDir, '--json'], { cwd: base, env });
  let owned;
  t.after(async () => {
    await Promise.all([...runtimes].map(r => r.close()));
    await Promise.all([...peers].map(p => p.close()));
    const current = await status();
    if (current.running) {
      assert.equal(current.nonce, owned.nonce, 'only stop the exact isolated test instance');
      const stopped = await fetch(`${current.endpoint}/api/v1/cli-instance`, { method: 'DELETE', headers: { 'If-Match': current.nonce } });
      assert.equal(stopped.status, 200);
      const isStopped = async () => {
        try { return !(await status()).running; }
        catch (error) { if (error.code === 'instance_unavailable') return false; throw error; }
      };
      for (let i = 0; i < 200 && !(await isStopped()); i++) await new Promise(r => setTimeout(r, 25));
      assert.equal(await isStopped(), true);
    }
    await fs.rm(base, { recursive: true, force: true });
    console.log('BINDING CLEANUP: isolated server stopped; projects, proxies and temporary home removed');
  });
  const runtime = create();
  let projectA, projectB;

  await t.test('empty and non-conflicting directories initialize concurrently without engine guidance', async () => {
    for (const dir of ['.git', '.codex', 'hooks']) await fs.mkdir(path.join(a, dir));
    const original = { 'notes.txt': 'keep notes', 'AGENTS.md': '# 我的原始指引\n', '.codex/config.toml': '# user-owned\n', 'hooks/custom.json': '{"keep":true}' };
    for (const [file, content] of Object.entries(original)) await fs.writeFile(path.join(a, file), content);
    const results = await Promise.all([
      runtime.execute('project_init', ctx(a, 'a', { idea: '狐狸送星星回家', name: '星星', generation_language: 'zh-Hans' })),
      runtime.execute('project_init', ctx(b, 'b', { idea: 'A bird finds a friend', name: 'Bird', generation_language: 'en' })),
    ]);
    owned = await status();
    projectA = ok(results[0]); projectB = ok(results[1]);
    assert.match(projectA.project.uuid, uuidPattern);
    assert.notEqual(projectA.project.uuid, projectB.project.uuid);
    assert.equal(projectA.setup.setup_status, 'draft');
    for (const [file, content] of Object.entries(original)) assert.equal(await fs.readFile(path.join(a, file), 'utf8'), content);
    for (const file of ['.codex', 'AGENTS.md', 'hooks', '.claude']) await assert.rejects(fs.stat(path.join(b, file)), { code: 'ENOENT' });
    assert.deepEqual(await fs.readdir(home), []);
    for (const result of results) {
      assert.equal(JSON.stringify(result).includes(base), false);
      assert.equal(JSON.stringify(result).includes('root_path'), false);
      assert.equal(JSON.stringify(result).includes('"id"'), false);
    }
    const binding = runtime.sessions.get('a');
    assert.equal(binding.projectUUID, projectA.project.uuid);
    assert.equal(binding.root, await fs.realpath(a));
    assert.equal(binding.instance.nonce, owned.nonce);
    assert.equal(binding.permission.workdir_is_read_only, false);
  });

  await t.test('two simultaneous first initializations in one root are idempotent', async () => {
    const [one, two] = await Promise.all([
      runtime.execute('project_init', ctx(c, 'c-one', { idea: 'First idea', name: 'First' })),
      runtime.execute('project_init', ctx(c, 'c-two', { idea: 'Second idea', name: 'Second' })),
    ]);
    assert.equal(ok(one).project.uuid, ok(two).project.uuid);
    assert.equal(one.data.project.name, two.data.project.name);
    assert.deepEqual(one.data.setup, two.data.setup);
    const shared = [...runtime.connections.values()].find(r => r.sessions.has('c-one'));
    assert.equal(shared.sessions.has('c-two'), true);
    await runtime.execute('runtime_release', ctx(c, 'c-one'));
    assert.equal(shared.sessions.has('c-one'), false);
    assert.equal(shared.sessions.has('c-two'), true);
    assert.equal(ok(await runtime.execute('project_overview', ctx(c, 'c-two'))).project.uuid, two.data.project.uuid);
    assert.equal((await status()).nonce, owned.nonce);
  });

  await t.test('setup uses revisions, finalizes through Manager, repeat init preserves story, assets and specification', async () => {
    let state = ok(await runtime.execute('project_setup', ctx(a, 'a', { action: 'get' })));
    const update = { action: 'update', expected_revision: state.revision, project_name: '回家的星星', generation_language: 'zh-Hans',
      overall_style: '温暖水彩', generation_brief: '帮助别人也找到勇气', picture_book: { format: 'classic_picture_book', aspect_ratio: { mode: 'landscape' }, large_image_minimal_text: false } };
    state = ok(await runtime.execute('project_setup', ctx(a, 'a', update)));
    assert.equal((await runtime.execute('project_setup', ctx(a, 'a', update))).success, false, 'stale revision rejected');
    state = ok(await runtime.execute('project_setup', ctx(a, 'a', { action: 'finalize', expected_revision: state.revision })));
    assert.equal(state.setup_status, 'ready');
    const peer = new MCPConnection({ executable, args: ['mcp', '--data-dir', dataDir], cwd: a, env });
    peers.add(peer); await peer.initialize();
    let p = ok(await peer.overview());
    ok(await peer.call('chapters', { action: 'create', chapter_code: 'vol01.ch01', title: '星星', content: '狐狸抬起头。', expected_revision: p.project.revision }));
    const settings = ok(await peer.call('settings', { action: 'list' }));
    ok(await peer.call('settings', { action: 'create', asset_type: 'character', title: '狐狸', summary: '勇敢的小狐狸', expected_revision: settings.premise.revision }));
    const before = ok(await peer.overview());
    const story = ok(await peer.call('chapters', { action: 'list' }));
    const assets = ok(await peer.call('settings', { action: 'list' }));
    const again = ok(await runtime.execute('project_init', ctx(a, 'a', { idea: 'MUST NOT replace the idea', name: 'MUST NOT rename', generation_language: 'en' })));
    assert.equal(again.project.uuid, before.project.uuid);
    assert.equal(again.project.name, before.project.name);
    assert.equal(again.project.revision, before.project.revision);
    assert.equal(again.setup.revision, before.setup.revision);
    assert.deepEqual(again.setup.final_picture_book, before.setup.final_picture_book);
    assert.deepEqual(ok(await peer.call('chapters', { action: 'list' })), story);
    assert.deepEqual(ok(await peer.call('settings', { action: 'list' })), assets);
    const wrongUUID = projectB.project.uuid;
    for (const tool of ['project_init', 'project_setup', 'project_overview']) {
      const args = tool === 'project_init' ? { idea: 'wrong' } : tool === 'project_setup' ? { action: 'get' } : {};
      const denied = await peer.call(tool, args, { expectedProjectUUID: wrongUUID });
      assert.equal(denied.error.code, 'project_identity_mismatch');
    }
  });

  await t.test('conflicts including dangling symlinks leave only the original files', async () => {
    for (const name of ['README.md', 'STORY.md', 'project.sqlite', 'project.sqlite-wal', 'project.sqlite-shm', 'project.sqlite-journal', '.lumi', 'assets', 'dangling']) {
      const root = await fs.mkdtemp(path.join(base, 'conflict-'));
      if (name === 'dangling') await fs.symlink(path.join(base, 'absent'), path.join(root, 'README.md'));
      else await fs.writeFile(path.join(root, name), 'user original');
      const before = await fs.readdir(root);
      const result = await runtime.execute('project_init', ctx(root, `conflict-${name}`, { idea: 'test' }));
      assert.equal(result.error.code, 'project_initialization_conflict');
      assert.deepEqual(await fs.readdir(root), before);
      if (name !== 'dangling') assert.equal(await fs.readFile(path.join(root, name), 'utf8'), 'user original');
      await runtime.execute('runtime_release', ctx(root, `conflict-${name}`));
    }
  });

  await t.test('permission downgrade, remote paths and caller-selected identities cannot authorize writes', async () => {
    const root = await fs.mkdtemp(path.join(base, 'readonly-'));
    for (const [patch, code] of [[{ workdir_is_read_only: true }, 'read_only_context'], [{ workdir_is_local: false }, 'context_not_local']]) {
      const input = ctx(root, 'denied', { idea: 'no' }); Object.assign(input.context, patch);
      await assert.rejects(runtime.execute('project_init', input), { code });
    }
    for (const args of [{ idea: 'x', root_path: a }, { idea: 'x', project_uuid: projectA.project.uuid }, { idea: 'x', session_context: ctx(a, 'a').context }]) {
      await assert.rejects(runtime.execute('project_init', ctx(root, 'forged', args)), { code: 'invalid_arguments' });
    }
    const downgraded = ctx(a, 'a'); downgraded.context.workdir_is_read_only = true;
    assert.equal(ok(await runtime.execute('project_overview', downgraded)).limitations.read_only, true);
    assert.equal(runtime.sessions.get('a').permission.workdir_is_read_only, true);
    await assert.rejects(runtime.execute('project_init', { ...downgraded, arguments: { idea: 'no' } }), { code: 'read_only_context' });
    assert.deepEqual(await fs.readdir(root), []);
    await fs.chmod(root, 0o555);
    try { await assert.rejects(runtime.execute('project_init', ctx(root, 'os-readonly', { idea: 'no' })), { code: 'workdir_unavailable' }); }
    finally { await fs.chmod(root, 0o755); }
  });

  await t.test('symlink normalization, retargeting and changed working directory require new trusted binding', async () => {
    const alias = path.join(base, 'alias'); await fs.symlink(a, alias);
    assert.equal(ok(await runtime.execute('project_overview', ctx(alias, 'alias'))).project.uuid, projectA.project.uuid);
    await fs.unlink(alias); await fs.symlink(b, alias);
    await assert.rejects(runtime.execute('project_setup', ctx(alias, 'alias', { action: 'get' })), { code: 'binding_changed' });
    assert.equal(ok(await runtime.execute('project_overview', ctx(alias, 'alias'))).project.uuid, projectB.project.uuid);
    await assert.rejects(runtime.execute('project_init', ctx(a, 'alias', { idea: 'changed directory' })), { code: 'binding_changed' });
    assert.equal(ok(await runtime.execute('project_overview', ctx(a, 'alias'))).project.uuid, projectA.project.uuid);
  });

  await t.test('copied projects with identical UUIDs never merge directories', async () => {
    const response = await fetch(`${owned.endpoint}/api/v1/open-projects/${projectB.project.uuid}`, { method: 'DELETE' });
    assert.equal(response.status, 200);
    const copy = path.join(base, 'copy'); await fs.cp(b, copy, { recursive: true });
    assert.equal(ok(await runtime.execute('project_overview', ctx(b, 'b'))).project.uuid, projectB.project.uuid);
    const result = await runtime.execute('project_overview', ctx(copy, 'copy'));
    assert.equal(result.error.code, 'project_identity_mismatch');
    assert.equal(runtime.sessions.get('copy')?.projectUUID, undefined);
    assert.equal(ok(await runtime.execute('project_overview', ctx(b, 'b'))).project.name, 'Bird');
  });

  await t.test('fresh task cannot reuse a live store after a same-UUID copy replaces its pathname', async () => {
    const copy = path.join(base, 'copy'), moved = path.join(base, 'moved-original');
    await runtime.execute('runtime_release', ctx(b, 'b'));
    await fs.rename(b, moved);
    try {
      await fs.rename(copy, b);
      const result = await runtime.execute('project_overview', ctx(b, 'replacement'));
      assert.equal(result.error?.code, 'project_identity_mismatch', JSON.stringify(result));
      assert.equal(runtime.sessions.has('replacement'), false);
      const setup = await runtime.execute('project_setup', ctx(b, 'replacement', { action: 'update', expected_revision: 0, project_name: 'must not rename original' }));
      assert.equal(setup.error?.code, 'project_identity_mismatch', JSON.stringify(setup));
    } finally {
      // Restore before closing the original store's SQLite handles.
      if (await fs.stat(b).catch(() => null)) await fs.rename(b, copy);
      await fs.rename(moved, b);
    }
    const original = ok(await runtime.execute('project_overview', ctx(b, 'b')));
    assert.equal(original.project.name, 'Bird');
    assert.equal(original.project.uuid, projectB.project.uuid);
  });

  await t.test('restart has no cached authority; fresh context reopens the same standard project', async () => {
    await runtime.close();
    const restarted = create();
    assert.equal(restarted.sessions.size, 0);
    const readonly = ctx(b, 'b'); readonly.context.workdir_is_read_only = true;
    assert.equal(ok(await restarted.execute('project_setup', { ...readonly, arguments: { action: 'get' } })).limitations.read_only, true);
    const reopened = ok(await restarted.execute('project_overview', ctx(b, 'b')));
    assert.equal(reopened.project.uuid, projectB.project.uuid);
    assert.equal((await status()).nonce, owned.nonce);
  });
});

test('public status and business errors keep internal paths and instance data in the adapter', () => {
  const uuid = '01989abc-def0-7000-8000-000000000001';
  const data = publicProject({ project: { uuid, name: 'book', revision: 1, id: 1, root_path: '/secret' },
    setup: { project_uuid: uuid, revision: 0, setup_status: 'draft', path: '/secret', reference_plan: { private: '/secret' } }, endpoint: 'http://127.0.0.1:1234' });
  assert.equal(JSON.stringify(data).includes('/secret'), false);
  assert.equal(JSON.stringify(data).includes('"id"'), false);
  const result = publicFailure({ success: false, data: null, error: { code: 'project_locked', message: '/secret', details: '/secret nonce' } });
  assert.equal(result.error.code, 'project_locked');
  assert.equal(JSON.stringify(result).includes('/secret'), false);
  assert.match(result.error.details, /桌面版/);
});
