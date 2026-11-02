'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const { runtimeOptions } = require('../lumi/node/config.cjs');
const { runCLI } = require('../lumi/node/runtime.cjs');

test('build-owned channel configuration separates identities, binaries and data; invalid combinations fail closed', () => {
  const prod = { channel: 'production', environment: 'production', binary: 'lumi', data_dir: null };
  const dev = { channel: 'development', environment: 'development', binary: 'lumi_dev', data_dir: null };
  assert.deepEqual(runtimeOptions(prod, '/bundle', '/isolated-home'), { executable: '/bundle/bin/lumi', environment: 'production', dataDir: '/isolated-home/.lumi' });
  assert.deepEqual(runtimeOptions(dev, '/bundle', '/isolated-home'), { executable: '/bundle/bin/lumi_dev', environment: 'development', dataDir: '/isolated-home/.lumi_dev' });
  assert.equal(runtimeOptions({ ...dev, data_dir: '/custom/data' }, '/bundle').dataDir, '/custom/data');
  for (const bad of [{ ...dev, binary: 'lumi' }, { ...dev, environment: 'production' }, { ...prod, data_dir: '/tmp/data' }, { ...dev, data_dir: '../escape' }]) assert.throws(() => runtimeOptions(bad, '/bundle'));
});

test('generated development bundle is self-contained and its CLI defaults to development without APP_ENV', async t => {
  const author = path.resolve(__dirname, '../../build/cindy-dev/lumi-dev');
  const prod = path.resolve(__dirname, '../lumi');
  const manifest = JSON.parse(await fs.readFile(path.join(author, 'ghost.json')));
  const build = JSON.parse(await fs.readFile(path.join(author, 'build-info.json')));
  const config = JSON.parse(await fs.readFile(path.join(author, 'node/runtime-config.json')));
  assert.equal(manifest.id, 'lumi-dev'); assert.equal(manifest.command, 'lumi-dev');
  assert.equal(JSON.parse(await fs.readFile(path.join(prod, 'ghost.json'))).id, 'lumi');
  assert.equal(build.channel, 'development'); assert.match(build.build_id, /^[a-f0-9]{64}$/);
  for (const name of ['main.js', 'node/runtime.cjs', 'node/worker.cjs', 'manual/authoring/MANUAL.md']) assert.deepEqual(await fs.readFile(path.join(author, name)), await fs.readFile(path.join(prod, name)), name);
  const binary = path.join(author, 'bin/lumi_dev');
  assert.equal((await fs.stat(binary)).mode & 0o777, 0o755);
  assert.equal(await fs.access(path.join(author, 'bin/lumi')).then(() => true, () => false), false);
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-cindy-dev-default-'));
  t.after(() => fs.rm(root, { recursive: true, force: true }));
  const value = await runCLI(binary, ['status', '--data-dir', root, '--json'], { cwd: root, env: { HOME: root, PATH: '/usr/bin:/bin', APP_ENV: 'production' } });
  assert.equal(value.environment, 'development'); assert.equal(value.running, false);
  assert.equal(runtimeOptions(config, author).environment, 'development');
});
