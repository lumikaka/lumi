'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { Runtime, runCLI } = require('../lumi/node/runtime.cjs');
const { runtimeOptions } = require('../lumi/node/config.cjs');

const ok = result => { assert.equal(result.success, true, JSON.stringify(result)); return result.data; };
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));

test('real production and development CLIs preserve independent books across development shutdown and restart', { timeout: 120000 }, async t => {
  const base = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-cindy-channels-'));
  const home = path.join(base, 'home');
  const env = { HOME: home, PATH: '/usr/bin:/bin', TMPDIR: os.tmpdir() };
  const channels = [];
  t.after(async () => {
    const cleaned = await Promise.allSettled(channels.map(async channel => {
        await channel.runtime.close();
        await channel.stop();
    }));
    const observed = await Promise.allSettled(channels.map(c => c.status()));
    // Retain files when cleanup fails, rather than deleting a live server's data.
    if (observed.every(r => r.status === 'fulfilled' && !r.value.running)) {
      await fs.rm(base, { recursive: true, force: true });
    }
    const failure = [...cleaned, ...observed].find(r => r.status === 'rejected');
    if (failure) throw failure.reason;
  });
  await fs.mkdir(home);

  for (const name of ['production', 'development']) {
    const author = path.resolve(__dirname, name === 'production' ? '../lumi' : '../../build/cindy-dev/lumi-dev');
    const config = JSON.parse(await fs.readFile(path.join(author, 'node/runtime-config.json'), 'utf8'));
    const build = JSON.parse(await fs.readFile(path.join(author, 'build-info.json'), 'utf8'));
    assert.equal(config.channel, name);
    // Exercise both packaged binaries without contacting an installed/shared instance.
    const options = runtimeOptions(name === 'development' ? { ...config, data_dir: path.join(base, 'dev-data') } : config, author, home);
    const root = path.join(base, `${name}-book`);
    await fs.mkdir(root);
    const channel = { name, root, options, build, runtime: new Runtime({ ...options, build, env }) };
    channel.status = () => runCLI(options.executable, ['status', '--environment', options.environment, '--data-dir', options.dataDir, '--json'], { cwd: base, env });
    channel.call = (tool, args = {}) => channel.runtime.execute('call_tool', {
      context: { session_id: 'same-task-label', workdir: root, workdir_is_local: true, workdir_is_read_only: false },
      arguments: { name: tool, args },
    });
    channel.stop = async () => {
      const current = await channel.status();
      if (!current.running) return;
      if (channel.identity) assert.equal(current.nonce, channel.identity.nonce, 'stop only this test server generation');
      assert.equal(current.environment, name);
      assert.equal((await fetch(`${current.endpoint}/api/v1/cli-instance`, {
        method: 'DELETE', headers: { 'If-Match': current.nonce },
      })).status, 200);
      for (let i = 0; i < 200; i++) {
        try { if (!(await channel.status()).running) return; }
        catch (error) { if (error.code !== 'instance_unavailable') throw error; }
        await pause(25);
      }
      assert.fail(`${name} test server failed to stop`);
    };
    channels.push(channel);
    assert.equal((await channel.status()).running, false);
  }
  const [prod, dev] = channels;
  assert.notEqual(prod.options.executable, dev.options.executable);
  assert.notEqual(prod.options.dataDir, dev.options.dataDir);
  assert.equal(prod.options.dataDir, path.join(home, '.lumi'));

  await Promise.all(channels.map(async channel => {
    channel.project = ok(await channel.call('project_init', { idea: `Synthetic ${channel.name} book`, name: channel.name }));
    channel.identity = await channel.status();
    assert.equal(channel.identity.running, true);
    assert.equal(channel.identity.environment, channel.name);
    let setup = ok(await channel.call('project_setup', { action: 'get' }));
    setup = ok(await channel.call('project_setup', { action: 'update', expected_revision: setup.revision,
      project_name: channel.name, generation_language: 'en', overall_style: 'Watercolor',
      generation_brief: 'Synthetic channel isolation fixture.',
      picture_book: { format: 'classic_picture_book', aspect_ratio: { mode: 'landscape' } },
    }));
    ok(await channel.call('project_setup', { action: 'finalize', expected_revision: setup.revision }));
    const overview = ok(await channel.call('project_overview'));
    channel.text = `Saved only in ${channel.name}.\nDistinct full text including its final marker: ${channel.name.toUpperCase()}-END`;
    channel.chapter = ok(await channel.call('chapters', { action: 'create', chapter_code: 'vol01.ch01', title: channel.name,
      content: channel.text, expected_revision: overview.project.revision }));
  }));
  assert.notEqual(prod.project.project.uuid, dev.project.project.uuid);
  for (const key of ['pid', 'nonce', 'endpoint']) assert.notEqual(prod.identity[key], dev.identity[key]);

  for (const [owner, foreign] of [[prod, dev], [dev, prod]]) {
    const read = ok(await owner.call('chapters', { action: 'get', chapter_uuid: owner.chapter.uuid }));
    assert.equal(read.current_story.content, owner.text);
    assert.equal((await owner.call('chapters', { action: 'get', chapter_uuid: foreign.chapter.uuid })).success, false);
    assert.equal((await owner.call('chapters', { action: 'write', chapter_uuid: foreign.chapter.uuid,
      content: 'Must never cross channel boundaries.', expected_revision: foreign.chapter.revision })).success, false);
    const unchanged = ok(await owner.call('chapters', { action: 'get', chapter_uuid: owner.chapter.uuid }));
    assert.deepEqual(unchanged, read);
  }
  const baseline = ok(await prod.call('chapters', { action: 'get', chapter_uuid: prod.chapter.uuid }));
  ok(await dev.call('runtime_release'));
  assert.equal((await dev.status()).nonce, dev.identity.nonce, 'release retains the detached service');
  await dev.runtime.close();
  await dev.stop();
  assert.equal((await dev.status()).running, false);
  assert.equal((await prod.status()).nonce, prod.identity.nonce);
  assert.deepEqual(ok(await prod.call('chapters', { action: 'get', chapter_uuid: prod.chapter.uuid })), baseline);

  dev.runtime = new Runtime({ ...dev.options, build: dev.build, env });
  assert.equal(ok(await dev.call('project_overview')).project.uuid, dev.project.project.uuid);
  const replacement = await dev.status();
  assert.equal(replacement.environment, 'development');
  assert.notEqual(replacement.nonce, dev.identity.nonce);
  dev.identity = replacement;
  const restored = ok(await dev.call('chapters', { action: 'get', chapter_uuid: dev.chapter.uuid }));
  assert.equal(restored.current_story.content, dev.text);
  assert.equal(restored.revision, dev.chapter.revision);
  assert.equal((await prod.status()).nonce, prod.identity.nonce);
  assert.deepEqual(ok(await prod.call('chapters', { action: 'get', chapter_uuid: prod.chapter.uuid })), baseline);
  console.log(JSON.stringify({ kind: 'real-cli-channel-isolation-not-cindy-host', books: 2,
    cross_channel_reads_and_writes_rejected: true, development_restart_preserved_text: true,
    production_generation_and_text_unchanged: true }));
});
